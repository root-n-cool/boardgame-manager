package manuals_test

import (
	"context"
	"database/sql"
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

var regolePagine = []manuals.StoredPage{
	{PageNumber: 3, Heading: "Turno del giocatore", Source: "pdf_text",
		Text: "Turno del giocatore\nAll'inizio del proprio turno il giocatore pesca due carte dal mazzo comune e ne scarta una."},
	{PageNumber: 4, Heading: "Fase di Upkeep", Source: "pdf_text",
		Text: "Fase di Upkeep\nAl termine di ogni round ogni giocatore paga una moneta per ciascun edificio posseduto."},
	{PageNumber: 5, Heading: "Rimescolare", Source: "pdf_text",
		Text: "Rimescolare\nQuando la pila di pesca si esaurisce in una partita a due giocatori si rimescola la pila degli scarti."},
	{PageNumber: 8, Heading: "Conteggio e pareggi", Source: "pdf_text",
		Text: "Conteggio e pareggi\nSe due giocatori totalizzano lo stesso punteggio vince chi ha piu' monete in riserva."},
}

func TestReplacePages_StoresPagesAndBuildsChunks(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()

	if err := store.ReplacePages(ctx, gameID, mediaID, "it", regolePagine); err != nil {
		t.Fatalf("replace: %v", err)
	}

	pages, err := store.ListPages(ctx, mediaID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pages) != 4 {
		t.Fatalf("attese 4 pagine, ottenute %d", len(pages))
	}
	if pages[0].PageNumber != 3 || pages[3].PageNumber != 8 {
		t.Fatalf("pagine non ordinate per numero: %d..%d", pages[0].PageNumber, pages[3].PageNumber)
	}
	if pages[1].Heading != "Fase di Upkeep" {
		t.Fatalf("heading perso: %q", pages[1].Heading)
	}

	var chunks int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM manual_chunk WHERE game_id = ?`, gameID).Scan(&chunks); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if chunks == 0 {
		t.Fatal("ReplacePages non ha costruito nessun chunk")
	}
	var indexed int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM manual_chunk_fts`).Scan(&indexed); err != nil {
		t.Fatalf("count fts: %v", err)
	}
	if indexed != chunks {
		t.Fatalf("l'indice FTS5 ha %d righe e i chunk %d: i trigger non allineano", indexed, chunks)
	}
}

func TestReplacePages_IsIdempotentAndRebuilds(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()

	if err := store.ReplacePages(ctx, gameID, mediaID, "it", regolePagine); err != nil {
		t.Fatalf("first replace: %v", err)
	}
	// Un secondo salvataggio con una pagina sola deve lasciare una pagina
	// sola: è il comportamento del bottone "salva" dell'admin, che
	// sostituisce quel che c'era.
	if err := store.ReplacePages(ctx, gameID, mediaID, "it", regolePagine[:1]); err != nil {
		t.Fatalf("second replace: %v", err)
	}
	pages, _ := store.ListPages(ctx, mediaID)
	if len(pages) != 1 {
		t.Fatalf("attesa 1 pagina dopo il secondo salvataggio, ottenute %d", len(pages))
	}
	var chunks int
	conn.QueryRow(`SELECT COUNT(*) FROM manual_chunk WHERE game_id = ?`, gameID).Scan(&chunks)
	if chunks != 1 {
		t.Fatalf("i chunk non sono stati ricostruiti: %d", chunks)
	}
	var indexed int
	conn.QueryRow(`SELECT COUNT(*) FROM manual_chunk_fts`).Scan(&indexed)
	if indexed != 1 {
		t.Fatalf("l'indice FTS5 ha %d righe dopo la ricostruzione", indexed)
	}

	// I conteggi da soli non distinguono "ricostruito col contenuto giusto"
	// da "ricostruito con contenuto vecchio che per caso conta uguale":
	// 'Upkeep' era solo nella pagina 4, scartata dal secondo salvataggio
	// (regolePagine[:1] è solo la pagina 3). Deve sparire anche dalla
	// ricerca, non solo dal conteggio.
	res, err := store.Search(ctx, gameID, "it", []string{"Upkeep"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Hits) != 0 {
		t.Fatalf("'Upkeep' era nella pagina scartata dal secondo salvataggio, ma è ancora trovabile: %+v", res.Hits)
	}
	if !containsString(res.Missing, "Upkeep") {
		t.Fatalf("'Upkeep' doveva comparire fra i mancanti dopo la ricostruzione, Missing = %v", res.Missing)
	}
}

func TestSearch_FindsByKeywordAndReportsTheMisses(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()
	if err := store.ReplacePages(ctx, gameID, mediaID, "it", regolePagine); err != nil {
		t.Fatalf("replace: %v", err)
	}

	res, err := store.Search(ctx, gameID, "it", []string{"Upkeep", "rimescola", "ripescare"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatal("nessun risultato per 'Upkeep', che è nel manuale")
	}
	var pages []int
	for _, h := range res.Hits {
		pages = append(pages, h.PageNumber)
		if h.ManualTitle != "Regolamento base" {
			t.Fatalf("titolo del manuale perso: %q", h.ManualTitle)
		}
		if h.LanguageCode != "it" {
			t.Fatalf("lingua persa: %q", h.LanguageCode)
		}
		if len(h.FoundWith) == 0 {
			t.Fatalf("hit a pagina %d senza FoundWith", h.PageNumber)
		}
	}
	if !containsInt(pages, 4) {
		t.Fatalf("'Upkeep' è a pagina 4, pagine trovate %v", pages)
	}
	// 'ripescare' non c'è nel manuale: deve comparire fra i mancanti, che
	// è l'informazione con cui il modello decide se riprovare.
	if !containsString(res.Missing, "ripescare") {
		t.Fatalf("'ripescare' doveva essere fra i mancanti, Missing = %v", res.Missing)
	}
	if containsString(res.Missing, "Upkeep") {
		t.Fatalf("'Upkeep' è stato trovato ma è fra i mancanti: %v", res.Missing)
	}
}

func TestSearch_DeduplicatesAndAccumulatesFoundWith(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()
	store.ReplacePages(ctx, gameID, mediaID, "it", regolePagine)

	// Due parole chiave che colpiscono lo stesso chunk: deve uscire una
	// volta sola, con entrambe in FoundWith.
	res, err := store.Search(ctx, gameID, "it", []string{"pila", "rimescola"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	seen := map[int]int{}
	for _, h := range res.Hits {
		seen[h.PageNumber]++
	}
	for page, n := range seen {
		if n > 1 {
			t.Fatalf("pagina %d compare %d volte: manca la deduplicazione", page, n)
		}
	}
	for _, h := range res.Hits {
		if h.PageNumber == 5 && len(h.FoundWith) < 2 {
			t.Fatalf("la pagina 5 è stata trovata da due parole ma FoundWith = %v", h.FoundWith)
		}
	}
}

func TestSearch_SurvivesAnApostrophe(t *testing.T) {
	// Regressione trovata in fase di design: una parola chiave con
	// l'apostrofo passata grezza a MATCH dà "fts5: syntax error".
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()
	store.ReplacePages(ctx, gameID, mediaID, "it", regolePagine)

	// "all'inizio" è l'unica delle tre che ha davvero un match nel
	// corpus (pagina 3: "All'inizio del proprio turno..."), quindi è
	// l'unica che può dimostrare che il fallback su escapeFTS non si limita
	// a evitare l'errore ma trova ancora qualcosa. Le altre due restano nel
	// test solo per il caso "non erra" (virgolette sbilanciate, un
	// apostrofo in mezzo a una frase lunga): non hanno match nel corpus e
	// non potrebbero provare niente di più, di proposito.
	res, err := store.Search(ctx, gameID, "it", []string{"all'inizio"})
	if err != nil {
		t.Fatalf(`Search("all'inizio") ha restituito errore: %v`, err)
	}
	if len(res.Hits) == 0 {
		t.Fatal(`"all'inizio" doveva trovare la pagina 3, nessun hit`)
	}
	if containsString(res.Missing, "all'inizio") {
		t.Fatalf(`"all'inizio" è fra i mancanti nonostante il match: %v`, res.Missing)
	}

	for _, kw := range []string{`virgoletta"dentro`, "chi vince in caso di parita'"} {
		res, err := store.Search(ctx, gameID, "it", []string{kw})
		if err != nil {
			t.Fatalf("Search(%q) ha restituito errore: %v", kw, err)
		}
		_ = res // qui il punto è solo che non erra: non c'è nessun match da provare
	}
}

func TestSearch_DoesNotLeakIntoAnotherGame(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameA, mediaA := seed(t, conn, "Wingspan", "it", "Regolamento A")
	gameB, mediaB := seed(t, conn, "Brass", "it", "Regolamento B")
	store.ReplacePages(ctx, gameA, mediaA, "it", regolePagine)
	store.ReplacePages(ctx, gameB, mediaB, "it", []manuals.StoredPage{
		{PageNumber: 1, Source: "manual", Text: "Fase di Upkeep di un gioco completamente diverso."},
	})

	res, err := store.Search(ctx, gameA, "it", []string{"Upkeep"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	for _, h := range res.Hits {
		if h.ManualTitle != "Regolamento A" {
			t.Fatalf("la ricerca su gameA ha restituito %q: sconfina su un altro gioco", h.ManualTitle)
		}
	}
}

func TestSearch_PrefersTheRequestedLanguage(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID, mediaIT := seed(t, conn, "Wingspan", "it", "Regolamento italiano")

	// Una seconda lingua sullo stesso gioco, con un manuale che contiene la
	// stessa parola chiave.
	l, _ := conn.ExecContext(ctx,
		`INSERT INTO game_languages (game_id, language_code, is_base_language, name)
		 VALUES (?, 'en', 0, 'Wingspan')`, gameID)
	langEN, _ := l.LastInsertId()
	m, _ := conn.ExecContext(ctx,
		`INSERT INTO game_media (game_language_id, type, url_or_path, title)
		 VALUES (?, 'file', 'rules-en.pdf', 'English rulebook')`, langEN)
	mediaEN, _ := m.LastInsertId()

	store.ReplacePages(ctx, gameID, mediaIT, "it", []manuals.StoredPage{
		{PageNumber: 4, Source: "manual", Text: "Fase di Upkeep: si paga una moneta per edificio."},
	})
	store.ReplacePages(ctx, gameID, mediaEN, "en", []manuals.StoredPage{
		{PageNumber: 9, Source: "manual", Text: "Upkeep phase: pay one coin per building."},
	})

	res, err := store.Search(ctx, gameID, "it", []string{"Upkeep"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatal("nessun risultato")
	}
	if res.Hits[0].LanguageCode != "it" {
		t.Fatalf("con preferLang=it il primo risultato deve essere italiano, è %q", res.Hits[0].LanguageCode)
	}
	// Entrambe le lingue restano cercabili: il modello deve poter citare il
	// regolamento inglese quando l'italiano non dice nulla.
	if len(res.Hits) < 2 {
		t.Fatalf("attesi risultati da entrambe le lingue, ottenuti %d", len(res.Hits))
	}
}

func TestCorpusAndHasPages(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")

	has, err := store.HasPages(ctx, gameID)
	if err != nil {
		t.Fatalf("has pages: %v", err)
	}
	if has {
		t.Fatal("un gioco senza manuale preparato non ha pagine")
	}

	store.ReplacePages(ctx, gameID, mediaID, "it", regolePagine)

	has, _ = store.HasPages(ctx, gameID)
	if !has {
		t.Fatal("dopo ReplacePages il gioco ha pagine")
	}

	corpus, err := store.Corpus(ctx, gameID)
	if err != nil {
		t.Fatalf("corpus: %v", err)
	}
	if len(corpus.Manuals) != 1 {
		t.Fatalf("atteso 1 manuale nel corpus, ottenuti %d", len(corpus.Manuals))
	}
	if len(corpus.Manuals[0].Pages) != 4 {
		t.Fatalf("attese 4 pagine nel corpus, ottenute %d", len(corpus.Manuals[0].Pages))
	}
	if corpus.Chars == 0 {
		t.Fatal("Chars a zero: è la misura con cui si decide se il manuale entra intero nel contesto")
	}
}

func TestCorpus_KeepsDistinctMediaSeparateWhenUntitled(t *testing.T) {
	// Regressione: raggruppare Corpus per (title, language_code) fa
	// collassare due media distinti nella stessa lingua quando entrambi non
	// hanno titolo, perché la query fa COALESCE(title, 'Manuale') — stesso
	// titolo fittizio, stessa chiave. Il secondo Path sparisce, e Path è ciò
	// che trasforma "pag. 7" in un link al PDF giusto: un Path sbagliato
	// manda al file sbagliato.
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()

	g, err := conn.ExecContext(ctx, `INSERT INTO games (name, seats) VALUES (?, 1)`, "Brass")
	if err != nil {
		t.Fatalf("insert game: %v", err)
	}
	gameID, _ := g.LastInsertId()
	l, err := conn.ExecContext(ctx,
		`INSERT INTO game_languages (game_id, language_code, is_base_language, name)
		 VALUES (?, 'it', 1, ?)`, gameID, "Brass")
	if err != nil {
		t.Fatalf("insert language: %v", err)
	}
	langID, _ := l.LastInsertId()

	// Due media distinti sotto la stessa lingua, entrambi senza titolo.
	m1, err := conn.ExecContext(ctx,
		`INSERT INTO game_media (game_language_id, type, url_or_path, title) VALUES (?, 'file', 'base.pdf', NULL)`, langID)
	if err != nil {
		t.Fatalf("insert media 1: %v", err)
	}
	media1, _ := m1.LastInsertId()
	m2, err := conn.ExecContext(ctx,
		`INSERT INTO game_media (game_language_id, type, url_or_path, title) VALUES (?, 'file', 'espansione.pdf', NULL)`, langID)
	if err != nil {
		t.Fatalf("insert media 2: %v", err)
	}
	media2, _ := m2.LastInsertId()

	if err := store.ReplacePages(ctx, gameID, media1, "it", []manuals.StoredPage{
		{PageNumber: 1, Source: "manual", Text: "Regole base."},
	}); err != nil {
		t.Fatalf("replace media 1: %v", err)
	}
	if err := store.ReplacePages(ctx, gameID, media2, "it", []manuals.StoredPage{
		{PageNumber: 1, Source: "manual", Text: "Regole dell'espansione."},
	}); err != nil {
		t.Fatalf("replace media 2: %v", err)
	}

	corpus, err := store.Corpus(ctx, gameID)
	if err != nil {
		t.Fatalf("corpus: %v", err)
	}
	if len(corpus.Manuals) != 2 {
		t.Fatalf("attesi 2 manuali distinti, ottenuti %d: %+v", len(corpus.Manuals), corpus.Manuals)
	}
	paths := map[string]int{}
	for _, m := range corpus.Manuals {
		paths[m.Path]++
		if len(m.Pages) != 1 {
			t.Fatalf("manuale con path %q: attesa 1 pagina, ottenute %d", m.Path, len(m.Pages))
		}
	}
	if paths["base.pdf"] != 1 || paths["espansione.pdf"] != 1 {
		t.Fatalf("i due path distinti non sono entrambi presenti una volta sola: %v", paths)
	}
}

func TestDeletePages_AndMediaCascade(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	ctx := context.Background()
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	store.ReplacePages(ctx, gameID, mediaID, "it", regolePagine)

	if err := store.DeletePages(ctx, mediaID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	pages, _ := store.ListPages(ctx, mediaID)
	if len(pages) != 0 {
		t.Fatalf("attese 0 pagine dopo DeletePages, ottenute %d", len(pages))
	}
	var chunks int
	conn.QueryRow(`SELECT COUNT(*) FROM manual_chunk WHERE game_id = ?`, gameID).Scan(&chunks)
	if chunks != 0 {
		t.Fatalf("DeletePages ha lasciato %d chunk", chunks)
	}

	// La cascata: cancellare il media porta via pagine e chunk senza che
	// nessuno lo chieda. Serve che le foreign key siano attive nella
	// connessione (db.Open lo fa con PRAGMA foreign_keys).
	store.ReplacePages(ctx, gameID, mediaID, "it", regolePagine)
	if _, err := conn.ExecContext(ctx, `DELETE FROM game_media WHERE id = ?`, mediaID); err != nil {
		t.Fatalf("delete media: %v", err)
	}
	var leftPages, leftChunks int
	conn.QueryRow(`SELECT COUNT(*) FROM manual_page WHERE game_media_id = ?`, mediaID).Scan(&leftPages)
	conn.QueryRow(`SELECT COUNT(*) FROM manual_chunk WHERE game_media_id = ?`, mediaID).Scan(&leftChunks)
	if leftPages != 0 || leftChunks != 0 {
		t.Fatalf("la cascata ha lasciato %d pagine e %d chunk", leftPages, leftChunks)
	}
}

func TestFormatSearchResult(t *testing.T) {
	out := manuals.FormatSearchResult(manuals.SearchResult{
		Hits: []manuals.Hit{
			{PageNumber: 7, LanguageCode: "it", ManualTitle: "Regolamento base",
				Text: "La partita termina quando la pila si esaurisce.", FoundWith: []string{"pila", "fine partita"}},
		},
		Missing: []string{"pareggio"},
	})
	for _, want := range []string{"[1]", "pag. 7", "Regolamento base", "pila, fine partita", "La partita termina", "pareggio"} {
		if !strings.Contains(out, want) {
			t.Fatalf("il payload non contiene %q:\n%s", want, out)
		}
	}

	empty := manuals.FormatSearchResult(manuals.SearchResult{Missing: []string{"a", "b"}})
	if !strings.Contains(strings.ToLower(empty), "nessun risultato") {
		t.Fatalf("senza risultati il payload deve dirlo al modello:\n%s", empty)
	}
}

func TestSearch_AttachesTheNeighbourChunkAtAPageBoundary(t *testing.T) {
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()

	// Una pagina lunga abbastanza da produrre più chunk, con la parola
	// chiave nel PRIMO e il seguito nel secondo.
	coda := strings.Repeat("Testo che continua il regolamento oltre il taglio. ", 30)
	if err := store.ReplacePages(ctx, gameID, mediaID, "it", []manuals.StoredPage{
		{PageNumber: 4, Source: "manual",
			Text: "Fase di Upkeep. Ogni giocatore paga una moneta. " + coda},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	var chunks int
	conn.QueryRow(`SELECT COUNT(*) FROM manual_chunk WHERE page_number = 4`).Scan(&chunks)
	if chunks < 2 {
		t.Skipf("la pagina ha prodotto %d chunk: niente bordo da verificare", chunks)
	}

	res, err := store.Search(ctx, gameID, "it", []string{"Upkeep"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatal("nessun risultato")
	}
	// Il chunk trovato è il primo della pagina: il payload deve portarsi
	// dietro anche il seguito, altrimenti una regola che continua nel
	// chunk successivo arriva al modello troncata.
	if !strings.Contains(res.Hits[0].Text, "continua il regolamento") {
		t.Fatalf("il chunk adiacente non è stato allegato:\n%s", res.Hits[0].Text)
	}
}

func TestSearch_AttachesThePreviousChunkAtTheLastChunkBoundary(t *testing.T) {
	// Simmetrico al test precedente: lì la parola chiave cade nel PRIMO
	// chunk della pagina (ramo "h.seq == 0" di attachNeighbours, allega il
	// seguito); qui cade nell'ULTIMO (ramo "h.seq == maxSeq", allega quel
	// che precede). Senza questo test il secondo ramo non è mai esercitato:
	// uno scambio di "after" o un off-by-one lì passerebbe inosservato.
	conn := newTestDB(t)
	store := manuals.NewStore(conn)
	gameID, mediaID := seed(t, conn, "Wingspan", "it", "Regolamento base")
	ctx := context.Background()

	coda := strings.Repeat("Testo che continua il regolamento oltre il taglio. ", 30)
	if err := store.ReplacePages(ctx, gameID, mediaID, "it", []manuals.StoredPage{
		{PageNumber: 4, Source: "manual",
			Text: "Testo introduttivo del regolamento. " + coda +
				"Fase finale Zibaldone: il gioco termina quando la plancia e' piena."},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	var chunks int
	conn.QueryRow(`SELECT COUNT(*) FROM manual_chunk WHERE page_number = 4`).Scan(&chunks)
	if chunks < 2 {
		t.Skipf("la pagina ha prodotto %d chunk: niente bordo da verificare", chunks)
	}

	res, err := store.Search(ctx, gameID, "it", []string{"Zibaldone"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatal("nessun risultato per 'Zibaldone'")
	}
	// Il chunk trovato è l'ultimo della pagina: il payload deve portarsi
	// dietro anche quel che lo precede — e "prima", non "dopo": un
	// HasPrefix invece di un Contains, perché uno scambio del flag "after"
	// (attaccherebbe comunque il testo giusto, solo in coda anziché in
	// testa) altrimenti passerebbe inosservato.
	if !strings.HasPrefix(res.Hits[0].Text, "Testo introduttivo") {
		t.Fatalf("il chunk precedente non è in testa (o non è allegato):\n%s", res.Hits[0].Text)
	}
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func containsString(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
