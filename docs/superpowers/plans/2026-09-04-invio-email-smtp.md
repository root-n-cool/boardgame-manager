# Invio email SMTP — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Mandare tre email opzionali — invito admin, conferma prenotazione con link diretti a disdetta e punteggi, avviso di annullamento — senza che nulla dell'app cambi comportamento quando l'SMTP non è configurato.

**Architecture:** Un package `internal/mailer` che parla SMTP con la stdlib ed espone un `Sender` iniettabile più `ErrNotConfigured`, esattamente come `internal/ai` fa per il provider di traduzione. La glue in `httpapi/mail.go` costruisce il sender a ogni richiesta dalle impostazioni salvate e manda in una goroutine fire-and-forget, così nessun handler cambia il proprio esito per un problema di posta. I contenuti vivono in funzioni pure in `httpapi/mail_templates.go`.

**Tech Stack:** Go 1.25 (`net/smtp`, `mime/multipart`, `mime/quotedprintable`, `net/mail`), SQLite via `modernc.org/sqlite`, Vue 3 `<script setup>` + TypeScript, Vue Router.

**Spec:** `docs/superpowers/specs/2026-09-04-invio-email-smtp-design.md`

## Global Constraints

- **SMTP è opzionale.** L'app senza SMTP configurato funziona esattamente come oggi. `mailer.ErrNotConfigured` non viene mai loggato come errore e non raggiunge mai una risposta HTTP, con l'unica eccezione di `POST /api/settings/smtp/test`. Nessun handler cambia il proprio esito per un guasto di posta. Nessun campo SMTP è obbligatorio in `PUT /api/settings`. La UI non mostra mai un avviso perché la posta manca.
- **Nessuna dipendenza nuova.** `backend/go.mod` non cambia: solo stdlib.
- **Comandi Go solo in Docker.** Ogni `go test` di questo piano si lancia così, dalla radice del repo:
  ```bash
  docker run --rm -v "$(pwd)/backend:/app" \
    -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
    -w /app golang:1.25 go test ./...
  ```
  Non sostituire i due volumi nominati. `npm` invece gira in locale in `frontend/`.
- **Migrazioni forward-only:** un file nuovo `NNNN_nome.sql` in `backend/internal/db/migrations/`, mai una modifica a uno già rilasciato.
- **UI in italiano**, stringhe dirette nei componenti, nessun i18n. Messaggi d'errore rivolti all'admin o al partecipante in italiano; i codici d'errore interni dell'API restano in inglese come il resto del progetto.
- **Palette delle mail** da `DESIGN.md`: feltro `#1f4d3a`, carta `#faf6ec`, carta-alt `#f2ead4`, inchiostro `#241f18`, inchiostro tenue `#6e6250`, accento `#9c2b2b`. Stili inline, tabelle per il layout, max 560px, nessuna immagine e nessun font esterno.
- **Git:** branch `main`, commit in inglese in stile conventional commits. Nessun push: i commit restano locali.
- **Ultimo task obbligatorio:** `/impeccable` sulla superficie frontend toccata, come richiede il CLAUDE.md.

---

### Task 1: `internal/mailer` — tipi e composizione del messaggio

Il cuore testabile del package: una funzione pura che produce i byte RFC 5322 da spedire. Deterministica perché boundary, Message-ID e data arrivano come parametri — è `Send` (Task 2) a generarli.

**Files:**
- Create: `backend/internal/mailer/mailer.go`
- Create: `backend/internal/mailer/message.go`
- Test: `backend/internal/mailer/message_test.go`

**Interfaces:**
- Consumes: niente.
- Produces:
  - `mailer.ErrNotConfigured error`
  - `mailer.Config{Host string; Port int; Username, Password, FromAddress, FromName string; TLSMode string}`
  - `mailer.Message{To, ToName, Subject, TextBody, HTMLBody string}`
  - `mailer.Sender interface{ Send(ctx context.Context, m Message) error }`
  - costanti `mailer.TLSModeSTARTTLS = "starttls"`, `mailer.TLSModeImplicit = "tls"`, `mailer.TLSModeNone = "none"`
  - `func (c Config) Configured() bool`
  - `func buildMessage(cfg Config, m Message, boundary, messageID string, now time.Time) ([]byte, error)` (non esportata, usata dal Task 2)

- [ ] **Step 1: Write the failing test**

Crea `backend/internal/mailer/message_test.go`:

```go
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
		TextBody: "Il tuo codice e' ABC23XYZ",
		HTMLBody: "<p>Il tuo codice e' ABC23XYZ</p>",
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
	// una sola devono scegliere l'ultima che capiscono, cioe' l'HTML.
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
		// Un relay in LAN senza autenticazione e' una configurazione valida:
		// user e password vuote non rendono la configurazione incompleta.
		{"senza credenziali", Config{Host: "h", Port: 25, FromAddress: "a@b.org", TLSMode: TLSModeNone}, true},
	}
	for _, tc := range cases {
		if got := tc.cfg.Configured(); got != tc.want {
			t.Errorf("%s: Configured() = %v, atteso %v", tc.name, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/mailer/ -v
```

Expected: FAIL — il package non esiste (`no Go files` / `undefined: Config`).

- [ ] **Step 3: Write `mailer.go` (tipi e contratto)**

```go
// Package mailer manda email via SMTP. Nel progetto serve alle tre
// comunicazioni verso l'esterno: l'invito di un amministratore, la
// conferma di una prenotazione e l'avviso di annullamento.
//
// Come internal/ai, e' un sottosistema opzionale: senza configurazione
// restituisce ErrNotConfigured, che per il chiamante non e' un guasto ma
// l'app che gira senza posta — lo stato in cui e' nata e in cui deve
// continuare a funzionare per intero.
package mailer

import (
	"context"
	"errors"
	"strings"
)

// ErrNotConfigured dice che l'admin non ha (ancora) messo un server SMTP.
// Non e' un errore da mostrare ne' da loggare: e' una configurazione
// valida e supportata.
var ErrNotConfigured = errors.New("smtp not configured")

// I tre modi di cifrare la connessione, che coprono i casi reali:
// STARTTLS sulla 587 (Gmail, Mailjet, Brevo), TLS implicito sulla 465,
// nessuna cifratura per un relay dentro la stessa rete.
const (
	TLSModeSTARTTLS = "starttls"
	TLSModeImplicit = "tls"
	TLSModeNone     = "none"
)

type Config struct {
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
	FromName    string
	// TLSMode e' una delle tre costanti sopra. Un valore vuoto o ignoto
	// si comporta come STARTTLS, che e' il caso di gran lunga piu' comune.
	TLSMode string
}

// Configured dice se c'e' abbastanza per provare a spedire. User e
// password non contano: un relay senza autenticazione e' legittimo.
func (c Config) Configured() bool {
	return strings.TrimSpace(c.Host) != "" && c.Port != 0 && strings.TrimSpace(c.FromAddress) != ""
}

// Message e' una mail a un solo destinatario, in testo e HTML. Il
// progetto non manda mai a piu' indirizzi insieme: ogni comunicazione e'
// personale, e un CC involontario esporrebbe i contatti dei partecipanti.
type Message struct {
	To       string
	ToName   string
	Subject  string
	TextBody string
	HTMLBody string
}

type Sender interface {
	Send(ctx context.Context, m Message) error
}
```

- [ ] **Step 4: Write `message.go` (composizione)**

```go
package mailer

import (
	"bytes"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
	"time"
)

// buildMessage produce i byte da consegnare a DATA. E' pura di proposito:
// boundary, Message-ID e ora arrivano dal chiamante, cosi' il contenuto e'
// verificabile byte per byte nei test invece di cambiare a ogni esecuzione.
//
// Il corpo e' sempre un multipart/alternative con la parte testo prima
// della parte HTML: i client mostrano l'ultima che sanno leggere, e chi
// legge in solo testo trova comunque codice e link.
func buildMessage(cfg Config, m Message, boundary, messageID string, now time.Time) ([]byte, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.SetBoundary(boundary); err != nil {
		return nil, fmt.Errorf("boundary: %w", err)
	}
	if err := writeQuotedPrintablePart(mw, "text/plain; charset=utf-8", m.TextBody); err != nil {
		return nil, err
	}
	if err := writeQuotedPrintablePart(mw, "text/html; charset=utf-8", m.HTMLBody); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	// mail.Address.String() mette le virgolette dove servono e codifica da
	// se' un nome non-ASCII; con nome vuoto rende il solo <indirizzo>.
	headers := []string{
		"From: " + (&mail.Address{Name: cfg.FromName, Address: cfg.FromAddress}).String(),
		"To: " + (&mail.Address{Name: m.ToName, Address: m.To}).String(),
		"Subject: " + mime.QEncoding.Encode("utf-8", m.Subject),
		"Date: " + now.Format(time.RFC1123Z),
		"Message-ID: <" + messageID + ">",
		"MIME-Version: 1.0",
		`Content-Type: multipart/alternative; boundary="` + boundary + `"`,
	}

	var out bytes.Buffer
	out.WriteString(strings.Join(headers, "\r\n"))
	out.WriteString("\r\n\r\n")
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

func writeQuotedPrintablePart(mw *multipart.Writer, contentType, content string) error {
	h := textproto.MIMEHeader{}
	h.Set("Content-Type", contentType)
	h.Set("Content-Transfer-Encoding", "quoted-printable")
	part, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	qp := quotedprintable.NewWriter(part)
	if _, err := qp.Write([]byte(content)); err != nil {
		return err
	}
	return qp.Close()
}
```

- [ ] **Step 5: Run the test to verify it passes**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/mailer/ -v
```

Expected: PASS, cinque test.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/mailer/
git commit -m "feat: add the mailer package message builder"
```

---

### Task 2: `internal/mailer` — invio SMTP

`Send` genera le parti casuali, apre la connessione secondo `TLSMode` e consegna. Verificato contro un finto server SMTP in-process, che rende il test veloce e senza rete.

**Files:**
- Create: `backend/internal/mailer/smtp.go`
- Create: `backend/internal/mailer/fakeserver_test.go`
- Test: `backend/internal/mailer/smtp_test.go`

**Interfaces:**
- Consumes: dal Task 1 `Config`, `Message`, `Sender`, `ErrNotConfigured`, `Configured()`, `buildMessage(...)`, le tre costanti `TLSMode*`.
- Produces:
  - `mailer.SMTPSender struct{ Config }`
  - `func NewSMTPSender(cfg Config) *SMTPSender`
  - `func (s *SMTPSender) Send(ctx context.Context, m Message) error` — implementa `Sender`

- [ ] **Step 1: Write the fake SMTP server (test helper)**

Crea `backend/internal/mailer/fakeserver_test.go`:

```go
package mailer

import (
	"bufio"
	"net"
	"strings"
	"sync"
	"testing"
)

// fakeSMTPServer parla il minimo indispensabile del protocollo SMTP su una
// porta locale: abbastanza per verificare che Send faccia l'handshake giusto
// e consegni il messaggio che ci aspettiamo, senza toccare la rete vera.
//
// Non annuncia STARTTLS: i test lo usano con TLSModeNone, perche' cifrare
// non e' quello che stiamo verificando qui.
type fakeSMTPServer struct {
	Addr string

	mu       sync.Mutex
	from     string
	rcpt     []string
	data     string
	authSeen bool

	listener net.Listener
	wg       sync.WaitGroup
}

func startFakeSMTPServer(t *testing.T) *fakeSMTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeSMTPServer{Addr: ln.Addr().String(), listener: ln}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			s.handle(conn)
		}
	}()
	t.Cleanup(func() {
		ln.Close()
		s.wg.Wait()
	})
	return s
}

func (s *fakeSMTPServer) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	write := func(line string) { conn.Write([]byte(line + "\r\n")) }

	write("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			// Risposta multi-riga: tutte con "-" tranne l'ultima.
			write("250-fake hello")
			write("250 AUTH PLAIN")
		case strings.HasPrefix(cmd, "AUTH"):
			s.mu.Lock()
			s.authSeen = true
			s.mu.Unlock()
			write("235 2.7.0 accettata")
		case strings.HasPrefix(cmd, "MAIL FROM"):
			s.mu.Lock()
			s.from = strings.TrimSpace(strings.TrimPrefix(cmd, "MAIL FROM:"))
			s.mu.Unlock()
			write("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO"):
			s.mu.Lock()
			s.rcpt = append(s.rcpt, strings.TrimSpace(strings.TrimPrefix(cmd, "RCPT TO:")))
			s.mu.Unlock()
			write("250 ok")
		case cmd == "DATA":
			write("354 manda pure")
			var body strings.Builder
			for {
				dl, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if dl == ".\r\n" {
					break
				}
				body.WriteString(dl)
			}
			s.mu.Lock()
			s.data = body.String()
			s.mu.Unlock()
			write("250 accettato")
		case cmd == "QUIT":
			write("221 ciao")
			return
		case cmd == "RSET", cmd == "NOOP":
			write("250 ok")
		default:
			write("500 comando sconosciuto")
		}
	}
}

func (s *fakeSMTPServer) snapshot() (from string, rcpt []string, data string, authSeen bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.from, append([]string(nil), s.rcpt...), s.data, s.authSeen
}

// hostPort spezza l'indirizzo del listener nei due valori che Config vuole.
func (s *fakeSMTPServer) hostPort(t *testing.T) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(s.Addr)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}
	return host, port
}
```

- [ ] **Step 2: Write the failing test**

Crea `backend/internal/mailer/smtp_test.go`:

```go
package mailer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSMTPSender_SendDeliversTheMessage(t *testing.T) {
	srv := startFakeSMTPServer(t)
	host, port := srv.hostPort(t)

	sender := NewSMTPSender(Config{
		Host: host, Port: port,
		FromAddress: "serate@example.org", FromName: "Serate Ludiche",
		TLSMode: TLSModeNone,
	})

	err := sender.Send(context.Background(), Message{
		To: "mario@example.com", ToName: "Mario Rossi",
		Subject: "Prenotazione confermata", TextBody: "codice ABC23XYZ", HTMLBody: "<p>codice ABC23XYZ</p>",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	from, rcpt, data, authSeen := srv.snapshot()
	if from != "<serate@example.org>" {
		t.Errorf("MAIL FROM = %q, atteso <serate@example.org>", from)
	}
	if len(rcpt) != 1 || rcpt[0] != "<mario@example.com>" {
		t.Errorf("RCPT TO = %v, atteso un solo <mario@example.com>", rcpt)
	}
	if authSeen {
		t.Error("senza username non deve partire nessun AUTH")
	}
	if !strings.Contains(data, "Subject: Prenotazione confermata") {
		t.Errorf("oggetto assente nel messaggio consegnato:\n%s", data)
	}
	if !strings.Contains(data, "codice ABC23XYZ") {
		t.Errorf("corpo assente nel messaggio consegnato:\n%s", data)
	}
}

func TestSMTPSender_NotConfiguredReturnsErrNotConfigured(t *testing.T) {
	sender := NewSMTPSender(Config{})
	err := sender.Send(context.Background(), Message{To: "mario@example.com"})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("atteso ErrNotConfigured, ottenuto %v", err)
	}
}

func TestSMTPSender_UnreachableHostReturnsAnError(t *testing.T) {
	// Porta 1 su localhost: nessuno ascolta, e il rifiuto e' immediato.
	sender := NewSMTPSender(Config{
		Host: "127.0.0.1", Port: 1, FromAddress: "serate@example.org", TLSMode: TLSModeNone,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := sender.Send(ctx, Message{To: "mario@example.com", Subject: "x", TextBody: "x", HTMLBody: "x"})
	if err == nil {
		t.Fatal("atteso un errore di connessione")
	}
	if errors.Is(err, ErrNotConfigured) {
		t.Fatal("un host irraggiungibile non e' 'non configurato'")
	}
}

func TestSMTPSender_STARTTLSRequiredButNotOfferedFails(t *testing.T) {
	// Il finto server non annuncia STARTTLS: chiedendolo, Send deve
	// rifiutare invece di consegnare in chiaro senza dirlo.
	srv := startFakeSMTPServer(t)
	host, port := srv.hostPort(t)
	sender := NewSMTPSender(Config{
		Host: host, Port: port, FromAddress: "serate@example.org", TLSMode: TLSModeSTARTTLS,
	})

	err := sender.Send(context.Background(), Message{
		To: "mario@example.com", Subject: "x", TextBody: "x", HTMLBody: "x",
	})
	if err == nil {
		t.Fatal("atteso un errore: STARTTLS richiesto e non offerto")
	}
	if !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("il messaggio deve nominare STARTTLS per essere azionabile, ottenuto: %v", err)
	}
	if _, _, data, _ := srv.snapshot(); data != "" {
		t.Error("nessun messaggio deve essere consegnato se la cifratura richiesta non c'e'")
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/mailer/ -run TestSMTPSender -v
```

Expected: FAIL con `undefined: NewSMTPSender`.

- [ ] **Step 4: Write `smtp.go`**

```go
package mailer

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// dialTimeout e' il tetto sull'apertura della connessione quando il
// chiamante non ha messo una scadenza nel context.
const dialTimeout = 15 * time.Second

type SMTPSender struct {
	Config
}

func NewSMTPSender(cfg Config) *SMTPSender {
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.FromAddress = strings.TrimSpace(cfg.FromAddress)
	cfg.FromName = strings.TrimSpace(cfg.FromName)
	cfg.TLSMode = strings.ToLower(strings.TrimSpace(cfg.TLSMode))
	return &SMTPSender{Config: cfg}
}

// Send consegna un messaggio a un destinatario. Gli errori sono scritti
// per essere letti dall'admin nel pannello impostazioni: dicono quale
// passo e' fallito, perche' "connessione rifiutata" e "autenticazione
// rifiutata" si risolvono in modi opposti.
func (s *SMTPSender) Send(ctx context.Context, m Message) error {
	if !s.Configured() {
		return ErrNotConfigured
	}

	raw, err := s.compose(m)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	dialer := &net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("connessione a %s non riuscita: %w", addr, err)
	}
	defer conn.Close()
	// Una scadenza sul socket: senza, un server che apre la connessione e
	// poi tace terrebbe la goroutine appesa a tempo indeterminato.
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	} else {
		conn.SetDeadline(time.Now().Add(dialTimeout))
	}

	if s.TLSMode == TLSModeImplicit {
		conn = tls.Client(conn, &tls.Config{ServerName: s.Host})
	}

	client, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return fmt.Errorf("handshake SMTP non riuscito: %w", err)
	}
	defer client.Close()

	// Tutto quello che non e' TLS implicito o "nessuna sicurezza" vuole
	// STARTTLS, compreso il valore vuoto: e' il default e il caso comune.
	if s.TLSMode != TLSModeImplicit && s.TLSMode != TLSModeNone {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("il server non offre STARTTLS: scegli TLS implicito o nessuna sicurezza")
		}
		if err := client.StartTLS(&tls.Config{ServerName: s.Host}); err != nil {
			return fmt.Errorf("STARTTLS non riuscito: %w", err)
		}
	}

	if s.Username != "" {
		// smtp.PlainAuth rifiuta di mandare la password su una connessione
		// non cifrata (tranne verso localhost): e' una protezione della
		// stdlib, e l'errore che ne esce e' quello giusto da mostrare.
		if err := client.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return fmt.Errorf("autenticazione rifiutata: %w", err)
		}
	}

	if err := client.Mail(s.FromAddress); err != nil {
		return fmt.Errorf("mittente %s rifiutato: %w", s.FromAddress, err)
	}
	if err := client.Rcpt(m.To); err != nil {
		return fmt.Errorf("destinatario %s rifiutato: %w", m.To, err)
	}
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("invio del corpo non riuscito: %w", err)
	}
	if _, err := wc.Write(raw); err != nil {
		wc.Close()
		return fmt.Errorf("scrittura del corpo non riuscita: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("il server ha rifiutato il messaggio: %w", err)
	}
	return client.Quit()
}

// compose genera le parti che devono cambiare a ogni messaggio e delega a
// buildMessage, che invece e' pura.
func (s *SMTPSender) compose(m Message) ([]byte, error) {
	boundary, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	id, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	// Il dominio del Message-ID e' quello del mittente: e' l'unico che
	// possiamo dire di rappresentare.
	domain := "localhost"
	if at := strings.LastIndex(s.FromAddress, "@"); at != -1 && at+1 < len(s.FromAddress) {
		domain = s.FromAddress[at+1:]
	}
	return buildMessage(s.Config, m, boundary, id+"@"+domain, time.Now())
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/mailer/ -v
```

Expected: PASS, nove test in tutto.

- [ ] **Step 6: Verify `go.mod` did not change**

```bash
git diff --stat backend/go.mod backend/go.sum
```

Expected: nessun output — solo stdlib, come da vincolo globale.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/mailer/
git commit -m "feat: send mail over SMTP with STARTTLS, implicit TLS or none"
```

---

### Task 3: migrazione e campi SMTP nelle impostazioni

**Files:**
- Create: `backend/internal/db/migrations/0012_smtp.sql`
- Modify: `backend/internal/settings/store.go`
- Test: `backend/internal/settings/store_test.go`

**Interfaces:**
- Consumes: niente dai task precedenti.
- Produces: su `settings.Settings` i campi `SMTPHost string`, `SMTPPort int`, `SMTPUsername string`, `SMTPPassword string`, `SMTPFromAddress string`, `SMTPFromName string`, `SMTPTLSMode string`, letti da `Get` e scritti da `Update` come gli altri.

- [ ] **Step 1: Write the failing test**

Aggiungi a `backend/internal/settings/store_test.go`:

```go
func TestUpdateAndGet_RoundTripsTheSMTPConfiguration(t *testing.T) {
	store, _ := newTestStore(t)
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
// impostazioni senza errori e trovare i campi SMTP vuoti: e' lo stato
// normale, non un dato mancante.
func TestGet_FreshDatabaseHasNoSMTPConfiguration(t *testing.T) {
	store, _ := newTestStore(t)

	out, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.SMTPHost != "" || out.SMTPPort != 0 || out.SMTPFromAddress != "" {
		t.Errorf("atteso nessun SMTP configurato, ottenuto host=%q porta=%d mittente=%q",
			out.SMTPHost, out.SMTPPort, out.SMTPFromAddress)
	}
}
```

Se `newTestStore` non esiste con questa firma in `store_test.go`, riusa l'helper già presente nel file invece di aggiungerne uno.

- [ ] **Step 2: Run the test to verify it fails**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/settings/ -v
```

Expected: FAIL — `unknown field SMTPHost in struct literal`.

- [ ] **Step 3: Write the migration**

Crea `backend/internal/db/migrations/0012_smtp.sql`:

```sql
-- Server SMTP opzionale. Con host, porta e indirizzo mittente valorizzati
-- l'app manda l'invito di un amministratore, la conferma di una
-- prenotazione e l'avviso di annullamento; senza si comporta come prima,
-- col codice di prenotazione solo a schermo.
--
-- smtp_username e smtp_password restano vuote per un relay senza
-- autenticazione: non fanno parte del minimo indispensabile.
ALTER TABLE app_settings ADD COLUMN smtp_host TEXT;
ALTER TABLE app_settings ADD COLUMN smtp_port INTEGER;
ALTER TABLE app_settings ADD COLUMN smtp_username TEXT;
ALTER TABLE app_settings ADD COLUMN smtp_password TEXT;
ALTER TABLE app_settings ADD COLUMN smtp_from_address TEXT;
ALTER TABLE app_settings ADD COLUMN smtp_from_name TEXT;

-- 'starttls' (587), 'tls' (465) o 'none'. Vuoto vale come 'starttls'.
ALTER TABLE app_settings ADD COLUMN smtp_tls_mode TEXT;
```

- [ ] **Step 4: Extend `settings.Settings`, `Get` and `Update`**

In `backend/internal/settings/store.go`, aggiungi i campi alla struct dopo quelli AI:

```go
	// I campi SMTP valgono solo con host, porta e indirizzo mittente
	// insieme; senza, l'app resta senza posta e non e' un errore. Come
	// AIAPIKey, SMTPPassword e' un segreto e non esce mai in chiaro
	// dall'API.
	SMTPHost        string
	SMTPPort        int
	SMTPUsername    string
	SMTPPassword    string
	SMTPFromAddress string
	SMTPFromName    string
	SMTPTLSMode     string
```

Sostituisci `Get`:

```go
func (s *Store) Get(ctx context.Context) (Settings, error) {
	var out Settings
	var baseURL, bggToken, aiBaseURL, aiAPIKey, aiModel sql.NullString
	var smtpHost, smtpUser, smtpPass, smtpFrom, smtpFromName, smtpTLS sql.NullString
	var smtpPort sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT default_language, public_base_url, bgg_api_token, ai_base_url, ai_api_key, ai_model,
		        smtp_host, smtp_port, smtp_username, smtp_password, smtp_from_address, smtp_from_name, smtp_tls_mode
		 FROM app_settings WHERE id = 1`,
	).Scan(&out.DefaultLanguage, &baseURL, &bggToken, &aiBaseURL, &aiAPIKey, &aiModel,
		&smtpHost, &smtpPort, &smtpUser, &smtpPass, &smtpFrom, &smtpFromName, &smtpTLS)
	if err != nil {
		return Settings{}, err
	}
	out.PublicBaseURL = baseURL.String
	out.BGGAPIToken = bggToken.String
	out.AIBaseURL = aiBaseURL.String
	out.AIAPIKey = aiAPIKey.String
	out.AIModel = aiModel.String
	out.SMTPHost = smtpHost.String
	out.SMTPPort = int(smtpPort.Int64)
	out.SMTPUsername = smtpUser.String
	out.SMTPPassword = smtpPass.String
	out.SMTPFromAddress = smtpFrom.String
	out.SMTPFromName = smtpFromName.String
	out.SMTPTLSMode = smtpTLS.String
	return out, nil
}
```

Sostituisci `Update` e aggiungi l'helper per la porta:

```go
func (s *Store) Update(ctx context.Context, in Settings) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE app_settings SET default_language = ?, public_base_url = ?, bgg_api_token = ?,
		 ai_base_url = ?, ai_api_key = ?, ai_model = ?,
		 smtp_host = ?, smtp_port = ?, smtp_username = ?, smtp_password = ?,
		 smtp_from_address = ?, smtp_from_name = ?, smtp_tls_mode = ?
		 WHERE id = 1`,
		in.DefaultLanguage, nullIfEmpty(in.PublicBaseURL), nullIfEmpty(in.BGGAPIToken),
		nullIfEmpty(in.AIBaseURL), nullIfEmpty(in.AIAPIKey), nullIfEmpty(in.AIModel),
		nullIfEmpty(in.SMTPHost), nullIfZero(in.SMTPPort), nullIfEmpty(in.SMTPUsername),
		nullIfEmpty(in.SMTPPassword), nullIfEmpty(in.SMTPFromAddress),
		nullIfEmpty(in.SMTPFromName), nullIfEmpty(in.SMTPTLSMode),
	)
	return err
}

func nullIfZero(v int) sql.NullInt64 {
	return sql.NullInt64{Int64: int64(v), Valid: v != 0}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/settings/ ./internal/db/ -v
```

Expected: PASS, compresi i test di migrazione già esistenti.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/db/migrations/0012_smtp.sql backend/internal/settings/
git commit -m "feat: store the optional SMTP configuration in settings"
```

---

### Task 4: API impostazioni — campi SMTP in `GET` e `PUT`

Nessun campo obbligatorio, password mascherata come `bggApiToken` e `aiApiKey`, e un booleano `smtpConfigured` su cui la UI decide se abilitare la prova.

**Files:**
- Modify: `backend/internal/httpapi/settings_handlers.go`
- Test: `backend/internal/httpapi/settings_handlers_test.go`

**Interfaces:**
- Consumes: dal Task 3 i campi `SMTP*` di `settings.Settings`.
- Produces: in `GET /api/settings` i campi JSON `smtpHost`, `smtpPort` (numero), `smtpUsername`, `smtpFromAddress`, `smtpFromName`, `smtpTlsMode`, `smtpPasswordSet` (bool), `smtpPasswordMasked` (omesso se assente), `smtpConfigured` (bool). In `PUT /api/settings` gli stessi nomi in ingresso più `smtpPassword`.

- [ ] **Step 1: Write the failing test**

Aggiungi a `backend/internal/httpapi/settings_handlers_test.go`:

```go
func TestPutSettings_SavesSMTPAndMasksThePassword(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]any{
		"defaultLanguage": "it",
		"smtpHost":        "smtp.gmail.com",
		"smtpPort":        587,
		"smtpUsername":    "serate@example.org",
		"smtpPassword":    "abcdefghilmnopqr",
		"smtpFromAddress": "serate@example.org",
		"smtpFromName":    "Serate Ludiche",
		"smtpTlsMode":     "starttls",
	})
	putReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(payload))
	putReq.AddCookie(cookie)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", putRec.Code, putRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	getReq.AddCookie(cookie)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	var body struct {
		SMTPHost           string `json:"smtpHost"`
		SMTPPort           int    `json:"smtpPort"`
		SMTPUsername       string `json:"smtpUsername"`
		SMTPFromAddress    string `json:"smtpFromAddress"`
		SMTPFromName       string `json:"smtpFromName"`
		SMTPTLSMode        string `json:"smtpTlsMode"`
		SMTPPasswordSet    bool   `json:"smtpPasswordSet"`
		SMTPPasswordMasked string `json:"smtpPasswordMasked"`
		SMTPConfigured     bool   `json:"smtpConfigured"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.SMTPHost != "smtp.gmail.com" || body.SMTPPort != 587 {
		t.Errorf("host/porta = %q/%d", body.SMTPHost, body.SMTPPort)
	}
	if body.SMTPFromName != "Serate Ludiche" || body.SMTPTLSMode != "starttls" {
		t.Errorf("mittente/tls = %q/%q", body.SMTPFromName, body.SMTPTLSMode)
	}
	if !body.SMTPPasswordSet {
		t.Error("expected smtpPasswordSet to be true")
	}
	if body.SMTPPasswordMasked == "abcdefghilmnopqr" {
		t.Error("expected the SMTP password to be masked, not returned in clear")
	}
	if !body.SMTPConfigured {
		t.Error("expected smtpConfigured to be true with host, port and sender set")
	}
}

func TestPutSettings_EmptySMTPPasswordPreservesTheExistingOne(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	first, _ := json.Marshal(map[string]any{
		"defaultLanguage": "it", "smtpHost": "smtp.example.org", "smtpPort": 587,
		"smtpPassword": "prima-password", "smtpFromAddress": "serate@example.org",
	})
	req1 := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(first))
	req1.AddCookie(cookie)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first put: %d %s", rec1.Code, rec1.Body.String())
	}

	// Secondo salvataggio senza password, come fa il form dopo il primo.
	second, _ := json.Marshal(map[string]any{
		"defaultLanguage": "it", "smtpHost": "smtp.example.org", "smtpPort": 465,
		"smtpPassword": "", "smtpFromAddress": "serate@example.org", "smtpTlsMode": "tls",
	})
	req2 := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(second))
	req2.AddCookie(cookie)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second put: %d %s", rec2.Code, rec2.Body.String())
	}

	var stored string
	if err := conn.QueryRow(`SELECT smtp_password FROM app_settings WHERE id = 1`).Scan(&stored); err != nil {
		t.Fatalf("read password: %v", err)
	}
	if stored != "prima-password" {
		t.Errorf("password = %q, attesa 'prima-password'", stored)
	}
}

// Il vincolo globale: nessun campo SMTP e' obbligatorio, e salvare le
// impostazioni senza toccarli deve restare possibile.
func TestPutSettings_SMTPFieldsAreOptional(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]any{"defaultLanguage": "it"})
	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	getReq.AddCookie(cookie)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	var body struct {
		SMTPConfigured  bool `json:"smtpConfigured"`
		SMTPPasswordSet bool `json:"smtpPasswordSet"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.SMTPConfigured || body.SMTPPasswordSet {
		t.Error("expected no SMTP configuration on a fresh instance")
	}
}

func TestPutSettings_RejectsAnUnknownTLSMode(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]any{"defaultLanguage": "it", "smtpTlsMode": "quantum"})
	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run TestPutSettings -v
```

Expected: FAIL — i campi `smtp*` non sono nella risposta e `smtpConfigured` è falso.

- [ ] **Step 3: Extend `settingsResponse` and `getSettingsHandler`**

In `backend/internal/httpapi/settings_handlers.go`, aggiungi alla struct `settingsResponse`:

```go
	// Host, porta, utente, mittente e modo TLS sono dati da rileggere e
	// controllare, come PublicBaseURL; la password e' un segreto e segue
	// BGGAPIToken. SMTPConfigured e' il booleano su cui la UI abilita la
	// prova d'invio: serve host, porta e mittente, non la password.
	SMTPHost           string `json:"smtpHost"`
	SMTPPort           int    `json:"smtpPort"`
	SMTPUsername       string `json:"smtpUsername"`
	SMTPFromAddress    string `json:"smtpFromAddress"`
	SMTPFromName       string `json:"smtpFromName"`
	SMTPTLSMode        string `json:"smtpTlsMode"`
	SMTPPasswordSet    bool   `json:"smtpPasswordSet"`
	SMTPPasswordMasked string `json:"smtpPasswordMasked,omitempty"`
	SMTPConfigured     bool   `json:"smtpConfigured"`
```

e in `getSettingsHandler`, prima di `writeJSON`:

```go
	resp.SMTPHost = cfg.SMTPHost
	resp.SMTPPort = cfg.SMTPPort
	resp.SMTPUsername = cfg.SMTPUsername
	resp.SMTPFromAddress = cfg.SMTPFromAddress
	resp.SMTPFromName = cfg.SMTPFromName
	resp.SMTPTLSMode = cfg.SMTPTLSMode
	resp.SMTPPasswordSet = cfg.SMTPPassword != ""
	if resp.SMTPPasswordSet {
		resp.SMTPPasswordMasked = maskKey(cfg.SMTPPassword)
	}
	resp.SMTPConfigured = smtpConfigFrom(cfg).Configured()
```

- [ ] **Step 4: Extend the request and `putSettingsHandler`**

Aggiungi a `updateSettingsRequest`:

```go
	SMTPHost        string `json:"smtpHost"`
	SMTPPort        int    `json:"smtpPort"`
	SMTPUsername    string `json:"smtpUsername"`
	SMTPPassword    string `json:"smtpPassword"`
	SMTPFromAddress string `json:"smtpFromAddress"`
	SMTPFromName    string `json:"smtpFromName"`
	SMTPTLSMode     string `json:"smtpTlsMode"`
```

Aggiungi il validatore del modo TLS, sopra `putSettingsHandler`:

```go
// normalizeTLSMode accetta il vuoto — l'admin che non ha ancora scelto —
// e lo lascia vuoto: e' mailer a trattarlo come STARTTLS. Un valore
// inventato invece e' un errore, perche' silenziosamente ripiegare su
// STARTTLS manderebbe la password su una connessione che l'admin credeva
// configurata diversamente.
func normalizeTLSMode(raw string) (string, bool) {
	switch mode := strings.ToLower(strings.TrimSpace(raw)); mode {
	case "", mailer.TLSModeSTARTTLS, mailer.TLSModeImplicit, mailer.TLSModeNone:
		return mode, true
	default:
		return "", false
	}
}
```

Aggiungi `"boardgames-manager/internal/mailer"` agli import. In `putSettingsHandler`, dopo la validazione di `aiBaseURL`:

```go
	tlsMode, ok := normalizeTLSMode(req.SMTPTLSMode)
	if !ok {
		writeError(w, http.StatusBadRequest, "la sicurezza SMTP deve essere starttls, tls o none")
		return
	}
```

e nel literal `next`, dopo i campi AI:

```go
		SMTPHost:        strings.TrimSpace(req.SMTPHost),
		SMTPPort:        req.SMTPPort,
		SMTPUsername:    strings.TrimSpace(req.SMTPUsername),
		SMTPPassword:    current.SMTPPassword,
		SMTPFromAddress: strings.TrimSpace(req.SMTPFromAddress),
		SMTPFromName:    strings.TrimSpace(req.SMTPFromName),
		SMTPTLSMode:     tlsMode,
```

e sotto il blocco che preserva `AIAPIKey`:

```go
	// Come le altre due credenziali: vuota vuol dire "lascia quella che
	// c'e'", perche' il form la rimanda vuota dopo ogni salvataggio.
	if req.SMTPPassword != "" {
		next.SMTPPassword = req.SMTPPassword
	}
```

- [ ] **Step 5: Add the shared config mapper**

Sempre in `settings_handlers.go`, in fondo al file:

```go
// smtpConfigFrom traduce le impostazioni salvate nella Config del package
// mailer. Vive qui perche' la usano sia la risposta di GET /api/settings
// (per smtpConfigured) sia la glue di invio.
func smtpConfigFrom(cfg settings.Settings) mailer.Config {
	return mailer.Config{
		Host:        cfg.SMTPHost,
		Port:        cfg.SMTPPort,
		Username:    cfg.SMTPUsername,
		Password:    cfg.SMTPPassword,
		FromAddress: cfg.SMTPFromAddress,
		FromName:    cfg.SMTPFromName,
		TLSMode:     cfg.SMTPTLSMode,
	}
}
```

- [ ] **Step 6: Run the tests to verify they pass**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -v
```

Expected: PASS, compresi i test settings preesistenti.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/httpapi/settings_handlers.go backend/internal/httpapi/settings_handlers_test.go
git commit -m "feat: expose the SMTP configuration through the settings API"
```

---

*(I task 5–15 continuano in questo stesso file: glue `httpapi/mail.go`, template, i quattro agganci, l'endpoint di prova, le rotte pubbliche, il pannello, i ritocchi UI, la documentazione e la verifica finale con `/impeccable`.)*

---

### Task 5: glue `httpapi/mail.go` e il finto mailer per i test

Il punto unico che decide *se* e *come* una mail parte. Nessun template e nessun contenuto: solo il sender, l'invio asincrono e la composizione dell'indirizzo pubblico.

**Files:**
- Create: `backend/internal/httpapi/mail.go`
- Create: `backend/internal/httpapi/mail_internal_test.go`
- Create: `backend/internal/httpapi/mail_fake_test.go`
- Modify: `backend/internal/httpapi/router.go` (campo `Mail` su `Server`)
- Modify: `backend/internal/httpapi/testhelpers_test.go` (helper `newTestServerWithMailer`)

**Interfaces:**
- Consumes: dal Task 1/2 `mailer.Sender`, `mailer.Message`, `mailer.ErrNotConfigured`, `mailer.NewSMTPSender`, `mailer.Config`; dal Task 4 `smtpConfigFrom(settings.Settings) mailer.Config`.
- Produces:
  - `Server.Mail mailer.Sender` (nil in produzione)
  - `func (s *Server) mailSender(ctx context.Context) mailer.Sender`
  - `func (s *Server) mailEnabled(ctx context.Context) bool`
  - `func (s *Server) sendMailAsync(sender mailer.Sender, m mailer.Message)`
  - `func (s *Server) publicBaseURL(r *http.Request) string`
  - test helper `newFakeMailer() *fakeMailer` con `waitForMail(t)`, `expectNoMail(t)`, campo `err`
  - test helper `newTestServerWithMailer(t, *fakeMailer) (*httpapi.Server, *sql.DB)`

- [ ] **Step 1: Write the failing test**

Crea `backend/internal/httpapi/mail_internal_test.go` — è un test *interno* (`package httpapi`), come `ratelimit_test.go`, perché verifica funzioni non esportate:

```go
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

	// Senza Settings non si puo' leggere l'indirizzo configurato: la
	// richiesta e' l'unica fonte, e non deve mai andare in panico.
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
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run 'TestPublicBaseURL|TestSMTPConfigFrom' -v
```

Expected: FAIL con `s.publicBaseURL undefined`.

- [ ] **Step 3: Write `mail.go`**

```go
package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"boardgames-manager/internal/mailer"
)

// mailSendTimeout e' il tetto su un singolo invio. Vive nella goroutine,
// non nella richiesta HTTP: il partecipante ha gia' avuto la sua risposta.
const mailSendTimeout = 20 * time.Second

// mailSender restituisce il sender per questa richiesta: quello iniettato
// se c'e' (i test), altrimenti uno costruito al volo dalle impostazioni
// salvate. Costruirlo per richiesta e' cio' che permette all'admin di
// cambiare provider senza riavviare il container, come per il traduttore.
func (s *Server) mailSender(ctx context.Context) mailer.Sender {
	if s.Mail != nil {
		return s.Mail
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		// Senza impostazioni non si manda niente; un sender vuoto
		// restituisce ErrNotConfigured, che e' l'esito giusto.
		log.Printf("mail: could not load settings: %v", err)
		return mailer.NewSMTPSender(mailer.Config{})
	}
	return mailer.NewSMTPSender(smtpConfigFrom(cfg))
}

// mailEnabled dice se una mail partira' davvero. Serve alle risposte che
// portano "mailQueued": promettere una mail che non partira' e' peggio
// che non prometterla.
func (s *Server) mailEnabled(ctx context.Context) bool {
	if s.Mail != nil {
		return true
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		return false
	}
	return smtpConfigFrom(cfg).Configured()
}

// sendMailAsync spedisce senza far aspettare la risposta HTTP. Il sender
// arriva risolto dal chiamante di proposito: costruirlo dentro la
// goroutine vorrebbe dire leggere le impostazioni con un context che la
// richiesta ha gia' chiuso.
//
// Nessun errore risale: una prenotazione, un annullamento e un invito
// riescono o falliscono per i loro motivi, mai per la posta.
// ErrNotConfigured non finisce nemmeno nei log — e' l'app senza SMTP, che
// e' una configurazione valida.
func (s *Server) sendMailAsync(sender mailer.Sender, m mailer.Message) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), mailSendTimeout)
		defer cancel()

		err := sender.Send(ctx, m)
		if err == nil || errors.Is(err, mailer.ErrNotConfigured) {
			return
		}
		log.Printf("mail to %s (%q) failed: %v", m.To, m.Subject, err)
	}()
}

// publicBaseURL e' la radice degli indirizzi che finiscono nelle mail,
// senza slash finale. Vince l'indirizzo pubblico configurato: chi riceve
// il link deve raggiungere l'app dal dominio dell'associazione, non da
// quello con cui il browser dell'admin sta navigando.
//
// Il ripiego sulla richiesta esiste perche' una mail non ha un browser su
// cui contare, e vale per l'installazione locale che non ha configurato
// niente. Si fida di X-Forwarded-Proto perche' il deploy previsto sta
// dietro il proprio reverse proxy; l'host invece resta quello della
// richiesta, e un'installazione esposta dovrebbe configurare l'indirizzo
// pubblico e non dipendere da questo ramo.
func (s *Server) publicBaseURL(r *http.Request) string {
	if s.Settings != nil {
		if cfg, err := s.Settings.Get(r.Context()); err == nil && cfg.PublicBaseURL != "" {
			return strings.TrimRight(cfg.PublicBaseURL, "/")
		}
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = strings.ToLower(strings.TrimSpace(strings.Split(proto, ",")[0]))
	}
	return scheme + "://" + r.Host
}
```

- [ ] **Step 4: Add the `Mail` field to `Server`**

In `backend/internal/httpapi/router.go`, dentro `type Server struct`, dopo il campo `AI`:

```go
	// Mail, quando e' valorizzato, e' il sender da usare. Lasciato a nil il
	// server ne costruisce uno per richiesta dalle impostazioni: come per
	// AI, cambiare provider non richiede un riavvio e i test possono
	// iniettare un finto. Nil e SMTP non configurato sono lo stesso caso
	// dal punto di vista di chi usa l'app: nessuna mail, tutto il resto
	// invariato.
	Mail mailer.Sender
```

e `"boardgames-manager/internal/mailer"` agli import.

- [ ] **Step 5: Run the tests to verify they pass**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run 'TestPublicBaseURL|TestSMTPConfigFrom' -v
```

Expected: PASS, quattro test.

- [ ] **Step 6: Write the fake mailer for the handler tests**

Crea `backend/internal/httpapi/mail_fake_test.go`:

```go
package httpapi_test

import (
	"context"
	"testing"
	"time"

	"boardgames-manager/internal/mailer"
)

// fakeMailer sta al posto del server SMTP nei test, come fakeTranslator
// sta al posto del provider AI.
//
// Le mail arrivano su un canale invece che su una slice perche' l'invio e'
// asincrono: senza il canale ogni test dovrebbe dormire un tempo
// arbitrario e sarebbe instabile.
type fakeMailer struct {
	sent chan mailer.Message
	// err, se valorizzato, e' l'errore che Send restituisce. Il messaggio
	// finisce comunque sul canale: serve a verificare che un guasto SMTP
	// non cambi l'esito HTTP dell'operazione.
	err error
}

func newFakeMailer() *fakeMailer {
	return &fakeMailer{sent: make(chan mailer.Message, 8)}
}

func (f *fakeMailer) Send(ctx context.Context, m mailer.Message) error {
	f.sent <- m
	return f.err
}

// waitForMail restituisce la prossima mail spedita, fallendo il test se
// non arriva. Due secondi sono un'eternita' per una goroutine locale e
// tengono il test stabile anche su una macchina carica.
func (f *fakeMailer) waitForMail(t *testing.T) mailer.Message {
	t.Helper()
	select {
	case m := <-f.sent:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("nessuna mail spedita entro 2s")
		return mailer.Message{}
	}
}

// expectNoMail verifica che non parta niente. La finestra e' breve di
// proposito: qui l'attesa e' il costo del test, non la sua correttezza.
func (f *fakeMailer) expectNoMail(t *testing.T) {
	t.Helper()
	select {
	case m := <-f.sent:
		t.Fatalf("nessuna mail attesa, spedita %q a %s", m.Subject, m.To)
	case <-time.After(300 * time.Millisecond):
	}
}
```

- [ ] **Step 7: Add the test server helper**

In `backend/internal/httpapi/testhelpers_test.go`, in fondo:

```go
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
```

- [ ] **Step 8: Run the whole suite to verify nothing regressed**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```

Expected: PASS ovunque. `newFakeMailer` e `newTestServerWithMailer` non sono ancora usati: se il compilatore si lamenta di un helper non usato, non lo fara' (sono funzioni, non variabili locali) — ma se `go vet` protesta, lascia il commit al task successivo che li usa.

- [ ] **Step 9: Commit**

```bash
git add backend/internal/httpapi/mail.go backend/internal/httpapi/mail_internal_test.go \
        backend/internal/httpapi/mail_fake_test.go backend/internal/httpapi/testhelpers_test.go \
        backend/internal/httpapi/router.go
git commit -m "feat: add the optional mail glue and its test doubles"
```

---

### Task 6: i tre template

Funzioni pure `(dati) → mailer.Message`, senza rete e senza database: prendono già risolti i dati che servono. Testabili sul contenuto, che è l'unica cosa che conta qui.

**Files:**
- Create: `backend/internal/httpapi/mail_templates.go`
- Test: `backend/internal/httpapi/mail_templates_test.go`

**Interfaces:**
- Consumes: dal Task 1 `mailer.Message`.
- Produces:
  - `type bookingMailData struct { ParticipantName, ParticipantEmail, BookingCode, GameLabel, EventTitle, EventDate, StartTime string; EventID int64; SharedTable bool }`
  - `func inviteMail(to, invitedBy, inviteURL string) mailer.Message`
  - `func bookingConfirmationMail(d bookingMailData, manageURL, scoreURL string) mailer.Message`
  - `func bookingCancelledMail(d bookingMailData, eventURL string, byAdmin bool) mailer.Message`
  - `func smtpTestMail(to string) mailer.Message`

- [ ] **Step 1: Write the failing test**

Crea `backend/internal/httpapi/mail_templates_test.go` (test interno, `package httpapi`):

```go
package httpapi

import (
	"strings"
	"testing"
)

func testBookingData() bookingMailData {
	return bookingMailData{
		ParticipantName:  "Mario Rossi",
		ParticipantEmail: "mario@example.com",
		BookingCode:      "ABC23XYZ",
		GameLabel:        "Catan #2",
		EventTitle:       "Serata giochi di settembre",
		EventDate:        "2026-09-18",
		StartTime:        "21:00",
		EventID:          7,
	}
}

// bothBodies e' la lista delle due parti: ogni cosa che deve arrivare al
// destinatario va verificata in entrambe, perche' un client di posta puo'
// mostrarne una sola.
func bothBodies(t *testing.T, subject, text, html string) []string {
	t.Helper()
	if strings.TrimSpace(subject) == "" {
		t.Error("oggetto vuoto")
	}
	if strings.TrimSpace(text) == "" {
		t.Error("parte testo vuota")
	}
	if !strings.Contains(html, "<") {
		t.Error("parte HTML senza markup")
	}
	return []string{text, html}
}

func TestInviteMail_CarriesTheLinkAndWhoInvited(t *testing.T) {
	m := inviteMail("nuovo@example.com", "admin@example.com", "https://giochi.example.org/invito/tok123")

	if m.To != "nuovo@example.com" {
		t.Errorf("To = %q", m.To)
	}
	for _, body := range bothBodies(t, m.Subject, m.TextBody, m.HTMLBody) {
		if !strings.Contains(body, "https://giochi.example.org/invito/tok123") {
			t.Errorf("link di invito assente:\n%s", body)
		}
		if !strings.Contains(body, "admin@example.com") {
			t.Errorf("chi ha invitato non e' nominato:\n%s", body)
		}
	}
}

func TestBookingConfirmationMail_CarriesCodeAndBothLinks(t *testing.T) {
	m := bookingConfirmationMail(testBookingData(),
		"https://giochi.example.org/prenotazione/ABC23XYZ",
		"https://giochi.example.org/prenotazione/ABC23XYZ/punteggio")

	if m.To != "mario@example.com" || m.ToName != "Mario Rossi" {
		t.Errorf("destinatario = %q / %q", m.To, m.ToName)
	}
	if !strings.Contains(m.Subject, "Catan #2") {
		t.Errorf("l'oggetto deve nominare il gioco: %q", m.Subject)
	}
	for _, body := range bothBodies(t, m.Subject, m.TextBody, m.HTMLBody) {
		for _, want := range []string{
			"ABC23XYZ",
			"Catan #2",
			"Serata giochi di settembre",
			"21:00",
			"https://giochi.example.org/prenotazione/ABC23XYZ",
			"https://giochi.example.org/prenotazione/ABC23XYZ/punteggio",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("manca %q:\n%s", want, body)
			}
		}
	}
}

func TestBookingConfirmationMail_MentionsASharedTableOnlyWhenItIsOne(t *testing.T) {
	d := testBookingData()

	solo := bookingConfirmationMail(d, "https://x/m", "https://x/s")
	if strings.Contains(solo.TextBody, "tavolo") && strings.Contains(solo.TextBody, "condiviso") {
		t.Error("con un tavolo non condiviso la nota non deve comparire")
	}

	d.SharedTable = true
	shared := bookingConfirmationMail(d, "https://x/m", "https://x/s")
	for _, body := range []string{shared.TextBody, shared.HTMLBody} {
		if !strings.Contains(body, "punteggio") || !strings.Contains(body, "tavolo") {
			t.Errorf("attesa la nota sul punteggio di tavolo:\n%s", body)
		}
	}
}

func TestBookingCancelledMail_DistinguishesWhoCancelled(t *testing.T) {
	byParticipant := bookingCancelledMail(testBookingData(), "https://giochi.example.org/events/7", false)
	byAdmin := bookingCancelledMail(testBookingData(), "https://giochi.example.org/events/7", true)

	if byParticipant.TextBody == byAdmin.TextBody {
		t.Fatal("le due varianti devono dire cose diverse: chi ha annullato cambia il senso della mail")
	}
	if !strings.Contains(byAdmin.TextBody, "organizzazione") {
		t.Errorf("la variante admin deve dire chi ha annullato:\n%s", byAdmin.TextBody)
	}
	for _, m := range []struct {
		name string
		msg  interface{ GetTo() string }
	}{} {
		_ = m
	}
	for _, body := range []string{
		byParticipant.TextBody, byParticipant.HTMLBody, byAdmin.TextBody, byAdmin.HTMLBody,
	} {
		if !strings.Contains(body, "https://giochi.example.org/events/7") {
			t.Errorf("manca il link all'evento per riprenotare:\n%s", body)
		}
		if !strings.Contains(body, "Catan #2") {
			t.Errorf("manca il gioco annullato:\n%s", body)
		}
	}
}

func TestSMTPTestMail_IsSelfExplanatory(t *testing.T) {
	m := smtpTestMail("admin@example.com")
	if m.To != "admin@example.com" {
		t.Errorf("To = %q", m.To)
	}
	bothBodies(t, m.Subject, m.TextBody, m.HTMLBody)
	if !strings.Contains(strings.ToLower(m.Subject), "prova") {
		t.Errorf("l'oggetto deve dire che e' una prova: %q", m.Subject)
	}
}

// Nessun template deve lasciare un placeholder non sostituito in giro.
func TestTemplates_LeaveNoPlaceholders(t *testing.T) {
	messages := []mailerMessageForTest{
		{"invito", inviteMail("a@b.org", "c@d.org", "https://x/invito/t")},
		{"conferma", bookingConfirmationMail(testBookingData(), "https://x/m", "https://x/s")},
		{"annullamento", bookingCancelledMail(testBookingData(), "https://x/e", true)},
		{"prova", smtpTestMail("a@b.org")},
	}
	for _, tc := range messages {
		for _, body := range []string{tc.m.Subject, tc.m.TextBody, tc.m.HTMLBody} {
			for _, bad := range []string{"%!", "%s", "%d", "<no value>", "{{"} {
				if strings.Contains(body, bad) {
					t.Errorf("%s: placeholder non sostituito %q in:\n%s", tc.name, bad, body)
				}
			}
		}
	}
}
```

Aggiungi in cima al file, sotto gli import, il tipo di appoggio usato dall'ultimo test:

```go
type mailerMessageForTest struct {
	name string
	m    mailerMessage
}
```

dove `mailerMessage` è un alias locale per tenere l'import corto:

```go
type mailerMessage = mailer.Message
```

con `"boardgames-manager/internal/mailer"` fra gli import. Rimuovi dal test `TestBookingCancelledMail_DistinguishesWhoCancelled` il ciclo vuoto `for _, m := range []struct{...}{}{}` — è rumore: cancellalo prima di eseguire.

- [ ] **Step 2: Run the test to verify it fails**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run 'Mail|Templates' -v
```

Expected: FAIL con `undefined: bookingMailData`, `undefined: inviteMail`, ecc.

- [ ] **Step 3: Write the shell of `mail_templates.go`**

```go
package httpapi

import (
	"fmt"
	"html"
	"strings"

	"boardgames-manager/internal/mailer"
)

// I contenuti delle mail, come funzioni pure: prendono dati gia' risolti e
// non toccano ne' rete ne' database. Cosi' il testo che il partecipante
// legge davvero e' verificabile in un test da un millisecondo.
//
// Ogni messaggio esce in due parti identiche nel contenuto: testo semplice
// e HTML. Chi legge in solo testo deve trovare codice e link per intero,
// non un invito a "guardare la versione HTML".
//
// L'HTML e' volutamente primitivo — tabelle, stili inline, nessuna
// immagine, nessun font esterno, 560px di larghezza massima — perche' i
// client di posta non sono browser. La palette e' quella di DESIGN.md.

const (
	mailColorFelt     = "#1f4d3a"
	mailColorCard     = "#faf6ec"
	mailColorCardAlt  = "#f2ead4"
	mailColorInk      = "#241f18"
	mailColorInkMuted = "#6e6250"
	mailColorAccent   = "#9c2b2b"
	mailColorLine     = "#ddd0ab"
)

// bookingMailData e' quello che serve alle due mail di prenotazione. I
// campi arrivano gia' composti: GameLabel porta il "#2" solo quando
// l'evento ha piu' copie di quel gioco, come fa la pagina pubblica, e
// SharedTable dice se il punteggio e' di tavolo.
type bookingMailData struct {
	ParticipantName  string
	ParticipantEmail string
	BookingCode      string
	GameLabel        string
	EventTitle       string
	EventDate        string
	StartTime        string
	EventID          int64
	SharedTable      bool
}
```

- [ ] **Step 4: Write the HTML shell helpers**

Ancora in `mail_templates.go`:

```go
// mailShell avvolge il corpo nella cornice comune: intestazione in feltro
// col nome dell'app, carta sotto, e nessun piede — non c'e' nulla da
// disiscrivere, sono tre mail transazionali.
func mailShell(heading, bodyHTML string) string {
	return `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f1ece0;margin:0;padding:24px 12px;">
<tr><td align="center">
<table role="presentation" width="560" cellpadding="0" cellspacing="0" style="width:100%;max-width:560px;border-collapse:collapse;">
<tr><td style="background:` + mailColorFelt + `;padding:18px 24px;border-radius:10px 10px 0 0;">
<span style="font:600 17px/1.3 'Space Grotesk',system-ui,sans-serif;color:#faf6ec;letter-spacing:-0.01em;">` +
		html.EscapeString(heading) + `</span>
</td></tr>
<tr><td style="background:` + mailColorCard + `;padding:24px;border-radius:0 0 10px 10px;border:1px solid ` + mailColorLine + `;border-top:0;font:15px/1.55 'IBM Plex Sans',system-ui,-apple-system,'Segoe UI',Roboto,sans-serif;color:` + mailColorInk + `;">` +
		bodyHTML + `</td></tr>
</table>
</td></tr></table>`
}

// mailButton e' un bottone che regge anche nei client che ignorano tutto:
// resta un link con uno sfondo.
func mailButton(label, url string) string {
	return `<table role="presentation" cellpadding="0" cellspacing="0" style="margin:8px 0;"><tr>
<td style="background:` + mailColorAccent + `;border-radius:6px;">
<a href="` + html.EscapeString(url) + `" style="display:inline-block;padding:11px 20px;color:#ffffff;text-decoration:none;font:600 15px/1 'IBM Plex Sans',system-ui,sans-serif;">` +
		html.EscapeString(label) + `</a>
</td></tr></table>`
}

func mailParagraph(text string) string {
	return `<p style="margin:0 0 14px;">` + html.EscapeString(text) + `</p>`
}

func mailNote(text string) string {
	return `<p style="margin:14px 0 0;font-size:13px;color:` + mailColorInkMuted + `;">` + html.EscapeString(text) + `</p>`
}

// mailCodeBlock mette il codice in evidenza in mono, come fa la scheda a
// schermo: e' l'unica cosa che il partecipante deve poter ritrovare in un
// colpo d'occhio.
func mailCodeBlock(code string) string {
	return `<table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 0 18px;"><tr>
<td style="background:` + mailColorCardAlt + `;border:1px solid ` + mailColorLine + `;border-radius:10px;padding:14px 20px;text-align:center;">
<div style="font-size:12px;letter-spacing:0.08em;text-transform:uppercase;color:` + mailColorInkMuted + `;margin-bottom:6px;">Il tuo codice</div>
<div style="font:600 26px/1.1 'IBM Plex Mono',ui-monospace,monospace;letter-spacing:0.1em;color:` + mailColorInk + `;">` +
		html.EscapeString(code) + `</div>
</td></tr></table>`
}

// mailFactRow e' una riga "etichetta: valore" del riepilogo evento.
func mailFactRow(label, value string) string {
	return `<tr>
<td style="padding:4px 12px 4px 0;color:` + mailColorInkMuted + `;font-size:13px;white-space:nowrap;">` + html.EscapeString(label) + `</td>
<td style="padding:4px 0;font-weight:600;">` + html.EscapeString(value) + `</td>
</tr>`
}
```

- [ ] **Step 5: Write the four templates**

```go
func inviteMail(to, invitedBy, inviteURL string) mailer.Message {
	text := strings.Join([]string{
		"Ciao,",
		"",
		fmt.Sprintf("%s ti ha aggiunto come amministratore di BoardGames Manager.", invitedBy),
		"",
		"Apri questo link per scegliere la tua password ed entrare:",
		inviteURL,
		"",
		"Il link e' personale e vale una volta sola: chi ti ha invitato non vedra' mai la password che scegli.",
	}, "\n")

	body := mailParagraph("Ciao,") +
		mailParagraph(fmt.Sprintf("%s ti ha aggiunto come amministratore di BoardGames Manager.", invitedBy)) +
		mailParagraph("Scegli la tua password ed entra:") +
		mailButton("Attiva il tuo accesso", inviteURL) +
		mailNote("Il link è personale e vale una volta sola: chi ti ha invitato non vedrà mai la password che scegli.")

	return mailer.Message{
		To:       to,
		Subject:  "Il tuo accesso da amministratore",
		TextBody: text,
		HTMLBody: mailShell("BoardGames Manager", body),
	}
}

func bookingConfirmationMail(d bookingMailData, manageURL, scoreURL string) mailer.Message {
	sharedNote := "Questo tavolo ha piu' posti prenotabili, uno a testa: il punteggio finale e' uno per tavolo, e chiunque sieda qui puo' inserirlo o correggerlo col proprio codice."

	lines := []string{
		fmt.Sprintf("Ciao %s,", d.ParticipantName),
		"",
		fmt.Sprintf("la tua prenotazione per %s e' confermata.", d.GameLabel),
		"",
		"Evento:   " + d.EventTitle,
		"Data:     " + d.EventDate,
		"Ora:      " + d.StartTime,
		"Gioco:    " + d.GameLabel,
		"Codice:   " + d.BookingCode,
		"",
		"Gestisci o annulla la prenotazione:",
		manageURL,
		"",
		"A fine partita, segna i punti:",
		scoreURL,
		"",
		"Conserva il codice: da solo basta a fare entrambe le cose.",
	}
	if d.SharedTable {
		lines = append(lines, "", sharedNote)
	}

	facts := `<table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 0 18px;border-collapse:collapse;">` +
		mailFactRow("Evento", d.EventTitle) +
		mailFactRow("Data", d.EventDate) +
		mailFactRow("Ora", d.StartTime) +
		mailFactRow("Gioco", d.GameLabel) +
		`</table>`

	body := mailParagraph(fmt.Sprintf("Ciao %s,", d.ParticipantName)) +
		mailParagraph(fmt.Sprintf("la tua prenotazione per %s è confermata.", d.GameLabel)) +
		facts +
		mailCodeBlock(d.BookingCode) +
		mailButton("Gestisci o annulla", manageURL) +
		mailButton("Segna i punti a fine partita", scoreURL) +
		mailNote("Conserva il codice: da solo basta a fare entrambe le cose.")
	if d.SharedTable {
		body += mailNote(sharedNote)
	}

	return mailer.Message{
		To:       d.ParticipantEmail,
		ToName:   d.ParticipantName,
		Subject:  fmt.Sprintf("Prenotazione confermata: %s — %s", d.GameLabel, d.EventDate),
		TextBody: strings.Join(lines, "\n"),
		HTMLBody: mailShell("Prenotazione confermata", body),
	}
}

func bookingCancelledMail(d bookingMailData, eventURL string, byAdmin bool) mailer.Message {
	opening := fmt.Sprintf("la tua prenotazione per %s e' stata annullata, come hai chiesto.", d.GameLabel)
	openingHTML := fmt.Sprintf("la tua prenotazione per %s è stata annullata, come hai chiesto.", d.GameLabel)
	closing := "Se hai cambiato idea puoi prenotare di nuovo, se restano posti."
	if byAdmin {
		opening = fmt.Sprintf("la tua prenotazione per %s e' stata annullata dall'organizzazione.", d.GameLabel)
		openingHTML = fmt.Sprintf("la tua prenotazione per %s è stata annullata dall'organizzazione.", d.GameLabel)
		closing = "Il posto e' tornato libero: puoi prenotare un altro gioco della serata."
	}

	text := strings.Join([]string{
		fmt.Sprintf("Ciao %s,", d.ParticipantName),
		"",
		opening,
		"",
		"Evento:   " + d.EventTitle,
		"Data:     " + d.EventDate,
		"Gioco:    " + d.GameLabel,
		"",
		closing,
		eventURL,
		"",
		"Il codice " + d.BookingCode + " non e' piu' valido.",
	}, "\n")

	facts := `<table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 0 18px;border-collapse:collapse;">` +
		mailFactRow("Evento", d.EventTitle) +
		mailFactRow("Data", d.EventDate) +
		mailFactRow("Gioco", d.GameLabel) +
		`</table>`

	body := mailParagraph(fmt.Sprintf("Ciao %s,", d.ParticipantName)) +
		mailParagraph(openingHTML) +
		facts +
		mailParagraph(closing) +
		mailButton("Vedi la serata", eventURL) +
		mailNote("Il codice " + d.BookingCode + " non è più valido.")

	return mailer.Message{
		To:       d.ParticipantEmail,
		ToName:   d.ParticipantName,
		Subject:  fmt.Sprintf("Prenotazione annullata: %s — %s", d.GameLabel, d.EventDate),
		TextBody: text,
		HTMLBody: mailShell("Prenotazione annullata", body),
	}
}

// smtpTestMail e' la mail del bottone "Invia email di prova": deve
// spiegarsi da sola a chi la trova in casella fra sei mesi.
func smtpTestMail(to string) mailer.Message {
	text := strings.Join([]string{
		"Se stai leggendo questa mail, la configurazione SMTP di BoardGames Manager funziona.",
		"",
		"Da qui in poi partiranno da sole: l'invito di un amministratore, la conferma di una prenotazione con il codice e i link, e l'avviso di annullamento.",
	}, "\n")

	body := mailParagraph("Se stai leggendo questa mail, la configurazione SMTP di BoardGames Manager funziona.") +
		mailParagraph("Da qui in poi partiranno da sole: l'invito di un amministratore, la conferma di una prenotazione con il codice e i link, e l'avviso di annullamento.")

	return mailer.Message{
		To:       to,
		Subject:  "Email di prova da BoardGames Manager",
		TextBody: text,
		HTMLBody: mailShell("Email di prova", body),
	}
}
```

- [ ] **Step 6: Run the tests to verify they pass**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run 'Mail|Templates' -v
```

Expected: PASS. Se `TestBookingConfirmationMail_MentionsASharedTableOnlyWhenItIsOne` fallisce sul caso non condiviso, controlla che nessun testo comune contenga insieme "tavolo" e "condiviso".

- [ ] **Step 7: Commit**

```bash
git add backend/internal/httpapi/mail_templates.go backend/internal/httpapi/mail_templates_test.go
git commit -m "feat: add the invite, booking and cancellation mail templates"
```

---

### Task 7: aggancio — invito amministratore

Il primo dei quattro agganci, e il più semplice: nessun dato da ricomporre, solo il token che l'handler ha già in mano.

**Files:**
- Modify: `backend/internal/httpapi/mail.go` (le funzioni che compongono gli URL)
- Modify: `backend/internal/httpapi/users_handlers.go`
- Test: `backend/internal/httpapi/users_handlers_test.go`

**Interfaces:**
- Consumes: dal Task 5 `mailSender`, `mailEnabled`, `sendMailAsync`, `publicBaseURL`, `newFakeMailer`, `newTestServerWithMailer`; dal Task 6 `inviteMail`.
- Produces:
  - `func inviteURL(base, token string) string`
  - `func bookingManageURL(base, code string) string`
  - `func bookingScoreURL(base, code string) string`
  - `func eventPublicURL(base string, eventID int64) string`
  - il campo `mailQueued` (bool) nella risposta di `POST /api/users`

- [ ] **Step 1: Write the failing test**

Aggiungi a `backend/internal/httpapi/users_handlers_test.go`:

```go
func TestCreateUser_SendsTheInviteWhenSMTPIsConfigured(t *testing.T) {
	mail := newFakeMailer()
	server, _ := newTestServerWithMailer(t, mail)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "capo@example.com", "supersecret1")

	// L'indirizzo pubblico configurato deve vincere sull'host della
	// richiesta: chi riceve l'invito raggiunge l'app dal dominio
	// dell'associazione, non da quello dell'admin.
	settingsPayload, _ := json.Marshal(map[string]any{
		"defaultLanguage": "it", "publicBaseUrl": "https://giochi.example.org",
	})
	setReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(settingsPayload))
	setReq.AddCookie(cookie)
	setRec := httptest.NewRecorder()
	router.ServeHTTP(setRec, setReq)
	if setRec.Code != http.StatusOK {
		t.Fatalf("put settings: %d %s", setRec.Code, setRec.Body.String())
	}

	payload, _ := json.Marshal(map[string]string{"email": "nuovo@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		InviteToken string `json:"inviteToken"`
		MailQueued  bool   `json:"mailQueued"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.MailQueued {
		t.Error("expected mailQueued to be true with SMTP configured")
	}
	// Il token continua a tornare: il bottone "Copia link" resta il
	// fallback quando la mail non arriva.
	if body.InviteToken == "" {
		t.Error("expected the invite token to keep coming back")
	}

	m := mail.waitForMail(t)
	if m.To != "nuovo@example.com" {
		t.Errorf("mail sent to %q", m.To)
	}
	wantLink := "https://giochi.example.org/invito/" + body.InviteToken
	if !strings.Contains(m.TextBody, wantLink) {
		t.Errorf("expected %q in the mail body:\n%s", wantLink, m.TextBody)
	}
	if !strings.Contains(m.TextBody, "capo@example.com") {
		t.Errorf("expected the inviter to be named:\n%s", m.TextBody)
	}
}

// Il vincolo globale, sul flusso invito: senza SMTP l'invito funziona
// esattamente come prima.
func TestCreateUser_WithoutSMTPStillMintsTheInvite(t *testing.T) {
	server := newTestServer(t) // Mail resta nil
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "capo@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"email": "nuovo@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		InviteToken string `json:"inviteToken"`
		MailQueued  bool   `json:"mailQueued"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.InviteToken == "" {
		t.Error("expected an invite token")
	}
	if body.MailQueued {
		t.Error("expected mailQueued to be false without SMTP: no mail will arrive")
	}
}

// Un guasto SMTP non deve trasformare un invito riuscito in un errore.
func TestCreateUser_SMTPFailureDoesNotBreakTheInvite(t *testing.T) {
	mail := newFakeMailer()
	mail.err = errors.New("connessione rifiutata")
	server, _ := newTestServerWithMailer(t, mail)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "capo@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"email": "nuovo@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 despite the SMTP failure, got %d: %s", rec.Code, rec.Body.String())
	}
	mail.waitForMail(t) // l'invio e' stato tentato
}
```

Aggiungi `"errors"` e `"strings"` agli import del file se non ci sono.

- [ ] **Step 2: Run the test to verify it fails**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run TestCreateUser -v
```

Expected: FAIL — `mailQueued` assente e nessuna mail spedita.

- [ ] **Step 3: Add the URL builders to `mail.go`**

In fondo a `backend/internal/httpapi/mail.go`:

```go
// Gli indirizzi che finiscono nelle mail. Stanno insieme perche' devono
// restare allineati alle rotte del frontend: cambiare un path in
// frontend/src/router/index.ts vuol dire cambiare qui.
//
// Ne' i token d'invito (hex) ne' i codici di prenotazione (lettere e cifre
// da un alfabeto senza caratteri ambigui) contengono caratteri da
// codificare, quindi la concatenazione basta e resta leggibile in una mail.

func inviteURL(base, token string) string {
	return base + "/invito/" + token
}

func bookingManageURL(base, code string) string {
	return base + "/prenotazione/" + code
}

func bookingScoreURL(base, code string) string {
	return base + "/prenotazione/" + code + "/punteggio"
}

func eventPublicURL(base string, eventID int64) string {
	return fmt.Sprintf("%s/events/%d", base, eventID)
}
```

Aggiungi `"fmt"` agli import di `mail.go`.

- [ ] **Step 4: Hook `createUserHandler`**

In `backend/internal/httpapi/users_handlers.go`, cambia il commento e la coda dell'handler. Il commento sopra `createUserHandler` diventa:

```go
// createUserHandler no longer accepts a password: whoever invites must not
// know another admin's. It mints an invite link, manda la mail se l'SMTP
// e' configurato, e in ogni caso restituisce il token — il bottone "Copia
// link" resta il modo di consegnare l'invito a mano, ed e' l'unico su
// un'installazione senza posta.
```

Sostituisci l'ultima riga (`writeJSON(w, http.StatusCreated, userResponse(user))`) con:

```go
	// La mail e' un extra: l'invito e' gia' valido e la risposta non
	// aspetta l'SMTP. mailQueued dice solo se l'invio e' stato affidato,
	// non se e' arrivato.
	queued := s.mailEnabled(r.Context())
	if queued {
		inviter := "Un amministratore"
		if actor, ok := currentUser(r); ok {
			inviter = actor.Email
		}
		link := inviteURL(s.publicBaseURL(r), token)
		s.sendMailAsync(s.mailSender(r.Context()), inviteMail(email, inviter, link))
	}

	resp := userResponse(user)
	resp["mailQueued"] = queued
	writeJSON(w, http.StatusCreated, resp)
```

- [ ] **Step 5: Run the tests to verify they pass**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run TestCreateUser -v
```

Expected: PASS, compresi i test `TestCreateUser*` preesistenti.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi/mail.go backend/internal/httpapi/users_handlers.go \
        backend/internal/httpapi/users_handlers_test.go
git commit -m "feat: email the invite link to a new admin"
```

---

### Task 8: aggancio — conferma di prenotazione

Il flusso che conta: il partecipante riceve codice e due link, e la POST non rallenta di un millisecondo.

**Files:**
- Modify: `backend/internal/httpapi/mail.go` (raccolta dati della prenotazione)
- Modify: `backend/internal/httpapi/events_bookings_handlers.go`
- Modify: `backend/internal/httpapi/events_responses.go` (`toBookingResponse` invariato; il campo si aggiunge nell'handler)
- Test: `backend/internal/httpapi/events_bookings_handlers_test.go`

**Interfaces:**
- Consumes: dal Task 6 `bookingMailData`, `bookingConfirmationMail`; dal Task 7 `bookingManageURL`, `bookingScoreURL`.
- Produces:
  - `func (s *Server) bookingMailDataFor(ctx context.Context, b events.Booking) (bookingMailData, error)`
  - `func (s *Server) sendBookingConfirmation(r *http.Request, b events.Booking)`
  - il campo `mailQueued` (bool) nella risposta di `POST /api/events/{id}/bookings`

- [ ] **Step 1: Write the failing test**

Aggiungi a `backend/internal/httpapi/events_bookings_handlers_test.go`:

```go
func TestCreateBooking_EmailsTheCodeAndBothLinks(t *testing.T) {
	mail := newFakeMailer()
	server, _ := newTestServerWithMailer(t, mail)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)

	payload, _ := json.Marshal(map[string]any{
		"eventGameId": eventGames[0].ID, "participantName": "Mario Rossi",
		"participantEmail": "mario@example.com", "participantPhone": "3331234567",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), bytes.NewReader(payload))
	req.Host = "giochi.local"
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		BookingCode string `json:"bookingCode"`
		MailQueued  bool   `json:"mailQueued"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.MailQueued {
		t.Error("expected mailQueued to be true with SMTP configured")
	}

	m := mail.waitForMail(t)
	if m.To != "mario@example.com" || m.ToName != "Mario Rossi" {
		t.Errorf("recipient = %q / %q", m.To, m.ToName)
	}
	for _, want := range []string{
		body.BookingCode,
		"Catan",
		"Serata giochi",
		"http://giochi.local/prenotazione/" + body.BookingCode,
		"http://giochi.local/prenotazione/" + body.BookingCode + "/punteggio",
	} {
		if !strings.Contains(m.TextBody, want) {
			t.Errorf("missing %q in the mail:\n%s", want, m.TextBody)
		}
	}
}

// Il vincolo globale, sul flusso prenotazione.
func TestCreateBooking_WithoutSMTPBehavesExactlyAsBefore(t *testing.T) {
	server := newTestServer(t) // Mail resta nil
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)

	payload, _ := json.Marshal(map[string]any{
		"eventGameId": eventGames[0].ID, "participantName": "Mario Rossi",
		"participantEmail": "mario@example.com", "participantPhone": "3331234567",
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), bytes.NewReader(payload)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		BookingCode string `json:"bookingCode"`
		MailQueued  bool   `json:"mailQueued"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.BookingCode) != 8 {
		t.Errorf("expected the code on screen as before, got %q", body.BookingCode)
	}
	if body.MailQueued {
		t.Error("expected mailQueued to be false without SMTP")
	}
}

// Un errore SMTP non deve annullare una prenotazione valida.
func TestCreateBooking_SMTPFailureKeepsTheBooking(t *testing.T) {
	mail := newFakeMailer()
	mail.err = errors.New("autenticazione rifiutata")
	server, _ := newTestServerWithMailer(t, mail)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)

	payload, _ := json.Marshal(map[string]any{
		"eventGameId": eventGames[0].ID, "participantName": "Mario Rossi",
		"participantEmail": "mario@example.com", "participantPhone": "3331234567",
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), bytes.NewReader(payload)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 despite the SMTP failure, got %d: %s", rec.Code, rec.Body.String())
	}
	mail.waitForMail(t)

	// E la prenotazione e' davvero nel database, non solo nella risposta.
	bookings, err := server.Events.ListBookingsForEvent(context.Background(), eventID)
	if err != nil {
		t.Fatalf("list bookings: %v", err)
	}
	if len(bookings) != 1 {
		t.Fatalf("expected 1 booking, got %d", len(bookings))
	}
}

// Con una copia sola il "#1" e' rumore, con due il numero serve: la mail
// segue la stessa regola della pagina pubblica.
func TestCreateBooking_MailLabelsTheCopyOnlyWithMoreThanOne(t *testing.T) {
	mail := newFakeMailer()
	server, _ := newTestServerWithMailer(t, mail)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 2)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)

	payload, _ := json.Marshal(map[string]any{
		"eventGameId": eventGames[1].ID, "participantName": "Mario Rossi",
		"participantEmail": "mario@example.com", "participantPhone": "3331234567",
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), bytes.NewReader(payload)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	m := mail.waitForMail(t)
	if !strings.Contains(m.TextBody, "Catan #2") {
		t.Errorf("expected 'Catan #2' with two copies in the event:\n%s", m.TextBody)
	}
}
```

Aggiungi `"errors"` agli import del file se non c'è.

- [ ] **Step 2: Run the test to verify it fails**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run TestCreateBooking -v
```

Expected: FAIL — nessuna mail e nessun `mailQueued`.

- [ ] **Step 3: Add the data gatherer to `mail.go`**

```go
// bookingMailDataFor raccoglie quello che le due mail di prenotazione
// devono dire. Ripercorre la stessa strada di toBookingDetailResponse
// perche' le due superfici devono raccontare la stessa prenotazione: se
// la pagina dice "Catan #2", la mail non puo' dire "Catan".
func (s *Server) bookingMailDataFor(ctx context.Context, b events.Booking) (bookingMailData, error) {
	event, err := s.Events.GetEvent(ctx, b.EventID)
	if err != nil {
		return bookingMailData{}, err
	}
	eventGame, err := s.Events.GetEventGame(ctx, b.EventGameID)
	if err != nil {
		return bookingMailData{}, err
	}
	game, err := s.Games.GetGame(ctx, eventGame.GameID)
	if err != nil {
		return bookingMailData{}, err
	}
	copies, err := s.Events.CountEventGameCopies(ctx, b.EventID, eventGame.GameID)
	if err != nil {
		return bookingMailData{}, err
	}

	// Il numero della copia serve solo quando l'evento porta piu' copie di
	// questo gioco, esattamente come in ManageBookingView.
	label := game.Name
	if copies > 1 {
		label = fmt.Sprintf("%s #%d", game.Name, eventGame.CopyIndex)
	}

	return bookingMailData{
		ParticipantName:  b.ParticipantName,
		ParticipantEmail: b.ParticipantEmail,
		BookingCode:      b.BookingCode,
		GameLabel:        label,
		EventTitle:       event.Title,
		EventDate:        event.EventDate,
		StartTime:        event.StartTime,
		EventID:          b.EventID,
		SharedTable:      eventGame.Seats > 1,
	}, nil
}

// sendBookingConfirmation manda la conferma, o non fa niente se non c'e'
// posta. Raccoglie i dati in modo sincrono — servono il context della
// richiesta e il database — e spedisce in modo asincrono.
//
// Un errore nel raccogliere i dati non risale: la prenotazione e' gia'
// fatta, e non mandare una mail e' meglio che rispondere con un errore
// per qualcosa che e' andato bene.
func (s *Server) sendBookingConfirmation(r *http.Request, b events.Booking) {
	if !s.mailEnabled(r.Context()) {
		return
	}
	data, err := s.bookingMailDataFor(r.Context(), b)
	if err != nil {
		log.Printf("mail: could not gather booking %d data: %v", b.ID, err)
		return
	}
	base := s.publicBaseURL(r)
	s.sendMailAsync(s.mailSender(r.Context()), bookingConfirmationMail(
		data,
		bookingManageURL(base, b.BookingCode),
		bookingScoreURL(base, b.BookingCode),
	))
}
```

Aggiungi `"boardgames-manager/internal/events"` agli import di `mail.go`.

- [ ] **Step 4: Hook `createBookingHandler`**

In `backend/internal/httpapi/events_bookings_handlers.go`, nel `default:` dello switch:

```go
	default:
		s.sendBookingConfirmation(r, booking)
		resp := toBookingResponse(booking)
		// mailQueued dice alla pagina se promettere una mail: senza SMTP
		// il codice a schermo e' l'unica cosa che il partecipante si porta
		// via, e la pagina lo dice cosi' com'e' sempre stato.
		resp["mailQueued"] = s.mailEnabled(r.Context())
		writeJSON(w, http.StatusCreated, resp)
```

- [ ] **Step 5: Run the tests to verify they pass**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run TestCreateBooking -v
```

Expected: PASS, compresi i quattro `TestCreateBooking*` preesistenti.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi/mail.go backend/internal/httpapi/events_bookings_handlers.go \
        backend/internal/httpapi/events_bookings_handlers_test.go
git commit -m "feat: email the booking code with direct manage and score links"
```

---

### Task 9: aggancio — i due annullamenti

Le due mail dicono cose diverse perché la situazione è diversa: chi ha annullato da sé vuole una ricevuta, chi si è visto cancellare il posto dall'organizzazione ha bisogno di saperlo.

**Files:**
- Modify: `backend/internal/httpapi/mail.go`
- Modify: `backend/internal/httpapi/events_bookings_handlers.go`
- Test: `backend/internal/httpapi/events_bookings_handlers_test.go`

**Interfaces:**
- Consumes: dal Task 6 `bookingCancelledMail`; dal Task 7 `eventPublicURL`; dal Task 8 `bookingMailDataFor`.
- Produces: `func (s *Server) sendBookingCancelled(r *http.Request, b events.Booking, byAdmin bool)`

- [ ] **Step 1: Write the failing test**

Aggiungi a `backend/internal/httpapi/events_bookings_handlers_test.go`:

```go
// bookForMailTest crea una prenotazione via API e restituisce id e codice,
// per i test che partono da una prenotazione esistente.
func bookForMailTest(t *testing.T, router http.Handler, eventID, eventGameID int64) (int64, string) {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{
		"eventGameId": eventGameID, "participantName": "Mario Rossi",
		"participantEmail": "mario@example.com", "participantPhone": "3331234567",
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), bytes.NewReader(payload)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create booking: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		ID          int64  `json:"id"`
		BookingCode string `json:"bookingCode"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode booking: %v", err)
	}
	return body.ID, body.BookingCode
}

func TestCancelBooking_EmailsTheParticipantAReceipt(t *testing.T) {
	mail := newFakeMailer()
	server, _ := newTestServerWithMailer(t, mail)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	bookingID, code := bookForMailTest(t, router, eventID, eventGames[0].ID)
	mail.waitForMail(t) // la conferma di prenotazione

	payload, _ := json.Marshal(map[string]string{"bookingCode": code})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/cancel", bookingID), bytes.NewReader(payload))
	req.Host = "giochi.local"
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	m := mail.waitForMail(t)
	if m.To != "mario@example.com" {
		t.Errorf("recipient = %q", m.To)
	}
	if !strings.Contains(strings.ToLower(m.Subject), "annullata") {
		t.Errorf("subject = %q", m.Subject)
	}
	if strings.Contains(m.TextBody, "organizzazione") {
		t.Errorf("a self-cancellation must not blame the organisers:\n%s", m.TextBody)
	}
	if !strings.Contains(m.TextBody, fmt.Sprintf("http://giochi.local/events/%d", eventID)) {
		t.Errorf("missing the event link to book again:\n%s", m.TextBody)
	}
}

func TestAdminCancelBooking_EmailsTheParticipantThatTheSeatIsFreed(t *testing.T) {
	mail := newFakeMailer()
	server, _ := newTestServerWithMailer(t, mail)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "capo@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	bookingID, _ := bookForMailTest(t, router, eventID, eventGames[0].ID)
	mail.waitForMail(t) // la conferma di prenotazione

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/bookings/%d", bookingID), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	m := mail.waitForMail(t)
	if m.To != "mario@example.com" {
		t.Errorf("recipient = %q", m.To)
	}
	if !strings.Contains(m.TextBody, "organizzazione") {
		t.Errorf("the participant must learn who cancelled:\n%s", m.TextBody)
	}
}

// Il vincolo globale, sui due annullamenti.
func TestCancelBooking_WithoutSMTPStillCancels(t *testing.T) {
	server := newTestServer(t) // Mail resta nil
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "capo@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 2)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)

	publicID, code := bookForMailTest(t, router, eventID, eventGames[0].ID)
	payload, _ := json.Marshal(map[string]string{"bookingCode": code})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/cancel", publicID), bytes.NewReader(payload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("public cancel: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	adminReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/bookings/%d", publicID), nil)
	adminReq.AddCookie(cookie)
	adminRec := httptest.NewRecorder()
	router.ServeHTTP(adminRec, adminReq)
	// Gia' annullata: 404, come prima di questa feature.
	if adminRec.Code != http.StatusNotFound {
		t.Fatalf("admin cancel of a cancelled booking: expected 404, got %d", adminRec.Code)
	}
}

// Una prenotazione che l'admin annulla e che non esiste piu' non deve
// mandare niente: non c'e' nessuno da avvisare.
func TestAdminCancelBooking_UnknownSendsNoMail(t *testing.T) {
	mail := newFakeMailer()
	server, _ := newTestServerWithMailer(t, mail)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "capo@example.com", "supersecret1")

	req := httptest.NewRequest(http.MethodDelete, "/api/bookings/999", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	mail.expectNoMail(t)
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run 'TestCancelBooking|TestAdminCancelBooking' -v
```

Expected: FAIL — nessuna mail di annullamento.

- [ ] **Step 3: Add the sender to `mail.go`**

```go
// sendBookingCancelled avvisa il partecipante. byAdmin cambia il testo,
// non il destinatario: la mail va sempre a chi aveva prenotato, ed e'
// nel caso dell'admin che serve davvero — e' l'unico modo in cui il
// partecipante scopre di non avere piu' il posto.
func (s *Server) sendBookingCancelled(r *http.Request, b events.Booking, byAdmin bool) {
	if !s.mailEnabled(r.Context()) {
		return
	}
	data, err := s.bookingMailDataFor(r.Context(), b)
	if err != nil {
		log.Printf("mail: could not gather cancelled booking %d data: %v", b.ID, err)
		return
	}
	s.sendMailAsync(s.mailSender(r.Context()), bookingCancelledMail(
		data,
		eventPublicURL(s.publicBaseURL(r), data.EventID),
		byAdmin,
	))
}
```

- [ ] **Step 4: Hook the public cancel**

In `cancelBookingHandler`, dopo il controllo degli errori di `CancelBooking` e prima di costruire la risposta:

```go
	s.sendBookingCancelled(r, booking, false)
```

- [ ] **Step 5: Hook the admin cancel**

`adminCancelBookingHandler` oggi scarta la prenotazione che riceve. Sostituisci il blocco:

```go
	booking, err := s.Events.AdminCancelBooking(r.Context(), id)
	if errors.Is(err, events.ErrNotFound) {
		writeError(w, http.StatusNotFound, "booking not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not cancel booking")
		return
	}
	// La prenotazione serve per sapere chi avvisare: e' l'unico modo in cui
	// il partecipante scopre che il suo posto e' stato liberato.
	s.sendBookingCancelled(r, booking, true)
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
```

`AdminCancelBooking` restituisce già `(Booking, error)` (`backend/internal/events/bookings.go:167`): l'handler la scartava, non serve toccare lo store.

- [ ] **Step 6: Run the tests to verify they pass**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```

Expected: PASS in tutti i package.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/httpapi/mail.go backend/internal/httpapi/events_bookings_handlers.go \
        backend/internal/httpapi/events_bookings_handlers_test.go backend/internal/events/
git commit -m "feat: email the participant when a booking is cancelled"
```

---

### Task 10: endpoint `POST /api/settings/smtp/test`

L'unico posto in cui un problema di posta si vede a schermo, perché è l'unico in cui l'admin l'ha chiesto. Invio **sincrono**: senza l'errore vero il bottone sarebbe inutile.

**Files:**
- Modify: `backend/internal/httpapi/settings_handlers.go`
- Modify: `backend/internal/httpapi/router.go`
- Test: `backend/internal/httpapi/settings_handlers_test.go`

**Interfaces:**
- Consumes: dal Task 5 `mailSender`, `mailSendTimeout`; dal Task 6 `smtpTestMail`; da `middleware_auth.go` `currentUser(r)`.
- Produces: `func (s *Server) testSMTPHandler(w http.ResponseWriter, r *http.Request)` montato su `POST /api/settings/smtp/test` con rate limit 5/minuto.

- [ ] **Step 1: Write the failing test**

Aggiungi a `backend/internal/httpapi/settings_handlers_test.go`:

```go
func TestSMTPTest_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/settings/smtp/test", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestSMTPTest_NotConfiguredReturns409(t *testing.T) {
	server := newTestServer(t) // Mail nil e nessuna impostazione SMTP
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	req := httptest.NewRequest(http.MethodPost, "/api/settings/smtp/test", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSMTPTest_SendsToTheLoggedInAdmin(t *testing.T) {
	mail := newFakeMailer()
	server, _ := newTestServerWithMailer(t, mail)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	req := httptest.NewRequest(http.MethodPost, "/api/settings/smtp/test", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		To string `json:"to"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.To != "admin@example.com" {
		t.Errorf("to = %q, atteso l'indirizzo dell'admin in sessione", body.To)
	}

	m := mail.waitForMail(t)
	if m.To != "admin@example.com" {
		t.Errorf("mail sent to %q", m.To)
	}
}

// A differenza di tutto il resto, qui l'errore SMTP deve arrivare a
// schermo: e' l'unico modo di capire che host, porta o password sono
// sbagliate senza aspettare una prenotazione vera.
func TestSMTPTest_FailureReturns502WithTheRealError(t *testing.T) {
	mail := newFakeMailer()
	mail.err = errors.New("autenticazione rifiutata: 535 credenziali non valide")
	server, _ := newTestServerWithMailer(t, mail)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	req := httptest.NewRequest(http.MethodPost, "/api/settings/smtp/test", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "535") {
		t.Errorf("expected the real SMTP error in the response, got: %s", rec.Body.String())
	}
}
```

Aggiungi `"errors"` e `"strings"` agli import del file se non ci sono.

- [ ] **Step 2: Run the test to verify it fails**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run TestSMTPTest -v
```

Expected: FAIL — la rotta non esiste (404 invece di 409/200).

- [ ] **Step 3: Write the handler**

In fondo a `backend/internal/httpapi/settings_handlers.go`:

```go
// testSMTPHandler manda una mail all'admin in sessione e riferisce
// l'esito. E' l'unica eccezione al silenzio sugli errori di posta: qui
// l'admin ha premuto un bottone, e un guasto muto lo lascerebbe a
// indovinare fra host, porta, cifratura e password.
//
// Prova la configurazione salvata, non quella nel form: e' quella che
// verra' usata davvero, e la UI dice di salvare prima.
func (s *Server) testSMTPHandler(w http.ResponseWriter, r *http.Request) {
	admin, ok := currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), mailSendTimeout)
	defer cancel()

	err := s.mailSender(ctx).Send(ctx, smtpTestMail(admin.Email))
	if errors.Is(err, mailer.ErrNotConfigured) {
		writeError(w, http.StatusConflict, "SMTP non configurato: compila server, porta e indirizzo mittente, poi salva")
		return
	}
	if err != nil {
		log.Printf("smtp test send to %s: %v", admin.Email, err)
		// Il messaggio del provider esce cosi' com'e': e' rivolto a un
		// admin autenticato, ed e' l'informazione che serve.
		writeError(w, http.StatusBadGateway, "invio non riuscito: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "sent", "to": admin.Email})
}
```

Aggiungi agli import di `settings_handlers.go`: `"context"`, `"errors"`, `"log"`.

- [ ] **Step 4: Mount the route**

In `backend/internal/httpapi/router.go`, accanto agli altri limiter:

```go
	// Un invio di prova apre una connessione verso un server esterno: cinque
	// al minuto bastano a sistemare una configurazione sbagliata e non
	// trasformano il bottone in un modo di mandare mail a raffica.
	smtpTestLimiter := newRateLimiter(5, time.Minute)
```

e dentro il gruppo protetto, dopo `protected.Put("/api/settings", ...)`:

```go
		protected.With(smtpTestLimiter.middleware).Post("/api/settings/smtp/test", s.testSMTPHandler)
```

- [ ] **Step 5: Run the tests to verify they pass**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -v
```

Expected: PASS in tutto il package.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi/settings_handlers.go backend/internal/httpapi/router.go \
        backend/internal/httpapi/settings_handlers_test.go
git commit -m "feat: add an SMTP test-send endpoint for the settings panel"
```

---

### Task 11: rotte pubbliche con deep-link

I due link della mail devono aprire la pagina già risolta, senza chiedere il codice a chi ce l'ha appena consegnato.

**Files:**
- Modify: `frontend/src/router/index.ts`
- Modify: `frontend/src/views/ManageBookingView.vue`

**Interfaces:**
- Consumes: dal Task 8 gli URL `/prenotazione/:code` e `/prenotazione/:code/punteggio` che le mail compongono.
- Produces: le rotte `booking-manage` e `booking-score`; `ManageBookingView` accetta le prop `code?: string` e `mode?: 'manage' | 'score'`.

- [ ] **Step 1: Add the two routes**

In `frontend/src/router/index.ts`, subito dopo la rotta `manage-booking`:

```ts
    // I due link che partono nella mail di conferma. Stesso componente di
    // /manage-booking: la pagina sa già fare entrambe le cose, e il path
    // decide solo se il codice arriva dall'indirizzo o dal form.
    // `props` è una funzione perché il codice viene dai params mentre
    // `mode` è fisso per rotta.
    {
      path: '/prenotazione/:code',
      name: 'booking-manage',
      component: ManageBookingView,
      props: (route) => ({ code: String(route.params.code), mode: 'manage' }),
      meta: { public: true },
    },
    {
      path: '/prenotazione/:code/punteggio',
      name: 'booking-score',
      component: ManageBookingView,
      props: (route) => ({ code: String(route.params.code), mode: 'score' }),
      meta: { public: true },
    },
```

- [ ] **Step 2: Accept the props in `ManageBookingView.vue`**

In cima al blocco `<script setup>`, dopo gli import, aggiungi:

```ts
/**
 * `code` arriva dai link mandati per mail: quando c'è, la pagina si
 * risolve da sé e il form del codice non compare — chiederlo a chi ha
 * appena cliccato un link che lo contiene sarebbe un passaggio in più
 * davanti a un tavolo di gioco.
 *
 * `mode` dice su cosa aprire: 'score' è il link "segna i punti a fine
 * partita", che porta direttamente al form del punteggio.
 */
const props = withDefaults(
  defineProps<{ code?: string; mode?: 'manage' | 'score' }>(),
  { code: '', mode: 'manage' },
)
```

Aggiorna la riga di import per prendere anche `onMounted`:

```ts
import { computed, onMounted, ref, nextTick } from 'vue'
```

- [ ] **Step 3: Resolve on mount and adjust the error copy**

Aggiungi, dopo la dichiarazione di `players`:

```ts
/** Vero quando il codice arriva dall'indirizzo invece che dal form. */
const deepLinked = computed(() => props.code !== '')
const scoreSection = ref<HTMLElement | null>(null)
```

Sostituisci il `catch` di `lookup()` con:

```ts
  } catch (e) {
    booking.value = null
    // Chi arriva da un link non ha sbagliato a digitare: il codice è
    // quello che gli abbiamo mandato noi. Se non risolve, la
    // prenotazione è stata annullata — LookupBooking cerca solo fra le
    // attive — e dirlo così evita di far sembrare un guasto nostro.
    error.value = deepLinked.value
      ? 'Questa prenotazione non è più attiva, o il link non è più valido.'
      : (e as Error).message
  }
```

e aggiungi in fondo al blocco `<script setup>`:

```ts
onMounted(async () => {
  if (!deepLinked.value) {
    return
  }
  bookingCode.value = props.code
  await lookup()
  if (props.mode === 'score' && booking.value?.status === 'active') {
    await nextTick()
    scoreSection.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }
})
```

- [ ] **Step 4: Hide the code form and mark the score section**

Nel `<template>`, avvolgi il form di ricerca in modo che compaia solo senza deep-link:

```vue
      <form v-if="!deepLinked" @submit.prevent="lookup">
        <label>
          Codice prenotazione
          <input v-model="bookingCode" required />
        </label>
        <button type="submit">Cerca</button>
      </form>
```

e aggiungi il riferimento al form del punteggio, sulla riga del `<form v-if="booking.status === 'active'" @submit.prevent="submitScore">`:

```vue
        <form
          v-if="booking.status === 'active'"
          ref="scoreSection"
          @submit.prevent="submitScore"
        >
```

- [ ] **Step 5: Type-check and build**

```bash
cd frontend && npm run build
```

Expected: build riuscita, nessun errore `vue-tsc`. Se `ref="scoreSection"` dà un errore di tipo su un `<form>`, dichiara il ref come `ref<HTMLFormElement | null>(null)`.

- [ ] **Step 6: Verify the three routes by hand**

```bash
cd .. && docker compose up -d --build
```

Poi, su http://localhost:8080:
1. prenota un gioco su un evento e copia il codice dalla conferma
2. apri `http://localhost:8080/prenotazione/<CODICE>` — la pagina deve mostrare la prenotazione senza chiedere niente, e senza il campo "Codice prenotazione"
3. apri `http://localhost:8080/prenotazione/<CODICE>/punteggio` — stessa pagina, con lo scroll sul form del punteggio
4. apri `http://localhost:8080/manage-booking` — deve ancora chiedere il codice come prima
5. apri `http://localhost:8080/prenotazione/XXXXXXXX` — deve dire "Questa prenotazione non è più attiva, o il link non è più valido."

- [ ] **Step 7: Commit**

```bash
git add frontend/src/router/index.ts frontend/src/views/ManageBookingView.vue
git commit -m "feat: open a booking straight from an emailed link"
```

---

### Task 12: pannello "Configurazione Email (SMTP)"

**Files:**
- Modify: `frontend/src/views/SettingsView.vue`

**Interfaces:**
- Consumes: dal Task 4 i campi `smtp*` di `GET`/`PUT /api/settings`; dal Task 10 `POST /api/settings/smtp/test`.
- Produces: niente per altri task.

- [ ] **Step 1: Extend the response interface and the refs**

In `frontend/src/views/SettingsView.vue`, aggiungi a `interface SettingsResponse`:

```ts
  smtpHost: string
  smtpPort: number
  smtpUsername: string
  smtpFromAddress: string
  smtpFromName: string
  smtpTlsMode: string
  smtpPasswordSet: boolean
  smtpPasswordMasked?: string
  smtpConfigured: boolean
```

e sotto i ref esistenti:

```ts
const smtpHost = ref('')
// 587 come valore di partenza: e' la porta di Gmail, Mailjet e Brevo, cioe'
// di quasi ogni provider che un'associazione userebbe.
const smtpPort = ref<number>(587)
const smtpUsername = ref('')
const smtpPassword = ref('')
const smtpPasswordMasked = ref('')
const smtpFromAddress = ref('')
const smtpFromName = ref('')
const smtpTlsMode = ref('starttls')
const smtpConfigured = ref(false)
const smtpTesting = ref(false)
const smtpTestMessage = ref('')
const smtpTestError = ref('')
```

- [ ] **Step 2: Read and write them in `load()` and `save()`**

In `load()`, dopo le righe AI:

```ts
  smtpHost.value = s.smtpHost || ''
  smtpPort.value = s.smtpPort || 587
  smtpUsername.value = s.smtpUsername || ''
  smtpPasswordMasked.value = s.smtpPasswordMasked || ''
  smtpFromAddress.value = s.smtpFromAddress || ''
  smtpFromName.value = s.smtpFromName || ''
  smtpTlsMode.value = s.smtpTlsMode || 'starttls'
  smtpConfigured.value = s.smtpConfigured
```

Nel corpo di `api.put('/settings', {...})`, dopo `aiModel`:

```ts
      smtpHost: smtpHost.value,
      smtpPort: smtpPort.value,
      smtpUsername: smtpUsername.value,
      smtpPassword: smtpPassword.value,
      smtpFromAddress: smtpFromAddress.value,
      smtpFromName: smtpFromName.value,
      smtpTlsMode: smtpTlsMode.value,
```

e subito dopo `aiApiKey.value = ''`:

```ts
    smtpPassword.value = ''
    // Un salvataggio cambia la configurazione che la prova userebbe:
    // l'esito precedente non vale piu' e resta a schermo mentendo.
    smtpTestMessage.value = ''
    smtpTestError.value = ''
```

- [ ] **Step 3: Add the test-send action**

Dopo la funzione `save()`:

```ts
/**
 * Prova la configurazione salvata, non quella nel form: e' quella che
 * verra' usata davvero quando parte una prenotazione. La mail arriva
 * all'indirizzo dell'admin in sessione.
 */
async function sendTestEmail() {
  smtpTesting.value = true
  smtpTestMessage.value = ''
  smtpTestError.value = ''
  try {
    const res = await api.post<{ to: string }>('/settings/smtp/test')
    smtpTestMessage.value = `Email di prova inviata a ${res.to}. Se non arriva, controlla la casella spam.`
  } catch (e) {
    smtpTestError.value = (e as Error).message
  } finally {
    smtpTesting.value = false
  }
}
```

- [ ] **Step 4: Add the panel to the template**

Nel `<template>`, dopo il `panel-card` "Provider AI" e prima di `<div class="form-actions">`:

```vue
      <div class="panel-card">
        <div class="section-head">
          <h2>Configurazione Email (SMTP)</h2>
        </div>
        <p class="field-hint">
          Se lo configuri, l'app manda da sé l'invito di un amministratore, la
          conferma di una prenotazione — con il codice e i link per disdire o
          segnare i punti — e l'avviso di annullamento. Lasciandolo vuoto
          funziona come prima: il codice resta solo a schermo e il link di
          invito si copia a mano.
        </p>

        <label>
          Server SMTP
          <input v-model="smtpHost" placeholder="smtp.gmail.com" autocomplete="off" />
        </label>

        <div class="field-row">
          <label>
            Porta
            <input v-model.number="smtpPort" type="number" min="1" max="65535" inputmode="numeric" />
          </label>
          <label>
            Sicurezza
            <select v-model="smtpTlsMode">
              <option value="starttls">STARTTLS (porta 587)</option>
              <option value="tls">TLS (porta 465)</option>
              <option value="none">Nessuna</option>
            </select>
          </label>
        </div>

        <label>
          Nome utente
          <input v-model="smtpUsername" autocomplete="off" placeholder="serate@example.org" />
        </label>

        <label>
          Password
          <input
            v-model="smtpPassword"
            type="password"
            autocomplete="new-password"
            :placeholder="smtpPasswordMasked || 'non configurata'"
          />
        </label>
        <p class="field-hint">
          Con Gmail serve una <strong>app password</strong> generata dal tuo
          account Google (richiede la verifica in due passaggi), non la password
          con cui accedi. Con Mailjet: <code>in-v3.mailjet.com</code>, nome
          utente = API key e password = secret key.
        </p>

        <label>
          Indirizzo mittente
          <input v-model="smtpFromAddress" type="email" inputmode="email" placeholder="serate@example.org" />
        </label>

        <label>
          Nome mittente
          <input v-model="smtpFromName" placeholder="Serate Ludiche" />
        </label>
        <p class="field-hint">
          È il nome che chi riceve legge nella casella, accanto all'indirizzo.
        </p>

        <div class="smtp-test">
          <button
            type="button"
            class="btn-secondary"
            :disabled="!smtpConfigured || smtpTesting"
            @click="sendTestEmail"
          >
            {{ smtpTesting ? 'Invio…' : 'Invia email di prova' }}
          </button>
          <p class="field-hint">
            <template v-if="smtpConfigured">
              Manda una mail al tuo indirizzo usando la configurazione salvata.
            </template>
            <template v-else>
              Compila server, porta e indirizzo mittente, poi salva: la prova usa
              la configurazione salvata.
            </template>
          </p>
        </div>
        <p v-if="smtpTestMessage" class="success">{{ smtpTestMessage }}</p>
        <p v-if="smtpTestError" class="error">{{ smtpTestError }}</p>
      </div>
```

- [ ] **Step 5: Add the two styles the panel needs**

`field-row` e `smtp-test` sono pattern nuovi: aggiungili a `frontend/src/app.css`, accanto agli altri stili di form, e annota il pattern in `DESIGN.md` al Task 14.

```css
/* Due campi corti sulla stessa riga: porta e sicurezza si leggono come
   una cosa sola, e da soli sprecherebbero mezza larghezza ciascuno. Su
   schermo stretto tornano incolonnati. */
.field-row {
  display: flex;
  gap: var(--space-md, 1rem);
  flex-wrap: wrap;
}

.field-row > label {
  flex: 1 1 12rem;
}

/* Il bottone di prova e la sua spiegazione: un'azione secondaria dentro un
   form, separata dal bottone di salvataggio che sta in fondo alla pagina. */
.smtp-test {
  display: flex;
  align-items: center;
  gap: var(--space-md, 1rem);
  flex-wrap: wrap;
  margin-top: var(--space-md, 1rem);
}

.smtp-test .field-hint {
  flex: 1 1 16rem;
  margin: 0;
}
```

Verifica i nomi delle variabili di spaziatura già in uso in `app.css` (`grep -n "space-md\|--space" frontend/src/app.css`) e usa quelli invece dei fallback se esistono.

- [ ] **Step 6: Type-check and build**

```bash
cd frontend && npm run build
```

Expected: build riuscita.

- [ ] **Step 7: Verify by hand against a real provider**

```bash
cd .. && docker compose up -d --build
```

Su http://localhost:8080/admin/settings:
1. il pannello compare, il bottone di prova è disabilitato con l'hint "compila… poi salva"
2. metti host, porta, sicurezza, utente, app password, mittente e nome, salva
3. il bottone si abilita; premilo e verifica che la mail arrivi
4. sbaglia la password di proposito, salva e riprova: l'errore SMTP vero deve comparire sotto il bottone
5. svuota server e mittente, salva: nessun errore, e il bottone torna disabilitato

- [ ] **Step 8: Commit**

```bash
git add frontend/src/views/SettingsView.vue frontend/src/app.css
git commit -m "feat: add the SMTP settings panel with a test send"
```

---

### Task 13: dire la verità sulle mail nelle due pagine

Due ritocchi piccoli e obbligatori: senza, la UI promette mail che su un'istanza senza SMTP non arrivano.

**Files:**
- Modify: `frontend/src/components/BookingConfirmation.vue`
- Modify: `frontend/src/views/EventDetailView.vue`
- Modify: `frontend/src/views/UsersView.vue`

**Interfaces:**
- Consumes: dal Task 7 e 8 il campo `mailQueued` nelle risposte di `POST /api/users` e `POST /api/events/{id}/bookings`.
- Produces: niente per altri task.

- [ ] **Step 1: Add the prop to `BookingConfirmation.vue`**

Sostituisci il blocco `<script setup>` con:

```ts
<script setup lang="ts">
/**
 * L'esito di una prenotazione. Il codice compare due volte — dentro la
 * modale appena confermata e nel riepilogo in cima alla pagina — e questo
 * componente tiene le due copie identiche.
 *
 * `hint` porta la riga "conservalo per...": vera nella modale, falsa nel
 * riepilogo, dove la nota si dice una volta sotto tutti i codici invece di
 * ripetersi identica sotto ognuno.
 *
 * `mailed` dice se una mail col codice e i link e' partita davvero: su
 * un'istanza senza SMTP configurato non parte niente, e il codice a
 * schermo resta l'unica cosa che il partecipante si porta via — prometterne
 * una che non arriva sarebbe il modo piu' rapido di fargli perdere il
 * codice.
 */
withDefaults(
  defineProps<{
    gameLabel: string
    code: string
    multiSeat: boolean
    hint?: boolean
    mailed?: boolean
  }>(),
  { hint: true, mailed: false },
)
</script>
```

e aggiungi nel template, subito dopo il `<p v-if="hint">`:

```vue
    <p v-if="mailed" class="booking-mailed">
      Ti abbiamo mandato una mail con il codice e i link per annullare o segnare
      i punti.
    </p>
```

- [ ] **Step 2: Pass `mailed` from the page that books**

`EventDetailView.vue` usa `BookingConfirmation` in due punti — nella modale appena confermata e nel riepilogo in cima al tavolo — e conserva una prenotazione per gioco in `confirmed`. Quindi `mailed` va per prenotazione, non per pagina.

Estendi le due interfacce (righe ~40–51):

```ts
interface BookingResult {
  id: number
  bookingCode: string
  mailQueued: boolean
}

/** Una prenotazione andata a buon fine in questa visita alla pagina. */
interface ConfirmedBooking {
  code: string
  label: string
  multiSeat: boolean
  /** Se per questa prenotazione è partita davvero una mail. */
  mailed: boolean
}
```

In `submitBooking()`, aggiungi il campo al push:

```ts
    confirmed.value.push({
      code: result.bookingCode,
      label: selectedLabel.value,
      multiSeat: !!selectedGame.value && selectedGame.value.seats > 1,
      mailed: result.mailQueued,
    })
```

Nel riepilogo, passa il valore per riga:

```vue
        <BookingConfirmation
          v-for="b in confirmed"
          :key="b.code"
          :game-label="b.label"
          :code="b.code"
          :multi-seat="b.multiSeat"
          :hint="false"
          :mailed="b.mailed"
        />
```

e nella modale:

```vue
        <BookingConfirmation
          :game-label="selectedLabel"
          :code="bookingResult.bookingCode"
          :multi-seat="!!selectedGame && selectedGame.seats > 1"
          :mailed="bookingResult.mailQueued"
        />
```

Il `<p class="recap-hint">` sotto il riepilogo resta com'è: dice di conservare il codice, che vale con o senza mail. La riga sulla mail la aggiunge già `BookingConfirmation` per ogni prenotazione che l'ha ricevuta.

- [ ] **Step 3: Say whether the invite was emailed, in `UsersView.vue`**

Aggiungi il ref e usalo dopo l'invito:

```ts
// Quale invito e' stato anche mandato per mail: senza SMTP resta vuoto e la
// pagina si comporta come prima, col solo link da copiare.
const mailedTo = ref('')
```

In `invite()`, sostituisci il corpo del `try`:

```ts
  try {
    const created = await api.post<AdminUser & { mailQueued: boolean }>('/users', {
      email: newEmail.value,
    })
    mailedTo.value = created.mailQueued ? created.email : ''
    cancelAdding()
    await loadUsers()
  } catch (e) {
```

e in `startAdding()`, con gli altri azzeramenti:

```ts
  mailedTo.value = ''
```

Nel template, subito dopo `<p v-if="error" class="error">{{ error }}</p>`:

```vue
    <p v-if="mailedTo" class="success">
      Invito inviato a {{ mailedTo }}. Il link resta qui sotto, se serve
      consegnarlo a mano.
    </p>
```

- [ ] **Step 4: Type-check and build**

```bash
cd frontend && npm run build
```

Expected: build riuscita.

- [ ] **Step 5: Verify both states by hand**

```bash
cd .. && docker compose up -d --build
```

Con SMTP configurato: invita un admin (compare "Invito inviato a…") e prenota un gioco (compare la riga sulla mail). Poi svuota il pannello SMTP, salva, e ripeti: **nessuna** delle due righe deve comparire, e tutto il resto deve funzionare identico.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/BookingConfirmation.vue frontend/src/views/EventDetailView.vue \
        frontend/src/views/UsersView.vue
git commit -m "feat: tell the user when a mail was actually sent"
```

---

### Task 14: documentazione

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `DESIGN.md`

- [ ] **Step 1: Document SMTP in the README**

Aggiungi una sezione "Email (SMTP)" accanto a quella del provider AI, dicendo:

- che è **opzionale**, e cosa fa l'app senza (codice a schermo, link di invito da copiare)
- le tre mail che parte quando c'è
- dove si configura (Impostazioni → Configurazione Email) e che i valori stanno nel database, non in variabili d'ambiente
- Gmail: `smtp.gmail.com`, porta 587, STARTTLS, nome utente = l'indirizzo, password = **app password** generata da Google con la verifica in due passaggi attiva
- Mailjet: `in-v3.mailjet.com`, porta 587, nome utente = API key, password = secret key
- che i link nelle mail usano l'**Indirizzo pubblico** delle impostazioni, e che senza quello ripiegano sull'host della richiesta — quindi va configurato su un'installazione raggiungibile da fuori
- il bottone "Invia email di prova" e che manda all'indirizzo dell'admin in sessione

- [ ] **Step 2: Correct the stale decision in `CLAUDE.md`**

Nella sezione *Prenotazioni & punteggi*, la riga

> Alla prenotazione si genera un `booking_code` mostrato a schermo.
> **Nessuna email inviata** (niente SMTP in v1).

va sostituita con la decisione vera:

```markdown
- Alla prenotazione si genera un `booking_code` mostrato a schermo.
  L'invio email è **opzionale**: configurando un server SMTP nelle
  impostazioni partono la conferma di prenotazione (col codice e i link
  diretti a disdetta e punteggi), l'invito di un amministratore e
  l'avviso di annullamento. Senza SMTP l'app funziona per intero come
  prima — il codice resta a schermo e il link d'invito si copia a mano.
  Vale la stessa regola del provider AI: nessun campo obbligatorio,
  nessun errore in UI perché la posta manca.
```

- [ ] **Step 3: Note the new patterns in `DESIGN.md`**

Aggiungi due voci:

- **Email transazionali** come superficie nuova: tabelle, stili inline, max 560px, nessuna immagine e nessun font esterno, palette del sistema (feltro per l'intestazione, carta per il corpo, accento per i bottoni, mono per il codice). Il testo semplice è la versione di riferimento, non un ripiego.
- I due componenti CSS nuovi `field-row` (due campi corti in linea) e `smtp-test` (azione secondaria dentro un form, con la sua spiegazione accanto).

- [ ] **Step 4: Commit**

```bash
git add README.md CLAUDE.md DESIGN.md
git commit -m "docs: document the optional SMTP configuration"
```

---

### Task 15: verifica finale e passaggio `impeccable`

**Files:** nessuno da modificare a priori; le correzioni che emergono dal pass `impeccable` toccano `SettingsView.vue`, `ManageBookingView.vue` e `app.css`.

- [ ] **Step 1: Run the whole backend suite**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```

Expected: PASS in ogni package. Incolla l'output nel resoconto: senza, non si dichiara niente finito.

- [ ] **Step 2: Run `go vet`**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go vet ./...
```

Expected: nessun output.

- [ ] **Step 3: Confirm no new dependency slipped in**

```bash
git diff main --stat -- backend/go.mod backend/go.sum
```

Expected: nessuna riga.

- [ ] **Step 4: Build the frontend**

```bash
cd frontend && npm run build && cd ..
```

Expected: build riuscita, nessun errore `vue-tsc`.

- [ ] **Step 5: Walk the invariant end to end with SMTP off**

```bash
docker compose up -d --build
```

Con il pannello SMTP **vuoto**, su http://localhost:8080:
1. prenota un gioco → il codice compare a schermo, nessuna riga che promette una mail
2. apri `/manage-booking`, inserisci il codice, annulla la prenotazione → funziona
3. invita un admin da `/admin/users` → il link compare e si copia
4. in `/admin/settings` il bottone di prova è disabilitato e nessun errore è a schermo

Nessuno di questi passaggi deve comportarsi diversamente da prima della feature.

- [ ] **Step 6: Run `/impeccable` on the two surfaces touched**

Lancia `/impeccable` su `SettingsView.vue` (il pannello nuovo, la coppia porta/sicurezza, il bottone di prova e i suoi tre stati) e su `ManageBookingView.vue` (la pagina aperta da deep-link, senza il campo codice, in `mode='score'`, su viewport da telefono). Applica le correzioni che ne escono e ricontrolla con `npm run build`.

- [ ] **Step 7: Commit whatever the pass changed**

```bash
git add -A
git commit -m "polish: refine the SMTP panel and the deep-linked booking page"
```

---

## Self-review

**Copertura della spec.** Ogni sezione della spec ha un task: l'invariante di opzionalità è nei Global Constraints e ha test propri nei task 4, 7, 8, 9 e nella verifica manuale del task 15; `internal/mailer` nei task 1–2; migrazione e store nel 3; API impostazioni nel 4; glue nel 5; template nel 6; i quattro agganci nei task 7–9; l'endpoint di prova nel 10; le rotte pubbliche nell'11; il pannello nel 12; `mailQueued` in UI nel 13; documentazione nel 14; verifica e `impeccable` nel 15.

**Nomi e firme.** `smtpConfigFrom` nasce nel task 4 e viene consumata nei task 5 e 10. `mailSender`/`mailEnabled`/`sendMailAsync`/`publicBaseURL` nascono nel 5 e sono usate nei task 7–10. `bookingMailData` nasce nel 6 e viene popolata da `bookingMailDataFor` nel task 8, riusata dal 9. Gli URL (`inviteURL`, `bookingManageURL`, `bookingScoreURL`, `eventPublicURL`) nascono nel task 7 e i path corrispondono uno a uno alle rotte del task 11.

**Verificato sul codice, non assunto.** `AdminCancelBooking` restituisce già `(Booking, error)` (`backend/internal/events/bookings.go:167`), quindi il task 9 non tocca lo store. Lo store elenca le prenotazioni con `ListBookingsForEvent`, non `ListEventBookings`. `EventDetailView.vue` monta `BookingConfirmation` in due punti e conserva una `ConfirmedBooking` per gioco: il task 13 porta `mailed` per prenotazione, non per pagina.

**Un solo punto ancora da confermare in esecuzione.** I nomi delle variabili di spaziatura in `frontend/src/app.css` (il task 12 usa `var(--space-md, 1rem)` con fallback e dice di allinearsi a quelle vere).
