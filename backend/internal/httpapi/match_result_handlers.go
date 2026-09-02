package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/leaderboard"
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

func (s *Server) listEventMatchResultsHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	list, err := s.Events.ListMatchResultsForEvent(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list match results")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, m := range list {
		out = append(out, map[string]any{
			"bookingId": m.BookingID, "participantName": m.ParticipantName,
			"gameName": m.GameName, "players": toPlayerScores(m.Players),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func toLeaderboardResponse(lb leaderboard.Leaderboard) map[string]any {
	playerStats := make([]map[string]any, 0, len(lb.Players))
	for _, p := range lb.Players {
		playerStats = append(playerStats, map[string]any{
			"name": p.Name, "gamesPlayed": p.GamesPlayed, "wins": p.Wins,
			"averageScore": p.AverageScore, "totalScore": p.TotalScore,
		})
	}
	matches := make([]map[string]any, 0, len(lb.Matches))
	for _, m := range lb.Matches {
		matchPlayers := make([]map[string]any, 0, len(m.Players))
		for _, p := range m.Players {
			matchPlayers = append(matchPlayers, map[string]any{"name": p.Name, "score": p.Score, "isWinner": p.IsWinner})
		}
		matches = append(matches, map[string]any{
			"eventTitle": m.EventTitle, "eventDate": m.EventDate, "startTime": m.StartTime, "players": matchPlayers,
		})
	}
	return map[string]any{"players": playerStats, "matches": matches}
}

func (s *Server) getLeaderboardHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}
	lb, err := s.Leaderboard.GetLeaderboard(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not build leaderboard")
		return
	}
	writeJSON(w, http.StatusOK, toLeaderboardResponse(lb))
}
