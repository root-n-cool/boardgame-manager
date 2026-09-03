package settings_test

import (
	"context"
	"testing"

	"boardgames-manager/internal/db"
	"boardgames-manager/internal/settings"
)

func newTestStore(t *testing.T) *settings.Store {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return settings.NewStore(conn)
}

func TestGet_ReturnsDefaultLanguageAfterMigration(t *testing.T) {
	store := newTestStore(t)
	cfg, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cfg.DefaultLanguage != "it" {
		t.Fatalf("expected default language 'it', got %q", cfg.DefaultLanguage)
	}
	if cfg.PublicBaseURL != "" || cfg.BGGAPIToken != "" {
		t.Fatalf("expected the optional settings to be empty by default, got %+v", cfg)
	}
}

func TestUpdate_PersistsChanges(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.Update(ctx, settings.Settings{
		DefaultLanguage: "en",
		PublicBaseURL:   "https://giochi.example.org",
		BGGAPIToken:     "bgg-token",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	cfg, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cfg.DefaultLanguage != "en" || cfg.PublicBaseURL != "https://giochi.example.org" ||
		cfg.BGGAPIToken != "bgg-token" {
		t.Fatalf("unexpected settings after update: %+v", cfg)
	}
}

func TestGet_AIProviderEmptyAfterMigration(t *testing.T) {
	store := newTestStore(t)
	cfg, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cfg.AIBaseURL != "" || cfg.AIAPIKey != "" || cfg.AIModel != "" {
		t.Fatalf("expected an unconfigured AI provider, got %+v", cfg)
	}
}

func TestUpdate_PersistsAIProvider(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.Update(ctx, settings.Settings{
		DefaultLanguage: "it",
		AIBaseURL:       "https://api.example.org/v1",
		AIAPIKey:        "sk-test",
		AIModel:         "gemini-flash-lite-latest",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	cfg, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cfg.AIBaseURL != "https://api.example.org/v1" || cfg.AIAPIKey != "sk-test" ||
		cfg.AIModel != "gemini-flash-lite-latest" {
		t.Fatalf("unexpected AI settings after update: %+v", cfg)
	}
}
