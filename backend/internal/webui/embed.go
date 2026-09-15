package webui

import (
	"context"
	"embed"
	"fmt"
	"html"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"boardgames-manager/internal/settings"
)

// dist/.gitkeep is tracked so this pattern still matches on a fresh clone
// with no frontend build; handlerFor is what turns "no real build output"
// into a loud startup error instead of a browsable directory listing.
//
//go:embed dist/*
var distFS embed.FS

// defaultSiteTitle e defaultFaviconHref sono quello che index.html mostra
// finché nessuno configura un titolo o una favicon dal pannello
// impostazioni, o se le impostazioni non sono raggiungibili: mai una
// pagina rotta per un DB irraggiungibile.
const (
	defaultSiteTitle   = "BoardGames Manager"
	defaultFaviconHref = "/favicon.svg"
)

// SettingsReader è il sottoinsieme di settings.Store che serve qui: leggere
// il branding del sito da iniettare in index.html prima di servirlo.
// settings.Store lo soddisfa già, senza bisogno di adattatori; un finto lo
// soddisfa altrettanto facilmente nei test, senza un database vero.
type SettingsReader interface {
	Get(ctx context.Context) (settings.Settings, error)
}

func Handler(store SettingsReader) (http.Handler, error) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}
	return handlerFor(sub, store)
}

// handlerFor serves root as a single-page app: known paths come straight from
// the filesystem, everything else falls back to index.html so client-side
// routes survive a full page load. index.html itself (direct request or
// fallback) never comes straight from the filesystem: its <title> and
// favicon <link> carry two placeholders, __SITE_TITLE__ and
// __FAVICON_URL__ (see frontend/index.html), replaced per-request with the
// configured branding so a hard refresh — not just the SPA's own DOM
// updates — already shows the right title and favicon.
func handlerFor(root fs.FS, store SettingsReader) (http.Handler, error) {
	// Without this check an unbuilt frontend fails silently: the SPA fallback
	// asks http.FileServer for "/", which happily answers a directory with no
	// index file with a 200 and a browsable file listing.
	if _, err := fs.Stat(root, "index.html"); err != nil {
		return nil, fmt.Errorf("embedded frontend not built: dist/index.html not found — run 'npm run build' in frontend/ before building the Go binary: %w", err)
	}
	indexTemplate, err := fs.ReadFile(root, "index.html")
	if err != nil {
		return nil, fmt.Errorf("read embedded index.html: %w", err)
	}

	fileServer := http.FileServer(http.FS(root))

	serveIndex := func(w http.ResponseWriter, r *http.Request) {
		title, faviconHref := defaultSiteTitle, defaultFaviconHref
		if cfg, err := store.Get(r.Context()); err != nil {
			log.Printf("webui: could not load site settings, serving defaults: %v", err)
		} else {
			if cfg.SiteTitle != "" {
				title = cfg.SiteTitle
			}
			if cfg.FaviconFilename != "" {
				faviconHref = "/api/uploads/" + cfg.FaviconFilename
			}
		}
		body := strings.NewReplacer(
			"__SITE_TITLE__", html.EscapeString(title),
			"__FAVICON_URL__", html.EscapeString(faviconHref),
		).Replace(string(indexTemplate))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(body))
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if len(path) > 0 {
			path = path[1:]
		}
		if path == "" || path == "index.html" {
			serveIndex(w, r)
			return
		}
		if _, err := fs.Stat(root, path); err != nil {
			serveIndex(w, r)
			return
		}
		fileServer.ServeHTTP(w, r)
	}), nil
}
