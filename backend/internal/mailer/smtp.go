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

// dialTimeout è il tetto sull'apertura della connessione quando il
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
// passo è fallito, perché "connessione rifiutata" e "autenticazione
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
	// Il defer chiude conn com'è al momento in cui gira, non com'è ora:
	// con TLS implicito la variabile viene riassegnata al *tls.Conn subito
	// sotto, e vogliamo chiudere quello, non la connessione grezza sostituita.
	defer func() { conn.Close() }()
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

	// Tutto quello che non è TLS implicito o "nessuna sicurezza" vuole
	// STARTTLS, compreso il valore vuoto: è il default e il caso comune.
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
		// non cifrata (tranne verso localhost): è una protezione della
		// stdlib, e l'errore che ne esce è quello giusto da mostrare.
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
	if err := client.Quit(); err != nil {
		return fmt.Errorf("chiusura della sessione SMTP non riuscita: %w", err)
	}
	return nil
}

// compose genera le parti che devono cambiare a ogni messaggio e delega a
// buildMessage, che invece è pura.
func (s *SMTPSender) compose(m Message) ([]byte, error) {
	boundary, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	id, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	// Il dominio del Message-ID è quello del mittente: è l'unico che
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
