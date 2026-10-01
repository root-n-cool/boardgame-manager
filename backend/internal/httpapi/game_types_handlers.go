package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"boardgames-manager/internal/games"
)

func toGameTypeResponse(gt games.GameType, withCount bool) map[string]any {
	out := map[string]any{
		"id": gt.ID, "name": gt.Name, "slug": gt.Slug,
		"bggSearch": gt.BGGSearch, "position": gt.Position,
	}
	// Quanti giochi ha una tipologia è un dato di gestione: la pagina
	// pubblica conta i giochi che ha già in mano.
	if withCount {
		out["gameCount"] = gt.GameCount
	}
	return out
}

func (s *Server) listGameTypesHandler(w http.ResponseWriter, r *http.Request) {
	list, err := s.Games.ListGameTypes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list game types")
		return
	}
	admin := s.hasAdminSession(r)
	out := make([]map[string]any, 0, len(list))
	for _, gt := range list {
		out = append(out, toGameTypeResponse(gt, admin))
	}
	writeJSON(w, http.StatusOK, out)
}

// writeGameTypeError traduce gli errori dello store; restituisce false se
// l'errore non è uno di quelli noti.
func writeGameTypeError(w http.ResponseWriter, err error) bool {
	var inUse *games.GameTypeInUseError
	switch {
	case errors.Is(err, games.ErrNotFound):
		writeError(w, http.StatusNotFound, "tipologia non trovata")
	case errors.Is(err, games.ErrGameTypeName), errors.Is(err, games.ErrGameTypeSlug):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, games.ErrSlugTaken), errors.Is(err, games.ErrLastGameType):
		writeError(w, http.StatusConflict, err.Error())
	case errors.As(err, &inUse):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "games": inUse.Count})
	default:
		return false
	}
	return true
}

type gameTypeRequest struct {
	Name      *string `json:"name"`
	Slug      *string `json:"slug"`
	BGGSearch *bool   `json:"bggSearch"`
}

func (s *Server) createGameTypeHandler(w http.ResponseWriter, r *http.Request) {
	var req gameTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in := games.GameTypeInput{BGGSearch: true}
	if req.Name != nil {
		in.Name = *req.Name
	}
	if req.Slug != nil {
		in.Slug = *req.Slug
	}
	if req.BGGSearch != nil {
		in.BGGSearch = *req.BGGSearch
	}
	gt, err := s.Games.CreateGameType(r.Context(), in)
	if err != nil {
		if !writeGameTypeError(w, err) {
			writeError(w, http.StatusInternalServerError, "could not create game type")
		}
		return
	}
	writeJSON(w, http.StatusCreated, toGameTypeResponse(gt, true))
}

func (s *Server) updateGameTypeHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game type id")
		return
	}
	var req gameTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	gt, err := s.Games.UpdateGameType(r.Context(), id, games.GameTypeUpdate{Name: req.Name, Slug: req.Slug, BGGSearch: req.BGGSearch})
	if err != nil {
		if !writeGameTypeError(w, err) {
			writeError(w, http.StatusInternalServerError, "could not update game type")
		}
		return
	}
	writeJSON(w, http.StatusOK, toGameTypeResponse(gt, true))
}

func (s *Server) moveGameTypeHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game type id")
		return
	}
	var req struct {
		Direction string `json:"direction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.Direction != "up" && req.Direction != "down") {
		writeError(w, http.StatusBadRequest, `direction deve essere "up" o "down"`)
		return
	}
	if err := s.Games.MoveGameType(r.Context(), id, req.Direction == "up"); err != nil {
		if !writeGameTypeError(w, err) {
			writeError(w, http.StatusInternalServerError, "could not move game type")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteGameTypeHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game type id")
		return
	}
	if err := s.Games.DeleteGameType(r.Context(), id); err != nil {
		if !writeGameTypeError(w, err) {
			writeError(w, http.StatusInternalServerError, "could not delete game type")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
