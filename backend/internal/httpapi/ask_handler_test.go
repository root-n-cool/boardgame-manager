package httpapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
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
	if err := store.ReplacePages(ctx, gameID, mediaID, "it", []manuals.StoredPage{
		{PageNumber: 4, Heading: "Fase di Upkeep", Source: "vision",
			Text: "Fase di Upkeep\n" + page4Text},
		{PageNumber: 7, Heading: "Fine partita", Source: "vision",
			Text: "Fine partita\nLa partita termina quando la pila di pesca si esaurisce."},
	}); err != nil {
		t.Fatalf("replace pages: %v", err)
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

	// La AskRequest deve portare il nome del gioco, lo storico, e il
	// corpus con il suo indice.
	if asker.got.GameName != "Wingspan" {
		t.Fatalf("nome gioco: %q", asker.got.GameName)
	}
	if len(asker.got.Turns) != 1 || asker.got.Turns[0].Text != "finite le carte che si fa?" {
		t.Fatalf("storico non passato: %+v", asker.got.Turns)
	}
	if asker.got.CorpusChars == 0 {
		t.Fatal("CorpusChars a zero: l'handler non ha caricato il corpus")
	}
	if !strings.Contains(asker.got.CorpusIndex, "Fase di Upkeep p.4") {
		t.Fatalf("indice non passato: %q", asker.got.CorpusIndex)
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
	if err := manuals.NewStore(conn).ReplacePages(ctx, gameID, mediaEN, "en", []manuals.StoredPage{
		{PageNumber: 12, Heading: "Upkeep phase", Source: "vision",
			Text: "Upkeep phase\nEach player pays one coin per building."},
	}); err != nil {
		t.Fatalf("replace pages: %v", err)
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
	if !strings.Contains(out, "pag. 4") {
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
	if strings.Count(out, "pag. 4") > 1 {
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
