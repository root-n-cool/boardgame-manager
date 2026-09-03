package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"boardgames-manager/internal/settings"
)

type settingsResponse struct {
	DefaultLanguage string `json:"defaultLanguage"`
	// PublicBaseURL goes out in clear on purpose: it is an address the admin has
	// to read back and check, not a credential like the token below.
	PublicBaseURL     string `json:"publicBaseUrl"`
	BGGAPITokenSet    bool   `json:"bggApiTokenSet"`
	BGGAPITokenMasked string `json:"bggApiTokenMasked,omitempty"`
	// L'indirizzo e il modello sono dati da rileggere, come PublicBaseURL;
	// la chiave è un segreto e segue BGGAPIToken.
	AIBaseURL      string `json:"aiBaseUrl"`
	AIModel        string `json:"aiModel"`
	AIAPIKeySet    bool   `json:"aiApiKeySet"`
	AIAPIKeyMasked string `json:"aiApiKeyMasked,omitempty"`
	// AIConfigured è il booleano su cui la UI decide se mostrare i comandi
	// di traduzione: servono tutti e tre i valori, non solo la chiave.
	AIConfigured bool `json:"aiConfigured"`
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
		DefaultLanguage: cfg.DefaultLanguage,
		PublicBaseURL:   cfg.PublicBaseURL,
		BGGAPITokenSet:  cfg.BGGAPIToken != "",
	}
	if resp.BGGAPITokenSet {
		resp.BGGAPITokenMasked = maskKey(cfg.BGGAPIToken)
	}
	resp.AIBaseURL = cfg.AIBaseURL
	resp.AIModel = cfg.AIModel
	resp.AIAPIKeySet = cfg.AIAPIKey != ""
	if resp.AIAPIKeySet {
		resp.AIAPIKeyMasked = maskKey(cfg.AIAPIKey)
	}
	resp.AIConfigured = cfg.AIBaseURL != "" && cfg.AIAPIKey != "" && cfg.AIModel != ""
	writeJSON(w, http.StatusOK, resp)
}

type updateSettingsRequest struct {
	DefaultLanguage string `json:"defaultLanguage"`
	PublicBaseURL   string `json:"publicBaseUrl"`
	BGGAPIToken     string `json:"bggApiToken"`
	AIBaseURL       string `json:"aiBaseUrl"`
	AIAPIKey        string `json:"aiApiKey"`
	AIModel         string `json:"aiModel"`
}

// normalizePublicBaseURL accepts an empty value — that is how the admin says
// "not configured" — and otherwise requires an absolute http(s) URL. It returns
// the value without its trailing slash so callers can append a path without
// producing a double one.
func normalizePublicBaseURL(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", true
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", false
	}
	return strings.TrimRight(trimmed, "/"), true
}

func (s *Server) putSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DefaultLanguage == "" {
		writeError(w, http.StatusBadRequest, "defaultLanguage is required")
		return
	}

	baseURL, ok := normalizePublicBaseURL(req.PublicBaseURL)
	if !ok {
		writeError(w, http.StatusBadRequest, "publicBaseUrl must be an absolute http or https address")
		return
	}

	aiBaseURL, ok := normalizePublicBaseURL(req.AIBaseURL)
	if !ok {
		writeError(w, http.StatusBadRequest, "l'indirizzo del provider AI deve essere assoluto, per esempio https://api.openai.com/v1")
		return
	}

	current, err := s.Settings.Get(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settings")
		return
	}

	// The base URL is not a secret, so unlike the token an empty value clears it
	// rather than meaning "leave what is there".
	next := settings.Settings{
		DefaultLanguage: req.DefaultLanguage,
		PublicBaseURL:   baseURL,
		BGGAPIToken:     current.BGGAPIToken,
		AIBaseURL:       aiBaseURL,
		AIModel:         strings.TrimSpace(req.AIModel),
		AIAPIKey:        current.AIAPIKey,
	}
	if req.BGGAPIToken != "" {
		next.BGGAPIToken = req.BGGAPIToken
	}
	// Come il token BGG: una chiave vuota vuol dire "lascia quella che c'è",
	// perché il form la rimanda vuota dopo ogni salvataggio.
	if req.AIAPIKey != "" {
		next.AIAPIKey = req.AIAPIKey
	}

	if err := s.Settings.Update(r.Context(), next); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save settings")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}
