package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"boardgames-manager/internal/events"
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

	// La join sulle mancanze si paga solo quando serve: un visitatore non
	// vedrà mai il campo, quindi non deve nemmeno costarlo. Questa rotta è
	// pubblica (non passa da requireAuth), quindi il controllo di sessione
	// è hasAdminSession, non currentUser: qui non c'è alcun contesto
	// popolato da valutare.
	var missing map[int64][]events.MissingPiece
	admin := s.hasAdminSession(r)
	if admin {
		ids := make([]int64, 0, len(list))
		for _, g := range list {
			ids = append(ids, g.ID)
		}
		missing, err = s.Events.GamesMissingPieces(r.Context(), ids)
		if err != nil {
			log.Printf("games: missing pieces: %v", err)
			writeError(w, http.StatusInternalServerError, "could not list games")
			return
		}
	}

	out := make([]map[string]any, 0, len(list))
	for _, g := range list {
		// Un gioco nascosto sparisce dal catalogo pubblico, non dall'admin:
		// i picker degli eventi e la lista di gestione leggono questa stessa
		// rotta e devono continuare a vederlo.
		if g.HiddenFromCatalog && !admin {
			continue
		}
		var incomplete *bool
		if missing != nil {
			v := len(missing[g.ID]) > 0
			incomplete = &v
		}
		summary := toGameSummary(g, incomplete)
		if admin {
			summary["hiddenFromCatalog"] = g.HiddenFromCatalog
		}
		out = append(out, summary)
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
	resp, err := s.toGameDetail(r.Context(), game, langs, s.hasAdminSession(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not build response")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

type updateGameRequest struct {
	Owner             *string  `json:"owner"`
	Year              *int     `json:"year"`
	MinPlayers        *int     `json:"minPlayers"`
	MaxPlayers        *int     `json:"maxPlayers"`
	PlaytimeMinutes   *int     `json:"playtimeMinutes"`
	Weight            *float64 `json:"weight"`
	Seats             *int     `json:"seats"`
	Kind              *string  `json:"kind"`
	HiddenFromCatalog *bool    `json:"hiddenFromCatalog"`
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
	if req.Seats != nil && *req.Seats < 1 {
		writeError(w, http.StatusBadRequest, "i posti prenotabili devono essere almeno 1")
		return
	}
	if req.Kind != nil && !games.ValidKind(*req.Kind) {
		writeError(w, http.StatusBadRequest, "tipologia di gioco sconosciuta")
		return
	}
	game, err := s.Games.UpdateGame(r.Context(), id, games.GameUpdate{
		Owner: req.Owner, Year: req.Year, MinPlayers: req.MinPlayers,
		MaxPlayers: req.MaxPlayers, PlaytimeMinutes: req.PlaytimeMinutes,
		Weight: req.Weight, Seats: req.Seats,
		Kind: req.Kind, HiddenFromCatalog: req.HiddenFromCatalog,
	})
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update game")
		return
	}
	summary := toGameSummary(game, nil)
	summary["hiddenFromCatalog"] = game.HiddenFromCatalog
	writeJSON(w, http.StatusOK, summary)
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
