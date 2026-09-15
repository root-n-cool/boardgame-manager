package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func TestGetSite_NoAuthRequired(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/site", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetSite_DefaultsToBoardGamesManagerWithNoLogo(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/site", nil))

	var body struct {
		SiteTitle    string `json:"siteTitle"`
		LogoFilename string `json:"logoFilename"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.SiteTitle != "BoardGames Manager" {
		t.Errorf("siteTitle = %q, atteso il fallback", body.SiteTitle)
	}
	if body.LogoFilename != "" {
		t.Errorf("logoFilename = %q, atteso vuoto senza logo caricato", body.LogoFilename)
	}
}

func TestGetSite_ReturnsTheConfiguredTitleAndLogo(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)

	if _, err := conn.Exec(`UPDATE app_settings SET site_title = ?, logo_filename = ? WHERE id = 1`,
		"Ludoteca Vicolo Corto", "abc123.png"); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/site", nil))

	var body struct {
		SiteTitle    string `json:"siteTitle"`
		LogoFilename string `json:"logoFilename"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.SiteTitle != "Ludoteca Vicolo Corto" {
		t.Errorf("siteTitle = %q", body.SiteTitle)
	}
	if body.LogoFilename != "abc123.png" {
		t.Errorf("logoFilename = %q", body.LogoFilename)
	}
}
