package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"boardgames-manager/internal/ai"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/manuals"
)

// asker restituisce l'agente per questa richiesta: quello iniettato se c'è,
// altrimenti uno costruito dalle impostazioni. Stesso schema di
// translator() e transcriber().
func (s *Server) asker(ctx context.Context) ai.Asker {
	if s.Asker != nil {
		return s.Asker
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		log.Printf("ask: could not load settings: %v", err)
		return ai.NewHTTPClient("", "", "")
	}
	return ai.NewHTTPClient(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel)
}

// aiConfigured dice se un provider è impostato, senza fare richieste. Serve
// a canAsk: la scheda gioco deve sapere se mostrare la chat prima che
// qualcuno faccia una domanda.
func (s *Server) aiConfigured(ctx context.Context) bool {
	if s.Asker != nil {
		return true
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		return false
	}
	return cfg.AIBaseURL != "" && cfg.AIAPIKey != "" && cfg.AIModel != ""
}

// askHTTPRequest è la forma che manda deep-chat: la conversazione intera,
// tagliata dal componente a requestBodyLimits.maxMessages.
type askHTTPRequest struct {
	Messages []struct {
		Role string `json:"role"`
		Text string `json:"text"`
	} `json:"messages"`
}

// I tre tetti sotto esistono perché questa è l'unica rotta del progetto che
// trasforma byte anonimi in denaro: è pubblica, non autenticata, e quel che
// arriva finisce dentro il prompt di un provider che si paga a token. Il
// rate limit (20/min per IP) limita la frequenza, non la dimensione: senza
// questi tetti una singola richiesta da 50 MB, o cento turni da 100 KB,
// sarebbero una richiesta legittima.
//
// I valori sono larghi rispetto all'uso vero — una domanda sulle regole
// battuta al telefono sta in poche centinaia di caratteri, e deep-chat manda
// al massimo gli ultimi maxMessages turni — e stretti rispetto all'abuso.
const (
	askMaxBodyBytes  = 128 << 10 // 128 KB di JSON
	askMaxTurns      = 30
	askMaxTurnChars  = 4000
	askTotalMaxChars = 24000
)

// trimTurns riduce la conversazione ai tetti, tagliando dalla TESTA: la
// domanda appena scritta è l'ultimo messaggio, quindi a cadere è il contesto
// più vecchio, mai la domanda. Un turno singolo non può da solo sforare il
// totale (askMaxTurnChars è molto minore di askTotalMaxChars), quindi
// l'ultimo turno sopravvive sempre.
func trimTurns(turns []ai.Turn) []ai.Turn {
	if len(turns) > askMaxTurns {
		turns = turns[len(turns)-askMaxTurns:]
	}
	total := 0
	for i := len(turns) - 1; i >= 0; i-- {
		total += len(turns[i].Text)
		if total > askTotalMaxChars {
			return turns[i+1:]
		}
	}
	return turns
}

func (s *Server) askHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, askMaxBodyBytes)
	var body askHTTPRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// MaxBytesReader fa fallire il Decode con un errore suo: per chi
		// scrive dal tavolo la differenza fra "JSON malformato" e "troppo
		// lungo" non cambia nulla, la risposta è la stessa.
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	turns := make([]ai.Turn, 0, len(body.Messages))
	for _, m := range body.Messages {
		text := strings.TrimSpace(m.Text)
		if text == "" {
			continue
		}
		if len(text) > askMaxTurnChars {
			text = text[:askMaxTurnChars]
		}
		// deep-chat chiama "ai" quel che il formato OpenAI chiama
		// "assistant": la traduzione va fatta qui, una volta.
		role := "user"
		if m.Role == "ai" || m.Role == "assistant" {
			role = "assistant"
		}
		turns = append(turns, ai.Turn{Role: role, Text: text})
	}
	turns = trimTurns(turns)
	if len(turns) == 0 {
		writeError(w, http.StatusBadRequest, "serve una domanda")
		return
	}

	game, err := s.Games.GetGame(r.Context(), gameID)
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game")
		return
	}

	hasChunks, err := s.Manuals.HasChunks(r.Context(), gameID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load the manual")
		return
	}
	// Nessuna fonte indicizzata: la rotta si comporta come inesistente,
	// esattamente come senza provider AI. Non c'è nulla da spiegare a un
	// partecipante — la chat, in quel caso, non è nemmeno comparsa.
	if !hasChunks {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	// La lingua preferita è quella base del gioco: è quella in cui la
	// scheda pubblica mostra tutto il resto. Nella stessa lettura si
	// raccolgono anche i percorsi dei manuali PDF del gioco (in ogni
	// lingua): linkifyCitations ne ha bisogno per il suo link "pag. N",
	// ed è la stessa informazione che prima veniva da manuals.Corpus,
	// tipo cancellato dal Task 1.
	preferLang := "it"
	var pdfPaths []string
	if langs, err := s.Games.ListLanguages(r.Context(), gameID); err == nil {
		for _, l := range langs {
			if l.IsBaseLanguage {
				preferLang = l.LanguageCode
			}
			media, err := s.Games.ListMedia(r.Context(), l.ID)
			if err != nil {
				continue
			}
			for _, m := range media {
				if m.Type == games.MediaTypeFile && strings.HasSuffix(strings.ToLower(m.URLOrPath), ".pdf") {
					pdfPaths = append(pdfPaths, m.URLOrPath)
				}
			}
		}
	}

	// La closure di ricerca è legata al gioco: il game_id NON è un
	// parametro del tool, così il modello non può leggere il manuale di un
	// altro gioco.
	search := func(ctx context.Context, keywords []string) (string, error) {
		hits, missing, err := s.Manuals.Search(ctx, gameID, preferLang, keywords)
		if err != nil {
			return "", err
		}
		return manuals.MarshalHits(hits, missing)
	}

	answer, err := s.asker(r.Context()).Ask(r.Context(), ai.AskRequest{
		GameName: game.Name,
		Turns:    turns,
		// CorpusIndex resta vuoto: l'indice per fonte (i titoli di
		// sezione, raggruppati per manuale) è il Task 7, che ha anche
		// l'accesso a manuals.Store.Summary per costruirlo.
		// TODO(task-7): l'indice per fonte.
		CorpusIndex: "",
		Search:      search,
	})
	if errors.Is(err, ai.ErrNotConfigured) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		log.Printf("ask about game %d: %v", gameID, err)
		writeError(w, http.StatusBadGateway,
			"Non riesco a rispondere in questo momento. Riprova, o guarda il manuale nella scheda del gioco.")
		return
	}

	// deep-chat legge {"text": ...}.
	writeJSON(w, http.StatusOK, map[string]any{"text": linkifyCitations(answer, pdfPaths)})
}

// citationRe trova le citazioni di pagina nella risposta del modello.
var citationRe = regexp.MustCompile(`pag\.\s*(\d+)`)

// linkifyCitations trasforma "pag. 7" in un link markdown al PDF aperto a
// quella pagina. È il pezzo che chiude il cerchio: una risposta generata
// non va creduta sulla fiducia, si apre il manuale e si verifica — e in una
// discussione sulle regole è la differenza fra un aiuto e un oracolo.
//
// La riscrittura è nostra e non del modello: chiedere a un modello
// economico di costruire URL corretti è un modo affidabile di ottenere URL
// sbagliati. Il fragment #page=N è onorato dalla quasi totalità dei viewer.
//
// Con PIÙ di un manuale PDF non si linka niente, e la citazione resta testo
// semplice. Il motivo, prima che qualcuno lo "aggiusti" al contrario: la
// riscrittura è cieca al manuale a cui la citazione si riferisce — sostituisce
// ogni "pag. N" della risposta con lo stesso file. La ricerca però restituisce
// davvero risultati da entrambe le lingue, e il modello scrive frasi come
// "il regolamento inglese, pag. 12". Un link così porterebbe a pagina 12 del
// manuale ITALIANO: chi lo apre per verificare trova un'altra regola e
// conclude che la risposta è inventata. Un link sbagliato è peggio di nessun
// link, perché distrugge esattamente la fiducia per cui il link esiste.
// Legare la citazione al manuale giusto vorrebbe dire far dichiarare al
// modello quale manuale sta citando (o riconoscerne il titolo nel testo): è
// la strada giusta, ma è una feature, non una correzione.
//
// pdfPaths sostituisce quello che prima veniva da manuals.Corpus (tipo
// cancellato dal Task 1): i percorsi di tutti i manuali PDF del gioco, in
// qualunque lingua. La funzione stessa non cambia: brutta e sbagliata con
// due manuali come lo era prima — sistemarla è il Task 7, un rifacimento a
// metà qui sarebbe peggio di nessuno.
func linkifyCitations(answer string, pdfPaths []string) string {
	// Se il modello ha già prodotto un link, non si raddoppia.
	if strings.Contains(answer, "](/api/uploads/") {
		return answer
	}

	path := ""
	pdfs := 0
	for _, p := range pdfPaths {
		pdfs++
		path = p
	}
	if path == "" || pdfs > 1 {
		return answer
	}

	return citationRe.ReplaceAllStringFunc(answer, func(match string) string {
		page := citationRe.FindStringSubmatch(match)[1]
		return fmt.Sprintf("[%s](/api/uploads/%s#page=%s)", match, path, page)
	})
}
