package httpapi_test

import (
	"context"
	"database/sql"
	"testing"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/db"
	"boardgames-manager/internal/events"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/httpapi"
	"boardgames-manager/internal/leaderboard"
	"boardgames-manager/internal/settings"
	"boardgames-manager/internal/storage"
	"boardgames-manager/internal/users"
)

func newTestServer(t *testing.T) *httpapi.Server {
	t.Helper()
	server, _ := newTestServerWithDB(t)
	return server
}

// newTestServerWithDB also hands back the connection, for tests that need to
// reach past the HTTP surface into the database.
func newTestServerWithDB(t *testing.T) (*httpapi.Server, *sql.DB) {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Every connection to ":memory:" is its own separate database, so pin the
	// pool to one: without this a second pooled connection would see an empty
	// schema, and tests that set up database state directly (triggers, rows)
	// could not rely on the handlers seeing it.
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &httpapi.Server{
		Users:       users.NewStore(conn),
		Sessions:    auth.NewSessionStore(conn),
		Settings:    settings.NewStore(conn),
		Games:       games.NewStore(conn),
		Events:      events.NewStore(conn),
		Leaderboard: leaderboard.NewStore(conn),
		Storage:     storage.NewStore(t.TempDir()),
		BGG:         &fakeBGGClient{},
	}, conn
}

// newTestServerWithTranslator monta il server con un provider AI finto già
// configurato: i test che vogliono l'app senza AI usano gli altri helper e
// lasciano il campo a nil.
func newTestServerWithTranslator(t *testing.T, tr *fakeTranslator) (*httpapi.Server, *sql.DB) {
	t.Helper()
	server, conn := newTestServerWithDB(t)
	server.AI = tr
	return server, conn
}

// newTestServerWithMailer monta il server con un server SMTP finto già
// configurato. I test che vogliono l'app senza posta usano gli altri
// helper e lasciano il campo a nil: è lo stato in cui gira
// un'installazione che non ha configurato niente, e ogni flusso ha un
// test in quella forma.
func newTestServerWithMailer(t *testing.T, m *fakeMailer) (*httpapi.Server, *sql.DB) {
	t.Helper()
	server, conn := newTestServerWithDB(t)
	server.Mail = m
	return server, conn
}
