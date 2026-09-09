package manuals_test

import (
	"context"
	"database/sql"
	"testing"

	"boardgames-manager/internal/db"
	"boardgames-manager/internal/manuals"
)

// seed prepara un gioco con una lingua e un manuale, e restituisce
// (gameID, mediaID). Scrive in SQL diretto: questo pacchetto non deve
// dipendere da internal/games solo per allestire i test.
func seed(t *testing.T, conn *sql.DB, gameName, lang, mediaTitle string) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	g, err := conn.ExecContext(ctx, `INSERT INTO games (name, seats) VALUES (?, 1)`, gameName)
	if err != nil {
		t.Fatalf("insert game: %v", err)
	}
	gameID, _ := g.LastInsertId()
	l, err := conn.ExecContext(ctx,
		`INSERT INTO game_languages (game_id, language_code, is_base_language, name)
		 VALUES (?, ?, 1, ?)`, gameID, lang, gameName)
	if err != nil {
		t.Fatalf("insert language: %v", err)
	}
	langID, _ := l.LastInsertId()
	m, err := conn.ExecContext(ctx,
		`INSERT INTO game_media (game_language_id, type, url_or_path, title)
		 VALUES (?, 'file', 'manuale.pdf', ?)`, langID, mediaTitle)
	if err != nil {
		t.Fatalf("insert media: %v", err)
	}
	mediaID, _ := m.LastInsertId()
	return gameID, mediaID
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Ogni connessione a ":memory:" è un database a sé: il pool va fissato
	// a una, altrimenti una seconda connessione vede uno schema vuoto.
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return conn
}

func TestReplaceSource_CascadesWhenTheMediaGoes(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()

	if err := store.ReplaceSource(ctx, gameID, &mediaID, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento base",
			ReferenceDetail: "pagina 1", Heading: "Preparazione",
			LanguageCode: "it", Seq: 0, Text: "Si distribuiscono cinque carte."},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	var indexed int
	conn.QueryRow(`SELECT COUNT(*) FROM game_source_chunk_fts`).Scan(&indexed)
	if indexed != 1 {
		t.Fatalf("l'indice FTS5 non è in pari coi chunk: %d", indexed)
	}

	if _, err := conn.ExecContext(ctx, `DELETE FROM game_media WHERE id = ?`, mediaID); err != nil {
		t.Fatalf("delete media: %v", err)
	}

	var chunks, stillIndexed int
	conn.QueryRow(`SELECT COUNT(*) FROM game_source_chunk`).Scan(&chunks)
	conn.QueryRow(`SELECT COUNT(*) FROM game_source_chunk_fts`).Scan(&stillIndexed)
	// COUNT(*) da solo non basta: su una tabella FTS5 in external content,
	// uno scan senza MATCH deve rileggere il testo dalla tabella di contenuto
	// esterna via content_rowid, quindi salta da solo le righe il cui
	// contenuto è sparito — anche se il trigger che tiene in pari l'indice
	// non fosse mai scattato. Solo una query MATCH reale, che legge dalle
	// strutture interne dell'indice, smaschera una voce fantasma lasciata da
	// un trigger rotto.
	var stillMatchable int
	conn.QueryRow(`SELECT COUNT(*) FROM game_source_chunk_fts WHERE game_source_chunk_fts MATCH 'cinque'`).Scan(&stillMatchable)
	if chunks != 0 || stillIndexed != 0 || stillMatchable != 0 {
		t.Fatalf("la cascata ha lasciato %d chunk, %d righe indicizzate, %d ancora trovabili via MATCH", chunks, stillIndexed, stillMatchable)
	}
}

func TestReplaceSource_IsIdempotentAndRebuildsWithNewContent(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()

	first := []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento base", ReferenceDetail: "pagina 3",
			Heading: "Turno del giocatore", LanguageCode: "it", Seq: 0,
			Text: "All'inizio del turno il giocatore pesca due carte."},
		{ReferenceType: "document", Reference: "Regolamento base", ReferenceDetail: "pagina 4",
			Heading: "Fase di Upkeep", LanguageCode: "it", Seq: 1,
			Text: "Ogni giocatore paga una moneta per edificio."},
	}
	if err := store.ReplaceSource(ctx, gameID, &mediaID, first); err != nil {
		t.Fatalf("first replace: %v", err)
	}
	var chunks int
	conn.QueryRow(`SELECT COUNT(*) FROM game_source_chunk WHERE game_id = ?`, gameID).Scan(&chunks)
	if chunks != 2 {
		t.Fatalf("attesi 2 chunk dopo il primo salvataggio, ottenuti %d", chunks)
	}

	// Un secondo salvataggio con un chunk solo, di contenuto DIVERSO, deve
	// sostituire e non accumulare: se ReplaceSource non cancellasse prima di
	// inserire, il conteggio salirebbe a 3 e il vecchio testo sopravviverebbe.
	second := []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento base", ReferenceDetail: "pagina 9",
			Heading: "Fine partita", LanguageCode: "it", Seq: 0,
			Text: "La partita termina quando il mazzo si esaurisce."},
	}
	if err := store.ReplaceSource(ctx, gameID, &mediaID, second); err != nil {
		t.Fatalf("second replace: %v", err)
	}

	conn.QueryRow(`SELECT COUNT(*) FROM game_source_chunk WHERE game_id = ?`, gameID).Scan(&chunks)
	if chunks != 1 {
		t.Fatalf("attesi 1 chunk dopo il secondo salvataggio, ottenuti %d: la sostituzione ha accumulato invece di rimpiazzare", chunks)
	}
	var text, detail string
	if err := conn.QueryRow(`SELECT text, reference_detail FROM game_source_chunk WHERE game_id = ?`, gameID).
		Scan(&text, &detail); err != nil {
		t.Fatalf("select survivor: %v", err)
	}
	if text != second[0].Text || detail != "pagina 9" {
		t.Fatalf("il chunk sopravvissuto non è quello del secondo salvataggio: text=%q detail=%q", text, detail)
	}

	var indexed int
	conn.QueryRow(`SELECT COUNT(*) FROM game_source_chunk_fts`).Scan(&indexed)
	if indexed != 1 {
		t.Fatalf("l'indice FTS5 ha %d righe dopo la ricostruzione, atteso 1", indexed)
	}
}

func TestReplaceSource_AcceptsAFAQWithoutMedia(t *testing.T) {
	// mediaID è *int64 proprio per questo caso: una FAQ non ha un game_media.
	// Con mediaID nil non c'è nessuna DELETE preliminare (non esiste una
	// game_media_id su cui filtrarla): la garanzia di sostituzione riguarda
	// solo le fonti caricate come media, quella per le FAQ arriva con la
	// fase che le costruisce davvero.
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, _ := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()

	err := store.ReplaceSource(ctx, gameID, nil, []manuals.SourceChunk{
		{ReferenceType: "faq", Reference: "https://boardgamegeek.com/thread/1",
			ReferenceDetail: "commento del 15/07/2023 08:00", Seq: 0,
			Text: "Il giocatore può ritirare i dadi in questi casi."},
	})
	if err != nil {
		t.Fatalf("replace: %v", err)
	}

	var mediaID sql.NullInt64
	var refType, reference string
	if err := conn.QueryRow(
		`SELECT game_media_id, reference_type, reference FROM game_source_chunk WHERE game_id = ?`, gameID).
		Scan(&mediaID, &refType, &reference); err != nil {
		t.Fatalf("select: %v", err)
	}
	if mediaID.Valid {
		t.Fatalf("una FAQ non deve avere un game_media_id, ottenuto %d", mediaID.Int64)
	}
	if refType != "faq" || reference != "https://boardgamegeek.com/thread/1" {
		t.Fatalf("reference_type/reference persi: %q %q", refType, reference)
	}
}

func TestDeleteSource_RemovesOnlyThatMediaAndCascadesToFTS(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID, mediaA := seed(t, conn, "Wingspan", "it", "Regolamento A")
	_, mediaB := seed(t, conn, "Brass", "it", "Regolamento B")

	if err := store.ReplaceSource(ctx, gameID, &mediaA, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento A", Seq: 0, Text: "Testo A."},
	}); err != nil {
		t.Fatalf("replace A: %v", err)
	}
	if err := store.ReplaceSource(ctx, gameID, &mediaB, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento B", Seq: 0, Text: "Testo B."},
	}); err != nil {
		t.Fatalf("replace B: %v", err)
	}

	if err := store.DeleteSource(ctx, mediaA); err != nil {
		t.Fatalf("delete: %v", err)
	}

	var chunksA, chunksB, indexed int
	conn.QueryRow(`SELECT COUNT(*) FROM game_source_chunk WHERE game_media_id = ?`, mediaA).Scan(&chunksA)
	conn.QueryRow(`SELECT COUNT(*) FROM game_source_chunk WHERE game_media_id = ?`, mediaB).Scan(&chunksB)
	conn.QueryRow(`SELECT COUNT(*) FROM game_source_chunk_fts`).Scan(&indexed)
	if chunksA != 0 {
		t.Fatalf("DeleteSource ha lasciato %d chunk del media cancellato", chunksA)
	}
	if chunksB != 1 {
		t.Fatalf("DeleteSource ha toccato il media sbagliato: chunk di B = %d", chunksB)
	}
	if indexed != 1 {
		t.Fatalf("l'indice FTS5 ha %d righe, atteso 1 (solo B)", indexed)
	}
}

func TestHasChunks_ReflectsWhetherAnySourceExists(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")

	has, err := store.HasChunks(ctx, gameID)
	if err != nil {
		t.Fatalf("has chunks: %v", err)
	}
	if has {
		t.Fatal("un gioco senza fonti preparate non ha chunk")
	}

	if err := store.ReplaceSource(ctx, gameID, &mediaID, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento base", Seq: 0, Text: "Testo."},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	has, err = store.HasChunks(ctx, gameID)
	if err != nil {
		t.Fatalf("has chunks: %v", err)
	}
	if !has {
		t.Fatal("dopo ReplaceSource il gioco ha chunk")
	}
}

func TestSummary_HeadingsAreDistinctAndInSeqOrderNotAlphabetical(t *testing.T) {
	// Sceglie due titoli il cui ordine alfabetico è l'INVERSO del loro ordine
	// nel documento: se Summary li ordinasse alfabeticamente (o li leggesse
	// da una mappa, il cui ordine di iterazione in Go è casuale) invece che
	// per seq, questo test lo scoprirebbe. Un test che si limitasse a
	// verificare che entrambi i titoli compaiono passerebbe comunque con
	// l'ordine sbagliato — motivo per cui qui si confronta la sequenza.
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()

	if err := store.ReplaceSource(ctx, gameID, &mediaID, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento base", Heading: "Zona di gioco",
			Seq: 0, Text: "Prima sezione."},
		{ReferenceType: "document", Reference: "Regolamento base", Heading: "Alimentazione",
			Seq: 1, Text: "Seconda sezione."},
		// Stesso heading della prima, in coda: deve essere scartato come
		// doppione, non spostare "Zona di gioco" in fondo.
		{ReferenceType: "document", Reference: "Regolamento base", Heading: "Zona di gioco",
			Seq: 2, Text: "Terza sezione, stessa sezione della prima."},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	sum, err := store.Summary(ctx, gameID)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	want := []string{"Zona di gioco", "Alimentazione"}
	if len(sum.Headings) != len(want) {
		t.Fatalf("attesi %d titoli distinti, ottenuti %v", len(want), sum.Headings)
	}
	for i, h := range want {
		if sum.Headings[i] != h {
			t.Fatalf("titolo in posizione %d = %q, atteso %q (Headings = %v)", i, sum.Headings[i], h, sum.Headings)
		}
	}
}

func TestSummary_GroupsHeadingsPerSourceForThePromptIndex(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID, mediaA := seed(t, conn, "Wingspan", "it", "Regolamento base")
	_, mediaB := seed(t, conn, "Wingspan2", "it", "Errata")

	if err := store.ReplaceSource(ctx, gameID, &mediaA, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento base", Heading: "Preparazione", Seq: 0, Text: "T1"},
		{ReferenceType: "document", Reference: "Regolamento base", Heading: "Turno del giocatore", Seq: 1, Text: "T2"},
	}); err != nil {
		t.Fatalf("replace A: %v", err)
	}
	if err := store.ReplaceSource(ctx, gameID, &mediaB, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Errata", Heading: "Correzione punteggio", Seq: 0, Text: "T3"},
	}); err != nil {
		t.Fatalf("replace B: %v", err)
	}

	sum, err := store.Summary(ctx, gameID)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if len(sum.Sources) != 2 {
		t.Fatalf("attese 2 fonti raggruppate, ottenute %d: %+v", len(sum.Sources), sum.Sources)
	}
	byRef := map[string][]string{}
	for _, s := range sum.Sources {
		byRef[s.Reference] = s.Headings
	}
	wantBase := []string{"Preparazione", "Turno del giocatore"}
	if len(byRef["Regolamento base"]) != len(wantBase) ||
		byRef["Regolamento base"][0] != wantBase[0] || byRef["Regolamento base"][1] != wantBase[1] {
		t.Fatalf("titoli di 'Regolamento base' = %v, attesi %v in ordine", byRef["Regolamento base"], wantBase)
	}
	if len(byRef["Errata"]) != 1 || byRef["Errata"][0] != "Correzione punteggio" {
		t.Fatalf("titoli di 'Errata' = %v", byRef["Errata"])
	}
}

func TestSummary_KeepsDistinctMediaSeparateWhenReferencesCollide(t *testing.T) {
	// Regressione: raggruppare Sources per `reference` invece che per
	// game_media_id fa collassare in una voce sola due media distinti che
	// condividono la stessa reference — cosa che può succedere finché il
	// Task 5 non le disambigua in scrittura. Una struttura di lettura non
	// deve dipendere da quell'invariante mantenuto due task più in là.
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID, mediaA := seed(t, conn, "Wingspan", "it", "Regolamento")
	_, mediaB := seed(t, conn, "Wingspan2", "it", "Regolamento")

	if err := store.ReplaceSource(ctx, gameID, &mediaA, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento", Heading: "Preparazione", Seq: 0, Text: "T1"},
	}); err != nil {
		t.Fatalf("replace A: %v", err)
	}
	if err := store.ReplaceSource(ctx, gameID, &mediaB, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento", Heading: "Fase finale", Seq: 0, Text: "T2"},
	}); err != nil {
		t.Fatalf("replace B: %v", err)
	}

	sum, err := store.Summary(ctx, gameID)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if len(sum.Sources) != 2 {
		t.Fatalf("due media con la stessa reference sono collassati in %d fonte/i invece di 2: %+v", len(sum.Sources), sum.Sources)
	}
	var headings []string
	for _, s := range sum.Sources {
		headings = append(headings, s.Headings...)
	}
	hasPreparazione, hasFaseFinale := false, false
	for _, h := range headings {
		hasPreparazione = hasPreparazione || h == "Preparazione"
		hasFaseFinale = hasFaseFinale || h == "Fase finale"
	}
	if !hasPreparazione || !hasFaseFinale {
		t.Fatalf("titoli persi nel collasso: %v", headings)
	}
}

func TestSummary_CountsChunksPerMediaForTheAdminPanel(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID, mediaA := seed(t, conn, "Wingspan", "it", "Regolamento base")
	_, mediaB := seed(t, conn, "Wingspan2", "it", "Errata")

	if err := store.ReplaceSource(ctx, gameID, &mediaA, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento base", Seq: 0, Text: "T1"},
		{ReferenceType: "document", Reference: "Regolamento base", Seq: 1, Text: "T2"},
		{ReferenceType: "document", Reference: "Regolamento base", Seq: 2, Text: "T3"},
	}); err != nil {
		t.Fatalf("replace A: %v", err)
	}
	if err := store.ReplaceSource(ctx, gameID, &mediaB, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Errata", Seq: 0, Text: "T4"},
	}); err != nil {
		t.Fatalf("replace B: %v", err)
	}

	sum, err := store.Summary(ctx, gameID)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if sum.PerMedia[mediaA] != 3 {
		t.Fatalf("PerMedia[mediaA] = %d, atteso 3", sum.PerMedia[mediaA])
	}
	if sum.PerMedia[mediaB] != 1 {
		t.Fatalf("PerMedia[mediaB] = %d, atteso 1", sum.PerMedia[mediaB])
	}
}

func TestGamesWithChunks_ReportsOnlyGamesThatHaveThem(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameWithSource, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	gameWithout, _ := seed(t, conn, "Brass", "it", "Regolamento senza fonti pronte")

	if err := store.ReplaceSource(ctx, gameWithSource, &mediaID, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento base", Seq: 0, Text: "Testo."},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	out, err := store.GamesWithChunks(ctx, []int64{gameWithSource, gameWithout})
	if err != nil {
		t.Fatalf("games with chunks: %v", err)
	}
	if !out[gameWithSource] {
		t.Fatalf("il gioco con fonti pronte non risulta nella mappa: %v", out)
	}
	if out[gameWithout] {
		t.Fatalf("il gioco senza fonti pronte risulta comunque nella mappa: %v", out)
	}
}
