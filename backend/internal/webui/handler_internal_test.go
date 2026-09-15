package webui

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"boardgames-manager/internal/settings"
)

var errTestSettingsUnavailable = errors.New("settings store unavailable")

type fakeSettingsReader struct {
	cfg settings.Settings
	err error
}

func (f fakeSettingsReader) Get(ctx context.Context) (settings.Settings, error) {
	return f.cfg, f.err
}

func TestHandlerFor_ErrorsWhenIndexHTMLMissing(t *testing.T) {
	gitkeepOnly := fstest.MapFS{
		".gitkeep": {Data: []byte{}},
	}

	handler, err := handlerFor(gitkeepOnly, fakeSettingsReader{})
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

func indexFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": {Data: []byte(
			`<title>__SITE_TITLE__</title><link rel="icon" href="__FAVICON_URL__" /><div id="app"></div>`,
		)},
		"assets/index.css": {Data: []byte(".layout{}")},
	}
}

func TestHandlerFor_ServesIndexHTMLForClientRoutes(t *testing.T) {
	handler, err := handlerFor(indexFS(), fakeSettingsReader{})
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

func TestHandlerFor_InjectsTheConfiguredTitleAndFavicon(t *testing.T) {
	handler, err := handlerFor(indexFS(), fakeSettingsReader{
		cfg: settings.Settings{SiteTitle: "Ludoteca Vicolo Corto", FaviconFilename: "abc123.png"},
	})
	if err != nil {
		t.Fatalf("handlerFor: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	body := rec.Body.String()
	if !strings.Contains(body, "<title>Ludoteca Vicolo Corto</title>") {
		t.Errorf("expected the configured title, got: %s", body)
	}
	if !strings.Contains(body, `href="/api/uploads/abc123.png"`) {
		t.Errorf("expected the configured favicon URL, got: %s", body)
	}
}

func TestHandlerFor_FallsBackToDefaultsWhenNothingIsConfigured(t *testing.T) {
	handler, err := handlerFor(indexFS(), fakeSettingsReader{})
	if err != nil {
		t.Fatalf("handlerFor: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	body := rec.Body.String()
	if !strings.Contains(body, "<title>BoardGames Manager</title>") {
		t.Errorf("expected the default title, got: %s", body)
	}
	if !strings.Contains(body, `href="/favicon.svg"`) {
		t.Errorf("expected the default favicon, got: %s", body)
	}
}

// Un DB irraggiungibile non deve mai rompere il caricamento della pagina:
// meglio i default che una pagina bianca.
func TestHandlerFor_FallsBackToDefaultsWhenSettingsFail(t *testing.T) {
	handler, err := handlerFor(indexFS(), fakeSettingsReader{err: errTestSettingsUnavailable})
	if err != nil {
		t.Fatalf("handlerFor: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != 200 {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<title>BoardGames Manager</title>") || !strings.Contains(body, `href="/favicon.svg"`) {
		t.Errorf("expected the defaults on a settings error, got: %s", body)
	}
}

func TestHandlerFor_EscapesTheConfiguredTitle(t *testing.T) {
	handler, err := handlerFor(indexFS(), fakeSettingsReader{
		cfg: settings.Settings{SiteTitle: `</title><script>alert(1)</script>`},
	})
	if err != nil {
		t.Fatalf("handlerFor: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	body := rec.Body.String()
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Fatalf("expected the site title to be HTML-escaped, got raw markup in: %s", body)
	}
	if !strings.Contains(body, "&lt;/title&gt;&lt;script&gt;") {
		t.Fatalf("expected an escaped title, got: %s", body)
	}
}
