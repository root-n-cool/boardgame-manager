# Traduzione AI delle descrizioni BGG — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Le descrizioni importate da BoardGameGeek arrivano già tradotte
nella lingua della scheda, tramite un provider AI OpenAI-compatible che
l'admin configura nelle impostazioni.

**Architecture:** Un package `internal/ai` con la stessa forma di
`internal/geocode` — interfaccia `Translator` più `HTTPClient` — parla il
formato OpenAI `chat/completions`. `httpapi` costruisce il client per
richiesta dalle impostazioni lette dal DB, così cambiare provider non
richiede un riavvio. La descrizione BGG grezza viene conservata in
`games.bgg_description` e resta la sorgente unica di ogni traduzione. Un
guasto del provider non fa mai fallire una scrittura: la descrizione
resta l'originale e l'admin ritraduce con un bottone.

**Tech Stack:** Go 1.25 (`net/http`, `encoding/json`, nessuna dipendenza
nuova), SQLite via runner di migrazioni custom, Vue 3 `<script setup>` +
TypeScript.

**Spec:** `docs/superpowers/specs/2026-09-03-traduzione-ai-design.md`

## Global Constraints

- **Comandi Go solo in Docker.** Il toolchain locale è rotto. Ogni
  esecuzione di test usa esattamente:
  ```bash
  docker run --rm -v "$(pwd)/backend:/app" \
    -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
    -w /app golang:1.25 go test ./...
  ```
  Non sostituire i volumi nominati `bgm-gomodcache` / `bgm-gocache`.
  `npm` invece gira in locale, dentro `frontend/`.
- **Zero dipendenze nuove**, Go e npm. Solo stdlib.
- **Migrazioni forward-only**: mai modificare un file `.sql` già
  esistente; il nuovo file è `0011_ai_provider.sql`.
- **UI in italiano**, stringhe dirette nei componenti, nessun i18n.
- **Messaggi di commit in inglese**, conventional commits.
- **Commit solo dove il piano lo dice** — CLAUDE.md vieta commit non
  richiesti, e questo piano è la richiesta.
- **Il provider AI è opzionale, e la mancanza non è un errore.** Senza
  provider configurato — o con un provider che fallisce — titolo e
  descrizione sono **quelli che arrivano da BGG**, esattamente come oggi:
  nessun campo vuoto, nessun messaggio d'errore, nessun comando di
  traduzione in UI. La stessa regola vale per le lingue aggiunte dopo.
- **Il nome del gioco non si traduce mai**, nemmeno con il provider
  attivo: `game_languages.name` è sempre `detail.Name` di BGG. Solo
  `description` passa dal traduttore.
- Token di configurazione: `ai_base_url` senza slash finale e senza
  `/chat/completions`; l'AI è attiva sse `base_url`, `api_key` e `model`
  sono tutti e tre non vuoti.
- **`doAuthedRequest` non esiste.** Nei test di questo piano è un nome di
  comodo per "richiesta HTTP autenticata come admin", scritto così per
  leggibilità. Prima di scrivere un test, leggi il file di test vicino e
  usa gli helper reali del package (`newTestServer`,
  `newTestServerWithDB`, e il modo in cui i test esistenti si
  autenticano), adattando le chiamate. Vale lo stesso per il setup del
  fake BGG: ricalca i test di `games_handlers_test.go`.

---

## File Structure

**Creati:**
- `backend/internal/db/migrations/0011_ai_provider.sql` — le quattro
  colonne nuove.
- `backend/internal/ai/client.go` — package `ai`: `Translator`,
  `HTTPClient`, `ErrNotConfigured`, nomi estesi delle lingue.
- `backend/internal/ai/client_test.go` — test del client contro
  `httptest.Server`.
- `backend/internal/httpapi/translate.go` — il punto di verità
  `translateDescription`, l'handler di ritraduzione e la costruzione del
  client dalle impostazioni.
- `backend/internal/httpapi/translate_test.go` — test dell'endpoint di
  ritraduzione.
- `backend/internal/httpapi/ai_fake_test.go` — `fakeTranslator`, sul
  calco di `bgg_fake_test.go`.

**Modificati:**
- `backend/internal/settings/store.go` — tre campi in `Settings`.
- `backend/internal/games/store.go` — campo `BGGDescription` in `Game`,
  scritto e letto dalle query.
- `backend/internal/httpapi/router.go` — campo `AI` nel `Server`, rotta
  nuova.
- `backend/internal/httpapi/settings_handlers.go` — i campi AI in GET e
  PUT.
- `backend/internal/httpapi/games_handlers.go` — `createGameFromBGG`
  salva il grezzo e traduce.
- `backend/internal/httpapi/game_languages_handlers.go` —
  `createLanguageHandler` traduce dal grezzo.
- `backend/internal/httpapi/games_responses.go` — `canTranslate`.
- `backend/cmd/server/main.go` — nessun wiring di client (si costruisce
  per richiesta); resta invariato salvo quanto detto nel Task 5.
- `frontend/src/utils/game.ts`, `frontend/src/views/SettingsView.vue`,
  `frontend/src/views/GameNewView.vue`,
  `frontend/src/views/GameAdminDetailView.vue`.
- `README.md` — la sezione di configurazione del provider.

---

### Task 1: Migrazione e store

Il fondamento: le colonne esistono e i due store le leggono e le
scrivono. Nessun comportamento visibile cambia.

**Files:**
- Create: `backend/internal/db/migrations/0011_ai_provider.sql`
- Modify: `backend/internal/settings/store.go`
- Modify: `backend/internal/games/store.go`
- Test: `backend/internal/settings/store_test.go`,
  `backend/internal/games/store_test.go`

**Interfaces:**
- Consumes: niente.
- Produces:
  - `settings.Settings` guadagna `AIBaseURL string`, `AIAPIKey string`,
    `AIModel string`.
  - `games.Game` guadagna `BGGDescription *string`.

- [ ] **Step 1: Scrivi i test che falliscono**

In `backend/internal/settings/store_test.go`, aggiungi in fondo:

```go
func TestGet_AIProviderEmptyAfterMigration(t *testing.T) {
	store := newTestStore(t)
	cfg, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cfg.AIBaseURL != "" || cfg.AIAPIKey != "" || cfg.AIModel != "" {
		t.Fatalf("expected an unconfigured AI provider, got %+v", cfg)
	}
}

func TestUpdate_PersistsAIProvider(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.Update(ctx, settings.Settings{
		DefaultLanguage: "it",
		AIBaseURL:       "https://api.example.org/v1",
		AIAPIKey:        "sk-test",
		AIModel:         "gemini-flash-lite-latest",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	cfg, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cfg.AIBaseURL != "https://api.example.org/v1" || cfg.AIAPIKey != "sk-test" ||
		cfg.AIModel != "gemini-flash-lite-latest" {
		t.Fatalf("unexpected AI settings after update: %+v", cfg)
	}
}
```

In `backend/internal/games/store_test.go`, aggiungi in fondo (il file usa
già un helper per aprire lo store: riusa quello che trovi in cima al
file, verificando il nome effettivo prima di scrivere il test):

```go
func TestCreateGame_PersistsBGGDescription(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	raw := "A worker placement game about birds."
	created, err := store.CreateGame(ctx, games.Game{Name: "Wingspan", BGGDescription: &raw})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.BGGDescription == nil || *created.BGGDescription != raw {
		t.Fatalf("expected the raw BGG description to survive the round trip, got %v", created.BGGDescription)
	}

	fetched, err := store.GetGame(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if fetched.BGGDescription == nil || *fetched.BGGDescription != raw {
		t.Fatalf("expected GetGame to return the raw description, got %v", fetched.BGGDescription)
	}
}

func TestCreateGame_WithoutBGGDescription(t *testing.T) {
	store := newTestStore(t)
	created, err := store.CreateGame(context.Background(), games.Game{Name: "Gioco a mano"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.BGGDescription != nil {
		t.Fatalf("expected no raw description for a manual game, got %q", *created.BGGDescription)
	}
}
```

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/settings/... ./internal/games/...
```

Atteso: FAIL in compilazione — `cfg.AIBaseURL undefined`,
`unknown field BGGDescription`.

- [ ] **Step 3: Scrivi la migrazione**

`backend/internal/db/migrations/0011_ai_provider.sql`:

```sql
-- Provider AI OpenAI-compatible, opzionale: con questi tre valori
-- valorizzati le descrizioni BGG arrivano tradotte, senza l'app si
-- comporta come prima.
ALTER TABLE app_settings ADD COLUMN ai_base_url TEXT;
ALTER TABLE app_settings ADD COLUMN ai_api_key TEXT;
ALTER TABLE app_settings ADD COLUMN ai_model TEXT;

-- La descrizione BGG grezza, in inglese: la sorgente da cui traduce
-- ogni lingua della scheda. Senza, tradurre in italiano cancellerebbe
-- l'originale e ogni lingua aggiunta dopo sarebbe la traduzione di una
-- traduzione.
ALTER TABLE games ADD COLUMN bgg_description TEXT;
```

- [ ] **Step 4: Estendi `settings.Store`**

In `backend/internal/settings/store.go`, aggiungi i tre campi alla
struct `Settings`, sotto `BGGAPIToken`:

```go
	// AIBaseURL, AIAPIKey e AIModel descrivono un provider
	// OpenAI-compatible. Valgono solo tutti e tre insieme: con uno vuoto
	// l'app resta senza AI, e non è un errore.
	AIBaseURL string
	AIAPIKey  string
	AIModel   string
```

Poi allarga `Get`:

```go
func (s *Store) Get(ctx context.Context) (Settings, error) {
	var out Settings
	var baseURL, bggToken, aiBaseURL, aiAPIKey, aiModel sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT default_language, public_base_url, bgg_api_token, ai_base_url, ai_api_key, ai_model
		 FROM app_settings WHERE id = 1`,
	).Scan(&out.DefaultLanguage, &baseURL, &bggToken, &aiBaseURL, &aiAPIKey, &aiModel)
	if err != nil {
		return Settings{}, err
	}
	out.PublicBaseURL = baseURL.String
	out.BGGAPIToken = bggToken.String
	out.AIBaseURL = aiBaseURL.String
	out.AIAPIKey = aiAPIKey.String
	out.AIModel = aiModel.String
	return out, nil
}
```

e `Update`:

```go
func (s *Store) Update(ctx context.Context, in Settings) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE app_settings SET default_language = ?, public_base_url = ?, bgg_api_token = ?,
		 ai_base_url = ?, ai_api_key = ?, ai_model = ? WHERE id = 1`,
		in.DefaultLanguage, nullIfEmpty(in.PublicBaseURL), nullIfEmpty(in.BGGAPIToken),
		nullIfEmpty(in.AIBaseURL), nullIfEmpty(in.AIAPIKey), nullIfEmpty(in.AIModel),
	)
	return err
}
```

- [ ] **Step 5: Estendi `games.Store`**

In `backend/internal/games/store.go`, nella struct `Game`, sotto
`CoverPath`:

```go
	// BGGDescription è la descrizione originale scaricata da BGG, in
	// inglese. Non esce mai in una risposta API: è la sorgente da cui
	// traducono tutte le lingue della scheda, così aggiungerne una non
	// produce la traduzione di una traduzione.
	BGGDescription *string
```

In `CreateGame`, aggiungi la colonna alla `INSERT`:

```go
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO games (bgg_id, name, year, min_players, max_players, playtime_minutes, owner, cover_path, seats, weight, bgg_description)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		g.BGGID, g.Name, g.Year, g.MinPlayers, g.MaxPlayers, g.PlaytimeMinutes, g.Owner, g.CoverPath, g.Seats, g.Weight, g.BGGDescription,
	)
```

In `GetGame` e in `ListGames`, aggiungi `bgg_description` in coda alla
`SELECT` (dopo `weight`, prima di `created_at`) e `&g.BGGDescription`
nella posizione corrispondente della `Scan`. Le due query diventano:

```go
`SELECT id, bgg_id, name, year, min_players, max_players, playtime_minutes, owner, cover_path, seats, weight, bgg_description, created_at
 FROM games WHERE id = ?`
```

```go
`SELECT id, bgg_id, name, year, min_players, max_players, playtime_minutes, owner, cover_path, seats, weight, bgg_description, created_at
 FROM games ORDER BY id`
```

con `Scan(..., &g.Weight, &g.BGGDescription, &createdAt)`.

Controlla `UpdateGame`: legge il gioco corrente con `GetGame` e riscrive
i campi. Se la sua `UPDATE` elenca le colonne una per una **non
aggiungere** `bgg_description` — non è modificabile dall'admin e riscriverla
sarebbe solo un modo di perderla per sbaglio.

- [ ] **Step 6: Lancia i test e verifica che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```

Atteso: PASS su tutti i package. Se `httpapi` fallisce a compilare,
qualche test costruisce `games.Game` per campi posizionali invece che per
nome: correggilo usando i nomi dei campi.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/db/migrations/0011_ai_provider.sql \
  backend/internal/settings/store.go backend/internal/settings/store_test.go \
  backend/internal/games/store.go backend/internal/games/store_test.go
git commit -m "feat: store the AI provider settings and the raw BGG description"
```

---

### Task 2: Il package `ai`

Il client verso il provider, isolato e testabile senza rete.

**Files:**
- Create: `backend/internal/ai/client.go`
- Test: `backend/internal/ai/client_test.go`

**Interfaces:**
- Consumes: niente (package foglia).
- Produces:
  - `ai.Translator` — `Translate(ctx context.Context, text, targetLang string) (string, error)`
  - `ai.HTTPClient` — campi `BaseURL`, `APIKey`, `Model` (string) e
    `HTTPClient *http.Client`; costruttore
    `ai.NewHTTPClient(baseURL, apiKey, model string) *HTTPClient`
  - `ai.ErrNotConfigured error`

- [ ] **Step 1: Scrivi i test che falliscono**

`backend/internal/ai/client_test.go`:

```go
package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boardgames-manager/internal/ai"
)

func TestTranslate_SendsAWellFormedRequest(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"Un gioco sugli uccelli."}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClient(srv.URL, "sk-test", "gemini-flash-lite-latest")
	out, err := client.Translate(context.Background(), "A game about birds.", "it")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if out != "Un gioco sugli uccelli." {
		t.Fatalf("unexpected translation: %q", out)
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("expected the OpenAI-compatible path, got %q", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("expected a bearer token, got %q", gotAuth)
	}

	var sent struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(gotBody), &sent); err != nil {
		t.Fatalf("the request body is not valid JSON: %v", err)
	}
	if sent.Model != "gemini-flash-lite-latest" {
		t.Fatalf("expected the configured model, got %q", sent.Model)
	}
	if len(sent.Messages) != 2 || sent.Messages[0].Role != "system" || sent.Messages[1].Role != "user" {
		t.Fatalf("expected a system prompt plus the text as the user message, got %+v", sent.Messages)
	}
	// Il codice ISO da solo confonde i modelli piccoli: nel prompt deve
	// finire il nome esteso della lingua.
	if !strings.Contains(strings.ToLower(sent.Messages[0].Content), "italiano") {
		t.Fatalf("expected the language spelled out in the system prompt, got %q", sent.Messages[0].Content)
	}
	if sent.Messages[1].Content != "A game about birds." {
		t.Fatalf("expected the source text as the user message, got %q", sent.Messages[1].Content)
	}
}

func TestTranslate_NotConfigured(t *testing.T) {
	client := ai.NewHTTPClient("", "", "")
	if _, err := client.Translate(context.Background(), "text", "it"); !errors.Is(err, ai.ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}

	partial := ai.NewHTTPClient("https://api.example.org/v1", "sk-test", "")
	if _, err := partial.Translate(context.Background(), "text", "it"); !errors.Is(err, ai.ErrNotConfigured) {
		t.Fatalf("expected a missing model to count as not configured, got %v", err)
	}
}

func TestTranslate_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"error":{"message":"quota exceeded"}}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClient(srv.URL, "sk-test", "m")
	_, err := client.Translate(context.Background(), "text", "it")
	if err == nil {
		t.Fatal("expected an error on a non-2xx response")
	}
	if errors.Is(err, ai.ErrNotConfigured) {
		t.Fatalf("a provider failure is not a missing configuration: %v", err)
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("expected the status code in the error, got %v", err)
	}
}

func TestTranslate_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClient(srv.URL, "sk-test", "m")
	if _, err := client.Translate(context.Background(), "text", "it"); err == nil {
		t.Fatal("expected an error when the provider returns no choices")
	}
}

func TestTranslate_TrimsTrailingSlashInBaseURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClient(srv.URL+"/", "sk-test", "m")
	if _, err := client.Translate(context.Background(), "text", "it"); err != nil {
		t.Fatalf("translate: %v", err)
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("expected a single slash before the path, got %q", gotPath)
	}
}

func TestTranslate_UnknownLanguageCodeUsesTheCodeItself(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	client := ai.NewHTTPClient(srv.URL, "sk-test", "m")
	if _, err := client.Translate(context.Background(), "text", "sv"); err != nil {
		t.Fatalf("translate: %v", err)
	}
	// Una lingua fuori dalla tabella non deve bloccare la traduzione: il
	// codice finisce nel prompt così com'è.
	if !strings.Contains(gotBody, "sv") {
		t.Fatalf("expected the raw code in the prompt, got %q", gotBody)
	}
}
```

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/ai/...
```

Atteso: FAIL — `no required module provides package boardgames-manager/internal/ai`.

- [ ] **Step 3: Scrivi il client**

`backend/internal/ai/client.go`:

```go
// Package ai parla con un provider di modelli linguistici che espone il
// formato OpenAI (chat/completions). Nel progetto serve a tradurre le
// descrizioni scaricate da BoardGameGeek, che arrivano solo in inglese.
//
// Il formato OpenAI è qui l'astrazione sul provider: con base URL,
// chiave e modello configurabili la stessa implementazione parla con
// Google Gemini, OpenAI, OpenRouter, Groq o un Ollama in locale.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrNotConfigured dice che l'admin non ha (ancora) messo un provider.
// Non è un guasto: è l'app senza AI, e chi la usa così non deve vedere
// errori da nessuna parte.
var ErrNotConfigured = errors.New("ai provider not configured")

// requestTimeout è generoso di proposito: una descrizione BGG sono
// qualche migliaio di caratteri e i modelli economici non sono veloci.
const requestTimeout = 60 * time.Second

type Translator interface {
	Translate(ctx context.Context, text, targetLang string) (string, error)
}

type HTTPClient struct {
	// BaseURL è la radice OpenAI-compatible, senza /chat/completions.
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

func NewHTTPClient(baseURL, apiKey, model string) *HTTPClient {
	return &HTTPClient{
		BaseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:     strings.TrimSpace(apiKey),
		Model:      strings.TrimSpace(model),
		HTTPClient: &http.Client{Timeout: requestTimeout},
	}
}

// languageNames rende leggibile il codice ISO: "traduci in italiano"
// funziona su qualunque modello, "traduci in it" no. Una lingua fuori da
// questa tabella passa col suo codice invece di bloccare la traduzione.
var languageNames = map[string]string{
	"it": "italiano",
	"en": "inglese",
	"fr": "francese",
	"de": "tedesco",
	"es": "spagnolo",
}

func languageName(code string) string {
	if name, ok := languageNames[strings.ToLower(strings.TrimSpace(code))]; ok {
		return name
	}
	return code
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func (c *HTTPClient) configured() bool {
	return c.BaseURL != "" && c.APIKey != "" && c.Model != ""
}

func (c *HTTPClient) Translate(ctx context.Context, text, targetLang string) (string, error) {
	if !c.configured() {
		return "", ErrNotConfigured
	}

	system := fmt.Sprintf(
		"Traduci in %s il testo che ricevi. È la descrizione di un gioco da tavolo presa da BoardGameGeek. "+
			"Rispondi con il solo testo tradotto: nessun commento, nessun preambolo, nessuna virgoletta intorno. "+
			"Mantieni gli a capo e i paragrafi dell'originale. Lascia invariati i nomi propri, i titoli dei giochi e delle espansioni.",
		languageName(targetLang),
	)

	// temperature 0: una traduzione non deve cambiare a ogni tentativo.
	payload, err := json.Marshal(chatRequest{
		Model:       c.Model,
		Temperature: 0,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: text},
		},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: requestTimeout}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read ai response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("ai provider returned status %d: %s", resp.StatusCode, string(body))
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parse ai response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("ai provider returned no choices")
	}
	out := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if out == "" {
		return "", errors.New("ai provider returned an empty translation")
	}
	return out, nil
}
```

- [ ] **Step 4: Lancia i test e verifica che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/ai/... -v
```

Atteso: PASS, sei test.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/ai
git commit -m "feat: add an OpenAI-compatible translation client"
```

---

### Task 3: Impostazioni via API

L'admin può configurare il provider. La chiave non esce mai in chiaro.

**Files:**
- Modify: `backend/internal/httpapi/settings_handlers.go`
- Test: `backend/internal/httpapi/settings_handlers_test.go`

**Interfaces:**
- Consumes: `settings.Settings.AIBaseURL/AIAPIKey/AIModel` (Task 1).
- Produces: `GET /api/settings` restituisce `aiBaseUrl`, `aiModel`,
  `aiApiKeySet`, `aiApiKeyMasked`, `aiConfigured`; `PUT /api/settings`
  accetta `aiBaseUrl`, `aiApiKey`, `aiModel`.

- [ ] **Step 1: Scrivi i test che falliscono**

In `backend/internal/httpapi/settings_handlers_test.go`, in fondo (usa gli
helper di richiesta autenticata già presenti nel file — leggi come sono
scritti i test esistenti e ricalcane la forma):

```go
func TestPutSettings_SavesAIProviderAndMasksTheKey(t *testing.T) {
	server := newTestServer(t)
	// Autentica come fanno gli altri test di questo file.

	body := `{"defaultLanguage":"it","aiBaseUrl":"https://api.example.org/v1/","aiApiKey":"sk-secret-1234","aiModel":"gemini-flash-lite-latest"}`
	rec := doAuthedRequest(t, server, http.MethodPut, "/api/settings", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = doAuthedRequest(t, server, http.MethodGet, "/api/settings", "")
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Lo slash finale viene tolto: il client ci appende /chat/completions.
	if got["aiBaseUrl"] != "https://api.example.org/v1" {
		t.Fatalf("expected the base URL without a trailing slash, got %v", got["aiBaseUrl"])
	}
	if got["aiModel"] != "gemini-flash-lite-latest" {
		t.Fatalf("unexpected model: %v", got["aiModel"])
	}
	if got["aiApiKeySet"] != true || got["aiConfigured"] != true {
		t.Fatalf("expected the provider to read as configured, got %v", got)
	}
	if got["aiApiKeyMasked"] != "****1234" {
		t.Fatalf("expected a masked key, got %v", got["aiApiKeyMasked"])
	}
	if _, leaked := got["aiApiKey"]; leaked {
		t.Fatal("the API key must never be served in clear")
	}
}

func TestPutSettings_EmptyAIKeyKeepsTheStoredOne(t *testing.T) {
	server := newTestServer(t)

	first := `{"defaultLanguage":"it","aiBaseUrl":"https://api.example.org/v1","aiApiKey":"sk-secret-1234","aiModel":"m"}`
	doAuthedRequest(t, server, http.MethodPut, "/api/settings", first)

	// Salvare di nuovo senza riscrivere la chiave non deve cancellarla:
	// il campo del form torna vuoto dopo ogni salvataggio.
	second := `{"defaultLanguage":"it","aiBaseUrl":"https://api.example.org/v1","aiApiKey":"","aiModel":"m2"}`
	rec := doAuthedRequest(t, server, http.MethodPut, "/api/settings", second)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = doAuthedRequest(t, server, http.MethodGet, "/api/settings", "")
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got["aiApiKeySet"] != true || got["aiModel"] != "m2" {
		t.Fatalf("expected the key kept and the model updated, got %v", got)
	}
}

func TestPutSettings_RejectsARelativeAIBaseURL(t *testing.T) {
	server := newTestServer(t)
	body := `{"defaultLanguage":"it","aiBaseUrl":"api.example.org","aiApiKey":"k","aiModel":"m"}`
	rec := doAuthedRequest(t, server, http.MethodPut, "/api/settings", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on a relative base URL, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetSettings_NotConfiguredByDefault(t *testing.T) {
	server := newTestServer(t)
	rec := doAuthedRequest(t, server, http.MethodGet, "/api/settings", "")
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got["aiConfigured"] != false || got["aiApiKeySet"] != false {
		t.Fatalf("expected no AI provider out of the box, got %v", got)
	}
}
```

Ricorda il vincolo globale su `doAuthedRequest`: è un nome di comodo, non
un helper esistente. Leggi `settings_handlers_test.go` e usa quelli veri.

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run Settings
```

Atteso: FAIL — `aiBaseUrl` assente dalla risposta, 200 invece di 400.

- [ ] **Step 3: Estendi gli handler**

In `settings_handlers.go`, allarga `settingsResponse`:

```go
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
```

In `getSettingsHandler`, dopo il blocco del token BGG:

```go
	resp.AIBaseURL = cfg.AIBaseURL
	resp.AIModel = cfg.AIModel
	resp.AIAPIKeySet = cfg.AIAPIKey != ""
	if resp.AIAPIKeySet {
		resp.AIAPIKeyMasked = maskKey(cfg.AIAPIKey)
	}
	resp.AIConfigured = cfg.AIBaseURL != "" && cfg.AIAPIKey != "" && cfg.AIModel != ""
```

In `updateSettingsRequest`:

```go
	AIBaseURL string `json:"aiBaseUrl"`
	AIAPIKey  string `json:"aiApiKey"`
	AIModel   string `json:"aiModel"`
```

In `putSettingsHandler`, dopo la validazione di `publicBaseUrl`, riusa la
stessa normalizzazione — le due regole sono identiche, un URL http(s)
assoluto o vuoto, senza slash finale:

```go
	aiBaseURL, ok := normalizePublicBaseURL(req.AIBaseURL)
	if !ok {
		writeError(w, http.StatusBadRequest, "l'indirizzo del provider AI deve essere assoluto, per esempio https://api.openai.com/v1")
		return
	}
```

e nel `next`:

```go
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
```

- [ ] **Step 4: Lancia i test e verifica che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/
```

Atteso: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/httpapi/settings_handlers.go backend/internal/httpapi/settings_handlers_test.go
git commit -m "feat: configure an AI provider from the settings API"
```

---

### Task 4: Il punto di verità della traduzione

Una funzione sola decide quando si traduce e quando no, e non risale mai
un errore al chiamante. Il fake per i test arriva qui.

**Files:**
- Create: `backend/internal/httpapi/translate.go`
- Create: `backend/internal/httpapi/ai_fake_test.go`
- Modify: `backend/internal/httpapi/router.go` (solo il campo `AI` nel
  `Server`)
- Modify: `backend/internal/httpapi/testhelpers_test.go`

**Interfaces:**
- Consumes: `ai.Translator`, `ai.NewHTTPClient`, `ai.ErrNotConfigured`
  (Task 2); `settings.Settings` (Task 1).
- Produces:
  - campo `AI ai.Translator` nella struct `httpapi.Server` — quando è
    `nil` il client si costruisce dalle impostazioni, quando è valorizzato
    si usa quello (è così che i test iniettano il fake)
  - `func (s *Server) translator(ctx context.Context) ai.Translator`
  - `func (s *Server) translateDescription(ctx context.Context, source, targetLang string) string`
  - `fakeTranslator` nei test, con campi `out string`, `err error`,
    `calls int`, `lastText string`, `lastLang string`

- [ ] **Step 1: Scrivi il fake e il test che fallisce**

`backend/internal/httpapi/ai_fake_test.go`:

```go
package httpapi_test

import "context"

// fakeTranslator sta al posto del provider AI nei test, come
// fakeBGGClient sta al posto di BoardGameGeek.
type fakeTranslator struct {
	out      string
	err      error
	calls    int
	lastText string
	lastLang string
}

func (f *fakeTranslator) Translate(ctx context.Context, text, targetLang string) (string, error) {
	f.calls++
	f.lastText = text
	f.lastLang = targetLang
	return f.out, f.err
}
```

In `backend/internal/httpapi/testhelpers_test.go`, aggiungi un helper che
monta un server con un traduttore finto, accanto a quelli esistenti:

```go
// newTestServerWithTranslator monta il server con un provider AI finto già
// configurato: i test che vogliono l'app senza AI usano gli altri helper e
// lasciano il campo a nil.
func newTestServerWithTranslator(t *testing.T, tr *fakeTranslator) (*httpapi.Server, *sql.DB) {
	t.Helper()
	server, conn := newTestServerWithDB(t)
	server.AI = tr
	return server, conn
}
```

`translateDescription` non è esportata e i test del package sono
`httpapi_test`: si prova **dall'esterno**, attraverso gli handler, cosa
che avviene nei Task 5 e 6. Questo task non aggiunge quindi test di
comportamento — il suo criterio di accettazione è che il fake e il campo
`AI` esistano, il progetto compili e la suite resti verde.

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/
```

Atteso: FAIL in compilazione — `server.AI undefined`.

- [ ] **Step 3: Aggiungi il campo al Server**

In `backend/internal/httpapi/router.go`, importa `"boardgames-manager/internal/ai"`
e aggiungi il campo in fondo alla struct:

```go
	// AI, quando è valorizzato, è il traduttore da usare. Lasciato a nil il
	// server ne costruisce uno per richiesta dalle impostazioni: così
	// cambiare provider non richiede un riavvio, e i test possono iniettare
	// un finto.
	AI ai.Translator
```

`backend/cmd/server/main.go` non va toccato: lasciare `AI` a nil è il
comportamento di produzione.

- [ ] **Step 4: Scrivi il punto di verità**

`backend/internal/httpapi/translate.go`:

```go
package httpapi

import (
	"context"
	"errors"
	"log"
	"strings"

	"boardgames-manager/internal/ai"
)

// translator restituisce il traduttore da usare per questa richiesta:
// quello iniettato se c'è (i test), altrimenti uno costruito al volo dalle
// impostazioni salvate. Costruirlo per richiesta è ciò che permette
// all'admin di cambiare provider senza riavviare il container.
func (s *Server) translator(ctx context.Context) ai.Translator {
	if s.AI != nil {
		return s.AI
	}
	cfg, err := s.Settings.Get(ctx)
	if err != nil {
		// Senza impostazioni non si traduce; il chiamante tratta la cosa
		// come "AI non configurata", che è l'esito giusto.
		log.Printf("translate: could not load settings: %v", err)
		return ai.NewHTTPClient("", "", "")
	}
	return ai.NewHTTPClient(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel)
}

// translateDescription è l'unico posto che decide se una descrizione va
// tradotta. Non restituisce mai un errore: se non si può tradurre, il
// testo originale è il risultato giusto — un gioco senza descrizione
// tradotta è comunque un gioco in catalogo, e la scheda ha il bottone per
// riprovare.
func (s *Server) translateDescription(ctx context.Context, source, targetLang string) string {
	if strings.TrimSpace(source) == "" {
		return source
	}
	// L'originale BGG è già in inglese: chiedere di tradurlo in inglese
	// costerebbe una chiamata per riavere lo stesso testo, peggiorato.
	if strings.EqualFold(strings.TrimSpace(targetLang), "en") {
		return source
	}

	out, err := s.translator(ctx).Translate(ctx, source, targetLang)
	if errors.Is(err, ai.ErrNotConfigured) {
		return source
	}
	if err != nil {
		log.Printf("translate into %q: %v", targetLang, err)
		return source
	}
	return out
}
```

- [ ] **Step 5: Lancia i test e verifica che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```

Atteso: PASS su tutti i package, con la suite invariata: questo task non
cambia ancora nessun comportamento.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi/translate.go backend/internal/httpapi/ai_fake_test.go \
  backend/internal/httpapi/router.go backend/internal/httpapi/testhelpers_test.go
git commit -m "feat: add the translation decision point behind an injectable translator"
```

---

### Task 5: Traduzione alla creazione del gioco

Il primo comportamento visibile: un gioco importato da BGG arriva con la
descrizione nella lingua scelta, e il grezzo resta in `bgg_description`.

**Files:**
- Modify: `backend/internal/httpapi/games_handlers.go:58-120`
  (`createGameFromBGG`)
- Modify: `backend/internal/httpapi/games_responses.go`
- Test: `backend/internal/httpapi/games_handlers_test.go`

**Interfaces:**
- Consumes: `s.translateDescription` (Task 4), `games.Game.BGGDescription`
  (Task 1), `fakeTranslator` (Task 4).
- Produces: `toGameSummary` include `canTranslate` (bool).

- [ ] **Step 1: Scrivi i test che falliscono**

In `backend/internal/httpapi/games_handlers_test.go` — leggi prima i test
di creazione da BGG già presenti e ricalcane la forma per l'autenticazione
e per il token BGG in impostazioni:

```go
func TestCreateGameFromBGG_TranslatesTheDescription(t *testing.T) {
	tr := &fakeTranslator{out: "Un gioco di piazzamento lavoratori sugli uccelli."}
	server, _ := newTestServerWithTranslator(t, tr)
	// Configura il token BGG e il fake BGG come fanno i test vicini,
	// con detail.Description = "A worker placement game about birds."

	rec := doAuthedRequest(t, server, http.MethodPost, "/api/games",
		`{"bggId":"266192","languageCode":"it"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	langs := got["languages"].([]any)
	lang := langs[0].(map[string]any)
	if lang["description"] != "Un gioco di piazzamento lavoratori sugli uccelli." {
		t.Fatalf("expected the translated description, got %v", lang["description"])
	}
	if tr.calls != 1 {
		t.Fatalf("expected exactly one translation call, got %d", tr.calls)
	}
	if tr.lastLang != "it" || tr.lastText != "A worker placement game about birds." {
		t.Fatalf("expected the raw BGG text translated into it, got %q / %q", tr.lastText, tr.lastLang)
	}
	// Il titolo non passa mai dal traduttore: "Wingspan" in Italia si
	// chiama Wingspan, e un modello lo renderebbe "Apertura alare".
	if lang["name"] != "Wingspan" {
		t.Fatalf("the title must stay the BGG one, got %v", lang["name"])
	}
	// Il grezzo resta la sorgente di ogni traduzione futura, e il suo
	// esserci si legge da fuori come canTranslate.
	if got["canTranslate"] != true {
		t.Fatalf("expected canTranslate true after a BGG import, got %v", got["canTranslate"])
	}
}

func TestCreateGameFromBGG_EnglishSkipsTheTranslator(t *testing.T) {
	tr := &fakeTranslator{out: "NON DEVE COMPARIRE"}
	server, _ := newTestServerWithTranslator(t, tr)
	// stesso setup, con detail.Description = "A worker placement game about birds."

	rec := doAuthedRequest(t, server, http.MethodPost, "/api/games",
		`{"bggId":"266192","languageCode":"en"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if tr.calls != 0 {
		t.Fatalf("the BGG original is already English: expected no call, got %d", tr.calls)
	}
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	lang := got["languages"].([]any)[0].(map[string]any)
	if lang["description"] != "A worker placement game about birds." {
		t.Fatalf("expected the original description, got %v", lang["description"])
	}
}

func TestCreateGameFromBGG_WithoutAIKeepsTheOriginal(t *testing.T) {
	server := newTestServer(t) // niente traduttore iniettato, niente provider in impostazioni
	// stesso setup BGG

	rec := doAuthedRequest(t, server, http.MethodPost, "/api/games",
		`{"bggId":"266192","languageCode":"it"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	lang := got["languages"].([]any)[0].(map[string]any)
	if lang["description"] != "A worker placement game about birds." {
		t.Fatalf("without an AI provider the description stays as BGG sent it, got %v", lang["description"])
	}
	// Il provider è opzionale: senza, la scheda è quella di BGG, titolo
	// compreso. Niente campi vuoti e niente errori.
	if lang["name"] != "Wingspan" || got["name"] != "Wingspan" {
		t.Fatalf("expected the BGG title, got %v / %v", got["name"], lang["name"])
	}
}

func TestCreateGameFromBGG_TranslationFailureDoesNotFailTheCreation(t *testing.T) {
	tr := &fakeTranslator{err: errors.New("provider down")}
	server, _ := newTestServerWithTranslator(t, tr)
	// stesso setup BGG

	rec := doAuthedRequest(t, server, http.MethodPost, "/api/games",
		`{"bggId":"266192","languageCode":"it"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("a broken provider must not break the import: got %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	lang := got["languages"].([]any)[0].(map[string]any)
	if lang["description"] != "A worker placement game about birds." {
		t.Fatalf("expected the original description as the fallback, got %v", lang["description"])
	}
}

func TestCreateGameManually_CannotTranslate(t *testing.T) {
	tr := &fakeTranslator{out: "NON DEVE COMPARIRE"}
	server, _ := newTestServerWithTranslator(t, tr)

	rec := doAuthedRequest(t, server, http.MethodPost, "/api/games",
		`{"name":"Gioco fatto in casa","languageCode":"it","nameTranslated":"Gioco fatto in casa"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if tr.calls != 0 {
		t.Fatalf("a manual game has no BGG original to translate, got %d calls", tr.calls)
	}
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got["canTranslate"] != false {
		t.Fatalf("expected canTranslate false for a manual game, got %v", got["canTranslate"])
	}
}
```

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run CreateGame
```

Atteso: FAIL — descrizione non tradotta, `canTranslate` assente.

- [ ] **Step 3: Innesta la traduzione in `createGameFromBGG`**

In `backend/internal/httpapi/games_handlers.go`, dentro
`createGameFromBGG`, passa il grezzo alla `CreateGame`:

```go
	// La descrizione BGG grezza resta sul gioco: è la sorgente da cui
	// traduce ogni lingua della scheda, oggi e quando se ne aggiunge una.
	rawDescription := detail.Description

	game, err := s.Games.CreateGame(r.Context(), games.Game{
		BGGID: &bggID, Name: detail.Name, Year: &year, MinPlayers: &minPlayers,
		MaxPlayers: &maxPlayers, PlaytimeMinutes: &playtime, Owner: &owner, CoverPath: coverPath,
		Weight: weight, Seats: requestedSeats(req), BGGDescription: nilIfEmptyString(rawDescription),
	})
```

e sostituisci le due righe che creano la lingua:

```go
	description := s.translateDescription(r.Context(), rawDescription, req.LanguageCode)
	lang, err := s.Games.CreateLanguage(r.Context(), games.GameLanguage{
		GameID: game.ID, LanguageCode: req.LanguageCode, IsBaseLanguage: true,
		Name: detail.Name, Description: &description,
	})
```

In fondo allo stesso file, accanto agli altri helper, aggiungi:

```go
// nilIfEmptyString distingue "nessuna descrizione BGG" da "descrizione
// vuota": la colonna resta NULL, e canTranslate legge falso.
func nilIfEmptyString(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return &v
}
```

Il file importa già `strings` (lo usa `rankSearchResults`); verificalo, e
se manca aggiungilo. Attenzione a non confonderla con la `nilIfEmpty`
già presente, che restituisce `any` per le mappe di risposta.

- [ ] **Step 4: Esponi `canTranslate`**

In `backend/internal/httpapi/games_responses.go`, in `toGameSummary`:

```go
func toGameSummary(g games.Game) map[string]any {
	return map[string]any{
		"id": g.ID, "bggId": g.BGGID, "name": g.Name, "year": g.Year,
		"minPlayers": g.MinPlayers, "maxPlayers": g.MaxPlayers,
		"playtimeMinutes": g.PlaytimeMinutes, "weight": g.Weight,
		"owner": g.Owner, "coverPath": g.CoverPath, "seats": g.Seats,
		// canTranslate dice che esiste un originale BGG da cui ritradurre.
		// Esce il booleano, non il testo: la scheda di modifica deve solo
		// sapere se il bottone ha una sorgente.
		"canTranslate": g.BGGDescription != nil && *g.BGGDescription != "",
	}
}
```

- [ ] **Step 5: Lancia i test e verifica che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```

Atteso: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi/games_handlers.go backend/internal/httpapi/games_responses.go \
  backend/internal/httpapi/games_handlers_test.go
git commit -m "feat: translate the BGG description when a game enters the catalogue"
```

---

### Task 6: Nuova lingua e ritraduzione a richiesta

**Files:**
- Modify: `backend/internal/httpapi/game_languages_handlers.go:44-60`
- Modify: `backend/internal/httpapi/translate.go` (l'handler)
- Modify: `backend/internal/httpapi/router.go` (la rotta)
- Create: `backend/internal/httpapi/translate_test.go`
- Test: `backend/internal/httpapi/game_languages_handlers_test.go`

**Interfaces:**
- Consumes: `s.translateDescription`, `s.translator` (Task 4);
  `games.Game.BGGDescription` (Task 1).
- Produces: rotta
  `POST /api/games/{id}/languages/{lang}/translate` →
  `s.translateLanguageHandler`, che restituisce 200 con la
  `GameLanguage` aggiornata nella stessa forma di
  `updateLanguageHandler`.

- [ ] **Step 1: Scrivi i test che falliscono**

`backend/internal/httpapi/translate_test.go` (adatta autenticazione e
setup agli helper reali del package):

```go
package httpapi_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestTranslateLanguage_OverwritesTheDescription(t *testing.T) {
	tr := &fakeTranslator{out: "Descrizione tradotta di nuovo."}
	server, _ := newTestServerWithTranslator(t, tr)
	// Crea un gioco da BGG con lingua base "it" (descrizione grezza
	// "A worker placement game about birds."), poi correggi a mano la
	// descrizione con una PATCH, così il test prova che il bottone
	// sovrascrive davvero.

	rec := doAuthedRequest(t, server, http.MethodPost, "/api/games/1/languages/it/translate", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got["description"] != "Descrizione tradotta di nuovo." {
		t.Fatalf("expected the retranslated text, got %v", got["description"])
	}
	// Si traduce sempre dal grezzo BGG, mai dal testo già in scheda:
	// altrimenti si ritradurrebbe una traduzione.
	if tr.lastText != "A worker placement game about birds." {
		t.Fatalf("expected the raw BGG description as the source, got %q", tr.lastText)
	}
}

func TestTranslateLanguage_WithoutAIReturns409(t *testing.T) {
	server := newTestServer(t)
	// Crea un gioco da BGG con lingua base "it".

	rec := doAuthedRequest(t, server, http.MethodPost, "/api/games/1/languages/it/translate", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 without an AI provider, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTranslateLanguage_WithoutARawDescriptionReturns409(t *testing.T) {
	tr := &fakeTranslator{out: "irrilevante"}
	server, _ := newTestServerWithTranslator(t, tr)
	// Crea un gioco a mano (nessun bgg_description) con lingua "it".

	rec := doAuthedRequest(t, server, http.MethodPost, "/api/games/1/languages/it/translate", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 with nothing to translate from, got %d: %s", rec.Code, rec.Body.String())
	}
	if tr.calls != 0 {
		t.Fatalf("expected no provider call, got %d", tr.calls)
	}
}

func TestTranslateLanguage_UnknownGameOrLanguage(t *testing.T) {
	tr := &fakeTranslator{out: "irrilevante"}
	server, _ := newTestServerWithTranslator(t, tr)

	rec := doAuthedRequest(t, server, http.MethodPost, "/api/games/999/languages/it/translate", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on an unknown game, got %d", rec.Code)
	}

	// Crea un gioco da BGG con lingua base "it", poi chiedi il francese,
	// che non esiste su quel gioco.
	rec = doAuthedRequest(t, server, http.MethodPost, "/api/games/1/languages/fr/translate", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on a language the game does not have, got %d", rec.Code)
	}
}

func TestTranslateLanguage_ProviderFailureReturns502(t *testing.T) {
	tr := &fakeTranslator{err: errors.New("provider down")}
	server, _ := newTestServerWithTranslator(t, tr)
	// Crea un gioco da BGG con lingua base "it".

	rec := doAuthedRequest(t, server, http.MethodPost, "/api/games/1/languages/it/translate", "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when the provider fails on an explicit request, got %d", rec.Code)
	}
}
```

In `backend/internal/httpapi/game_languages_handlers_test.go`:

```go
func TestCreateLanguage_TranslatesFromTheRawBGGDescription(t *testing.T) {
	tr := &fakeTranslator{out: "Ein Spiel über Vögel."}
	server, _ := newTestServerWithTranslator(t, tr)
	// Crea un gioco da BGG con lingua base "it" (il fake traduce anche
	// quella: rimetti tr.calls a 0 prima della chiamata sotto, o leggi il
	// delta).

	rec := doAuthedRequest(t, server, http.MethodPost, "/api/games/1/languages",
		`{"languageCode":"de"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if tr.lastText != "A worker placement game about birds." || tr.lastLang != "de" {
		t.Fatalf("expected the raw BGG text translated into de, got %q / %q", tr.lastText, tr.lastLang)
	}
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got["description"] != "Ein Spiel über Vögel." {
		t.Fatalf("expected the translated description, got %v", got["description"])
	}
}

func TestCreateLanguage_ManualGameFallsBackToTheBaseLanguage(t *testing.T) {
	tr := &fakeTranslator{out: "NON DEVE COMPARIRE"}
	server, _ := newTestServerWithTranslator(t, tr)
	// Crea un gioco a mano con lingua base "it" e una descrizione scritta
	// dall'admin, per esempio "Un gioco di carte fatto in casa".

	rec := doAuthedRequest(t, server, http.MethodPost, "/api/games/1/languages",
		`{"languageCode":"en"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	// Senza originale BGG resta il ripiego di sempre: si copia la lingua
	// base come punto di partenza per una traduzione a mano.
	if got["description"] != "Un gioco di carte fatto in casa" {
		t.Fatalf("expected the base language text as the fallback, got %v", got["description"])
	}
}
```

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run 'Translate|CreateLanguage'
```

Atteso: FAIL — 404 sulla rotta inesistente, descrizione non tradotta.

- [ ] **Step 3: Traduci quando si aggiunge una lingua**

In `backend/internal/httpapi/game_languages_handlers.go`, sostituisci il
blocco di precompilazione con:

```go
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
```

- [ ] **Step 4: Scrivi l'handler di ritraduzione**

In fondo a `backend/internal/httpapi/translate.go`:

```go
// translateLanguageHandler ritraduce a richiesta la descrizione di una
// lingua, sempre dall'originale BGG. Sovrascrive quel che c'è, comprese le
// correzioni a mano: è il senso del bottone, e la UI lo dice prima di
// chiamarlo.
//
// A differenza degli innesti automatici, qui un guasto si mostra: l'admin
// ha premuto un bottone e ha diritto di sapere che non ha funzionato.
func (s *Server) translateLanguageHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}
	code := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "lang")))
	if code == "" {
		writeError(w, http.StatusBadRequest, "language code is required")
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

	lang, err := s.Games.GetLanguage(r.Context(), gameID, code)
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "language not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load language")
		return
	}

	if game.BGGDescription == nil || strings.TrimSpace(*game.BGGDescription) == "" {
		writeError(w, http.StatusConflict, "questo gioco non ha una descrizione originale da BoardGameGeek da tradurre")
		return
	}

	translated, err := s.translator(r.Context()).Translate(r.Context(), *game.BGGDescription, code)
	if errors.Is(err, ai.ErrNotConfigured) {
		writeError(w, http.StatusConflict, "il provider AI non è configurato: aggiungilo nelle impostazioni")
		return
	}
	if err != nil {
		log.Printf("translate language %s of game %d: %v", code, gameID, err)
		writeError(w, http.StatusBadGateway, "la traduzione non è riuscita: riprova, o controlla il provider nelle impostazioni")
		return
	}

	updated, err := s.Games.UpdateLanguage(r.Context(), gameID, code, lang.Name, &translated)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save the translation")
		return
	}

	media, err := s.Games.ListMedia(r.Context(), updated.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load media")
		return
	}
	mediaOut := make([]map[string]any, 0, len(media))
	for _, m := range media {
		mediaOut = append(mediaOut, toMediaResponse(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": updated.LanguageCode, "isBaseLanguage": updated.IsBaseLanguage,
		"name": updated.Name, "description": updated.Description, "media": mediaOut,
	})
}
```

Aggiungi gli import mancanti a `translate.go`: `net/http`,
`github.com/go-chi/chi/v5`, `boardgames-manager/internal/games`.

Prima di scrivere la risposta, apri `updateLanguageHandler` in
`game_languages_handlers.go` e **usa la sua stessa forma di risposta**: se
lì c'è già un helper che serializza una `GameLanguage` con i suoi media,
chiamalo invece di duplicare le ultime dodici righe qui sopra.

- [ ] **Step 5: Registra la rotta**

In `backend/internal/httpapi/router.go`, dentro il gruppo `protected`,
subito dopo la riga della PATCH sulla lingua:

```go
		protected.Post("/api/games/{id}/languages/{lang}/translate", s.translateLanguageHandler)
```

- [ ] **Step 6: Lancia i test e verifica che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```

Atteso: PASS su tutti i package.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/httpapi/translate.go backend/internal/httpapi/translate_test.go \
  backend/internal/httpapi/game_languages_handlers.go \
  backend/internal/httpapi/game_languages_handlers_test.go backend/internal/httpapi/router.go
git commit -m "feat: translate a new language and retranslate on demand"
```

---

### Task 7: Impostazioni nel frontend

**Files:**
- Modify: `frontend/src/views/SettingsView.vue`

**Interfaces:**
- Consumes: `GET`/`PUT /api/settings` con i campi AI (Task 3).
- Produces: niente per gli altri task; `aiConfigured` viene riletto dal
  Task 8 con una chiamata propria.

- [ ] **Step 1: Estendi l'interfaccia e lo stato**

In `frontend/src/views/SettingsView.vue`, in `SettingsResponse`:

```ts
  aiBaseUrl: string
  aiModel: string
  aiApiKeySet: boolean
  aiApiKeyMasked?: string
  aiConfigured: boolean
```

e accanto agli altri `ref`:

```ts
const aiBaseUrl = ref('')
const aiModel = ref('')
const aiApiKey = ref('')
const aiApiKeyMasked = ref('')
```

In `load()`:

```ts
  aiBaseUrl.value = s.aiBaseUrl || ''
  aiModel.value = s.aiModel || ''
  aiApiKeyMasked.value = s.aiApiKeyMasked || ''
```

In `save()`, nel corpo della PUT:

```ts
      aiBaseUrl: aiBaseUrl.value,
      aiApiKey: aiApiKey.value,
      aiModel: aiModel.value,
```

e subito dopo `bggApiToken.value = ''`:

```ts
    aiApiKey.value = ''
```

- [ ] **Step 2: Aggiungi il blocco nel template**

Dopo il campo "Indirizzo pubblico" e il suo `field-hint`, prima del
bottone Salva:

```html
      <h2>Provider AI</h2>
      <p class="field-hint">
        Se lo configuri, le descrizioni scaricate da BoardGameGeek arrivano già
        tradotte nella lingua della scheda. Vale un qualsiasi servizio
        compatibile con le API OpenAI: Google Gemini, OpenAI, OpenRouter, o un
        Ollama in casa. Lasciandolo vuoto l'app funziona come prima, con le
        descrizioni in inglese.
      </p>

      <label>
        Indirizzo del provider
        <input
          v-model="aiBaseUrl"
          type="url"
          inputmode="url"
          placeholder="https://generativelanguage.googleapis.com/v1beta/openai"
        />
      </label>
      <p class="field-hint">
        L'indirizzo base, senza <code>/chat/completions</code> in fondo. Gemini:
        <code>https://generativelanguage.googleapis.com/v1beta/openai</code> ·
        OpenAI: <code>https://api.openai.com/v1</code> · Ollama:
        <code>http://localhost:11434/v1</code>
      </p>

      <label>
        Chiave API
        <input v-model="aiApiKey" type="password" :placeholder="aiApiKeyMasked || 'non configurata'" />
      </label>

      <label>
        Modello
        <input v-model="aiModel" placeholder="gemini-flash-lite-latest" />
      </label>
      <p class="field-hint">
        Per tradurre basta un modello economico e veloce. Esempi:
        <code>gemini-flash-lite-latest</code>, <code>gpt-4.1-mini</code>,
        <code>llama3.1</code>.
      </p>
```

- [ ] **Step 3: Traduci il messaggio di errore dell'indirizzo**

In `save()`, il `catch` traduce già l'errore di `publicBaseUrl`. Aggiungi
il caso nuovo — il backend risponde già in italiano per l'indirizzo AI,
quindi non serve mappare nulla: verifica solo che il messaggio arrivi
fino a `error.value` senza essere sovrascritto dal ramo esistente.

- [ ] **Step 4: Verifica il build**

```bash
cd frontend && npm run build
```

Atteso: build completato, nessun errore di `vue-tsc`.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/views/SettingsView.vue
git commit -m "feat: configure the AI provider from the settings page"
```

---

### Task 8: Traduzione nel frontend

**Files:**
- Modify: `frontend/src/utils/game.ts`
- Modify: `frontend/src/views/GameNewView.vue`
- Modify: `frontend/src/views/GameAdminDetailView.vue`

**Interfaces:**
- Consumes: `canTranslate` su `GameDetail` (Task 5); `aiConfigured` da
  `GET /api/settings` (Task 3); `POST /api/games/{id}/languages/{lang}/translate`
  (Task 6).
- Produces: niente.

- [ ] **Step 1: Estendi il tipo condiviso**

In `frontend/src/utils/game.ts`, in `GameDetail`, sotto `seats`:

```ts
  /** Vero quando esiste una descrizione BGG originale da cui ritradurre. */
  canTranslate: boolean
```

- [ ] **Step 2: L'attesa in creazione**

In `frontend/src/views/GameNewView.vue`, aggiungi lo stato accanto agli
altri `ref`:

```ts
// La creazione da BGG passa dal traduttore: può durare qualche secondo, e
// uno spinner muto su un salvataggio di solito istantaneo si legge come un
// blocco.
const aiConfigured = ref(false)
```

e in fondo al blocco `<script setup>`:

```ts
onMounted(async () => {
  try {
    const s = await api.get<{ aiConfigured: boolean }>('/settings')
    aiConfigured.value = s.aiConfigured
  } catch {
    // Non sapere se l'AI è attiva costa solo un'etichetta meno precisa.
  }
})
```

aggiungendo `onMounted` all'import da `vue`.

Nel template, il bottone di submit mostra l'etichetta giusta. Cerca il
bottone di salvataggio in fondo al form e rendi il suo testo:

```html
        <button type="submit" :disabled="!ready || saving">
          {{ saving && aiConfigured && !manual ? 'Traduzione in corso…' : saving ? 'Salvataggio…' : 'Aggiungi' }}
        </button>
```

Adatta la forma esatta a quella del bottone che trovi nel file — se ha
un'icona SVG, conserva l'icona e sostituisci solo il testo. Il ramo
`!manual` c'è perché l'inserimento a mano non traduce niente.

- [ ] **Step 3: Il bottone di ritraduzione**

In `frontend/src/views/GameAdminDetailView.vue`, accanto agli altri `ref`:

```ts
const aiConfigured = ref(false)
const translating = ref(false)
const translateError = ref('')

// Il nome esteso della lingua rende il bottone leggibile: "Traduci in
// italiano" invece di "Traduci in it".
const languageNames: Record<string, string> = {
  it: 'italiano',
  en: 'inglese',
  fr: 'francese',
  de: 'tedesco',
  es: 'spagnolo',
}

function languageName(code: string): string {
  return languageNames[code] || code
}

async function translateDescription() {
  if (!window.confirm(`Ritradurre la descrizione in ${languageName(activeLangCode.value)}? Il testo attuale viene sostituito.`)) {
    return
  }
  translateError.value = ''
  translating.value = true
  try {
    await api.post(`/games/${gameId}/languages/${activeLangCode.value}/translate`, {})
    await load()
    selectLanguage(activeLangCode.value)
    saveMessage.value = 'Descrizione tradotta'
  } catch (e) {
    translateError.value = (e as Error).message
  } finally {
    translating.value = false
  }
}
```

Nel `load()` esistente, aggiungi in coda la lettura delle impostazioni:

```ts
  try {
    const s = await api.get<{ aiConfigured: boolean }>('/settings')
    aiConfigured.value = s.aiConfigured
  } catch {
    aiConfigured.value = false
  }
```

Nel template, dentro il form della scheda lingua, tra la textarea della
descrizione e `saveMessage`:

```html
          <p v-if="game.canTranslate && aiConfigured" class="field-hint">
            <button
              type="button"
              class="link-button"
              :disabled="translating"
              @click="translateDescription"
            >
              {{ translating ? 'Traduzione in corso…' : `Traduci in ${languageName(activeLangCode)} da BoardGameGeek` }}
            </button>
            — sostituisce il testo qui sopra con una nuova traduzione della
            descrizione originale.
          </p>
          <p v-if="translateError" class="error">{{ translateError }}</p>
```

- [ ] **Step 4: Verifica il build**

```bash
cd frontend && npm run build
```

Atteso: build completato, nessun errore di `vue-tsc`.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/utils/game.ts frontend/src/views/GameNewView.vue \
  frontend/src/views/GameAdminDetailView.vue
git commit -m "feat: translate a game description from the admin card"
```

---

### Task 9: Documentazione, verifica finale e pass `impeccable`

**Files:**
- Modify: `README.md`
- Modify: `DESIGN.md` (solo se il Task 8 ha introdotto un pattern visivo
  nuovo)

- [ ] **Step 1: Documenta il provider nel README**

Nella sezione delle impostazioni (accanto a dove si spiega il token BGG),
aggiungi:

```markdown
### Traduzione automatica delle descrizioni (opzionale)

BoardGameGeek serve le descrizioni solo in inglese. Configurando un
provider AI in **Impostazioni → Provider AI**, ogni gioco importato entra
in catalogo con la descrizione già tradotta nella lingua della scheda, e
lo stesso vale quando si aggiunge una lingua a un gioco esistente.

Va bene qualunque servizio compatibile con le API OpenAI. Servono tre
valori:

| Campo | Esempio |
|---|---|
| Indirizzo | `https://generativelanguage.googleapis.com/v1beta/openai` |
| Chiave API | la chiave del provider |
| Modello | `gemini-flash-lite-latest` |

Per un'associazione la scelta più semplice è **Google Gemini via AI
Studio**: la chiave è gratuita, non serve una carta di credito, e il
piano gratuito basta ampiamente a tradurre un catalogo. In alternativa
funzionano OpenAI (`https://api.openai.com/v1`), OpenRouter, Groq, o un
Ollama sulla stessa macchina (`http://localhost:11434/v1`).

Senza provider configurato l'app funziona esattamente come prima: le
descrizioni restano quelle originali di BoardGameGeek, e nella scheda di
un gioco non compare nessun comando di traduzione.

La descrizione tradotta resta modificabile a mano dalla scheda del gioco.
Il pulsante *Traduci* ritraduce sempre dall'originale BoardGameGeek e
sostituisce il testo corrente.
```

- [ ] **Step 2: Suite backend completa**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```

Atteso: PASS su tutti i package. Incolla l'output nel resoconto: senza
output non si dichiara niente concluso.

- [ ] **Step 3: Build frontend**

```bash
cd frontend && npm run build
```

Atteso: build completato senza errori di tipo.

- [ ] **Step 4: Prova manuale sull'app vera**

```bash
docker compose up -d --build
```

Su http://localhost:8080: configura un provider in Impostazioni, importa
un gioco da BGG con lingua base italiana e verifica che la descrizione
arrivi in italiano; poi aggiungi la lingua inglese allo stesso gioco e
verifica che la descrizione inglese sia l'originale BGG, non una
ritraduzione. Infine cancella il campo Modello in Impostazioni — così il
provider torna incompleto — e verifica che il bottone *Traduci* sparisca
dalla scheda del gioco.

- [ ] **Step 5: Pass `impeccable`**

Come prescritto da CLAUDE.md, l'ultimo passo di ogni lavoro che tocca il
frontend. Lancia `/impeccable` in modalità `polish` sulle tre superfici
toccate: la pagina Impostazioni, il form di creazione gioco e la scheda
gioco lato admin. Applica quel che ne esce, poi rilancia
`npm run build`.

- [ ] **Step 6: Commit**

```bash
git add README.md DESIGN.md frontend
git commit -m "docs: document the optional AI translation provider"
```
