package httpapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"boardgames-manager/internal/ai"
	"boardgames-manager/internal/httpapi"
	"boardgames-manager/internal/manuals"
)

// fakeAsker cattura la AskRequest che l'handler costruisce: è lì che si
// verifica che il corpus, l'indice e la ricerca siano stati agganciati.
type fakeAsker struct {
	got    ai.AskRequest
	answer string
	err    error
}

func (f *fakeAsker) Ask(ctx context.Context, req ai.AskRequest) (string, error) {
	f.got = req
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
	// CorpusIndex resta vuoto per adesso: l'indice per fonte è il Task 7
	// (vedi il TODO in ask_handler.go), che non è ancora stato fatto.
	if asker.got.CorpusIndex != "" {
		t.Fatalf("CorpusIndex doveva restare vuoto (Task 7 non ancora fatto), è %q", asker.got.CorpusIndex)
	}
	if asker.got.Search == nil {
		t.Fatal("Search non agganciata: con un manuale lungo il modello non avrebbe come cercare")
	}
}

func TestAskHandler_TurnsPageCitationsIntoLinksToThePDF(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	server.Asker = &fakeAsker{answer: "La partita finisce subito. Regolamento base, pag. 7."}
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	rec := postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
	var body struct {
		Text string `json:"text"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)

	// È il pezzo che rende verificabile la risposta: si apre il manuale a
	// quella pagina invece di fidarsi.
	if !strings.Contains(body.Text, "[pag. 7](/api/uploads/manuale.pdf#page=7)") {
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

func TestAskHandler_DoesNotLinkCitationsWhenThereIsMoreThanOneManual(t *testing.T) {
	// Con due manuali la riscrittura non sa a quale dei due si riferisce
	// "pag. 12": applicherebbe a entrambe le citazioni lo stesso file, e
	// "il regolamento inglese, pag. 12" diventerebbe un link a pagina 12 di
	// quello ITALIANO. Chi lo apre per verificare trova un'altra regola e
	// conclude che la risposta è inventata — peggio che non avere il link.
	server, conn := newTestServerWithDB(t)
	server.Asker = &fakeAsker{answer: "Sì: il regolamento inglese, pag. 12, lo dice."}
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)
	addSecondManual(t, conn, gameID)

	rec := postAsk(t, router, gameID, `{"messages":[{"role":"user","text":"?"}]}`)
	var body struct {
		Text string `json:"text"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)

	if strings.Contains(body.Text, "/api/uploads/") {
		t.Fatalf("con più manuali la citazione deve restare testo semplice: %q", body.Text)
	}
	if !strings.Contains(body.Text, "pag. 12") {
		t.Fatalf("la citazione deve restare leggibile: %q", body.Text)
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

func TestGameDetail_ExposesTheManualHeadings(t *testing.T) {
	// I titoli di sezione alimentano le domande suggerite della chat
	// ("Cosa dice il manuale su «Fase di Upkeep»?"). Senza questo campo il
	// pannello ripiegava per sempre sulle tre domande fisse, mentre
	// DESIGN.md descriveva un comportamento che non poteva accadere.
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	req := httptest.NewRequest(http.MethodGet, "/api/games/"+strconv.FormatInt(gameID, 10), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET game: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		SourceHeadings []string `json:"sourceHeadings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("risposta non JSON: %v", err)
	}
	if len(body.SourceHeadings) != 2 {
		t.Fatalf("attesi i 2 titoli del manuale, ottenuti %v", body.SourceHeadings)
	}
	if body.SourceHeadings[0] != "Fase di Upkeep" || body.SourceHeadings[1] != "Fine partita" {
		t.Fatalf("titoli sbagliati o fuori ordine di pagina: %v", body.SourceHeadings)
	}
}

func TestGameDetail_CapsTheManualHeadings(t *testing.T) {
	// Un manuale di quaranta pagine non deve mandare quaranta titoli al
	// telefono di chi sta al tavolo per costruirne tre domande.
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	gameID := seedGameWithPreparedManual(t, conn)

	// Si riscrive il manuale con quaranta pagine, ognuna col suo titolo.
	var mediaID int64
	if err := conn.QueryRow(`SELECT id FROM game_media LIMIT 1`).Scan(&mediaID); err != nil {
		t.Fatalf("media: %v", err)
	}
	chunks := make([]manuals.SourceChunk, 0, 40)
	for i := 1; i <= 40; i++ {
		chunks = append(chunks, manuals.SourceChunk{
			ReferenceType: "document", Reference: "Regolamento base",
			ReferenceDetail: fmt.Sprintf("pagina %d", i),
			Heading:         fmt.Sprintf("Sezione %d", i),
			LanguageCode:    "it", Seq: i - 1,
			Text: fmt.Sprintf("Testo della sezione numero %d.", i),
		})
	}
	if err := manuals.NewStore(conn).ReplaceSource(
		context.Background(), gameID, &mediaID, chunks); err != nil {
		t.Fatalf("replace: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/games/"+strconv.FormatInt(gameID, 10), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var body struct {
		SourceHeadings []string `json:"sourceHeadings"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.SourceHeadings) == 0 {
		t.Fatal("nessun titolo: la scheda non ha di che costruire le domande")
	}
	if len(body.SourceHeadings) > 8 {
		t.Fatalf("%d titoli mandati alla scheda pubblica: nessun tetto", len(body.SourceHeadings))
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
		`{"eventGameId":%d,"participantName":"Ada","participantEmail":"ada@example.com","participantPhone":"3330000000"}`,
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
