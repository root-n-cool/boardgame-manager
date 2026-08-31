package users_test

import (
	"context"
	"errors"
	"testing"

	"boardgames-manager/internal/db"
	"boardgames-manager/internal/users"
)

func newTestStore(t *testing.T) *users.Store {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return users.NewStore(conn)
}

func TestCreateAndGetByEmail(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	created, err := store.Create(ctx, "admin@example.com", "hashed-value")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected a non-zero id")
	}

	found, err := store.GetByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("get by email: %v", err)
	}
	if found.ID != created.ID {
		t.Fatalf("expected same id, got %d vs %d", found.ID, created.ID)
	}
}

func TestCreate_DuplicateEmailReturnsError(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Create(ctx, "dup@example.com", "hash1"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := store.Create(ctx, "dup@example.com", "hash2")
	if !errors.Is(err, users.ErrDuplicateEmail) {
		t.Fatalf("expected ErrDuplicateEmail, got %v", err)
	}
}

func TestCount_ReflectsNumberOfUsers(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	count, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 users initially, got %d", count)
	}

	if _, err := store.Create(ctx, "a@example.com", "hash"); err != nil {
		t.Fatalf("create: %v", err)
	}
	count, err = store.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 user, got %d", count)
	}
}

func TestListAndDelete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	a, _ := store.Create(ctx, "a@example.com", "hash")
	_, _ = store.Create(ctx, "b@example.com", "hash")

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 users, got %d", len(list))
	}

	if err := store.Delete(ctx, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	list, err = store.List(ctx)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 user after delete, got %d", len(list))
	}
}

func TestGetByEmail_NotFound(t *testing.T) {
	store := newTestStore(t)
	_, err := store.GetByEmail(context.Background(), "missing@example.com")
	if !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
