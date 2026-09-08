package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"boardgames-manager/internal/ai"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/manuals"
)

// minUsableExtractedChars è la soglia sotto la quale il testo letto da
// ExtractText si considera "quasi vuoto" e non un'estrazione riuscita.
// Un paragrafo vero di manuale, anche breve, sta ben sopra: una singola
// riga di titolo ("Fase di Upkeep") supera già questa soglia. Il valore è
// deliberatamente basso, non alto: l'errore da evitare è quello grave —
// dire "va bene" a un'estrazione che in realtà non ha preso niente, e
// perdere così in silenzio il percorso vision — non quello lieve di
// mandare in più a vision un manuale che aveva davvero pochissimo testo
// leggibile.
const minUsableExtractedChars = 20

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
	if err != nil {
		return 0, 0, "", games.GameMedia{}, errors.New("language not found")
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
	return 0, 0, "", games.GameMedia{}, errors.New("media not found for this game and language")
}

type manualPageResponse struct {
	PageNumber int    `json:"pageNumber"`
	Text       string `json:"text"`
	Heading    string `json:"heading"`
	Source     string `json:"source"`
}

// usableTextChars somma i caratteri di testo (dopo trim) su tutte le
// pagine: è la misura con cui extractManualHandler decide se il layer
// testo letto da ExtractText vale davvero, o se è meglio ripiegare su
// vision.
func usableTextChars(pages []manuals.Page) int {
	n := 0
	for _, p := range pages {
		n += len(strings.TrimSpace(p.Text))
	}
	return n
}

// extractManualHandler propone il testo di un manuale senza salvarlo. Non
// salva di proposito: l'admin conferma sempre, come già per
// l'arricchimento BGG.
//
// Il percorso si sceglie così: se HasTextLayer dice che c'è un layer
// testo, si prova ExtractText; ma se quel tentativo fallisce o produce
// solo qualche carattere (vedi minUsableExtractedChars), si ripiega sul
// percorso vision invece di rispondere con pagine vuote. Due ragioni
// concrete, non ipotetiche: ExtractText si ferma alla prima pagina
// corrotta (limite noto della libreria, vedi il suo commento), e un PDF i
// cui font vivono in un object stream compresso risponde false a
// HasTextLayer pur avendo un vero layer testo — casi diversi, stesso
// rimedio. Il vision funziona su qualunque PDF e nel peggiore dei casi
// costa solo qualche chiamata in più al modello: un manuale che finisce
// muto perché si è scelto il ramo testo per errore è il guasto peggiore.
func (s *Server) extractManualHandler(w http.ResponseWriter, r *http.Request) {
	_, _, _, media, err := s.manualTarget(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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

	if manuals.HasTextLayer(raw) {
		pages, textErr := manuals.ExtractText(raw)
		if textErr != nil {
			log.Printf("manuals: extract text: %v", textErr)
		} else if usableTextChars(pages) >= minUsableExtractedChars {
			out := make([]manualPageResponse, 0, len(pages))
			for _, p := range pages {
				out = append(out, manualPageResponse{
					PageNumber: p.Number, Text: p.Text,
					Heading: manuals.DetectHeading(p.Text), Source: "pdf_text",
				})
			}
			writeJSON(w, http.StatusOK, map[string]any{"source": "pdf_text", "pages": out})
			return
		}
		// Nessun return sopra: un errore di ExtractText, o un'estrazione
		// che ha prodotto troppo poco testo per essere vera, cadono
		// entrambi qui e proseguono sul percorso vision sotto.
	}

	images, err := manuals.ExtractPageImages(raw)
	if err != nil {
		log.Printf("manuals: extract images: %v", err)
		writeError(w, http.StatusUnprocessableEntity,
			"non è stato possibile leggere le pagine di questo PDF: puoi scrivere il testo a mano")
		return
	}
	if len(images) == 0 {
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
		writeError(w, http.StatusBadRequest, err.Error())
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
		writeError(w, http.StatusBadRequest, err.Error())
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
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Manuals.DeletePages(r.Context(), mediaID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete manual pages")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
