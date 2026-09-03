package httpapi

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"boardgames-manager/internal/bgg"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/storage"
)

var coverDownloadClient = &http.Client{Timeout: 15 * time.Second}

type createGameRequest struct {
	BGGID           string   `json:"bggId"`
	LanguageCode    string   `json:"languageCode"`
	Owner           string   `json:"owner"`
	Name            string   `json:"name"`
	Year            *int     `json:"year"`
	MinPlayers      *int     `json:"minPlayers"`
	MaxPlayers      *int     `json:"maxPlayers"`
	PlaytimeMinutes *int     `json:"playtimeMinutes"`
	Weight          *float64 `json:"weight"`
	// Seats sono i posti prenotabili per copia: assente vale 1.
	Seats                 *int   `json:"seats"`
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
	if req.Seats != nil && *req.Seats < 1 {
		writeError(w, http.StatusBadRequest, "i posti prenotabili devono essere almeno 1")
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

	// A game with no BGG votes has no weight — storing 0 would read as
	// "trivially light" in the catalogue.
	var weight *float64
	if detail.Weight > 0 {
		weight = &detail.Weight
	}

	game, err := s.Games.CreateGame(r.Context(), games.Game{
		BGGID: &bggID, Name: detail.Name, Year: &year, MinPlayers: &minPlayers,
		MaxPlayers: &maxPlayers, PlaytimeMinutes: &playtime, Owner: &owner, CoverPath: coverPath,
		Weight: weight, Seats: requestedSeats(req),
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
		Weight: req.Weight, Seats: requestedSeats(req),
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

// searchMinQuery mirrors the frontend picker: below three characters a BGG
// search is mostly noise, and every keystroke costs an upstream call.
const searchMinQuery = 3

// searchMaxResults caps how many hits reach the picker. BGG answers a common
// word with well over a hundred items; a dozen ranked rows is what fits on a
// screen, and it is also how many thumbnails we ask BGG for.
const searchMaxResults = 12

func (s *Server) searchGamesHandler(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) < searchMinQuery {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("q must be at least %d characters", searchMinQuery))
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

	results = rankSearchResults(results, query)
	if len(results) > searchMaxResults {
		results = results[:searchMaxResults]
	}

	ids := make([]string, 0, len(results))
	for _, res := range results {
		ids = append(ids, res.ID)
	}
	// Thumbnails and weights are a nicety: if the second BGG call fails the
	// picker still lists the games, just without covers.
	details, err := s.BGG.Details(r.Context(), cfg.BGGAPIToken, ids)
	if err != nil {
		details = nil
	}

	out := make([]map[string]any, 0, len(results))
	for _, res := range results {
		row := map[string]any{"bggId": res.ID, "name": res.Name, "year": res.Year}
		detail := details[res.ID]
		row["thumbnailUrl"] = nilIfEmpty(detail.ThumbnailURL)
		row["weight"] = nilIfUnrated(detail.Weight)
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, out)
}

// rankSearchResults reorders BGG's alphabetical hits by how well they answer
// the query — exact name, then names starting with it, then the rest — with
// the shortest name winning ties. Without this the cap above would routinely
// drop the game the admin actually typed.
func rankSearchResults(results []bgg.SearchResult, query string) []bgg.SearchResult {
	needle := strings.ToLower(strings.TrimSpace(query))
	score := func(name string) int {
		lower := strings.ToLower(name)
		switch {
		case lower == needle:
			return 0
		case strings.HasPrefix(lower, needle):
			return 1
		case strings.Contains(lower, needle):
			return 2
		default:
			return 3
		}
	}
	ranked := slices.Clone(results)
	slices.SortStableFunc(ranked, func(a, b bgg.SearchResult) int {
		if c := cmp.Compare(score(a.Name), score(b.Name)); c != 0 {
			return c
		}
		return cmp.Compare(len(a.Name), len(b.Name))
	})
	return ranked
}

func nilIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// nilIfUnrated turns BGG's zero weight into "unknown": a game nobody has
// rated is not a featherweight, we simply do not know.
func nilIfUnrated(weight float64) any {
	if weight <= 0 {
		return nil
	}
	return weight
}

// requestedSeats traduce il campo opzionale in un valore concreto: un gioco
// senza posti prenotabili dichiarati è un gioco a copia singola.
func requestedSeats(req createGameRequest) int {
	if req.Seats == nil {
		return 1
	}
	return *req.Seats
}
