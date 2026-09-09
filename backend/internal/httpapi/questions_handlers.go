package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log"

	"boardgames-manager/internal/ai"
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

// regenerateQuestions genera le tre domande e le salva. Con all = true
// sovrascrive anche quelle scritte a mano e azzera i flag (è il pulsante
// "rigenera"); con all = false rispetta le posizioni modificate (è la
// reindicizzazione).
//
// Non scrive niente sulla ResponseWriter: i due chiamanti raccontano
// l'esito in modo diverso — l'indicizzazione lo ignora, il pulsante lo
// riporta all'admin.
func (s *Server) regenerateQuestions(ctx context.Context, gameID int64, all bool) error {
	// Il nome del gioco lo carica questa funzione, non il chiamante:
	// indexMediaHandler ha in scope solo gameID, e farglielo caricare
	// significherebbe scriverlo due volte per i due chiamanti.
	game, err := s.Games.GetGame(ctx, gameID)
	if err != nil {
		return fmt.Errorf("questions: get game %d: %w", gameID, err)
	}

	summary, err := s.Manuals.Summary(ctx, gameID)
	if err != nil {
		return fmt.Errorf("questions: summary: %w", err)
	}
	if len(summary.Headings) == 0 {
		return errNoHeadings
	}

	texts, err := s.suggester(ctx).SuggestQuestions(ctx, game.Name, summary.Headings)
	if err != nil {
		return err
	}
	if len(texts) != manuals.SuggestedQuestionCount {
		// SuggestQuestions valida già il conteggio; questo è il controllo
		// che protegge lo store da un finto scritto male nei test.
		return fmt.Errorf("%w: ricevute %d domande", ai.ErrSuggestionsRejected, len(texts))
	}

	if all {
		return s.Manuals.SaveAllQuestions(ctx, gameID, texts)
	}
	return s.Manuals.SaveGeneratedQuestions(ctx, gameID, texts)
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
