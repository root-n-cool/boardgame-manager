package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"boardgames-manager/internal/games"
)

func parseIDParam(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, name), 10, 64)
}

func (s *Server) listGamesHandler(w http.ResponseWriter, r *http.Request) {
	list, err := s.Games.ListGames(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list games")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, g := range list {
		out = append(out, toGameSummary(g))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getGameHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}
	game, err := s.Games.GetGame(r.Context(), id)
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game")
		return
	}
	langs, err := s.Games.ListLanguages(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load languages")
		return
	}
	resp, err := s.toGameDetail(r.Context(), game, langs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not build response")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

type updateGameRequest struct {
	Owner           *string `json:"owner"`
	Year            *int    `json:"year"`
	MinPlayers      *int    `json:"minPlayers"`
	MaxPlayers      *int    `json:"maxPlayers"`
	PlaytimeMinutes *int    `json:"playtimeMinutes"`
}

func (s *Server) updateGameHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}
	var req updateGameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	game, err := s.Games.UpdateGame(r.Context(), id, games.GameUpdate{
		Owner: req.Owner, Year: req.Year, MinPlayers: req.MinPlayers,
		MaxPlayers: req.MaxPlayers, PlaytimeMinutes: req.PlaytimeMinutes,
	})
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update game")
		return
	}
	writeJSON(w, http.StatusOK, toGameSummary(game))
}

func (s *Server) deleteGameHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}
	if err := s.Games.DeleteGame(r.Context(), id); errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "game not found")
		return
	} else if errors.Is(err, games.ErrGameInUse) {
		writeError(w, http.StatusConflict, "il gioco è usato in uno o più eventi")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete game")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
