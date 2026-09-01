package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"boardgames-manager/internal/games"
	"boardgames-manager/internal/storage"
)

var coverDownloadClient = &http.Client{Timeout: 15 * time.Second}

type createGameRequest struct {
	BGGID                 string `json:"bggId"`
	LanguageCode          string `json:"languageCode"`
	Owner                 string `json:"owner"`
	Name                  string `json:"name"`
	Year                  *int   `json:"year"`
	MinPlayers            *int   `json:"minPlayers"`
	MaxPlayers            *int   `json:"maxPlayers"`
	PlaytimeMinutes       *int   `json:"playtimeMinutes"`
	NameTranslated        string `json:"nameTranslated"`
	DescriptionTranslated string `json:"descriptionTranslated"`
}

func (s *Server) createGameHandler(w http.ResponseWriter, r *http.Request) {
	var req createGameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.LanguageCode == "" {
		writeError(w, http.StatusBadRequest, "languageCode is required")
		return
	}

	if req.BGGID != "" {
		s.createGameFromBGG(w, r, req)
		return
	}
	s.createGameManually(w, r, req)
}

func (s *Server) createGameFromBGG(w http.ResponseWriter, r *http.Request, req createGameRequest) {
	cfg, err := s.Settings.Get(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settings")
		return
	}
	if cfg.BGGAPIToken == "" {
		writeError(w, http.StatusConflict, "BGG API token not configured")
		return
	}

	detail, err := s.BGG.GetThing(r.Context(), cfg.BGGAPIToken, req.BGGID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not fetch game from BGG")
		return
	}

	var coverPath *string
	if detail.ImageURL != "" {
		if path, err := s.downloadCover(r.Context(), detail.ImageURL); err == nil {
			coverPath = &path
		}
		// A failed cover download is not fatal — the game is still created without one.
	}

	bggID := detail.ID
	year := detail.Year
	minPlayers := detail.MinPlayers
	maxPlayers := detail.MaxPlayers
	playtime := detail.PlayingTime
	owner := req.Owner

	game, err := s.Games.CreateGame(r.Context(), games.Game{
		BGGID: &bggID, Name: detail.Name, Year: &year, MinPlayers: &minPlayers,
		MaxPlayers: &maxPlayers, PlaytimeMinutes: &playtime, Owner: &owner, CoverPath: coverPath,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create game")
		return
	}

	description := detail.Description
	lang, err := s.Games.CreateLanguage(r.Context(), games.GameLanguage{
		GameID: game.ID, LanguageCode: req.LanguageCode, IsBaseLanguage: true,
		Name: detail.Name, Description: &description,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create game language")
		return
	}

	resp, err := s.toGameDetail(r.Context(), game, []games.GameLanguage{lang})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not build response")
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) createGameManually(w http.ResponseWriter, r *http.Request, req createGameRequest) {
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	owner := req.Owner
	game, err := s.Games.CreateGame(r.Context(), games.Game{
		Name: req.Name, Year: req.Year, MinPlayers: req.MinPlayers,
		MaxPlayers: req.MaxPlayers, PlaytimeMinutes: req.PlaytimeMinutes, Owner: &owner,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create game")
		return
	}

	name := req.NameTranslated
	if name == "" {
		name = req.Name
	}
	var description *string
	if req.DescriptionTranslated != "" {
		description = &req.DescriptionTranslated
	}

	lang, err := s.Games.CreateLanguage(r.Context(), games.GameLanguage{
		GameID: game.ID, LanguageCode: req.LanguageCode, IsBaseLanguage: true,
		Name: name, Description: description,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create game language")
		return
	}

	resp, err := s.toGameDetail(r.Context(), game, []games.GameLanguage{lang})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not build response")
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) downloadCover(ctx context.Context, imageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := coverDownloadClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cover download returned status %d", resp.StatusCode)
	}
	return s.Storage.Save(storage.CoverCategory, resp.Body)
}

func (s *Server) searchGamesHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}
	cfg, err := s.Settings.Get(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settings")
		return
	}
	if cfg.BGGAPIToken == "" {
		writeError(w, http.StatusConflict, "BGG API token not configured")
		return
	}
	results, err := s.BGG.Search(r.Context(), cfg.BGGAPIToken, query)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not search BGG")
		return
	}
	out := make([]map[string]any, 0, len(results))
	for _, res := range results {
		out = append(out, map[string]any{"bggId": res.ID, "name": res.Name, "year": res.Year})
	}
	writeJSON(w, http.StatusOK, out)
}
