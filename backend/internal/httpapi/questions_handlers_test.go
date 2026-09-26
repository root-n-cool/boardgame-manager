package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boardgames-manager/internal/ai"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/httpapi"
	"boardgames-manager/internal/manuals"
)

func questionsPath(gameID int64) string {
	return fmt.Sprintf("/api/games/%d/suggested-questions", gameID)
}

// seedBareGame crea un gioco senza lingue né media: basta alle rotte delle
// domande, che non guardano le fonti (tranne regenerate).
func seedBareGame(t *testing.T, server *httpapi.Server) int64 {
	t.Helper()
	game, err := server.Games.CreateGame(context.Background(), games.Game{Name: "Carcassonne"})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	return game.ID
}

func TestGetSuggestedQuestions_RequiresAuth(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	gameID := seedBareGame(t, server)

	req := httptest.NewRequest(http.MethodGet, questionsPath(gameID), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("atteso 401, ottenuto %d", rec.Code)
	}
}

func TestGetSuggestedQuestions_EmptyGameReturnsThreeBlanks(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID := seedBareGame(t, server)

	req := httptest.NewRequest(http.MethodGet, questionsPath(gameID), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Questions []struct {
			Text   string `json:"text"`
			Edited bool   `json:"edited"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("risposta non JSON: %v (%s)", err, rec.Body.String())
	}
	// Sempre tre voci: il pannello admin ha tre campi da riempire, e uno
	// slot vuoto è una voce con testo vuoto, non una voce assente.
	if len(resp.Questions) != 3 {
		t.Fatalf("attese 3 voci anche su un gioco vuoto, ottenute %d", len(resp.Questions))
	}
	for i, q := range resp.Questions {
		if q.Text != "" {
			t.Fatalf("voce %d: atteso testo vuoto, ottenuto %q", i, q.Text)
		}
	}
}

func TestPutSuggestedQuestions_SavesAndMarksEdited(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID := seedBareGame(t, server)

	body := `{"questions":["Come si piazza una tessera?","Quando finisce?","Quanti punti vale un castello?"]}`
	req := httptest.NewRequest(http.MethodPut, questionsPath(gameID), bytes.NewBufferString(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}

	got, err := server.Manuals.SuggestedQuestions(context.Background(), gameID, manuals.AgentRules)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	if len(got) != 3 || got[0].Text != "Come si piazza una tessera?" {
		t.Fatalf("domande non salvate: %v", got)
	}
	if !got[0].Edited {
		t.Fatal("un testo scritto dall'admin nasce edited")
	}
}

func TestPutSuggestedQuestions_RejectsTheWrongCount(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID := seedBareGame(t, server)

	body := `{"questions":["Solo una?"]}`
	req := httptest.NewRequest(http.MethodPut, questionsPath(gameID), bytes.NewBufferString(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("atteso 400 per un conteggio sbagliato, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPutSuggestedQuestions_RejectsAnEmptyText(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID := seedBareGame(t, server)

	body := `{"questions":["Buona?","   ","Anche buona?"]}`
	req := httptest.NewRequest(http.MethodPut, questionsPath(gameID), bytes.NewBufferString(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("una domanda vuota finirebbe in un bottone vuoto: atteso 400, ottenuto %d", rec.Code)
	}
}

func TestRegenerateSuggestedQuestions_WithoutAnIndexSaysToIndexFirst(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Suggester = &fakeSuggester{}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID := seedBareGame(t, server)

	req := httptest.NewRequest(http.MethodPost, questionsPath(gameID)+"/regenerate", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("atteso 422, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("indicizza")) {
		t.Fatalf("il messaggio deve dire di indicizzare prima un manuale: %s", rec.Body.String())
	}
}

// TestRegenerateSuggestedQuestions_OverwritesEditedToo: è un pulsante
// premuto a mano, quindi sovrascrive tutto — la decisione di prodotto
// presa in fase di design.
func TestRegenerateSuggestedQuestions_OverwritesEditedToo(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	sug := &fakeSuggester{out: []string{"Nuova 1?", "Nuova 2?", "Nuova 3?"}}
	server.Suggester = sug
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server,
		[]byte("## Preparazione\n\nMescola il mazzo di carte e dai tre carte a ciascun giocatore."),
		"regole.md", "")

	// Indicizza (così ci sono titoli) e poi riscrivi tutte tre a mano.
	if rec := postIndex(cookie, router, gameID, mediaID); rec.Code != http.StatusOK {
		t.Fatalf("index: atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if err := server.Manuals.SaveEditedQuestions(context.Background(), gameID, manuals.AgentRules,
		[]string{"Mia 1?", "Mia 2?", "Mia 3?"}); err != nil {
		t.Fatalf("save edited: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, questionsPath(gameID)+"/regenerate", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("atteso 200, ottenuto %d: %s", rec.Code, rec.Body.String())
	}

	got, err := server.Manuals.SuggestedQuestions(context.Background(), gameID, manuals.AgentRules)
	if err != nil {
		t.Fatalf("suggested questions: %v", err)
	}
	if got[0].Text != "Nuova 1?" {
		t.Fatalf("rigenera deve sovrascrivere anche le domande a mano: %v", got)
	}
	if got[0].Edited {
		t.Fatal("rigenera azzera edited")
	}
}

func TestRegenerateSuggestedQuestions_ProviderRejectionIsAClearError(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	server.Segmenter = &fakeSegmenter{}
	server.Suggester = &fakeSuggester{err: ai.ErrSuggestionsRejected}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID, mediaID := seedGameWithFile(t, server,
		[]byte("## Preparazione\n\nMescola il mazzo di carte e dai tre carte a ciascun giocatore."),
		"regole.md", "")
	if rec := postIndex(cookie, router, gameID, mediaID); rec.Code != http.StatusOK {
		t.Fatalf("index: atteso 200, ottenuto %d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodPost, questionsPath(gameID)+"/regenerate", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("atteso 422, ottenuto %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("riprova")) {
		t.Fatalf("il messaggio deve invitare a riprovare: %s", rec.Body.String())
	}
}

// TestRegenerateQuestions_StrategyUsesTheBGGDescription: l'agente Strategia
// non guarda il manuale indicizzato (non ce l'ha) ma nome e descrizione BGG
// del gioco, salvati alla creazione.
func TestRegenerateQuestions_StrategyUsesTheBGGDescription(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	sug := &fakeSuggester{}
	server.Suggester = sug
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	g, _ := conn.Exec(`INSERT INTO games (name, seats, bgg_id, bgg_description) VALUES ('Wingspan', 1, '266192', 'Attract birds.')`)
	gameID, _ := g.LastInsertId()

	rec := doLoanRequest(router, http.MethodPost,
		fmt.Sprintf("/api/games/%d/suggested-questions/regenerate?agent=strategy", gameID), cookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", rec.Code, rec.Body.String())
	}
	if sug.strategyCalls.Load() != 1 || sug.lastDescription != "Attract birds." {
		t.Fatalf("expected one strategy call with the BGG description, got %d %q", sug.strategyCalls.Load(), sug.lastDescription)
	}
	if !strings.Contains(rec.Body.String(), "Strategia 1?") {
		t.Fatalf("response must carry the strategy questions:\n%s", rec.Body.String())
	}
	// Le domande del Manuale restano intatte (nessuna riga).
	rec = doLoanRequest(router, http.MethodGet, fmt.Sprintf("/api/games/%d/suggested-questions", gameID), cookie, "")
	if strings.Contains(rec.Body.String(), "Strategia") {
		t.Fatalf("rules questions must not contain strategy ones:\n%s", rec.Body.String())
	}
}

// TestRegenerateQuestions_StrategyWithoutBGGIDIsExplained: un gioco inserito
// a mano non ha un BGGID né una descrizione da cui partire.
func TestRegenerateQuestions_StrategyWithoutBGGIDIsExplained(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	server.Suggester = &fakeSuggester{}
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	g, _ := conn.Exec(`INSERT INTO games (name, seats) VALUES ('Fatto in casa', 1)`)
	gameID, _ := g.LastInsertId()

	rec := doLoanRequest(router, http.MethodPost,
		fmt.Sprintf("/api/games/%d/suggested-questions/regenerate?agent=strategy", gameID), cookie, "")
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "BoardGameGeek") {
		t.Fatalf("expected 422 mentioning BoardGameGeek, got %d %s", rec.Code, rec.Body.String())
	}
}

// TestSuggestedQuestions_RejectsAnUnknownAgent: ?agent= arriva dalla query
// string di una rotta admin, quindi un valore diverso dai due è un errore
// del client, non un 500 o un fallback silenzioso.
func TestSuggestedQuestions_RejectsAnUnknownAgent(t *testing.T) {
	server, _ := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID := seedBareGame(t, server)

	rec := doLoanRequest(router, http.MethodGet, fmt.Sprintf("/api/games/%d/suggested-questions?agent=boh", gameID), cookie, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// TestPutSuggestedQuestions_StrategyAgent: PUT con ?agent=strategy scrive
// nelle righe dell'agente Strategia, non in quelle del Manuale.
func TestPutSuggestedQuestions_StrategyAgent(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := loginAsAdmin(t, router)
	gameID := seedBareGame(t, server)

	rec := doLoanRequest(router, http.MethodPut,
		fmt.Sprintf("/api/games/%d/suggested-questions?agent=strategy", gameID), cookie,
		`{"questions":["A?","B?","C?"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", rec.Code, rec.Body.String())
	}
	qs, _ := manuals.NewStore(conn).SuggestedQuestions(context.Background(), gameID, manuals.AgentStrategy)
	if len(qs) != 3 || qs[0].Text != "A?" {
		t.Fatalf("strategy questions not saved: %+v", qs)
	}
}
