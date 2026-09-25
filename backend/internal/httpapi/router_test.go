package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func TestHealthEndpoint_ReturnsOK(t *testing.T) {
	router := httpapi.NewRouter(newTestServer(t))

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// Il sito si raggiunge per QR code alle serate, non da un motore di
// ricerca: ogni risposta chiede di non essere indicizzata, anche quando
// qualcuno condivide un link in giro.
func TestRouter_AsksSearchEnginesNotToIndex(t *testing.T) {
	router := httpapi.NewRouter(newTestServer(t))

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Robots-Tag"); got != "noindex, nofollow" {
		t.Fatalf("expected X-Robots-Tag %q, got %q", "noindex, nofollow", got)
	}
}

func TestRouter_LetsSearchEnginesIndexWhenTheAdminAllowsIt(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	rec := putSettingsJSON(t, router, cookie, map[string]any{
		"defaultLanguage":       "it",
		"hideFromSearchEngines": false,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("save settings: %d %s", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Robots-Tag"); got != "" {
		t.Fatalf("expected no X-Robots-Tag once indexing is allowed, got %q", got)
	}
}
