package webui

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
)

// dist/.gitkeep is tracked so this pattern still matches on a fresh clone
// with no frontend build; handlerFor is what turns "no real build output"
// into a loud startup error instead of a browsable directory listing.
//
//go:embed dist/*
var distFS embed.FS

func Handler() (http.Handler, error) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}
	return handlerFor(sub)
}

// handlerFor serves root as a single-page app: known paths come straight from
// the filesystem, everything else falls back to index.html so client-side
// routes survive a full page load.
func handlerFor(root fs.FS) (http.Handler, error) {
	// Without this check an unbuilt frontend fails silently: the SPA fallback
	// asks http.FileServer for "/", which happily answers a directory with no
	// index file with a 200 and a browsable file listing.
	if _, err := fs.Stat(root, "index.html"); err != nil {
		return nil, fmt.Errorf("embedded frontend not built: dist/index.html not found — run 'npm run build' in frontend/ before building the Go binary: %w", err)
	}

	fileServer := http.FileServer(http.FS(root))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if len(path) > 0 {
			path = path[1:]
		}
		if _, err := fs.Stat(root, path); err != nil {
			indexReq := r.Clone(r.Context())
			indexReq.URL.Path = "/"
			fileServer.ServeHTTP(w, indexReq)
			return
		}
		fileServer.ServeHTTP(w, r)
	}), nil
}
