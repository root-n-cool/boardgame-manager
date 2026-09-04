package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"boardgames-manager/internal/mailer"
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
	// Host, porta, utente, mittente e modo TLS sono dati da rileggere e
	// controllare, come PublicBaseURL; la password è un segreto e segue
	// BGGAPIToken. SMTPConfigured è il booleano su cui la UI abilita la
	// prova d'invio: serve host, porta e mittente, non la password.
	SMTPHost           string `json:"smtpHost"`
	SMTPPort           int    `json:"smtpPort"`
	SMTPUsername       string `json:"smtpUsername"`
	SMTPFromAddress    string `json:"smtpFromAddress"`
	SMTPFromName       string `json:"smtpFromName"`
	SMTPTLSMode        string `json:"smtpTlsMode"`
	SMTPPasswordSet    bool   `json:"smtpPasswordSet"`
	SMTPPasswordMasked string `json:"smtpPasswordMasked,omitempty"`
	SMTPConfigured     bool   `json:"smtpConfigured"`
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
	resp.SMTPHost = cfg.SMTPHost
	resp.SMTPPort = cfg.SMTPPort
	resp.SMTPUsername = cfg.SMTPUsername
	resp.SMTPFromAddress = cfg.SMTPFromAddress
	resp.SMTPFromName = cfg.SMTPFromName
	resp.SMTPTLSMode = cfg.SMTPTLSMode
	resp.SMTPPasswordSet = cfg.SMTPPassword != ""
	if resp.SMTPPasswordSet {
		resp.SMTPPasswordMasked = maskKey(cfg.SMTPPassword)
	}
	resp.SMTPConfigured = smtpConfigFrom(cfg).Configured()
	writeJSON(w, http.StatusOK, resp)
}

type updateSettingsRequest struct {
	DefaultLanguage string        `json:"defaultLanguage"`
	PublicBaseURL   string        `json:"publicBaseUrl"`
	BGGAPIToken     string        `json:"bggApiToken"`
	AIBaseURL       string        `json:"aiBaseUrl"`
	AIAPIKey        string        `json:"aiApiKey"`
	AIModel         string        `json:"aiModel"`
	SMTPHost        string        `json:"smtpHost"`
	SMTPPort        smtpPortValue `json:"smtpPort"`
	SMTPUsername    string        `json:"smtpUsername"`
	SMTPPassword    string        `json:"smtpPassword"`
	SMTPFromAddress string        `json:"smtpFromAddress"`
	SMTPFromName    string        `json:"smtpFromName"`
	SMTPTLSMode     string        `json:"smtpTlsMode"`
}

// smtpPortValue decodes SMTPPort tolerantly. No SMTP field is required, so a
// malformed one must not fail the whole settings save: it can arrive as a
// JSON number, a numeric string, an empty string (a number input cleared by
// the admin decodes to "" client-side), or be missing entirely. Anything
// that is not a clean integer — including outright garbage — becomes 0,
// which the rest of the code already treats as "not set", same as before
// this type existed.
type smtpPortValue int

func (v *smtpPortValue) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(strings.Trim(string(data), `"`))
	if s == "" || s == "null" {
		*v = 0
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		*v = 0
		return nil
	}
	*v = smtpPortValue(n)
	return nil
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

// normalizeTLSMode accetta il vuoto — l'admin che non ha ancora scelto —
// e lo lascia vuoto: è mailer a trattarlo come STARTTLS. Un valore
// inventato invece è un errore, perché silenziosamente ripiegare su
// STARTTLS manderebbe la password su una connessione che l'admin credeva
// configurata diversamente.
func normalizeTLSMode(raw string) (string, bool) {
	switch mode := strings.ToLower(strings.TrimSpace(raw)); mode {
	case "", mailer.TLSModeSTARTTLS, mailer.TLSModeImplicit, mailer.TLSModeNone:
		return mode, true
	default:
		return "", false
	}
}

func (s *Server) putSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DefaultLanguage == "" {
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

	tlsMode, ok := normalizeTLSMode(req.SMTPTLSMode)
	if !ok {
		writeError(w, http.StatusBadRequest, "la sicurezza SMTP deve essere starttls, tls o none")
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
		SMTPHost:        strings.TrimSpace(req.SMTPHost),
		SMTPPort:        int(req.SMTPPort),
		SMTPUsername:    strings.TrimSpace(req.SMTPUsername),
		SMTPPassword:    current.SMTPPassword,
		SMTPFromAddress: strings.TrimSpace(req.SMTPFromAddress),
		SMTPFromName:    strings.TrimSpace(req.SMTPFromName),
		SMTPTLSMode:     tlsMode,
	}
	if req.BGGAPIToken != "" {
		next.BGGAPIToken = req.BGGAPIToken
	}
	// Come il token BGG: una chiave vuota vuol dire "lascia quella che c'è",
	// perché il form la rimanda vuota dopo ogni salvataggio.
	if req.AIAPIKey != "" {
		next.AIAPIKey = req.AIAPIKey
	}
	// Come le altre due credenziali: vuota vuol dire "lascia quella che
	// c'è", perché il form la rimanda vuota dopo ogni salvataggio.
	if req.SMTPPassword != "" {
		next.SMTPPassword = req.SMTPPassword
	}

	if err := s.Settings.Update(r.Context(), next); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save settings")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// smtpConfigFrom traduce le impostazioni salvate nella Config del package
// mailer. Vive qui perché la usano sia la risposta di GET /api/settings
// (per smtpConfigured) sia la glue di invio.
func smtpConfigFrom(cfg settings.Settings) mailer.Config {
	return mailer.Config{
		Host:        cfg.SMTPHost,
		Port:        cfg.SMTPPort,
		Username:    cfg.SMTPUsername,
		Password:    cfg.SMTPPassword,
		FromAddress: cfg.SMTPFromAddress,
		FromName:    cfg.SMTPFromName,
		TLSMode:     cfg.SMTPTLSMode,
	}
}

// testSMTPHandler manda una mail all'admin in sessione e riferisce
// l'esito. È l'unica eccezione al silenzio sugli errori di posta: qui
// l'admin ha premuto un bottone, e un guasto muto lo lascerebbe a
// indovinare fra host, porta, cifratura e password.
//
// Prova la configurazione salvata, non quella nel form: è quella che
// verrà usata davvero, e la UI dice di salvare prima.
func (s *Server) testSMTPHandler(w http.ResponseWriter, r *http.Request) {
	admin, ok := currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), mailSendTimeout)
	defer cancel()

	err := s.mailSender(ctx).Send(ctx, smtpTestMail(admin.Email))
	if errors.Is(err, mailer.ErrNotConfigured) {
		writeError(w, http.StatusConflict, "SMTP non configurato: compila server, porta e indirizzo mittente, poi salva")
		return
	}
	if err != nil {
		log.Printf("smtp test send to %s: %v", admin.Email, err)
		// Il messaggio del provider esce così com'è: è rivolto a un
		// admin autenticato, ed è l'informazione che serve.
		writeError(w, http.StatusBadGateway, "invio non riuscito: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "sent", "to": admin.Email})
}
