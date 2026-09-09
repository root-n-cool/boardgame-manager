package manuals_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
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

// regoleChunks è una fonte a 4 chunk (seq 0..3): la parola cercata nei test
// sotto cade sempre su un chunk INTERNO (seq 1 o 2), mai sul primo o
// sull'ultimo, apposta per non far scattare attachNeighbours e sporcare i
// test che non lo riguardano.
var regoleChunks = []manuals.SourceChunk{
	{ReferenceType: "document", Reference: "Regolamento base", ReferenceDetail: "pagina 3",
		Heading: "Turno del giocatore", LanguageCode: "it", Seq: 0,
		Text: "Turno del giocatore. All'inizio del proprio turno il giocatore pesca due carte dal mazzo comune e ne scarta una."},
	{ReferenceType: "document", Reference: "Regolamento base", ReferenceDetail: "pagina 4",
		Heading: "Fase di Upkeep", LanguageCode: "it", Seq: 1,
		Text: "Fase di Upkeep. Al termine di ogni round ogni giocatore paga una moneta per ciascun edificio posseduto."},
	{ReferenceType: "document", Reference: "Regolamento base", ReferenceDetail: "pagina 5",
		Heading: "Rimescolare", LanguageCode: "it", Seq: 2,
		Text: "Rimescolare. Quando la pila di pesca si esaurisce in una partita a due giocatori si rimescola la pila degli scarti."},
	{ReferenceType: "document", Reference: "Regolamento base", ReferenceDetail: "pagina 8",
		Heading: "Conteggio e pareggi", LanguageCode: "it", Seq: 3,
		Text: "Conteggio e pareggi. Se due giocatori totalizzano lo stesso punteggio vince chi ha piu' monete in riserva."},
}

func TestSearch_FindsByKeywordAndReportsTheMisses(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()
	if err := store.ReplaceSource(ctx, gameID, &mediaID, regoleChunks); err != nil {
		t.Fatalf("replace: %v", err)
	}

	hits, missing, err := store.Search(ctx, gameID, "it", []string{"Upkeep", "rimescola", "ripescare"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("nessun risultato per 'Upkeep', che è nella fonte")
	}
	var foundUpkeep bool
	for _, h := range hits {
		if h.ReferenceType != "document" {
			t.Fatalf("reference_type perso: %q", h.ReferenceType)
		}
		if h.Reference != "Regolamento base" {
			t.Fatalf("reference persa: %q", h.Reference)
		}
		if h.ReferenceDetail == "" {
			t.Fatalf("reference_detail persa per l'hit %+v", h)
		}
		if len(h.FoundWith) == 0 {
			t.Fatalf("hit %+v senza FoundWith", h)
		}
		if strings.Contains(h.Text, "Al termine di ogni round") {
			foundUpkeep = true
		}
	}
	if !foundUpkeep {
		t.Fatalf("'Upkeep' è a pagina 4, non trovato fra gli hit: %+v", hits)
	}
	// 'ripescare' non c'è nella fonte: deve comparire fra i mancanti, che è
	// l'informazione con cui il modello decide se riprovare con altre
	// parole.
	if !containsString(missing, "ripescare") {
		t.Fatalf("'ripescare' doveva essere fra i mancanti, missing = %v", missing)
	}
	if containsString(missing, "Upkeep") {
		t.Fatalf("'Upkeep' è stato trovato ma è fra i mancanti: %v", missing)
	}
}

func TestSearch_DeduplicatesAndAccumulatesFoundWith(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()
	if err := store.ReplaceSource(ctx, gameID, &mediaID, regoleChunks); err != nil {
		t.Fatalf("replace: %v", err)
	}

	// Due parole chiave che colpiscono lo stesso chunk (pagina 5): deve
	// uscire una volta sola, con entrambe in FoundWith.
	hits, _, err := store.Search(ctx, gameID, "it", []string{"pila", "rimescola"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	seen := map[string]int{}
	for _, h := range hits {
		seen[h.ReferenceDetail]++
	}
	for detail, n := range seen {
		if n > 1 {
			t.Fatalf("%q compare %d volte: manca la deduplicazione", detail, n)
		}
	}
	var foundBoth bool
	for _, h := range hits {
		if h.ReferenceDetail == "pagina 5" && len(h.FoundWith) >= 2 {
			foundBoth = true
		}
	}
	if !foundBoth {
		t.Fatalf("la pagina 5 doveva essere trovata da due parole, hits = %+v", hits)
	}
}

func TestSearch_SurvivesAnApostrophe(t *testing.T) {
	// Regressione nota: una parola chiave con l'apostrofo passata grezza a
	// MATCH dà "fts5: syntax error"; il fallback su escapeFTS deve
	// scattare e trovare ancora qualcosa.
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()
	if err := store.ReplaceSource(ctx, gameID, &mediaID, regoleChunks); err != nil {
		t.Fatalf("replace: %v", err)
	}

	hits, missing, err := store.Search(ctx, gameID, "it", []string{"all'inizio"})
	if err != nil {
		t.Fatalf(`Search("all'inizio") ha restituito errore: %v`, err)
	}
	if len(hits) == 0 {
		t.Fatal(`"all'inizio" doveva trovare la pagina 3, nessun hit`)
	}
	if containsString(missing, "all'inizio") {
		t.Fatalf(`"all'inizio" è fra i mancanti nonostante il match: %v`, missing)
	}

	for _, kw := range []string{`virgoletta"dentro`, "chi vince in caso di parita'"} {
		if _, _, err := store.Search(ctx, gameID, "it", []string{kw}); err != nil {
			t.Fatalf("Search(%q) ha restituito errore: %v", kw, err)
		}
	}
}

func TestSearch_DoesNotLeakIntoAnotherGame(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameA, mediaA := seed(t, conn, "Wingspan", "it", "Regolamento A")
	gameB, mediaB := seed(t, conn, "Brass", "it", "Regolamento B")
	if err := store.ReplaceSource(ctx, gameA, &mediaA, regoleChunks); err != nil {
		t.Fatalf("replace A: %v", err)
	}
	if err := store.ReplaceSource(ctx, gameB, &mediaB, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento B", Seq: 0,
			Text: "Fase di Upkeep di un gioco completamente diverso."},
	}); err != nil {
		t.Fatalf("replace B: %v", err)
	}

	hits, _, err := store.Search(ctx, gameA, "it", []string{"Upkeep"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("la ricerca su gameA non ha trovato niente: senza risultati questo test non verifica nessuno sconfinamento")
	}
	for _, h := range hits {
		// regoleChunks (la fonte di gameA) ha reference "Regolamento base":
		// qualunque altro valore vorrebbe dire che la ricerca ha letto
		// chunk del gioco B.
		if h.Reference != "Regolamento base" {
			t.Fatalf("la ricerca su gameA ha restituito %q: sconfina su un altro gioco", h.Reference)
		}
	}
}

func TestSearch_CapsTheNumberOfKeywords(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	if err := store.ReplaceSource(ctx, gameID, &mediaID, regoleChunks); err != nil {
		t.Fatalf("replace: %v", err)
	}

	// Dodici parole che non trovano niente, e alla tredicesima una che
	// troverebbe: oltre il tetto non deve essere nemmeno cercata.
	keywords := make([]string, 0, 40)
	for i := 0; i < 12; i++ {
		keywords = append(keywords, fmt.Sprintf("parolachenonesiste%d", i))
	}
	keywords = append(keywords, "Upkeep")
	for i := 0; i < 27; i++ {
		keywords = append(keywords, fmt.Sprintf("altraparolainutile%d", i))
	}

	hits, missing, err := store.Search(ctx, gameID, "it", keywords)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(missing) > 12 {
		t.Fatalf("cercate %d parole chiave: nessun tetto", len(missing))
	}
	if len(hits) != 0 {
		t.Fatalf("la tredicesima parola chiave è stata cercata lo stesso: %d risultati", len(hits))
	}
}

func TestSearch_PrefersTheRequestedLanguage(t *testing.T) {
	// L'ordine di inserimento è deliberato: il chunk INGLESE entra per
	// primo (rowid più basso), quello ITALIANO per secondo. Senza il
	// riordino esplicito per lingua preferita, l'ordine "naturale" di un
	// pareggio di rank BM25 su questo motore segue il rowid crescente, e
	// metterebbe l'inglese davanti — quindi un'asserzione su hits[0] può
	// davvero andare in rosso se il riordino sparisce (verificato a mano:
	// vedi il report).
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID, mediaIT := seed(t, conn, "Wingspan", "it", "Regolamento italiano")

	l, _ := conn.ExecContext(ctx,
		`INSERT INTO game_languages (game_id, language_code, is_base_language, name)
		 VALUES (?, 'en', 0, 'Wingspan')`, gameID)
	langEN, _ := l.LastInsertId()
	m, _ := conn.ExecContext(ctx,
		`INSERT INTO game_media (game_language_id, type, url_or_path, title)
		 VALUES (?, 'file', 'rules-en.pdf', 'English rulebook')`, langEN)
	mediaEN, _ := m.LastInsertId()

	if err := store.ReplaceSource(ctx, gameID, &mediaEN, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "English rulebook", LanguageCode: "en", Seq: 0,
			Text: "Upkeep phase: pay one coin per building."},
	}); err != nil {
		t.Fatalf("replace en: %v", err)
	}
	if err := store.ReplaceSource(ctx, gameID, &mediaIT, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento italiano", LanguageCode: "it", Seq: 0,
			Text: "Fase di Upkeep: si paga una moneta per edificio."},
	}); err != nil {
		t.Fatalf("replace it: %v", err)
	}

	hits, _, err := store.Search(ctx, gameID, "it", []string{"Upkeep"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) < 2 {
		t.Fatalf("attesi risultati da entrambe le lingue, ottenuti %d: %+v", len(hits), hits)
	}
	if hits[0].Reference != "Regolamento italiano" {
		t.Fatalf("con preferLang=it il primo risultato deve essere quello italiano, è %q", hits[0].Reference)
	}
}

func TestSearch_AttachesNeighbourAtTheStartOfASource(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()
	if err := store.ReplaceSource(ctx, gameID, &mediaID, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento base", Seq: 0,
			Text: "Fase di Upkeep. Ogni giocatore paga una moneta."},
		{ReferenceType: "document", Reference: "Regolamento base", Seq: 1,
			Text: "Testo intermedio che collega le due sezioni della stessa fonte."},
		{ReferenceType: "document", Reference: "Regolamento base", Seq: 2,
			Text: "Fase finale Zibaldone: il gioco termina quando la plancia e' piena."},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	hits, _, err := store.Search(ctx, gameID, "it", []string{"Upkeep"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("nessun risultato per 'Upkeep'")
	}
	if !strings.Contains(hits[0].Text, "Testo intermedio") {
		t.Fatalf("il chunk successivo non è stato allegato al primo chunk della fonte:\n%s", hits[0].Text)
	}
}

func TestSearch_AttachesPreviousChunkAtTheEndOfASource(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()
	if err := store.ReplaceSource(ctx, gameID, &mediaID, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento base", Seq: 0,
			Text: "Fase di Upkeep. Ogni giocatore paga una moneta."},
		{ReferenceType: "document", Reference: "Regolamento base", Seq: 1,
			Text: "Testo intermedio che collega le due sezioni della stessa fonte."},
		{ReferenceType: "document", Reference: "Regolamento base", Seq: 2,
			Text: "Fase finale Zibaldone: il gioco termina quando la plancia e' piena."},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	hits, _, err := store.Search(ctx, gameID, "it", []string{"Zibaldone"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("nessun risultato per 'Zibaldone'")
	}
	const zibaldone = "Fase finale Zibaldone"
	idx := strings.Index(hits[0].Text, zibaldone)
	if idx <= 0 {
		t.Fatalf("il chunk precedente non è stato allegato in testa (idx=%d):\n%s", idx, hits[0].Text)
	}
	if strings.Contains(hits[0].Text, "Ogni giocatore paga una moneta") {
		t.Fatalf("è stato allegato anche il chunk NON adiacente (seq 0):\n%s", hits[0].Text)
	}
}

func TestSearch_NoNeighbourForAnInteriorChunk(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()
	chunks := []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento base", Seq: 0, Text: "Primo chunk."},
		{ReferenceType: "document", Reference: "Regolamento base", Seq: 1, Text: "Chunk centrale con la parola collegaunica dentro."},
		{ReferenceType: "document", Reference: "Regolamento base", Seq: 2, Text: "Ultimo chunk."},
	}
	if err := store.ReplaceSource(ctx, gameID, &mediaID, chunks); err != nil {
		t.Fatalf("replace: %v", err)
	}

	hits, _, err := store.Search(ctx, gameID, "it", []string{"collegaunica"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("attesa 1 hit, ottenute %d", len(hits))
	}
	if hits[0].Text != chunks[1].Text {
		t.Fatalf("un chunk interno non deve accumulare vicini, testo = %q", hits[0].Text)
	}
}

func TestSearch_AttachesNeighbourWithinAFAQDespiteNullMediaID(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, _ := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()
	if err := store.ReplaceSource(ctx, gameID, nil, []manuals.SourceChunk{
		{ReferenceType: "faq", Reference: "https://boardgamegeek.com/thread/1",
			ReferenceDetail: "commento del 15/07/2023 08:00", Seq: 0,
			Text: "Si puo' ripeterturnokw il proprio turno in un solo caso."},
		{ReferenceType: "faq", Reference: "https://boardgamegeek.com/thread/1",
			ReferenceDetail: "commento del 15/07/2023 09:00", Seq: 1,
			Text: "Approfondimento: il caso e' quello in cui si e' pescata la carta sbagliata."},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	hits, _, err := store.Search(ctx, gameID, "it", []string{"ripeterturnokw"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("nessun risultato")
	}
	if !strings.Contains(hits[0].Text, "Approfondimento") {
		t.Fatalf("il vicino non è stato allegato a una FAQ con game_media_id NULL:\n%s", hits[0].Text)
	}
}

func TestSearch_FAQNeighboursDoNotLeakAcrossDifferentFAQs(t *testing.T) {
	// Regressione: game_media_id è NULL per OGNI FAQ di OGNI gioco. Se
	// attachNeighbours filtrasse i vicini solo con "game_media_id IS
	// NULL" (senza legarli anche a game_id + reference), il vicino di UNA
	// FAQ potrebbe arrivare dai chunk di una FAQ di un gioco
	// completamente diverso. Verificato a mano: vedi il report.
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameA, _ := seed(t, conn, "GameA", "it", "n/a")
	gameB, _ := seed(t, conn, "GameB", "it", "n/a")

	// gameA ha UNA sola FAQ con UN solo chunk: nessun vicino possibile.
	if err := store.ReplaceSource(ctx, gameA, nil, []manuals.SourceChunk{
		{ReferenceType: "faq", Reference: "https://boardgamegeek.com/thread/A", Seq: 0,
			Text: "Risposta unica UNICOTERMINEA su una regola di gameA."},
	}); err != nil {
		t.Fatalf("replace A: %v", err)
	}
	// gameB ha una FAQ diversa con un seq 1 il cui testo non deve MAI
	// comparire nella risposta di gameA.
	if err := store.ReplaceSource(ctx, gameB, nil, []manuals.SourceChunk{
		{ReferenceType: "faq", Reference: "https://boardgamegeek.com/thread/B", Seq: 0,
			Text: "Prima parte della FAQ di gameB."},
		{ReferenceType: "faq", Reference: "https://boardgamegeek.com/thread/B", Seq: 1,
			Text: "MARCATOREFUGA: questo testo di gameB non deve mai comparire nella risposta di gameA."},
	}); err != nil {
		t.Fatalf("replace B: %v", err)
	}

	hits, _, err := store.Search(ctx, gameA, "it", []string{"UNICOTERMINEA"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("attesa 1 hit su gameA, ottenute %d: %+v", len(hits), hits)
	}
	if strings.Contains(hits[0].Text, "MARCATOREFUGA") {
		t.Fatalf("il vicino di una FAQ di un altro gioco è trapelato nella risposta:\n%s", hits[0].Text)
	}
}

func TestSearch_MediaPathComesFromGameMediaAndIsEmptyForFAQ(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base") // seed usa 'manuale.pdf'
	ctx := context.Background()
	if err := store.ReplaceSource(ctx, gameID, &mediaID, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento base", Seq: 0, Text: "Testo del documento con parolachiavedoc."},
	}); err != nil {
		t.Fatalf("replace doc: %v", err)
	}
	if err := store.ReplaceSource(ctx, gameID, nil, []manuals.SourceChunk{
		{ReferenceType: "faq", Reference: "https://boardgamegeek.com/thread/1", Seq: 0,
			Text: "Testo della FAQ con parolachiavefaq."},
	}); err != nil {
		t.Fatalf("replace faq: %v", err)
	}

	docHits, _, err := store.Search(ctx, gameID, "it", []string{"parolachiavedoc"})
	if err != nil {
		t.Fatalf("search doc: %v", err)
	}
	if len(docHits) != 1 || docHits[0].MediaPath != "manuale.pdf" {
		t.Fatalf("MediaPath del documento atteso 'manuale.pdf', hits = %+v", docHits)
	}

	faqHits, _, err := store.Search(ctx, gameID, "it", []string{"parolachiavefaq"})
	if err != nil {
		t.Fatalf("search faq: %v", err)
	}
	if len(faqHits) != 1 || faqHits[0].MediaPath != "" {
		t.Fatalf("MediaPath di una FAQ deve restare vuoto, hits = %+v", faqHits)
	}
}

func TestMarshalHits_ShapeAndOmitsDiagnosticFields(t *testing.T) {
	out, err := manuals.MarshalHits([]manuals.SourceHit{
		{ReferenceType: "document", Reference: "Regolamento base", ReferenceDetail: "pagina 7",
			Text:      "La partita termina quando la pila si esaurisce.",
			FoundWith: []string{"pila", "fine partita"}, MediaPath: "manuale.pdf"},
	}, []string{"pareggio"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("il payload non è un oggetto JSON valido: %v\n%s", err, out)
	}
	var results []map[string]any
	if err := json.Unmarshal(decoded["risultati"], &results); err != nil {
		t.Fatalf(`manca o non decodifica "risultati": %v`, err)
	}
	if len(results) != 1 {
		t.Fatalf("attesa 1 hit in 'risultati', ottenute %d", len(results))
	}
	r := results[0]
	if r["reference_type"] != "document" || r["reference"] != "Regolamento base" ||
		r["reference_detail"] != "pagina 7" {
		t.Fatalf("campi del contratto persi o alterati: %+v", r)
	}
	if _, ok := r["found_with"]; ok {
		t.Fatalf("FoundWith non deve uscire nel JSON verso il modello: %+v", r)
	}
	if _, ok := r["media_path"]; ok {
		t.Fatalf("MediaPath non deve uscire nel JSON verso il modello: %+v", r)
	}

	var missing []string
	if err := json.Unmarshal(decoded["nessun_risultato_per"], &missing); err != nil {
		t.Fatalf(`manca o non decodifica "nessun_risultato_per": %v`, err)
	}
	if len(missing) != 1 || missing[0] != "pareggio" {
		t.Fatalf("la lista dei mancanti è persa: %v", missing)
	}

	// Senza hit, "risultati" deve essere un array vuoto, non null: il
	// modello non deve gestire due forme diverse di "nessun risultato".
	empty, err := manuals.MarshalHits(nil, []string{"a", "b"})
	if err != nil {
		t.Fatalf("marshal vuoto: %v", err)
	}
	if !strings.Contains(empty, `"risultati":[]`) {
		t.Fatalf(`atteso "risultati":[] quando non ci sono hit, ottenuto: %s`, empty)
	}
}

func containsString(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
