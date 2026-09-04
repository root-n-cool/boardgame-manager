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
	// Source dice da quale testo partire: "bgg" per l'originale di
	// BoardGameGeek, oppure il codice di una lingua già presente. Vuoto
	// significa "scegli tu", il comportamento che c'era prima che la scelta
	// esistesse: l'originale BGG se c'è, altrimenti la lingua base.
	Source string `json:"source"`
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

	// Il nome della nuova lingua parte sempre da quello della lingua base:
	// è il titolo dell'edizione, non un testo da tradurre.
	name := game.Name
	var baseDescription *string
	existing, err := s.Games.ListLanguages(r.Context(), gameID)
	if err == nil {
		for _, l := range existing {
			if l.IsBaseLanguage {
				name = l.Name
				baseDescription = l.Description
				break
			}
		}
	}

	source, ok := resolveTranslationSource(req.Source, game, existing, baseDescription)
	if !ok {
		writeError(w, http.StatusNotFound, "la descrizione di partenza non esiste su questo gioco")
		return
	}

	description := baseDescription
	if source != "" {
		// translateDescription restituisce la sorgente invariata quando l'AI
		// non è configurata: senza provider la scelta è un "copia da", che
		// resta utile per tradurre a mano.
		translated := s.translateDescription(r.Context(), source, req.LanguageCode)
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

// resolveTranslationSource sceglie il testo da cui partire. "bgg" prende
// l'originale di BoardGameGeek, un codice lingua prende la descrizione di
// quella lingua, e la stringa vuota lascia decidere al server come prima:
// l'originale BGG se c'è, altrimenti la lingua base.
//
// Scegliere una lingua esistente significa tradurre una traduzione, cosa che
// il flusso automatico evita apposta. Qui è una decisione esplicita
// dell'admin, e spesso è quella giusta: una descrizione corretta a mano è una
// sorgente migliore dell'inglese grezzo di BGG.
func resolveTranslationSource(requested string, game games.Game, existing []games.GameLanguage, baseDescription *string) (string, bool) {
	bggOriginal := ""
	if game.BGGDescription != nil {
		bggOriginal = *game.BGGDescription
	}

	switch strings.ToLower(strings.TrimSpace(requested)) {
	case "":
		return bggOriginal, true
	case "bgg":
		if bggOriginal == "" {
			return "", false
		}
		return bggOriginal, true
	default:
		code := strings.ToLower(strings.TrimSpace(requested))
		for _, l := range existing {
			if strings.ToLower(l.LanguageCode) == code {
				if l.Description == nil {
					return "", true
				}
				return *l.Description, true
			}
		}
		return "", false
	}
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
