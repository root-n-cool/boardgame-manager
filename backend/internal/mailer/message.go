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

// buildMessage produce i byte da consegnare a DATA. È pura di proposito:
// boundary, Message-ID e ora arrivano dal chiamante, così il contenuto è
// verificabile byte per byte nei test invece di cambiare a ogni esecuzione.
//
// Il corpo è sempre un multipart/alternative con la parte testo prima
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
	// sé un nome non-ASCII; con nome vuoto rende il solo <indirizzo>.
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
