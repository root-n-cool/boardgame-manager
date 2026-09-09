package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"boardgames-manager/internal/ai"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/manuals"
)

// minAvgUsableCharsPerPage separa un layer testo che è davvero la prosa
// del manuale da uno che è solo rumore dello scanner (un'intestazione o un
// numero di pagina impressi su una pagina altrimenti scansionata).
//
// È una media per pagina, non un totale: un totale non scala. Una
// scansione di quattro pagine la cui unica "estrazione" è un'intestazione
// ripetuta tipo "Wingspan — Regolamento" (~22 caratteri) somma a ~88
// caratteri, che un tetto sul totale di 20 chiamerebbe "usabile" a
// prescindere da quante pagine ha la scansione — è la scala che tradisce
// la soglia, non la soglia in sé. Mediando invece il segnale per pagina
// resta stabile: un'intestazione o un rumore restano sulle decine di
// caratteri qualunque sia il numero di pagine, mentre una vera pagina di
// manuale (un paragrafo o più di regole) media sulle centinaia. 100 sta
// comodamente sopra il tetto del rumore e comodamente sotto la prosa vera.
//
// Una pagina vera ma davvero corta (un cartoncino di riferimento di una
// pagina) può restare sotto questa media — è previsto, e gestito altrove:
// extractManualHandler ripiega comunque su quel testo (per quanto debole)
// invece di rispondere con un errore, quando il percorso vision non ha
// nemmeno un'immagine da trascrivere (vedi il commento lì).
const minAvgUsableCharsPerPage = 100

// transcriber restituisce il trascrittore per questa richiesta: quello
// iniettato se c'è (i test), altrimenti uno costruito dalle impostazioni.
// Stesso schema di translator() in translate.go, e per la stessa ragione:
// cambiare modello non deve richiedere un riavvio.
func (s *Server) transcriber(ctx context.Context) ai.Transcriber {
	if s.Vision != nil {
		return s.Vision
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		log.Printf("manuals: could not load settings: %v", err)
		return ai.NewHTTPClientWithVision("", "", "", "")
	}
	return ai.NewHTTPClientWithVision(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel, cfg.AIVisionModel)
}

// manualTarget risolve i tre parametri di rotta in un manuale concreto, e
// verifica che il media appartenga davvero a quel gioco e a quella lingua:
// senza il controllo, l'id di un media di un altro gioco passerebbe.
//
// Un caso "non trovato" (lingua o media inesistenti per questo gioco)
// avvolge games.ErrNotFound, cosicché writeManualTargetError possa
// distinguerlo da un parametro di rotta malformato: stesso schema di
// translateLanguageHandler (translate.go), che differenzia allo stesso
// modo per lo stesso genere di lookup.
func (s *Server) manualTarget(r *http.Request) (gameID int64, mediaID int64, lang string, media games.GameMedia, err error) {
	gameID, err = parseIDParam(r, "id")
	if err != nil {
		return 0, 0, "", games.GameMedia{}, errors.New("invalid game id")
	}
	mediaID, err = parseIDParam(r, "mediaId")
	if err != nil {
		return 0, 0, "", games.GameMedia{}, errors.New("invalid media id")
	}
	lang = strings.ToLower(strings.TrimSpace(chi.URLParam(r, "lang")))
	if lang == "" {
		return 0, 0, "", games.GameMedia{}, errors.New("language code is required")
	}

	gl, err := s.Games.GetLanguage(r.Context(), gameID, lang)
	if errors.Is(err, games.ErrNotFound) {
		return 0, 0, "", games.GameMedia{}, fmt.Errorf("language not found: %w", games.ErrNotFound)
	}
	if err != nil {
		return 0, 0, "", games.GameMedia{}, errors.New("could not load language")
	}
	list, err := s.Games.ListMedia(r.Context(), gl.ID)
	if err != nil {
		return 0, 0, "", games.GameMedia{}, errors.New("could not load media")
	}
	for _, m := range list {
		if m.ID == mediaID {
			return gameID, mediaID, lang, m, nil
		}
	}
	return 0, 0, "", games.GameMedia{}, fmt.Errorf("media not found for this game and language: %w", games.ErrNotFound)
}

// writeManualTargetError traduce l'errore di manualTarget nello status
// giusto: 404 per un gioco/lingua/media che davvero non esiste (come fa
// translateLanguageHandler per lo stesso genere di lookup), 400 per un
// parametro di rotta malformato (id non numerico, lingua mancante).
func writeManualTargetError(w http.ResponseWriter, err error) {
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}

type manualPageResponse struct {
	PageNumber int    `json:"pageNumber"`
	Text       string `json:"text"`
	Heading    string `json:"heading"`
	Source     string `json:"source"`
}

// averageUsableTextChars è la media dei caratteri di testo (dopo trim) per
// pagina: è la misura con cui extractManualHandler decide se il layer
// testo letto da ExtractText vale davvero, o se è meglio ripiegare su
// vision. Vedi il commento su minAvgUsableCharsPerPage per il perché di
// una media e non di un totale.
func averageUsableTextChars(pages []manuals.Page) float64 {
	if len(pages) == 0 {
		return 0
	}
	total := 0
	for _, p := range pages {
		total += len(strings.TrimSpace(p.Text))
	}
	return float64(total) / float64(len(pages))
}

// pdfTextPageResponses converte le Page di manuals.ExtractText nella forma
// di risposta, rilevando l'heading di ciascuna. Usata sia dal percorso
// primario (testo sopra soglia) sia dal ripiego (testo debole ma nessuna
// immagine da trascrivere): lo stesso mapping, non due copie.
func pdfTextPageResponses(pages []manuals.Page) []manualPageResponse {
	out := make([]manualPageResponse, 0, len(pages))
	for _, p := range pages {
		out = append(out, manualPageResponse{
			PageNumber: p.Number, Text: p.Text,
			Heading: manuals.DetectHeading(p.Text), Source: "pdf_text",
		})
	}
	return out
}

// extractManualHandler propone il testo di un manuale senza salvarlo. Non
// salva di proposito: l'admin conferma sempre, come già per
// l'arricchimento BGG.
//
// Il percorso si sceglie in tre passi, non due:
//  1. Se HasTextLayer dice che c'è un layer testo, si prova ExtractText.
//     Se il risultato è buono in media (vedi minAvgUsableCharsPerPage) è
//     la risposta: nessun bisogno di vision.
//  2. Altrimenti (nessun layer testo, ExtractText fallito, o testo troppo
//     debole in media) si prova il percorso vision. Due ragioni concrete
//     per cui "debole" conta quanto "assente": ExtractText si ferma alla
//     prima pagina corrotta (limite noto della libreria, vedi il suo
//     commento), e un PDF i cui font vivono in un object stream compresso
//     risponde false a HasTextLayer pur avendo un vero layer testo.
//  3. Se il percorso vision non trova nemmeno un'immagine da trascrivere
//     (ExtractPageImages torna vuoto) ma il passo 1 aveva comunque estratto
//     del testo, per quanto debole in media, quel testo diventa la
//     risposta invece di un errore: è il caso di un PDF di solo testo
//     davvero corto (un cartoncino di riferimento di una pagina), che senza
//     questo ripiego finirebbe rifiutato nonostante avesse un contenuto
//     vero e leggibile. Un errore va restituito solo quando *nessuno* dei
//     due percorsi ha prodotto niente: quello sì è un file davvero
//     inutilizzabile.
func (s *Server) extractManualHandler(w http.ResponseWriter, r *http.Request) {
	_, _, _, media, err := s.manualTarget(r)
	if err != nil {
		writeManualTargetError(w, err)
		return
	}
	if media.Type != games.MediaTypeFile || !strings.HasSuffix(strings.ToLower(media.URLOrPath), ".pdf") {
		writeError(w, http.StatusConflict, "questo media non è un manuale PDF")
		return
	}

	f, err := s.Storage.Open(media.URLOrPath)
	if err != nil {
		log.Printf("manuals: open %s: %v", media.URLOrPath, err)
		writeError(w, http.StatusNotFound, "il file del manuale non è più sul disco")
		return
	}
	raw, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		log.Printf("manuals: read %s: %v", media.URLOrPath, err)
		writeError(w, http.StatusInternalServerError, "non è stato possibile leggere il file del manuale")
		return
	}

	// extractedPages tiene il risultato di ExtractText anche quando è
	// troppo debole in media per essere la risposta primaria: serve come
	// ripiego al passo 3, se il percorso vision non trova nessuna pagina
	// da trascrivere.
	var extractedPages []manuals.Page
	if manuals.HasTextLayer(raw) {
		pages, textErr := manuals.ExtractText(raw)
		if textErr != nil {
			log.Printf("manuals: extract text: %v", textErr)
		} else {
			extractedPages = pages
			if averageUsableTextChars(pages) >= minAvgUsableCharsPerPage {
				writeJSON(w, http.StatusOK, map[string]any{
					"source": "pdf_text", "pages": pdfTextPageResponses(pages),
				})
				return
			}
			// Nessun return: la media è troppo bassa per fidarsene come
			// risposta primaria, ma extractedPages resta come ripiego più
			// sotto se vision non trova immagini.
		}
	}

	// ExtractPageImages non restituisce mai un errore: una pagina illeggibile
	// viene saltata e il resto del file continua a essere estratto, quindi
	// "non ho trovato immagini" arriva sempre come lista vuota. Il ramo
	// `if err != nil` che stava qui era morto, e se fosse mai tornato in vita
	// avrebbe risposto 422 senza guardare extractedPages — contro la regola
	// per cui il 422 si dà solo quando NESSUNO dei due percorsi ha prodotto
	// qualcosa. La lista vuota qui sotto è l'unico punto in cui si decide.
	images, _ := manuals.ExtractPageImages(raw)
	if len(images) == 0 {
		if len(extractedPages) > 0 {
			// Nessuna immagine da trascrivere, ma un po' di testo vero
			// (per quanto debole in media) c'è: è meglio di un errore, e
			// l'admin lo legge e lo corregge dove serve.
			writeJSON(w, http.StatusOK, map[string]any{
				"source": "pdf_text", "pages": pdfTextPageResponses(extractedPages),
			})
			return
		}
		writeError(w, http.StatusUnprocessableEntity,
			"questo PDF non ha né testo né pagine leggibili: puoi scrivere il testo a mano")
		return
	}

	// Una richiesta per pagina, non tutte insieme: se la pagina 3 fallisce
	// non si perdono le altre, e l'admin riprova solo quella.
	vision := s.transcriber(r.Context())
	out := make([]manualPageResponse, 0, len(images))
	for _, img := range images {
		text, err := vision.Transcribe(r.Context(), img.JPEG, img.Number)
		if err != nil {
			// Senza modello vision, o con un guasto del provider, la pagina
			// esce vuota: l'anteprima si apre comunque e si riempie a mano.
			if !errors.Is(err, ai.ErrNotConfigured) {
				log.Printf("manuals: transcribe page %d: %v", img.Number, err)
			}
			out = append(out, manualPageResponse{PageNumber: img.Number, Source: "manual"})
			continue
		}
		out = append(out, manualPageResponse{
			PageNumber: img.Number, Text: text,
			Heading: manuals.DetectHeading(text), Source: "vision",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "vision", "pages": out})
}

func (s *Server) listManualPagesHandler(w http.ResponseWriter, r *http.Request) {
	_, mediaID, _, _, err := s.manualTarget(r)
	if err != nil {
		writeManualTargetError(w, err)
		return
	}
	pages, err := s.Manuals.ListPages(r.Context(), mediaID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load manual pages")
		return
	}
	out := make([]manualPageResponse, 0, len(pages))
	for _, p := range pages {
		out = append(out, manualPageResponse{
			PageNumber: p.PageNumber, Text: p.Text, Heading: p.Heading, Source: p.Source,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": out})
}

type putManualPagesRequest struct {
	Pages []struct {
		PageNumber int    `json:"pageNumber"`
		Text       string `json:"text"`
		Source     string `json:"source"`
	} `json:"pages"`
}

// putManualPagesHandler salva le pagine confermate (o corrette a mano)
// dall'admin: è l'unico punto che scrive manual_page, e ReplacePages
// ricostruisce anche i chunk cercabili nella stessa transazione, quindi la
// ricerca funziona subito dopo, senza un passaggio separato.
func (s *Server) putManualPagesHandler(w http.ResponseWriter, r *http.Request) {
	gameID, mediaID, lang, _, err := s.manualTarget(r)
	if err != nil {
		writeManualTargetError(w, err)
		return
	}
	var body putManualPagesRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	pages := make([]manuals.StoredPage, 0, len(body.Pages))
	for _, p := range body.Pages {
		if p.PageNumber < 1 {
			writeError(w, http.StatusBadRequest, "il numero di pagina parte da 1")
			return
		}
		source := p.Source
		switch source {
		case "pdf_text", "vision", "manual":
		default:
			// Una pagina scritta o corretta a mano è "manual": è il default
			// più onesto quando il client non lo dice.
			source = "manual"
		}
		pages = append(pages, manuals.StoredPage{
			PageNumber: p.PageNumber, Text: p.Text, Source: source,
		})
	}

	if err := s.Manuals.ReplacePages(r.Context(), gameID, mediaID, lang, pages); err != nil {
		log.Printf("manuals: replace pages of media %d: %v", mediaID, err)
		writeError(w, http.StatusInternalServerError, "could not save manual pages")
		return
	}
	s.listManualPagesHandler(w, r)
}

func (s *Server) deleteManualPagesHandler(w http.ResponseWriter, r *http.Request) {
	_, mediaID, _, _, err := s.manualTarget(r)
	if err != nil {
		writeManualTargetError(w, err)
		return
	}
	if err := s.Manuals.DeletePages(r.Context(), mediaID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete manual pages")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
