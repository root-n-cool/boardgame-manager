package httpapi

import (
	"strings"
	"testing"

	"boardgames-manager/internal/mailer"
)

func testBookingData() bookingMailData {
	return bookingMailData{
		ParticipantName:  "Mario Rossi",
		ParticipantEmail: "mario@example.com",
		BookingCode:      "ABC23XYZ",
		GameLabel:        "Catan #2",
		EventTitle:       "Serata giochi di settembre",
		EventDate:        "2026-09-18",
		Hours:            "21:00",
		EventID:          7,
	}
}

// bothBodies è la lista delle due parti: ogni cosa che deve arrivare al
// destinatario va verificata in entrambe, perché un client di posta può
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
	m := inviteMail("BoardGames Manager", "nuovo@example.com", "admin@example.com", "https://giochi.example.org/invito/tok123")

	if m.To != "nuovo@example.com" {
		t.Errorf("To = %q", m.To)
	}
	for _, body := range bothBodies(t, m.Subject, m.TextBody, m.HTMLBody) {
		if !strings.Contains(body, "https://giochi.example.org/invito/tok123") {
			t.Errorf("link di invito assente:\n%s", body)
		}
		if !strings.Contains(body, "admin@example.com") {
			t.Errorf("chi ha invitato non è nominato:\n%s", body)
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

func TestSMTPTestMail_IsSelfExplanatory(t *testing.T) {
	m := smtpTestMail("BoardGames Manager", "admin@example.com")
	if m.To != "admin@example.com" {
		t.Errorf("To = %q", m.To)
	}
	bothBodies(t, m.Subject, m.TextBody, m.HTMLBody)
	if !strings.Contains(strings.ToLower(m.Subject), "prova") {
		t.Errorf("l'oggetto deve dire che è una prova: %q", m.Subject)
	}
}

// Nessun template deve lasciare un placeholder non sostituito in giro.
func TestTemplates_LeaveNoPlaceholders(t *testing.T) {
	messages := []struct {
		name string
		m    mailer.Message
	}{
		{"invito", inviteMail("BoardGames Manager", "a@b.org", "c@d.org", "https://x/invito/t")},
		{"conferma", bookingConfirmationMail(testBookingData(), "https://x/m", "https://x/s")},
		{"prova", smtpTestMail("BoardGames Manager", "a@b.org")},
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

func TestInviteMail_UsesTheConfiguredSiteName(t *testing.T) {
	m := inviteMail("Ludoteca Vicolo Corto", "nuovo@example.com", "admin@example.com", "https://x/invito/t")
	for _, body := range bothBodies(t, m.Subject, m.TextBody, m.HTMLBody) {
		if !strings.Contains(body, "Ludoteca Vicolo Corto") {
			t.Errorf("expected the configured site name, got:\n%s", body)
		}
	}
}

func TestSMTPTestMail_UsesTheConfiguredSiteName(t *testing.T) {
	m := smtpTestMail("Ludoteca Vicolo Corto", "admin@example.com")
	for _, body := range bothBodies(t, m.Subject, m.TextBody, m.HTMLBody) {
		if !strings.Contains(body, "Ludoteca Vicolo Corto") {
			t.Errorf("expected the configured site name, got:\n%s", body)
		}
	}
}
