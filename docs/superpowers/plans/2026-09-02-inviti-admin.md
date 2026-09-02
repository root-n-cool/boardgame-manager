# Inviti admin e rifacimento /users — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Un amministratore aggiunge un collega inserendo solo l'email, copia il link di invito generato e lo manda a mano; il destinatario apre il link, sceglie la propria password ed entra — e la pagina `/users` viene ridisegnata nel linguaggio visivo del resto dell'app.

**Architecture:** L'invito vive dentro la tabella `users`: un admin in attesa è una riga con `password_hash = ''` e `invite_token` valorizzato (in chiaro, così il link è ricopiabile). All'attivazione si scrive l'hash bcrypt e si azzera il token, il che uccide il link. Due endpoint pubblici rate-limited (`GET`/`POST /api/invites/{token}`) servono la pagina di attivazione, che apre direttamente la sessione.

**Tech Stack:** Go 1.25 + chi, SQLite (modernc.org/sqlite), migrazioni custom su `embed.FS`, Vue 3 `<script setup>` + TypeScript + Vite, Pinia.

**Spec:** `docs/superpowers/specs/2026-09-02-inviti-admin-design.md`

## Global Constraints

- Comandi Go **solo in Docker**, riusando i volumi nominati:
  ```bash
  docker run --rm -v "$(pwd)/backend:/app" \
    -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
    -w /app golang:1.25 go test ./...
  ```
  (da lanciare dalla root del repo; non sostituire i volumi nominati)
- `npm` gira in locale, dentro `frontend/`.
- Migrazioni **forward-only**: nuovo file `NNNN_nome.sql`, mai modificare
  un file già rilasciato.
- Zero nuove dipendenze Go o npm. Solo stdlib, chi, e ciò che c'è già.
- Nessun i18n: stringhe italiane direttamente nei componenti.
- UI: token e classi in `frontend/src/app.css`, sistema visivo in
  `DESIGN.md`.
- Commit dopo ogni task, messaggi in inglese conventional-commits.
- Ultimo task obbligatorio: `/impeccable polish` sulle superfici toccate.

## File Structure

**Backend**
- `backend/internal/db/migrations/0006_admin_invites.sql` (nuovo) —
  colonna `invite_token` + indice unico parziale.
- `backend/internal/users/store.go` (modifica) — `User.InviteToken`,
  `Pending()`, `CreateInvite`, `GetByInviteToken`, `Activate`, nuovo
  criterio in `DeleteIfNotLast`.
- `backend/internal/httpapi/users_handlers.go` (modifica) — create con
  sola email, `userResponse` condiviso.
- `backend/internal/httpapi/invites_handlers.go` (nuovo) — i due
  endpoint pubblici dell'invito. Stanno in un file loro perché sono
  l'unica superficie pubblica non autenticata che scrive su `users`.
- `backend/internal/httpapi/auth_handlers.go` (modifica) — login
  rifiuta un utente senza password.
- `backend/internal/httpapi/router.go` (modifica) — rotte + rate limit.

**Frontend**
- `frontend/src/views/UsersView.vue` (riscritta) — lista admin a righe,
  invito inline, copia link, elimina con conferma.
- `frontend/src/views/InviteAcceptView.vue` (nuovo) — pagina pubblica
  di attivazione.
- `frontend/src/router/index.ts` (modifica) — rotta pubblica
  `/invito/:token`.
- `frontend/src/stores/auth.ts` (modifica) — action `acceptInvite`.
- `frontend/src/app.css` (modifica) — blocco "Amministratori".

**Docs**
- `DESIGN.md`, `README.md` (modifica).

---

### Task 1: Migrazione e store degli inviti

**Files:**
- Create: `backend/internal/db/migrations/0006_admin_invites.sql`
- Modify: `backend/internal/users/store.go`
- Test: `backend/internal/users/store_test.go`

**Interfaces:**
- Consumes: niente (primo task).
- Produces:
  - `users.User` con campo `InviteToken *string` e metodo
    `func (u User) Pending() bool`
  - `func (s *Store) CreateInvite(ctx context.Context, email, token string) (User, error)`
  - `func (s *Store) GetByInviteToken(ctx context.Context, token string) (User, error)`
  - `func (s *Store) Activate(ctx context.Context, id int64, passwordHash string) error`
  - `DeleteIfNotLast` invariata nella firma, nuova semantica.

- [ ] **Step 1: Scrivi i test falliti nello store**

Appendi a `backend/internal/users/store_test.go`:

```go
func TestCreateInvite_IsPendingAndFoundByToken(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	created, err := store.CreateInvite(ctx, "invited@example.com", "tok-abc")
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	if !created.Pending() {
		t.Fatal("a freshly invited admin must be pending")
	}
	if created.InviteToken == nil || *created.InviteToken != "tok-abc" {
		t.Fatalf("expected invite token tok-abc, got %v", created.InviteToken)
	}
	if created.PasswordHash != "" {
		t.Fatalf("expected an empty password hash, got %q", created.PasswordHash)
	}

	found, err := store.GetByInviteToken(ctx, "tok-abc")
	if err != nil {
		t.Fatalf("get by invite token: %v", err)
	}
	if found.ID != created.ID {
		t.Fatalf("expected user %d, got %d", created.ID, found.ID)
	}
}

func TestCreateInvite_DuplicateEmailReturnsError(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Create(ctx, "admin@example.com", "hashed-value"); err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err := store.CreateInvite(ctx, "admin@example.com", "tok-abc")
	if !errors.Is(err, users.ErrDuplicateEmail) {
		t.Fatalf("expected ErrDuplicateEmail, got %v", err)
	}
}

func TestGetByInviteToken_UnknownTokenReturnsNotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.GetByInviteToken(context.Background(), "nope")
	if !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestActivate_SetsPasswordAndKillsTheLink(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	invited, err := store.CreateInvite(ctx, "invited@example.com", "tok-abc")
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}

	if err := store.Activate(ctx, invited.ID, "hashed-value"); err != nil {
		t.Fatalf("activate: %v", err)
	}

	active, err := store.GetByID(ctx, invited.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if active.Pending() {
		t.Fatal("an activated admin must not be pending")
	}
	if active.PasswordHash != "hashed-value" {
		t.Fatalf("expected the new hash, got %q", active.PasswordHash)
	}

	// The link must die: nobody may reuse it to rewrite the password.
	if _, err := store.GetByInviteToken(ctx, "tok-abc"); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("expected the token to be gone, got %v", err)
	}
	if err := store.Activate(ctx, invited.ID, "second-hash"); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("expected a second activation to fail, got %v", err)
	}
}

// A pending invite is not a working way in, so it must not keep the instance
// alive in place of the only active admin.
func TestDeleteIfNotLast_PendingInviteDoesNotCountAsAnAdmin(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	active, err := store.Create(ctx, "admin@example.com", "hashed-value")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.CreateInvite(ctx, "invited@example.com", "tok-abc"); err != nil {
		t.Fatalf("create invite: %v", err)
	}

	if err := store.DeleteIfNotLast(ctx, active.ID); !errors.Is(err, users.ErrCannotDeleteLastUser) {
		t.Fatalf("expected ErrCannotDeleteLastUser, got %v", err)
	}
}

// ...and conversely it must always be revocable, even with a single active admin.
func TestDeleteIfNotLast_PendingInviteCanAlwaysBeRevoked(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Create(ctx, "admin@example.com", "hashed-value"); err != nil {
		t.Fatalf("create: %v", err)
	}
	invited, err := store.CreateInvite(ctx, "invited@example.com", "tok-abc")
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}

	if err := store.DeleteIfNotLast(ctx, invited.ID); err != nil {
		t.Fatalf("expected the pending invite to be deletable, got %v", err)
	}
	if _, err := store.GetByID(ctx, invited.ID); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("expected the invite to be gone, got %v", err)
	}
}
```

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/users/ -run 'Invite|Activate' -v
```

Atteso: FAIL in compilazione — `store.CreateInvite undefined`,
`created.Pending undefined`, `created.InviteToken undefined`.

- [ ] **Step 3: Scrivi la migrazione**

`backend/internal/db/migrations/0006_admin_invites.sql`:

```sql
-- Un admin invitato ma non ancora attivo è una riga users con
-- password_hash = '' e invite_token valorizzato. Il token è in chiaro di
-- proposito: il bottone "Copia link invito" deve poter mostrare sempre lo
-- stesso link, e l'unico a poterlo leggere è un admin già autenticato.
-- All'attivazione il token torna NULL, ed è così che il link muore.
ALTER TABLE users ADD COLUMN invite_token TEXT;

CREATE UNIQUE INDEX users_invite_token_unique
    ON users(invite_token) WHERE invite_token IS NOT NULL;
```

- [ ] **Step 4: Estendi lo store**

In `backend/internal/users/store.go`, aggiungi il campo alla struct
`User` (dopo `PasswordHash`):

```go
	// InviteToken is the plaintext token of the invite link, non-nil only
	// until the admin has chosen their own password. Clearing it is what
	// makes the link unusable.
	InviteToken *string
```

e il metodo derivato, subito dopo la struct:

```go
// Pending reports whether the admin still has an invite to accept.
// invite_token is the only "active" criterion the code uses: password_hash
// stays an internal detail of the store.
func (u User) Pending() bool { return u.InviteToken != nil }
```

Aggiorna le tre SELECT esistenti (`GetByEmail`, `GetByID`, `List`) per
leggere anche `invite_token`, e `scanOne` per farne lo scan:

```go
func (s *Store) GetByEmail(ctx context.Context, email string) (User, error) {
	return s.scanOne(ctx, `SELECT id, email, password_hash, invite_token, created_at FROM users WHERE email = ?`, email)
}

func (s *Store) GetByID(ctx context.Context, id int64) (User, error) {
	return s.scanOne(ctx, `SELECT id, email, password_hash, invite_token, created_at FROM users WHERE id = ?`, id)
}

func (s *Store) GetByInviteToken(ctx context.Context, token string) (User, error) {
	return s.scanOne(ctx, `SELECT id, email, password_hash, invite_token, created_at FROM users WHERE invite_token = ?`, token)
}

func (s *Store) scanOne(ctx context.Context, query string, arg any) (User, error) {
	var u User
	var inviteToken sql.NullString
	var createdAt string
	err := s.db.QueryRowContext(ctx, query, arg).Scan(&u.ID, &u.Email, &u.PasswordHash, &inviteToken, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	if inviteToken.Valid {
		token := inviteToken.String
		u.InviteToken = &token
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return u, nil
}

func (s *Store) List(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, email, password_hash, invite_token, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []User
	for rows.Next() {
		var u User
		var inviteToken sql.NullString
		var createdAt string
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &inviteToken, &createdAt); err != nil {
			return nil, err
		}
		if inviteToken.Valid {
			token := inviteToken.String
			u.InviteToken = &token
		}
		u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		out = append(out, u)
	}
	return out, rows.Err()
}
```

Aggiungi i due nuovi metodi di scrittura, dopo `Create`:

```go
// CreateInvite creates a pending admin: no password, one invite token. The
// password is written by the invitee themselves through the link (see
// Activate), so whoever invited them never knows it.
func (s *Store) CreateInvite(ctx context.Context, email, token string) (User, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO users (email, password_hash, invite_token) VALUES (?, '', ?)`, email, token)
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

// Activate writes the password the invitee chose and clears the token.
//
// The `invite_token IS NOT NULL` condition in the WHERE is what makes the link
// single-use: two concurrent POSTs on the same invite cannot set two different
// passwords — the second one matches zero rows and gets ErrNotFound.
func (s *Store) Activate(ctx context.Context, id int64, passwordHash string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, invite_token = NULL WHERE id = ? AND invite_token IS NOT NULL`,
		passwordHash, id)
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

Infine cambia il conteggio in `DeleteIfNotLast`, sostituendo il blocco
`var count int … if count <= 1 { … }` con:

```go
	// Count the ACTIVE admins other than the target: if none is left, the
	// deletion would leave the instance with nobody able to sign in, while
	// POST /api/bootstrap stays closed (it looks at the total COUNT(*)).
	// A pending invite is not a working way in, so it does not count as an
	// admin — and for the same reason it is always revocable.
	var otherActive int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE invite_token IS NULL AND id != ?`, id).Scan(&otherActive); err != nil {
		return err
	}
	if otherActive == 0 {
		return ErrCannotDeleteLastUser
	}
```

Aggiorna anche il commento-dottrina sopra `DeleteIfNotLast` perché parla
di "count == 2": la sostanza (una sola transazione, TOCTOU) resta, il
criterio no.

**Tutti i commenti Go nuovi — anche nei file `_test.go` — vanno in
inglese**, come tutto il resto del backend (vedi il commento su
`PasswordHash` e quello su `DeleteIfNotLast`). L'italiano nel codice sta
solo nel frontend.

- [ ] **Step 5: Lancia i test dello store e verifica che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/users/ -v
```

Atteso: PASS su tutti i test, vecchi e nuovi. Se
`TestDeleteIfNotLast_RefusesToEmptyTheUsersTable` fallisce, il criterio
nuovo è stato scritto male: con un solo utente attivo `otherActive`
deve essere `0`.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/db/migrations/0006_admin_invites.sql \
        backend/internal/users/store.go backend/internal/users/store_test.go
git commit -m "feat: store pending admin invites in the users table"
```

---

### Task 2: POST /api/users con la sola email

**Files:**
- Modify: `backend/internal/httpapi/users_handlers.go`
- Test: `backend/internal/httpapi/users_handlers_test.go`

**Interfaces:**
- Consumes: `users.Store.CreateInvite`, `User.Pending()`,
  `User.InviteToken` (Task 1); `auth.GenerateToken()` (esistente).
- Produces:
  - `POST /api/users` body `{"email": "..."}` → `201`
    `{id, email, createdAt, pending: true, inviteToken: "<hex>"}`
  - `GET /api/users` → array degli stessi oggetti (`inviteToken: null`
    per gli attivi)
  - helper interno `func userResponse(u users.User) map[string]any`

- [ ] **Step 1: Scrivi i test falliti**

In `backend/internal/httpapi/users_handlers_test.go` sostituisci
`TestCreateUser_AsAdminSucceeds` con questi due test e aggiungi il
terzo:

```go
func TestCreateUser_WithOnlyAnEmailReturnsAnInviteToken(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"email": "second@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID          int64  `json:"id"`
		Email       string `json:"email"`
		Pending     bool   `json:"pending"`
		InviteToken string `json:"inviteToken"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode created user: %v", err)
	}
	if created.Email != "second@example.com" {
		t.Errorf("expected the invited email back, got %q", created.Email)
	}
	if !created.Pending {
		t.Error("a freshly invited admin must be reported as pending")
	}
	if len(created.InviteToken) != 64 {
		t.Errorf("expected a 64-char hex invite token, got %q", created.InviteToken)
	}
}

func TestCreateUser_MissingEmailReturnsBadRequest(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"email": "   "})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListUsers_ReportsPendingAndActiveAdmins(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")

	payload, _ := json.Marshal(map[string]string{"email": "second@example.com"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(payload))
	createReq.AddCookie(cookie)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("invite second admin: %d %s", createRec.Code, createRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	listReq.AddCookie(cookie)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	var list []struct {
		Email       string  `json:"email"`
		Pending     bool    `json:"pending"`
		InviteToken *string `json:"inviteToken"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 admins, got %d", len(list))
	}
	if list[0].Pending || list[0].InviteToken != nil {
		t.Errorf("the bootstrapped admin must be active with a null token, got %+v", list[0])
	}
	if !list[1].Pending || list[1].InviteToken == nil || *list[1].InviteToken == "" {
		t.Errorf("the invited admin must be pending with a token, got %+v", list[1])
	}
}
```

Nello stesso file, nei quattro test che creano un secondo admin
(`TestCreateUser_DuplicateEmailReturnsConflict`,
`TestDeleteUser_RemovesTheUserAndReturnsOK`,
`TestDeleteUser_UnknownIDReturnsNotFound`,
`TestDeleteUser_DeletedUsersSessionIsRejected`), togli la chiave
`"password"` dal payload: ora l'endpoint accetta solo l'email.
`TestDeleteUser_DeletedUsersSessionIsRejected` va sistemato a fondo nel
Task 3, quando esiste l'endpoint di attivazione; per ora lascialo
compilare togliendo la password e marcandolo:

```go
	t.Skip("riattivato nel Task 3: la vittima ora entra accettando l'invito")
```

come prima riga del test.

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run 'TestCreateUser|TestListUsers' -v
```

Atteso: FAIL — `inviteToken` vuoto e `pending` falso, perché l'handler
attuale pretende ancora la password e non espone i nuovi campi.

- [ ] **Step 3: Riscrivi gli handler**

In `backend/internal/httpapi/users_handlers.go` sostituisci
`listUsersHandler` e `createUserHandler`, e aggiungi l'helper. Serve
anche l'import di `strings`:

```go
// userResponse is the JSON shape of an admin. inviteToken stays a nil
// interface for active admins, so it marshals to null rather than an empty
// string; for pending ones the plaintext token is the point of the feature —
// only an authenticated admin reads it, and they are the one who has to copy
// the link.
func userResponse(u users.User) map[string]any {
	var inviteToken any
	if u.InviteToken != nil {
		inviteToken = *u.InviteToken
	}
	return map[string]any{
		"id":          u.ID,
		"email":       u.Email,
		"createdAt":   u.CreatedAt,
		"pending":     u.Pending(),
		"inviteToken": inviteToken,
	}
}

func (s *Server) listUsersHandler(w http.ResponseWriter, r *http.Request) {
	list, err := s.Users.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list users")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, u := range list {
		out = append(out, userResponse(u))
	}
	writeJSON(w, http.StatusOK, out)
}

type inviteUserRequest struct {
	Email string `json:"email"`
}

// createUserHandler no longer accepts a password: whoever invites must not
// know another admin's. It mints an invite link instead, which the caller
// copies and delivers by hand (no SMTP in v1).
func (s *Server) createUserHandler(w http.ResponseWriter, r *http.Request) {
	var req inviteUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	email := strings.TrimSpace(req.Email)
	if email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	token, err := auth.GenerateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create invite")
		return
	}

	user, err := s.Users.CreateInvite(r.Context(), email, token)
	if err != nil {
		if errors.Is(err, users.ErrDuplicateEmail) {
			writeError(w, http.StatusConflict, "email already in use")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create invite")
		return
	}

	writeJSON(w, http.StatusCreated, userResponse(user))
}
```

- [ ] **Step 4: Lancia i test e verifica che passino**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -v
```

Atteso: PASS, con `TestDeleteUser_DeletedUsersSessionIsRejected`
riportato come SKIP.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/httpapi/users_handlers.go \
        backend/internal/httpapi/users_handlers_test.go
git commit -m "feat: create admins by email only, returning an invite token"
```

---

### Task 3: Endpoint pubblici dell'invito e login degli utenti in attesa

**Files:**
- Create: `backend/internal/httpapi/invites_handlers.go`
- Create: `backend/internal/httpapi/invites_handlers_test.go`
- Modify: `backend/internal/httpapi/router.go`
- Modify: `backend/internal/httpapi/auth_handlers.go`
- Test: `backend/internal/httpapi/auth_handlers_test.go`,
  `backend/internal/httpapi/users_handlers_test.go` (rimuovi lo skip)

**Interfaces:**
- Consumes: `users.Store.GetByInviteToken`, `users.Store.Activate`
  (Task 1); `s.startSession` e `auth.HashPassword` (esistenti).
- Produces:
  - `GET /api/invites/{token}` → `200 {"email": "..."}` | `404`
  - `POST /api/invites/{token}` body `{"password": "..."}` →
    `200 {id, email}` + cookie `session_token` | `400` | `404`

- [ ] **Step 1: Scrivi i test falliti**

Crea `backend/internal/httpapi/invites_handlers_test.go`:

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

// inviteAdmin invites an admin and returns the token of their link.
func inviteAdmin(t *testing.T, router http.Handler, cookie *http.Cookie, email string) string {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"email": email})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite %s: %d %s", email, rec.Code, rec.Body.String())
	}
	var created struct {
		InviteToken string `json:"inviteToken"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode invite: %v", err)
	}
	if created.InviteToken == "" {
		t.Fatal("no invite token returned")
	}
	return created.InviteToken
}

func TestGetInvite_ReturnsTheInvitedEmailWithoutAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	token := inviteAdmin(t, router, cookie, "invited@example.com")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/invites/"+token, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode invite: %v", err)
	}
	if body.Email != "invited@example.com" {
		t.Errorf("expected invited@example.com, got %q", body.Email)
	}
}

func TestGetInvite_UnknownTokenReturnsNotFound(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/invites/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAcceptInvite_SetsThePasswordAndOpensASession(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	token := inviteAdmin(t, router, cookie, "invited@example.com")

	payload, _ := json.Marshal(map[string]string{"password": "chosenpass1"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/invites/"+token, bytes.NewReader(payload)))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session_token" {
			session = c
		}
	}
	if session == nil {
		t.Fatal("accepting an invite must open a session")
	}

	// The freshly opened session must work on the protected routes.
	meReq := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	meReq.AddCookie(session)
	meRec := httptest.NewRecorder()
	router.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("expected the new session to work, got %d", meRec.Code)
	}

	// And the chosen password must work on the next login.
	loginAndGetCookie(t, router, "invited@example.com", "chosenpass1")
}

func TestAcceptInvite_LinkWorksOnlyOnce(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	token := inviteAdmin(t, router, cookie, "invited@example.com")

	payload, _ := json.Marshal(map[string]string{"password": "chosenpass1"})
	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/api/invites/"+token, bytes.NewReader(payload)))
	if first.Code != http.StatusOK {
		t.Fatalf("first accept: %d %s", first.Code, first.Body.String())
	}

	second := httptest.NewRecorder()
	payload2, _ := json.Marshal(map[string]string{"password": "hijacked999"})
	router.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/api/invites/"+token, bytes.NewReader(payload2)))
	if second.Code != http.StatusNotFound {
		t.Fatalf("expected 404 reusing a spent invite, got %d: %s", second.Code, second.Body.String())
	}

	// The second attempt must not have rewritten the password.
	loginAndGetCookie(t, router, "invited@example.com", "chosenpass1")
}

func TestAcceptInvite_ShortPasswordReturnsBadRequest(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	token := inviteAdmin(t, router, cookie, "invited@example.com")

	payload, _ := json.Marshal(map[string]string{"password": "corta"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/invites/"+token, bytes.NewReader(payload)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

Aggiungi a `backend/internal/httpapi/auth_handlers_test.go`:

```go
// An invited but not yet active admin has no password: login must reject them
// like any wrong credential, without revealing that the email exists pending.
func TestLogin_PendingAdminCannotSignIn(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	inviteAdmin(t, router, cookie, "invited@example.com")

	payload, _ := json.Marshal(map[string]string{"email": "invited@example.com", "password": ""})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payload)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("an empty password is a bad request, got %d", rec.Code)
	}

	payload, _ = json.Marshal(map[string]string{"email": "invited@example.com", "password": "anything123"})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payload)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a pending admin, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

In `backend/internal/httpapi/users_handlers_test.go` togli la riga
`t.Skip(...)` da `TestDeleteUser_DeletedUsersSessionIsRejected` e
sostituisci il blocco che crea la vittima e fa il login con:

```go
	victimToken := inviteAdmin(t, router, adminCookie, "victim@example.com")

	acceptPayload, _ := json.Marshal(map[string]string{"password": "victimpass1"})
	acceptRec := httptest.NewRecorder()
	router.ServeHTTP(acceptRec, httptest.NewRequest(http.MethodPost, "/api/invites/"+victimToken, bytes.NewReader(acceptPayload)))
	if acceptRec.Code != http.StatusOK {
		t.Fatalf("accept invite: %d %s", acceptRec.Code, acceptRec.Body.String())
	}

	var victim struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(acceptRec.Body).Decode(&victim); err != nil {
		t.Fatalf("decode victim: %v", err)
	}

	victimCookie := loginAndGetCookie(t, router, "victim@example.com", "victimpass1")
```

- [ ] **Step 2: Lancia i test e verifica che falliscano**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/httpapi/ -run 'Invite|TestLogin_Pending' -v
```

Atteso: FAIL — `GET /api/invites/...` risponde 404 dal router (rotta
inesistente) e `POST` idem; il test del login in attesa può passare per
caso (bcrypt fallisce su hash vuoto), il controllo esplicito lo rende
intenzionale.

- [ ] **Step 3: Scrivi gli handler dell'invito**

Crea `backend/internal/httpapi/invites_handlers.go`:

```go
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"boardgames-manager/internal/auth"
)

// minPasswordLength is the same minimum the frontend forms ask for
// (minlength=8); the check here is the one that counts.
const minPasswordLength = 8

// getInviteHandler serves the public activation page: it says who the link
// belongs to, so the invitee sees their own email and knows they are on the
// right page. A spent token and an unknown one are indistinguishable: 404.
func (s *Server) getInviteHandler(w http.ResponseWriter, r *http.Request) {
	user, err := s.Users.GetByInviteToken(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		writeError(w, http.StatusNotFound, "invite not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"email": user.Email})
}

type acceptInviteRequest struct {
	Password string `json:"password"`
}

// acceptInviteHandler closes the invite loop: the invitee writes their own
// password, the token dies and the session starts right away — whoever invited
// them never saw that password.
func (s *Server) acceptInviteHandler(w http.ResponseWriter, r *http.Request) {
	var req acceptInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}
	if len(req.Password) < minPasswordLength {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	user, err := s.Users.GetByInviteToken(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		writeError(w, http.StatusNotFound, "invite not found")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not set the password")
		return
	}

	// Activate fails if the token was spent between the read above and now:
	// two requests on the same link cannot set two different passwords.
	if err := s.Users.Activate(r.Context(), user.ID, hash); err != nil {
		writeError(w, http.StatusNotFound, "invite not found")
		return
	}

	if err := s.startSession(w, r, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not start session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"id": user.ID, "email": user.Email})
}
```

- [ ] **Step 4: Registra le rotte e chiudi il login**

In `backend/internal/httpapi/router.go`, accanto agli altri limiter:

```go
	inviteLimiter := newRateLimiter(10, time.Minute)
```

e fra le rotte pubbliche, dopo `r.Post("/api/login", s.loginHandler)`:

```go
	// The token is 32 random bytes: the rate limit is not there against
	// bruteforce, it is there so these two endpoints cannot be used as a probe.
	r.With(inviteLimiter.middleware).Get("/api/invites/{token}", s.getInviteHandler)
	r.With(inviteLimiter.middleware).Post("/api/invites/{token}", s.acceptInviteHandler)
```

In `backend/internal/httpapi/auth_handlers.go`, dentro `loginHandler`,
subito dopo il `GetByEmail`:

```go
	// An admin with an invite still to accept has no password: bcrypt would
	// fail on an empty hash anyway, but saying it here makes the intent
	// readable and does not lean on the library's behaviour.
	if user.Pending() {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
```

- [ ] **Step 5: Lancia tutta la suite backend e verifica che passi**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```

Atteso: `ok` per ogni package, nessuno SKIP residuo.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi/invites_handlers.go \
        backend/internal/httpapi/invites_handlers_test.go \
        backend/internal/httpapi/router.go \
        backend/internal/httpapi/auth_handlers.go \
        backend/internal/httpapi/auth_handlers_test.go \
        backend/internal/httpapi/users_handlers_test.go
git commit -m "feat: accept an admin invite link and set your own password"
```

---

### Task 4: Pagina /users a righe, con invito inline e copia link

**Files:**
- Modify: `frontend/src/app.css` (blocco nuovo in fondo)
- Modify: `frontend/src/views/UsersView.vue` (riscrittura completa)

**Interfaces:**
- Consumes: `GET /api/users` (`{id, email, createdAt, pending,
  inviteToken}`), `POST /api/users` (`{email}`),
  `DELETE /api/users/{id}` (Task 2); `ModalDialog` con props
  `{open: boolean, title: string}` ed evento `close` (esistente).
- Produces: la rotta `/invito/<token>` come formato del link mostrato
  (la pagina che lo serve arriva nel Task 5).

- [ ] **Step 1: Aggiungi gli stili**

In fondo a `frontend/src/app.css`:

```css
/* ---------- Amministratori (/users) ---------- */

/* Una riga per admin dentro un foglio: la pedina con l'iniziale al posto
   della copertina, il badge di stato dove il catalogo mette l'anno. Il
   filetto fra le righe arriva dalla regola globale su li. */
/* La regola globale `ul` in questo foglio veste ogni lista da card (fondo,
   bordo, raggio, ombra) e `li` è una flex-row con padding proprio: qui la
   card è già il .panel-card che contiene la lista, quindi la lista si
   spoglia — come fa .event-list — e la riga lascia il layout a .admin-row. */
.admin-list {
  list-style: none;
  margin: 0;
  padding: 0;
  background: none;
  border: none;
  border-radius: 0;
  overflow: visible;
  box-shadow: none;
}

.admin-list li {
  display: block;
  padding: 0;
}

.admin-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.7rem;
  padding: 0.7rem 0;
}

.admin-pawn {
  flex: none;
  width: 2.25rem;
  height: 2.25rem;
  border-radius: 999px;
  display: grid;
  place-items: center;
  background: var(--card-alt);
  border: 1px solid var(--card-line);
  font-family: 'Display', system-ui, sans-serif;
  font-weight: 700;
  text-transform: uppercase;
  color: var(--ink-muted);
}

/* Chi non è ancora entrato è una pedina non ancora posata: tratteggiata. */
.admin-row.is-pending .admin-pawn {
  background: none;
  border-style: dashed;
}

.admin-email {
  flex: 1 1 12rem;
  min-width: 0;
  font-weight: 600;
  overflow-wrap: anywhere;
}

.admin-row-actions {
  display: flex;
  gap: 0.4rem;
  margin-left: auto;
}

.admin-list li button {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
}

.admin-list li button svg {
  flex: none;
  width: 0.85rem;
  height: 0.85rem;
}

/* Nella lista il rosso è il colore di "Rimuovi" (regola globale su li
   button): copiare un link non è un'azione d'allarme, quindi passa all'oro. */
.admin-list li button.btn-invite {
  color: var(--gold-text);
}

.admin-list li button.btn-invite:hover {
  background: var(--gold-bg);
  border-color: var(--gold);
}

/* Il link vive sotto la riga, rientrato oltre la pedina: è un dato da
   copiare, quindi mono, e non deve mai allargare il foglio. */
.admin-invite {
  flex: 1 1 100%;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-left: 2.95rem;
  padding: 0.35rem 0.6rem;
  background: var(--card-alt);
  border: 1px solid var(--card-line);
  border-radius: var(--radius-sm);
}

.admin-invite code {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  font-family: 'Data', monospace;
  font-size: 0.82rem;
  color: var(--ink-muted);
}

.admin-invite-copied {
  flex: none;
  font-size: 0.82rem;
  font-weight: 600;
  color: var(--success);
}

/* Lo slot vuoto in fondo alla lista, come la cella "Aggiungi gioco" del
   catalogo: tratteggio che si chiude al passaggio. */
.admin-add {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  width: 100%;
  margin-top: 0.9rem;
  padding: 0.75rem 0.9rem;
  border: 2px dashed color-mix(in srgb, var(--ink-muted) 70%, var(--card-line));
  border-radius: var(--radius);
  background: none;
  color: var(--ink-muted);
  font-family: 'Display', system-ui, sans-serif;
  font-size: 1rem;
  font-weight: 600;
  transition: border-color 0.15s ease, background-color 0.15s ease, color 0.15s ease;
}

.admin-add:hover {
  border-style: solid;
  border-color: var(--accent);
  background: var(--card-alt);
  color: var(--accent);
}

.admin-add svg {
  flex: none;
  width: 1rem;
  height: 1rem;
}

.admin-add-form {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-top: 0.9rem;
}

.admin-add-form input {
  flex: 1 1 14rem;
  min-width: 0;
}
```

- [ ] **Step 2: Riscrivi la view**

Sostituisci l'intero `frontend/src/views/UsersView.vue`:

```vue
<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { api } from '../api/client'
import ModalDialog from '../components/ModalDialog.vue'

interface AdminUser {
  id: number
  email: string
  createdAt: string
  pending: boolean
  inviteToken: string | null
}

// Gli errori dell'API sono in inglese perché sono codici, non copy: qui
// diventano le frasi che l'admin legge davvero.
const MESSAGES: Record<string, string> = {
  'email is required': "Inserisci l'email del nuovo amministratore.",
  'email already in use': 'Questa email è già registrata.',
  'cannot delete the last remaining user': "Non puoi eliminare l'unico amministratore attivo.",
  'user not found': 'Questo amministratore non esiste più.',
}

function toItalian(message: string) {
  return MESSAGES[message] ?? message
}

const users = ref<AdminUser[]>([])
const error = ref('')
const adding = ref(false)
const newEmail = ref('')
const emailInput = ref<HTMLInputElement | null>(null)
const saving = ref(false)
const copiedId = ref<number | null>(null)
const pendingDelete = ref<AdminUser | null>(null)

const countLabel = computed(() =>
  users.value.length === 1 ? '1 amministratore' : `${users.value.length} amministratori`,
)

function initial(email: string) {
  return email.trim().charAt(0) || '?'
}

// Il link lo compone il browser: il backend non conosce il proprio URL
// pubblico e non vogliamo una variabile d'ambiente in più per il selfhost.
function inviteUrl(user: AdminUser) {
  return `${window.location.origin}/invito/${user.inviteToken}`
}

async function loadUsers() {
  users.value = await api.get<AdminUser[]>('/users')
}

async function startAdding() {
  adding.value = true
  newEmail.value = ''
  error.value = ''
  await nextTick()
  emailInput.value?.focus()
}

function cancelAdding() {
  adding.value = false
  newEmail.value = ''
}

async function invite() {
  error.value = ''
  saving.value = true
  try {
    await api.post<AdminUser>('/users', { email: newEmail.value })
    cancelAdding()
    await loadUsers()
  } catch (e) {
    error.value = toItalian((e as Error).message)
  } finally {
    saving.value = false
  }
}

async function copyInvite(user: AdminUser) {
  error.value = ''
  try {
    await navigator.clipboard.writeText(inviteUrl(user))
    copiedId.value = user.id
    window.setTimeout(() => {
      if (copiedId.value === user.id) {
        copiedId.value = null
      }
    }, 2000)
  } catch {
    // Clipboard non disponibile (origine non sicura, permesso negato): il
    // link resta a schermo e selezionabile, quindi basta dirlo.
    error.value = 'Copia non riuscita: seleziona il link e copialo a mano.'
  }
}

async function confirmDelete() {
  const target = pendingDelete.value
  if (!target) {
    return
  }
  pendingDelete.value = null
  error.value = ''
  try {
    await api.delete(`/users/${target.id}`)
    await loadUsers()
  } catch (e) {
    error.value = toItalian((e as Error).message)
  }
}

onMounted(async () => {
  // Senza il try questa diventa una unhandled rejection e una pagina vuota
  // ogni volta che la richiesta fallisce per un motivo diverso dal 401.
  try {
    await loadUsers()
  } catch (e) {
    error.value = toItalian((e as Error).message)
  }
})
</script>

<template>
  <div>
    <div class="page-head">
      <div class="page-head-text">
        <h1>Amministratori</h1>
        <p class="page-meta">{{ countLabel }}</p>
      </div>
    </div>

    <p v-if="error" class="error">{{ error }}</p>

    <div class="panel-card">
      <ul role="list" class="admin-list">
        <li v-for="u in users" :key="u.id">
          <div class="admin-row" :class="{ 'is-pending': u.pending }">
            <span class="admin-pawn" aria-hidden="true">{{ initial(u.email) }}</span>
            <span class="admin-email">{{ u.email }}</span>
            <span class="status-badge" :class="u.pending ? 'status-cancelled' : 'status-active'">
              {{ u.pending ? 'In attesa' : 'Attivo' }}
            </span>
            <div class="admin-row-actions">
              <button
                v-if="u.pending"
                type="button"
                class="btn-invite"
                @click="copyInvite(u)"
              >
                <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                  <path
                    d="M10 13a5 5 0 007.07 0l2.12-2.12a5 5 0 00-7.07-7.07L10.7 5.24M14 11a5 5 0 00-7.07 0L4.81 13.12a5 5 0 007.07 7.07l1.42-1.41"
                    stroke="currentColor"
                    stroke-width="1.7"
                    stroke-linecap="round"
                  />
                </svg>
                Copia link invito
              </button>
              <button type="button" @click="pendingDelete = u">
                <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                  <path
                    d="M4 7h16M9 7V5a1 1 0 011-1h4a1 1 0 011 1v2M6 7l1 12a1 1 0 001 1h8a1 1 0 001-1l1-12M10 11v6M14 11v6"
                    stroke="currentColor"
                    stroke-width="1.7"
                    stroke-linecap="round"
                  />
                </svg>
                Elimina
              </button>
            </div>
            <div v-if="u.pending && u.inviteToken" class="admin-invite">
              <code>{{ inviteUrl(u) }}</code>
              <span v-if="copiedId === u.id" class="admin-invite-copied" role="status">Copiato</span>
            </div>
          </div>
        </li>
      </ul>

      <form v-if="adding" class="admin-add-form" @submit.prevent="invite">
        <input
          ref="emailInput"
          v-model="newEmail"
          type="email"
          required
          placeholder="email@esempio.it"
          aria-label="Email del nuovo amministratore"
          @keydown.esc="cancelAdding"
        />
        <button type="submit" :disabled="saving">{{ saving ? 'Invito…' : 'Invita' }}</button>
        <button type="button" class="btn-secondary" @click="cancelAdding">Annulla</button>
      </form>
      <button v-else type="button" class="admin-add" @click="startAdding">
        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <path d="M12 5v14M5 12h14" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
        </svg>
        Aggiungi admin
      </button>
    </div>

    <ModalDialog
      :open="pendingDelete !== null"
      title="Eliminare l'amministratore?"
      @close="pendingDelete = null"
    >
      <p>
        {{ pendingDelete?.email }} non potrà più accedere.
        <template v-if="pendingDelete?.pending">Il link di invito smetterà di funzionare.</template>
      </p>
      <div class="form-actions">
        <button type="button" class="btn-secondary" @click="pendingDelete = null">Annulla</button>
        <button type="button" @click="confirmDelete">Elimina</button>
      </div>
    </ModalDialog>
  </div>
</template>
```

- [ ] **Step 3: Verifica il type-check e il build**

```bash
cd frontend && npm run build
```

Atteso: build completato senza errori `vue-tsc`. `.form-actions` e
`.btn-secondary` esistono già in `app.css` e sono le stesse classi usate
dalle modali di `GameDetailView.vue` (con "Annulla" a sinistra e
l'azione a destra): non ne servono di nuove.

- [ ] **Step 4: Prova in browser**

```bash
docker compose up -d --build
```

Su http://localhost:8080/users, verifica: il conteggio in testa; la
riga dell'admin attivo con badge "Attivo" e solo "Elimina"; il click su
"Aggiungi admin" che apre il campo email col focus; l'invito che crea
una riga "In attesa" con il link visibile; "Copia link invito" che
mostra "Copiato"; "Elimina" che apre la modale e non `window.confirm`.
Prova anche la larghezza a 375px (una riga non deve mai far scorrere la
pagina in orizzontale).

- [ ] **Step 5: Commit**

```bash
git add frontend/src/app.css frontend/src/views/UsersView.vue
git commit -m "feat: rebuild the admins page as rows with inline invites"
```

---

### Task 5: Pagina pubblica di attivazione dell'invito

**Files:**
- Create: `frontend/src/views/InviteAcceptView.vue`
- Modify: `frontend/src/router/index.ts`
- Modify: `frontend/src/stores/auth.ts`

**Interfaces:**
- Consumes: `GET /api/invites/{token}`, `POST /api/invites/{token}`
  (Task 3); il formato del link prodotto dal Task 4
  (`/invito/<token>`).
- Produces: rotta nominata `invite-accept` su `/invito/:token`
  (`meta.public = true`) e l'action
  `useAuthStore().acceptInvite(token: string, password: string)`.

- [ ] **Step 1: Aggiungi l'action allo store auth**

In `frontend/src/stores/auth.ts`, dopo `bootstrap`:

```ts
    // Accettare un invito apre già la sessione lato server: qui non serve
    // altro che registrare l'utente, come fa bootstrap per il primo admin.
    async acceptInvite(token: string, password: string) {
      this.user = await api.post<CurrentUser>(`/invites/${token}`, { password })
      this.needsSetup = false
    },
```

- [ ] **Step 2: Crea la view**

`frontend/src/views/InviteAcceptView.vue`:

```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const token = String(route.params.token)
const state = ref<'loading' | 'ready' | 'invalid'>('loading')
const email = ref('')
const password = ref('')
const confirmation = ref('')
const error = ref('')
const saving = ref(false)

onMounted(async () => {
  try {
    // skipAuthRedirect non serve (un token invalido risponde 404, non 401) ma
    // vale come dichiarazione: questa pagina non deve mai finire su /login
    // per colpa del client.
    const invite = await api.get<{ email: string }>(`/invites/${token}`, { skipAuthRedirect: true })
    email.value = invite.email
    state.value = 'ready'
  } catch {
    state.value = 'invalid'
  }
})

async function submit() {
  error.value = ''
  if (password.value !== confirmation.value) {
    error.value = 'Le due password non coincidono.'
    return
  }
  saving.value = true
  try {
    await auth.acceptInvite(token, password.value)
    router.push({ name: 'users' })
  } catch (e) {
    const message = (e as Error).message
    error.value =
      message === 'invite not found'
        ? 'Questo invito non è più valido: la password è già stata impostata.'
        : message === 'password must be at least 8 characters'
          ? 'La password deve essere di almeno 8 caratteri.'
          : message
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="auth-page">
    <template v-if="state === 'loading'">
      <h1>Invito</h1>
      <p>Verifica del link…</p>
    </template>

    <template v-else-if="state === 'invalid'">
      <h1>Invito non valido</h1>
      <p>
        Questo link non funziona più: la password è già stata impostata, oppure
        l'invito è stato annullato. Chiedi a chi ti ha invitato di generarne uno nuovo.
      </p>
      <p><router-link to="/login">Vai all'accesso</router-link></p>
    </template>

    <template v-else>
      <h1>Imposta la tua password</h1>
      <form @submit.prevent="submit">
        <label>
          Email
          <input :value="email" type="email" disabled />
        </label>
        <label>
          Password
          <input v-model="password" type="password" required minlength="8" autocomplete="new-password" />
        </label>
        <label>
          Conferma password
          <input v-model="confirmation" type="password" required minlength="8" autocomplete="new-password" />
        </label>
        <button type="submit" :disabled="saving">
          {{ saving ? 'Salvataggio…' : 'Imposta password e accedi' }}
        </button>
        <p v-if="error" class="error">{{ error }}</p>
      </form>
    </template>
  </div>
</template>
```

- [ ] **Step 3: Registra la rotta pubblica**

In `frontend/src/router/index.ts`, aggiungi l'import accanto agli altri:

```ts
import InviteAcceptView from '../views/InviteAcceptView.vue'
```

e la rotta fra quelle pubbliche, dopo `manage-booking`:

```ts
    {
      path: '/invito/:token',
      name: 'invite-accept',
      component: InviteAcceptView,
      meta: { public: true },
    },
```

- [ ] **Step 4: Verifica il build**

```bash
cd frontend && npm run build
```

Atteso: nessun errore. Se `vue-tsc` protesta su `acceptInvite`, l'action
è stata messa fuori dal blocco `actions` dello store.

- [ ] **Step 5: Prova il giro completo in browser**

```bash
docker compose up -d --build
```

1. Su `/users` invita `prova@example.com` e copia il link.
2. Apri il link in una finestra anonima: deve mostrare
   `prova@example.com` in sola lettura e i due campi password.
3. Password diverse → "Le due password non coincidono."
4. Password uguali (≥8) → arrivi su `/users` già autenticato come il
   nuovo admin.
5. Ricarica lo stesso link → "Invito non valido".
6. Su `/users` la riga di `prova@example.com` è ora "Attivo", senza
   link né bottone di copia.
7. Prova la pagina a 375px di larghezza: è la superficie che qualcuno
   aprirà dal telefono, dal messaggio che gli hai mandato.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/views/InviteAcceptView.vue \
        frontend/src/router/index.ts frontend/src/stores/auth.ts
git commit -m "feat: add the public invite page where a new admin sets a password"
```

---

### Task 6: Documentazione e passata finale

**Files:**
- Modify: `DESIGN.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: tutto ciò che i Task 1–5 hanno prodotto.
- Produces: niente codice.

- [ ] **Step 1: Documenta i pattern visivi nuovi**

In `DESIGN.md`, nella sezione dei componenti, aggiungi una voce per la
**riga admin** (pedina con iniziale, badge di stato, azioni a destra,
riga del link in mono rientrata) e per lo **slot "aggiungi" a riga
piena** (`.admin-add`), notando che è la variante orizzontale della
cella `.add-card` del catalogo: stesso tratteggio che si chiude al
passaggio. Segnala anche la regola di colore: nelle liste il rosso è
"Rimuovi", quindi le azioni non distruttive passano all'oro
(`.btn-invite`).

- [ ] **Step 2: Aggiorna il README**

Nella descrizione delle funzionalità visibili, sostituisci l'eventuale
riferimento alla creazione di un admin con email e password con il
flusso nuovo: un admin invita un collega inserendo solo l'email, copia
il link generato e lo recapita a mano; il collega apre il link e sceglie
la propria password, che chi lo ha invitato non conosce. Precisa che
il link non scade e resta valido finché non viene usato o l'invito non
viene eliminato.

- [ ] **Step 3: Verifica finale, entrambe le suite**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./...
```

```bash
cd frontend && npm run build
```

Atteso: `ok` su tutti i package Go e build frontend pulito. Riporta
l'output: nessuna affermazione di "funziona" senza di quello.

- [ ] **Step 4: Commit**

```bash
git add DESIGN.md README.md
git commit -m "docs: document admin invites and the admin row pattern"
```

- [ ] **Step 5: Passata `impeccable`**

Lancia `/impeccable polish` sulle due superfici toccate — `/users` e
`/invito/:token` — e applica quello che ne esce. È l'ultimo task
previsto dalle regole di progetto per qualsiasi intervento sul
frontend, non un extra opzionale.

- [ ] **Step 6: Commit della passata**

```bash
git add frontend/src DESIGN.md
git commit -m "fix: polish the admins page and the invite page"
```

**Mai `git add -A` o `git add .` in questo repo durante questo lavoro:**
il working tree contiene lavoro non committato di un'altra sessione (una
feature "immagine evento" che tocca `backend/internal/events/`,
`backend/internal/httpapi/events_*` e `router.go`). Aggiungi sempre
percorsi espliciti.

---

## Note per chi esegue

- **Non toccare `frontend/src/api/client.ts`.** Il redirect automatico
  al login scatta solo sui 401: gli endpoint dell'invito rispondono
  404, quindi la pagina pubblica funziona così com'è.
- **Il primo admin resta il bootstrap**, con email *e* password: quel
  flusso non cambia, e `users.Store.Create` continua a servire solo a
  lui.
- **Il token in chiaro nel DB è una scelta, non una dimenticanza**:
  serve a rendere il link ricopiabile a distanza di giorni. Se in
  futuro si vuole l'hash, serve anche un bottone "rigenera invito".
