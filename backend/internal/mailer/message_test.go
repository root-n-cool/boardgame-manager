package mailer

import (
	"strings"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		Host:        "smtp.example.org",
		Port:        587,
		Username:    "utente",
		Password:    "segreta",
		FromAddress: "serate@example.org",
		FromName:    "Serate Ludiche",
		TLSMode:     TLSModeSTARTTLS,
	}
}

func TestBuildMessage_ProducesAMultipartAlternative(t *testing.T) {
	now := time.Date(2026, 9, 4, 21, 30, 0, 0, time.UTC)
	raw, err := buildMessage(testConfig(), Message{
		To:       "mario@example.com",
		ToName:   "Mario Rossi",
		Subject:  "Prenotazione confermata",
		TextBody: "Il tuo codice è ABC23XYZ",
		HTMLBody: "<p>Il tuo codice è ABC23XYZ</p>",
	}, "BOUNDARY123", "id-fisso@example.org", now)
	if err != nil {
		t.Fatalf("buildMessage: %v", err)
	}
	got := string(raw)

	for _, want := range []string{
		"From: \"Serate Ludiche\" <serate@example.org>\r\n",
		"To: \"Mario Rossi\" <mario@example.com>\r\n",
		"Subject: Prenotazione confermata\r\n",
		"Date: Fri, 04 Sep 2026 21:30:00 +0000\r\n",
		"Message-ID: <id-fisso@example.org>\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: multipart/alternative; boundary=\"BOUNDARY123\"\r\n",
		"--BOUNDARY123\r\n",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Type: text/html; charset=utf-8",
		"Content-Transfer-Encoding: quoted-printable",
		"--BOUNDARY123--",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("il messaggio non contiene %q\n---\n%s", want, got)
		}
	}

	// Gli header finiscono con una riga vuota, e il corpo comincia col boundary.
	headerEnd := strings.Index(got, "\r\n\r\n")
	if headerEnd == -1 {
		t.Fatal("nessuna riga vuota tra header e corpo")
	}
	if !strings.HasPrefix(got[headerEnd+4:], "--BOUNDARY123\r\n") {
		t.Errorf("il corpo non comincia col boundary: %q", got[headerEnd+4:headerEnd+40])
	}
	// La parte testo viene prima della parte HTML: i client che ne mostrano
	// una sola devono scegliere l'ultima che capiscono, cioè l'HTML.
	if strings.Index(got, "text/plain") > strings.Index(got, "text/html") {
		t.Error("la parte text/plain deve precedere la text/html")
	}
}

func TestBuildMessage_EncodesANonASCIISubject(t *testing.T) {
	raw, err := buildMessage(testConfig(), Message{
		To:       "mario@example.com",
		Subject:  "Prenotazione annullata — Catan",
		TextBody: "ciao",
		HTMLBody: "<p>ciao</p>",
	}, "B", "id@example.org", time.Now())
	if err != nil {
		t.Fatalf("buildMessage: %v", err)
	}
	got := string(raw)
	if strings.Contains(got, "Prenotazione annullata — Catan") {
		t.Error("un oggetto non-ASCII deve viaggiare codificato, non in chiaro")
	}
	if !strings.Contains(got, "=?utf-8?q?") && !strings.Contains(got, "=?utf-8?b?") {
		t.Errorf("oggetto non codificato secondo RFC 2047:\n%s", got)
	}
}

func TestBuildMessage_EncodesAccentsInTheBodyAsQuotedPrintable(t *testing.T) {
	raw, err := buildMessage(testConfig(), Message{
		To:       "mario@example.com",
		Subject:  "test",
		TextBody: "però",
		HTMLBody: "<p>però</p>",
	}, "B", "id@example.org", time.Now())
	if err != nil {
		t.Fatalf("buildMessage: %v", err)
	}
	if strings.Contains(string(raw), "però") {
		t.Error("gli accenti devono uscire in quoted-printable, non in UTF-8 grezzo")
	}
	if !strings.Contains(string(raw), "per=C3=B2") {
		t.Errorf("atteso 'per=C3=B2' nel corpo:\n%s", raw)
	}
}

func TestBuildMessage_OmitsAnEmptyDisplayName(t *testing.T) {
	cfg := testConfig()
	cfg.FromName = ""
	raw, err := buildMessage(cfg, Message{
		To: "mario@example.com", Subject: "test", TextBody: "ciao", HTMLBody: "<p>ciao</p>",
	}, "B", "id@example.org", time.Now())
	if err != nil {
		t.Fatalf("buildMessage: %v", err)
	}
	if !strings.Contains(string(raw), "From: <serate@example.org>\r\n") {
		t.Errorf("atteso un From senza nome visualizzato:\n%s", raw)
	}
}

func TestConfigured(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"completa", testConfig(), true},
		{"senza host", Config{Port: 587, FromAddress: "a@b.org"}, false},
		{"senza porta", Config{Host: "h", FromAddress: "a@b.org"}, false},
		{"senza mittente", Config{Host: "h", Port: 587}, false},
		// Un relay in LAN senza autenticazione è una configurazione valida:
		// user e password vuote non rendono la configurazione incompleta.
		{"senza credenziali", Config{Host: "h", Port: 25, FromAddress: "a@b.org", TLSMode: TLSModeNone}, true},
	}
	for _, tc := range cases {
		if got := tc.cfg.Configured(); got != tc.want {
			t.Errorf("%s: Configured() = %v, atteso %v", tc.name, got, tc.want)
		}
	}
}
