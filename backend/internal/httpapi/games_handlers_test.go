package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
