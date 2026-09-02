package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"boardgames-manager/internal/events"
)

const maxPlayersPerMatch = 20

type playerScoreRequest struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
}

type matchResultRequest struct {
	BookingCode string               `json:"bookingCode"`
	Players     []playerScoreRequest `json:"players"`
}

func decodeMatchResultRequest(r *http.Request) ([]events.PlayerScore, string, bool) {
	var req matchResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", false
	}
	if req.BookingCode == "" || len(req.Players) == 0 || len(req.Players) > maxPlayersPerMatch {
		return nil, "", false
	}
	players := make([]events.PlayerScore, 0, len(req.Players))
	for _, p := range req.Players {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			return nil, "", false
		}
		players = append(players, events.PlayerScore{Name: name, Score: p.Score})
	}
	return players, req.BookingCode, true
}

func (s *Server) submitMatchResultHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid booking id")
		return
	}
	players, code, ok := decodeMatchResultRequest(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "bookingCode and at least one player with a name are required")
		return
	}
	result, err := s.Events.SubmitMatchResult(r.Context(), id, code, players)
	switch {
	case errors.Is(err, events.ErrNotFound), errors.Is(err, events.ErrInvalidBookingCredentials):
		writeError(w, http.StatusNotFound, "prenotazione non trovata")
	case errors.Is(err, events.ErrBookingNotActive):
		writeError(w, http.StatusConflict, "la prenotazione non è più attiva")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not submit match result")
	default:
		writeJSON(w, http.StatusOK, toMatchResultResponse(result))
	}
}
