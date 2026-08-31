package httpapi

import (
	"encoding/json"
	"net/http"

	"boardgames-manager/internal/settings"
)

type settingsResponse struct {
	DefaultLanguage     string `json:"defaultLanguage"`
	YouTubeAPIKeySet    bool   `json:"youtubeApiKeySet"`
	YouTubeAPIKeyMasked string `json:"youtubeApiKeyMasked,omitempty"`
	SearchAPIKeySet     bool   `json:"searchApiKeySet"`
	SearchAPIKeyMasked  string `json:"searchApiKeyMasked,omitempty"`
	SearchAPIProvider   string `json:"searchApiProvider"`
}

func maskKey(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return "****" + key[len(key)-4:]
}

func (s *Server) getSettingsHandler(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.Settings.Get(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settings")
		return
	}

	resp := settingsResponse{
		DefaultLanguage:   cfg.DefaultLanguage,
		SearchAPIProvider: cfg.SearchAPIProvider,
		YouTubeAPIKeySet:  cfg.YouTubeAPIKey != "",
		SearchAPIKeySet:   cfg.SearchAPIKey != "",
	}
	if resp.YouTubeAPIKeySet {
		resp.YouTubeAPIKeyMasked = maskKey(cfg.YouTubeAPIKey)
	}
	if resp.SearchAPIKeySet {
		resp.SearchAPIKeyMasked = maskKey(cfg.SearchAPIKey)
	}
	writeJSON(w, http.StatusOK, resp)
}

type updateSettingsRequest struct {
	DefaultLanguage   string `json:"defaultLanguage"`
	YouTubeAPIKey     string `json:"youtubeApiKey"`
	SearchAPIKey      string `json:"searchApiKey"`
	SearchAPIProvider string `json:"searchApiProvider"`
}

func (s *Server) putSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DefaultLanguage == "" {
		writeError(w, http.StatusBadRequest, "defaultLanguage is required")
		return
	}

	current, err := s.Settings.Get(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settings")
		return
	}

	next := settings.Settings{
		DefaultLanguage:   req.DefaultLanguage,
		YouTubeAPIKey:     current.YouTubeAPIKey,
		SearchAPIKey:      current.SearchAPIKey,
		SearchAPIProvider: req.SearchAPIProvider,
	}
	if req.YouTubeAPIKey != "" {
		next.YouTubeAPIKey = req.YouTubeAPIKey
	}
	if req.SearchAPIKey != "" {
		next.SearchAPIKey = req.SearchAPIKey
	}

	if err := s.Settings.Update(r.Context(), next); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save settings")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}
