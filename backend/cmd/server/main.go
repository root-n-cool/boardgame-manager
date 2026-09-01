package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/bgg"
	"boardgames-manager/internal/db"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/httpapi"
	"boardgames-manager/internal/settings"
	"boardgames-manager/internal/storage"
	"boardgames-manager/internal/users"
	"boardgames-manager/internal/webui"
)

func main() {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	conn, err := db.Open(dataDir + "/app.db")
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer conn.Close()

	if err := db.Migrate(context.Background(), conn); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	server := &httpapi.Server{
		Users:    users.NewStore(conn),
		Sessions: auth.NewSessionStore(conn),
		Settings: settings.NewStore(conn),
		Games:    games.NewStore(conn),
		Storage:  storage.NewStore(dataDir + "/uploads"),
		BGG:      bgg.NewHTTPClient(),
	}

	apiRouter := httpapi.NewRouter(server)

	uiHandler, err := webui.Handler()
	if err != nil {
		log.Fatalf("load embedded frontend: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", apiRouter)
	mux.Handle("/", uiHandler)

	log.Printf("listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
