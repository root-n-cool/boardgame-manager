package httpapi

import (
	"net/http"
	"strings"
)

// placeMinQuery è sotto quante lettere non vale la pena disturbare
// Nominatim: "vi" restituirebbe mezzo mondo.
const placeMinQuery = 3

// searchPlacesHandler cerca un luogo su OpenStreetMap per conto dell'admin.
// Passare dal server, invece di far chiamare Nominatim al browser, è quello
// che permette di mandare uno User-Agent che identifica l'applicazione e di
// tenere il traffico dentro il limite che la sua usage policy chiede.
func (s *Server) searchPlacesHandler(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) < placeMinQuery {
		writeError(w, http.StatusBadRequest, "q must be at least 3 characters")
		return
	}

	places, err := s.Geocode.Search(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not search OpenStreetMap")
		return
	}

	out := make([]map[string]any, 0, len(places))
	for _, p := range places {
		out = append(out, map[string]any{
			"name": p.Name, "address": p.Address, "lat": p.Lat, "lon": p.Lon,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
