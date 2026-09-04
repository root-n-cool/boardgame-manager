// Package mailer manda email via SMTP. Nel progetto serve alle tre
// comunicazioni verso l'esterno: l'invito di un amministratore, la
// conferma di una prenotazione e l'avviso di annullamento.
//
// Come internal/ai, è un sottosistema opzionale: senza configurazione
// restituisce ErrNotConfigured, che per il chiamante non è un guasto ma
// l'app che gira senza posta — lo stato in cui è nata e in cui deve
// continuare a funzionare per intero.
package mailer

import (
	"context"
	"errors"
	"strings"
)

// ErrNotConfigured dice che l'admin non ha (ancora) messo un server SMTP.
// Non è un errore da mostrare né da loggare: è una configurazione
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
	// TLSMode è una delle tre costanti sopra. Un valore vuoto o ignoto
	// si comporta come STARTTLS, che è il caso di gran lunga più comune.
	TLSMode string
}

// Configured dice se c'è abbastanza per provare a spedire. User e
// password non contano: un relay senza autenticazione è legittimo.
func (c Config) Configured() bool {
	return strings.TrimSpace(c.Host) != "" && c.Port != 0 && strings.TrimSpace(c.FromAddress) != ""
}

// Message è una mail a un solo destinatario, in testo e HTML. Il
// progetto non manda mai a più indirizzi insieme: ogni comunicazione è
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
