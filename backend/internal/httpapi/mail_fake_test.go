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
// Le mail arrivano su un canale invece che su una slice perché l'invio è
// asincrono: senza il canale ogni test dovrebbe dormire un tempo
// arbitrario e sarebbe instabile.
type fakeMailer struct {
	sent chan mailer.Message
	// err, se valorizzato, è l'errore che Send restituisce. Il messaggio
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
// non arriva. Due secondi sono un'eternità per una goroutine locale e
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

// expectNoMail verifica che non parta niente. La finestra è breve di
// proposito: qui l'attesa è il costo del test, non la sua correttezza.
func (f *fakeMailer) expectNoMail(t *testing.T) {
	t.Helper()
	select {
	case m := <-f.sent:
		t.Fatalf("nessuna mail attesa, spedita %q a %s", m.Subject, m.To)
	case <-time.After(300 * time.Millisecond):
	}
}
