package manuals

import (
	"context"
	"database/sql"
	"fmt"
)

// SuggestedQuestionCount è quante domande suggerite ha un gioco: tre, come
// i tre bottoni nello stato di riposo della chat. Non è configurabile —
// vedi i non-obiettivi della spec.
const SuggestedQuestionCount = 3

// SuggestedQuestion è una delle tre domande mostrate nello stato di riposo
// della chat. Edited dice che l'ha riscritta l'admin: è il flag che
// protegge il suo lavoro da una reindicizzazione.
type SuggestedQuestion struct {
	Position int
	Text     string
	Edited   bool
}

// SuggestedQuestions restituisce le domande di un gioco in ordine di
// posizione. Una lista vuota è un esito normale: un gioco mai indicizzato
// non ne ha, e il frontend ripiega sulle domande fisse.
func (s *Store) SuggestedQuestions(ctx context.Context, gameID int64) ([]SuggestedQuestion, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT position, text, edited FROM game_suggested_question
		 WHERE game_id = ? ORDER BY position`, gameID)
	if err != nil {
		return nil, fmt.Errorf("suggested questions: %w", err)
	}
	defer rows.Close()

	var out []SuggestedQuestion
	for rows.Next() {
		var q SuggestedQuestion
		if err := rows.Scan(&q.Position, &q.Text, &q.Edited); err != nil {
			return nil, fmt.Errorf("scan suggested question: %w", err)
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// SaveGeneratedQuestions scrive le domande prodotte dal modello SALTANDO le
// posizioni che l'admin ha riscritto a mano. È il percorso della
// reindicizzazione: uno scan migliore aggiorna le domande da sé, il lavoro
// manuale non si perde mai.
//
// La clausola WHERE excluded.edited = 0 non basterebbe: excluded è la riga
// in arrivo, che ha sempre edited = 0. Il confronto va fatto sulla riga
// ESISTENTE, ed è per questo che l'upsert ha la sua condizione.
func (s *Store) SaveGeneratedQuestions(ctx context.Context, gameID int64, texts []string) error {
	return s.saveQuestions(ctx, gameID, texts,
		`INSERT INTO game_suggested_question (game_id, position, text, edited)
		 VALUES (?, ?, ?, 0)
		 ON CONFLICT(game_id, position) DO UPDATE SET text = excluded.text
		 WHERE game_suggested_question.edited = 0`)
}

// SaveAllQuestions sovrascrive tutte e tre le domande e azzera edited: è il
// pulsante "rigenera", premuto deliberatamente dall'admin. Se lo premi, lo
// stai chiedendo — anche per le domande che avevi scritto a mano.
func (s *Store) SaveAllQuestions(ctx context.Context, gameID int64, texts []string) error {
	return s.saveQuestions(ctx, gameID, texts,
		`INSERT INTO game_suggested_question (game_id, position, text, edited)
		 VALUES (?, ?, ?, 0)
		 ON CONFLICT(game_id, position) DO UPDATE SET text = excluded.text, edited = 0`)
}

// SaveEditedQuestions salva i testi che arrivano dal pannello admin,
// marcando edited SOLO quelli diversi da ciò che era salvato. Il confronto
// lo fa il server e non un flag del client: solo il server sa cosa aveva
// scritto il modello, e un client che rimanda invariata una domanda
// generata non deve poterla promuovere a "scritta a mano" — altrimenti un
// salvataggio senza modifiche congelerebbe le tre domande per sempre.
//
// È un upsert perché un gioco mai indicizzato non ha righe: l'admin deve
// poter scrivere le sue tre domande prima (o invece) di ogni generazione, e
// quelle nascono edited.
func (s *Store) SaveEditedQuestions(ctx context.Context, gameID int64, texts []string) error {
	if len(texts) != SuggestedQuestionCount {
		return fmt.Errorf("suggested questions: attesi %d testi, ricevuti %d",
			SuggestedQuestionCount, len(texts))
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	for i, text := range texts {
		// Il testo attuale, se c'è: serve a decidere se questo salvataggio
		// è una modifica o un no-op. exists distingue "nessuna riga" da
		// "la riga esiste e vale stringa vuota" — confonderli è il bug del
		// round 1: un testo in arrivo vuoto su una posizione senza riga
		// veniva scambiato per "invariato" (current defaultava a ""), e
		// l'UPDATE che ne seguiva non creava mai la riga mancante.
		var current string
		err := tx.QueryRowContext(ctx,
			`SELECT text FROM game_suggested_question WHERE game_id = ? AND position = ?`,
			gameID, i).Scan(&current)
		exists := true
		switch {
		case err == sql.ErrNoRows:
			exists = false
		case err != nil:
			return fmt.Errorf("read current question %d: %w", i, err)
		}

		// edited = 1 solo per un testo NON VUOTO diverso da quanto
		// salvato (o assente). Un testo vuoto non è mai "edited": marcare
		// così uno slot vuoto lo congelerebbe per sempre, perché
		// SaveGeneratedQuestions salta le righe già edited — nessuna
		// rigenerazione lo riempirebbe più.
		changed := !exists || text != current
		edited := changed && text != ""

		if exists && !changed {
			// Rimandata invariata: conserva il flag che aveva, non
			// promuoverla.
			if _, err := tx.ExecContext(ctx,
				`UPDATE game_suggested_question SET text = ?
				 WHERE game_id = ? AND position = ?`, text, gameID, i); err != nil {
				return fmt.Errorf("update question %d: %w", i, err)
			}
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO game_suggested_question (game_id, position, text, edited)
			 VALUES (?, ?, ?, ?)
			 ON CONFLICT(game_id, position) DO UPDATE SET text = excluded.text, edited = excluded.edited`,
			gameID, i, text, edited); err != nil {
			return fmt.Errorf("upsert question %d: %w", i, err)
		}
	}
	return tx.Commit()
}

// saveQuestions è il corpo comune di SaveGeneratedQuestions e
// SaveAllQuestions: stessa transazione, stesso ciclo, solo la query
// dell'upsert cambia.
func (s *Store) saveQuestions(ctx context.Context, gameID int64, texts []string, stmt string) error {
	if len(texts) != SuggestedQuestionCount {
		return fmt.Errorf("suggested questions: attesi %d testi, ricevuti %d",
			SuggestedQuestionCount, len(texts))
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	for i, text := range texts {
		if _, err := tx.ExecContext(ctx, stmt, gameID, i, text); err != nil {
			return fmt.Errorf("save question %d: %w", i, err)
		}
	}
	return tx.Commit()
}
