package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func legalMarkdown(t *testing.T, router http.Handler, path string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body.String())
	}
	var body struct {
		Markdown string `json:"markdown"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.Markdown
}

func TestGetLegalTerms_EmptyByDefaultNever404(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	if got := legalMarkdown(t, router, "/api/legal/terms"); got != "" {
		t.Fatalf("expected empty terms by default, got %q", got)
	}
}

func TestGetLegalPrivacy_ReturnsTheConfiguredMarkdown(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)

	if _, err := conn.Exec(`UPDATE app_settings SET privacy_markdown = ? WHERE id = 1`, "# Privacy\n\nTesto."); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if got := legalMarkdown(t, router, "/api/legal/privacy"); got != "# Privacy\n\nTesto." {
		t.Fatalf("markdown = %q", got)
	}
}
