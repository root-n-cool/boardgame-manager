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
	// Porta 1 su localhost: nessuno ascolta, e il rifiuto è immediato.
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
		t.Fatal("un host irraggiungibile non è 'non configurato'")
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
		t.Error("nessun messaggio deve essere consegnato se la cifratura richiesta non c'è")
	}
}
