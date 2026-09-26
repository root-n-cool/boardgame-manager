package manuals_test

import (
	"context"
	"database/sql"
	"testing"

	"boardgames-manager/internal/manuals"
)

// seedGame crea un gioco minimo: le domande hanno una foreign key su
// games, quindi senza una riga vera l'insert fallisce.
func seedGameForQuestions(t *testing.T, conn *sql.DB) int64 {
	t.Helper()
	res, err := conn.Exec(`INSERT INTO games (name) VALUES ('Gioco di Prova')`)
	if err != nil {
		t.Fatalf("seed game: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	return id
}

func TestSuggestedQuestions_EmptyWhenNothingSaved(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID := seedGameForQuestions(t, conn)

	got, err := store.SuggestedQuestions(context.Background(), gameID, manuals.AgentRules)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("un gioco senza domande deve restituire una lista vuota, ottenuto %v", got)
	}
}

func TestSaveGeneratedQuestions_WritesThreeInOrder(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID := seedGameForQuestions(t, conn)

	texts := []string{"Come si prepara?", "Cosa faccio nel turno?", "Come finisce?"}
	if err := store.SaveGeneratedQuestions(context.Background(), gameID, manuals.AgentRules, texts); err != nil {
		t.Fatalf("save generated: %v", err)
	}

	got, err := store.SuggestedQuestions(context.Background(), gameID, manuals.AgentRules)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("attese 3 domande, ottenute %d: %v", len(got), got)
	}
	for i, q := range got {
		if q.Position != i {
			t.Fatalf("posizione %d attesa in indice %d, ottenuta %d", i, i, q.Position)
		}
		if q.Text != texts[i] {
			t.Fatalf("posizione %d: atteso %q, ottenuto %q", i, texts[i], q.Text)
		}
		if q.Edited {
			t.Fatalf("posizione %d: una domanda generata non è edited", i)
		}
	}
}

// TestSaveGeneratedQuestions_PreservesEdited è IL test di questo task: è il
// comportamento scelto in fase di design. Una reindicizzazione riscrive le
// domande generate dal modello e non tocca mai quelle scritte a mano.
func TestSaveGeneratedQuestions_PreservesEdited(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID := seedGameForQuestions(t, conn)

	if err := store.SaveGeneratedQuestions(ctx, gameID, manuals.AgentRules,
		[]string{"Generata A?", "Generata B?", "Generata C?"}); err != nil {
		t.Fatalf("save generated: %v", err)
	}
	// L'admin riscrive la seconda: solo quella diventa edited.
	if err := store.SaveEditedQuestions(ctx, gameID, manuals.AgentRules,
		[]string{"Generata A?", "Scritta a mano?", "Generata C?"}); err != nil {
		t.Fatalf("save edited: %v", err)
	}
	// Una reindicizzazione rigenera tutte e tre.
	if err := store.SaveGeneratedQuestions(ctx, gameID, manuals.AgentRules,
		[]string{"Rigenerata A?", "Rigenerata B?", "Rigenerata C?"}); err != nil {
		t.Fatalf("save generated dopo edit: %v", err)
	}

	got, err := store.SuggestedQuestions(ctx, gameID, manuals.AgentRules)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	want := []string{"Rigenerata A?", "Scritta a mano?", "Rigenerata C?"}
	for i, q := range got {
		if q.Text != want[i] {
			t.Fatalf("posizione %d: atteso %q, ottenuto %q (tutte: %v)", i, want[i], q.Text, got)
		}
	}
	if !got[1].Edited {
		t.Fatal("la domanda scritta a mano deve restare marcata edited")
	}
	if got[0].Edited || got[2].Edited {
		t.Fatal("le domande rigenerate non devono essere marcate edited")
	}
}

// TestSaveEditedQuestions_MarksOnlyChangedTexts: il flag lo decide il
// server confrontando col testo salvato, non un campo che arriva dal
// client. Senza questo, un salvataggio che non cambia nulla congelerebbe
// tutte tre le domande per sempre.
func TestSaveEditedQuestions_MarksOnlyChangedTexts(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID := seedGameForQuestions(t, conn)

	if err := store.SaveGeneratedQuestions(ctx, gameID, manuals.AgentRules,
		[]string{"Generata A?", "Generata B?", "Generata C?"}); err != nil {
		t.Fatalf("save generated: %v", err)
	}
	// Rimanda le tre domande cambiandone solo una.
	if err := store.SaveEditedQuestions(ctx, gameID, manuals.AgentRules,
		[]string{"Generata A?", "Cambiata?", "Generata C?"}); err != nil {
		t.Fatalf("save edited: %v", err)
	}

	got, err := store.SuggestedQuestions(ctx, gameID, manuals.AgentRules)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	if got[0].Edited || got[2].Edited {
		t.Fatalf("un testo rimandato invariato non diventa edited: %v", got)
	}
	if !got[1].Edited {
		t.Fatal("il solo testo cambiato deve diventare edited")
	}
}

// TestSaveEditedQuestions_UpsertsOnAGameWithNoRows: un gioco mai
// indicizzato non ha righe, e l'admin deve poter scrivere le sue tre
// domande a mano prima (o invece) di qualunque generazione.
func TestSaveEditedQuestions_UpsertsOnAGameWithNoRows(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID := seedGameForQuestions(t, conn)

	if err := store.SaveEditedQuestions(ctx, gameID, manuals.AgentRules,
		[]string{"Prima?", "Seconda?", "Terza?"}); err != nil {
		t.Fatalf("save edited su gioco senza righe: %v", err)
	}

	got, err := store.SuggestedQuestions(ctx, gameID, manuals.AgentRules)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("attese 3 domande, ottenute %d", len(got))
	}
	for i, q := range got {
		if !q.Edited {
			t.Fatalf("posizione %d: una domanda creata da zero a mano nasce edited", i)
		}
	}
}

// TestSaveEditedQuestions_EmptyTextOnAGameWithNoRowsStillCreatesThreeRows è
// il bug di round 1: su un gioco senza righe, un testo vuoto in arrivo
// veniva confuso col "nessuna riga esiste" (current defaultava a ""), e
// l'UPDATE che ne seguiva non toccava nessuna riga. Restavano solo due
// domande invece di tre. La riga per la posizione vuota deve comunque
// esistere, e non deve mai diventare edited: altrimenti una rigenerazione
// non la riempirebbe mai più.
func TestSaveEditedQuestions_EmptyTextOnAGameWithNoRowsStillCreatesThreeRows(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID := seedGameForQuestions(t, conn)

	if err := store.SaveEditedQuestions(ctx, gameID, manuals.AgentRules,
		[]string{"", "Seconda?", "Terza?"}); err != nil {
		t.Fatalf("save edited con testo vuoto: %v", err)
	}

	got, err := store.SuggestedQuestions(ctx, gameID, manuals.AgentRules)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("attese 3 domande anche con un testo vuoto, ottenute %d: %v", len(got), got)
	}
	if got[0].Text != "" {
		t.Fatalf("posizione 0: atteso testo vuoto, ottenuto %q", got[0].Text)
	}
	if got[0].Edited {
		t.Fatal("posizione 0: un testo vuoto non deve mai diventare edited")
	}
	if !got[1].Edited || !got[2].Edited {
		t.Fatalf("posizioni 1 e 2: testi non vuoti scritti a mano devono essere edited, ottenuto %v", got)
	}
}

// TestSaveAllQuestions_OverwritesEditedToo: è il pulsante "rigenera", che
// l'admin premette deliberatamente — quindi sovrascrive tutto e azzera i
// flag.
func TestSaveAllQuestions_OverwritesEditedToo(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID := seedGameForQuestions(t, conn)

	if err := store.SaveEditedQuestions(ctx, gameID, manuals.AgentRules,
		[]string{"A mano 1?", "A mano 2?", "A mano 3?"}); err != nil {
		t.Fatalf("save edited: %v", err)
	}
	if err := store.SaveAllQuestions(ctx, gameID, manuals.AgentRules,
		[]string{"Nuova 1?", "Nuova 2?", "Nuova 3?"}); err != nil {
		t.Fatalf("save all: %v", err)
	}

	got, err := store.SuggestedQuestions(ctx, gameID, manuals.AgentRules)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	for i, q := range got {
		if q.Text != []string{"Nuova 1?", "Nuova 2?", "Nuova 3?"}[i] {
			t.Fatalf("posizione %d non sovrascritta: %q", i, q.Text)
		}
		if q.Edited {
			t.Fatalf("posizione %d: rigenera azzera edited, ottenuto true", i)
		}
	}
}

func TestSuggestedQuestions_CascadeOnGameDelete(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID := seedGameForQuestions(t, conn)

	if err := store.SaveGeneratedQuestions(ctx, gameID, manuals.AgentRules,
		[]string{"A?", "B?", "C?"}); err != nil {
		t.Fatalf("save generated: %v", err)
	}
	if _, err := conn.Exec(`DELETE FROM games WHERE id = ?`, gameID); err != nil {
		t.Fatalf("delete game: %v", err)
	}

	var count int
	if err := conn.QueryRow(
		`SELECT COUNT(*) FROM game_suggested_question WHERE game_id = ?`, gameID,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("la cascata deve portare via le domande col gioco, restano %d righe", count)
	}
}

func TestSuggestedQuestions_AreSeparatePerAgent(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID := seedGameForQuestions(t, conn)
	ctx := context.Background()

	if err := store.SaveGeneratedQuestions(ctx, gameID, manuals.AgentRules, []string{"R1?", "R2?", "R3?"}); err != nil {
		t.Fatalf("save rules: %v", err)
	}
	if err := store.SaveGeneratedQuestions(ctx, gameID, manuals.AgentStrategy, []string{"S1?", "S2?", "S3?"}); err != nil {
		t.Fatalf("save strategy: %v", err)
	}
	if err := store.SaveEditedQuestions(ctx, gameID, manuals.AgentStrategy, []string{"S1?", "Mia?", "S3?"}); err != nil {
		t.Fatalf("edit strategy: %v", err)
	}

	rules, _ := store.SuggestedQuestions(ctx, gameID, manuals.AgentRules)
	strategy, _ := store.SuggestedQuestions(ctx, gameID, manuals.AgentStrategy)
	if len(rules) != 3 || rules[1].Text != "R2?" || rules[1].Edited {
		t.Fatalf("editing strategy must not touch rules, got %+v", rules)
	}
	if len(strategy) != 3 || strategy[1].Text != "Mia?" || !strategy[1].Edited {
		t.Fatalf("unexpected strategy questions %+v", strategy)
	}
}

func TestValidAgent(t *testing.T) {
	for _, a := range []string{"rules", "strategy"} {
		if !manuals.ValidAgent(a) {
			t.Fatalf("%q must be valid", a)
		}
	}
	for _, a := range []string{"", "Rules", "x"} {
		if manuals.ValidAgent(a) {
			t.Fatalf("%q must be invalid", a)
		}
	}
}
