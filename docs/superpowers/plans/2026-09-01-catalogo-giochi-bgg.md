# Fase 2 — Catalogo Giochi + Integrazione BGG Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Aggiungere al BoardGames Manager il catalogo giochi: ricerca/import da BoardGameGeek o creazione manuale, gestione di una o più lingue per gioco (nome/descrizione tradotti), gestione media (manuale PDF, link, YouTube) per lingua, con lettura pubblica e scrittura riservata agli admin.

**Architecture:** Nuovi pacchetti Go `internal/bgg` (client XML API2), `internal/games` (repository Game/GameLanguage/GameMedia), `internal/storage` (file content-addressed su disco). `internal/settings` esteso con un token BGG. `internal/httpapi` guadagna handler pubblici (lettura catalogo, file) e protetti (scrittura). Frontend Vue aggiunge viste per elenco/ricerca/creazione/dettaglio gioco.

**Tech Stack:** Go (`encoding/xml` per il parsing BGG, `net/http` multipart per gli upload), Vue 3 + TypeScript (già in uso).

**Spec:** `docs/superpowers/specs/2026-09-01-catalogo-giochi-bgg-design.md`

## Global Constraints

- Nessuna ricerca automatica di manuali/tutorial in questa fase — solo import BGG (dati base) e inserimento manuale di manuale/tutorial.
- BGG XML API2 richiede ora un token di autorizzazione (`Authorization: Bearer <token>` — formato non documentato ufficialmente, best-guess isolato in un unico punto del client, da correggere con un token reale se necessario).
- Lettura del catalogo (elenco, dettaglio, file) pubblica senza login; scrittura (crea/modifica/elimina gioco, lingue, media) dietro sessione admin.
- File (copertine, manuali) salvati content-addressed (sha256) in `/data/uploads/`; eliminare una riga DB non elimina il file fisico.
- Manuale: solo `application/pdf`, max 20MB. Copertina: jpg/png/webp, max 5MB.
- Nome e descrizione vivono su `GameLanguage`, non su `Game` (che ha solo il nome ufficiale/di riferimento).
- Ogni link/router-link Vue verso una route non ancora registrata nello STESSO task deve usare una stringa di path semplice (es. `to="/games/new"`), mai un oggetto `{name: '...'}` — una `RouterLink` con nome non registrato lancia un errore che blocca il rendering dell'intera pagina (bug reale scoperto e corretto in Fase 1).
- Questo machine's local Go toolchain è rotto (binario x86_64 su host arm64): ogni comando `go` va eseguito via Docker (`golang:1.25`, stesso wrapper di Fase 1).

---

### Task 1: Impostazioni — token BGG

**Files:**
- Create: `backend/internal/db/migrations/0002_add_bgg_token.sql`
- Modify: `backend/internal/settings/store.go`
- Modify: `backend/internal/settings/store_test.go`
- Modify: `backend/internal/httpapi/settings_handlers.go`
- Modify: `backend/internal/httpapi/settings_handlers_test.go`

**Interfaces:**
- Modifica: `settings.Settings` guadagna il campo `BGGAPIToken string`.
- Modifica: la risposta JSON di `GET /api/settings` guadagna `bggApiTokenSet`/`bggApiTokenMasked`; `PUT /api/settings` accetta `bggApiToken` (stessa semantica preserve-on-empty delle altre chiavi).

- [ ] **Step 1: Scrivere la migrazione**

`backend/internal/db/migrations/0002_add_bgg_token.sql`:
```sql
ALTER TABLE app_settings ADD COLUMN bgg_api_token TEXT;
```

- [ ] **Step 2: Estendere il test del repository impostazioni**

`backend/internal/settings/store_test.go` (file completo aggiornato):
```go
package settings_test

import (
	"context"
	"testing"

	"boardgames-manager/internal/db"
	"boardgames-manager/internal/settings"
)

func newTestStore(t *testing.T) *settings.Store {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return settings.NewStore(conn)
}

func TestGet_ReturnsDefaultLanguageAfterMigration(t *testing.T) {
	store := newTestStore(t)
	cfg, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cfg.DefaultLanguage != "it" {
		t.Fatalf("expected default language 'it', got %q", cfg.DefaultLanguage)
	}
	if cfg.YouTubeAPIKey != "" || cfg.BGGAPIToken != "" {
		t.Fatalf("expected empty optional keys by default, got %+v", cfg)
	}
}

func TestUpdate_PersistsChanges(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.Update(ctx, settings.Settings{
		DefaultLanguage:   "en",
		YouTubeAPIKey:     "yt-key",
		SearchAPIKey:      "search-key",
		SearchAPIProvider: "google",
		BGGAPIToken:       "bgg-token",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	cfg, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cfg.DefaultLanguage != "en" || cfg.YouTubeAPIKey != "yt-key" || cfg.SearchAPIKey != "search-key" ||
		cfg.SearchAPIProvider != "google" || cfg.BGGAPIToken != "bgg-token" {
		t.Fatalf("unexpected settings after update: %+v", cfg)
	}
}
```

- [ ] **Step 3: Eseguire il test e verificare che fallisca**

Run: `docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/settings/... -v`
Expected: FAIL (colonna/campo `BGGAPIToken` non esiste ancora)

- [ ] **Step 4: Implementare il repository**

`backend/internal/settings/store.go` (file completo aggiornato):
```go
package settings

import (
	"context"
	"database/sql"
)

type Settings struct {
	DefaultLanguage   string
	YouTubeAPIKey     string
	SearchAPIKey      string
	SearchAPIProvider string
	BGGAPIToken       string
}

type Store struct {
	db *sql.DB
}

func NewStore(conn *sql.DB) *Store {
	return &Store{db: conn}
}

func (s *Store) Get(ctx context.Context) (Settings, error) {
	var out Settings
	var youtubeKey, searchKey, provider, bggToken sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT default_language, youtube_api_key, search_api_key, search_api_provider, bgg_api_token FROM app_settings WHERE id = 1`,
	).Scan(&out.DefaultLanguage, &youtubeKey, &searchKey, &provider, &bggToken)
	if err != nil {
		return Settings{}, err
	}
	out.YouTubeAPIKey = youtubeKey.String
	out.SearchAPIKey = searchKey.String
	out.SearchAPIProvider = provider.String
	out.BGGAPIToken = bggToken.String
	return out, nil
}

func (s *Store) Update(ctx context.Context, in Settings) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE app_settings SET default_language = ?, youtube_api_key = ?, search_api_key = ?, search_api_provider = ?, bgg_api_token = ? WHERE id = 1`,
		in.DefaultLanguage, nullIfEmpty(in.YouTubeAPIKey), nullIfEmpty(in.SearchAPIKey), nullIfEmpty(in.SearchAPIProvider), nullIfEmpty(in.BGGAPIToken),
	)
	return err
}

func nullIfEmpty(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}
```

- [ ] **Step 5: Eseguire il test e verificare che passi**

Run: (stesso comando dello Step 3)
Expected: PASS

- [ ] **Step 6: Estendere i test HTTP delle impostazioni**

Aggiungere a `backend/internal/httpapi/settings_handlers_test.go` (non sostituire il file, aggiungere questi test):
```go
func TestPutSettings_HandlesBGGToken(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{
		"defaultLanguage": "it",
		"bggApiToken":     "abcd1234efgh",
	})
	putReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(payload))
	putReq.AddCookie(cookie)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", putRec.Code, putRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	getReq.AddCookie(cookie)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	var body struct {
		BGGAPITokenSet    bool   `json:"bggApiTokenSet"`
		BGGAPITokenMasked string `json:"bggApiTokenMasked"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.BGGAPITokenSet {
		t.Fatal("expected bggApiTokenSet to be true")
	}
	if body.BGGAPITokenMasked == "abcd1234efgh" {
		t.Fatal("expected bgg token to be masked, not returned in clear")
	}
}
```

- [ ] **Step 7: Eseguire il test e verificare che fallisca**

Run: `docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/... -run TestPutSettings_HandlesBGGToken -v`
Expected: FAIL (`bggApiToken` non gestito)

- [ ] **Step 8: Implementare gli handler**

`backend/internal/httpapi/settings_handlers.go` (file completo aggiornato):
```go
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
	BGGAPITokenSet      bool   `json:"bggApiTokenSet"`
	BGGAPITokenMasked   string `json:"bggApiTokenMasked,omitempty"`
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
		BGGAPITokenSet:    cfg.BGGAPIToken != "",
	}
	if resp.YouTubeAPIKeySet {
		resp.YouTubeAPIKeyMasked = maskKey(cfg.YouTubeAPIKey)
	}
	if resp.SearchAPIKeySet {
		resp.SearchAPIKeyMasked = maskKey(cfg.SearchAPIKey)
	}
	if resp.BGGAPITokenSet {
		resp.BGGAPITokenMasked = maskKey(cfg.BGGAPIToken)
	}
	writeJSON(w, http.StatusOK, resp)
}

type updateSettingsRequest struct {
	DefaultLanguage   string `json:"defaultLanguage"`
	YouTubeAPIKey     string `json:"youtubeApiKey"`
	SearchAPIKey      string `json:"searchApiKey"`
	SearchAPIProvider string `json:"searchApiProvider"`
	BGGAPIToken       string `json:"bggApiToken"`
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
		BGGAPIToken:       current.BGGAPIToken,
	}
	if req.YouTubeAPIKey != "" {
		next.YouTubeAPIKey = req.YouTubeAPIKey
	}
	if req.SearchAPIKey != "" {
		next.SearchAPIKey = req.SearchAPIKey
	}
	if req.BGGAPIToken != "" {
		next.BGGAPIToken = req.BGGAPIToken
	}

	if err := s.Settings.Update(r.Context(), next); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save settings")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}
```

- [ ] **Step 9: Eseguire tutti i test e verificare che passino**

Run: `docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./... -v`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add backend/internal/db/migrations/0002_add_bgg_token.sql backend/internal/settings backend/internal/httpapi/settings_handlers.go backend/internal/httpapi/settings_handlers_test.go
git commit -m "feat: add BGG API token to application settings"
```

---

### Task 2: Storage — file content-addressed

**Files:**
- Create: `backend/internal/storage/store.go`
- Create: `backend/internal/storage/store_test.go`

**Interfaces:**
- Produces: `storage.Category{Name string; AllowedTypes map[string]string; MaxBytes int64}`, `storage.ManualCategory`, `storage.CoverCategory`, `storage.Store`, `storage.NewStore(baseDir string) *Store`, `(*Store).Save(category Category, r io.Reader) (filename string, err error)`, `(*Store).Open(filename string) (io.ReadCloser, error)`, `storage.ErrUnsupportedType`, `storage.ErrTooLarge`.

- [ ] **Step 1: Scrivere i test**

`backend/internal/storage/store_test.go`:
```go
package storage_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"boardgames-manager/internal/storage"
)

func TestSave_ValidPDFIsStoredAndReturnsContentAddressedName(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	content := "%PDF-1.4 fake pdf content for testing"
	name, err := store.Save(storage.ManualCategory, strings.NewReader(content))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !strings.HasSuffix(name, ".pdf") {
		t.Fatalf("expected .pdf extension, got %q", name)
	}

	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(data) != content {
		t.Fatalf("saved content mismatch")
	}
}

func TestSave_RejectsWrongType(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	_, err := store.Save(storage.ManualCategory, strings.NewReader("plain text, not a pdf"))
	if err != storage.ErrUnsupportedType {
		t.Fatalf("expected ErrUnsupportedType, got %v", err)
	}
}

func TestSave_RejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	tiny := storage.Category{
		Name:         "tiny",
		AllowedTypes: map[string]string{"application/pdf": ".pdf"},
		MaxBytes:     10,
	}
	_, err := store.Save(tiny, strings.NewReader("%PDF-1.4 this is definitely more than ten bytes"))
	if err != storage.ErrTooLarge {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

func TestSave_DeduplicatesIdenticalContent(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	content := "%PDF-1.4 identical content"
	name1, err := store.Save(storage.ManualCategory, strings.NewReader(content))
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	name2, err := store.Save(storage.ManualCategory, strings.NewReader(content))
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if name1 != name2 {
		t.Fatalf("expected same content-addressed name, got %q and %q", name1, name2)
	}
}

func TestOpen_ReturnsSavedContent(t *testing.T) {
	dir := t.TempDir()
	store := storage.NewStore(dir)

	content := "%PDF-1.4 open me"
	name, err := store.Save(storage.ManualCategory, strings.NewReader(content))
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	f, err := store.Open(name)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != content {
		t.Fatalf("content mismatch")
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

Run: `docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/storage/... -v`
Expected: FAIL (pacchetto `storage` non esiste)

- [ ] **Step 3: Implementare**

`backend/internal/storage/store.go`:
```go
package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type Category struct {
	Name         string
	AllowedTypes map[string]string // MIME type -> file extension
	MaxBytes     int64
}

var ManualCategory = Category{
	Name: "manual",
	AllowedTypes: map[string]string{
		"application/pdf": ".pdf",
	},
	MaxBytes: 20 << 20, // 20 MB
}

var CoverCategory = Category{
	Name: "cover",
	AllowedTypes: map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/webp": ".webp",
	},
	MaxBytes: 5 << 20, // 5 MB
}

var ErrUnsupportedType = errors.New("unsupported file type")
var ErrTooLarge = errors.New("file too large")

type Store struct {
	baseDir string
}

func NewStore(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

// Save reads r fully, validates its content type and size against category,
// and writes it to disk under a content-addressed filename (sha256 of the
// content + extension). Returns the filename (not a full path) to store in
// the DB. Saving identical content twice returns the same filename without
// writing a duplicate file.
func (s *Store) Save(category Category, r io.Reader) (string, error) {
	limited := io.LimitReader(r, category.MaxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read upload: %w", err)
	}
	if int64(len(data)) > category.MaxBytes {
		return "", ErrTooLarge
	}

	contentType := http.DetectContentType(data)
	ext, ok := category.AllowedTypes[contentType]
	if !ok {
		return "", ErrUnsupportedType
	}

	sum := sha256.Sum256(data)
	filename := hex.EncodeToString(sum[:]) + ext

	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return "", fmt.Errorf("create uploads dir: %w", err)
	}

	fullPath := filepath.Join(s.baseDir, filename)
	if _, err := os.Stat(fullPath); err == nil {
		return filename, nil
	}

	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write upload: %w", err)
	}

	return filename, nil
}

// Open opens a previously saved file by its filename (as returned by Save).
func (s *Store) Open(filename string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(s.baseDir, filename))
}
```

- [ ] **Step 4: Eseguire i test e verificare che passino**

Run: (stesso comando dello Step 2)
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/storage
git commit -m "feat: add content-addressed file storage"
```

---

### Task 3: Repository giochi — Game + GameLanguage

**Files:**
- Create: `backend/internal/db/migrations/0003_games.sql`
- Create: `backend/internal/games/store.go`
- Create: `backend/internal/games/store_test.go`

**Interfaces:**
- Produces: `games.Game{ID int64; BGGID *string; Name string; Year, MinPlayers, MaxPlayers, PlaytimeMinutes *int; Owner, CoverPath *string; CreatedAt time.Time}`, `games.GameLanguage{ID, GameID int64; LanguageCode string; IsBaseLanguage bool; Name string; Description *string}`, `games.GameUpdate{Owner, Year, MinPlayers, MaxPlayers, PlaytimeMinutes *...}`, `games.Store`, `games.NewStore(conn *sql.DB) *Store`, `(*Store).CreateGame`, `GetGame`, `ListGames`, `UpdateGame`, `DeleteGame`, `CreateLanguage`, `GetLanguage`, `ListLanguages`, `UpdateLanguage`, `games.ErrNotFound`.

- [ ] **Step 1: Scrivere la migrazione**

`backend/internal/db/migrations/0003_games.sql`:
```sql
CREATE TABLE games (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bgg_id TEXT,
    name TEXT NOT NULL,
    year INTEGER,
    min_players INTEGER,
    max_players INTEGER,
    playtime_minutes INTEGER,
    owner TEXT,
    cover_path TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE game_languages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    language_code TEXT NOT NULL,
    is_base_language INTEGER NOT NULL DEFAULT 0,
    name TEXT NOT NULL,
    description TEXT,
    UNIQUE(game_id, language_code)
);

CREATE UNIQUE INDEX idx_one_base_language_per_game
    ON game_languages(game_id) WHERE is_base_language = 1;

CREATE TABLE game_media (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_language_id INTEGER NOT NULL REFERENCES game_languages(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('file', 'link', 'youtube')),
    url_or_path TEXT NOT NULL,
    title TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

Nota: `game_media` è creata qui (serve alla FK `game_languages`), ma il repository per `GameMedia` arriva nel Task 4 — questo task copre solo `Game`/`GameLanguage`.

- [ ] **Step 2: Scrivere i test**

`backend/internal/games/store_test.go`:
```go
package games_test

import (
	"context"
	"errors"
	"testing"

	"boardgames-manager/internal/db"
	"boardgames-manager/internal/games"
)

func newTestStore(t *testing.T) *games.Store {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return games.NewStore(conn)
}

func strPtr(v string) *string { return &v }
func intPtr(v int) *int       { return &v }

func TestCreateAndGetGame(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	created, err := store.CreateGame(ctx, games.Game{
		Name: "Catan", Year: intPtr(1995), MinPlayers: intPtr(3), MaxPlayers: intPtr(4),
		PlaytimeMinutes: intPtr(90), Owner: strPtr("Mario"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected a non-zero id")
	}

	found, err := store.GetGame(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if found.Name != "Catan" || *found.Year != 1995 || *found.Owner != "Mario" {
		t.Fatalf("unexpected game: %+v", found)
	}
}

func TestGetGame_NotFound(t *testing.T) {
	store := newTestStore(t)
	_, err := store.GetGame(context.Background(), 999)
	if !errors.Is(err, games.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListGames_ReturnsAllCreated(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.CreateGame(ctx, games.Game{Name: "Azul"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.CreateGame(ctx, games.Game{Name: "Wingspan"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	list, err := store.ListGames(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 games, got %d", len(list))
	}
}

func TestUpdateGame_ChangesOnlyProvidedFields(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	created, err := store.CreateGame(ctx, games.Game{Name: "Azul", Owner: strPtr("Mario"), Year: intPtr(2017)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := store.UpdateGame(ctx, created.ID, games.GameUpdate{Owner: strPtr("Luigi")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if *updated.Owner != "Luigi" {
		t.Fatalf("expected owner Luigi, got %v", updated.Owner)
	}
	if *updated.Year != 2017 {
		t.Fatalf("expected year to stay 2017, got %v", updated.Year)
	}
}

func TestDeleteGame_RemovesIt(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	created, err := store.CreateGame(ctx, games.Game{Name: "Azul"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.DeleteGame(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = store.GetGame(ctx, created.ID)
	if !errors.Is(err, games.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteGame_NotFound(t *testing.T) {
	store := newTestStore(t)
	err := store.DeleteGame(context.Background(), 999)
	if !errors.Is(err, games.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCreateLanguageAndList(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	game, err := store.CreateGame(ctx, games.Game{Name: "Catan"})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}

	lang, err := store.CreateLanguage(ctx, games.GameLanguage{
		GameID: game.ID, LanguageCode: "it", IsBaseLanguage: true,
		Name: "Catan", Description: strPtr("Un gioco di insediamento."),
	})
	if err != nil {
		t.Fatalf("create language: %v", err)
	}
	if !lang.IsBaseLanguage {
		t.Fatal("expected is_base_language to be true")
	}

	list, err := store.ListLanguages(ctx, game.ID)
	if err != nil {
		t.Fatalf("list languages: %v", err)
	}
	if len(list) != 1 || list[0].LanguageCode != "it" {
		t.Fatalf("unexpected languages: %+v", list)
	}
}

func TestGetLanguage_NotFound(t *testing.T) {
	store := newTestStore(t)
	game, err := store.CreateGame(context.Background(), games.Game{Name: "Catan"})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	_, err = store.GetLanguage(context.Background(), game.ID, "en")
	if !errors.Is(err, games.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateLanguage_ChangesNameAndDescription(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	game, err := store.CreateGame(ctx, games.Game{Name: "Catan"})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	if _, err := store.CreateLanguage(ctx, games.GameLanguage{
		GameID: game.ID, LanguageCode: "it", IsBaseLanguage: true, Name: "Catan",
	}); err != nil {
		t.Fatalf("create language: %v", err)
	}

	updated, err := store.UpdateLanguage(ctx, game.ID, "it", "I Coloni di Catan", strPtr("Descrizione aggiornata."))
	if err != nil {
		t.Fatalf("update language: %v", err)
	}
	if updated.Name != "I Coloni di Catan" || *updated.Description != "Descrizione aggiornata." {
		t.Fatalf("unexpected updated language: %+v", updated)
	}
}
```

- [ ] **Step 3: Eseguire i test e verificare che falliscano**

Run: `docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/games/... -v`
Expected: FAIL (pacchetto `games` non esiste)

- [ ] **Step 4: Implementare il repository**

`backend/internal/games/store.go`:
```go
package games

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Game struct {
	ID              int64
	BGGID           *string
	Name            string
	Year            *int
	MinPlayers      *int
	MaxPlayers      *int
	PlaytimeMinutes *int
	Owner           *string
	CoverPath       *string
	CreatedAt       time.Time
}

type GameLanguage struct {
	ID             int64
	GameID         int64
	LanguageCode   string
	IsBaseLanguage bool
	Name           string
	Description    *string
}

type GameUpdate struct {
	Owner           *string
	Year            *int
	MinPlayers      *int
	MaxPlayers      *int
	PlaytimeMinutes *int
}

var ErrNotFound = errors.New("not found")

type Store struct {
	db *sql.DB
}

func NewStore(conn *sql.DB) *Store {
	return &Store{db: conn}
}

func (s *Store) CreateGame(ctx context.Context, g Game) (Game, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO games (bgg_id, name, year, min_players, max_players, playtime_minutes, owner, cover_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		g.BGGID, g.Name, g.Year, g.MinPlayers, g.MaxPlayers, g.PlaytimeMinutes, g.Owner, g.CoverPath,
	)
	if err != nil {
		return Game{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Game{}, err
	}
	return s.GetGame(ctx, id)
}

func (s *Store) GetGame(ctx context.Context, id int64) (Game, error) {
	var g Game
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, bgg_id, name, year, min_players, max_players, playtime_minutes, owner, cover_path, created_at
		 FROM games WHERE id = ?`, id,
	).Scan(&g.ID, &g.BGGID, &g.Name, &g.Year, &g.MinPlayers, &g.MaxPlayers, &g.PlaytimeMinutes, &g.Owner, &g.CoverPath, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Game{}, ErrNotFound
	}
	if err != nil {
		return Game{}, err
	}
	g.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return g, nil
}

func (s *Store) ListGames(ctx context.Context) ([]Game, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, bgg_id, name, year, min_players, max_players, playtime_minutes, owner, cover_path, created_at
		 FROM games ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Game
	for rows.Next() {
		var g Game
		var createdAt string
		if err := rows.Scan(&g.ID, &g.BGGID, &g.Name, &g.Year, &g.MinPlayers, &g.MaxPlayers, &g.PlaytimeMinutes, &g.Owner, &g.CoverPath, &createdAt); err != nil {
			return nil, err
		}
		g.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) UpdateGame(ctx context.Context, id int64, upd GameUpdate) (Game, error) {
	current, err := s.GetGame(ctx, id)
	if err != nil {
		return Game{}, err
	}
	if upd.Owner != nil {
		current.Owner = upd.Owner
	}
	if upd.Year != nil {
		current.Year = upd.Year
	}
	if upd.MinPlayers != nil {
		current.MinPlayers = upd.MinPlayers
	}
	if upd.MaxPlayers != nil {
		current.MaxPlayers = upd.MaxPlayers
	}
	if upd.PlaytimeMinutes != nil {
		current.PlaytimeMinutes = upd.PlaytimeMinutes
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE games SET owner = ?, year = ?, min_players = ?, max_players = ?, playtime_minutes = ? WHERE id = ?`,
		current.Owner, current.Year, current.MinPlayers, current.MaxPlayers, current.PlaytimeMinutes, id,
	)
	if err != nil {
		return Game{}, err
	}
	return s.GetGame(ctx, id)
}

func (s *Store) DeleteGame(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM games WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateLanguage(ctx context.Context, gl GameLanguage) (GameLanguage, error) {
	isBase := 0
	if gl.IsBaseLanguage {
		isBase = 1
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO game_languages (game_id, language_code, is_base_language, name, description)
		 VALUES (?, ?, ?, ?, ?)`,
		gl.GameID, gl.LanguageCode, isBase, gl.Name, gl.Description,
	)
	if err != nil {
		return GameLanguage{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return GameLanguage{}, err
	}
	return s.getLanguageByID(ctx, id)
}

func (s *Store) getLanguageByID(ctx context.Context, id int64) (GameLanguage, error) {
	var gl GameLanguage
	var isBase int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, game_id, language_code, is_base_language, name, description FROM game_languages WHERE id = ?`, id,
	).Scan(&gl.ID, &gl.GameID, &gl.LanguageCode, &isBase, &gl.Name, &gl.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return GameLanguage{}, ErrNotFound
	}
	if err != nil {
		return GameLanguage{}, err
	}
	gl.IsBaseLanguage = isBase != 0
	return gl, nil
}

func (s *Store) GetLanguage(ctx context.Context, gameID int64, code string) (GameLanguage, error) {
	var gl GameLanguage
	var isBase int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, game_id, language_code, is_base_language, name, description
		 FROM game_languages WHERE game_id = ? AND language_code = ?`, gameID, code,
	).Scan(&gl.ID, &gl.GameID, &gl.LanguageCode, &isBase, &gl.Name, &gl.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return GameLanguage{}, ErrNotFound
	}
	if err != nil {
		return GameLanguage{}, err
	}
	gl.IsBaseLanguage = isBase != 0
	return gl, nil
}

func (s *Store) ListLanguages(ctx context.Context, gameID int64) ([]GameLanguage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, game_id, language_code, is_base_language, name, description
		 FROM game_languages WHERE game_id = ? ORDER BY id`, gameID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GameLanguage
	for rows.Next() {
		var gl GameLanguage
		var isBase int
		if err := rows.Scan(&gl.ID, &gl.GameID, &gl.LanguageCode, &isBase, &gl.Name, &gl.Description); err != nil {
			return nil, err
		}
		gl.IsBaseLanguage = isBase != 0
		out = append(out, gl)
	}
	return out, rows.Err()
}

func (s *Store) UpdateLanguage(ctx context.Context, gameID int64, code string, name string, description *string) (GameLanguage, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE game_languages SET name = ?, description = ? WHERE game_id = ? AND language_code = ?`,
		name, description, gameID, code,
	)
	if err != nil {
		return GameLanguage{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return GameLanguage{}, err
	}
	if affected == 0 {
		return GameLanguage{}, ErrNotFound
	}
	return s.GetLanguage(ctx, gameID, code)
}
```

- [ ] **Step 5: Eseguire i test e verificare che passino**

Run: (stesso comando dello Step 3)
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/db/migrations/0003_games.sql backend/internal/games
git commit -m "feat: add games and game_languages repository"
```

---

### Task 4: Repository giochi — GameMedia

**Files:**
- Create: `backend/internal/games/media.go`
- Create: `backend/internal/games/media_test.go`

**Interfaces:**
- Consumes: `games.Store`, `games.ErrNotFound` (Task 3).
- Produces: `games.GameMedia{ID, GameLanguageID int64; Type string; URLOrPath string; Title *string; CreatedAt time.Time}`, `games.MediaTypeFile`, `games.MediaTypeLink`, `games.MediaTypeYoutube` (costanti stringa), `(*Store).CreateMedia`, `ListMedia`, `DeleteMedia`.

- [ ] **Step 1: Scrivere i test**

`backend/internal/games/media_test.go`:
```go
package games_test

import (
	"context"
	"errors"
	"testing"

	"boardgames-manager/internal/games"
)

func newTestLanguage(t *testing.T, store *games.Store) games.GameLanguage {
	t.Helper()
	ctx := context.Background()
	game, err := store.CreateGame(ctx, games.Game{Name: "Catan"})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	lang, err := store.CreateLanguage(ctx, games.GameLanguage{
		GameID: game.ID, LanguageCode: "it", IsBaseLanguage: true, Name: "Catan",
	})
	if err != nil {
		t.Fatalf("create language: %v", err)
	}
	return lang
}

func TestCreateAndListMedia(t *testing.T) {
	store := newTestStore(t)
	lang := newTestLanguage(t, store)
	ctx := context.Background()

	title := "Manuale ufficiale"
	media, err := store.CreateMedia(ctx, games.GameMedia{
		GameLanguageID: lang.ID, Type: games.MediaTypeFile, URLOrPath: "abc123.pdf", Title: &title,
	})
	if err != nil {
		t.Fatalf("create media: %v", err)
	}
	if media.Type != games.MediaTypeFile {
		t.Fatalf("unexpected type: %q", media.Type)
	}

	list, err := store.ListMedia(ctx, lang.ID)
	if err != nil {
		t.Fatalf("list media: %v", err)
	}
	if len(list) != 1 || list[0].URLOrPath != "abc123.pdf" {
		t.Fatalf("unexpected media list: %+v", list)
	}
}

func TestDeleteMedia_RemovesIt(t *testing.T) {
	store := newTestStore(t)
	lang := newTestLanguage(t, store)
	ctx := context.Background()

	media, err := store.CreateMedia(ctx, games.GameMedia{
		GameLanguageID: lang.ID, Type: games.MediaTypeLink, URLOrPath: "https://example.com/rules",
	})
	if err != nil {
		t.Fatalf("create media: %v", err)
	}

	if err := store.DeleteMedia(ctx, media.ID); err != nil {
		t.Fatalf("delete media: %v", err)
	}

	list, err := store.ListMedia(ctx, lang.ID)
	if err != nil {
		t.Fatalf("list media: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no media after delete, got %+v", list)
	}
}

func TestDeleteMedia_NotFound(t *testing.T) {
	store := newTestStore(t)
	err := store.DeleteMedia(context.Background(), 999)
	if !errors.Is(err, games.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

Run: `docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/games/... -run Media -v`
Expected: FAIL (`GameMedia`/`CreateMedia` non esistono)

- [ ] **Step 3: Implementare**

`backend/internal/games/media.go`:
```go
package games

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const (
	MediaTypeFile    = "file"
	MediaTypeLink    = "link"
	MediaTypeYoutube = "youtube"
)

type GameMedia struct {
	ID             int64
	GameLanguageID int64
	Type           string
	URLOrPath      string
	Title          *string
	CreatedAt      time.Time
}

func (s *Store) CreateMedia(ctx context.Context, m GameMedia) (GameMedia, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO game_media (game_language_id, type, url_or_path, title) VALUES (?, ?, ?, ?)`,
		m.GameLanguageID, m.Type, m.URLOrPath, m.Title,
	)
	if err != nil {
		return GameMedia{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return GameMedia{}, err
	}
	return s.getMediaByID(ctx, id)
}

func (s *Store) getMediaByID(ctx context.Context, id int64) (GameMedia, error) {
	var m GameMedia
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, game_language_id, type, url_or_path, title, created_at FROM game_media WHERE id = ?`, id,
	).Scan(&m.ID, &m.GameLanguageID, &m.Type, &m.URLOrPath, &m.Title, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GameMedia{}, ErrNotFound
	}
	if err != nil {
		return GameMedia{}, err
	}
	m.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return m, nil
}

func (s *Store) ListMedia(ctx context.Context, gameLanguageID int64) ([]GameMedia, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, game_language_id, type, url_or_path, title, created_at FROM game_media WHERE game_language_id = ? ORDER BY id`,
		gameLanguageID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GameMedia
	for rows.Next() {
		var m GameMedia
		var createdAt string
		if err := rows.Scan(&m.ID, &m.GameLanguageID, &m.Type, &m.URLOrPath, &m.Title, &createdAt); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) DeleteMedia(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM game_media WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 4: Eseguire i test e verificare che passino**

Run: (stesso comando dello Step 2, senza `-run`)

```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/games/... -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/games/media.go backend/internal/games/media_test.go
git commit -m "feat: add game media repository"
```

---

### Task 5: Client BGG (XML API2)

**Files:**
- Create: `backend/internal/bgg/client.go`
- Create: `backend/internal/bgg/client_test.go`

**Interfaces:**
- Produces: `bgg.SearchResult{ID string; Name string; Year int}`, `bgg.ThingDetail{ID, Name, Description string; Year, MinPlayers, MaxPlayers, PlayingTime int; ImageURL string}`, `bgg.Client` (interfaccia: `Search(ctx, token, query string) ([]SearchResult, error)`, `GetThing(ctx, token, id string) (ThingDetail, error)`), `bgg.HTTPClient{BaseURL string; HTTPClient *http.Client}`, `bgg.NewHTTPClient() *HTTPClient`.

- [ ] **Step 1: Scrivere i test**

`backend/internal/bgg/client_test.go`:
```go
package bgg_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/bgg"
)

func TestSearch_ParsesResultsAndSendsAuthHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Authorization header with test-token, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(`<?xml version="1.0"?>
<items>
	<item type="boardgame" id="13">
		<name type="primary" value="Catan"/>
		<yearpublished value="1995"/>
	</item>
</items>`))
	}))
	defer server.Close()

	client := &bgg.HTTPClient{BaseURL: server.URL, HTTPClient: server.Client()}
	results, err := client.Search(context.Background(), "test-token", "catan")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "13" || results[0].Name != "Catan" || results[0].Year != 1995 {
		t.Fatalf("unexpected result: %+v", results[0])
	}
}

func TestGetThing_ParsesDetailAndPicksPrimaryName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(`<?xml version="1.0"?>
<items>
	<item type="boardgame" id="13">
		<image>https://example.com/catan.jpg</image>
		<name type="primary" value="Catan"/>
		<name type="alternate" value="Die Siedler von Catan"/>
		<description>A game about settling an island.</description>
		<yearpublished value="1995"/>
		<minplayers value="3"/>
		<maxplayers value="4"/>
		<playingtime value="90"/>
	</item>
</items>`))
	}))
	defer server.Close()

	client := &bgg.HTTPClient{BaseURL: server.URL, HTTPClient: server.Client()}
	detail, err := client.GetThing(context.Background(), "test-token", "13")
	if err != nil {
		t.Fatalf("get thing: %v", err)
	}
	if detail.Name != "Catan" {
		t.Fatalf("expected primary name Catan, got %q", detail.Name)
	}
	if detail.Description != "A game about settling an island." {
		t.Fatalf("unexpected description: %q", detail.Description)
	}
	if detail.Year != 1995 || detail.MinPlayers != 3 || detail.MaxPlayers != 4 || detail.PlayingTime != 90 {
		t.Fatalf("unexpected numeric fields: %+v", detail)
	}
	if detail.ImageURL != "https://example.com/catan.jpg" {
		t.Fatalf("unexpected image url: %q", detail.ImageURL)
	}
}

func TestSearch_NonOKStatusReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
	}))
	defer server.Close()

	client := &bgg.HTTPClient{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := client.Search(context.Background(), "bad-token", "catan")
	if err == nil {
		t.Fatal("expected an error for non-200 status")
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

Run: `docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/bgg/... -v`
Expected: FAIL (pacchetto `bgg` non esiste)

- [ ] **Step 3: Implementare**

`backend/internal/bgg/client.go`:
```go
package bgg

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

const DefaultBaseURL = "https://boardgamegeek.com/xmlapi2"

type SearchResult struct {
	ID   string
	Name string
	Year int
}

type ThingDetail struct {
	ID          string
	Name        string
	Description string
	Year        int
	MinPlayers  int
	MaxPlayers  int
	PlayingTime int
	ImageURL    string
}

type Client interface {
	Search(ctx context.Context, token, query string) ([]SearchResult, error)
	GetThing(ctx context.Context, token, id string) (ThingDetail, error)
}

type HTTPClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		BaseURL:    DefaultBaseURL,
		HTTPClient: &http.Client{},
	}
}

type searchResponseXML struct {
	Items []struct {
		ID   string `xml:"id,attr"`
		Name struct {
			Value string `xml:"value,attr"`
		} `xml:"name"`
		YearPublished struct {
			Value string `xml:"value,attr"`
		} `xml:"yearpublished"`
	} `xml:"item"`
}

func (c *HTTPClient) Search(ctx context.Context, token, query string) ([]SearchResult, error) {
	q := url.Values{}
	q.Set("query", query)
	q.Set("type", "boardgame")

	body, err := c.doRequest(ctx, token, "/search", q)
	if err != nil {
		return nil, err
	}

	var parsed searchResponseXML
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse bgg search response: %w", err)
	}

	out := make([]SearchResult, 0, len(parsed.Items))
	for _, item := range parsed.Items {
		year, _ := strconv.Atoi(item.YearPublished.Value)
		out = append(out, SearchResult{ID: item.ID, Name: item.Name.Value, Year: year})
	}
	return out, nil
}

type thingResponseXML struct {
	Items []struct {
		ID    string `xml:"id,attr"`
		Image string `xml:"image"`
		Names []struct {
			Type  string `xml:"type,attr"`
			Value string `xml:"value,attr"`
		} `xml:"name"`
		Description   string `xml:"description"`
		YearPublished struct {
			Value string `xml:"value,attr"`
		} `xml:"yearpublished"`
		MinPlayers struct {
			Value string `xml:"value,attr"`
		} `xml:"minplayers"`
		MaxPlayers struct {
			Value string `xml:"value,attr"`
		} `xml:"maxplayers"`
		PlayingTime struct {
			Value string `xml:"value,attr"`
		} `xml:"playingtime"`
	} `xml:"item"`
}

func (c *HTTPClient) GetThing(ctx context.Context, token, id string) (ThingDetail, error) {
	q := url.Values{}
	q.Set("id", id)

	body, err := c.doRequest(ctx, token, "/thing", q)
	if err != nil {
		return ThingDetail{}, err
	}

	var parsed thingResponseXML
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return ThingDetail{}, fmt.Errorf("parse bgg thing response: %w", err)
	}
	if len(parsed.Items) == 0 {
		return ThingDetail{}, fmt.Errorf("bgg: no item found for id %s", id)
	}

	item := parsed.Items[0]
	primaryName := item.ID
	for _, n := range item.Names {
		if n.Type == "primary" {
			primaryName = n.Value
			break
		}
	}

	year, _ := strconv.Atoi(item.YearPublished.Value)
	minPlayers, _ := strconv.Atoi(item.MinPlayers.Value)
	maxPlayers, _ := strconv.Atoi(item.MaxPlayers.Value)
	playingTime, _ := strconv.Atoi(item.PlayingTime.Value)

	return ThingDetail{
		ID: item.ID, Name: primaryName, Description: item.Description,
		Year: year, MinPlayers: minPlayers, MaxPlayers: maxPlayers,
		PlayingTime: playingTime, ImageURL: item.Image,
	}, nil
}

// doRequest issues an authenticated GET request to the BGG XML API2.
//
// NOTE: BGG recently began requiring an application token for XML API
// access but does not clearly document the expected header format at the
// time this was written. "Authorization: Bearer <token>" is a best guess —
// if real requests fail with 401/403 once a real token is available, this
// is the single place to adjust (e.g. a different header name, or a raw
// token without the "Bearer " prefix).
func (c *HTTPClient) doRequest(ctx context.Context, token, path string, query url.Values) ([]byte, error) {
	fullURL := c.BaseURL + path + "?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bgg request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read bgg response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bgg returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
```

- [ ] **Step 4: Eseguire i test e verificare che passino**

Run: (stesso comando dello Step 2)
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/bgg
git commit -m "feat: add BoardGameGeek XML API2 client"
```

---

### Task 6: HTTP — creazione gioco (import BGG + manuale) e ricerca

**Files:**
- Create: `backend/internal/httpapi/games_handlers.go`
- Create: `backend/internal/httpapi/games_responses.go`
- Create: `backend/internal/httpapi/bgg_fake_test.go`
- Create: `backend/internal/httpapi/games_handlers_test.go`
- Modify: `backend/internal/httpapi/router.go`
- Modify: `backend/internal/httpapi/testhelpers_test.go`
- Modify: `backend/cmd/server/main.go`

**Interfaces:**
- Consumes: `games.Store`, `bgg.Client`, `storage.Store`, `s.Settings.Get` (Task 1, 2, 3, 4, 5).
- Produces: `Server` guadagna i campi `Games *games.Store`, `Storage *storage.Store`, `BGG bgg.Client`. `toGameSummary(g games.Game) map[string]any`, `toMediaResponse(m games.GameMedia) map[string]any`, `(*Server).toGameDetail(ctx, g games.Game, langs []games.GameLanguage) (map[string]any, error)` — riusati dai Task 7, 8, 9. `fakeBGGClient` in `bgg_fake_test.go` — riusato dai Task 7, 8, 9 nei test.

- [ ] **Step 1: Scrivere l'helper fake BGG per i test**

`backend/internal/httpapi/bgg_fake_test.go`:
```go
package httpapi_test

import (
	"context"

	"boardgames-manager/internal/bgg"
)

type fakeBGGClient struct {
	searchResults []bgg.SearchResult
	searchErr     error
	thing         bgg.ThingDetail
	thingErr      error
}

func (f *fakeBGGClient) Search(ctx context.Context, token, query string) ([]bgg.SearchResult, error) {
	return f.searchResults, f.searchErr
}

func (f *fakeBGGClient) GetThing(ctx context.Context, token, id string) (bgg.ThingDetail, error) {
	return f.thing, f.thingErr
}
```

- [ ] **Step 2: Aggiornare l'helper di test condiviso**

`backend/internal/httpapi/testhelpers_test.go` (file completo aggiornato):
```go
package httpapi_test

import (
	"context"
	"testing"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/db"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/httpapi"
	"boardgames-manager/internal/settings"
	"boardgames-manager/internal/storage"
	"boardgames-manager/internal/users"
)

func newTestServer(t *testing.T) *httpapi.Server {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &httpapi.Server{
		Users:    users.NewStore(conn),
		Sessions: auth.NewSessionStore(conn),
		Settings: settings.NewStore(conn),
		Games:    games.NewStore(conn),
		Storage:  storage.NewStore(t.TempDir()),
		BGG:      &fakeBGGClient{},
	}
}
```

- [ ] **Step 3: Scrivere i test degli handler di creazione**

`backend/internal/httpapi/games_handlers_test.go`:
```go
package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/bgg"
	"boardgames-manager/internal/httpapi"
)

func TestCreateGame_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	payload, _ := json.Marshal(map[string]string{"languageCode": "it", "name": "Test Game"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCreateGame_ManualCreationSucceeds(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]any{
		"languageCode":   "it",
		"name":           "Azul",
		"owner":          "Mario",
		"nameTranslated": "Azul",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Name      string `json:"name"`
		Languages []struct {
			Code string `json:"code"`
			Name string `json:"name"`
		} `json:"languages"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Name != "Azul" {
		t.Fatalf("expected name Azul, got %q", body.Name)
	}
	if len(body.Languages) != 1 || body.Languages[0].Code != "it" {
		t.Fatalf("expected one 'it' language, got %+v", body.Languages)
	}
}

func TestCreateGame_ManualCreationRequiresName(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"languageCode": "it"})
	req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreateGame_FromBGGRequiresToken(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"bggId": "13", "languageCode": "it"})
	req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 without a configured BGG token, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateGame_FromBGGSucceedsWithFakeClient(t *testing.T) {
	server := newTestServer(t)
	server.BGG = &fakeBGGClient{thing: bgg.ThingDetail{
		ID: "13", Name: "Catan", Description: "A settling game.",
		Year: 1995, MinPlayers: 3, MaxPlayers: 4, PlayingTime: 90,
	}}
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	settingsPayload, _ := json.Marshal(map[string]string{"defaultLanguage": "it", "bggApiToken": "fake-token"})
	settingsReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(settingsPayload))
	settingsReq.AddCookie(cookie)
	router.ServeHTTP(httptest.NewRecorder(), settingsReq)

	payload, _ := json.Marshal(map[string]string{"bggId": "13", "languageCode": "it", "owner": "Mario"})
	req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Name      string `json:"name"`
		Languages []struct {
			Name        string  `json:"name"`
			Description *string `json:"description"`
		} `json:"languages"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Name != "Catan" {
		t.Fatalf("expected name Catan, got %q", body.Name)
	}
	if len(body.Languages) != 1 || body.Languages[0].Name != "Catan" {
		t.Fatalf("expected base language prefilled with BGG name, got %+v", body.Languages)
	}
	if body.Languages[0].Description == nil || *body.Languages[0].Description != "A settling game." {
		t.Fatal("expected base language prefilled with BGG description")
	}
}

func TestSearchGames_RequiresToken(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	req := httptest.NewRequest(http.MethodGet, "/api/games/search?q=catan", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestSearchGames_ReturnsFakeResults(t *testing.T) {
	server := newTestServer(t)
	server.BGG = &fakeBGGClient{searchResults: []bgg.SearchResult{{ID: "13", Name: "Catan", Year: 1995}}}
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	settingsPayload, _ := json.Marshal(map[string]string{"defaultLanguage": "it", "bggApiToken": "fake-token"})
	settingsReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(settingsPayload))
	settingsReq.AddCookie(cookie)
	router.ServeHTTP(httptest.NewRecorder(), settingsReq)

	req := httptest.NewRequest(http.MethodGet, "/api/games/search?q=catan", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var results []struct {
		BGGID string `json:"bggId"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&results); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(results) != 1 || results[0].BGGID != "13" || results[0].Name != "Catan" {
		t.Fatalf("unexpected results: %+v", results)
	}
}
```

- [ ] **Step 4: Eseguire i test e verificare che falliscano**

Run: `docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/... -v`
Expected: FAIL (route/campi `Games`/`Storage`/`BGG` non esistono)

- [ ] **Step 5: Implementare i response builder condivisi**

`backend/internal/httpapi/games_responses.go`:
```go
package httpapi

import (
	"context"

	"boardgames-manager/internal/games"
)

func toGameSummary(g games.Game) map[string]any {
	return map[string]any{
		"id": g.ID, "bggId": g.BGGID, "name": g.Name, "year": g.Year,
		"minPlayers": g.MinPlayers, "maxPlayers": g.MaxPlayers,
		"playtimeMinutes": g.PlaytimeMinutes, "owner": g.Owner, "coverPath": g.CoverPath,
	}
}

func toMediaResponse(m games.GameMedia) map[string]any {
	return map[string]any{"id": m.ID, "type": m.Type, "url": m.URLOrPath, "title": m.Title}
}

func (s *Server) toGameDetail(ctx context.Context, g games.Game, langs []games.GameLanguage) (map[string]any, error) {
	langOut := make([]map[string]any, 0, len(langs))
	for _, l := range langs {
		media, err := s.Games.ListMedia(ctx, l.ID)
		if err != nil {
			return nil, err
		}
		mediaOut := make([]map[string]any, 0, len(media))
		for _, m := range media {
			mediaOut = append(mediaOut, toMediaResponse(m))
		}
		langOut = append(langOut, map[string]any{
			"code": l.LanguageCode, "isBaseLanguage": l.IsBaseLanguage,
			"name": l.Name, "description": l.Description, "media": mediaOut,
		})
	}
	detail := toGameSummary(g)
	detail["languages"] = langOut
	return detail, nil
}
```

- [ ] **Step 6: Implementare gli handler di creazione e ricerca**

`backend/internal/httpapi/games_handlers.go`:
```go
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"boardgames-manager/internal/games"
	"boardgames-manager/internal/storage"
)

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
	resp, err := http.DefaultClient.Do(req)
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
```

- [ ] **Step 7: Aggiornare il router**

`backend/internal/httpapi/router.go` (file completo aggiornato):
```go
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/bgg"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/settings"
	"boardgames-manager/internal/storage"
	"boardgames-manager/internal/users"
)

type Server struct {
	Users    *users.Store
	Sessions *auth.SessionStore
	Settings *settings.Store
	Games    *games.Store
	Storage  *storage.Store
	BGG      bgg.Client
}

func NewRouter(s *Server) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)

	r.Get("/api/health", healthHandler)
	r.Get("/api/bootstrap/status", s.bootstrapStatusHandler)
	r.Post("/api/bootstrap", s.bootstrapHandler)
	r.Post("/api/login", s.loginHandler)

	r.Group(func(protected chi.Router) {
		protected.Use(s.requireAuth)
		protected.Post("/api/logout", s.logoutHandler)
		protected.Get("/api/me", s.meHandler)
		protected.Get("/api/users", s.listUsersHandler)
		protected.Post("/api/users", s.createUserHandler)
		protected.Delete("/api/users/{id}", s.deleteUserHandler)
		protected.Get("/api/settings", s.getSettingsHandler)
		protected.Put("/api/settings", s.putSettingsHandler)
		protected.Get("/api/games/search", s.searchGamesHandler)
		protected.Post("/api/games", s.createGameHandler)
	})

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

- [ ] **Step 8: Aggiornare main.go**

`backend/cmd/server/main.go` (file completo aggiornato):
```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/bgg"
	"boardgames-manager/internal/db"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/httpapi"
	"boardgames-manager/internal/settings"
	"boardgames-manager/internal/storage"
	"boardgames-manager/internal/users"
	"boardgames-manager/internal/webui"
)

func main() {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	conn, err := db.Open(dataDir + "/app.db")
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer conn.Close()

	if err := db.Migrate(context.Background(), conn); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	server := &httpapi.Server{
		Users:    users.NewStore(conn),
		Sessions: auth.NewSessionStore(conn),
		Settings: settings.NewStore(conn),
		Games:    games.NewStore(conn),
		Storage:  storage.NewStore(dataDir + "/uploads"),
		BGG:      bgg.NewHTTPClient(),
	}

	apiRouter := httpapi.NewRouter(server)

	uiHandler, err := webui.Handler()
	if err != nil {
		log.Fatalf("load embedded frontend: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", apiRouter)
	mux.Handle("/", uiHandler)

	log.Printf("listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
```

- [ ] **Step 9: Eseguire tutti i test e verificare che passino**

Run: `docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./... -v`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add backend/internal/httpapi backend/cmd/server/main.go
git commit -m "feat: add game creation (BGG import + manual) and BGG search endpoints"
```

---

### Task 7: HTTP — lettura, modifica, eliminazione gioco

**Files:**
- Create: `backend/internal/httpapi/games_read_handlers.go`
- Create: `backend/internal/httpapi/games_read_handlers_test.go`
- Modify: `backend/internal/httpapi/router.go`

**Interfaces:**
- Consumes: `games.Store`, `toGameSummary`, `(*Server).toGameDetail` (Task 6).
- Produces: `parseIDParam(r *http.Request, name string) (int64, error)` — riusato dai Task 8, 9.

- [ ] **Step 1: Scrivere i test**

`backend/internal/httpapi/games_read_handlers_test.go`:
```go
package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func createTestGame(t *testing.T, router http.Handler, cookie *http.Cookie, name string) int64 {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"languageCode": "it", "name": name, "nameTranslated": name})
	req := httptest.NewRequest(http.MethodPost, "/api/games", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create game setup failed: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.ID
}

func TestListGames_IsPublic(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	createTestGame(t, router, cookie, "Azul")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/games", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 without auth, got %d", rec.Code)
	}
	var list []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 game, got %d", len(list))
	}
}

func TestGetGame_IsPublicAndIncludesLanguages(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/games/%d", id), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body struct {
		Name      string `json:"name"`
		Languages []any  `json:"languages"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Name != "Azul" || len(body.Languages) != 1 {
		t.Fatalf("unexpected detail: %+v", body)
	}
}

func TestGetGame_NotFoundReturns404(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/games/999", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestUpdateGame_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"owner": "Luigi"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/games/%d", id), bytes.NewReader(payload)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestUpdateGame_ChangesOwner(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"owner": "Luigi"})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/games/%d", id), bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Owner *string `json:"owner"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Owner == nil || *body.Owner != "Luigi" {
		t.Fatalf("expected owner Luigi, got %v", body.Owner)
	}
}

func TestDeleteGame_RemovesIt(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/games/%d", id), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/games/%d", id), nil))
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", getRec.Code)
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

Run: `docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/... -run TestListGames -v`
Expected: FAIL (route `GET /api/games` non esiste)

- [ ] **Step 3: Implementare gli handler**

`backend/internal/httpapi/games_read_handlers.go`:
```go
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"boardgames-manager/internal/games"
)

func parseIDParam(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, name), 10, 64)
}

func (s *Server) listGamesHandler(w http.ResponseWriter, r *http.Request) {
	list, err := s.Games.ListGames(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list games")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, g := range list {
		out = append(out, toGameSummary(g))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getGameHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}
	game, err := s.Games.GetGame(r.Context(), id)
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game")
		return
	}
	langs, err := s.Games.ListLanguages(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load languages")
		return
	}
	resp, err := s.toGameDetail(r.Context(), game, langs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not build response")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

type updateGameRequest struct {
	Owner           *string `json:"owner"`
	Year            *int    `json:"year"`
	MinPlayers      *int    `json:"minPlayers"`
	MaxPlayers      *int    `json:"maxPlayers"`
	PlaytimeMinutes *int    `json:"playtimeMinutes"`
}

func (s *Server) updateGameHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}
	var req updateGameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	game, err := s.Games.UpdateGame(r.Context(), id, games.GameUpdate{
		Owner: req.Owner, Year: req.Year, MinPlayers: req.MinPlayers,
		MaxPlayers: req.MaxPlayers, PlaytimeMinutes: req.PlaytimeMinutes,
	})
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update game")
		return
	}
	writeJSON(w, http.StatusOK, toGameSummary(game))
}

func (s *Server) deleteGameHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}
	if err := s.Games.DeleteGame(r.Context(), id); errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "game not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete game")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
```

- [ ] **Step 4: Aggiornare il router**

`backend/internal/httpapi/router.go` (file completo aggiornato):
```go
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/bgg"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/settings"
	"boardgames-manager/internal/storage"
	"boardgames-manager/internal/users"
)

type Server struct {
	Users    *users.Store
	Sessions *auth.SessionStore
	Settings *settings.Store
	Games    *games.Store
	Storage  *storage.Store
	BGG      bgg.Client
}

func NewRouter(s *Server) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)

	r.Get("/api/health", healthHandler)
	r.Get("/api/bootstrap/status", s.bootstrapStatusHandler)
	r.Post("/api/bootstrap", s.bootstrapHandler)
	r.Post("/api/login", s.loginHandler)
	r.Get("/api/games", s.listGamesHandler)
	r.Get("/api/games/{id}", s.getGameHandler)

	r.Group(func(protected chi.Router) {
		protected.Use(s.requireAuth)
		protected.Post("/api/logout", s.logoutHandler)
		protected.Get("/api/me", s.meHandler)
		protected.Get("/api/users", s.listUsersHandler)
		protected.Post("/api/users", s.createUserHandler)
		protected.Delete("/api/users/{id}", s.deleteUserHandler)
		protected.Get("/api/settings", s.getSettingsHandler)
		protected.Put("/api/settings", s.putSettingsHandler)
		protected.Get("/api/games/search", s.searchGamesHandler)
		protected.Post("/api/games", s.createGameHandler)
		protected.Patch("/api/games/{id}", s.updateGameHandler)
		protected.Delete("/api/games/{id}", s.deleteGameHandler)
	})

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

- [ ] **Step 5: Eseguire tutti i test e verificare che passino**

Run: `docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi
git commit -m "feat: add game read, update and delete endpoints"
```

---

### Task 8: HTTP — gestione lingue di un gioco

**Files:**
- Create: `backend/internal/httpapi/game_languages_handlers.go`
- Create: `backend/internal/httpapi/game_languages_handlers_test.go`
- Modify: `backend/internal/httpapi/router.go`

**Interfaces:**
- Consumes: `games.Store`, `parseIDParam` (Task 3, 7).

- [ ] **Step 1: Scrivere i test**

`backend/internal/httpapi/game_languages_handlers_test.go`:
```go
package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func TestCreateLanguage_PrefillsFromBaseLanguage(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"languageCode": "en"})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages", id), bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Code           string `json:"code"`
		IsBaseLanguage bool   `json:"isBaseLanguage"`
		Name           string `json:"name"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != "en" || body.IsBaseLanguage {
		t.Fatalf("unexpected new language: %+v", body)
	}
	if body.Name != "Azul" {
		t.Fatalf("expected new language prefilled with base name 'Azul', got %q", body.Name)
	}
}

func TestCreateLanguage_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"languageCode": "en"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages", id), bytes.NewReader(payload)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestUpdateLanguage_ChangesNameAndDescription(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"name": "Azul (IT)", "description": "Un gioco di piastrelle."})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/games/%d/languages/it", id), bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Name != "Azul (IT)" || body.Description == nil || *body.Description != "Un gioco di piastrelle." {
		t.Fatalf("unexpected updated language: %+v", body)
	}
}

func TestUpdateLanguage_NotFoundReturns404(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"name": "Nope"})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/games/%d/languages/de", id), bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

Run: `docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/... -run TestCreateLanguage -v`
Expected: FAIL (route non esiste)

- [ ] **Step 3: Implementare gli handler**

`backend/internal/httpapi/game_languages_handlers.go`:
```go
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.LanguageCode == "" {
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

	// Pre-fill from the base language's text as a translation starting point.
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

	lang, err := s.Games.CreateLanguage(r.Context(), games.GameLanguage{
		GameID: gameID, LanguageCode: req.LanguageCode, IsBaseLanguage: false,
		Name: name, Description: description,
	})
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
```

- [ ] **Step 4: Aggiornare il router**

Nel blocco `protected` di `backend/internal/httpapi/router.go`, aggiungere dopo `protected.Delete("/api/games/{id}", s.deleteGameHandler)`:
```go
		protected.Post("/api/games/{id}/languages", s.createLanguageHandler)
		protected.Patch("/api/games/{id}/languages/{lang}", s.updateLanguageHandler)
```

- [ ] **Step 5: Eseguire tutti i test e verificare che passino**

Run: `docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi
git commit -m "feat: add game language management endpoints"
```

---

### Task 9: HTTP — media (upload/link/youtube) e file serving pubblico

**Files:**
- Create: `backend/internal/httpapi/game_media_handlers.go`
- Create: `backend/internal/httpapi/game_media_handlers_test.go`
- Modify: `backend/internal/httpapi/router.go`

**Interfaces:**
- Consumes: `games.Store`, `storage.Store`, `parseIDParam`, `toMediaResponse` (Task 2, 3, 4, 6, 7).

- [ ] **Step 1: Scrivere i test**

`backend/internal/httpapi/game_media_handlers_test.go`:
```go
package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func TestCreateMedia_LinkSucceeds(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"type": "link", "url": "https://example.com/rules.pdf", "title": "Regolamento"})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages/it/media", id), bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateMedia_YoutubeRejectsNonYoutubeURL(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"type": "youtube", "url": "https://example.com/video"})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages/it/media", id), bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreateMedia_FileUploadSucceedsAndIsPubliclyServed(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "manuale.pdf")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	part.Write([]byte("%PDF-1.4 fake manual content"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages/it/media", id), &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/api/uploads/"+body.URL, nil))
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected uploaded file to be servable without auth, got %d", getRec.Code)
	}
}

func TestDeleteMedia_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	id := createTestGame(t, router, cookie, "Azul")

	payload, _ := json.Marshal(map[string]string{"type": "link", "url": "https://example.com/rules"})
	createReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/games/%d/languages/it/media", id), bytes.NewReader(payload))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(cookie)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	var created struct {
		ID int64 `json:"id"`
	}
	json.NewDecoder(createRec.Body).Decode(&created)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/games/%d/languages/it/media/%d", id, created.ID), nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

Run: `docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/... -run TestCreateMedia -v`
Expected: FAIL (route non esiste)

- [ ] **Step 3: Implementare gli handler**

`backend/internal/httpapi/game_media_handlers.go`:
```go
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"boardgames-manager/internal/games"
	"boardgames-manager/internal/storage"
)

func (s *Server) createMediaHandler(w http.ResponseWriter, r *http.Request) {
	gameID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}
	code := chi.URLParam(r, "lang")
	lang, err := s.Games.GetLanguage(r.Context(), gameID, code)
	if errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "language not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load language")
		return
	}

	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		s.createFileMediaHandler(w, r, lang)
		return
	}
	s.createLinkMediaHandler(w, r, lang)
}

func (s *Server) createFileMediaHandler(w http.ResponseWriter, r *http.Request, lang games.GameLanguage) {
	if err := r.ParseMultipartForm(storage.ManualCategory.MaxBytes + 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	path, err := s.Storage.Save(storage.ManualCategory, file)
	if errors.Is(err, storage.ErrUnsupportedType) {
		writeError(w, http.StatusBadRequest, "only PDF files are allowed")
		return
	}
	if errors.Is(err, storage.ErrTooLarge) {
		writeError(w, http.StatusBadRequest, "file exceeds the 20MB limit")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save file")
		return
	}

	title := r.FormValue("title")
	if title == "" {
		title = header.Filename
	}

	media, err := s.Games.CreateMedia(r.Context(), games.GameMedia{
		GameLanguageID: lang.ID, Type: games.MediaTypeFile, URLOrPath: path, Title: &title,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save media")
		return
	}
	writeJSON(w, http.StatusCreated, toMediaResponse(media))
}

type createLinkMediaRequest struct {
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

func (s *Server) createLinkMediaHandler(w http.ResponseWriter, r *http.Request, lang games.GameLanguage) {
	var req createLinkMediaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Type != games.MediaTypeLink && req.Type != games.MediaTypeYoutube {
		writeError(w, http.StatusBadRequest, "type must be 'link' or 'youtube'")
		return
	}
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		writeError(w, http.StatusBadRequest, "url must start with http:// or https://")
		return
	}
	if req.Type == games.MediaTypeYoutube && !strings.Contains(req.URL, "youtube.com") && !strings.Contains(req.URL, "youtu.be") {
		writeError(w, http.StatusBadRequest, "youtube url must contain youtube.com or youtu.be")
		return
	}

	var titlePtr *string
	if req.Title != "" {
		titlePtr = &req.Title
	}

	media, err := s.Games.CreateMedia(r.Context(), games.GameMedia{
		GameLanguageID: lang.ID, Type: req.Type, URLOrPath: req.URL, Title: titlePtr,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save media")
		return
	}
	writeJSON(w, http.StatusCreated, toMediaResponse(media))
}

func (s *Server) deleteMediaHandler(w http.ResponseWriter, r *http.Request) {
	mediaID, err := parseIDParam(r, "mediaId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid media id")
		return
	}
	if err := s.Games.DeleteMedia(r.Context(), mediaID); errors.Is(err, games.ErrNotFound) {
		writeError(w, http.StatusNotFound, "media not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete media")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) getUploadHandler(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	if strings.Contains(filename, "/") || strings.Contains(filename, "..") {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}
	f, err := s.Storage.Open(filename)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	defer f.Close()

	ext := filepath.Ext(filename)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	io.Copy(w, f)
}
```

- [ ] **Step 4: Aggiornare il router**

`backend/internal/httpapi/router.go` (file completo aggiornato):
```go
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/bgg"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/settings"
	"boardgames-manager/internal/storage"
	"boardgames-manager/internal/users"
)

type Server struct {
	Users    *users.Store
	Sessions *auth.SessionStore
	Settings *settings.Store
	Games    *games.Store
	Storage  *storage.Store
	BGG      bgg.Client
}

func NewRouter(s *Server) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)

	r.Get("/api/health", healthHandler)
	r.Get("/api/bootstrap/status", s.bootstrapStatusHandler)
	r.Post("/api/bootstrap", s.bootstrapHandler)
	r.Post("/api/login", s.loginHandler)
	r.Get("/api/games", s.listGamesHandler)
	r.Get("/api/games/{id}", s.getGameHandler)
	r.Get("/api/uploads/{filename}", s.getUploadHandler)

	r.Group(func(protected chi.Router) {
		protected.Use(s.requireAuth)
		protected.Post("/api/logout", s.logoutHandler)
		protected.Get("/api/me", s.meHandler)
		protected.Get("/api/users", s.listUsersHandler)
		protected.Post("/api/users", s.createUserHandler)
		protected.Delete("/api/users/{id}", s.deleteUserHandler)
		protected.Get("/api/settings", s.getSettingsHandler)
		protected.Put("/api/settings", s.putSettingsHandler)
		protected.Get("/api/games/search", s.searchGamesHandler)
		protected.Post("/api/games", s.createGameHandler)
		protected.Patch("/api/games/{id}", s.updateGameHandler)
		protected.Delete("/api/games/{id}", s.deleteGameHandler)
		protected.Post("/api/games/{id}/languages", s.createLanguageHandler)
		protected.Patch("/api/games/{id}/languages/{lang}", s.updateLanguageHandler)
		protected.Post("/api/games/{id}/languages/{lang}/media", s.createMediaHandler)
		protected.Delete("/api/games/{id}/languages/{lang}/media/{mediaId}", s.deleteMediaHandler)
	})

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

- [ ] **Step 5: Eseguire tutti i test e verificare che passino**

Run: `docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi
git commit -m "feat: add game media endpoints and public upload serving"
```

---

### Task 10: Frontend — client API (patch/upload), elenco giochi

**Files:**
- Modify: `frontend/src/api/client.ts`
- Modify: `frontend/src/views/DashboardLayout.vue`
- Modify: `frontend/src/router/index.ts`
- Create: `frontend/src/views/GamesView.vue`
- Modify: `frontend/src/app.css`

**Interfaces:**
- Produces: `api.patch<T>(path, body?)`, `api.upload<T>(path, formData)` — riusati dai Task 11, 12, 13.

- [ ] **Step 1: Estendere il client API**

`frontend/src/api/client.ts` (file completo aggiornato — le uniche righe nuove sono nella funzione `request` per FormData e il metodo `patch`/`upload` in `api`, il resto è invariato rispetto a quanto già in repo):
```ts
const BASE_URL = '/api'

const LOGIN_PATH = '/login'

/** Set once we start navigating, so a burst of 401s only redirects once. */
let redirecting = false

/**
 * A session can go invalid at any moment: it expires in a long-open tab, or an
 * admin deletes the account (which cascade-deletes its sessions). Without a
 * central handler every caller has to cope on its own, and the UI dead-ends —
 * even the logout button stops working once POST /logout is the call that 401s.
 *
 * A full page load is the cheapest reliable reset: it clears the Pinia store,
 * the router state and any in-flight requests without this module having to
 * import — and circularly depend on — the auth store or the router. 401s are
 * rare in an admin panel, so the reload costs nothing in practice.
 */
function redirectToLogin() {
  // The login page itself probes GET /api/me through the auth store and gets a
  // 401 whenever nobody is signed in — that is the normal state, not a session
  // loss. Redirecting on it would reload /login forever.
  if (redirecting || window.location.pathname === LOGIN_PATH) {
    return
  }
  redirecting = true
  window.location.href = LOGIN_PATH
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const isFormData = options.body instanceof FormData
  const headers: Record<string, string> = {
    ...(isFormData ? {} : { 'Content-Type': 'application/json' }),
    ...((options.headers as Record<string, string>) || {}),
  }

  const res = await fetch(`${BASE_URL}${path}`, {
    ...options,
    credentials: 'include',
    headers,
  })

  if (!res.ok) {
    // Only 401 means "your session is gone"; a 404 or 409 from a normal call
    // must still just throw for the caller's own error handling.
    if (res.status === 401) {
      redirectToLogin()
    }
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error || 'Request failed')
  }

  if (res.status === 204) {
    return undefined as T
  }

  return res.json() as Promise<T>
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, {
      method: 'POST',
      body: body instanceof FormData ? body : body ? JSON.stringify(body) : undefined,
    }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PUT', body: JSON.stringify(body) }),
  patch: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PATCH', body: JSON.stringify(body) }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
}
```

Nota: `api.post` ora accetta anche un `FormData` come body (usato dal Task 13 per l'upload dei manuali) oltre al normale oggetto JSON — non serve un metodo `upload` separato, `post` rileva da solo il tipo del body.

- [ ] **Step 2: Creare la vista elenco giochi**

`frontend/src/views/GamesView.vue`:
```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'

interface GameSummary {
  id: number
  name: string
  year: number | null
  owner: string | null
  coverPath: string | null
}

const games = ref<GameSummary[]>([])
const error = ref('')

async function loadGames() {
  try {
    games.value = await api.get<GameSummary[]>('/games')
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(loadGames)
</script>

<template>
  <div>
    <h1>Catalogo giochi</h1>
    <router-link to="/games/new">Aggiungi gioco</router-link>
    <p v-if="error" class="error">{{ error }}</p>
    <ul class="game-grid">
      <li v-for="g in games" :key="g.id">
        <router-link :to="`/games/${g.id}`">
          <img v-if="g.coverPath" :src="`/api/uploads/${g.coverPath}`" :alt="g.name" />
          <h2>{{ g.name }}</h2>
          <p v-if="g.year">{{ g.year }}</p>
          <p v-if="g.owner">Proprietario: {{ g.owner }}</p>
        </router-link>
      </li>
    </ul>
  </div>
</template>
```

Nota importante: i link usano stringhe di path semplici (`to="/games/new"`, `:to="`/games/${g.id}`"`), **non** oggetti `{name: '...'}` — le route `game-new`/`game-detail` non esistono ancora in questo task (arrivano nei Task 11/12) e una `RouterLink` con un nome non registrato lancia un errore che blocca il rendering dell'intera pagina (vedi Global Constraints).

- [ ] **Step 3: Aggiungere la route e il link di navigazione**

`frontend/src/router/index.ts`: aggiungere l'import `import GamesView from '../views/GamesView.vue'` e, nell'array `children` della route `/`, la riga:
```ts
{ path: 'games', name: 'games', component: GamesView },
```

`frontend/src/views/DashboardLayout.vue`: aggiungere un link "Giochi" nella `<nav>`, prima di "Utenti":
```html
<router-link :to="{ name: 'games' }">Giochi</router-link>
```

- [ ] **Step 4: Aggiungere stile minimo per la griglia dei giochi**

Aggiungere in fondo a `frontend/src/app.css`:
```css
.game-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 1rem;
  list-style: none;
  padding: 0;
}

.game-grid li {
  border: 1px solid #d0d5dd;
  border-radius: 8px;
  padding: 1rem;
  text-align: center;
}

.game-grid img {
  max-width: 100%;
  border-radius: 4px;
  margin-bottom: 0.5rem;
}

.game-grid a {
  text-decoration: none;
  color: inherit;
}
```

- [ ] **Step 5: Verificare che il progetto compili**

Run: `cd frontend && npm run build`
Expected: build completata senza errori TypeScript.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/api/client.ts frontend/src/views/GamesView.vue frontend/src/router/index.ts frontend/src/views/DashboardLayout.vue frontend/src/app.css
git commit -m "feat: add games list view and extend api client with patch"
```

---

### Task 11: Frontend — flusso creazione gioco (ricerca BGG + form manuale)

**Files:**
- Create: `frontend/src/views/GameNewView.vue`
- Modify: `frontend/src/router/index.ts`

**Interfaces:**
- Consumes: `api.get`, `api.post` (Task 10).

- [ ] **Step 1: Creare la vista di creazione**

`frontend/src/views/GameNewView.vue`:
```vue
<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client'

interface BGGSearchResult {
  bggId: string
  name: string
  year: number
}

const router = useRouter()
const mode = ref<'search' | 'bgg-import' | 'manual'>('search')

const query = ref('')
const results = ref<BGGSearchResult[]>([])
const searchError = ref('')

const selectedBggId = ref('')
const selectedName = ref('')
const languageCode = ref('it')
const owner = ref('')

const manualName = ref('')
const manualYear = ref<number | null>(null)
const manualMinPlayers = ref<number | null>(null)
const manualMaxPlayers = ref<number | null>(null)
const manualPlaytime = ref<number | null>(null)

const createError = ref('')

async function search() {
  searchError.value = ''
  try {
    results.value = await api.get<BGGSearchResult[]>(`/games/search?q=${encodeURIComponent(query.value)}`)
  } catch (e) {
    searchError.value = (e as Error).message
  }
}

function selectResult(r: BGGSearchResult) {
  selectedBggId.value = r.bggId
  selectedName.value = r.name
  mode.value = 'bgg-import'
}

function startManual() {
  mode.value = 'manual'
}

async function createFromBGG() {
  createError.value = ''
  try {
    const game = await api.post<{ id: number }>('/games', {
      bggId: selectedBggId.value,
      languageCode: languageCode.value,
      owner: owner.value,
    })
    router.push(`/games/${game.id}`)
  } catch (e) {
    createError.value = (e as Error).message
  }
}

async function createManual() {
  createError.value = ''
  try {
    const game = await api.post<{ id: number }>('/games', {
      name: manualName.value,
      year: manualYear.value,
      minPlayers: manualMinPlayers.value,
      maxPlayers: manualMaxPlayers.value,
      playtimeMinutes: manualPlaytime.value,
      owner: owner.value,
      languageCode: languageCode.value,
      nameTranslated: manualName.value,
    })
    router.push(`/games/${game.id}`)
  } catch (e) {
    createError.value = (e as Error).message
  }
}
</script>

<template>
  <div>
    <h1>Aggiungi gioco</h1>

    <div v-if="mode === 'search'">
      <form @submit.prevent="search">
        <label>
          Cerca su BoardGameGeek
          <input v-model="query" required />
        </label>
        <button type="submit">Cerca</button>
      </form>
      <p v-if="searchError" class="error">{{ searchError }}</p>
      <ul>
        <li v-for="r in results" :key="r.bggId">
          <button type="button" @click="selectResult(r)">{{ r.name }} ({{ r.year }})</button>
        </li>
      </ul>
      <button type="button" @click="startManual">Crea manualmente</button>
    </div>

    <div v-if="mode === 'bgg-import'">
      <h2>{{ selectedName }}</h2>
      <form @submit.prevent="createFromBGG">
        <label>
          Lingua base
          <select v-model="languageCode">
            <option value="it">Italiano</option>
            <option value="en">Inglese</option>
          </select>
        </label>
        <label>
          Proprietario
          <input v-model="owner" />
        </label>
        <button type="submit">Importa</button>
      </form>
      <p v-if="createError" class="error">{{ createError }}</p>
    </div>

    <div v-if="mode === 'manual'">
      <form @submit.prevent="createManual">
        <label>
          Nome
          <input v-model="manualName" required />
        </label>
        <label>
          Anno
          <input v-model.number="manualYear" type="number" />
        </label>
        <label>
          Min giocatori
          <input v-model.number="manualMinPlayers" type="number" />
        </label>
        <label>
          Max giocatori
          <input v-model.number="manualMaxPlayers" type="number" />
        </label>
        <label>
          Durata (minuti)
          <input v-model.number="manualPlaytime" type="number" />
        </label>
        <label>
          Lingua base
          <select v-model="languageCode">
            <option value="it">Italiano</option>
            <option value="en">Inglese</option>
          </select>
        </label>
        <label>
          Proprietario
          <input v-model="owner" />
        </label>
        <button type="submit">Crea</button>
      </form>
      <p v-if="createError" class="error">{{ createError }}</p>
    </div>
  </div>
</template>
```

Nota: `router.push(`/games/${game.id}`)` usa un path semplice, non `{name: 'game-detail'}` — quella route arriva nel Task 12; un `push` verso una route inesistente fallisce silenziosamente (nessun match, non naviga) ma non crasha nulla — a differenza di una `RouterLink`, `push` non blocca il rendering della pagina corrente.

- [ ] **Step 2: Aggiungere la route**

`frontend/src/router/index.ts`: aggiungere l'import `import GameNewView from '../views/GameNewView.vue'` e, nell'array `children`, la riga:
```ts
{ path: 'games/new', name: 'game-new', component: GameNewView },
```

- [ ] **Step 3: Verificare che il progetto compili**

Run: `cd frontend && npm run build`
Expected: build completata senza errori TypeScript.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/GameNewView.vue frontend/src/router/index.ts
git commit -m "feat: add game creation flow (BGG search + manual form)"
```

---

### Task 12: Frontend — dettaglio gioco (dati base + lingue)

**Files:**
- Create: `frontend/src/views/GameDetailView.vue`
- Modify: `frontend/src/router/index.ts`

**Interfaces:**
- Consumes: `api.get`, `api.patch`, `api.post`, `api.delete` (Task 10).
- Produces: struttura dati `GameDetail`/`GameLanguageInfo` nel file — riusata (estesa) dal Task 13 che modifica questo stesso file per aggiungere la gestione dei media.

- [ ] **Step 1: Creare la vista di dettaglio**

`frontend/src/views/GameDetailView.vue`:
```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'

interface GameLanguageInfo {
  code: string
  isBaseLanguage: boolean
  name: string
  description: string | null
}

interface GameDetail {
  id: number
  name: string
  year: number | null
  owner: string | null
  coverPath: string | null
  languages: GameLanguageInfo[]
}

const route = useRoute()
const router = useRouter()
const gameId = route.params.id as string

const game = ref<GameDetail | null>(null)
const error = ref('')
const activeLangCode = ref('')

const editName = ref('')
const editDescription = ref('')
const saveMessage = ref('')

const newLangCode = ref('')

async function load() {
  game.value = await api.get<GameDetail>(`/games/${gameId}`)
}

function selectLanguage(code: string) {
  activeLangCode.value = code
  const lang = game.value?.languages.find((l) => l.code === code)
  if (lang) {
    editName.value = lang.name
    editDescription.value = lang.description || ''
  }
}

async function saveLanguage() {
  error.value = ''
  saveMessage.value = ''
  try {
    await api.patch(`/games/${gameId}/languages/${activeLangCode.value}`, {
      name: editName.value,
      description: editDescription.value || null,
    })
    saveMessage.value = 'Salvato'
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function addLanguage() {
  error.value = ''
  try {
    await api.post(`/games/${gameId}/languages`, { languageCode: newLangCode.value })
    newLangCode.value = ''
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function deleteGame() {
  try {
    await api.delete(`/games/${gameId}`)
    router.push('/games')
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(async () => {
  try {
    await load()
    if (game.value && game.value.languages.length > 0) {
      selectLanguage(game.value.languages[0].code)
    }
  } catch (e) {
    error.value = (e as Error).message
  }
})
</script>

<template>
  <div v-if="game">
    <h1>{{ game.name }}</h1>
    <img v-if="game.coverPath" :src="`/api/uploads/${game.coverPath}`" :alt="game.name" class="cover" />
    <p v-if="game.owner">Proprietario: {{ game.owner }}</p>
    <button type="button" @click="deleteGame">Elimina gioco</button>

    <nav class="language-tabs">
      <button
        v-for="l in game.languages"
        :key="l.code"
        type="button"
        :class="{ active: l.code === activeLangCode }"
        @click="selectLanguage(l.code)"
      >
        {{ l.code }}
      </button>
    </nav>

    <form @submit.prevent="saveLanguage">
      <label>
        Nome
        <input v-model="editName" required />
      </label>
      <label>
        Descrizione
        <textarea v-model="editDescription"></textarea>
      </label>
      <button type="submit">Salva</button>
      <p v-if="saveMessage" class="success">{{ saveMessage }}</p>
    </form>

    <form @submit.prevent="addLanguage">
      <label>
        Aggiungi lingua (es. en)
        <input v-model="newLangCode" required />
      </label>
      <button type="submit">Aggiungi</button>
    </form>

    <p v-if="error" class="error">{{ error }}</p>
  </div>
</template>
```

- [ ] **Step 2: Aggiungere la route**

`frontend/src/router/index.ts`: aggiungere l'import `import GameDetailView from '../views/GameDetailView.vue'` e, nell'array `children`, la riga:
```ts
{ path: 'games/:id', name: 'game-detail', component: GameDetailView },
```

- [ ] **Step 3: Verificare che il progetto compili**

Run: `cd frontend && npm run build`
Expected: build completata senza errori TypeScript.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/GameDetailView.vue frontend/src/router/index.ts
git commit -m "feat: add game detail view with language management"
```

---

### Task 13: Frontend — gestione media nel dettaglio gioco

**Files:**
- Modify: `frontend/src/views/GameDetailView.vue`

**Interfaces:**
- Consumes: `api.post` (con `FormData` per l'upload), `api.delete` (Task 10).

- [ ] **Step 1: Estendere GameDetailView.vue con la gestione media**

Sostituire l'intero contenuto di `frontend/src/views/GameDetailView.vue` con:
```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'

interface GameMediaInfo {
  id: number
  type: 'file' | 'link' | 'youtube'
  url: string
  title: string | null
}

interface GameLanguageInfo {
  code: string
  isBaseLanguage: boolean
  name: string
  description: string | null
  media: GameMediaInfo[]
}

interface GameDetail {
  id: number
  name: string
  year: number | null
  owner: string | null
  coverPath: string | null
  languages: GameLanguageInfo[]
}

const route = useRoute()
const router = useRouter()
const gameId = route.params.id as string

const game = ref<GameDetail | null>(null)
const error = ref('')
const activeLangCode = ref('')

const editName = ref('')
const editDescription = ref('')
const saveMessage = ref('')

const newLangCode = ref('')

const linkUrl = ref('')
const linkTitle = ref('')
const linkType = ref<'link' | 'youtube'>('link')
const uploadFile = ref<File | null>(null)
const mediaError = ref('')

function activeLanguage(): GameLanguageInfo | undefined {
  return game.value?.languages.find((l) => l.code === activeLangCode.value)
}

async function load() {
  game.value = await api.get<GameDetail>(`/games/${gameId}`)
}

function selectLanguage(code: string) {
  activeLangCode.value = code
  const lang = activeLanguage()
  if (lang) {
    editName.value = lang.name
    editDescription.value = lang.description || ''
  }
}

async function saveLanguage() {
  error.value = ''
  saveMessage.value = ''
  try {
    await api.patch(`/games/${gameId}/languages/${activeLangCode.value}`, {
      name: editName.value,
      description: editDescription.value || null,
    })
    saveMessage.value = 'Salvato'
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function addLanguage() {
  error.value = ''
  try {
    await api.post(`/games/${gameId}/languages`, { languageCode: newLangCode.value })
    newLangCode.value = ''
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function deleteGame() {
  try {
    await api.delete(`/games/${gameId}`)
    router.push('/games')
  } catch (e) {
    error.value = (e as Error).message
  }
}

function onFileSelected(event: Event) {
  const target = event.target as HTMLInputElement
  uploadFile.value = target.files?.[0] || null
}

async function addLinkMedia() {
  mediaError.value = ''
  try {
    await api.post(`/games/${gameId}/languages/${activeLangCode.value}/media`, {
      type: linkType.value,
      url: linkUrl.value,
      title: linkTitle.value,
    })
    linkUrl.value = ''
    linkTitle.value = ''
    await load()
  } catch (e) {
    mediaError.value = (e as Error).message
  }
}

async function uploadManual() {
  mediaError.value = ''
  if (!uploadFile.value) {
    mediaError.value = 'Seleziona un file PDF'
    return
  }
  const formData = new FormData()
  formData.append('file', uploadFile.value)
  try {
    await api.post(`/games/${gameId}/languages/${activeLangCode.value}/media`, formData)
    uploadFile.value = null
    await load()
  } catch (e) {
    mediaError.value = (e as Error).message
  }
}

async function removeMedia(mediaId: number) {
  mediaError.value = ''
  try {
    await api.delete(`/games/${gameId}/languages/${activeLangCode.value}/media/${mediaId}`)
    await load()
  } catch (e) {
    mediaError.value = (e as Error).message
  }
}

onMounted(async () => {
  try {
    await load()
    if (game.value && game.value.languages.length > 0) {
      selectLanguage(game.value.languages[0].code)
    }
  } catch (e) {
    error.value = (e as Error).message
  }
})
</script>

<template>
  <div v-if="game">
    <h1>{{ game.name }}</h1>
    <img v-if="game.coverPath" :src="`/api/uploads/${game.coverPath}`" :alt="game.name" class="cover" />
    <p v-if="game.owner">Proprietario: {{ game.owner }}</p>
    <button type="button" @click="deleteGame">Elimina gioco</button>

    <nav class="language-tabs">
      <button
        v-for="l in game.languages"
        :key="l.code"
        type="button"
        :class="{ active: l.code === activeLangCode }"
        @click="selectLanguage(l.code)"
      >
        {{ l.code }}
      </button>
    </nav>

    <form @submit.prevent="saveLanguage">
      <label>
        Nome
        <input v-model="editName" required />
      </label>
      <label>
        Descrizione
        <textarea v-model="editDescription"></textarea>
      </label>
      <button type="submit">Salva</button>
      <p v-if="saveMessage" class="success">{{ saveMessage }}</p>
    </form>

    <form @submit.prevent="addLanguage">
      <label>
        Aggiungi lingua (es. en)
        <input v-model="newLangCode" required />
      </label>
      <button type="submit">Aggiungi</button>
    </form>

    <h2>Manuale e tutorial ({{ activeLangCode }})</h2>
    <ul>
      <li v-for="m in activeLanguage()?.media || []" :key="m.id">
        <a v-if="m.type === 'file'" :href="`/api/uploads/${m.url}`" target="_blank">{{ m.title || 'Manuale' }}</a>
        <a v-else :href="m.url" target="_blank">{{ m.title || m.url }}</a>
        ({{ m.type }})
        <button type="button" @click="removeMedia(m.id)">Rimuovi</button>
      </li>
    </ul>

    <form @submit.prevent="uploadManual">
      <label>
        Carica manuale (solo PDF, max 20MB)
        <input type="file" accept="application/pdf" @change="onFileSelected" />
      </label>
      <button type="submit">Carica</button>
    </form>

    <form @submit.prevent="addLinkMedia">
      <label>
        Tipo
        <select v-model="linkType">
          <option value="link">Link</option>
          <option value="youtube">YouTube</option>
        </select>
      </label>
      <label>
        URL
        <input v-model="linkUrl" required />
      </label>
      <label>
        Titolo
        <input v-model="linkTitle" />
      </label>
      <button type="submit">Aggiungi</button>
    </form>
    <p v-if="mediaError" class="error">{{ mediaError }}</p>

    <p v-if="error" class="error">{{ error }}</p>
  </div>
</template>
```

- [ ] **Step 2: Verificare che il progetto compili**

Run: `cd frontend && npm run build`
Expected: build completata senza errori TypeScript.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/views/GameDetailView.vue
git commit -m "feat: add media management (upload/link/youtube) to game detail view"
```

---

### Task 14: Frontend — campo token BGG nelle impostazioni

**Files:**
- Modify: `frontend/src/views/SettingsView.vue`

- [ ] **Step 1: Aggiungere il campo bggApiToken**

Sostituire l'intero contenuto di `frontend/src/views/SettingsView.vue` con:
```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'

interface SettingsResponse {
  defaultLanguage: string
  youtubeApiKeySet: boolean
  youtubeApiKeyMasked?: string
  searchApiKeySet: boolean
  searchApiKeyMasked?: string
  searchApiProvider: string
  bggApiTokenSet: boolean
  bggApiTokenMasked?: string
}

const defaultLanguage = ref('it')
const youtubeApiKey = ref('')
const searchApiKey = ref('')
const searchApiProvider = ref('google')
const bggApiToken = ref('')
const youtubeApiKeyMasked = ref('')
const searchApiKeyMasked = ref('')
const bggApiTokenMasked = ref('')
const message = ref('')
const error = ref('')

async function load() {
  const s = await api.get<SettingsResponse>('/settings')
  defaultLanguage.value = s.defaultLanguage
  searchApiProvider.value = s.searchApiProvider || 'google'
  youtubeApiKeyMasked.value = s.youtubeApiKeyMasked || ''
  searchApiKeyMasked.value = s.searchApiKeyMasked || ''
  bggApiTokenMasked.value = s.bggApiTokenMasked || ''
}

async function save() {
  error.value = ''
  message.value = ''
  try {
    await api.put('/settings', {
      defaultLanguage: defaultLanguage.value,
      youtubeApiKey: youtubeApiKey.value,
      searchApiKey: searchApiKey.value,
      searchApiProvider: searchApiProvider.value,
      bggApiToken: bggApiToken.value,
    })
    youtubeApiKey.value = ''
    searchApiKey.value = ''
    bggApiToken.value = ''
    message.value = 'Impostazioni salvate'
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(async () => {
  try {
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
})
</script>

<template>
  <div>
    <h1>Impostazioni</h1>
    <form @submit.prevent="save">
      <label>
        Lingua di default
        <select v-model="defaultLanguage">
          <option value="it">Italiano</option>
          <option value="en">Inglese</option>
        </select>
      </label>

      <label>
        BoardGameGeek API token
        <input v-model="bggApiToken" type="password" :placeholder="bggApiTokenMasked || 'non configurato'" />
      </label>

      <label>
        YouTube Data API key
        <input v-model="youtubeApiKey" type="password" :placeholder="youtubeApiKeyMasked || 'non configurata'" />
      </label>

      <label>
        Provider ricerca web
        <select v-model="searchApiProvider">
          <option value="google">Google Custom Search</option>
          <option value="bing">Bing Search</option>
        </select>
      </label>

      <label>
        Search API key
        <input v-model="searchApiKey" type="password" :placeholder="searchApiKeyMasked || 'non configurata'" />
      </label>

      <button type="submit">Salva</button>
      <p v-if="message" class="success">{{ message }}</p>
      <p v-if="error" class="error">{{ error }}</p>
    </form>
  </div>
</template>
```

- [ ] **Step 2: Verificare che il progetto compili**

Run: `cd frontend && npm run build`
Expected: build completata senza errori TypeScript.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/views/SettingsView.vue
git commit -m "feat: add BGG API token field to settings view"
```

---

### Task 15: Verifica con token BGG reale (richiede l'utente)

**Files:** nessuno (solo verifica, eventuale fix del formato header se necessario)

- [ ] **Step 1: Ottenere un token BGG reale**

Questo task richiede un input dell'utente umano: chiedere all'utente di registrarsi sul sito di BoardGameGeek per ottenere un token dell'XML API (la procedura esatta non era verificabile in fase di ricerca — indirizzare l'utente a `https://boardgamegeek.com/using_the_xml_api` per le istruzioni ufficiali più aggiornate). **Non chiedere all'utente di incollare il token in chat**: l'utente deve avviare l'app (backend + frontend, o l'immagine Docker) nel proprio browser, andare su Impostazioni, incollare il token lì e salvare — così il token non transita mai in chiaro nella conversazione.

- [ ] **Step 2: Verificare la ricerca reale**

Con il token configurato, usare la pagina "Aggiungi gioco" → ricerca per cercare un gioco reale (es. "Catan"). Se la richiesta fallisce con 401/403, il formato dell'header `Authorization` ipotizzato in `backend/internal/bgg/client.go` (`"Bearer " + token`) è sbagliato — provare alternative comuni (solo il token senza prefisso, un header diverso come `X-API-Key`) modificando `doRequest` in `backend/internal/bgg/client.go`, ricompilando e ritestando, finché la ricerca non funziona.

- [ ] **Step 3: Verificare l'import**

Selezionare un risultato dalla ricerca e importarlo. Confermare che il gioco viene creato con nome, anno, min/max giocatori, durata e copertina corretti (la copertina deve essere visibile nell'elenco giochi).

- [ ] **Step 4: Se è stato necessario un fix, aggiornare i test e fare commit**

Se il formato dell'header è stato cambiato, aggiornare l'assert nel test `TestSearch_ParsesResultsAndSendsAuthHeader` (Task 5) di conseguenza, rieseguire `go test ./internal/bgg/... -v`, e fare un commit separato che documenti la correzione (es. `fix: correct BGG Authorization header format based on real API testing`).

---

### Task 16: Verifica end-to-end finale

**Files:** nessuno (solo verifica)

- [ ] **Step 1: Ricostruire ed avviare l'applicazione**

```bash
docker compose build && docker compose up -d
curl -s http://localhost:8080/api/health
```
Expected: `{"status":"ok"}`

- [ ] **Step 2: Verificare il flusso completo in browser**

1. Login come admin (creato nella Fase 1).
2. Vai su "Giochi" → elenco vuoto.
3. "Aggiungi gioco" → crea un gioco manualmente (senza token BGG, per non dipendere da una chiave esterna in questa verifica) → conferma redirect al dettaglio.
4. Nel dettaglio, modifica nome/descrizione della lingua base, salva, conferma il messaggio di successo.
5. Aggiungi una seconda lingua (es. "en"), conferma che appaia precompilata col testo della lingua base.
6. Aggiungi un media di tipo link e uno di tipo youtube, verifica che compaiano nell'elenco.
7. Carica un PDF come manuale, verifica che il link al file funzioni (apre/scarica il PDF).
8. Rimuovi un media, conferma che sparisca dall'elenco.
9. Torna all'elenco giochi, conferma che il gioco creato appaia.
10. Elimina il gioco, conferma che sparisca dall'elenco.
11. Controlla la console del browser: nessun errore oltre agli avvisi cosmetici già noti (autocomplete/id).

- [ ] **Step 3: Arrestare l'ambiente**

```bash
docker compose down
```

- [ ] **Step 4: Annotare l'esito**

Se tutti i passaggi sono verificati con successo (compreso il Task 15 con un token BGG reale, se disponibile), la Fase 2 è completa.
