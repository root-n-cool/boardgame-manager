package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"boardgames-manager/internal/games"
)

// toMaterialsResponse manda sempre una lista, mai null: una lista vuota che
// arriva come null costringe ogni punto della UI a difendersi.
func toMaterialsResponse(ms []games.Material) map[string]any {
	out := make([]map[string]any, 0, len(ms))
	for _, m := range ms {
		out = append(out, map[string]any{"id": m.ID, "name": m.Name, "quantity": m.Quantity})
	}
	return map[string]any{"materials": out}
}

// requireGame risponde 404/500 e dice al chiamante se può proseguire. Le due
// rotte dei materiali cominciano entrambe così: un id inventato deve dare
// 404 prima di qualunque lavoro.
func (s *Server) requireGame(w http.ResponseWriter, r *http.Request, gameID int64) (games.Game, bool) {
	game, err := s.Games.GetGame(r.Context(), gameID)
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "gioco non trovato")
		return games.Game{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game")
		return games.Game{}, false
	}
	return game, true
}

func (s *Server) listMaterialsHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id del gioco non valido")
		return
	}
	if _, ok := s.requireGame(w, r, gameID); !ok {
		return
	}
	ms, err := s.Games.ListMaterials(r.Context(), gameID)
	if err != nil {
		log.Printf("materials: list for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not read the materials")
		return
	}
	writeJSON(w, http.StatusOK, toMaterialsResponse(ms))
}

type materialsRequest struct {
	Materials []struct {
		Name     string `json:"name"`
		Quantity int    `json:"quantity"`
	} `json:"materials"`
}

func (s *Server) putMaterialsHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "id del gioco non valido")
		return
	}
	if _, ok := s.requireGame(w, r, gameID); !ok {
		return
	}
	var req materialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo della richiesta non valido")
		return
	}
	in := make([]games.MaterialInput, 0, len(req.Materials))
	for _, m := range req.Materials {
		in = append(in, games.MaterialInput{Name: m.Name, Quantity: m.Quantity})
	}

	ms, err := s.Games.ReplaceMaterials(r.Context(), gameID, in)
	switch {
	case errors.Is(err, games.ErrMaterialInvalid):
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"ogni voce vuole un nome (fino a %d caratteri) e una quantità da 1 a %d",
			games.MaxMaterialNameChars, games.MaxMaterialQuantity))
	case errors.Is(err, games.ErrDuplicateMaterial):
		writeError(w, http.StatusBadRequest, "due voci con lo stesso nome: unisci le righe")
	case errors.Is(err, games.ErrTooManyMaterials):
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"al massimo %d voci per gioco", games.MaxMaterialsPerGame))
	case err != nil:
		log.Printf("materials: save for game %d: %v", gameID, err)
		writeError(w, http.StatusInternalServerError, "could not save the materials")
	default:
		writeJSON(w, http.StatusOK, toMaterialsResponse(ms))
	}
}
