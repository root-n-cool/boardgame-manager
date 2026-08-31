# Fase 1 — Scaffolding + Autenticazione/Amministrazione Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Far partire un'applicazione BoardGames Manager funzionante end-to-end per la sola parte di autenticazione/amministrazione: bootstrap del primo utente stile n8n, login/logout, gestione utenti admin, pagina impostazioni, il tutto in un singolo binario Go che serve anche il frontend Vue, eseguibile via Docker Compose.

**Architecture:** Backend Go monolitico (router `chi`, SQLite via `modernc.org/sqlite`, migrazioni custom via `embed.FS`), sessioni via cookie httpOnly. Frontend Vue 3 + Vite + Vue Router + Pinia, buildato come asset statico ed embeddato nel binario Go. Un solo servizio Docker.

**Tech Stack:** Go (stdlib `net/http` + `github.com/go-chi/chi/v5`), `modernc.org/sqlite`, `golang.org/x/crypto/bcrypt`, Vue 3 + TypeScript + Vite + `vue-router` + `pinia`, Docker/docker-compose.

**Spec:** `docs/superpowers/specs/2026-08-31-boardgames-manager-design.md`

## Global Constraints

- Backend Go monolite, nessun framework web pesante — router `chi` o stdlib.
- Storage: SQLite (`modernc.org/sqlite`, driver cgo-free) — nessun altro database.
- Auth: sessioni via cookie httpOnly (`session_token`), niente JWT.
- Nessun invio email in questa fase (niente SMTP).
- Un solo ruolo utente: admin/organizzatore, nessuna gerarchia di permessi.
- Frontend Vue 3 + Vite, build statico embeddato nel binario Go (`embed.FS`), un solo container serve API + UI.
- Deployment: Docker/docker-compose, un solo servizio, volume `/data` per SQLite.
- Chiavi API esterne (usate solo dalla Fase 2 in poi) vanno salvate ma mai restituite in chiaro dalle API — solo mascherate.

---

### Task 1: Scaffold backend Go + health endpoint

**Files:**
- Create: `backend/go.mod`
- Create: `backend/cmd/server/main.go`
- Create: `backend/internal/httpapi/router.go`
- Create: `backend/internal/httpapi/json.go`
- Create: `backend/internal/httpapi/router_test.go`
- Create: `Makefile`
- Create: `.gitignore`

**Interfaces:**
- Produces: `httpapi.NewRouter() http.Handler`, `httpapi.writeJSON(w, status, v)`, `httpapi.writeError(w, status, message)` (usate da tutti i task successivi in `internal/httpapi`).

- [ ] **Step 1: Inizializzare il modulo Go**

```bash
mkdir -p backend/cmd/server backend/internal/httpapi
cd backend && go mod init boardgames-manager
```

- [ ] **Step 2: Aggiungere la dipendenza chi**

```bash
cd backend && go get github.com/go-chi/chi/v5@latest
```

- [ ] **Step 3: Scrivere il test per l'health endpoint**

`backend/internal/httpapi/router_test.go`:
```go
package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func TestHealthEndpoint_ReturnsOK(t *testing.T) {
	router := httpapi.NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
```

- [ ] **Step 4: Eseguire il test e verificare che fallisca**

Run: `cd backend && go test ./... -run TestHealthEndpoint_ReturnsOK -v`
Expected: FAIL (compile error, `httpapi.NewRouter` non esiste ancora)

- [ ] **Step 5: Implementare il router e l'health handler**

`backend/internal/httpapi/json.go`:
```go
package httpapi

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
```

`backend/internal/httpapi/router.go`:
```go
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)

	r.Get("/api/health", healthHandler)

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

`backend/cmd/server/main.go`:
```go
package main

import (
	"log"
	"net/http"
	"os"

	"boardgames-manager/internal/httpapi"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	router := httpapi.NewRouter()

	log.Printf("listening on :%s", port)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
```

- [ ] **Step 6: Eseguire il test e verificare che passi**

Run: `cd backend && go test ./... -v`
Expected: PASS

- [ ] **Step 7: Creare Makefile e .gitignore alla radice del repo**

`Makefile`:
```makefile
.PHONY: backend-test backend-build frontend-build build run docker-build

backend-test:
	cd backend && go test ./...

backend-build:
	cd backend && CGO_ENABLED=0 go build -o ../bin/server ./cmd/server

frontend-build:
	cd frontend && npm install && npm run build

build: frontend-build backend-build

run:
	cd backend && go run ./cmd/server

docker-build:
	docker compose build
```

`.gitignore`:
```
/backend/data/
/data/
*.db
/frontend/node_modules/
/frontend/dist/
/backend/internal/webui/dist/*
!/backend/internal/webui/dist/.gitkeep
/bin/
```

- [ ] **Step 8: Creare il placeholder per l'embed del frontend**

```bash
mkdir -p backend/internal/webui/dist
touch backend/internal/webui/dist/.gitkeep
```

- [ ] **Step 9: Commit**

```bash
git add backend/go.mod backend/go.sum backend/cmd backend/internal/httpapi backend/internal/webui Makefile .gitignore
git commit -m "feat: scaffold Go backend with health endpoint"
```

---

### Task 2: Connessione SQLite + migrazioni

**Files:**
- Create: `backend/internal/db/db.go`
- Create: `backend/internal/db/migrate.go`
- Create: `backend/internal/db/migrate_test.go`
- Create: `backend/internal/db/migrations/0001_init.sql`

**Interfaces:**
- Produces: `db.Open(path string) (*sql.DB, error)`, `db.Migrate(ctx context.Context, conn *sql.DB) error`.

- [ ] **Step 1: Aggiungere la dipendenza sqlite**

```bash
cd backend && go get modernc.org/sqlite@latest
```

- [ ] **Step 2: Scrivere il test di migrazione (fallirà per mancanza del pacchetto)**

`backend/internal/db/migrate_test.go`:
```go
package db_test

import (
	"context"
	"testing"

	"boardgames-manager/internal/db"
)

func TestMigrate_CreatesExpectedTables(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	for _, table := range []string{"users", "sessions", "app_settings"} {
		var name string
		err := conn.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("expected table %q to exist: %v", table, err)
		}
	}
}

func TestMigrate_IsIdempotent(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("second migrate should be a no-op, got error: %v", err)
	}
}
```

- [ ] **Step 3: Eseguire il test e verificare che fallisca**

Run: `cd backend && go test ./internal/db/... -v`
Expected: FAIL (pacchetto `db` non esiste)

- [ ] **Step 4: Implementare la connessione al database**

`backend/internal/db/db.go`:
```go
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return conn, nil
}
```

- [ ] **Step 5: Creare la prima migrazione**

`backend/internal/db/migrations/0001_init.sql`:
```sql
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE app_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    default_language TEXT NOT NULL DEFAULT 'it',
    youtube_api_key TEXT,
    search_api_key TEXT,
    search_api_provider TEXT
);

INSERT INTO app_settings (id, default_language) VALUES (1, 'it');
```

- [ ] **Step 6: Implementare il runner delle migrazioni**

`backend/internal/db/migrate.go`:
```go
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Migrate(ctx context.Context, conn *sql.DB) error {
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var count int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, name).Scan(&count); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if count > 0 {
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}

	return nil
}
```

- [ ] **Step 7: Eseguire il test e verificare che passi**

Run: `cd backend && go test ./internal/db/... -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add backend/internal/db backend/go.mod backend/go.sum
git commit -m "feat: add sqlite connection and migration runner"
```

---

### Task 3: Hashing password

**Files:**
- Create: `backend/internal/auth/password.go`
- Create: `backend/internal/auth/password_test.go`

**Interfaces:**
- Produces: `auth.HashPassword(plain string) (string, error)`, `auth.VerifyPassword(hash, plain string) bool`.

- [ ] **Step 1: Aggiungere la dipendenza x/crypto**

```bash
cd backend && go get golang.org/x/crypto@latest
```

- [ ] **Step 2: Scrivere il test**

`backend/internal/auth/password_test.go`:
```go
package auth_test

import (
	"testing"

	"boardgames-manager/internal/auth"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "correct-horse-battery" {
		t.Fatal("expected hash to differ from plaintext")
	}
	if !auth.VerifyPassword(hash, "correct-horse-battery") {
		t.Fatal("expected correct password to verify")
	}
	if auth.VerifyPassword(hash, "wrong-password") {
		t.Fatal("expected wrong password to fail verification")
	}
}
```

- [ ] **Step 3: Eseguire il test e verificare che fallisca**

Run: `cd backend && go test ./internal/auth/... -v`
Expected: FAIL (pacchetto `auth` non esiste)

- [ ] **Step 4: Implementare**

`backend/internal/auth/password.go`:
```go
package auth

import "golang.org/x/crypto/bcrypt"

func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
```

- [ ] **Step 5: Eseguire il test e verificare che passi**

Run: `cd backend && go test ./internal/auth/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/auth backend/go.mod backend/go.sum
git commit -m "feat: add password hashing"
```

---

### Task 4: Token di sessione

**Files:**
- Create: `backend/internal/auth/token.go`
- Create: `backend/internal/auth/token_test.go`

**Interfaces:**
- Produces: `auth.GenerateToken() (string, error)`, `auth.HashToken(token string) string`.

- [ ] **Step 1: Scrivere il test**

`backend/internal/auth/token_test.go`:
```go
package auth_test

import (
	"testing"

	"boardgames-manager/internal/auth"
)

func TestGenerateToken_ReturnsUniqueValues(t *testing.T) {
	a, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	b, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if a == b {
		t.Fatal("expected two generated tokens to differ")
	}
}

func TestHashToken_IsDeterministicAndDiffersFromInput(t *testing.T) {
	token := "sample-token"
	h1 := auth.HashToken(token)
	h2 := auth.HashToken(token)
	if h1 != h2 {
		t.Fatal("expected hashing the same token twice to produce the same result")
	}
	if h1 == token {
		t.Fatal("expected hash to differ from the raw token")
	}
}
```

- [ ] **Step 2: Eseguire il test e verificare che fallisca**

Run: `cd backend && go test ./internal/auth/... -run TestGenerateToken -v`
Expected: FAIL (`GenerateToken` non esiste)

- [ ] **Step 3: Implementare**

`backend/internal/auth/token.go`:
```go
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

func GenerateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 4: Eseguire il test e verificare che passi**

Run: `cd backend && go test ./internal/auth/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/auth
git commit -m "feat: add session token generation and hashing"
```

---

### Task 5: Repository utenti

**Files:**
- Create: `backend/internal/users/store.go`
- Create: `backend/internal/users/store_test.go`

**Interfaces:**
- Consumes: `db.Open`, `db.Migrate` (Task 2).
- Produces: `users.User{ID int64; Email, PasswordHash string; CreatedAt time.Time}`, `users.Store`, `users.NewStore(conn *sql.DB) *Store`, `(*Store).Count(ctx) (int, error)`, `(*Store).Create(ctx, email, passwordHash string) (User, error)`, `(*Store).GetByEmail(ctx, email string) (User, error)`, `(*Store).GetByID(ctx, id int64) (User, error)`, `(*Store).List(ctx) ([]User, error)`, `(*Store).Delete(ctx, id int64) error`, `users.ErrNotFound`, `users.ErrDuplicateEmail`.

- [ ] **Step 1: Scrivere i test**

`backend/internal/users/store_test.go`:
```go
package users_test

import (
	"context"
	"errors"
	"testing"

	"boardgames-manager/internal/db"
	"boardgames-manager/internal/users"
)

func newTestStore(t *testing.T) *users.Store {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return users.NewStore(conn)
}

func TestCreateAndGetByEmail(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	created, err := store.Create(ctx, "admin@example.com", "hashed-value")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected a non-zero id")
	}

	found, err := store.GetByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("get by email: %v", err)
	}
	if found.ID != created.ID {
		t.Fatalf("expected same id, got %d vs %d", found.ID, created.ID)
	}
}

func TestCreate_DuplicateEmailReturnsError(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Create(ctx, "dup@example.com", "hash1"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := store.Create(ctx, "dup@example.com", "hash2")
	if !errors.Is(err, users.ErrDuplicateEmail) {
		t.Fatalf("expected ErrDuplicateEmail, got %v", err)
	}
}

func TestCount_ReflectsNumberOfUsers(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	count, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 users initially, got %d", count)
	}

	if _, err := store.Create(ctx, "a@example.com", "hash"); err != nil {
		t.Fatalf("create: %v", err)
	}
	count, err = store.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 user, got %d", count)
	}
}

func TestListAndDelete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	a, _ := store.Create(ctx, "a@example.com", "hash")
	_, _ = store.Create(ctx, "b@example.com", "hash")

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 users, got %d", len(list))
	}

	if err := store.Delete(ctx, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	list, err = store.List(ctx)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 user after delete, got %d", len(list))
	}
}

func TestGetByEmail_NotFound(t *testing.T) {
	store := newTestStore(t)
	_, err := store.GetByEmail(context.Background(), "missing@example.com")
	if !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

Run: `cd backend && go test ./internal/users/... -v`
Expected: FAIL (pacchetto `users` non esiste)

- [ ] **Step 3: Implementare il repository**

`backend/internal/users/store.go`:
```go
package users

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type User struct {
	ID           int64
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

var ErrNotFound = errors.New("user not found")
var ErrDuplicateEmail = errors.New("email already in use")

type Store struct {
	db *sql.DB
}

func NewStore(conn *sql.DB) *Store {
	return &Store{db: conn}
}

func (s *Store) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) Create(ctx context.Context, email, passwordHash string) (User, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO users (email, password_hash) VALUES (?, ?)`, email, passwordHash)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return User{}, ErrDuplicateEmail
		}
		return User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *Store) GetByEmail(ctx context.Context, email string) (User, error) {
	return s.scanOne(ctx, `SELECT id, email, password_hash, created_at FROM users WHERE email = ?`, email)
}

func (s *Store) GetByID(ctx context.Context, id int64) (User, error) {
	return s.scanOne(ctx, `SELECT id, email, password_hash, created_at FROM users WHERE id = ?`, id)
}

func (s *Store) scanOne(ctx context.Context, query string, arg any) (User, error) {
	var u User
	var createdAt string
	err := s.db.QueryRowContext(ctx, query, arg).Scan(&u.ID, &u.Email, &u.PasswordHash, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return u, nil
}

func (s *Store) List(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, email, password_hash, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []User
	for rows.Next() {
		var u User
		var createdAt string
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &createdAt); err != nil {
			return nil, err
		}
		u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
```

- [ ] **Step 4: Eseguire i test e verificare che passino**

Run: `cd backend && go test ./internal/users/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/users
git commit -m "feat: add users repository"
```

---

### Task 6: Repository sessioni

**Files:**
- Create: `backend/internal/auth/session_store.go`
- Create: `backend/internal/auth/session_store_test.go`

**Interfaces:**
- Consumes: `db.Open`, `db.Migrate` (Task 2).
- Produces: `auth.Session{ID, UserID int64; TokenHash string; ExpiresAt time.Time}`, `auth.SessionStore`, `auth.NewSessionStore(conn *sql.DB) *SessionStore`, `(*SessionStore).Create(ctx, userID int64, tokenHash string, expiresAt time.Time) error`, `(*SessionStore).GetValidByTokenHash(ctx, tokenHash string) (Session, error)`, `(*SessionStore).Delete(ctx, tokenHash string) error`, `auth.ErrSessionNotFound`.

- [ ] **Step 1: Scrivere i test**

`backend/internal/auth/session_store_test.go`:
```go
package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/db"
	"boardgames-manager/internal/users"
)

func newSessionTestSetup(t *testing.T) (*auth.SessionStore, int64) {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	u, err := users.NewStore(conn).Create(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return auth.NewSessionStore(conn), u.ID
}

func TestSessionCreateAndGetValid(t *testing.T) {
	store, userID := newSessionTestSetup(t)
	ctx := context.Background()

	if err := store.Create(ctx, userID, "hashed-token", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}

	sess, err := store.GetValidByTokenHash(ctx, "hashed-token")
	if err != nil {
		t.Fatalf("get valid: %v", err)
	}
	if sess.UserID != userID {
		t.Fatalf("expected user id %d, got %d", userID, sess.UserID)
	}
}

func TestSession_ExpiredIsNotReturned(t *testing.T) {
	store, userID := newSessionTestSetup(t)
	ctx := context.Background()

	if err := store.Create(ctx, userID, "expired-token", time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}

	_, err := store.GetValidByTokenHash(ctx, "expired-token")
	if !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound for expired session, got %v", err)
	}
}

func TestSession_DeleteRemovesIt(t *testing.T) {
	store, userID := newSessionTestSetup(t)
	ctx := context.Background()

	if err := store.Create(ctx, userID, "some-token", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.Delete(ctx, "some-token"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := store.GetValidByTokenHash(ctx, "some-token")
	if !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound after delete, got %v", err)
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

Run: `cd backend && go test ./internal/auth/... -run TestSession -v`
Expected: FAIL (`SessionStore` non esiste)

- [ ] **Step 3: Implementare**

`backend/internal/auth/session_store.go`:
```go
package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Session struct {
	ID        int64
	UserID    int64
	TokenHash string
	ExpiresAt time.Time
}

var ErrSessionNotFound = errors.New("session not found")

type SessionStore struct {
	db *sql.DB
}

func NewSessionStore(conn *sql.DB) *SessionStore {
	return &SessionStore{db: conn}
}

func (s *SessionStore) Create(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (user_id, token_hash, expires_at) VALUES (?, ?, ?)`,
		userID, tokenHash, expiresAt.UTC().Format(time.RFC3339))
	return err
}

func (s *SessionStore) GetValidByTokenHash(ctx context.Context, tokenHash string) (Session, error) {
	var sess Session
	var expiresAtStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, expires_at FROM sessions WHERE token_hash = ?`,
		tokenHash).Scan(&sess.ID, &sess.UserID, &sess.TokenHash, &expiresAtStr)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, err
	}
	sess.ExpiresAt, err = time.Parse(time.RFC3339, expiresAtStr)
	if err != nil {
		return Session{}, err
	}
	if sess.ExpiresAt.Before(time.Now()) {
		return Session{}, ErrSessionNotFound
	}
	return sess, nil
}

func (s *SessionStore) Delete(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}
```

- [ ] **Step 4: Eseguire i test e verificare che passino**

Run: `cd backend && go test ./internal/auth/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/auth
git commit -m "feat: add session repository"
```

---

### Task 7: Bootstrap primo utente

**Files:**
- Modify: `backend/internal/httpapi/router.go`
- Modify: `backend/cmd/server/main.go`
- Create: `backend/internal/httpapi/bootstrap.go`
- Create: `backend/internal/httpapi/testhelpers_test.go`
- Create: `backend/internal/httpapi/bootstrap_test.go`

**Interfaces:**
- Consumes: `users.Store`, `auth.SessionStore`, `auth.HashPassword`, `auth.GenerateToken`, `auth.HashToken` (Task 3, 4, 5, 6).
- Produces: `httpapi.Server{Users *users.Store; Sessions *auth.SessionStore}`, `httpapi.NewRouter(s *Server) http.Handler`, `credentialsRequest{Email, Password string}` (riusato dai task successivi), `(*Server).startSession(w, r, userID int64) error` (riusato dai task successivi), `newTestServer(t) *httpapi.Server` in `testhelpers_test.go` (riusato dai task successivi).

- [ ] **Step 1: Scrivere l'helper di test condiviso**

`backend/internal/httpapi/testhelpers_test.go`:
```go
package httpapi_test

import (
	"context"
	"testing"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/db"
	"boardgames-manager/internal/httpapi"
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
	}
}
```

- [ ] **Step 2: Scrivere i test del bootstrap**

`backend/internal/httpapi/bootstrap_test.go`:
```go
package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func TestBootstrapStatus_NeedsSetupWhenNoUsers(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	req := httptest.NewRequest(http.MethodGet, "/api/bootstrap/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body struct {
		NeedsSetup bool `json:"needsSetup"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.NeedsSetup {
		t.Fatal("expected needsSetup to be true with no users")
	}
}

func TestBootstrap_CreatesFirstAdminAndSetsCookie(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	payload, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "supersecret1"})
	req := httptest.NewRequest(http.MethodPost, "/api/bootstrap", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("expected a session cookie to be set")
	}
}

func TestBootstrap_RejectedWhenUserAlreadyExists(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	payload, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "supersecret1"})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/bootstrap", bytes.NewReader(payload)))

	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/api/bootstrap", bytes.NewReader(payload)))

	if rec2.Code != http.StatusConflict {
		t.Fatalf("expected 409 on second bootstrap attempt, got %d", rec2.Code)
	}
}
```

- [ ] **Step 3: Eseguire i test e verificare che falliscano**

Run: `cd backend && go test ./internal/httpapi/... -v`
Expected: FAIL (`httpapi.Server`, `NewRouter(s)` non esistono ancora con quella firma)

- [ ] **Step 4: Implementare gli handler di bootstrap**

`backend/internal/httpapi/bootstrap.go`:
```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"boardgames-manager/internal/auth"
)

type bootstrapStatusResponse struct {
	NeedsSetup bool `json:"needsSetup"`
}

func (s *Server) bootstrapStatusHandler(w http.ResponseWriter, r *http.Request) {
	count, err := s.Users.Count(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check setup status")
		return
	}
	writeJSON(w, http.StatusOK, bootstrapStatusResponse{NeedsSetup: count == 0})
}

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) bootstrapHandler(w http.ResponseWriter, r *http.Request) {
	count, err := s.Users.Count(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check setup status")
		return
	}
	if count > 0 {
		writeError(w, http.StatusConflict, "setup already completed")
		return
	}

	var req credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create user")
		return
	}

	user, err := s.Users.Create(r.Context(), req.Email, hash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create user")
		return
	}

	if err := s.startSession(w, r, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not start session")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": user.ID, "email": user.Email})
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	token, err := auth.GenerateToken()
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	if err := s.Sessions.Create(r.Context(), userID, auth.HashToken(token), expiresAt); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	})
	return nil
}
```

- [ ] **Step 5: Aggiornare il router**

`backend/internal/httpapi/router.go`:
```go
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/users"
)

type Server struct {
	Users    *users.Store
	Sessions *auth.SessionStore
}

func NewRouter(s *Server) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)

	r.Get("/api/health", healthHandler)
	r.Get("/api/bootstrap/status", s.bootstrapStatusHandler)
	r.Post("/api/bootstrap", s.bootstrapHandler)

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

- [ ] **Step 6: Aggiornare main.go per costruire il Server**

`backend/cmd/server/main.go`:
```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/db"
	"boardgames-manager/internal/httpapi"
	"boardgames-manager/internal/users"
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
	}

	router := httpapi.NewRouter(server)

	log.Printf("listening on :%s", port)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
```

- [ ] **Step 7: Eseguire tutti i test e verificare che passino**

Run: `cd backend && go test ./... -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add backend/internal/httpapi backend/cmd/server/main.go
git commit -m "feat: add first-admin bootstrap flow"
```

---

### Task 8: Login, logout, sessione corrente

**Files:**
- Create: `backend/internal/httpapi/auth_handlers.go`
- Create: `backend/internal/httpapi/middleware_auth.go`
- Create: `backend/internal/httpapi/auth_handlers_test.go`
- Modify: `backend/internal/httpapi/router.go`

**Interfaces:**
- Consumes: `Server.Users`, `Server.Sessions`, `credentialsRequest`, `(*Server).startSession` (Task 7).
- Produces: `(*Server).requireAuth(next http.Handler) http.Handler`, `currentUser(r *http.Request) (users.User, bool)` (riusati dai task successivi per proteggere route).

- [ ] **Step 1: Scrivere i test**

`backend/internal/httpapi/auth_handlers_test.go`:
```go
package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func bootstrapFirstAdmin(t *testing.T, router http.Handler, email, password string) *http.Cookie {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"email": email, "password": password})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/bootstrap", bytes.NewReader(payload)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("bootstrap failed: %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session_token" {
			return c
		}
	}
	t.Fatal("no session cookie returned by bootstrap")
	return nil
}

func TestLogin_WithValidCredentialsSetsCookie(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "supersecret1"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payload)))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("expected a session cookie")
	}
}

func TestLogin_WithWrongPasswordFails(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "wrong"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payload)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMe_RequiresAuthentication(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session, got %d", rec.Code)
	}
}

func TestMe_ReturnsCurrentUserWhenAuthenticated(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLogout_InvalidatesSession(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	logoutReq.AddCookie(cookie)
	router.ServeHTTP(httptest.NewRecorder(), logoutReq)

	meReq := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	meReq.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, meReq)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

Run: `cd backend && go test ./internal/httpapi/... -v`
Expected: FAIL (route `/api/login`, `/api/logout`, `/api/me` non esistono)

- [ ] **Step 3: Implementare il middleware di autenticazione**

`backend/internal/httpapi/middleware_auth.go`:
```go
package httpapi

import (
	"context"
	"net/http"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/users"
)

type contextKey string

const userContextKey contextKey = "current_user"

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}

		tokenHash := auth.HashToken(cookie.Value)
		sess, err := s.Sessions.GetValidByTokenHash(r.Context(), tokenHash)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}

		user, err := s.Users.GetByID(r.Context(), sess.UserID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func currentUser(r *http.Request) (users.User, bool) {
	u, ok := r.Context().Value(userContextKey).(users.User)
	return u, ok
}
```

- [ ] **Step 4: Implementare login/logout/me**

`backend/internal/httpapi/auth_handlers.go`:
```go
package httpapi

import (
	"encoding/json"
	"net/http"

	"boardgames-manager/internal/auth"
)

func (s *Server) loginHandler(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, err := s.Users.GetByEmail(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if !auth.VerifyPassword(user.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := s.startSession(w, r, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not start session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"id": user.ID, "email": user.Email})
}

func (s *Server) logoutHandler(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session_token"); err == nil {
		_ = s.Sessions.Delete(r.Context(), auth.HashToken(cookie.Value))
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

func (s *Server) meHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": user.ID, "email": user.Email})
}
```

- [ ] **Step 5: Aggiornare il router**

`backend/internal/httpapi/router.go`:
```go
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/users"
)

type Server struct {
	Users    *users.Store
	Sessions *auth.SessionStore
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
	})

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

- [ ] **Step 6: Eseguire tutti i test e verificare che passino**

Run: `cd backend && go test ./... -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add backend/internal/httpapi
git commit -m "feat: add login, logout and current-user endpoints"
```

---

### Task 9: Gestione utenti admin (CRUD)

**Files:**
- Create: `backend/internal/httpapi/users_handlers.go`
- Create: `backend/internal/httpapi/users_handlers_test.go`
- Modify: `backend/internal/httpapi/router.go`

**Interfaces:**
- Consumes: `Server.Users`, `credentialsRequest`, `s.requireAuth`, `bootstrapFirstAdmin` test helper (Task 7, 8).
- Produces: `GET /api/users`, `POST /api/users`, `DELETE /api/users/{id}` (protette).

- [ ] **Step 1: Scrivere i test**

`backend/internal/httpapi/users_handlers_test.go`:
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

func TestListUsers_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/users", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCreateUser_AsAdminSucceeds(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"email": "second@example.com", "password": "anotherpass1"})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateUser_DuplicateEmailReturnsConflict(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "anotherpass1"})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestDeleteUser_CannotDeleteLastRemainingUser(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	listReq := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	listReq.AddCookie(cookie)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	var list []struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected exactly 1 user, got %d", len(list))
	}

	delReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/users/%d", list[0].ID), nil)
	delReq.AddCookie(cookie)
	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, delReq)

	if delRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 when deleting last user, got %d", delRec.Code)
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

Run: `cd backend && go test ./internal/httpapi/... -run TestListUsers -v`
Expected: FAIL (route `/api/users` non esiste)

- [ ] **Step 3: Implementare gli handler**

`backend/internal/httpapi/users_handlers.go`:
```go
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/users"
)

func (s *Server) listUsersHandler(w http.ResponseWriter, r *http.Request) {
	list, err := s.Users.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list users")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, u := range list {
		out = append(out, map[string]any{"id": u.ID, "email": u.Email, "createdAt": u.CreatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createUserHandler(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create user")
		return
	}

	user, err := s.Users.Create(r.Context(), req.Email, hash)
	if err != nil {
		if errors.Is(err, users.ErrDuplicateEmail) {
			writeError(w, http.StatusConflict, "email already in use")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create user")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": user.ID, "email": user.Email})
}

func (s *Server) deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	count, err := s.Users.Count(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete user")
		return
	}
	if count <= 1 {
		writeError(w, http.StatusConflict, "cannot delete the last remaining user")
		return
	}

	if err := s.Users.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete user")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
```

- [ ] **Step 4: Aggiornare il router**

`backend/internal/httpapi/router.go`:
```go
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/users"
)

type Server struct {
	Users    *users.Store
	Sessions *auth.SessionStore
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
	})

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

- [ ] **Step 5: Eseguire tutti i test e verificare che passino**

Run: `cd backend && go test ./... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi
git commit -m "feat: add admin user management endpoints"
```

---

### Task 10: Impostazioni applicazione

**Files:**
- Create: `backend/internal/settings/store.go`
- Create: `backend/internal/settings/store_test.go`
- Create: `backend/internal/httpapi/settings_handlers.go`
- Create: `backend/internal/httpapi/settings_handlers_test.go`
- Modify: `backend/internal/httpapi/router.go`
- Modify: `backend/internal/httpapi/testhelpers_test.go`
- Modify: `backend/cmd/server/main.go`

**Interfaces:**
- Produces: `settings.Settings{DefaultLanguage, YouTubeAPIKey, SearchAPIKey, SearchAPIProvider string}`, `settings.Store`, `settings.NewStore(conn *sql.DB) *Store`, `(*Store).Get(ctx) (Settings, error)`, `(*Store).Update(ctx, Settings) error`.
- Modifica: `Server` guadagna il campo `Settings *settings.Store`.

- [ ] **Step 1: Scrivere il test del repository impostazioni**

`backend/internal/settings/store_test.go`:
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
	if cfg.YouTubeAPIKey != "" {
		t.Fatalf("expected empty youtube key by default, got %q", cfg.YouTubeAPIKey)
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
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	cfg, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cfg.DefaultLanguage != "en" || cfg.YouTubeAPIKey != "yt-key" || cfg.SearchAPIKey != "search-key" || cfg.SearchAPIProvider != "google" {
		t.Fatalf("unexpected settings after update: %+v", cfg)
	}
}
```

- [ ] **Step 2: Eseguire il test e verificare che fallisca**

Run: `cd backend && go test ./internal/settings/... -v`
Expected: FAIL (pacchetto `settings` non esiste)

- [ ] **Step 3: Implementare il repository**

`backend/internal/settings/store.go`:
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
}

type Store struct {
	db *sql.DB
}

func NewStore(conn *sql.DB) *Store {
	return &Store{db: conn}
}

func (s *Store) Get(ctx context.Context) (Settings, error) {
	var out Settings
	var youtubeKey, searchKey, provider sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT default_language, youtube_api_key, search_api_key, search_api_provider FROM app_settings WHERE id = 1`,
	).Scan(&out.DefaultLanguage, &youtubeKey, &searchKey, &provider)
	if err != nil {
		return Settings{}, err
	}
	out.YouTubeAPIKey = youtubeKey.String
	out.SearchAPIKey = searchKey.String
	out.SearchAPIProvider = provider.String
	return out, nil
}

func (s *Store) Update(ctx context.Context, in Settings) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE app_settings SET default_language = ?, youtube_api_key = ?, search_api_key = ?, search_api_provider = ? WHERE id = 1`,
		in.DefaultLanguage, nullIfEmpty(in.YouTubeAPIKey), nullIfEmpty(in.SearchAPIKey), nullIfEmpty(in.SearchAPIProvider),
	)
	return err
}

func nullIfEmpty(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}
```

- [ ] **Step 4: Eseguire il test e verificare che passi**

Run: `cd backend && go test ./internal/settings/... -v`
Expected: PASS

- [ ] **Step 5: Aggiornare l'helper di test condiviso**

`backend/internal/httpapi/testhelpers_test.go`:
```go
package httpapi_test

import (
	"context"
	"testing"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/db"
	"boardgames-manager/internal/httpapi"
	"boardgames-manager/internal/settings"
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
	}
}
```

- [ ] **Step 6: Scrivere i test HTTP delle impostazioni**

`backend/internal/httpapi/settings_handlers_test.go`:
```go
package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

func TestGetSettings_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestPutSettings_UpdatesLanguageAndMasksKeys(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{
		"defaultLanguage":   "en",
		"youtubeApiKey":     "abcd1234efgh",
		"searchApiKey":      "search-secret-key",
		"searchApiProvider": "google",
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
		DefaultLanguage     string `json:"defaultLanguage"`
		YouTubeAPIKeySet    bool   `json:"youtubeApiKeySet"`
		YouTubeAPIKeyMasked string `json:"youtubeApiKeyMasked"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.DefaultLanguage != "en" {
		t.Fatalf("expected language 'en', got %q", body.DefaultLanguage)
	}
	if !body.YouTubeAPIKeySet {
		t.Fatal("expected youtubeApiKeySet to be true")
	}
	if body.YouTubeAPIKeyMasked == "abcd1234efgh" {
		t.Fatal("expected youtube key to be masked, not returned in clear")
	}
}
```

- [ ] **Step 7: Eseguire i test e verificare che falliscano**

Run: `cd backend && go test ./internal/httpapi/... -run TestGetSettings -v`
Expected: FAIL (route `/api/settings` non esiste)

- [ ] **Step 8: Implementare gli handler**

`backend/internal/httpapi/settings_handlers.go`:
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
```

- [ ] **Step 9: Aggiornare il router**

`backend/internal/httpapi/router.go`:
```go
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/settings"
	"boardgames-manager/internal/users"
)

type Server struct {
	Users    *users.Store
	Sessions *auth.SessionStore
	Settings *settings.Store
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
	})

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

- [ ] **Step 10: Aggiornare main.go**

`backend/cmd/server/main.go`:
```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/db"
	"boardgames-manager/internal/httpapi"
	"boardgames-manager/internal/settings"
	"boardgames-manager/internal/users"
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
	}

	router := httpapi.NewRouter(server)

	log.Printf("listening on :%s", port)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
```

- [ ] **Step 11: Eseguire tutti i test e verificare che passino**

Run: `cd backend && go test ./... -v`
Expected: PASS

- [ ] **Step 12: Commit**

```bash
git add backend/internal/settings backend/internal/httpapi backend/cmd/server/main.go
git commit -m "feat: add application settings endpoints"
```

---

### Task 11: Scaffold frontend Vue

**Files:**
- Create: `frontend/` (progetto Vite generato)
- Create: `frontend/src/api/client.ts`
- Modify: `frontend/vite.config.ts`

**Interfaces:**
- Produces: `api.get<T>(path)`, `api.post<T>(path, body?)`, `api.put<T>(path, body?)`, `api.delete<T>(path)` in `src/api/client.ts` (usati da tutti gli store/view successivi).

- [ ] **Step 1: Generare il progetto Vite**

```bash
npm create vite@latest frontend -- --template vue-ts
cd frontend && npm install
npm install vue-router@latest pinia@latest
```

- [ ] **Step 2: Configurare build output e proxy dev**

`frontend/vite.config.ts`:
```ts
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: '../backend/internal/webui/dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
```

- [ ] **Step 3: Creare il client API**

`frontend/src/api/client.ts`:
```ts
const BASE_URL = '/api'

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    ...options,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(options.headers || {}),
    },
  })

  if (!res.ok) {
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
    request<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PUT', body: JSON.stringify(body) }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
}
```

- [ ] **Step 4: Verificare che il progetto compili**

Run: `cd frontend && npm run build`
Expected: build completata senza errori, cartella `backend/internal/webui/dist` popolata con `index.html` e asset.

- [ ] **Step 5: Commit**

```bash
git add frontend package-lock.json 2>/dev/null
git add frontend/.gitignore frontend/package.json frontend/package-lock.json frontend/vite.config.ts frontend/src frontend/index.html frontend/tsconfig*.json
git commit -m "feat: scaffold Vue frontend project"
```

---

### Task 12: Store di autenticazione + pagina di setup

**Files:**
- Create: `frontend/src/stores/auth.ts`
- Create: `frontend/src/router/index.ts`
- Create: `frontend/src/views/SetupView.vue`
- Modify: `frontend/src/main.ts`
- Modify: `frontend/src/App.vue`

**Interfaces:**
- Consumes: `api` (Task 11).
- Produces: `useAuthStore()` con state `{user, needsSetup, checked}` e azioni `checkStatus()`, `bootstrap(email, password)`, `login(email, password)`, `logout()` (usato da tutte le view successive); router con guardia di navigazione basata su `useAuthStore`.

- [ ] **Step 1: Creare lo store Pinia**

`frontend/src/stores/auth.ts`:
```ts
import { defineStore } from 'pinia'
import { api } from '../api/client'

interface CurrentUser {
  id: number
  email: string
}

export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: null as CurrentUser | null,
    needsSetup: false,
    checked: false,
  }),
  actions: {
    async checkStatus() {
      const status = await api.get<{ needsSetup: boolean }>('/bootstrap/status')
      this.needsSetup = status.needsSetup
      if (!this.needsSetup) {
        try {
          this.user = await api.get<CurrentUser>('/me')
        } catch {
          this.user = null
        }
      }
      this.checked = true
    },
    async bootstrap(email: string, password: string) {
      this.user = await api.post<CurrentUser>('/bootstrap', { email, password })
      this.needsSetup = false
    },
    async login(email: string, password: string) {
      this.user = await api.post<CurrentUser>('/login', { email, password })
    },
    async logout() {
      await api.post('/logout')
      this.user = null
    },
  },
})
```

- [ ] **Step 2: Creare la view di setup**

`frontend/src/views/SetupView.vue`:
```vue
<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const email = ref('')
const password = ref('')
const error = ref('')
const auth = useAuthStore()
const router = useRouter()

async function submit() {
  error.value = ''
  try {
    await auth.bootstrap(email.value, password.value)
    router.push({ name: 'users' })
  } catch (e) {
    error.value = (e as Error).message
  }
}
</script>

<template>
  <div class="auth-page">
    <h1>Crea il primo amministratore</h1>
    <form @submit.prevent="submit">
      <label>
        Email
        <input v-model="email" type="email" required />
      </label>
      <label>
        Password
        <input v-model="password" type="password" required minlength="8" />
      </label>
      <button type="submit">Crea account</button>
      <p v-if="error" class="error">{{ error }}</p>
    </form>
  </div>
</template>
```

- [ ] **Step 3: Creare il router con placeholder per le view non ancora esistenti**

`frontend/src/router/index.ts` (i placeholder `LoginView`, `DashboardLayout`, `UsersView`, `SettingsView` verranno creati nei prossimi task; per ora il router compila solo con `SetupView`, quindi si aggiungono le route incrementalmente):
```ts
import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import SetupView from '../views/SetupView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/setup', name: 'setup', component: SetupView },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (!auth.checked) {
    await auth.checkStatus()
  }

  if (auth.needsSetup && to.name !== 'setup') {
    return { name: 'setup' }
  }
  return true
})

export default router
```

- [ ] **Step 4: Collegare router e pinia in main.ts**

`frontend/src/main.ts`:
```ts
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'

createApp(App).use(createPinia()).use(router).mount('#app')
```

`frontend/src/App.vue`:
```vue
<template>
  <router-view />
</template>
```

- [ ] **Step 5: Verificare manualmente in dev**

Run: `cd backend && go run ./cmd/server` (in un terminale) e `cd frontend && npm run dev` (in un altro), poi apri `http://localhost:5173/` nel browser: deve reindirizzare a `/setup` e mostrare il form; compilando email+password e inviando, la richiesta a `/api/bootstrap` deve avere successo (verificabile con gli strumenti browser).
Expected: form di setup visibile, submit funzionante (redirect a `/users` fallirà finché non esiste ancora quella route — è atteso, verrà risolto nel Task 14).

- [ ] **Step 6: Commit**

```bash
git add frontend/src/stores frontend/src/router frontend/src/views/SetupView.vue frontend/src/main.ts frontend/src/App.vue
git commit -m "feat: add auth store, router guard and setup view"
```

---

### Task 13: Pagina di login

**Files:**
- Create: `frontend/src/views/LoginView.vue`
- Modify: `frontend/src/router/index.ts`

**Interfaces:**
- Consumes: `useAuthStore` (Task 12).

- [ ] **Step 1: Creare la view di login**

`frontend/src/views/LoginView.vue`:
```vue
<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const email = ref('')
const password = ref('')
const error = ref('')
const auth = useAuthStore()
const router = useRouter()

async function submit() {
  error.value = ''
  try {
    await auth.login(email.value, password.value)
    router.push({ name: 'users' })
  } catch (e) {
    error.value = (e as Error).message
  }
}
</script>

<template>
  <div class="auth-page">
    <h1>Accedi</h1>
    <form @submit.prevent="submit">
      <label>
        Email
        <input v-model="email" type="email" required />
      </label>
      <label>
        Password
        <input v-model="password" type="password" required />
      </label>
      <button type="submit">Accedi</button>
      <p v-if="error" class="error">{{ error }}</p>
    </form>
  </div>
</template>
```

- [ ] **Step 2: Aggiungere la route e la logica di redirect nel router**

`frontend/src/router/index.ts`:
```ts
import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import SetupView from '../views/SetupView.vue'
import LoginView from '../views/LoginView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/setup', name: 'setup', component: SetupView },
    { path: '/login', name: 'login', component: LoginView },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (!auth.checked) {
    await auth.checkStatus()
  }

  if (auth.needsSetup && to.name !== 'setup') {
    return { name: 'setup' }
  }
  if (!auth.needsSetup && to.name === 'setup') {
    return { name: 'login' }
  }
  if (!auth.needsSetup && !auth.user && to.name !== 'login') {
    return { name: 'login' }
  }
  if (auth.user && to.name === 'login') {
    return { name: 'login' }
  }
  return true
})

export default router
```

Nota: l'ultima regola (`auth.user && to.name === 'login'`) verrà corretta nel Task 14 per reindirizzare a `users` non appena quella route esisterà; per ora, senza altre route disponibili, resta su `login` per evitare un router error su una route inesistente.

- [ ] **Step 3: Verificare manualmente in dev**

Run: (server e dev server già avviati) apri `http://localhost:5173/login` dopo aver completato il setup: deve mostrare il form di login e autenticarsi correttamente con le credenziali create nel Task 12.
Expected: login riuscito, nessun errore in console.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/LoginView.vue frontend/src/router/index.ts
git commit -m "feat: add login view"
```

---

### Task 14: Layout dashboard + vista utenti

**Files:**
- Create: `frontend/src/views/DashboardLayout.vue`
- Create: `frontend/src/views/UsersView.vue`
- Modify: `frontend/src/router/index.ts`

**Interfaces:**
- Consumes: `useAuthStore`, `api` (Task 11, 12).

- [ ] **Step 1: Creare il layout della dashboard**

`frontend/src/views/DashboardLayout.vue`:
```vue
<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const router = useRouter()

async function logout() {
  await auth.logout()
  router.push({ name: 'login' })
}
</script>

<template>
  <div class="layout">
    <nav>
      <router-link :to="{ name: 'users' }">Utenti</router-link>
      <router-link :to="{ name: 'settings' }">Impostazioni</router-link>
      <button @click="logout">Esci</button>
    </nav>
    <main>
      <router-view />
    </main>
  </div>
</template>
```

- [ ] **Step 2: Creare la vista utenti**

`frontend/src/views/UsersView.vue`:
```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'

interface AdminUser {
  id: number
  email: string
  createdAt: string
}

const users = ref<AdminUser[]>([])
const newEmail = ref('')
const newPassword = ref('')
const error = ref('')

async function loadUsers() {
  users.value = await api.get<AdminUser[]>('/users')
}

async function addUser() {
  error.value = ''
  try {
    await api.post('/users', { email: newEmail.value, password: newPassword.value })
    newEmail.value = ''
    newPassword.value = ''
    await loadUsers()
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function removeUser(id: number) {
  error.value = ''
  try {
    await api.delete(`/users/${id}`)
    await loadUsers()
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(loadUsers)
</script>

<template>
  <div>
    <h1>Utenti</h1>
    <ul>
      <li v-for="u in users" :key="u.id">
        {{ u.email }}
        <button @click="removeUser(u.id)">Rimuovi</button>
      </li>
    </ul>

    <h2>Aggiungi amministratore</h2>
    <form @submit.prevent="addUser">
      <label>
        Email
        <input v-model="newEmail" type="email" required />
      </label>
      <label>
        Password
        <input v-model="newPassword" type="password" required minlength="8" />
      </label>
      <button type="submit">Aggiungi</button>
      <p v-if="error" class="error">{{ error }}</p>
    </form>
  </div>
</template>
```

- [ ] **Step 3: Aggiornare il router con la route utenti (Impostazioni resta per il Task 15)**

`frontend/src/router/index.ts`:
```ts
import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import SetupView from '../views/SetupView.vue'
import LoginView from '../views/LoginView.vue'
import DashboardLayout from '../views/DashboardLayout.vue'
import UsersView from '../views/UsersView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/setup', name: 'setup', component: SetupView },
    { path: '/login', name: 'login', component: LoginView },
    {
      path: '/',
      component: DashboardLayout,
      children: [
        { path: '', redirect: '/users' },
        { path: 'users', name: 'users', component: UsersView },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (!auth.checked) {
    await auth.checkStatus()
  }

  if (auth.needsSetup && to.name !== 'setup') {
    return { name: 'setup' }
  }
  if (!auth.needsSetup && to.name === 'setup') {
    return { name: 'users' }
  }
  if (!auth.needsSetup && !auth.user && to.name !== 'login') {
    return { name: 'login' }
  }
  if (auth.user && to.name === 'login') {
    return { name: 'users' }
  }
  return true
})

export default router
```

- [ ] **Step 4: Verificare manualmente in dev**

Run: apri `http://localhost:5173/` dopo il login: deve reindirizzare a `/users`, mostrare l'utente admin creato nel setup, e permettere di aggiungerne un secondo e rimuoverlo (ma non l'ultimo rimanente — verificare che l'API risponda 409 in quel caso).
Expected: flusso completo funzionante senza errori in console.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/views/DashboardLayout.vue frontend/src/views/UsersView.vue frontend/src/router/index.ts
git commit -m "feat: add dashboard layout and users view"
```

---

### Task 15: Vista impostazioni

**Files:**
- Create: `frontend/src/views/SettingsView.vue`
- Modify: `frontend/src/router/index.ts`

**Interfaces:**
- Consumes: `api` (Task 11).

- [ ] **Step 1: Creare la vista impostazioni**

`frontend/src/views/SettingsView.vue`:
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
}

const defaultLanguage = ref('it')
const youtubeApiKey = ref('')
const searchApiKey = ref('')
const searchApiProvider = ref('google')
const youtubeApiKeyMasked = ref('')
const searchApiKeyMasked = ref('')
const message = ref('')
const error = ref('')

async function load() {
  const s = await api.get<SettingsResponse>('/settings')
  defaultLanguage.value = s.defaultLanguage
  searchApiProvider.value = s.searchApiProvider || 'google'
  youtubeApiKeyMasked.value = s.youtubeApiKeyMasked || ''
  searchApiKeyMasked.value = s.searchApiKeyMasked || ''
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
    })
    youtubeApiKey.value = ''
    searchApiKey.value = ''
    message.value = 'Impostazioni salvate'
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(load)
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

- [ ] **Step 2: Aggiungere la route impostazioni**

`frontend/src/router/index.ts`: aggiungere l'import `import SettingsView from '../views/SettingsView.vue'` e, nell'array `children` della route `/`, la riga:
```ts
{ path: 'settings', name: 'settings', component: SettingsView },
```

- [ ] **Step 3: Verificare manualmente in dev**

Run: apri `http://localhost:5173/settings` da loggato, inserisci una chiave YouTube fittizia e salva: ricaricando la pagina la chiave deve apparire mascherata (non in chiaro) nel placeholder del campo.
Expected: salvataggio riuscito, chiave mascherata al reload.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/SettingsView.vue frontend/src/router/index.ts
git commit -m "feat: add settings view"
```

---

### Task 16: Embed del frontend nel binario Go

**Files:**
- Create: `backend/internal/webui/embed.go`
- Modify: `backend/cmd/server/main.go`

**Interfaces:**
- Produces: `webui.Handler() (http.Handler, error)`.

- [ ] **Step 1: Ricostruire il frontend per popolare la cartella embeddata**

```bash
cd frontend && npm run build
```

- [ ] **Step 2: Implementare l'handler statico con fallback SPA**

`backend/internal/webui/embed.go`:
```go
package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist/*
var distFS embed.FS

func Handler() (http.Handler, error) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if len(path) > 0 {
			path = path[1:]
		}
		if _, err := fs.Stat(sub, path); err != nil {
			indexReq := new(http.Request)
			*indexReq = *r
			indexReq.URL.Path = "/"
			fileServer.ServeHTTP(w, indexReq)
			return
		}
		fileServer.ServeHTTP(w, r)
	}), nil
}
```

- [ ] **Step 3: Aggiornare main.go per servire API e frontend sullo stesso processo**

`backend/cmd/server/main.go`:
```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/db"
	"boardgames-manager/internal/httpapi"
	"boardgames-manager/internal/settings"
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

- [ ] **Step 4: Verificare che il binario serva sia API sia frontend**

```bash
cd backend && go build -o ../bin/server ./cmd/server && DATA_DIR=/tmp/bgm-data PORT=8081 ../bin/server &
curl -s http://localhost:8081/api/health
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8081/
kill %1
```

Expected: la prima `curl` risponde `{"status":"ok"}`, la seconda risponde `200` (index.html servito).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/webui/embed.go backend/cmd/server/main.go
git commit -m "feat: embed frontend build into the Go binary"
```

---

### Task 17: Dockerfile e docker-compose

**Files:**
- Create: `Dockerfile`
- Create: `docker-compose.yml`
- Create: `.dockerignore`

- [ ] **Step 1: Creare il Dockerfile multi-stage**

`Dockerfile`:
```dockerfile
# syntax=docker/dockerfile:1

FROM node:22-alpine AS frontend-build
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm install
COPY frontend/ ./
RUN npm run build

FROM golang:1.23-alpine AS backend-build
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend-build /app/backend/internal/webui/dist ./internal/webui/dist
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=backend-build /out/server /server
EXPOSE 8080
ENV DATA_DIR=/data
ENTRYPOINT ["/server"]
```

- [ ] **Step 2: Creare docker-compose.yml**

`docker-compose.yml`:
```yaml
services:
  app:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
    environment:
      - PORT=8080
      - DATA_DIR=/data
    restart: unless-stopped
```

- [ ] **Step 3: Creare .dockerignore**

`.dockerignore`:
```
frontend/node_modules
frontend/dist
backend/data
data
bin
.git
```

- [ ] **Step 4: Verificare la build dell'immagine**

Run: `docker compose build`
Expected: build completata senza errori.

- [ ] **Step 5: Commit**

```bash
git add Dockerfile docker-compose.yml .dockerignore
git commit -m "feat: add Docker build and compose setup"
```

---

### Task 18: Verifica end-to-end in browser

**Files:** nessuno (solo verifica)

- [ ] **Step 1: Avviare l'applicazione via Docker Compose**

```bash
docker compose up -d --build
```

- [ ] **Step 2: Verificare l'health check**

```bash
curl -s http://localhost:8080/api/health
```

Expected: `{"status":"ok"}`

- [ ] **Step 3: Verificare il flusso completo in browser**

Apri `http://localhost:8080/` nel browser e verifica, in ordine:
1. Reindirizzamento automatico a `/setup` (nessun utente esistente).
2. Creazione del primo admin tramite il form → reindirizzamento a `/users`.
3. Logout → reindirizzamento a `/login`.
4. Login con le stesse credenziali → torna su `/users`.
5. Aggiunta di un secondo amministratore dalla vista Utenti.
6. Tentativo di rimuovere l'utente rimanente dopo aver rimosso il secondo → deve fallire con un messaggio d'errore (ultimo utente non eliminabile).
7. Vai su `/settings`, inserisci una chiave YouTube fittizia, salva, ricarica la pagina e verifica che appaia mascherata.

Expected: tutti i passaggi funzionano senza errori in console del browser.

- [ ] **Step 4: Arrestare l'ambiente**

```bash
docker compose down
```

- [ ] **Step 5: Annotare l'esito**

Se tutti i passaggi del Task 18 sono verificati con successo, la Fase 1 è considerata completa e pronta per la Fase 2 (Catalogo giochi + integrazione BGG).
