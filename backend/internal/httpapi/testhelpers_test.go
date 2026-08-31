package httpapi_test

import (
	"context"
	"testing"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/db"
	"boardgames-manager/internal/httpapi"
	"boardgames-manager/internal/settings"
	"boardgames-manager/internal/users"
)

func newTestServer(t *testing.T) *httpapi.Server {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &httpapi.Server{
		Users:    users.NewStore(conn),
		Sessions: auth.NewSessionStore(conn),
		Settings: settings.NewStore(conn),
	}
}
