# Branding del sito e pagine legali — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rendere il sito rimarchizzabile dal pannello impostazioni (titolo HTML, favicon, logo nell'header) e pubblicare due pagine legali a URL fisso (`/terms`, `/privacy`) il cui contenuto Markdown si edita dall'admin.

**Architecture:** Cinque nuove colonne opzionali su `app_settings` (stesso pattern delle sezioni SMTP/AI già esistenti). Logo e favicon sono upload che riusano `storage.CoverCategory` e si servono da `/api/uploads/{filename}`, già pubblico. `webui.Handler()` inietta titolo e favicon in `index.html` a runtime tramite due placeholder testuali. Due nuovi endpoint pubblici (`/api/site`, `/api/legal/{terms,privacy}`) alimentano header, footer e le due nuove view Vue pubbliche.

**Tech Stack:** Go 1.25 (chi, modernc.org/sqlite), Vue 3 + TypeScript + Pinia, `md-editor-v3` (già in uso per `MarkdownEditor.vue`).

**Spec:** `docs/superpowers/specs/2026-09-15-branding-pagine-legali-design.md`

## Global Constraints

- Nessun campo è obbligatorio: titolo, logo, favicon, termini e privacy vuoti sono tutti stati validi, mai un errore (vedi la sezione "Invariante" della spec).
- Upload di logo e favicon: solo JPEG/PNG/WebP, riusando `storage.CoverCategory` (5MB) così com'è — nessuna nuova categoria di storage.
- `GET /api/settings` (admin) non applica mai il fallback "BoardGames Manager" a `siteTitle`: esce grezzo, così il form non scambia il default per un valore salvato. Il fallback si applica solo in lettura pubblica (`GET /api/site`, `webui`, email).
- Un solo titolo `<title>` globale per tutte le rotte — niente titoli per pagina.
- Comandi Go **solo** in Docker:
  ```bash
  docker run --rm -v "$(pwd)/backend:/app" \
    -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
    -w /app golang:1.25 go test ./...
  ```
- `npm run build` (in `frontend/`) gira in locale, senza Docker, e fa anche il type-check (`vue-tsc -b`).
- Lingua UI: italiano, nessun i18n, stringhe dirette nei componenti.
- Ultimo task della todo: pass `/impeccable` sulla superficie toccata (header, footer, pagine legali, sezione "Sito").

---

## Task 1: Migrazione e `settings.Store`

**Files:**
- Create: `backend/internal/db/migrations/0019_site_branding.sql`
- Modify: `backend/internal/settings/store.go`
- Test: `backend/internal/settings/store_test.go`

**Interfaces:**
- Produces: `settings.Settings` guadagna i campi `SiteTitle`, `LogoFilename`, `FaviconFilename`, `TermsMarkdown`, `PrivacyMarkdown` (tutti `string`, vuoto = non impostato). Ogni task successivo che legge/scrive impostazioni usa questi nomi esatti.

- [ ] **Step 1: Scrivi il test che fallisce**

Aggiungi in fondo a `backend/internal/settings/store_test.go`:

```go
func TestUpdateAndGet_RoundTripsSiteBranding(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	in := settings.Settings{
		DefaultLanguage:  "it",
		SiteTitle:        "Ludoteca Vicolo Corto",
		LogoFilename:     "abc123.png",
		FaviconFilename:  "def456.png",
		TermsMarkdown:    "# Termini\n\nTesto.",
		PrivacyMarkdown:  "# Privacy\n\nTesto.",
	}
	if err := store.Update(ctx, in); err != nil {
		t.Fatalf("update: %v", err)
	}

	out, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.SiteTitle != in.SiteTitle || out.LogoFilename != in.LogoFilename ||
		out.FaviconFilename != in.FaviconFilename || out.TermsMarkdown != in.TermsMarkdown ||
		out.PrivacyMarkdown != in.PrivacyMarkdown {
		t.Fatalf("unexpected branding settings after update: %+v", out)
	}
}

// Un'installazione appena migrata deve leggere questi campi come stringa
// vuota, non come errore: sono tutti opzionali, come public_base_url.
func TestGet_SiteBrandingEmptyAfterMigration(t *testing.T) {
	store := newTestStore(t)
	out, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.SiteTitle != "" || out.LogoFilename != "" || out.FaviconFilename != "" ||
		out.TermsMarkdown != "" || out.PrivacyMarkdown != "" {
		t.Fatalf("expected empty site branding on a fresh instance, got %+v", out)
	}
}
```

- [ ] **Step 2: Verifica che fallisca**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/settings/... -run SiteBranding -v
```
Atteso: FAIL — `settings.Settings` non ha ancora i campi `SiteTitle` ecc. (errore di compilazione).

- [ ] **Step 3: Migrazione**

Crea `backend/internal/db/migrations/0019_site_branding.sql`:

```sql
-- Titolo del sito, logo e favicon personalizzabili, e i testi (in
-- Markdown) delle pagine /terms e /privacy. Tutti opzionali: NULL vuol
-- dire "non configurato", non un errore — stessa filosofia di
-- public_base_url.
ALTER TABLE app_settings ADD COLUMN site_title TEXT;
ALTER TABLE app_settings ADD COLUMN logo_filename TEXT;
ALTER TABLE app_settings ADD COLUMN favicon_filename TEXT;
ALTER TABLE app_settings ADD COLUMN terms_markdown TEXT;
ALTER TABLE app_settings ADD COLUMN privacy_markdown TEXT;
```

- [ ] **Step 4: Estendi `settings.Store`**

In `backend/internal/settings/store.go`, aggiungi i campi alla struct `Settings` (dopo `SMTPTLSMode`):

```go
	SMTPTLSMode     string
	// SiteTitle, LogoFilename, FaviconFilename, TermsMarkdown e
	// PrivacyMarkdown rimarchizzano l'installazione: nessuno è un
	// segreto, tutti escono in chiaro come PublicBaseURL. Vuoto è lo
	// stato di partenza legittimo, non un errore.
	SiteTitle       string
	LogoFilename    string
	FaviconFilename string
	TermsMarkdown   string
	PrivacyMarkdown string
}
```

Sostituisci il corpo di `Get`:

```go
func (s *Store) Get(ctx context.Context) (Settings, error) {
	var out Settings
	var baseURL, bggToken, aiBaseURL, aiAPIKey, aiModel, aiVisionModel sql.NullString
	var smtpHost, smtpUser, smtpPass, smtpFrom, smtpFromName, smtpTLS sql.NullString
	var smtpPort sql.NullInt64
	var siteTitle, logoFilename, faviconFilename, termsMarkdown, privacyMarkdown sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT default_language, public_base_url, bgg_api_token, ai_base_url, ai_api_key, ai_model, ai_vision_model,
		        smtp_host, smtp_port, smtp_username, smtp_password, smtp_from_address, smtp_from_name, smtp_tls_mode,
		        site_title, logo_filename, favicon_filename, terms_markdown, privacy_markdown
		 FROM app_settings WHERE id = 1`,
	).Scan(&out.DefaultLanguage, &baseURL, &bggToken, &aiBaseURL, &aiAPIKey, &aiModel, &aiVisionModel,
		&smtpHost, &smtpPort, &smtpUser, &smtpPass, &smtpFrom, &smtpFromName, &smtpTLS,
		&siteTitle, &logoFilename, &faviconFilename, &termsMarkdown, &privacyMarkdown)
	if err != nil {
		return Settings{}, err
	}
	out.PublicBaseURL = baseURL.String
	out.BGGAPIToken = bggToken.String
	out.AIBaseURL = aiBaseURL.String
	out.AIAPIKey = aiAPIKey.String
	out.AIModel = aiModel.String
	out.AIVisionModel = aiVisionModel.String
	out.SMTPHost = smtpHost.String
	out.SMTPPort = int(smtpPort.Int64)
	out.SMTPUsername = smtpUser.String
	out.SMTPPassword = smtpPass.String
	out.SMTPFromAddress = smtpFrom.String
	out.SMTPFromName = smtpFromName.String
	out.SMTPTLSMode = smtpTLS.String
	out.SiteTitle = siteTitle.String
	out.LogoFilename = logoFilename.String
	out.FaviconFilename = faviconFilename.String
	out.TermsMarkdown = termsMarkdown.String
	out.PrivacyMarkdown = privacyMarkdown.String
	return out, nil
}
```

Sostituisci il corpo di `Update`:

```go
func (s *Store) Update(ctx context.Context, in Settings) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE app_settings SET default_language = ?, public_base_url = ?, bgg_api_token = ?,
		 ai_base_url = ?, ai_api_key = ?, ai_model = ?, ai_vision_model = ?,
		 smtp_host = ?, smtp_port = ?, smtp_username = ?, smtp_password = ?,
		 smtp_from_address = ?, smtp_from_name = ?, smtp_tls_mode = ?,
		 site_title = ?, logo_filename = ?, favicon_filename = ?, terms_markdown = ?, privacy_markdown = ?
		 WHERE id = 1`,
		in.DefaultLanguage, nullIfEmpty(in.PublicBaseURL), nullIfEmpty(in.BGGAPIToken),
		nullIfEmpty(in.AIBaseURL), nullIfEmpty(in.AIAPIKey), nullIfEmpty(in.AIModel), nullIfEmpty(in.AIVisionModel),
		nullIfEmpty(in.SMTPHost), nullIfZero(in.SMTPPort), nullIfEmpty(in.SMTPUsername),
		nullIfEmpty(in.SMTPPassword), nullIfEmpty(in.SMTPFromAddress),
		nullIfEmpty(in.SMTPFromName), nullIfEmpty(in.SMTPTLSMode),
		nullIfEmpty(in.SiteTitle), nullIfEmpty(in.LogoFilename), nullIfEmpty(in.FaviconFilename),
		nullIfEmpty(in.TermsMarkdown), nullIfEmpty(in.PrivacyMarkdown),
	)
	return err
}
```

- [ ] **Step 5: Verifica che passi**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/settings/... -v
```
Atteso: PASS su tutti i test del package (i vecchi e i due nuovi).

- [ ] **Step 6: Commit**

```bash
git add backend/internal/db/migrations/0019_site_branding.sql backend/internal/settings/store.go backend/internal/settings/store_test.go
git commit -m "feat: add site branding and legal pages columns to settings"
```

---

## Task 2: `GET /api/site` (pubblico)

**Files:**
- Create: `backend/internal/httpapi/site_handlers.go`
- Create: `backend/internal/httpapi/site_handlers_test.go`
- Modify: `backend/internal/httpapi/router.go`

**Interfaces:**
- Consumes: `s.Settings.Get(ctx) (settings.Settings, error)` da Task 1.
- Produces: `GET /api/site` → `{"siteTitle": string, "logoFilename": string}`. `siteTitle` è **sempre** valorizzato (fallback `"BoardGames Manager"` applicato qui); `logoFilename` è `""` quando non impostato — il frontend (Task 8) lo confronta con la stringa vuota, non con `null`.

- [ ] **Step 1: Scrivi il test che fallisce**

Crea `backend/internal/httpapi/site_handlers_test.go`:

```go
package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func TestGetSite_NoAuthRequired(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/site", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetSite_DefaultsToBoardGamesManagerWithNoLogo(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/site", nil))

	var body struct {
		SiteTitle    string `json:"siteTitle"`
		LogoFilename string `json:"logoFilename"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.SiteTitle != "BoardGames Manager" {
		t.Errorf("siteTitle = %q, atteso il fallback", body.SiteTitle)
	}
	if body.LogoFilename != "" {
		t.Errorf("logoFilename = %q, atteso vuoto senza logo caricato", body.LogoFilename)
	}
}

func TestGetSite_ReturnsTheConfiguredTitleAndLogo(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)

	if _, err := conn.Exec(`UPDATE app_settings SET site_title = ?, logo_filename = ? WHERE id = 1`,
		"Ludoteca Vicolo Corto", "abc123.png"); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/site", nil))

	var body struct {
		SiteTitle    string `json:"siteTitle"`
		LogoFilename string `json:"logoFilename"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.SiteTitle != "Ludoteca Vicolo Corto" {
		t.Errorf("siteTitle = %q", body.SiteTitle)
	}
	if body.LogoFilename != "abc123.png" {
		t.Errorf("logoFilename = %q", body.LogoFilename)
	}
}
```

- [ ] **Step 2: Verifica che fallisca**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/... -run TestGetSite -v
```
Atteso: FAIL — `/api/site` non esiste (404) o il pacchetto non compila (`getSiteHandler` non definito una volta scritto il router).

- [ ] **Step 3: Implementa l'handler**

Crea `backend/internal/httpapi/site_handlers.go`:

```go
package httpapi

import (
	"log"
	"net/http"
)

// defaultSiteTitle è il nome che l'app porta finché nessuno lo cambia
// dal pannello impostazioni.
const defaultSiteTitle = "BoardGames Manager"

// siteResponse è quello che l'header (con o senza sessione) legge per
// mostrare il marchio del sito: mai un segreto, sempre leggibile senza
// autenticazione.
type siteResponse struct {
	SiteTitle    string `json:"siteTitle"`
	LogoFilename string `json:"logoFilename"`
}

// getSiteHandler è pubblico apposta: l'header compare anche su /login,
// /setup e le pagine pubbliche, prima che una sessione esista.
func (s *Server) getSiteHandler(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.Settings.Get(r.Context())
	if err != nil {
		log.Printf("site: could not load settings, serving defaults: %v", err)
		writeJSON(w, http.StatusOK, siteResponse{SiteTitle: defaultSiteTitle})
		return
	}

	title := cfg.SiteTitle
	if title == "" {
		title = defaultSiteTitle
	}
	writeJSON(w, http.StatusOK, siteResponse{SiteTitle: title, LogoFilename: cfg.LogoFilename})
}
```

In `backend/internal/httpapi/router.go`, aggiungi la rotta pubblica subito dopo `r.Get("/api/health", healthHandler)`:

```go
	r.Get("/api/health", healthHandler)
	r.Get("/api/site", s.getSiteHandler)
	r.Get("/api/bootstrap/status", s.bootstrapStatusHandler)
```

- [ ] **Step 4: Verifica che passi**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/... -run TestGetSite -v
```
Atteso: PASS sui tre test.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/httpapi/site_handlers.go backend/internal/httpapi/site_handlers_test.go backend/internal/httpapi/router.go
git commit -m "feat: expose GET /api/site for the public header branding"
```

---

## Task 3: Estendi `GET`/`PUT /api/settings` con titolo e testi legali

**Files:**
- Modify: `backend/internal/httpapi/settings_handlers.go`
- Test: `backend/internal/httpapi/settings_handlers_test.go`

**Interfaces:**
- Produces: `settingsResponse` guadagna `siteTitle` (grezzo, no fallback), `logoFilename`, `faviconFilename` (sola lettura, valorizzati solo da Task 4), `termsMarkdown`, `privacyMarkdown`. `updateSettingsRequest` guadagna `siteTitle`, `termsMarkdown`, `privacyMarkdown` (non `logoFilename`/`faviconFilename`: quelli si scrivono solo dagli endpoint di upload di Task 4).

- [ ] **Step 1: Scrivi il test che fallisce**

Aggiungi in fondo a `backend/internal/httpapi/settings_handlers_test.go`:

```go
func TestPutSettings_SavesSiteTitleRawWithoutTheFallback(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	// Il fallback "BoardGames Manager" non è ancora impostato: la lettura
	// grezza deve tornare la stringa vuota, non il fallback che GET
	// /api/site userebbe.
	if got := getSettings(t, router, cookie)["siteTitle"]; got != "" {
		t.Fatalf("expected an empty siteTitle before it is set, got %v", got)
	}

	rec := putSettings(t, router, cookie, map[string]string{
		"defaultLanguage": "it",
		"siteTitle":       "Ludoteca Vicolo Corto",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := getSettings(t, router, cookie)["siteTitle"]; got != "Ludoteca Vicolo Corto" {
		t.Fatalf("expected the saved site title, got %v", got)
	}
}

func TestPutSettings_SavesTermsAndPrivacyMarkdown(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	rec := putSettings(t, router, cookie, map[string]string{
		"defaultLanguage": "it",
		"termsMarkdown":   "# Termini\n\nTesto di prova.",
		"privacyMarkdown": "# Privacy\n\nTesto di prova.",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	got := getSettings(t, router, cookie)
	if got["termsMarkdown"] != "# Termini\n\nTesto di prova." {
		t.Errorf("termsMarkdown = %v", got["termsMarkdown"])
	}
	if got["privacyMarkdown"] != "# Privacy\n\nTesto di prova." {
		t.Errorf("privacyMarkdown = %v", got["privacyMarkdown"])
	}
}
```

- [ ] **Step 2: Verifica che fallisca**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/... -run "TestPutSettings_SavesSiteTitle|TestPutSettings_SavesTermsAndPrivacy" -v
```
Atteso: FAIL — i campi non arrivano ancora nella risposta/non vengono ancora salvati.

- [ ] **Step 3: Estendi handler e tipi**

In `backend/internal/httpapi/settings_handlers.go`, aggiungi campi a `settingsResponse` (dopo `SMTPConfigured`):

```go
	SMTPConfigured     bool   `json:"smtpConfigured"`
	// SiteTitle esce grezzo (vuoto se non impostato): è il campo che
	// l'admin edita, e mostrargli già "BoardGames Manager" lo farebbe
	// sembrare un valore salvato quando non lo è. Il fallback si applica
	// solo in GET /api/site, nel webui e nelle email.
	SiteTitle       string `json:"siteTitle"`
	LogoFilename    string `json:"logoFilename"`
	FaviconFilename string `json:"faviconFilename"`
	TermsMarkdown   string `json:"termsMarkdown"`
	PrivacyMarkdown string `json:"privacyMarkdown"`
}
```

In `getSettingsHandler`, prima di `writeJSON(w, http.StatusOK, resp)`:

```go
	resp.SiteTitle = cfg.SiteTitle
	resp.LogoFilename = cfg.LogoFilename
	resp.FaviconFilename = cfg.FaviconFilename
	resp.TermsMarkdown = cfg.TermsMarkdown
	resp.PrivacyMarkdown = cfg.PrivacyMarkdown
	writeJSON(w, http.StatusOK, resp)
```

Aggiungi campi a `updateSettingsRequest` (dopo `SMTPTLSMode`):

```go
	SMTPTLSMode     string        `json:"smtpTlsMode"`
	SiteTitle       string        `json:"siteTitle"`
	TermsMarkdown   string        `json:"termsMarkdown"`
	PrivacyMarkdown string        `json:"privacyMarkdown"`
}
```

In `putSettingsHandler`, nella costruzione di `next` — `LogoFilename` e `FaviconFilename` vengono da `current`, non dalla richiesta: solo gli endpoint di upload di Task 4 li cambiano, e un `PUT /api/settings` non deve mai azzerarli.

```go
	next := settings.Settings{
		DefaultLanguage: req.DefaultLanguage,
		PublicBaseURL:   baseURL,
		BGGAPIToken:     current.BGGAPIToken,
		AIBaseURL:       aiBaseURL,
		AIModel:         strings.TrimSpace(req.AIModel),
		AIVisionModel:   strings.TrimSpace(req.AIVisionModel),
		AIAPIKey:        current.AIAPIKey,
		SMTPHost:        strings.TrimSpace(req.SMTPHost),
		SMTPPort:        int(req.SMTPPort),
		SMTPUsername:    strings.TrimSpace(req.SMTPUsername),
		SMTPPassword:    current.SMTPPassword,
		SMTPFromAddress: strings.TrimSpace(req.SMTPFromAddress),
		SMTPFromName:    strings.TrimSpace(req.SMTPFromName),
		SMTPTLSMode:     tlsMode,
		SiteTitle:       strings.TrimSpace(req.SiteTitle),
		LogoFilename:    current.LogoFilename,
		FaviconFilename: current.FaviconFilename,
		TermsMarkdown:   req.TermsMarkdown,
		PrivacyMarkdown: req.PrivacyMarkdown,
	}
```

(`TermsMarkdown`/`PrivacyMarkdown` non passano per `strings.TrimSpace`: un admin potrebbe volere del Markdown che inizia con spazi rilevanti in un blocco di codice — a differenza dei campi di configurazione qui sopra, questo è testo libero.)

- [ ] **Step 4: Verifica che passi**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/... -run TestPutSettings -v
```
Atteso: PASS su tutti i test di `PutSettings` (vecchi e nuovi).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/httpapi/settings_handlers.go backend/internal/httpapi/settings_handlers_test.go
git commit -m "feat: manage site title and legal pages markdown from settings"
```

---

## Task 4: Upload di logo e favicon

**Files:**
- Create: `backend/internal/httpapi/settings_branding_handlers.go`
- Create: `backend/internal/httpapi/settings_branding_handlers_test.go`
- Modify: `backend/internal/httpapi/router.go`

**Interfaces:**
- Consumes: `storage.CoverCategory` (`backend/internal/storage/store.go`), `s.Storage.Save(category, r, filename) (string, error)`, `s.Settings.Get`/`Update`.
- Produces: `POST /api/settings/logo` e `POST /api/settings/favicon` (dentro il gruppo `protected`), multipart `file`, risposta `{"status": "saved"}`. Aggiornano rispettivamente `logo_filename`/`favicon_filename` senza toccare il resto della riga.

- [ ] **Step 1: Scrivi il test che fallisce**

Crea `backend/internal/httpapi/settings_branding_handlers_test.go`:

```go
package httpapi_test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

// tinyPNG produce un PNG valido di un pixel: basta a passare la
// convalida di storage.CoverCategory (estensione + tipo sniffato).
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func uploadFile(t *testing.T, router http.Handler, cookie *http.Cookie, path, filename string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestUploadLogo_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/settings/logo", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestUploadLogo_SavesTheFilenameAndItIsReadableBySite(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	rec := uploadFile(t, router, cookie, "/api/settings/logo", "logo.png", tinyPNG(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	filename, _ := getSettings(t, router, cookie)["logoFilename"].(string)
	if filename == "" {
		t.Fatal("expected logoFilename to be set after upload")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/uploads/"+filename, nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected the uploaded logo to be servable, got %d", getRec.Code)
	}

	site := httptest.NewRecorder()
	router.ServeHTTP(site, httptest.NewRequest(http.MethodGet, "/api/site", nil))
	var siteBody struct {
		LogoFilename string `json:"logoFilename"`
	}
	if err := json.NewDecoder(site.Body).Decode(&siteBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if siteBody.LogoFilename != filename {
		t.Fatalf("GET /api/site logoFilename = %q, atteso %q", siteBody.LogoFilename, filename)
	}
}

func TestUploadLogo_RejectsAnUnsupportedType(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	rec := uploadFile(t, router, cookie, "/api/settings/logo", "logo.gif", []byte("GIF89a not a real gif"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadFavicon_SavesTheFilenameAndPreservesOtherSettings(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	// Un titolo già salvato non deve sparire dopo l'upload della favicon:
	// l'handler deve preservare il resto della riga, non sovrascriverla.
	if rec := putSettings(t, router, cookie, map[string]string{
		"defaultLanguage": "it",
		"siteTitle":       "Ludoteca Vicolo Corto",
	}); rec.Code != http.StatusOK {
		t.Fatalf("put settings: %d %s", rec.Code, rec.Body.String())
	}

	rec := uploadFile(t, router, cookie, "/api/settings/favicon", "favicon.png", tinyPNG(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	got := getSettings(t, router, cookie)
	if got["siteTitle"] != "Ludoteca Vicolo Corto" {
		t.Fatalf("expected the site title to survive the favicon upload, got %v", got["siteTitle"])
	}
	if got["faviconFilename"] == "" || got["faviconFilename"] == nil {
		t.Fatal("expected faviconFilename to be set after upload")
	}
}
```

- [ ] **Step 2: Verifica che fallisca**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/... -run "TestUploadLogo|TestUploadFavicon" -v
```
Atteso: FAIL — le rotte non esistono (404).

- [ ] **Step 3: Implementa gli handler**

Crea `backend/internal/httpapi/settings_branding_handlers.go`:

```go
package httpapi

import (
	"errors"
	"net/http"

	"boardgames-manager/internal/settings"
	"boardgames-manager/internal/storage"
)

// uploadBrandingFile è la parte comune a logo e favicon: valida e salva il
// file con storage.CoverCategory, poi scrive il filename risultante nella
// riga di app_settings tramite apply, senza toccare il resto della
// configurazione — current è lo stato letto appena prima, così un campo
// come SMTPPassword non sparisce mai per un upload che non lo riguarda.
func (s *Server) uploadBrandingFile(w http.ResponseWriter, r *http.Request, apply func(current *settings.Settings, filename string)) {
	if err := r.ParseMultipartForm(storage.CoverCategory.MaxBytes + 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	filename, err := s.Storage.Save(storage.CoverCategory, file, header.Filename)
	if errors.Is(err, storage.ErrUnsupportedType) {
		writeError(w, http.StatusBadRequest, "only JPEG, PNG or WebP images are allowed")
		return
	}
	if errors.Is(err, storage.ErrTooLarge) {
		writeError(w, http.StatusBadRequest, "file exceeds the 5MB limit")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save file")
		return
	}

	current, err := s.Settings.Get(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settings")
		return
	}
	apply(&current, filename)
	if err := s.Settings.Update(r.Context(), current); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save settings")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) uploadLogoHandler(w http.ResponseWriter, r *http.Request) {
	s.uploadBrandingFile(w, r, func(current *settings.Settings, filename string) {
		current.LogoFilename = filename
	})
}

func (s *Server) uploadFaviconHandler(w http.ResponseWriter, r *http.Request) {
	s.uploadBrandingFile(w, r, func(current *settings.Settings, filename string) {
		current.FaviconFilename = filename
	})
}
```

In `backend/internal/httpapi/router.go`, dentro il gruppo `protected`, subito dopo la riga dell'SMTP test:

```go
		protected.With(smtpTestLimiter.middleware).Post("/api/settings/smtp/test", s.testSMTPHandler)
		protected.Post("/api/settings/logo", s.uploadLogoHandler)
		protected.Post("/api/settings/favicon", s.uploadFaviconHandler)
```

- [ ] **Step 4: Verifica che passi**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/... -run "TestUploadLogo|TestUploadFavicon" -v
```
Atteso: PASS su tutti e quattro.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/httpapi/settings_branding_handlers.go backend/internal/httpapi/settings_branding_handlers_test.go backend/internal/httpapi/router.go
git commit -m "feat: let the admin upload a custom logo and favicon"
```

---

## Task 5: `GET /api/legal/terms` e `GET /api/legal/privacy` (pubblici)

**Files:**
- Create: `backend/internal/httpapi/legal_handlers.go`
- Create: `backend/internal/httpapi/legal_handlers_test.go`
- Modify: `backend/internal/httpapi/router.go`

**Interfaces:**
- Produces: `GET /api/legal/terms` e `GET /api/legal/privacy` → `{"markdown": string}` (`""` se non configurato, mai 404).

- [ ] **Step 1: Scrivi il test che fallisce**

Crea `backend/internal/httpapi/legal_handlers_test.go`:

```go
package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func legalMarkdown(t *testing.T, router http.Handler, path string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body.String())
	}
	var body struct {
		Markdown string `json:"markdown"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.Markdown
}

func TestGetLegalTerms_EmptyByDefaultNever404(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	if got := legalMarkdown(t, router, "/api/legal/terms"); got != "" {
		t.Fatalf("expected empty terms by default, got %q", got)
	}
}

func TestGetLegalPrivacy_ReturnsTheConfiguredMarkdown(t *testing.T) {
	server, conn := newTestServerWithDB(t)
	router := httpapi.NewRouter(server)

	if _, err := conn.Exec(`UPDATE app_settings SET privacy_markdown = ? WHERE id = 1`, "# Privacy\n\nTesto."); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if got := legalMarkdown(t, router, "/api/legal/privacy"); got != "# Privacy\n\nTesto." {
		t.Fatalf("markdown = %q", got)
	}
}
```

- [ ] **Step 2: Verifica che fallisca**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/... -run TestGetLegal -v
```
Atteso: FAIL — le rotte non esistono.

- [ ] **Step 3: Implementa gli handler**

Crea `backend/internal/httpapi/legal_handlers.go`:

```go
package httpapi

import (
	"log"
	"net/http"
)

type legalPageResponse struct {
	Markdown string `json:"markdown"`
}

// getTermsHandler e getPrivacyHandler rispondono sempre 200: un testo
// vuoto è uno stato valido (l'admin non ha ancora scritto niente), e la
// view pubblica mostra un messaggio neutro invece di un errore — vedi
// TermsView/PrivacyView.
func (s *Server) getTermsHandler(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.Settings.Get(r.Context())
	if err != nil {
		log.Printf("legal: could not load settings, serving empty terms: %v", err)
		writeJSON(w, http.StatusOK, legalPageResponse{})
		return
	}
	writeJSON(w, http.StatusOK, legalPageResponse{Markdown: cfg.TermsMarkdown})
}

func (s *Server) getPrivacyHandler(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.Settings.Get(r.Context())
	if err != nil {
		log.Printf("legal: could not load settings, serving empty privacy: %v", err)
		writeJSON(w, http.StatusOK, legalPageResponse{})
		return
	}
	writeJSON(w, http.StatusOK, legalPageResponse{Markdown: cfg.PrivacyMarkdown})
}
```

In `backend/internal/httpapi/router.go`, subito dopo `r.Get("/api/site", s.getSiteHandler)`:

```go
	r.Get("/api/site", s.getSiteHandler)
	r.Get("/api/legal/terms", s.getTermsHandler)
	r.Get("/api/legal/privacy", s.getPrivacyHandler)
```

- [ ] **Step 4: Verifica che passi**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/... -run TestGetLegal -v
```
Atteso: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/httpapi/legal_handlers.go backend/internal/httpapi/legal_handlers_test.go backend/internal/httpapi/router.go
git commit -m "feat: serve the terms and privacy markdown publicly"
```

---

## Task 6: Titolo e favicon iniettati in `index.html`

**Files:**
- Modify: `backend/internal/webui/embed.go`
- Modify: `backend/internal/webui/handler_internal_test.go`
- Modify: `backend/internal/webui/embed_test.go`
- Modify: `backend/cmd/server/main.go`
- Modify: `frontend/index.html`

**Interfaces:**
- Consumes: `settings.Settings` (campi `SiteTitle`, `FaviconFilename`) da Task 1.
- Produces: `webui.Handler(store SettingsReader) (http.Handler, error)` — firma cambiata, ogni chiamante (solo `main.go`) passa ora `*settings.Store`. `webui.SettingsReader` è l'interfaccia `Get(ctx context.Context) (settings.Settings, error)`, che `*settings.Store` soddisfa già.

- [ ] **Step 1: Scrivi il test che fallisce**

Sostituisci il contenuto di `backend/internal/webui/handler_internal_test.go`:

```go
package webui

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"boardgames-manager/internal/settings"
)

type fakeSettingsReader struct {
	cfg settings.Settings
	err error
}

func (f fakeSettingsReader) Get(ctx context.Context) (settings.Settings, error) {
	return f.cfg, f.err
}

func TestHandlerFor_ErrorsWhenIndexHTMLMissing(t *testing.T) {
	gitkeepOnly := fstest.MapFS{
		".gitkeep": {Data: []byte{}},
	}

	handler, err := handlerFor(gitkeepOnly, fakeSettingsReader{})
	if err == nil {
		t.Fatal("expected an error when dist/ has no index.html, got nil")
	}
	if handler != nil {
		t.Fatalf("expected a nil handler alongside the error, got %#v", handler)
	}
	if !strings.Contains(err.Error(), "index.html") {
		t.Errorf("error should name the missing file, got: %v", err)
	}
	if !strings.Contains(err.Error(), "npm run build") {
		t.Errorf("error should tell the operator how to fix it, got: %v", err)
	}
}

func indexFS(title string) fstest.MapFS {
	return fstest.MapFS{
		"index.html": {Data: []byte(
			`<title>__SITE_TITLE__</title><link rel="icon" href="__FAVICON_URL__" /><div id="app"></div>`,
		)},
		"assets/index.css": {Data: []byte(".layout{}")},
	}
}

func TestHandlerFor_ServesIndexHTMLForClientRoutes(t *testing.T) {
	handler, err := handlerFor(indexFS(""), fakeSettingsReader{})
	if err != nil {
		t.Fatalf("handlerFor: %v", err)
	}

	t.Run("unknown path falls back to index.html", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/users", nil))

		if rec.Code != 200 {
			t.Fatalf("got status %d, want 200", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `id="app"`) {
			t.Fatalf("expected index.html body, got %q", rec.Body.String())
		}
	})

	t.Run("real asset is served from the filesystem", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/assets/index.css", nil))

		if rec.Code != 200 {
			t.Fatalf("got status %d, want 200", rec.Code)
		}
		if got := rec.Body.String(); got != ".layout{}" {
			t.Fatalf("expected the asset body, got %q", got)
		}
	})
}

func TestHandlerFor_InjectsTheConfiguredTitleAndFavicon(t *testing.T) {
	handler, err := handlerFor(indexFS(""), fakeSettingsReader{
		cfg: settings.Settings{SiteTitle: "Ludoteca Vicolo Corto", FaviconFilename: "abc123.png"},
	})
	if err != nil {
		t.Fatalf("handlerFor: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	body := rec.Body.String()
	if !strings.Contains(body, "<title>Ludoteca Vicolo Corto</title>") {
		t.Errorf("expected the configured title, got: %s", body)
	}
	if !strings.Contains(body, `href="/api/uploads/abc123.png"`) {
		t.Errorf("expected the configured favicon URL, got: %s", body)
	}
}

func TestHandlerFor_FallsBackToDefaultsWhenNothingIsConfigured(t *testing.T) {
	handler, err := handlerFor(indexFS(""), fakeSettingsReader{})
	if err != nil {
		t.Fatalf("handlerFor: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	body := rec.Body.String()
	if !strings.Contains(body, "<title>BoardGames Manager</title>") {
		t.Errorf("expected the default title, got: %s", body)
	}
	if !strings.Contains(body, `href="/favicon.svg"`) {
		t.Errorf("expected the default favicon, got: %s", body)
	}
}

// Un DB irraggiungibile non deve mai rompere il caricamento della pagina:
// meglio i default che una pagina bianca.
func TestHandlerFor_FallsBackToDefaultsWhenSettingsFail(t *testing.T) {
	handler, err := handlerFor(indexFS(""), fakeSettingsReader{err: errTestSettingsUnavailable})
	if err != nil {
		t.Fatalf("handlerFor: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != 200 {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<title>BoardGames Manager</title>") || !strings.Contains(body, `href="/favicon.svg"`) {
		t.Errorf("expected the defaults on a settings error, got: %s", body)
	}
}
```

Aggiungi in cima al file, subito dopo il blocco `import`, l'errore usato solo da quest'ultimo test:

```go
var errTestSettingsUnavailable = errors.New("settings store unavailable")
```

- [ ] **Step 2: Verifica che fallisca**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/webui/... -v
```
Atteso: FAIL — `handlerFor` prende ancora un solo argomento, il pacchetto non compila.

- [ ] **Step 3: Implementa l'iniezione**

Sostituisci `backend/internal/webui/embed.go`:

```go
package webui

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"boardgames-manager/internal/settings"
)

// dist/.gitkeep is tracked so this pattern still matches on a fresh clone
// with no frontend build; handlerFor is what turns "no real build output"
// into a loud startup error instead of a browsable directory listing.
//
//go:embed dist/*
var distFS embed.FS

// defaultSiteTitle e defaultFaviconHref sono quello che index.html mostra
// finché nessuno configura un titolo o una favicon dal pannello
// impostazioni, o se le impostazioni non sono raggiungibili: mai una
// pagina rotta per un DB irraggiungibile.
const (
	defaultSiteTitle   = "BoardGames Manager"
	defaultFaviconHref = "/favicon.svg"
)

// SettingsReader è il sottoinsieme di settings.Store che serve qui: leggere
// il branding del sito da iniettare in index.html prima di servirlo.
// settings.Store lo soddisfa già, senza bisogno di adattatori; un finto lo
// soddisfa altrettanto facilmente nei test, senza un database vero.
type SettingsReader interface {
	Get(ctx context.Context) (settings.Settings, error)
}

func Handler(store SettingsReader) (http.Handler, error) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}
	return handlerFor(sub, store)
}

// handlerFor serves root as a single-page app: known paths come straight from
// the filesystem, everything else falls back to index.html so client-side
// routes survive a full page load. index.html itself (direct request or
// fallback) never comes straight from the filesystem: its <title> and
// favicon <link> carry two placeholders, __SITE_TITLE__ and
// __FAVICON_URL__ (see frontend/index.html), replaced per-request with the
// configured branding so a hard refresh — not just the SPA's own DOM
// updates — already shows the right title and favicon.
func handlerFor(root fs.FS, store SettingsReader) (http.Handler, error) {
	// Without this check an unbuilt frontend fails silently: the SPA fallback
	// asks http.FileServer for "/", which happily answers a directory with no
	// index file with a 200 and a browsable file listing.
	if _, err := fs.Stat(root, "index.html"); err != nil {
		return nil, fmt.Errorf("embedded frontend not built: dist/index.html not found — run 'npm run build' in frontend/ before building the Go binary: %w", err)
	}
	indexTemplate, err := fs.ReadFile(root, "index.html")
	if err != nil {
		return nil, fmt.Errorf("read embedded index.html: %w", err)
	}

	fileServer := http.FileServer(http.FS(root))

	serveIndex := func(w http.ResponseWriter, r *http.Request) {
		title, faviconHref := defaultSiteTitle, defaultFaviconHref
		if cfg, err := store.Get(r.Context()); err != nil {
			log.Printf("webui: could not load site settings, serving defaults: %v", err)
		} else {
			if cfg.SiteTitle != "" {
				title = cfg.SiteTitle
			}
			if cfg.FaviconFilename != "" {
				faviconHref = "/api/uploads/" + cfg.FaviconFilename
			}
		}
		body := strings.NewReplacer(
			"__SITE_TITLE__", title,
			"__FAVICON_URL__", faviconHref,
		).Replace(string(indexTemplate))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(body))
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if len(path) > 0 {
			path = path[1:]
		}
		if path == "" || path == "index.html" {
			serveIndex(w, r)
			return
		}
		if _, err := fs.Stat(root, path); err != nil {
			serveIndex(w, r)
			return
		}
		fileServer.ServeHTTP(w, r)
	}), nil
}
```

In `frontend/index.html`, sostituisci le due righe fisse:

```html
    <link rel="icon" type="image/svg+xml" href="/favicon.svg" />
```
```html
    <link rel="icon" href="__FAVICON_URL__" />
```

(il `type` fisso sparisce: una favicon caricata dall'admin può essere PNG o WebP, non solo SVG, e il browser non ne ha comunque bisogno per renderizzarla)

```html
    <title>BoardGames Manager</title>
```
```html
    <title>__SITE_TITLE__</title>
```

In `backend/cmd/server/main.go`, cambia la chiamata a `webui.Handler`:

```go
	uiHandler, err := webui.Handler(server.Settings)
```

- [ ] **Step 4: Aggiorna `embed_test.go`**

`backend/internal/webui/embed_test.go` chiama `webui.Handler()` senza argomenti: essendo un test package esterno (`webui_test`), gli basta un tipo che implementi `webui.SettingsReader`. Sostituisci il contenuto:

```go
package webui_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"boardgames-manager/internal/settings"
	"boardgames-manager/internal/webui"
)

type fakeSiteSettings struct{}

func (fakeSiteSettings) Get(ctx context.Context) (settings.Settings, error) {
	return settings.Settings{}, nil
}

// TestHandler_SPAFallbackDoesNotMutateOriginalRequest guards against a
// regression where constructing the index.html fallback request aliased the
// original request's *url.URL (via a shallow struct copy), causing writes to
// the fallback request's URL.Path to also mutate the caller's r.URL.Path as
// a side effect. Any middleware that inspects r.URL.Path after ServeHTTP
// returns (access logging, tracing, etc.) would then see the wrong path.
func TestHandler_SPAFallbackDoesNotMutateOriginalRequest(t *testing.T) {
	handler, err := webui.Handler(fakeSiteSettings{})
	if err != nil {
		t.Fatalf("webui.Handler: %v", err)
	}

	const clientRoute = "/users"
	req := httptest.NewRequest("GET", clientRoute, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if req.URL.Path != clientRoute {
		t.Fatalf("original request URL.Path mutated: got %q, want %q", req.URL.Path, clientRoute)
	}
	if rec.Code != 200 {
		t.Fatalf("unexpected status code: got %d, want 200", rec.Code)
	}
	// A 200 alone used to be satisfiable by a directory listing served out of
	// an unbuilt dist/, so assert we really got the SPA shell back.
	if body := rec.Body.String(); !strings.Contains(body, `id="app"`) {
		t.Fatalf("expected the SPA index.html for a client route, got: %s", body)
	}
}

// TestHandler_SucceedsWithEmbeddedBuildOutput is the positive half of the
// "frontend really got built" contract; see handlerFor's own tests for the
// missing-index.html case, which needs an alternate filesystem to exercise.
func TestHandler_SucceedsWithEmbeddedBuildOutput(t *testing.T) {
	if _, err := webui.Handler(fakeSiteSettings{}); err != nil {
		t.Fatalf("Handler() failed — is the frontend built? %v", err)
	}
}
```

- [ ] **Step 5: Verifica che passi**

Prima ricostruisci il frontend (`dist/index.html` deve avere i placeholder, altrimenti `TestHandler_SucceedsWithEmbeddedBuildOutput` gira contro un vecchio build):

```bash
cd frontend && npm run build && cd ..
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go build ./... && \
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/webui/... ./cmd/... -v
```
Atteso: PASS su tutti i test di `webui` e build pulita di `cmd/server`.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/webui/embed.go backend/internal/webui/handler_internal_test.go \
        backend/internal/webui/embed_test.go backend/cmd/server/main.go frontend/index.html
git commit -m "feat: inject the configured site title and favicon into index.html"
```

---

## Task 7: Titolo del sito nelle email

**Files:**
- Modify: `backend/internal/httpapi/mail.go`
- Modify: `backend/internal/httpapi/mail_templates.go`
- Modify: `backend/internal/httpapi/mail_templates_test.go`
- Modify: `backend/internal/httpapi/users_handlers.go`
- Modify: `backend/internal/httpapi/settings_handlers.go`

**Interfaces:**
- Produces: `func (s *Server) siteName(ctx context.Context) string` in `mail.go`. `inviteMail(siteName, to, invitedBy, inviteURL string) mailer.Message` e `smtpTestMail(siteName, to string) mailer.Message` guadagnano un primo parametro `siteName`.

- [ ] **Step 1: Scrivi il test che fallisce**

In `backend/internal/httpapi/mail_templates_test.go`, aggiorna le due chiamate esistenti e aggiungi due test sul nome configurabile. Sostituisci:

```go
	m := inviteMail("nuovo@example.com", "admin@example.com", "https://giochi.example.org/invito/tok123")
```
con:
```go
	m := inviteMail("BoardGames Manager", "nuovo@example.com", "admin@example.com", "https://giochi.example.org/invito/tok123")
```

Sostituisci:
```go
	m := smtpTestMail("admin@example.com")
```
con:
```go
	m := smtpTestMail("BoardGames Manager", "admin@example.com")
```

Sostituisci nella tabella dei test più in basso:
```go
		{"invito", inviteMail("a@b.org", "c@d.org", "https://x/invito/t")},
```
```go
		{"invito", inviteMail("BoardGames Manager", "a@b.org", "c@d.org", "https://x/invito/t")},
```
e:
```go
		{"prova", smtpTestMail("a@b.org")},
```
```go
		{"prova", smtpTestMail("BoardGames Manager", "a@b.org")},
```

Aggiungi in fondo al file:

```go
func TestInviteMail_UsesTheConfiguredSiteName(t *testing.T) {
	m := inviteMail("Ludoteca Vicolo Corto", "nuovo@example.com", "admin@example.com", "https://x/invito/t")
	for _, body := range bothBodies(t, m.Subject, m.TextBody, m.HTMLBody) {
		if !strings.Contains(body, "Ludoteca Vicolo Corto") {
			t.Errorf("expected the configured site name, got:\n%s", body)
		}
	}
}

func TestSMTPTestMail_UsesTheConfiguredSiteName(t *testing.T) {
	m := smtpTestMail("Ludoteca Vicolo Corto", "admin@example.com")
	for _, body := range bothBodies(t, m.Subject, m.TextBody, m.HTMLBody) {
		if !strings.Contains(body, "Ludoteca Vicolo Corto") {
			t.Errorf("expected the configured site name, got:\n%s", body)
		}
	}
}
```

- [ ] **Step 2: Verifica che fallisca**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/... -run "Mail" -v
```
Atteso: FAIL — il pacchetto non compila ancora (le firme non hanno il nuovo parametro).

- [ ] **Step 3: Threading del nome del sito**

In `backend/internal/httpapi/mail_templates.go`, cambia le firme e i corpi:

```go
func inviteMail(siteName, to, invitedBy, inviteURL string) mailer.Message {
	text := strings.Join([]string{
		"Ciao,",
		"",
		fmt.Sprintf("%s ti ha aggiunto come amministratore di %s.", invitedBy, siteName),
		"",
		"Apri questo link per scegliere la tua password ed entrare:",
		inviteURL,
		"",
		"Il link è personale e vale una volta sola: chi ti ha invitato non vedrà mai la password che scegli.",
	}, "\n")

	body := mailParagraph("Ciao,") +
		mailParagraph(fmt.Sprintf("%s ti ha aggiunto come amministratore di %s.", invitedBy, siteName)) +
		mailParagraph("Scegli la tua password ed entra:") +
		mailButton("Attiva il tuo accesso", inviteURL) +
		mailNote("Il link è personale e vale una volta sola: chi ti ha invitato non vedrà mai la password che scegli.")

	return mailer.Message{
		To:       to,
		Subject:  "Il tuo accesso da amministratore",
		TextBody: text,
		HTMLBody: mailShell(siteName, body),
	}
}
```

E:

```go
func smtpTestMail(siteName, to string) mailer.Message {
	text := strings.Join([]string{
		fmt.Sprintf("Se stai leggendo questa mail, la configurazione SMTP di %s funziona.", siteName),
		"",
		"Da qui in poi partiranno da sole: l'invito di un amministratore, la conferma di una prenotazione con il codice e i link, e l'avviso di annullamento.",
	}, "\n")

	body := mailParagraph(fmt.Sprintf("Se stai leggendo questa mail, la configurazione SMTP di %s funziona.", siteName)) +
		mailParagraph("Da qui in poi partiranno da sole: l'invito di un amministratore, la conferma di una prenotazione con il codice e i link, e l'avviso di annullamento.")

	return mailer.Message{
		To:       to,
		Subject:  fmt.Sprintf("Email di prova da %s", siteName),
		TextBody: text,
		HTMLBody: mailShell("Email di prova", body),
	}
}
```

In `backend/internal/httpapi/mail.go`, aggiungi accanto a `publicBaseURL`:

```go
// siteName è il nome che le email usano per presentare l'app: il titolo
// configurato dall'admin, o "BoardGames Manager" quando non è impostato —
// lo stesso fallback di GET /api/site e di webui.
func (s *Server) siteName(ctx context.Context) string {
	if s.Settings != nil {
		if cfg, err := s.Settings.Get(ctx); err == nil && cfg.SiteTitle != "" {
			return cfg.SiteTitle
		}
	}
	return defaultSiteTitle
}
```

In `backend/internal/httpapi/users_handlers.go`, aggiorna la chiamata:

```go
		s.sendMailAsync(s.mailSender(r.Context()), inviteMail(s.siteName(r.Context()), email, inviter, link))
```

In `backend/internal/httpapi/settings_handlers.go` (`testSMTPHandler`), aggiorna la chiamata:

```go
	err := s.mailSender(ctx).Send(ctx, smtpTestMail(s.siteName(r.Context()), admin.Email))
```

- [ ] **Step 4: Verifica che passi**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/... -v
```
Atteso: PASS sull'intero package (nessuna rottura nei test SMTP/inviti già esistenti).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/httpapi/mail.go backend/internal/httpapi/mail_templates.go \
        backend/internal/httpapi/mail_templates_test.go backend/internal/httpapi/users_handlers.go \
        backend/internal/httpapi/settings_handlers.go
git commit -m "feat: use the configured site name in admin invite and SMTP test emails"
```

---

## Task 8: Store Pinia del branding pubblico

**Files:**
- Create: `frontend/src/stores/site.ts`

**Interfaces:**
- Produces: `useSiteStore()` con stato `{ siteTitle: string, logoFilename: string, loaded: boolean }` e azione `load(): Promise<void>`. Task 9 consuma `site.siteTitle`, `site.logoFilename`; Task 11 chiama `load()` di nuovo dopo un upload per aggiornare l'anteprima.

- [ ] **Step 1: Implementa lo store**

Crea `frontend/src/stores/site.ts`:

```typescript
import { defineStore } from 'pinia'
import { api } from '../api/client'

interface SiteResponse {
  siteTitle: string
  logoFilename: string
}

// Letto senza autenticazione: l'header mostra marchio e titolo anche su
// /login, /setup e le pagine pubbliche, prima che una sessione esista.
export const useSiteStore = defineStore('site', {
  state: () => ({
    siteTitle: 'BoardGames Manager',
    logoFilename: '',
    loaded: false,
  }),
  actions: {
    async load() {
      try {
        const site = await api.get<SiteResponse>('/site', { skipAuthRedirect: true })
        this.siteTitle = site.siteTitle
        this.logoFilename = site.logoFilename
      } catch (e) {
        // Il backend irraggiungibile non deve rompere l'header: restano i
        // valori di default già nello stato iniziale.
        console.error('could not load site branding', e)
      } finally {
        this.loaded = true
      }
    },
  },
})
```

- [ ] **Step 2: Verifica il type-check**

```bash
cd frontend && npm run build
```
Atteso: build pulita (lo store non è ancora usato da nessun componente, ma deve compilare).

- [ ] **Step 3: Commit**

```bash
git add frontend/src/stores/site.ts
git commit -m "feat: add a Pinia store for the public site branding"
```

---

## Task 9: Header con logo/titolo e footer legale

**Files:**
- Modify: `frontend/src/components/AppShell.vue`
- Modify: `frontend/src/app.css`

**Interfaces:**
- Consumes: `useSiteStore()` da Task 8.

- [ ] **Step 1: Aggiorna lo script**

In `frontend/src/components/AppShell.vue`, aggiungi l'import e l'istanza dello store, e carica il branding in `onMounted` (fire-and-forget: non deve bloccare il primo render della shell). Vicino agli import esistenti:

```typescript
import { useAuthStore } from '../stores/auth'
import { useSiteStore } from '../stores/site'
import UserMenu from './UserMenu.vue'
```

Subito dopo `const auth = useAuthStore()`:

```typescript
const site = useSiteStore()
```

Nel blocco `onMounted` già esistente (quello che ripristina lo stato della sidebar), aggiungi la chiamata senza `await`:

```typescript
onMounted(() => {
  site.load()
  // ... resto del corpo esistente invariato
})
```

Se `onMounted` esistente è `async` per via del resto del corpo, aggiungi comunque `site.load()` senza `await` davanti (fuoco e dimentica): il resto della funzione non deve aspettare la risposta.

- [ ] **Step 2: Aggiorna il template dell'header**

Sostituisci il blocco `.brand`:

```html
      <router-link :to="{ name: 'events' }" class="brand">
        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <circle cx="12" cy="5.6" r="2.6" fill="currentColor" />
          <path
            d="M9 9.2h6c1.6 0 2.6.1 3.3.5l.1 3c.05.7-.6 1.2-1.3 1l-1.7-.5-.4 2c.6 1.8.8 3.6.6 5.4h-2.3l-.6-4.6h-1.4l-.6 4.6H8.4c-.2-1.8 0-3.6.6-5.4l-.4-2-1.7.5c-.7.2-1.35-.3-1.3-1l.1-3c.7-.4 1.7-.5 3.3-.5Z"
            fill="currentColor"
          />
        </svg>
        <span class="brand-name">BoardGames Manager</span>
      </router-link>
```

con:

```html
      <router-link :to="{ name: 'events' }" class="brand">
        <img
          v-if="site.logoFilename"
          :src="`/api/uploads/${site.logoFilename}`"
          class="brand-logo"
          alt=""
        />
        <template v-else>
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <circle cx="12" cy="5.6" r="2.6" fill="currentColor" />
            <path
              d="M9 9.2h6c1.6 0 2.6.1 3.3.5l.1 3c.05.7-.6 1.2-1.3 1l-1.7-.5-.4 2c.6 1.8.8 3.6.6 5.4h-2.3l-.6-4.6h-1.4l-.6 4.6H8.4c-.2-1.8 0-3.6.6-5.4l-.4-2-1.7.5c-.7.2-1.35-.3-1.3-1l.1-3c.7-.4 1.7-.5 3.3-.5Z"
              fill="currentColor"
            />
          </svg>
          <span class="brand-name">{{ site.siteTitle }}</span>
        </template>
      </router-link>
```

(`alt=""` sul logo: il link che lo racchiude porta già all'home, un testo alternativo qui duplicherebbe l'annuncio per chi usa uno screen reader)

- [ ] **Step 3: Aggiungi il footer legale**

Nel template, dentro `<main class="app-main">`, dopo `<div class="app-page"><slot /></div>` e prima della chiusura di `</main>`:

```html
      <main class="app-main" :inert="isDrawer">
        <div class="app-page">
          <slot />
        </div>
        <footer class="app-footer">
          <router-link to="/terms">Termini e condizioni</router-link>
          <span aria-hidden="true">·</span>
          <router-link to="/privacy">Privacy</router-link>
        </footer>
      </main>
```

- [ ] **Step 4: Stili**

In `frontend/src/app.css`, subito dopo la regola `.brand svg { ... }`:

```css
/* Il logo sostituisce icona e nome insieme: stessa altezza dell'icona che
   rimpiazza, larghezza libera per non deformare loghi non quadrati. */
.brand-logo {
  flex: none;
  height: 22px;
  width: auto;
}
```

Subito dopo la regola `.app-page { ... }` (e dopo la sua media query per il telefono):

```css
/* Striscia minima in fondo a ogni pagina della shell: le due pagine
   legali non hanno una voce nel menù, solo qui e da link diretto. */
.app-footer {
  display: flex;
  justify-content: center;
  gap: 0.6rem;
  width: 100%;
  max-width: 56rem;
  margin: 0 auto;
  padding: 1rem 1.5rem 2rem;
  font-size: 0.82rem;
  color: var(--ink-muted);
}

.app-footer a {
  color: inherit;
  text-decoration: underline;
  text-underline-offset: 0.15em;
}

@media (max-width: 640px) {
  .app-footer {
    padding: 1rem 1rem 1.75rem;
  }
}
```

- [ ] **Step 5: Build e verifica nel browser**

```bash
cd frontend && npm run build
```

Poi, con `docker compose up -d --build` attivo, apri http://localhost:8080 in Claude in Chrome: verifica che senza logo configurato l'header mostri ancora icona + "BoardGames Manager", che il footer con i due link compaia in fondo alla pagina, desktop e a 390px, e controlla la console per errori.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/AppShell.vue frontend/src/app.css
git commit -m "feat: show the configured logo and site title in the header, add a legal footer"
```

---

## Task 10: Rotte e view pubbliche `/terms` e `/privacy`

**Files:**
- Create: `frontend/src/views/LegalPageView.vue`
- Modify: `frontend/src/router/index.ts`

**Interfaces:**
- Consumes: `GET /api/legal/terms`, `GET /api/legal/privacy` da Task 5; `MarkdownText.vue` (prop `text: string`).
- Un solo componente per le due pagine, guidato da props — stesso schema di `ManageBookingView`, che il router già usa per più rotte (`/manage-booking`, `/prenotazione/:code`, `/prenotazione/:code/punteggio`) passando `mode` come prop invece di duplicare il componente.

- [ ] **Step 1: Crea la view**

Crea `frontend/src/views/LegalPageView.vue`:

```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'
import MarkdownText from '../components/MarkdownText.vue'

const props = defineProps<{
  title: string
  apiPath: '/legal/terms' | '/legal/privacy'
}>()

const markdown = ref('')
const loaded = ref(false)

onMounted(async () => {
  try {
    const res = await api.get<{ markdown: string }>(props.apiPath, { skipAuthRedirect: true })
    markdown.value = res.markdown
  } finally {
    loaded.value = true
  }
})
</script>

<template>
  <div>
    <h1>{{ title }}</h1>
    <MarkdownText v-if="markdown" :text="markdown" />
    <p v-else-if="loaded" class="field-hint">Contenuto non ancora disponibile.</p>
  </div>
</template>
```

- [ ] **Step 2: Aggiungi le rotte**

In `frontend/src/router/index.ts`, aggiungi l'import in cima:

```typescript
import LegalPageView from '../views/LegalPageView.vue'
```

E le due rotte, subito dopo quella di `/manage-booking`, entrambe sullo stesso componente con props diverse (come già fanno le rotte di `ManageBookingView` più sotto):

```typescript
    { path: '/manage-booking', name: 'manage-booking', component: ManageBookingView, meta: { public: true } },
    {
      path: '/terms',
      name: 'terms',
      component: LegalPageView,
      props: { title: 'Termini e condizioni', apiPath: '/legal/terms' },
      meta: { public: true },
    },
    {
      path: '/privacy',
      name: 'privacy',
      component: LegalPageView,
      props: { title: 'Privacy', apiPath: '/legal/privacy' },
      meta: { public: true },
    },
```

- [ ] **Step 3: Build e verifica nel browser**

```bash
cd frontend && npm run build
```

Con l'app in esecuzione, apri `/terms` e `/privacy` in Claude in Chrome: senza contenuto configurato mostrano "Contenuto non ancora disponibile.", i link del footer (Task 9) ci arrivano, e la console non ha errori. Verifica anche a 390px.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/LegalPageView.vue frontend/src/router/index.ts
git commit -m "feat: publish /terms and /privacy pages"
```

---

## Task 11: Sezione "Sito" in `SettingsView`

**Files:**
- Modify: `frontend/src/views/SettingsView.vue`

**Interfaces:**
- Consumes: `GET/PUT /api/settings` esteso (Task 3), `POST /api/settings/logo`, `POST /api/settings/favicon` (Task 4), `MarkdownEditor.vue` (props `modelValue`, `placeholder?`, `ariaLabel?`, evento `update:modelValue`).

- [ ] **Step 1: Estendi lo script**

In `frontend/src/views/SettingsView.vue`, aggiungi l'import di `MarkdownEditor`:

```typescript
import MarkdownEditor from '../components/MarkdownEditor.vue'
```

Estendi l'interfaccia `SettingsResponse`:

```typescript
interface SettingsResponse {
  defaultLanguage: string
  publicBaseUrl: string
  siteTitle: string
  logoFilename: string
  faviconFilename: string
  termsMarkdown: string
  privacyMarkdown: string
  bggApiTokenSet: boolean
  // ... resto dei campi esistenti invariato
```

Aggiungi lo stato, vicino a `const publicBaseUrl = ref('')`:

```typescript
const siteTitle = ref('')
const logoFilename = ref('')
const faviconFilename = ref('')
const termsMarkdown = ref('')
const privacyMarkdown = ref('')
const logoInput = ref<HTMLInputElement | null>(null)
const faviconInput = ref<HTMLInputElement | null>(null)
const logoUploading = ref(false)
const faviconUploading = ref(false)
const brandingError = ref('')
```

In `load()`, aggiungi:

```typescript
  siteTitle.value = s.siteTitle || ''
  logoFilename.value = s.logoFilename || ''
  faviconFilename.value = s.faviconFilename || ''
  termsMarkdown.value = s.termsMarkdown || ''
  privacyMarkdown.value = s.privacyMarkdown || ''
```

In `save()`, nel corpo passato a `api.put('/settings', { ... })`, aggiungi:

```typescript
      siteTitle: siteTitle.value,
      termsMarkdown: termsMarkdown.value,
      privacyMarkdown: privacyMarkdown.value,
```

Aggiungi, sotto le funzioni esistenti (`sendTestEmail`, prima di `onMounted`), i due upload — stesso schema di `onCoverFileSelected` in `GameAdminDetailView.vue`: scelto il file, parte subito, senza passare dal bottone "Salva":

```typescript
function pickLogo() {
  brandingError.value = ''
  logoInput.value?.click()
}

function pickFavicon() {
  brandingError.value = ''
  faviconInput.value?.click()
}

async function onLogoSelected(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  target.value = ''
  if (!file) return
  brandingError.value = ''
  logoUploading.value = true
  const formData = new FormData()
  formData.append('file', file)
  try {
    await api.post('/settings/logo', formData)
    await load()
  } catch (e) {
    brandingError.value = (e as Error).message
  } finally {
    logoUploading.value = false
  }
}

async function onFaviconSelected(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  target.value = ''
  if (!file) return
  brandingError.value = ''
  faviconUploading.value = true
  const formData = new FormData()
  formData.append('file', file)
  try {
    await api.post('/settings/favicon', formData)
    await load()
  } catch (e) {
    brandingError.value = (e as Error).message
  } finally {
    faviconUploading.value = false
  }
}
```

- [ ] **Step 2: Aggiungi il pannello nel template**

Subito dopo il `panel-card` "Generale" (dopo il suo `</div>` di chiusura) e prima di "Provider AI":

```html
      <div class="panel-card">
        <div class="section-head">
          <h2>Sito</h2>
        </div>
        <p class="field-hint">
          Rimarchizza l'installazione: il titolo compare nel titolo della pagina
          e nell'header quando non c'è un logo, logo e favicon sono immagini
          JPEG, PNG o WebP fino a 5MB.
        </p>

        <label>
          Titolo del sito
          <input v-model="siteTitle" placeholder="BoardGames Manager" />
        </label>

        <div class="field-row">
          <div>
            <span class="field-label">Logo</span>
            <div class="branding-upload">
              <img
                v-if="logoFilename"
                :src="`/api/uploads/${logoFilename}`"
                class="branding-preview"
                alt="Logo attuale"
              />
              <button type="button" class="btn-secondary" :disabled="logoUploading" @click="pickLogo">
                {{ logoUploading ? 'Caricamento…' : logoFilename ? 'Cambia logo' : 'Carica logo' }}
              </button>
              <input
                ref="logoInput"
                type="file"
                accept="image/png,image/jpeg,image/webp"
                class="visually-hidden"
                @change="onLogoSelected"
              />
            </div>
          </div>

          <div>
            <span class="field-label">Favicon</span>
            <div class="branding-upload">
              <img
                v-if="faviconFilename"
                :src="`/api/uploads/${faviconFilename}`"
                class="branding-preview branding-preview-small"
                alt="Favicon attuale"
              />
              <button type="button" class="btn-secondary" :disabled="faviconUploading" @click="pickFavicon">
                {{ faviconUploading ? 'Caricamento…' : faviconFilename ? 'Cambia favicon' : 'Carica favicon' }}
              </button>
              <input
                ref="faviconInput"
                type="file"
                accept="image/png,image/jpeg,image/webp"
                class="visually-hidden"
                @change="onFaviconSelected"
              />
            </div>
          </div>
        </div>
        <p v-if="brandingError" class="error">{{ brandingError }}</p>

        <label>
          Termini e condizioni
          <MarkdownEditor v-model="termsMarkdown" aria-label="Termini e condizioni" />
        </label>
        <p class="field-hint">Pubblicati alla pagina <code>/terms</code>, raggiungibile dal footer.</p>

        <label>
          Privacy
          <MarkdownEditor v-model="privacyMarkdown" aria-label="Privacy" />
        </label>
        <p class="field-hint">Pubblicata alla pagina <code>/privacy</code>, raggiungibile dal footer.</p>
      </div>
```

- [ ] **Step 3: Stili per l'anteprima**

In `frontend/src/app.css`, aggiungi vicino alle regole delle altre card di `SettingsView` (cerca `.field-row` per trovare il punto giusto):

```css
/* Etichetta di testo sopra un gruppo di controlli che non è, di per sé,
   un <label> per un singolo input (il gruppo logo/favicon ha dentro
   un'anteprima E un bottone). Stessa taglia e colore del testo che
   `label` (in cima al file) usa per le proprie etichette. */
.field-label {
  font-size: 0.82rem;
  font-weight: 500;
  color: var(--ink-muted);
}

.branding-upload {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-top: 0.35rem;
}

.branding-preview {
  height: 40px;
  width: auto;
  max-width: 120px;
  object-fit: contain;
  border-radius: var(--radius-sm);
  background: var(--card);
  border: 1px solid var(--card-line);
  padding: 0.2rem;
}

.branding-preview-small {
  height: 28px;
}
```

- [ ] **Step 4: Build e verifica nel browser**

```bash
cd frontend && npm run build
```

In Claude in Chrome, su `/admin/settings`: carica un'immagine come logo, verifica che l'header (Task 9) la mostri subito dopo il ricaricamento della pagina, poi carica una favicon e verifica l'anteprima nel pannello. Scrivi del testo nei due editor Markdown, salva, ricarica la pagina e controlla che sia rimasto. Prova anche `/terms` e `/privacy` con contenuto configurato. Desktop e 390px, console pulita.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/views/SettingsView.vue frontend/src/app.css
git commit -m "feat: manage site branding and legal pages from the settings screen"
```

---

## Task 12: Chiusura — suite completa, documentazione, `/impeccable`

**Files:**
- Modify: `README.md`
- Modify: `DESIGN.md`

- [ ] **Step 1: Suite completa**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
cd frontend && npm run build
```
Atteso: PASS e build pulita. Nessuna affermazione di "funziona" senza questo output nel report.

- [ ] **Step 2: Documenta in `DESIGN.md`**

Nella sezione dei componenti della shell, aggiungi una breve voce: il logo (quando caricato) sostituisce icona e nome nell'header, mantenendo la stessa altezza dell'icona; il footer minimale con i link a termini/privacy compare in fondo a ogni pagina della shell. Poche righe, nello stile già presente nel file.

- [ ] **Step 3: Aggiorna `README.md`**

Dove il README già descrive come personalizzare l'installazione (impostazioni, provider opzionali), aggiungi che dal pannello impostazioni si può anche impostare il titolo del sito, caricare un logo e una favicon personalizzati, e scrivere in Markdown i testi di `/terms` e `/privacy`. Poche righe, nel registro del testo che c'è.

- [ ] **Step 4: Pass `/impeccable`**

Ultimo task obbligatorio per ogni lavoro che tocca la UI:

```
/impeccable polish — l'header con logo/titolo configurabile, il footer
legale, le pagine /terms e /privacy, e la nuova sezione "Sito" in
SettingsView. Desktop e 390px.
```

Applica ciò che il pass trova.

- [ ] **Step 5: Commit**

```bash
git add README.md DESIGN.md
git commit -m "docs: describe the site branding and legal pages settings"
```
