package httpapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"boardgames-manager/internal/ai"
	"boardgames-manager/internal/bgg"
	"boardgames-manager/internal/httpapi"
	"boardgames-manager/internal/manuals"
	"boardgames-manager/internal/websearch"
)

// fakeAsker cattura la AskRequest che l'handler costruisce: è lì che si
// verifica che il corpus, l'indice e la ricerca siano stati agganciati.
//
// searchQueries simula il modello che chiama lo strumento di ricerca prima
// di rispondere: ogni voce è le parole chiave di UNA chiamata, eseguita
// davvero contro req.Search. Serve perché la mappa reference → percorso che
// linkifyCitations usa si popola SOLO dalle hit che una ricerca restituisce
// (vedi ask_handler.go): senza simulare la chiamata, un test che imposta
// solo `answer` non troverebbe mai nessun link, qualunque sia il testo
// della risposta.
type fakeAsker struct {
	got           ai.AskRequest
	answer        string
	err           error
	searchQueries [][]string
}

func (f *fakeAsker) Ask(ctx context.Context, req ai.AskRequest) (string, error) {
	f.got = req
	for _, keywords := range f.searchQueries {
		if req.Search != nil {
			_, _ = req.Search(ctx, keywords)
		}
	}
	if f.err != nil {
		return "", f.err
	}
	return f.answer, nil
}

// seedGameWithPreparedManual crea un gioco con un manuale già indicizzato.
func seedGameWithPreparedManual(t *testing.T, conn *sql.DB) int64 {
	t.Helper()
	return seedGameWithManualPage4Text(t, conn, "Ogni giocatore paga una moneta per ciascun edificio.")
}

// seedGameWithManualPage4Text crea un gioco con un manuale il cui testo di
// pagina 4 è quello passato: usato per distinguere, nei test di scoping,
// il contenuto di un gioco da quello di un altro gioco altrimenti
// identico (stesso titolo di sezione, stesso numero di pagina).
func seedGameWithManualPage4Text(t *testing.T, conn *sql.DB, page4Text string) int64 {
	t.Helper()
	ctx := context.Background()
	g, err := conn.ExecContext(ctx, `INSERT INTO games (name, seats) VALUES ('Wingspan', 1)`)
	if err != nil {
		t.Fatalf("insert game: %v", err)
	}
	gameID, _ := g.LastInsertId()
	l, _ := conn.ExecContext(ctx,
		`INSERT INTO game_languages (game_id, language_code, is_base_language, name)
		 VALUES (?, 'it', 1, 'Wingspan')`, gameID)
	langID, _ := l.LastInsertId()
	m, _ := conn.ExecContext(ctx,
		`INSERT INTO game_media (game_language_id, type, url_or_path, title)
		 VALUES (?, 'file', 'manuale.pdf', 'Regolamento base')`, langID)
	mediaID, _ := m.LastInsertId()

	store := manuals.NewStore(conn)
	// L'indice FTS5 (game_source_chunk_fts) copre solo la colonna `text`,
	// non `heading` (vedi il trigger in 0015_game_sources.sql): il testo
	// del chunk deve quindi contenere davvero la parola che i test cercano
	// ("Upkeep"), non bastare che sia nel titolo.
	if err := store.ReplaceSource(ctx, gameID, &mediaID, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento base", ReferenceDetail: "pagina 4",
			Heading: "Fase di Upkeep", LanguageCode: "it", Seq: 0,
			Text: "Fase di Upkeep. " + page4Text},
		{ReferenceType: "document", Reference: "Regolamento base", ReferenceDetail: "pagina 7",
			Heading: "Fine partita", LanguageCode: "it", Seq: 1,
			Text: "Fine partita. La partita termina quando la pila di pesca si esaurisce."},
	}); err != nil {
		t.Fatalf("replace source: %v", err)
	}
	return gameID
}

func postAsk(t *testing.T, router http.Handler, gameID int64, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/api/games/"+strconv.FormatInt(gameID, 10)+"/ask", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestAskHandler_AnswersInTheShapeDeepChatExpects(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	asker := &fakeAsker{answer: "La partita finisce subito.\n\n_Regolamento base — pag. 7_"}
	server.Asker = asker
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	rec := postAsk(t, router, gameID,
		`{"messages":[{"role":"user","text":"finite le carte che si fa?"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	// deep-chat legge {"text": ...}: qualunque altra forma gli fa mostrare
	// un errore generico invece della risposta.
	var body struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("risposta non JSON: %v", err)
	}
	if !strings.Contains(body.Text, "pag. 7") {
		t.Fatalf("il campo text non contiene la risposta: %q", body.Text)
	}

	// La AskRequest deve portare il nome del gioco e lo storico.
	if asker.got.GameName != "Wingspan" {
		t.Fatalf("nome gioco: %q", asker.got.GameName)
	}
	if len(asker.got.Turns) != 1 || asker.got.Turns[0].Text != "finite le carte che si fa?" {
		t.Fatalf("storico non passato: %+v", asker.got.Turns)
	}
	// L'indice per fonte deve arrivare pieno: senza di esso il modello perde
	// l'unica cosa che gli evita la ricerca esplorativa (vedi
	// askSystemPrompt in internal/ai/ask.go). Si verifica un titolo di
	// sezione vero e proprio, non una sottostringa generica che potrebbe
	// comparire per un altro motivo.
	if !strings.Contains(asker.got.CorpusIndex, "Fase di Upkeep") {
		t.Fatalf("CorpusIndex non contiene i titoli di sezione delle fonti: %q", asker.got.CorpusIndex)
	}
	if asker.got.Search == nil {
		t.Fatal("Search non agganciata: con un manuale lungo il modello non avrebbe come cercare")
	}
}

func TestAskHandler_TurnsPageCitationsIntoLinksToThePDF(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	server.Asker = &fakeAsker{
		answer: "La partita finisce subito. Regolamento base, pagina 7.",
		// Il modello ha chiamato lo strumento di ricerca e "pesca" ha
		// trovato il chunk di pagina 7 ("...pila di pesca si esaurisce"):
		// è questa hit vera a popolare la mappa reference → percorso che
		// linkifyCitations usa.
		searchQueries: [][]string{{"pesca"}},
	}
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	rec := postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
	var body struct {
		Text string `json:"text"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)

	// È il pezzo che rende verificabile la risposta: si apre il manuale a
	// quella pagina invece di fidarsi.
	if !strings.Contains(body.Text, "[Regolamento base, pagina 7](/api/uploads/manuale.pdf#page=7)") {
		t.Fatalf("la citazione non è diventata un link al PDF: %q", body.Text)
	}
}

func TestAskHandler_DoesNotDoubleLinkACitation(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	server.Asker = &fakeAsker{answer: "Vedi [pag. 7](/api/uploads/manuale.pdf#page=7)."}
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	rec := postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
	var body struct {
		Text string `json:"text"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if strings.Count(body.Text, "/api/uploads/") != 1 {
		t.Fatalf("un link già presente non va riscritto: %q", body.Text)
	}
}

// addSecondManual aggiunge al gioco una seconda lingua con un secondo PDF
// già indicizzato: la situazione in cui la citazione del modello può
// riferirsi all'uno o all'altro.
func addSecondManual(t *testing.T, conn *sql.DB, gameID int64) {
	t.Helper()
	ctx := context.Background()
	l, err := conn.ExecContext(ctx,
		`INSERT INTO game_languages (game_id, language_code, is_base_language, name)
		 VALUES (?, 'en', 0, 'Wingspan')`, gameID)
	if err != nil {
		t.Fatalf("insert language: %v", err)
	}
	langEN, _ := l.LastInsertId()
	m, err := conn.ExecContext(ctx,
		`INSERT INTO game_media (game_language_id, type, url_or_path, title)
		 VALUES (?, 'file', 'rules-en.pdf', 'English rulebook')`, langEN)
	if err != nil {
		t.Fatalf("insert media: %v", err)
	}
	mediaEN, _ := m.LastInsertId()
	if err := manuals.NewStore(conn).ReplaceSource(ctx, gameID, &mediaEN, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "English rulebook", ReferenceDetail: "pagina 12",
			Heading: "Upkeep phase", LanguageCode: "en", Seq: 0,
			Text: "Each player pays one coin per building."},
	}); err != nil {
		t.Fatalf("replace source: %v", err)
	}
}

func TestAskHandler_LinksEachCitationToItsOwnManual(t *testing.T) {
	// È il test che dimostra che il bug è chiuso: prima, con due manuali,
	// la riscrittura non sapeva a quale dei due si riferisse una citazione
	// e non linkava NIENTE (vedi il commit "fix: do not link page
	// citations when a game has several manuals"). Ora l'handler conosce
	// la fonte di ogni hit davvero ricevuta, quindi linka CIASCUNA
	// citazione al proprio file — e l'asserzione che conta è che la prima
	// vada al primo file e la seconda al secondo, non solo che un link
	// qualunque compaia.
	server, conn := newTestServerWithDB(t)
	gameID := seedGameWithPreparedManual(t, conn)
	addSecondManual(t, conn, gameID)

	server.Asker = &fakeAsker{
		answer: "Sì: vedi Regolamento base, pagina 7, e anche English rulebook, pagina 12.",
		// Due chiamate al tool, una per manuale: "pesca" trova solo il
		// chunk italiano (pagina 7, manuale.pdf), "coin" solo quello
		// inglese (pagina 12, rules-en.pdf).
		searchQueries: [][]string{{"pesca"}, {"coin"}},
	}
	router := httpapi.NewRouter(server)

	rec := postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
	var body struct {
		Text string `json:"text"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)

	// L'asserzione da evitare è "la risposta contiene /api/uploads/": è
	// vera anche se entrambi i link puntassero al primo file, che è
	// esattamente il bug. Si verifica invece l'href ESATTO di ciascuna
	// citazione.
	if !strings.Contains(body.Text, "[Regolamento base, pagina 7](/api/uploads/manuale.pdf#page=7)") {
		t.Fatalf("la prima citazione non punta al suo manuale: %q", body.Text)
	}
	if !strings.Contains(body.Text, "[English rulebook, pagina 12](/api/uploads/rules-en.pdf#page=12)") {
		t.Fatalf("la seconda citazione non punta al suo manuale: %q", body.Text)
	}
}

func TestAskHandler_LinksDisambiguatedReferencesToTheirOwnManual(t *testing.T) {
	// TestAskHandler_LinksEachCitationToItsOwnManual usa due manuali con
	// TITOLI GIÀ distinti ("Regolamento base", "English rulebook"): in quel
	// caso un'implementazione a lookup-per-titolo funzionerebbe per caso,
	// perché ogni titolo individua un solo media. Il caso che DAVVERO mette
	// alla prova la mappa costruita dalle hit (invece che da un lookup per
	// titolo) è due media con lo STESSO titolo: sourceReference (in
	// manuals_handlers.go, Task 5) li disambigua aggiungendo la lingua al
	// secondo — "Regolamento" e "Regolamento (it)" — e un lookup per
	// titolo non saprebbe scegliere fra i due, risolvendo entrambe le
	// citazioni sullo stesso file.
	server, conn := newTestServerWithDB(t)
	ctx := context.Background()

	g, err := conn.ExecContext(ctx, `INSERT INTO games (name, seats) VALUES ('Wingspan', 1)`)
	if err != nil {
		t.Fatalf("insert game: %v", err)
	}
	gameID, _ := g.LastInsertId()
	l, err := conn.ExecContext(ctx,
		`INSERT INTO game_languages (game_id, language_code, is_base_language, name)
		 VALUES (?, 'it', 1, 'Wingspan')`, gameID)
	if err != nil {
		t.Fatalf("insert language: %v", err)
	}
	langID, _ := l.LastInsertId()

	// Due media, STESSO titolo, url_or_path distinti: esattamente il caso
	// che sourceReference disambigua in scrittura.
	m1, err := conn.ExecContext(ctx,
		`INSERT INTO game_media (game_language_id, type, url_or_path, title)
		 VALUES (?, 'file', 'manuale-v1.pdf', 'Regolamento')`, langID)
	if err != nil {
		t.Fatalf("insert media 1: %v", err)
	}
	media1, _ := m1.LastInsertId()
	m2, err := conn.ExecContext(ctx,
		`INSERT INTO game_media (game_language_id, type, url_or_path, title)
		 VALUES (?, 'file', 'manuale-v2.pdf', 'Regolamento')`, langID)
	if err != nil {
		t.Fatalf("insert media 2: %v", err)
	}
	media2, _ := m2.LastInsertId()

	// Le reference sono quelle che sourceReference produce DAVVERO per due
	// fonti con lo stesso titolo e la stessa lingua (letta in
	// manuals_handlers.go, non indovinata): la prima resta il titolo
	// così com'è, la seconda guadagna " (it)" perché "Regolamento" risulta
	// già used quando si indicizza la seconda.
	store := manuals.NewStore(conn)
	if err := store.ReplaceSource(ctx, gameID, &media1, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento", ReferenceDetail: "pagina 3",
			Heading: "Preparazione", LanguageCode: "it", Seq: 0,
			Text: "Preparazione. Si mescolano le carte e si formano i mazzi."},
	}); err != nil {
		t.Fatalf("replace source 1: %v", err)
	}
	if err := store.ReplaceSource(ctx, gameID, &media2, []manuals.SourceChunk{
		{ReferenceType: "document", Reference: "Regolamento (it)", ReferenceDetail: "pagina 9",
			Heading: "Varianti", LanguageCode: "it", Seq: 0,
			Text: "Varianti. Si possono aggiungere le espansioni."},
	}); err != nil {
		t.Fatalf("replace source 2: %v", err)
	}

	server.Asker = &fakeAsker{
		answer: "Vedi Regolamento, pagina 3, e anche Regolamento (it), pagina 9.",
		// Una ricerca per manuale: "mescolano" trova solo il chunk del
		// primo media, "espansioni" solo quello del secondo.
		searchQueries: [][]string{{"mescolano"}, {"espansioni"}},
	}
	router := httpapi.NewRouter(server)

	rec := postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
	var body struct {
		Text string `json:"text"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)

	if !strings.Contains(body.Text, "[Regolamento, pagina 3](/api/uploads/manuale-v1.pdf#page=3)") {
		t.Fatalf("la reference non disambiguata non punta al primo manuale: %q", body.Text)
	}
	if !strings.Contains(body.Text, "[Regolamento (it), pagina 9](/api/uploads/manuale-v2.pdf#page=9)") {
		t.Fatalf("la reference disambiguata non punta al secondo manuale: %q", body.Text)
	}
}

func TestAskHandler_BuildsTheCorpusIndexGroupedBySource(t *testing.T) {
	// Senza l'indice il modello perde l'unica cosa che gli evita la
	// ricerca esplorativa (vedi askSystemPrompt in internal/ai/ask.go).
	// Deve arrivare raggruppato per fonte, coi titoli nell'ordine di seq
	// che manuals.Store.Summary già restituisce — non un elenco piatto.
	server, conn := newTestServerWithDB(t)
	asker := &fakeAsker{answer: "ok"}
	server.Asker = asker
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)

	want := "Regolamento base — Fase di Upkeep · Fine partita"
	if !strings.Contains(asker.got.CorpusIndex, want) {
		t.Fatalf("CorpusIndex non raggruppa i titoli per fonte nell'ordine atteso: %q", asker.got.CorpusIndex)
	}
}

func TestAskHandler_CapsWhatAnAnonymousRequestCanSendToTheProvider(t *testing.T) {
	// Rotta pubblica, non autenticata, il cui contenuto finisce nel prompt
	// di un provider che si paga a token: il rate limit governa la
	// frequenza, non la dimensione. Cento turni da 100 KB sarebbero una
	// richiesta sola, perfettamente legittima.
	server, conn := newTestServerWithDB(t)
	asker := &fakeAsker{answer: "ok"}
	server.Asker = asker
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	// Sotto il tetto sul body (128 KB) ma ben sopra gli altri due: 41 turni
	// per un tetto di 30, e uno da 5000 caratteri per un tetto di 4000. Il
	// turno lungo sta in fondo, così sopravvive al taglio ed è davvero la
	// troncatura per turno a doverlo accorciare.
	var msgs []string
	for i := 0; i < 39; i++ {
		msgs = append(msgs, `{"role":"user","text":"`+strings.Repeat("a", 2000)+`"}`)
	}
	msgs = append(msgs, `{"role":"user","text":"`+strings.Repeat("b", 5000)+`"}`)
	msgs = append(msgs, `{"role":"user","text":"la domanda vera"}`)
	rec := postAsk(t, router, gameID, `{"messages":[`+strings.Join(msgs, ",")+`]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}

	if len(asker.got.Turns) > 30 {
		t.Fatalf("%d turni passati al provider: nessun tetto sul numero", len(asker.got.Turns))
	}
	total := 0
	for _, turn := range asker.got.Turns {
		if len(turn.Text) > 4000 {
			t.Fatalf("un turno da %d caratteri è arrivato intero al provider", len(turn.Text))
		}
		total += len(turn.Text)
	}
	if total > 24000 {
		t.Fatalf("%d caratteri di conversazione passati al provider: nessun tetto complessivo", total)
	}
	// Il taglio deve cadere sul contesto vecchio, non sulla domanda: è
	// l'ultimo messaggio, ed è l'unica cosa a cui rispondere.
	if len(asker.got.Turns) == 0 || asker.got.Turns[len(asker.got.Turns)-1].Text != "la domanda vera" {
		t.Fatal("il taglio ha buttato via la domanda invece del contesto più vecchio")
	}
}

func TestAskHandler_RejectsAnOversizedBody(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	asker := &fakeAsker{answer: "ok"}
	server.Asker = asker
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	// Mezzo megabyte di JSON: senza MaxBytesReader il body viene letto,
	// deserializzato e tenuto in memoria per intero prima di qualunque
	// controllo.
	rec := postAsk(t, router, gameID,
		`{"messages":[{"role":"user","text":"`+strings.Repeat("a", 512*1024)+`"}]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("atteso 400 su un body fuori misura, ottenuto %d", rec.Code)
	}
	if asker.got.GameName != "" {
		t.Fatal("il provider è stato chiamato lo stesso: il tetto sul body non ha fermato niente")
	}
}

func TestAskHandler_MapsDeepChatRolesToTheModel(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	asker := &fakeAsker{answer: "ok"}
	server.Asker = asker
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	// deep-chat usa "ai" per le sue risposte, non "assistant".
	postAsk(t, router, gameID, `{"messages":[
	  {"role":"user","text":"come si contano i punti?"},
	  {"role":"ai","text":"Ogni edificio vale i punti stampati."},
	  {"role":"user","text":"e se siamo pari?"}
	]}`)

	if len(asker.got.Turns) != 3 {
		t.Fatalf("attesi 3 turni, ottenuti %d", len(asker.got.Turns))
	}
	if asker.got.Turns[1].Role != "assistant" {
		t.Fatalf("il ruolo 'ai' di deep-chat va tradotto in 'assistant', è %q", asker.got.Turns[1].Role)
	}
}

func TestAskHandler_SearchClosureIsScopedToTheGame(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	asker := &fakeAsker{answer: "ok"}
	server.Asker = asker
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	// Un secondo gioco con la stessa intestazione di sezione e lo stesso
	// numero di pagina, ma un testo di corpo diverso: due giochi davvero
	// identici non basterebbero a scoprire uno scambio di gameID nella
	// closure, perché produrrebbero lo stesso identico output qualunque
	// dei due venisse interrogato per errore. Con un testo distintivo,
	// invece, uno scambio (anche verso un gioco che esiste davvero, non
	// solo verso "nessun gioco") si vede.
	seedGameWithManualPage4Text(t, conn, "Ogni giocatore scarta una carta bonus.")

	postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
	if asker.got.Search == nil {
		t.Fatal("Search non agganciata")
	}
	out, err := asker.got.Search(context.Background(), []string{"Upkeep"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	// out è il JSON di manuals.MarshalHits: reference_detail porta "pagina
	// 4", non più "pag. 4" (quel formato era di manuals.FormatSearchResult,
	// cancellato dal Task 1).
	if !strings.Contains(out, "pagina 4") {
		t.Fatalf("la ricerca non trova la pagina del gioco chiesto:\n%s", out)
	}
	// Il testo del gioco richiesto deve esserci...
	if !strings.Contains(out, "moneta per ciascun edificio") {
		t.Fatalf("manca il testo del gioco richiesto: la ricerca sembra puntare a un altro gioco:\n%s", out)
	}
	// ...e quello dell'altro gioco no, né da solo né in aggiunta: il
	// modello non ha il game_id fra i parametri del tool proprio per
	// impedire che la ricerca sconfini su un altro gioco.
	if strings.Contains(out, "scarta una carta bonus") {
		t.Fatalf("la ricerca sconfina sul contenuto di un altro gioco:\n%s", out)
	}
	if strings.Count(out, "pagina 4") > 1 {
		t.Fatalf("la ricerca sconfina su un altro gioco:\n%s", out)
	}
}

func TestAskHandler_WithoutAProviderIs404(t *testing.T) {
	// Nessun provider configurato: la rotta si comporta come inesistente.
	// È la stessa degradazione di SMTP — nessun errore da spiegare, la
	// funzione semplicemente non c'è.
	server, conn := newTestServerWithDB(t)
	server.Asker = &fakeAsker{err: ai.ErrNotConfigured}
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	rec := postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("atteso 404, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAskHandler_WithoutAPreparedManualIs404(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	server.Asker = &fakeAsker{answer: "ok"}
	router := httpapi.NewRouter(server)

	// Gioco senza manuale indicizzato.
	g, _ := conn.Exec(`INSERT INTO games (name, seats) VALUES ('Senza manuale', 1)`)
	gameID, _ := g.LastInsertId()

	rec := postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("atteso 404, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAskHandler_RejectsAnEmptyQuestion(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	server.Asker = &fakeAsker{answer: "ok"}
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	for _, body := range []string{`{"messages":[]}`, `{"messages":[{"role":"user","text":"   "}]}`} {
		rec := postAsk(t, router, gameID, body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s: atteso 400, ottenuto %d", body, rec.Code)
		}
	}
}

func TestAskHandler_IsRateLimited(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	server.Asker = &fakeAsker{answer: "ok"}
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	limited := false
	// Il limite dichiarato in router.go è 20/minuto: entro trenta
	// richieste deve scattare, e ogni richiesta prima di scattare deve
	// restare un 200 (altrimenti il test potrebbe "passare" per un
	// motivo che non è il rate limit).
	for i := 0; i < 30; i++ {
		rec := postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
		if rec.Code == http.StatusTooManyRequests {
			limited = true
			break
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("richiesta %d: atteso 200 o 429 prima del limite, ottenuto %d: %s", i, rec.Code, rec.Body.String())
		}
	}
	if !limited {
		t.Fatal("l'endpoint pubblico deve avere un rate limit: una domanda costa una chiamata a pagamento")
	}
}

func TestGameDetail_ExposesCanAsk(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)

	// Senza provider e senza manuale: falso.
	g, _ := conn.Exec(`INSERT INTO games (name, seats) VALUES ('Nudo', 1)`)
	bareID, _ := g.LastInsertId()
	if canAsk(t, router, bareID) {
		t.Fatal("un gioco senza manuale e senza AI non può ricevere domande")
	}

	// Con manuale ma senza provider: ancora falso.
	preparedID := seedGameWithPreparedManual(t, conn)
	if canAsk(t, router, preparedID) {
		t.Fatal("senza provider AI configurato canAsk deve essere falso")
	}

	// Con provider e con manuale: vero.
	server.Asker = &fakeAsker{answer: "ok"}
	if !canAsk(t, router, preparedID) {
		t.Fatal("con provider e manuale preparato canAsk deve essere vero")
	}

	// Con provider ma senza manuale: falso.
	if canAsk(t, router, bareID) {
		t.Fatal("senza manuale preparato canAsk deve essere falso anche con l'AI attiva")
	}
}

// TestGameDetail_ExposesSuggestedQuestionsNotHeadings: la scheda pubblica
// manda le domande già formulate, non i titoli di sezione da cui il
// frontend le costruiva con una tabella fissa. Solo le domande NON vuote
// escono: il frontend ripiega sulle domande fisse quando la lista è vuota,
// e tre stringhe vuote non sono una lista vuota.
func TestGameDetail_ExposesSuggestedQuestionsNotHeadings(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	gameID := seedBareGame(t, server)

	if err := server.Manuals.SaveGeneratedQuestions(context.Background(), gameID,
		[]string{"Come si piazza una tessera?", "Quando finisce?", "Quanti punti?"}); err != nil {
		t.Fatalf("save generated: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/games/%d", gameID), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("risposta non JSON: %v", err)
	}
	if _, ok := resp["sourceHeadings"]; ok {
		t.Fatal("sourceHeadings non deve più esistere nella risposta")
	}
	qs, ok := resp["suggestedQuestions"].([]any)
	if !ok {
		t.Fatalf("suggestedQuestions manca o non è una lista: %s", rec.Body.String())
	}
	if len(qs) != 3 || qs[0] != "Come si piazza una tessera?" {
		t.Fatalf("domande inattese: %v", qs)
	}
}

func TestGameDetail_SuggestedQuestionsIsAlwaysAnArray(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	gameID := seedBareGame(t, server)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/games/%d", gameID), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("risposta non JSON: %v", err)
	}
	qs, ok := resp["suggestedQuestions"].([]any)
	if !ok {
		t.Fatalf("un gioco senza domande deve mandare una lista vuota, non null: %s", rec.Body.String())
	}
	if len(qs) != 0 {
		t.Fatalf("attesa lista vuota, ottenuta %v", qs)
	}
}

func TestEventDetail_ExposesCanAskPerGame(t *testing.T) {
	// Il link "Dubbi sulle regole? Chiedi al manuale" compariva su ogni
	// gioco della serata. Su uno senza manuale preparato portava a una
	// scheda dove non succedeva niente: nessuna chat, nessun messaggio. Al
	// tavolo si legge come un'app rotta, non come una funzione assente.
	server, conn := newTestServerWithDB(t)
	server.Asker = &fakeAsker{answer: "ok"}
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	conManuale := seedGameWithPreparedManual(t, conn)
	senzaManuale := createTestGameForEvent(t, server.Games, "Senza manuale")

	body := fmt.Sprintf(
		`{"title":"Serata","eventDate":"2099-01-01","startTime":"21:00",`+
			`"games":[{"gameId":%d,"copies":1},{"gameId":%d,"copies":1}]}`,
		conManuale, senzaManuale)
	rec := doLoanRequest(router, http.MethodPost, "/api/events", cookie, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("creazione evento: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	json.NewDecoder(rec.Body).Decode(&created)

	detail := getEventDetailGames(t, router, created.ID)
	if len(detail.Games) != 2 {
		t.Fatalf("attesi 2 giochi, ottenuti %d", len(detail.Games))
	}
	byGame := map[int64]bool{}
	for _, g := range detail.Games {
		byGame[g.GameID] = g.CanAsk
	}
	if !byGame[conManuale] {
		t.Fatal("il gioco col manuale preparato deve avere canAsk vero")
	}
	if byGame[senzaManuale] {
		t.Fatal("il gioco senza manuale deve avere canAsk falso: il link non ha nulla dietro")
	}
}

func TestBookingDetail_ExposesCanAsk(t *testing.T) {
	// Stessa cosa sulla pagina della prenotazione, dove il gioco è uno solo.
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := seedGameWithPreparedManual(t, conn)

	body := fmt.Sprintf(
		`{"title":"Serata","eventDate":"2099-01-01","startTime":"21:00","games":[{"gameId":%d,"copies":1}]}`,
		gameID)
	rec := doLoanRequest(router, http.MethodPost, "/api/events", cookie, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("creazione evento: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	json.NewDecoder(rec.Body).Decode(&created)
	detail := getEventDetailGames(t, router, created.ID)

	booking := fmt.Sprintf(
		`{"eventGameId":%d,"participantName":"Ada","participantEmail":"ada@example.com","termsAccepted":true}`,
		detail.Games[0].EventGameID)
	rec = doLoanRequest(router, http.MethodPost,
		fmt.Sprintf("/api/events/%d/bookings", created.ID), nil, booking)
	if rec.Code != http.StatusCreated {
		t.Fatalf("prenotazione: %d %s", rec.Code, rec.Body.String())
	}
	var b struct {
		BookingCode string `json:"bookingCode"`
	}
	json.NewDecoder(rec.Body).Decode(&b)

	lookup := `{"bookingCode":"` + b.BookingCode + `"}`
	rec = doLoanRequest(router, http.MethodPost, "/api/bookings/lookup", nil, lookup)
	if rec.Code != http.StatusOK {
		t.Fatalf("lettura prenotazione: %d %s", rec.Code, rec.Body.String())
	}
	var got struct {
		CanAsk bool `json:"canAsk"`
	}
	json.NewDecoder(rec.Body).Decode(&got)
	// Senza provider AI configurato canAsk resta falso anche col manuale:
	// sono due condizioni, come sulla scheda gioco.
	if got.CanAsk {
		t.Fatal("senza provider AI la prenotazione non deve promettere la chat")
	}

	server.Asker = &fakeAsker{answer: "ok"}
	rec = doLoanRequest(router, http.MethodPost, "/api/bookings/lookup", nil, lookup)
	json.NewDecoder(rec.Body).Decode(&got)
	if !got.CanAsk {
		t.Fatal("col manuale preparato e il provider configurato canAsk deve essere vero")
	}
}

func canAsk(t *testing.T, router http.Handler, gameID int64) bool {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/games/"+strconv.FormatInt(gameID, 10), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET game %d: %d %s", gameID, rec.Code, rec.Body.String())
	}
	var body struct {
		CanAsk bool `json:"canAsk"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("risposta non JSON: %v", err)
	}
	return body.CanAsk
}

type fakeWebSearch struct {
	results []websearch.Result
	// resultsSeq, quando presente, dà un risultato diverso per ciascuna
	// chiamata (nell'ordine): serve ai test con più chiamate al tool FAQ
	// nella stessa richiesta, dove la seconda ricerca deve trovare un
	// thread diverso dalla prima.
	resultsSeq [][]websearch.Result
	err        error
	calls      int
}

func (f *fakeWebSearch) Search(ctx context.Context, q string, d []string, max int) ([]websearch.Result, error) {
	f.calls++
	if f.resultsSeq != nil {
		if f.calls > len(f.resultsSeq) {
			return nil, f.err
		}
		return f.resultsSeq[f.calls-1], f.err
	}
	return f.results, f.err
}

// fakeFAQAsker è fakeAsker più una chiamata al tool FAQ: le citazioni FAQ
// si popolano solo dalle hit che la closure restituisce davvero.
type fakeFAQAsker struct {
	fakeAsker
	faqQuery  string
	faqResult string
	faqErr    error
}

func (f *fakeFAQAsker) Ask(ctx context.Context, req ai.AskRequest) (string, error) {
	f.got = req
	if req.SearchFAQ != nil && f.faqQuery != "" {
		f.faqResult, f.faqErr = req.SearchFAQ(ctx, f.faqQuery)
	}
	return f.answer, nil
}

func setBGGID(t *testing.T, conn *sql.DB, gameID int64, bggID string) {
	t.Helper()
	if _, err := conn.Exec(`UPDATE games SET bgg_id = ? WHERE id = ?`, bggID, gameID); err != nil {
		t.Fatalf("set bgg id: %v", err)
	}
}

func birdfeederThread() bgg.Thread {
	posted, _ := time.Parse("2006-01-02", "2019-01-07")
	return bgg.Thread{
		ID: "100", Subject: "Refreshing the birdfeeder", ObjectType: "things", ObjectID: "266192", Forum: "Rules",
		Articles: []bgg.Article{{ID: "2", Body: "You reroll only if all dice match.",
			Link: "https://boardgamegeek.com/thread/100/article/2#2", PostDate: posted}},
	}
}

func TestAskHandler_FAQToolIsOffWithoutAKeyOrABGGID(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	asker := &fakeAsker{answer: "ok"}
	server.Asker = asker
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	// Nessuna chiave Tavily e nessun WebSearch iniettato.
	setBGGID(t, conn, gameID, "266192")
	postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
	if asker.got.SearchFAQ != nil {
		t.Fatal("FAQ search must be off without a Tavily key")
	}

	// Ricerca disponibile, ma il gioco non ha un bggId.
	server.WebSearch = &fakeWebSearch{}
	other := seedGameWithManualPage4Text(t, conn, "altro")
	postAsk(t, router, other, `{"messages":[{"role":"user","text":"?"}]}`)
	if asker.got.SearchFAQ != nil {
		t.Fatal("FAQ search must be off for a game without a bggId")
	}
}

func TestAskHandler_LinksAFAQCitationToTheComment(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	server.WebSearch = &fakeWebSearch{results: []websearch.Result{{URL: "https://boardgamegeek.com/thread/100/refreshing"}}}
	server.BGG = &fakeBGGClient{threads: map[string]bgg.Thread{"100": birdfeederThread()}}
	asker := &fakeFAQAsker{faqQuery: "refresh birdfeeder"}
	asker.answer = "Sul forum: BGG: Refreshing the birdfeeder, commento del 07/01/2019."
	server.Asker = asker
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)
	setBGGID(t, conn, gameID, "266192")

	rec := postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	want := "[BGG: Refreshing the birdfeeder, commento del 07/01/2019](https://boardgamegeek.com/thread/100/article/2#2)"
	if !strings.Contains(body["text"], want) {
		t.Fatalf("expected a link to the comment, got %q", body["text"])
	}
	// Il payload al modello ha la forma di cerca_nelle_fonti, con
	// reference_type faq, e non espone gli URL.
	if !strings.Contains(asker.faqResult, `"reference_type":"faq"`) || strings.Contains(asker.faqResult, "article/2") {
		t.Fatalf("unexpected FAQ payload: %s", asker.faqResult)
	}
}

// fakeMultiFAQAsker chiama SearchFAQ una volta per ogni query in
// faqQueries, nell'ordine, e registra ciascun risultato: serve ai test che
// esercitano PIÙ chiamate al tool nella stessa richiesta (disambiguazione
// delle reference fra chiamate, tetto sul numero di ricerche).
type fakeMultiFAQAsker struct {
	fakeAsker
	faqQueries []string
	faqResults []string
}

func (f *fakeMultiFAQAsker) Ask(ctx context.Context, req ai.AskRequest) (string, error) {
	f.got = req
	for _, q := range f.faqQueries {
		out, _ := req.SearchFAQ(ctx, q)
		f.faqResults = append(f.faqResults, out)
	}
	return f.answer, nil
}

func TestAskHandler_DisambiguatesFAQReferencesAcrossToolCalls(t *testing.T) {
	// usedRefs in faq.Search vive DENTRO una chiamata a Search: due
	// cerca_nelle_faq nella stessa domanda, che trovano due thread diversi
	// con lo stesso Subject, produrrebbero altrimenti la stessa reference
	// "BGG: Question" per entrambi, e i due link collasserebbero sullo
	// stesso thread nella risposta finale.
	posted, _ := time.Parse("2006-01-02", "2019-01-07")
	threadA := bgg.Thread{
		ID: "100", Subject: "Question", ObjectType: "things", ObjectID: "266192", Forum: "Rules",
		Articles: []bgg.Article{{ID: "a", Body: "Answer A.",
			Link: "https://boardgamegeek.com/thread/100/article/1#1", PostDate: posted}},
	}
	threadB := bgg.Thread{
		ID: "200", Subject: "Question", ObjectType: "things", ObjectID: "266192", Forum: "Rules",
		Articles: []bgg.Article{{ID: "b", Body: "Answer B.",
			Link: "https://boardgamegeek.com/thread/200/article/1#1", PostDate: posted}},
	}

	server, conn := newTestServerWithDB(t)
	server.WebSearch = &fakeWebSearch{resultsSeq: [][]websearch.Result{
		{{URL: "https://boardgamegeek.com/thread/100/question"}},
		{{URL: "https://boardgamegeek.com/thread/200/question"}},
	}}
	server.BGG = &fakeBGGClient{threads: map[string]bgg.Thread{"100": threadA, "200": threadB}}
	asker := &fakeMultiFAQAsker{faqQueries: []string{"question one", "question two"}}
	asker.answer = "Vedi BGG: Question, commento del 07/01/2019, e anche BGG: Question #200, commento del 07/01/2019."
	server.Asker = asker
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)
	setBGGID(t, conn, gameID, "266192")

	rec := postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(asker.faqResults) != 2 {
		t.Fatalf("expected 2 FAQ tool results, got %d", len(asker.faqResults))
	}
	// Il secondo payload al modello deve già portare la reference
	// disambiguata, non quella "nuda" che collide con la prima.
	if !strings.Contains(asker.faqResults[1], `"reference":"BGG: Question #200"`) {
		t.Fatalf("second FAQ payload does not use the disambiguated reference:\n%s", asker.faqResults[1])
	}
	if strings.Contains(asker.faqResults[0], "#") {
		t.Fatalf("first FAQ payload should not be disambiguated: %s", asker.faqResults[0])
	}

	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	wantFirst := "[BGG: Question, commento del 07/01/2019](https://boardgamegeek.com/thread/100/article/1#1)"
	wantSecond := "[BGG: Question #200, commento del 07/01/2019](https://boardgamegeek.com/thread/200/article/1#1)"
	if !strings.Contains(body["text"], wantFirst) {
		t.Fatalf("first citation does not link to its own thread: %q", body["text"])
	}
	if !strings.Contains(body["text"], wantSecond) {
		t.Fatalf("second citation does not link to its own thread: %q", body["text"])
	}
}

func TestAskHandler_CapsFAQSearchesPerRequest(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	ws := &fakeWebSearch{results: []websearch.Result{{URL: "https://boardgamegeek.com/thread/100/refreshing"}}}
	server.WebSearch = ws
	server.BGG = &fakeBGGClient{threads: map[string]bgg.Thread{"100": birdfeederThread()}}
	asker := &fakeMultiFAQAsker{faqQueries: []string{"q1", "q2", "q3"}}
	asker.answer = "ok"
	server.Asker = asker
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)
	setBGGID(t, conn, gameID, "266192")

	postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)

	if ws.calls != 2 {
		t.Fatalf("expected at most 2 web searches, the third must not hit Tavily: got %d calls", ws.calls)
	}
	if len(asker.faqResults) != 3 {
		t.Fatalf("expected 3 FAQ tool invocations, got %d", len(asker.faqResults))
	}
	want := "Hai già cercato nel forum abbastanza per questa domanda: rispondi con quello che hai."
	if asker.faqResults[2] != want {
		t.Fatalf("unexpected third FAQ result: %q", asker.faqResults[2])
	}
}

func TestAskHandler_FAQSearchErrorBecomesAnError(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	server.WebSearch = &fakeWebSearch{err: errors.New("tavily down")}
	asker := &fakeFAQAsker{faqQuery: "x"}
	asker.answer = "ok"
	server.Asker = asker
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)
	setBGGID(t, conn, gameID, "266192")

	rec := postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("a failing FAQ search must not fail the request: %d", rec.Code)
	}
	// L'errore torna all'agente, che lo trasforma nel messaggio per il
	// modello (Task 5): l'handler non lo inghiotte.
	if asker.faqErr == nil {
		t.Fatal("expected the web search error to reach the agent")
	}
}
