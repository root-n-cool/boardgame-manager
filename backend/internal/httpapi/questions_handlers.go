package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"boardgames-manager/internal/ai"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/manuals"
)

// suggester restituisce il generatore per questa richiesta: quello
// iniettato se c'è (i test), altrimenti uno costruito dalle impostazioni.
// Stesso schema di segmenter() e transcriber(), e per la stessa ragione:
// cambiare modello non deve richiedere un riavvio.
func (s *Server) suggester(ctx context.Context) ai.QuestionSuggester {
	if s.Suggester != nil {
		return s.Suggester
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		log.Printf("questions: could not load settings: %v", err)
		return ai.NewHTTPClient("", "", "")
	}
	return ai.NewHTTPClient(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel)
}

// errNoHeadings dice che il gioco non ha nessuna fonte indicizzata, quindi
// non ci sono titoli da cui generare. Distinto dagli errori del provider
// perché il pannello admin deve dire "indicizza prima un manuale" e non
// "riprova".
var errNoHeadings = errors.New("questions: il gioco non ha nessuna fonte indicizzata")

// errNoBGGID: le domande della Strategia partono da nome e descrizione BGG,
// e un gioco inserito a mano non ha né l'una né il forum dietro.
var errNoBGGID = errors.New("questions: il gioco non è collegato a BoardGameGeek")

// questionsAgent legge ?agent= dalle rotte admin. Assente = Manuale, come
// prima di questa funzione; un valore diverso dai due è un errore del client.
func questionsAgent(r *http.Request) (string, bool) {
	a := r.URL.Query().Get("agent")
	if a == "" {
		return manuals.AgentRules, true
	}
	return a, manuals.ValidAgent(a)
}

// regenerateQuestions genera le tre domande e le salva. Con all = true
// sovrascrive anche quelle scritte a mano e azzera i flag (è il pulsante
// "rigenera"); con all = false rispetta le posizioni modificate (è la
// reindicizzazione).
//
// Non scrive niente sulla ResponseWriter: i due chiamanti raccontano
// l'esito in modo diverso — l'indicizzazione lo ignora, il pulsante lo
// riporta all'admin.
func (s *Server) regenerateQuestions(ctx context.Context, gameID int64, agent string, all bool) error {
	// Il nome del gioco lo carica questa funzione, non il chiamante:
	// indexMediaHandler ha in scope solo gameID, e farglielo caricare
	// significherebbe scriverlo due volte per i due chiamanti.
	game, err := s.Games.GetGame(ctx, gameID)
	if err != nil {
		return fmt.Errorf("questions: get game %d: %w", gameID, err)
	}

	var texts []string
	if agent == manuals.AgentStrategy {
		// La Strategia non guarda il manuale indicizzato (non ce l'ha):
		// parte da nome e descrizione BGG, salvati alla creazione del
		// gioco. Senza un BGGID non c'è niente da cui generare.
		if game.BGGID == nil || *game.BGGID == "" {
			return errNoBGGID
		}
		description := ""
		if game.BGGDescription != nil {
			description = *game.BGGDescription
		}
		texts, err = s.suggester(ctx).SuggestStrategyQuestions(ctx, game.Name, description)
	} else {
		summary, sErr := s.Manuals.Summary(ctx, gameID)
		if sErr != nil {
			return fmt.Errorf("questions: summary: %w", sErr)
		}
		if len(summary.Headings) == 0 {
			return errNoHeadings
		}
		texts, err = s.suggester(ctx).SuggestQuestions(ctx, game.Name, summary.Headings)
	}
	if err != nil {
		return err
	}
	if len(texts) != manuals.SuggestedQuestionCount {
		// SuggestQuestions/SuggestStrategyQuestions validano già il
		// conteggio; questo è il controllo che protegge lo store da un
		// finto scritto male nei test.
		return fmt.Errorf("%w: ricevute %d domande", ai.ErrSuggestionsRejected, len(texts))
	}

	if all {
		return s.Manuals.SaveAllQuestions(ctx, gameID, agent, texts)
	}
	return s.Manuals.SaveGeneratedQuestions(ctx, gameID, agent, texts)
}

// allQuestionsEdited dice se le tre domande sono tutte scritte a mano: in
// quel caso una reindicizzazione non ha niente da riscrivere e la chiamata
// al modello si salta del tutto.
func allQuestionsEdited(qs []manuals.SuggestedQuestion) bool {
	if len(qs) < manuals.SuggestedQuestionCount {
		return false
	}
	for _, q := range qs {
		if !q.Edited {
			return false
		}
	}
	return true
}

// suggestedQuestionsResponse manda SEMPRE tre voci, anche per un gioco che
// non ne ha nessuna: il pannello admin ha tre campi da riempire, e uno slot
// vuoto è una voce col testo vuoto, non una voce assente. Senza questo il
// frontend dovrebbe pareggiare la lista da solo.
func suggestedQuestionsResponse(qs []manuals.SuggestedQuestion) map[string]any {
	out := make([]map[string]any, manuals.SuggestedQuestionCount)
	for i := range out {
		out[i] = map[string]any{"text": "", "edited": false}
	}
	for _, q := range qs {
		if q.Position < 0 || q.Position >= manuals.SuggestedQuestionCount {
			continue // una riga fuori range non esiste, ma non deve andare in panic
		}
		out[q.Position] = map[string]any{"text": q.Text, "edited": q.Edited}
	}
	return map[string]any{"questions": out}
}

func (s *Server) getSuggestedQuestionsHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id del gioco non valido")
		return
	}
	agent, ok := questionsAgent(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "agente non valido")
		return
	}
	qs, err := s.Manuals.SuggestedQuestions(r.Context(), gameID, agent)
	if err != nil {
		log.Printf("questions: read for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not read the suggested questions")
		return
	}
	writeJSON(w, http.StatusOK, suggestedQuestionsResponse(qs))
}

func (s *Server) putSuggestedQuestionsHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id del gioco non valido")
		return
	}
	agent, ok := questionsAgent(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "agente non valido")
		return
	}

	var body struct {
		Questions []string `json:"questions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "corpo della richiesta non valido")
		return
	}
	if len(body.Questions) != manuals.SuggestedQuestionCount {
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"servono esattamente %d domande", manuals.SuggestedQuestionCount))
		return
	}
	texts := make([]string, len(body.Questions))
	for i, q := range body.Questions {
		texts[i] = strings.TrimSpace(q)
		if texts[i] == "" {
			// Una domanda vuota finirebbe in un bottone vuoto nella chat.
			writeError(w, http.StatusBadRequest, "nessuna delle tre domande può essere vuota")
			return
		}
		if len([]rune(texts[i])) > ai.MaxSuggestionChars {
			writeError(w, http.StatusBadRequest, fmt.Sprintf(
				"ogni domanda deve stare sotto i %d caratteri", ai.MaxSuggestionChars))
			return
		}
	}

	if err := s.Manuals.SaveEditedQuestions(r.Context(), gameID, agent, texts); err != nil {
		log.Printf("questions: save for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not save the suggested questions")
		return
	}

	qs, err := s.Manuals.SuggestedQuestions(r.Context(), gameID, agent)
	if err != nil {
		log.Printf("questions: read back for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not read the suggested questions")
		return
	}
	writeJSON(w, http.StatusOK, suggestedQuestionsResponse(qs))
}

func (s *Server) regenerateSuggestedQuestionsHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id del gioco non valido")
		return
	}
	agent, ok := questionsAgent(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "agente non valido")
		return
	}
	// Il 404 su un gioco inesistente prima di qualunque lavoro:
	// regenerateQuestions ricaricherà il gioco per il suo nome, ma un id
	// inventato deve rispondere 404 e non un errore del provider. Un guasto
	// DB vero, invece, non è "gioco non trovato": è un 500, come fa
	// getGameHandler.
	if _, err := s.Games.GetGame(r.Context(), gameID); err != nil {
		if errors.Is(err, games.ErrNotFound) {
			writeError(w, http.StatusNotFound, "gioco non trovato")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load game")
		return
	}

	// A differenza dell'indicizzazione, qui l'esito si racconta: è un
	// pulsante premuto a mano, e chi lo preme deve sapere se ha funzionato.
	switch err := s.regenerateQuestions(r.Context(), gameID, agent, true); {
	case err == nil:
	case errors.Is(err, errNoHeadings):
		writeError(w, http.StatusUnprocessableEntity,
			"Per generare le domande serve un manuale già indicizzato: indicizza prima un documento nella sezione Chatbot.")
		return
	case errors.Is(err, errNoBGGID):
		writeError(w, http.StatusUnprocessableEntity,
			"Le domande per la Strategia servono solo ai giochi collegati a BoardGameGeek.")
		return
	case errors.Is(err, ai.ErrNotConfigured):
		writeError(w, http.StatusUnprocessableEntity,
			"Nessun provider AI configurato: controlla le impostazioni.")
		return
	case errors.Is(err, ai.ErrSuggestionsRejected):
		writeError(w, http.StatusUnprocessableEntity,
			"Il modello non ha risposto con tre domande valide: riprova.")
		return
	default:
		log.Printf("questions: regenerate for game %d: %v", gameID, err)
		writeError(w, http.StatusBadGateway, "Il provider AI non ha risposto: riprova.")
		return
	}

	qs, err := s.Manuals.SuggestedQuestions(r.Context(), gameID, agent)
	if err != nil {
		log.Printf("questions: read back for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not read the suggested questions")
		return
	}
	writeJSON(w, http.StatusOK, suggestedQuestionsResponse(qs))
}
