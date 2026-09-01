package webui

import (
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// TestHandlerFor_ErrorsWhenIndexHTMLMissing guards against the frontend
// silently not being built. //go:embed dist/* matches the tracked
// dist/.gitkeep placeholder on its own, so the package compiles fine with no
// real build output; before this check the SPA fallback then asked
// http.FileServer to serve "/" from a directory with no index file, which
// answers with a 200 and a browsable file listing rather than any error.
func TestHandlerFor_ErrorsWhenIndexHTMLMissing(t *testing.T) {
	// Mirrors a fresh clone's dist/: the placeholder and nothing else.
	gitkeepOnly := fstest.MapFS{
		".gitkeep": {Data: []byte{}},
	}

	handler, err := handlerFor(gitkeepOnly)
	if err == nil {
		t.Fatal("expected an error when dist/ has no index.html, got nil")
	}
	if handler != nil {
		t.Fatalf("expected a nil handler alongside the error, got %#v", handler)
	}
	if !strings.Contains(err.Error(), "index.html") {
		t.Errorf("error should name the missing file, got: %v", err)
	}
	if !strings.Contains(err.Error(), "npm run build") {
		t.Errorf("error should tell the operator how to fix it, got: %v", err)
	}
}

func TestHandlerFor_ServesIndexHTMLForClientRoutes(t *testing.T) {
	built := fstest.MapFS{
		"index.html":       {Data: []byte("<div id=\"app\"></div>")},
		"assets/index.css": {Data: []byte(".layout{}")},
	}

	handler, err := handlerFor(built)
	if err != nil {
		t.Fatalf("handlerFor: %v", err)
	}

	t.Run("unknown path falls back to index.html", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/users", nil))

		if rec.Code != 200 {
			t.Fatalf("got status %d, want 200", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `id="app"`) {
			t.Fatalf("expected index.html body, got %q", rec.Body.String())
		}
	})

	t.Run("real asset is served from the filesystem", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/assets/index.css", nil))

		if rec.Code != 200 {
			t.Fatalf("got status %d, want 200", rec.Code)
		}
		if got := rec.Body.String(); got != ".layout{}" {
			t.Fatalf("expected the asset body, got %q", got)
		}
	})
}
