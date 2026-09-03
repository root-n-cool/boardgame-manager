package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"boardgames-manager/internal/games"
)

type createLanguageRequest struct {
	LanguageCode string `json:"languageCode"`
}

func (s *Server) createLanguageHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}
	var req createLanguageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "languageCode is required")
		return
	}
	req.LanguageCode = strings.ToLower(strings.TrimSpace(req.LanguageCode))
	if req.LanguageCode == "" {
		writeError(w, http.StatusBadRequest, "languageCode is required")
		return
	}

	game, err := s.Games.GetGame(r.Context(), gameID)
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game")
		return
	}

	// Con l'originale BGG a disposizione si traduce da lì: partire dalla
	// lingua base darebbe la traduzione di una traduzione. Senza originale
	// — gioco inserito a mano, o entrato in catalogo prima della 0011 —
	// resta il ripiego di sempre, il testo della lingua base come punto di
	// partenza per una correzione a mano.
	name := game.Name
	var description *string
	existing, err := s.Games.ListLanguages(r.Context(), gameID)
	if err == nil {
		for _, l := range existing {
			if l.IsBaseLanguage {
				name = l.Name
				description = l.Description
				break
			}
		}
	}
	if game.BGGDescription != nil && *game.BGGDescription != "" {
		translated := s.translateDescription(r.Context(), *game.BGGDescription, req.LanguageCode)
		description = &translated
	}

	lang, err := s.Games.CreateLanguage(r.Context(), games.GameLanguage{
		GameID: gameID, LanguageCode: req.LanguageCode, IsBaseLanguage: false,
		Name: name, Description: description,
	})
	if errors.Is(err, games.ErrDuplicateLanguage) {
		writeError(w, http.StatusConflict, "questa lingua esiste già")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create language")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"code": lang.LanguageCode, "isBaseLanguage": lang.IsBaseLanguage,
		"name": lang.Name, "description": lang.Description,
	})
}

type updateLanguageRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

func (s *Server) updateLanguageHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}
	code := chi.URLParam(r, "lang")

	var req updateLanguageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	lang, err := s.Games.UpdateLanguage(r.Context(), gameID, code, req.Name, req.Description)
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "language not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update language")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": lang.LanguageCode, "isBaseLanguage": lang.IsBaseLanguage,
		"name": lang.Name, "description": lang.Description,
	})
}
