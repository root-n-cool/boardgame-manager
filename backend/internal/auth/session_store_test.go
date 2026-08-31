package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/db"
	"boardgames-manager/internal/users"
)

func newSessionTestSetup(t *testing.T) (*auth.SessionStore, int64) {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	u, err := users.NewStore(conn).Create(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return auth.NewSessionStore(conn), u.ID
}

func TestSessionCreateAndGetValid(t *testing.T) {
	store, userID := newSessionTestSetup(t)
	ctx := context.Background()

	if err := store.Create(ctx, userID, "hashed-token", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}

	sess, err := store.GetValidByTokenHash(ctx, "hashed-token")
	if err != nil {
		t.Fatalf("get valid: %v", err)
	}
	if sess.UserID != userID {
		t.Fatalf("expected user id %d, got %d", userID, sess.UserID)
	}
}

func TestSession_ExpiredIsNotReturned(t *testing.T) {
	store, userID := newSessionTestSetup(t)
	ctx := context.Background()

	if err := store.Create(ctx, userID, "expired-token", time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}

	_, err := store.GetValidByTokenHash(ctx, "expired-token")
	if !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound for expired session, got %v", err)
	}
}

func TestSession_DeleteRemovesIt(t *testing.T) {
	store, userID := newSessionTestSetup(t)
	ctx := context.Background()

	if err := store.Create(ctx, userID, "some-token", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.Delete(ctx, "some-token"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := store.GetValidByTokenHash(ctx, "some-token")
	if !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound after delete, got %v", err)
	}
}
