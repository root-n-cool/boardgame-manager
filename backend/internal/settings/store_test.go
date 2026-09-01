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
	if cfg.YouTubeAPIKey != "" || cfg.BGGAPIToken != "" {
		t.Fatalf("expected empty optional keys by default, got %+v", cfg)
	}
}

func TestUpdate_PersistsChanges(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.Update(ctx, settings.Settings{
		DefaultLanguage:   "en",
		YouTubeAPIKey:     "yt-key",
		SearchAPIKey:      "search-key",
		SearchAPIProvider: "google",
		BGGAPIToken:       "bgg-token",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	cfg, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cfg.DefaultLanguage != "en" || cfg.YouTubeAPIKey != "yt-key" || cfg.SearchAPIKey != "search-key" ||
		cfg.SearchAPIProvider != "google" || cfg.BGGAPIToken != "bgg-token" {
		t.Fatalf("unexpected settings after update: %+v", cfg)
	}
}
