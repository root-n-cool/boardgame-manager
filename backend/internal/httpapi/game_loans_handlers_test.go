package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/httpapi"
)

// incompleteFixture monta un gioco con una voce di materiali, una serata,
// una copia consegnata e restituita con una mancanza: il minimo perché il
// gioco risulti incompleto.
func incompleteFixture(t *testing.T) (http.Handler, *http.Cookie, int64) {
	t.Helper()
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Carcassonne")

	saved, err := server.Games.ReplaceMaterials(context.Background(), gameID,
		[]games.MaterialInput{{Name: "carte", Quantity: 40}})
	if err != nil {
		t.Fatalf("materials: %v", err)
	}
	event, err := server.Events.CreateEvent(context.Background(), events.EventInput{
		Title: "Serata", EventDate: "2099-01-01", StartTime: "21:00",
		Games: []events.EventGameInput{{GameID: gameID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("event: %v", err)
	}
	copies, err := server.Events.ListEventGames(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("copies: %v", err)
	}
	loan := lend(t, router, cookie, event.ID, copies[0].ID, "Anna", "3331234567", "")
	body := fmt.Sprintf(`{"materials":[{"materialId":%d,"complete":false,"returned":35}]}`, saved[0].ID)
	if rec := doLoanRequest(router, http.MethodPost,
		fmt.Sprintf("/api/loans/%d/return", loan.ID), cookie, body); rec.Code != http.StatusOK {
		t.Fatalf("return: %d %s", rec.Code, rec.Body.String())
	}
	return router, cookie, gameID
}

func TestGamesListMarksIncompleteOnlyForAdmins(t *testing.T) {
	router, cookie, gameID := incompleteFixture(t)

	// Con sessione: il marchio c'è ed è vero.
	rec := doLoanRequest(router, http.MethodGet, "/api/games", cookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/games: %d %s", rec.Code, rec.Body.String())
	}
	var withSession []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &withSession); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, g := range withSession {
		if int64(g["id"].(float64)) == gameID {
			found = true
			if g["incomplete"] != true {
				t.Errorf("incomplete = %v, volevo true", g["incomplete"])
			}
		}
	}
	if !found {
		t.Fatal("il gioco non è nell'elenco")
	}

	// Senza sessione: il campo non deve esistere affatto. Il catalogo
	// pubblico non racconta a nessuno che a una scatola mancano pezzi.
	rec = doLoanRequest(router, http.MethodGet, "/api/games", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET pubblica: %d", rec.Code)
	}
	var anonymous []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &anonymous); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, g := range anonymous {
		if _, ok := g["incomplete"]; ok {
			t.Fatalf("la risposta pubblica non deve portare incomplete: %+v", g)
		}
	}
}

func TestGameDetailCarriesMissingPiecesForAdmins(t *testing.T) {
	router, cookie, gameID := incompleteFixture(t)
	path := fmt.Sprintf("/api/games/%d", gameID)

	rec := doLoanRequest(router, http.MethodGet, path, cookie, "")
	var detail map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	pieces, ok := detail["missingPieces"].([]any)
	if !ok || len(pieces) != 1 {
		t.Fatalf("missingPieces inatteso: %+v", detail["missingPieces"])
	}
	first := pieces[0].(map[string]any)
	if first["name"] != "carte" || first["expected"].(float64) != 40 || first["returned"].(float64) != 35 {
		t.Errorf("voce inattesa: %+v", first)
	}

	rec = doLoanRequest(router, http.MethodGet, path, nil, "")
	var public map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &public); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := public["missingPieces"]; ok {
		t.Fatalf("la scheda pubblica non deve portare missingPieces: %+v", public)
	}
}

func TestResolveClearsTheMark(t *testing.T) {
	router, cookie, gameID := incompleteFixture(t)
	rec := doLoanRequest(router, http.MethodPost,
		fmt.Sprintf("/api/games/%d/materials/resolve", gameID), cookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("resolve: %d %s", rec.Code, rec.Body.String())
	}

	rec = doLoanRequest(router, http.MethodGet, fmt.Sprintf("/api/games/%d", gameID), cookie, "")
	var detail map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if pieces, _ := detail["missingPieces"].([]any); len(pieces) != 0 {
		t.Fatalf("dopo la risoluzione non deve mancare niente: %+v", pieces)
	}
}

func TestResolveNeedsASessionAndAnExistingGame(t *testing.T) {
	router, cookie, gameID := incompleteFixture(t)

	if rec := doLoanRequest(router, http.MethodPost,
		fmt.Sprintf("/api/games/%d/materials/resolve", gameID), nil, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("senza cookie: volevo 401, ho %d", rec.Code)
	}
	if rec := doLoanRequest(router, http.MethodPost,
		"/api/games/9999/materials/resolve", cookie, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("gioco inesistente: volevo 404, ho %d", rec.Code)
	}
}

func TestGameLoanLogShowsWhatWasMissing(t *testing.T) {
	router, cookie, gameID := incompleteFixture(t)
	path := fmt.Sprintf("/api/games/%d/loans", gameID)

	if rec := doLoanRequest(router, http.MethodGet, path, nil, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("senza cookie: volevo 401, ho %d", rec.Code)
	}

	rec := doLoanRequest(router, http.MethodGet, path, cookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("log: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Loans []struct {
			EventTitle     string `json:"eventTitle"`
			EventDate      string `json:"eventDate"`
			BorrowerName   string `json:"borrowerName"`
			ReturnedAt     string `json:"returnedAt"`
			MaterialIssues []struct {
				Name     string `json:"name"`
				Expected int    `json:"expected"`
				Returned *int   `json:"returned"`
			} `json:"materialIssues"`
		} `json:"loans"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Loans) != 1 {
		t.Fatalf("volevo un prestito, ho %d", len(body.Loans))
	}
	row := body.Loans[0]
	if row.EventTitle != "Serata" || row.BorrowerName != "Anna" {
		t.Errorf("riga inattesa: %+v", row)
	}
	if len(row.MaterialIssues) != 1 || row.MaterialIssues[0].Name != "carte" ||
		row.MaterialIssues[0].Returned == nil || *row.MaterialIssues[0].Returned != 35 {
		t.Errorf("il log deve dire cosa mancava: %+v", row.MaterialIssues)
	}
}

func TestGameLoanLogOnAMissingGameIs404(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	rec := doLoanRequest(router, http.MethodGet, "/api/games/9999/loans", cookie, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("volevo 404, ho %d: %s", rec.Code, rec.Body.String())
	}
}
