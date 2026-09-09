package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/bgg"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/httpapi"
)

type bggFilesResponse struct {
	Items []struct {
		Title        string `json:"title"`
		Filename     string `json:"filename"`
		Language     string `json:"language"`
		Positive     int    `json:"positive"`
		SizeBytes    int64  `json:"sizeBytes"`
		PageURL      string `json:"pageUrl"`
		LanguageCode string `json:"languageCode"`
	} `json:"items"`
	LanguageFiltered bool `json:"languageFiltered"`
}

func createTestGameWithBGGID(t *testing.T, server *httpapi.Server, bggID string) int64 {
	t.Helper()
	g, err := server.Games.CreateGame(context.Background(), games.Game{Name: "Carcassonne", BGGID: &bggID})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	return g.ID
}

func getBggFiles(t *testing.T, router http.Handler, cookie *http.Cookie, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestListBggFiles_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	id := createTestGameWithBGGID(t, server, "822")

	rec := getBggFiles(t, router, nil, fmt.Sprintf("/api/games/%d/bgg-files?lang=it", id))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestListBggFiles_FiltersByActiveLanguage(t *testing.T) {
	fake := &fakeBGGClient{files: []bgg.FileEntry{{
		Title: "Carcopedia", Filename: "Carcopedia.pdf", Language: "Italian", LanguageID: "2193",
		Positive: 13, SizeBytes: 18083546, PageURL: "https://boardgamegeek.com/filepage/143911/carcopedia",
	}}}
	server := newTestServer(t)
	server.BGG = fake
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGameWithBGGID(t, server, "822")

	rec := getBggFiles(t, router, cookie, fmt.Sprintf("/api/games/%d/bgg-files?lang=it", id))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.filesBGGID != "822" {
		t.Errorf("expected the game's BGG id, got %q", fake.filesBGGID)
	}
	if fake.filesLanguageID != "2193" {
		t.Errorf("expected Italian language id 2193, got %q", fake.filesLanguageID)
	}
	var body bggFilesResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.LanguageFiltered {
		t.Error("expected languageFiltered true")
	}
	if len(body.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(body.Items))
	}
	item := body.Items[0]
	if item.Title != "Carcopedia" || item.Filename != "Carcopedia.pdf" || item.Language != "Italian" {
		t.Errorf("unexpected item: %+v", item)
	}
	if item.Positive != 13 || item.SizeBytes != 18083546 || item.PageURL == "" {
		t.Errorf("unexpected item metadata: %+v", item)
	}
	// Il codice lingua serve alla UI per scrivere "italiano" invece del nome
	// inglese con cui BGG etichetta i suoi file.
	if item.LanguageCode != "it" {
		t.Errorf("expected languageCode it for BGG id 2193, got %q", item.LanguageCode)
	}
}

// Un file "(neutral)", o in una lingua che non abbiamo in mappa, non ha un
// codice da dare alla UI: resta il nome che manda BGG.
func TestListBggFiles_UnmappedFileLanguageHasNoCode(t *testing.T) {
	fake := &fakeBGGClient{files: []bgg.FileEntry{
		{Title: "Tiles", Filename: "tiles.zip", Language: "", LanguageID: ""},
		{Title: "Regeln", Filename: "regeln.pdf", Language: "Sorbian", LanguageID: "9999"},
	}}
	server := newTestServer(t)
	server.BGG = fake
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGameWithBGGID(t, server, "822")

	rec := getBggFiles(t, router, cookie, fmt.Sprintf("/api/games/%d/bgg-files?lang=it&all=1", id))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body bggFilesResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(body.Items))
	}
	for _, item := range body.Items {
		if item.LanguageCode != "" {
			t.Errorf("expected no language code for %q, got %q", item.Title, item.LanguageCode)
		}
	}
}

func TestListBggFiles_AllDropsTheLanguageFilter(t *testing.T) {
	fake := &fakeBGGClient{}
	server := newTestServer(t)
	server.BGG = fake
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGameWithBGGID(t, server, "822")

	rec := getBggFiles(t, router, cookie, fmt.Sprintf("/api/games/%d/bgg-files?lang=it&all=1", id))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.filesLanguageID != "" {
		t.Errorf("expected no language filter, got %q", fake.filesLanguageID)
	}
	var body bggFilesResponse
	json.NewDecoder(rec.Body).Decode(&body)
	if body.LanguageFiltered {
		t.Error("expected languageFiltered false")
	}
}

// Le lingue di un gioco non sono limitate a quelle che BGG conosce: per un
// codice che non sappiamo tradurre mostriamo tutti i file, dicendo alla UI
// che il filtro non è stato applicato invece di restituire un errore.
func TestListBggFiles_UnknownLanguageCodeFallsBackToAllFiles(t *testing.T) {
	fake := &fakeBGGClient{}
	server := newTestServer(t)
	server.BGG = fake
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGameWithBGGID(t, server, "822")

	rec := getBggFiles(t, router, cookie, fmt.Sprintf("/api/games/%d/bgg-files?lang=zz", id))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.filesLanguageID != "" {
		t.Errorf("expected no language filter, got %q", fake.filesLanguageID)
	}
	var body bggFilesResponse
	json.NewDecoder(rec.Body).Decode(&body)
	if body.LanguageFiltered {
		t.Error("expected languageFiltered false")
	}
}

func TestListBggFiles_GameWithoutBGGIDReturnsConflict(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	rec := getBggFiles(t, router, cookie, fmt.Sprintf("/api/games/%d/bgg-files?lang=it", id))

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListBggFiles_UnknownGameReturnsNotFound(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	rec := getBggFiles(t, router, cookie, "/api/games/999/bgg-files?lang=it")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestListBggFiles_UpstreamFailureReturnsBadGateway(t *testing.T) {
	server := newTestServer(t)
	server.BGG = &fakeBGGClient{filesErr: errors.New("boom")}
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGameWithBGGID(t, server, "822")

	rec := getBggFiles(t, router, cookie, fmt.Sprintf("/api/games/%d/bgg-files?lang=it", id))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}
