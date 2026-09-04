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

func TestUpdateAndGet_RoundTripsTheSMTPConfiguration(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	in := settings.Settings{
		DefaultLanguage: "it",
		SMTPHost:        "smtp.gmail.com",
		SMTPPort:        587,
		SMTPUsername:    "serate@example.org",
		SMTPPassword:    "app-password-16ch",
		SMTPFromAddress: "serate@example.org",
		SMTPFromName:    "Serate Ludiche",
		SMTPTLSMode:     "starttls",
	}
	if err := store.Update(ctx, in); err != nil {
		t.Fatalf("update: %v", err)
	}

	out, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.SMTPHost != in.SMTPHost || out.SMTPPort != in.SMTPPort {
		t.Errorf("host/porta = %q/%d, attesi %q/%d", out.SMTPHost, out.SMTPPort, in.SMTPHost, in.SMTPPort)
	}
	if out.SMTPUsername != in.SMTPUsername || out.SMTPPassword != in.SMTPPassword {
		t.Errorf("credenziali non conservate: %q / %q", out.SMTPUsername, out.SMTPPassword)
	}
	if out.SMTPFromAddress != in.SMTPFromAddress || out.SMTPFromName != in.SMTPFromName {
		t.Errorf("mittente non conservato: %q / %q", out.SMTPFromAddress, out.SMTPFromName)
	}
	if out.SMTPTLSMode != in.SMTPTLSMode {
		t.Errorf("TLSMode = %q, atteso %q", out.SMTPTLSMode, in.SMTPTLSMode)
	}
}

// Un'installazione che non ha mai configurato la posta deve leggere le
// impostazioni senza errori e trovare i campi SMTP vuoti: è lo stato
// normale, non un dato mancante.
func TestGet_FreshDatabaseHasNoSMTPConfiguration(t *testing.T) {
	store := newTestStore(t)

	out, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.SMTPHost != "" || out.SMTPPort != 0 || out.SMTPFromAddress != "" {
		t.Errorf("atteso nessun SMTP configurato, ottenuto host=%q porta=%d mittente=%q",
			out.SMTPHost, out.SMTPPort, out.SMTPFromAddress)
	}
}
