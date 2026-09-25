package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"boardgames-manager/internal/events"
)

// eventCalendarHandler serve l'evento come file iCalendar (RFC 5545), da
// aprire col calendario del telefono. Gli orari sono "floating" — senza
// fuso né Z — perché l'app non conosce il fuso dell'associazione: l'evento
// cade alle 20:30 dell'orologio di chi lo apre, che è quello della serata.
func (s *Server) eventCalendarHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	event, err := s.Events.GetEvent(r.Context(), id)
	if errors.Is(err, events.ErrNotFound) {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load event")
		return
	}
	start, err := event.StartsAt()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read the event date")
		return
	}
	end, err := event.EndsAt()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read the event end")
		return
	}

	base := s.publicBaseURL(r)
	eventURL := eventPageURL(base, event.ID)
	host := r.Host
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		host = u.Host
	}

	const floating = "20060102T150405"
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//BoardGames Manager//IT",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"BEGIN:VEVENT",
		"UID:event-" + strconv.FormatInt(event.ID, 10) + "@" + host,
		"DTSTAMP:" + time.Now().UTC().Format(floating) + "Z",
		"DTSTART:" + start.Format(floating),
		"DTEND:" + end.Format(floating),
		"SUMMARY:" + icsText(event.Title),
	}
	description := eventURL
	if event.Description != nil && strings.TrimSpace(*event.Description) != "" {
		description = strings.TrimSpace(*event.Description) + "\n\n" + eventURL
	}
	lines = append(lines, "DESCRIPTION:"+icsText(description))
	if v := event.Venue; v != nil {
		location := v.Address
		if v.Name != "" && !strings.Contains(v.Address, v.Name) {
			location = v.Name + ", " + v.Address
		}
		lines = append(lines, "LOCATION:"+icsText(location))
		if v.Lat != nil && v.Lon != nil {
			lines = append(lines, fmt.Sprintf("GEO:%s;%s",
				strconv.FormatFloat(*v.Lat, 'f', -1, 64), strconv.FormatFloat(*v.Lon, 'f', -1, 64)))
		}
	}
	lines = append(lines, "URL:"+eventURL, "END:VEVENT", "END:VCALENDAR")

	var b strings.Builder
	for _, line := range lines {
		b.WriteString(icsFold(line))
		b.WriteString("\r\n")
	}

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="evento-%d.ics"`, event.ID))
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, b.String())
}

// icsText applica l'escape dei valori TEXT di iCalendar: backslash,
// virgola, punto e virgola e a capo hanno un significato nel formato.
func icsText(s string) string {
	return icsEscaper.Replace(s)
}

var icsEscaper = strings.NewReplacer(
	`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`,
)

// icsFold spezza una riga oltre i 75 ottetti come vuole la RFC 5545: la
// continuazione comincia con uno spazio. Il taglio non cade mai dentro un
// carattere UTF-8, che un calendario leggerebbe come spazzatura.
func icsFold(line string) string {
	const limit = 75
	if len(line) <= limit {
		return line
	}
	var b strings.Builder
	width := limit
	for len(line) > width {
		cut := width
		for cut > 0 && !utf8.RuneStart(line[cut]) {
			cut--
		}
		b.WriteString(line[:cut])
		b.WriteString("\r\n ")
		line = line[cut:]
		width = limit - 1 // lo spazio iniziale conta
	}
	b.WriteString(line)
	return b.String()
}
