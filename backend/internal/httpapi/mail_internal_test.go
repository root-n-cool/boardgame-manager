package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/settings"
)

func TestPublicBaseURL_FallsBackToTheRequestHost(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "http://giochi.local:8080/api/events/1/bookings", nil)

	// Senza Settings non si può leggere l'indirizzo configurato: la
	// richiesta è l'unica fonte, e non deve mai andare in panico.
	if got := s.publicBaseURL(req); got != "http://giochi.local:8080" {
		t.Errorf("publicBaseURL = %q, atteso http://giochi.local:8080", got)
	}
}

func TestPublicBaseURL_HonoursForwardedProto(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "http://giochi.local/api/health", nil)
	req.Header.Set("X-Forwarded-Proto", "https")

	if got := s.publicBaseURL(req); got != "https://giochi.local" {
		t.Errorf("publicBaseURL = %q, atteso https://giochi.local", got)
	}
}

func TestSMTPConfigFrom_MapsEverySetting(t *testing.T) {
	cfg := smtpConfigFrom(settings.Settings{
		SMTPHost: "smtp.example.org", SMTPPort: 465, SMTPUsername: "u", SMTPPassword: "p",
		SMTPFromAddress: "serate@example.org", SMTPFromName: "Serate", SMTPTLSMode: "tls",
	})
	if !cfg.Configured() {
		t.Fatal("attesa una Config completa")
	}
	if cfg.Host != "smtp.example.org" || cfg.Port != 465 || cfg.Username != "u" ||
		cfg.Password != "p" || cfg.FromAddress != "serate@example.org" ||
		cfg.FromName != "Serate" || cfg.TLSMode != "tls" {
		t.Errorf("mappatura incompleta: %+v", cfg)
	}
}

func TestSMTPConfigFrom_EmptySettingsAreNotConfigured(t *testing.T) {
	if smtpConfigFrom(settings.Settings{}).Configured() {
		t.Fatal("un'istanza senza posta non deve risultare configurata")
	}
}
