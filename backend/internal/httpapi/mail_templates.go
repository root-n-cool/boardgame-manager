package httpapi

import (
	"fmt"
	"html"
	"strings"

	"boardgames-manager/internal/mailer"
)

// I contenuti delle mail, come funzioni pure: prendono dati già risolti e
// non toccano né rete né database. Così il testo che il partecipante
// legge davvero è verificabile in un test da un millisecondo.
//
// Ogni messaggio esce in due parti identiche nel contenuto: testo semplice
// e HTML. Chi legge in solo testo deve trovare codice e link per intero,
// non un invito a "guardare la versione HTML".
//
// L'HTML è volutamente primitivo — tabelle, stili inline, nessuna
// immagine, nessun font esterno, 560px di larghezza massima — perché i
// client di posta non sono browser. La palette è quella di DESIGN.md.

const (
	mailColorFelt     = "#1f4d3a"
	mailColorCard     = "#faf6ec"
	mailColorCardAlt  = "#f2ead4"
	mailColorInk      = "#241f18"
	mailColorInkMuted = "#6e6250"
	mailColorAccent   = "#9c2b2b"
	mailColorLine     = "#ddd0ab"
)

// bookingMailData è quello che serve alle due mail di prenotazione. I
// campi arrivano già composti: GameLabel porta il "#2" solo quando
// l'evento ha più copie di quel gioco, come fa la pagina pubblica, e
// SharedTable dice se il punteggio è di tavolo.
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

// mailShell avvolge il corpo nella cornice comune: intestazione in feltro
// col nome dell'app, carta sotto, e nessun piede — non c'è nulla da
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

// mailButton è un bottone che regge anche nei client che ignorano tutto:
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
// schermo: è l'unica cosa che il partecipante deve poter ritrovare in un
// colpo d'occhio.
func mailCodeBlock(code string) string {
	return `<table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 0 18px;"><tr>
<td style="background:` + mailColorCardAlt + `;border:1px solid ` + mailColorLine + `;border-radius:10px;padding:14px 20px;text-align:center;">
<div style="font-size:12px;letter-spacing:0.08em;text-transform:uppercase;color:` + mailColorInkMuted + `;margin-bottom:6px;">Il tuo codice</div>
<div style="font:600 26px/1.1 'IBM Plex Mono',ui-monospace,monospace;letter-spacing:0.1em;color:` + mailColorInk + `;">` +
		html.EscapeString(code) + `</div>
</td></tr></table>`
}

// mailFactRow è una riga "etichetta: valore" del riepilogo evento.
func mailFactRow(label, value string) string {
	return `<tr>
<td style="padding:4px 12px 4px 0;color:` + mailColorInkMuted + `;font-size:13px;white-space:nowrap;">` + html.EscapeString(label) + `</td>
<td style="padding:4px 0;font-weight:600;">` + html.EscapeString(value) + `</td>
</tr>`
}

func inviteMail(to, invitedBy, inviteURL string) mailer.Message {
	text := strings.Join([]string{
		"Ciao,",
		"",
		fmt.Sprintf("%s ti ha aggiunto come amministratore di BoardGames Manager.", invitedBy),
		"",
		"Apri questo link per scegliere la tua password ed entrare:",
		inviteURL,
		"",
		"Il link è personale e vale una volta sola: chi ti ha invitato non vedrà mai la password che scegli.",
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
	sharedNote := "Questo tavolo ha più posti prenotabili, uno a testa: il punteggio finale è uno per tavolo, e chiunque sieda qui può inserirlo o correggerlo col proprio codice."

	lines := []string{
		fmt.Sprintf("Ciao %s,", d.ParticipantName),
		"",
		fmt.Sprintf("la tua prenotazione per %s è confermata.", d.GameLabel),
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
	// Testo e HTML dicono la stessa frase: una variabile sola, così non
	// possono divergere a una modifica futura.
	opening := fmt.Sprintf("la tua prenotazione per %s è stata annullata, come hai chiesto.", d.GameLabel)
	closing := "Se hai cambiato idea puoi prenotare di nuovo, se restano posti."
	if byAdmin {
		opening = fmt.Sprintf("la tua prenotazione per %s è stata annullata dall'organizzazione.", d.GameLabel)
		closing = "Il posto è tornato libero: puoi prenotare un altro gioco della serata."
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
		"Il codice " + d.BookingCode + " non è più valido.",
	}, "\n")

	facts := `<table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 0 18px;border-collapse:collapse;">` +
		mailFactRow("Evento", d.EventTitle) +
		mailFactRow("Data", d.EventDate) +
		mailFactRow("Gioco", d.GameLabel) +
		`</table>`

	body := mailParagraph(fmt.Sprintf("Ciao %s,", d.ParticipantName)) +
		mailParagraph(opening) +
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

// smtpTestMail è la mail del bottone "Invia email di prova": deve
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
