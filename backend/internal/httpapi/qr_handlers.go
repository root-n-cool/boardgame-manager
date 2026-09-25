package httpapi

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"

	"rsc.io/qr"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/games"
)

// qrQuietZone è il margine bianco attorno al codice, in moduli: quattro è il
// minimo della specifica, e rsc.io/qr non lo aggiunge da sé. Senza, un
// cartellino ritagliato a filo non si legge.
const qrQuietZone = 4

// gameQRHandler serve il cartellino della scheda pubblica del gioco, da
// stampare e mettere nella scatola: chi lo inquadra al tavolo arriva a
// regole, tutorial e classifica senza cercare niente.
func (s *Server) gameQRHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}
	game, err := s.Games.GetGame(r.Context(), id)
	switch {
	case errors.Is(err, games.ErrNotFound):
		writeError(w, http.StatusNotFound, "game not found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not load game")
		return
	}
	base, configured := s.publicAddress(r)
	s.writeQR(w, gamePageURL(base, id), configured, map[string]any{"title": game.Name})
}

// eventQRHandler serve il cartellino della pagina pubblica della serata.
func (s *Server) eventQRHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	event, err := s.Events.GetEvent(r.Context(), id)
	switch {
	case errors.Is(err, events.ErrNotFound):
		writeError(w, http.StatusNotFound, "event not found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not load event")
		return
	}
	base, configured := s.publicAddress(r)
	s.writeQR(w, eventPageURL(base, id), configured, map[string]any{
		"title": event.Title, "eventDate": event.EventDate,
		"startTime": event.StartTime, "endTime": event.EndTime,
	})
}

// writeQR manda tutto quello che serve al cartellino in una risposta sola:
// l'indirizzo codificato (quello che la pagina stampa in chiaro sotto il
// codice, lo stesso e non ricalcolato), se è l'indirizzo pubblico
// configurato o il ripiego sull'host della richiesta, e il codice come SVG.
// Livello Q (circa un quarto del codice recuperabile) perché una scatola si
// graffia.
func (s *Server) writeQR(w http.ResponseWriter, target string, configured bool, card map[string]any) {
	code, err := qr.Encode(target, qr.Q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not encode the QR code")
		return
	}
	card["url"] = target
	card["publicAddressConfigured"] = configured
	card["svg"] = qrSVG(code, target)
	writeJSON(w, http.StatusOK, card)
}

// qrSVG disegna il codice come un solo path, un rettangolo per ogni tratto
// orizzontale di moduli neri: resta nitido a qualunque misura di stampa, e
// il viewBox in moduli lascia la dimensione a chi lo impagina. L'indirizzo
// codificato sta nell'etichetta accessibile.
func qrSVG(code *qr.Code, target string) string {
	side := code.Size + 2*qrQuietZone
	var path strings.Builder
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; {
			if !code.Black(x, y) {
				x++
				continue
			}
			run := 1
			for x+run < code.Size && code.Black(x+run, y) {
				run++
			}
			fmt.Fprintf(&path, "M%d %dh%dv1h-%dz", x+qrQuietZone, y+qrQuietZone, run, run)
			x += run
		}
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges" role="img" aria-label="QR code: %s">`+
		`<rect width="100%%" height="100%%" fill="#fff"/><path fill="#000" d="%s"/></svg>`,
		side, side, html.EscapeString(target), path.String())
}
