package httpapi_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"boardgames-manager/internal/bgg"
	"boardgames-manager/internal/httpapi"
)

func TestCreateGame_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	payload, _ := json.Marshal(map[string]string{"languageCode": "it", "name": "Test Game"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCreateGame_ManualCreationSucceeds(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]any{
		"languageCode":   "it",
		"name":           "Azul",
		"owner":          "Mario",
		"nameTranslated": "Azul",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Name      string `json:"name"`
		Languages []struct {
			Code string `json:"code"`
			Name string `json:"name"`
		} `json:"languages"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Name != "Azul" {
		t.Fatalf("expected name Azul, got %q", body.Name)
	}
	if len(body.Languages) != 1 || body.Languages[0].Code != "it" {
		t.Fatalf("expected one 'it' language, got %+v", body.Languages)
	}
}

func TestCreateGame_ManualCreationRequiresName(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"languageCode": "it"})
	req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreateGame_FromBGGRequiresToken(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"bggId": "13", "languageCode": "it"})
	req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 without a configured BGG token, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateGame_FromBGGSucceedsWithFakeClient(t *testing.T) {
	server := newTestServer(t)
	server.BGG = &fakeBGGClient{thing: bgg.ThingDetail{
		ID: "13", Name: "Catan", Description: "A settling game.",
		Year: 1995, MinPlayers: 3, MaxPlayers: 4, PlayingTime: 90,
	}}
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	settingsPayload, _ := json.Marshal(map[string]string{"defaultLanguage": "it", "bggApiToken": "fake-token"})
	settingsReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(settingsPayload))
	settingsReq.AddCookie(cookie)
	router.ServeHTTP(httptest.NewRecorder(), settingsReq)

	payload, _ := json.Marshal(map[string]string{"bggId": "13", "languageCode": "it", "owner": "Mario"})
	req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Name      string `json:"name"`
		Languages []struct {
			Name        string  `json:"name"`
			Description *string `json:"description"`
		} `json:"languages"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Name != "Catan" {
		t.Fatalf("expected name Catan, got %q", body.Name)
	}
	if len(body.Languages) != 1 || body.Languages[0].Name != "Catan" {
		t.Fatalf("expected base language prefilled with BGG name, got %+v", body.Languages)
	}
	if body.Languages[0].Description == nil || *body.Languages[0].Description != "A settling game." {
		t.Fatal("expected base language prefilled with BGG description")
	}
}

func TestSearchGames_RequiresToken(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	req := httptest.NewRequest(http.MethodGet, "/api/games/search?q=catan", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestSearchGames_ReturnsFakeResults(t *testing.T) {
	server := newTestServer(t)
	server.BGG = &fakeBGGClient{searchResults: []bgg.SearchResult{{ID: "13", Name: "Catan", Year: 1995}}}
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	settingsPayload, _ := json.Marshal(map[string]string{"defaultLanguage": "it", "bggApiToken": "fake-token"})
	settingsReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(settingsPayload))
	settingsReq.AddCookie(cookie)
	router.ServeHTTP(httptest.NewRecorder(), settingsReq)

	req := httptest.NewRequest(http.MethodGet, "/api/games/search?q=catan", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var results []struct {
		BGGID string `json:"bggId"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&results); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(results) != 1 || results[0].BGGID != "13" || results[0].Name != "Catan" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

// searchWithBGG boots a server whose BGG client is the given fake and whose
// settings carry a token, then runs one search and returns the recorder.
func searchWithBGG(t *testing.T, fake *fakeBGGClient, query string) *httptest.ResponseRecorder {
	t.Helper()
	server := newTestServer(t)
	server.BGG = fake
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	settingsPayload, _ := json.Marshal(map[string]string{"defaultLanguage": "it", "bggApiToken": "fake-token"})
	settingsReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(settingsPayload))
	settingsReq.AddCookie(cookie)
	router.ServeHTTP(httptest.NewRecorder(), settingsReq)

	req := httptest.NewRequest(http.MethodGet, "/api/games/search?q="+url.QueryEscape(query), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

type searchRow struct {
	BGGID        string   `json:"bggId"`
	Name         string   `json:"name"`
	Year         int      `json:"year"`
	ThumbnailURL *string  `json:"thumbnailUrl"`
	Weight       *float64 `json:"weight"`
}

func decodeSearchRows(t *testing.T, rec *httptest.ResponseRecorder) []searchRow {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var rows []searchRow
	if err := json.NewDecoder(rec.Body).Decode(&rows); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return rows
}

func TestSearchGames_RejectsShortQueries(t *testing.T) {
	rec := searchWithBGG(t, &fakeBGGClient{}, "ca")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a 2-character query, got %d", rec.Code)
	}
}

func TestSearchGames_AddsThumbnailAndWeight(t *testing.T) {
	fake := &fakeBGGClient{
		searchResults: []bgg.SearchResult{{ID: "13", Name: "Catan", Year: 1995}},
		details: map[string]bgg.ThingDetail{
			"13": {ID: "13", ThumbnailURL: "https://example.com/catan_t.png", Weight: 2.2809},
		},
	}
	rows := decodeSearchRows(t, searchWithBGG(t, fake, "catan"))

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].ThumbnailURL == nil || *rows[0].ThumbnailURL != "https://example.com/catan_t.png" {
		t.Fatalf("unexpected thumbnail: %v", rows[0].ThumbnailURL)
	}
	if rows[0].Weight == nil || *rows[0].Weight != 2.2809 {
		t.Fatalf("unexpected weight: %v", rows[0].Weight)
	}
	if fake.detailsCalls != 1 {
		t.Fatalf("expected the thumbnails to cost exactly one upstream call, got %d", fake.detailsCalls)
	}
}

func TestSearchGames_UnratedGameHasNoWeight(t *testing.T) {
	fake := &fakeBGGClient{
		searchResults: []bgg.SearchResult{{ID: "13", Name: "Catan", Year: 1995}},
		details:       map[string]bgg.ThingDetail{"13": {ID: "13", Weight: 0}},
	}
	rows := decodeSearchRows(t, searchWithBGG(t, fake, "catan"))
	if rows[0].Weight != nil {
		t.Fatalf("an unrated game has an unknown weight, got %v", *rows[0].Weight)
	}
}

func TestSearchGames_SurvivesDetailsFailure(t *testing.T) {
	fake := &fakeBGGClient{
		searchResults: []bgg.SearchResult{{ID: "13", Name: "Catan", Year: 1995}},
		detailsErr:    errors.New("bgg is having a bad day"),
	}
	rows := decodeSearchRows(t, searchWithBGG(t, fake, "catan"))
	if len(rows) != 1 || rows[0].Name != "Catan" {
		t.Fatalf("results should survive a missing thumbnail, got %+v", rows)
	}
	if rows[0].ThumbnailURL != nil {
		t.Fatalf("expected no thumbnail, got %v", *rows[0].ThumbnailURL)
	}
}

func TestSearchGames_RanksExactMatchFirstAndCapsResults(t *testing.T) {
	// BGG answers "catan" with 142 items in alphabetical order, so the game
	// everyone means is nowhere near the top. Rank before capping, otherwise
	// the cap throws away the only row that matters.
	results := []bgg.SearchResult{
		{ID: "1", Name: "Baden-Württemberg Catan", Year: 2012},
		{ID: "2", Name: "Catan: Cities & Knights", Year: 1998},
		{ID: "3", Name: "Catan", Year: 1995},
		{ID: "4", Name: "Catan Dice Game", Year: 2007},
	}
	for i := 0; i < 30; i++ {
		results = append(results, bgg.SearchResult{ID: fmt.Sprintf("f%d", i), Name: fmt.Sprintf("A fan expansion for Catan %d", i)})
	}
	fake := &fakeBGGClient{searchResults: results}
	rows := decodeSearchRows(t, searchWithBGG(t, fake, "catan"))

	if len(rows) != 12 {
		t.Fatalf("expected the list capped at 12, got %d", len(rows))
	}
	if rows[0].Name != "Catan" {
		t.Fatalf("expected the exact match first, got %q", rows[0].Name)
	}
	if rows[1].Name != "Catan Dice Game" {
		t.Fatalf("expected prefix matches next, shortest first, got %q", rows[1].Name)
	}
	if len(fake.detailsIDs) != 12 {
		t.Fatalf("thumbnails should be fetched only for the rows shown, got %d ids", len(fake.detailsIDs))
	}
}

func TestCreateGame_FromBGGStoresWeight(t *testing.T) {
	server := newTestServer(t)
	server.BGG = &fakeBGGClient{thing: bgg.ThingDetail{ID: "13", Name: "Catan", Weight: 2.2809}}
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	settingsPayload, _ := json.Marshal(map[string]string{"defaultLanguage": "it", "bggApiToken": "fake-token"})
	settingsReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(settingsPayload))
	settingsReq.AddCookie(cookie)
	router.ServeHTTP(httptest.NewRecorder(), settingsReq)

	payload, _ := json.Marshal(map[string]any{"bggId": "13", "languageCode": "it"})
	req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Weight *float64 `json:"weight"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Weight == nil || *body.Weight != 2.2809 {
		t.Fatalf("expected the BGG weight to be stored, got %v", body.Weight)
	}
}

func TestCreateGame_ManualWeightIsOptional(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	for _, tc := range []struct {
		name    string
		payload map[string]any
		want    *float64
	}{
		{"con peso", map[string]any{"name": "Azul", "languageCode": "it", "weight": 1.8}, ptrTo(1.8)},
		{"senza peso", map[string]any{"name": "Azul", "languageCode": "it"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, _ := json.Marshal(tc.payload)
			req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
			req.AddCookie(cookie)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusCreated {
				t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
			}
			var body struct {
				Weight *float64 `json:"weight"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if (body.Weight == nil) != (tc.want == nil) || (tc.want != nil && *body.Weight != *tc.want) {
				t.Fatalf("expected weight %v, got %v", tc.want, body.Weight)
			}
		})
	}
}

func TestCreateGame_StoresBookableSeats(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]any{
		"languageCode": "it", "name": "D&D", "nameTranslated": "D&D", "seats": 5,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		ID    int64 `json:"id"`
		Seats int   `json:"seats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Seats != 5 {
		t.Fatalf("expected seats 5, got %d", body.Seats)
	}
}

func TestCreateGame_DefaultsBookableSeatsToOne(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]any{
		"languageCode": "it", "name": "Catan", "nameTranslated": "Catan",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Seats int `json:"seats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Seats != 1 {
		t.Fatalf("expected seats 1, got %d", body.Seats)
	}
}

func TestCreateGame_RejectsZeroSeats(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]any{
		"languageCode": "it", "name": "D&D", "nameTranslated": "D&D", "seats": 0,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateGame_ChangesBookableSeats(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "D&D")

	patch := func(seats int) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(map[string]any{"seats": seats})
		req := httptest.NewRequest(http.MethodPatch,
			fmt.Sprintf("/api/games/%d", gameID), bytes.NewReader(payload))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	rec := patch(6)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Seats int `json:"seats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Seats != 6 {
		t.Fatalf("expected seats 6, got %d", body.Seats)
	}

	if rec := patch(0); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for zero seats, got %d", rec.Code)
	}
}

func ptrTo[T any](v T) *T { return &v }
