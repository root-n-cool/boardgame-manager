package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist/*
var distFS embed.FS

func Handler() (http.Handler, error) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if len(path) > 0 {
			path = path[1:]
		}
		if _, err := fs.Stat(sub, path); err != nil {
			indexReq := new(http.Request)
			*indexReq = *r
			indexReq.URL.Path = "/"
			fileServer.ServeHTTP(w, indexReq)
			return
		}
		fileServer.ServeHTTP(w, r)
	}), nil
}
