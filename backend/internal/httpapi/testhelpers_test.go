package httpapi_test

import (
	"context"
	"database/sql"
	"testing"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/db"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/httpapi"
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
		Users:    users.NewStore(conn),
		Sessions: auth.NewSessionStore(conn),
		Settings: settings.NewStore(conn),
		Games:    games.NewStore(conn),
		Storage:  storage.NewStore(t.TempDir()),
		BGG:      &fakeBGGClient{},
	}, conn
}
