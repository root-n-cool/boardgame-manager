# Fase 4 — Punteggi e Classifiche Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Aggiungere al BoardGames Manager l'inserimento del punteggio a fine partita (tramite `booking_code`) e la classifica pubblica storica per gioco, semplificando contestualmente lookup/cancellazione prenotazione a identificarsi con il solo `booking_code`.

**Architecture:** Il pacchetto Go `internal/events` si estende con `MatchResult`/`MatchPlayerScore` (stesso pattern di `Booking`, dato che un match result appartiene sempre a un booking). Un nuovo pacchetto `internal/leaderboard`, con un'unica responsabilità — aggregare tutti i match result di un gioco attraverso eventi diversi — espone la classifica. `internal/httpapi` guadagna un endpoint pubblico rate-limited per il submit del punteggio, un endpoint pubblico per la classifica, un endpoint admin di sola lettura per i risultati di un evento, e modifica i due endpoint di lookup/cancellazione esistenti per non richiedere più l'email.

**Tech Stack:** Go (`database/sql`), Vue 3 + TypeScript (già in uso), SQLite.

**Spec:** `docs/superpowers/specs/2026-09-02-punteggi-classifiche-design.md`

## Global Constraints

- Ogni comando Go va eseguito via Docker (il toolchain locale è rotto su questa macchina):
  ```
  docker run --rm -v "$(pwd)/backend":/app \
    -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
    -w /app golang:1.25 go test ./...
  ```
  Riusa sempre i volumi nominati `bgm-gomodcache`/`bgm-gocache`. Il frontend (`npm`) funziona regolarmente in locale, non serve Docker.
- Lookup e cancellazione prenotazione (e il nuovo submit punteggio) si identificano con il solo `booking_code` — l'email non è più richiesta come credenziale, resta solo un dato raccolto alla prenotazione.
- Un booking ha al più un `MatchResult` (`booking_id UNIQUE`): il submit del punteggio è sempre un upsert (sostituisce i giocatori/punteggi precedenti), mai un insert duplicato — "il punteggio è sempre modificabile" si implementa così.
- Solo un booking con `status='active'` può inserire/modificare un punteggio.
- Minimo 1 giocatore per match, massimo 20 (limite anti-abuso). Vince chi ha il punteggio più alto nel match; in caso di pareggio al massimo, tutti i giocatori con quel punteggio sono vincitori — nessuno spareggio.
- Le risposte di errore su credenziali booking (lookup/cancel/submit punteggio) restano generiche (404 "prenotazione non trovata") per non facilitare l'enumerazione; uno stato non più attivo (booking cancellato) è invece un 409 esplicito, distinto dal codice sbagliato.
- La classifica aggrega per `player_name` normalizzato (lowercase + trim) — nessun registro giocatori separato.
- L'admin ha solo una vista di sola lettura sui risultati di un evento in questa fase — nessuna modifica/cancellazione da backoffice.

---

### Task 1: Migrazione punteggi

**Files:**
- Create: `backend/internal/db/migrations/0005_scores.sql`

**Interfaces:**
- Produces: tabelle `match_results(id, booking_id UNIQUE REFERENCES bookings(id) ON DELETE CASCADE, submitted_at)` e `match_player_scores(id, match_result_id REFERENCES match_results(id) ON DELETE CASCADE, player_name, score)`.

- [ ] **Step 1: Scrivere la migrazione**

`backend/internal/db/migrations/0005_scores.sql`:
```sql
CREATE TABLE match_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    booking_id INTEGER NOT NULL UNIQUE REFERENCES bookings(id) ON DELETE CASCADE,
    submitted_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE match_player_scores (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    match_result_id INTEGER NOT NULL REFERENCES match_results(id) ON DELETE CASCADE,
    player_name TEXT NOT NULL,
    score INTEGER NOT NULL
);
```

- [ ] **Step 2: Verificare che le migrazioni esistenti continuino a passare**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/db/... -v
```
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/db/migrations/0005_scores.sql
git commit -m "feat: add match_results and match_player_scores tables"
```

---

### Task 2: Semplificare LookupBooking/CancelBooking al solo booking_code

**Files:**
- Modify: `backend/internal/events/bookings.go`
- Modify: `backend/internal/events/bookings_test.go`

**Interfaces:**
- Produces: `func (s *Store) LookupBooking(ctx context.Context, code string) (Booking, error)`, `func (s *Store) CancelBooking(ctx context.Context, id int64, code string) (Booking, error)` — firme senza `email`, usate da tutte le fasi successive.

- [ ] **Step 1: Aggiornare `LookupBooking`**

In `backend/internal/events/bookings.go`, sostituire:
```go
func (s *Store) LookupBooking(ctx context.Context, email, code string) (Booking, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	var b Booking
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, event_id, event_game_id, participant_name, participant_email, participant_phone, booking_code, status, created_at
		 FROM bookings WHERE booking_code = ? AND status = 'active'`, code,
	).Scan(&b.ID, &b.EventID, &b.EventGameID, &b.ParticipantName, &b.ParticipantEmail, &b.ParticipantPhone, &b.BookingCode, &b.Status, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Booking{}, ErrInvalidBookingCredentials
	}
	if err != nil {
		return Booking{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(b.ParticipantEmail), strings.TrimSpace(email)) {
		return Booking{}, ErrInvalidBookingCredentials
	}
	b.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return b, nil
}
```
con:
```go
func (s *Store) LookupBooking(ctx context.Context, code string) (Booking, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	var b Booking
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, event_id, event_game_id, participant_name, participant_email, participant_phone, booking_code, status, created_at
		 FROM bookings WHERE booking_code = ? AND status = 'active'`, code,
	).Scan(&b.ID, &b.EventID, &b.EventGameID, &b.ParticipantName, &b.ParticipantEmail, &b.ParticipantPhone, &b.BookingCode, &b.Status, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Booking{}, ErrInvalidBookingCredentials
	}
	if err != nil {
		return Booking{}, err
	}
	b.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return b, nil
}
```

- [ ] **Step 2: Aggiornare `CancelBooking`**

Sostituire:
```go
func (s *Store) CancelBooking(ctx context.Context, id int64, email, code string) (Booking, error) {
	b, err := s.getBookingByID(ctx, id)
	if err != nil {
		return Booking{}, err
	}
	if b.Status != BookingStatusActive ||
		strings.ToUpper(strings.TrimSpace(code)) != b.BookingCode ||
		!strings.EqualFold(strings.TrimSpace(b.ParticipantEmail), strings.TrimSpace(email)) {
		return Booking{}, ErrInvalidBookingCredentials
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE bookings SET status = ? WHERE id = ?`, BookingStatusCancelled, id); err != nil {
		return Booking{}, err
	}
	b.Status = BookingStatusCancelled
	return b, nil
}
```
con:
```go
func (s *Store) CancelBooking(ctx context.Context, id int64, code string) (Booking, error) {
	b, err := s.getBookingByID(ctx, id)
	if err != nil {
		return Booking{}, err
	}
	if b.Status != BookingStatusActive || strings.ToUpper(strings.TrimSpace(code)) != b.BookingCode {
		return Booking{}, ErrInvalidBookingCredentials
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE bookings SET status = ? WHERE id = ?`, BookingStatusCancelled, id); err != nil {
		return Booking{}, err
	}
	b.Status = BookingStatusCancelled
	return b, nil
}
```

- [ ] **Step 3: Eseguire i test — devono fallire a compilare**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/events/... -v
```
Expected: FAIL — `bookings_test.go` chiama ancora `LookupBooking`/`CancelBooking` con 3/4 argomenti (troppi argomenti nella chiamata).

- [ ] **Step 4: Aggiornare le chiamate in `bookings_test.go`**

In `backend/internal/events/bookings_test.go`, aggiungere `"strings"` agli import esistenti (`context`, `errors`, `testing`, `time`, `boardgames-manager/internal/events`).

Sostituire `TestCreateBooking_AllowsSamePhoneAfterCancellation`, cambiando solo la riga della cancel:
```go
	if _, err := eventStore.CancelBooking(ctx, first.ID, first.ParticipantEmail, first.BookingCode); err != nil {
		t.Fatalf("cancel: %v", err)
	}
```
in:
```go
	if _, err := eventStore.CancelBooking(ctx, first.ID, first.BookingCode); err != nil {
		t.Fatalf("cancel: %v", err)
	}
```

Sostituire l'intera funzione `TestLookupBooking_FindsActiveBookingByEmailAndCode` con:
```go
func TestLookupBooking_FindsActiveBookingByCode(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	created, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	found, err := eventStore.LookupBooking(ctx, strings.ToLower(created.BookingCode))
	if err != nil {
		t.Fatalf("lookup with lowercase code: %v", err)
	}
	if found.ID != created.ID {
		t.Fatalf("expected booking %d, got %d", created.ID, found.ID)
	}
}
```

Sostituire l'intera funzione `TestLookupBooking_WrongEmailOrCodeReturnsGenericError` con:
```go
func TestLookupBooking_WrongCodeReturnsGenericError(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	if _, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now); err != nil {
		t.Fatalf("create booking: %v", err)
	}

	if _, err := eventStore.LookupBooking(ctx, "WRONGCOD"); !errors.Is(err, events.ErrInvalidBookingCredentials) {
		t.Fatalf("expected ErrInvalidBookingCredentials for wrong code, got %v", err)
	}
}
```

In `TestLookupBooking_CancelledBookingIsNotFound`, sostituire:
```go
	if _, err := eventStore.CancelBooking(ctx, created.ID, created.ParticipantEmail, created.BookingCode); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	if _, err := eventStore.LookupBooking(ctx, "mario@example.com", created.BookingCode); !errors.Is(err, events.ErrInvalidBookingCredentials) {
		t.Fatalf("expected a cancelled booking to be unreachable by lookup, got %v", err)
	}
```
con:
```go
	if _, err := eventStore.CancelBooking(ctx, created.ID, created.BookingCode); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	if _, err := eventStore.LookupBooking(ctx, created.BookingCode); !errors.Is(err, events.ErrInvalidBookingCredentials) {
		t.Fatalf("expected a cancelled booking to be unreachable by lookup, got %v", err)
	}
```

Sostituire l'intera funzione `TestCancelBooking_RejectsWrongCredentials` con:
```go
func TestCancelBooking_RejectsWrongCode(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	created, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	_, err = eventStore.CancelBooking(ctx, created.ID, "WRONGCOD")
	if !errors.Is(err, events.ErrInvalidBookingCredentials) {
		t.Fatalf("expected ErrInvalidBookingCredentials, got %v", err)
	}

	remaining, err := eventStore.RemainingCapacity(ctx, eventGames[0].ID)
	if err != nil {
		t.Fatalf("remaining: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("a rejected cancel must not free up capacity; expected 0, got %d", remaining)
	}
}
```

In `TestListBookingsForEvent_ReturnsOnlyActiveWithGameName`, sostituire:
```go
	if _, err := eventStore.CancelBooking(ctx, toCancel.ID, toCancel.ParticipantEmail, toCancel.BookingCode); err != nil {
		t.Fatalf("cancel: %v", err)
	}
```
con:
```go
	if _, err := eventStore.CancelBooking(ctx, toCancel.ID, toCancel.BookingCode); err != nil {
		t.Fatalf("cancel: %v", err)
	}
```

- [ ] **Step 5: Eseguire i test — devono passare**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/events/... -v
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/events/bookings.go backend/internal/events/bookings_test.go
git commit -m "feat: identify bookings by booking_code alone, drop email requirement"
```

---

### Task 3: Semplificare gli handler HTTP lookup/cancel

**Files:**
- Modify: `backend/internal/httpapi/events_bookings_handlers.go`
- Modify: `backend/internal/httpapi/events_bookings_handlers_test.go`

**Interfaces:**
- Consumes: `s.Events.LookupBooking(ctx, code)`, `s.Events.CancelBooking(ctx, id, code)` (Task 2).
- Produces: `type bookingCodeRequest struct { BookingCode string }`, usata anche dal submit punteggio (Task 6).

- [ ] **Step 1: Aggiornare gli handler**

In `backend/internal/httpapi/events_bookings_handlers.go`, sostituire:
```go
type bookingCredentialsRequest struct {
	Email       string `json:"email"`
	BookingCode string `json:"bookingCode"`
}

func (s *Server) lookupBookingHandler(w http.ResponseWriter, r *http.Request) {
	var req bookingCredentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.BookingCode == "" {
		writeError(w, http.StatusBadRequest, "email and bookingCode are required")
		return
	}
	booking, err := s.Events.LookupBooking(r.Context(), req.Email, req.BookingCode)
```
con:
```go
type bookingCodeRequest struct {
	BookingCode string `json:"bookingCode"`
}

func (s *Server) lookupBookingHandler(w http.ResponseWriter, r *http.Request) {
	var req bookingCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BookingCode == "" {
		writeError(w, http.StatusBadRequest, "bookingCode is required")
		return
	}
	booking, err := s.Events.LookupBooking(r.Context(), req.BookingCode)
```

Sostituire (nel `cancelBookingHandler`):
```go
	var req bookingCredentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.BookingCode == "" {
		writeError(w, http.StatusBadRequest, "email and bookingCode are required")
		return
	}
	booking, err := s.Events.CancelBooking(r.Context(), id, req.Email, req.BookingCode)
```
con:
```go
	var req bookingCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BookingCode == "" {
		writeError(w, http.StatusBadRequest, "bookingCode is required")
		return
	}
	booking, err := s.Events.CancelBooking(r.Context(), id, req.BookingCode)
```

- [ ] **Step 2: Aggiornare i test esistenti**

In `backend/internal/httpapi/events_bookings_handlers_test.go`, in `TestLookupAndCancelBooking_FullFlow` sostituire entrambi:
```go
	lookupPayload, _ := json.Marshal(map[string]string{"email": "mario@example.com", "bookingCode": created.BookingCode})
```
```go
	cancelPayload, _ := json.Marshal(map[string]string{"email": "mario@example.com", "bookingCode": created.BookingCode})
```
con:
```go
	lookupPayload, _ := json.Marshal(map[string]string{"bookingCode": created.BookingCode})
```
```go
	cancelPayload, _ := json.Marshal(map[string]string{"bookingCode": created.BookingCode})
```

Rinominare `TestLookupBooking_WrongCredentialsReturns404` in `TestLookupBooking_WrongCodeReturns404` e sostituire:
```go
	payload, _ := json.Marshal(map[string]string{"email": "nobody@example.com", "bookingCode": "AAAAAAAA"})
```
con:
```go
	payload, _ := json.Marshal(map[string]string{"bookingCode": "AAAAAAAA"})
```

- [ ] **Step 3: Eseguire i test**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/... -v
```
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/httpapi/events_bookings_handlers.go backend/internal/httpapi/events_bookings_handlers_test.go
git commit -m "feat: drop email requirement from booking lookup/cancel endpoints"
```

---

### Task 4: Dominio MatchResult — submit e lettura

**Files:**
- Create: `backend/internal/events/matches.go`
- Create: `backend/internal/events/matches_test.go`

**Interfaces:**
- Consumes: `s.getBookingByID` (privato, già in `bookings.go`), `queryer` (interfaccia già in `store.go`), `ErrInvalidBookingCredentials`, `BookingStatusActive` (già in `bookings.go`).
- Produces:
  ```go
  type PlayerScore struct { Name string; Score int }
  type MatchResult struct { ID int64; BookingID int64; SubmittedAt time.Time; Players []PlayerScore }
  var ErrEmptyPlayers = errors.New("at least one player is required")
  var ErrBookingNotActive = errors.New("booking is not active")
  func (s *Store) SubmitMatchResult(ctx context.Context, bookingID int64, code string, players []PlayerScore) (MatchResult, error)
  func (s *Store) GetMatchResultForBooking(ctx context.Context, bookingID int64) (*MatchResult, error)
  ```
  usate da `internal/httpapi` (Task 5, 6) e da `internal/leaderboard` indirettamente tramite le tabelle che questo task popola.

- [ ] **Step 1: Scrivere i test (falliranno finché non esiste `matches.go`)**

`backend/internal/events/matches_test.go`:
```go
package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"boardgames-manager/internal/events"
)

func TestSubmitMatchResult_CreatesAndReturnsPlayers(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	result, err := eventStore.SubmitMatchResult(ctx, booking.ID, booking.BookingCode,
		[]events.PlayerScore{{Name: "Mario", Score: 42}, {Name: "Luigi", Score: 30}})
	if err != nil {
		t.Fatalf("submit match result: %v", err)
	}
	if result.BookingID != booking.ID {
		t.Fatalf("expected booking id %d, got %d", booking.ID, result.BookingID)
	}
	if len(result.Players) != 2 || result.Players[0].Name != "Mario" || result.Players[0].Score != 42 {
		t.Fatalf("unexpected players: %+v", result.Players)
	}
}

func TestSubmitMatchResult_ResubmittingReplacesPlayersWithoutDuplicating(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	if _, err := eventStore.SubmitMatchResult(ctx, booking.ID, booking.BookingCode,
		[]events.PlayerScore{{Name: "Mario", Score: 10}}); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	result, err := eventStore.SubmitMatchResult(ctx, booking.ID, booking.BookingCode,
		[]events.PlayerScore{{Name: "Mario", Score: 50}, {Name: "Luigi", Score: 20}})
	if err != nil {
		t.Fatalf("second submit: %v", err)
	}
	if len(result.Players) != 2 {
		t.Fatalf("expected the resubmission to replace players, not accumulate them; got %+v", result.Players)
	}

	fetched, err := eventStore.GetMatchResultForBooking(ctx, booking.ID)
	if err != nil {
		t.Fatalf("get match result: %v", err)
	}
	if fetched == nil || len(fetched.Players) != 2 || fetched.Players[0].Score != 50 {
		t.Fatalf("unexpected stored match result: %+v", fetched)
	}
}

func TestSubmitMatchResult_RejectsWrongCode(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	_, err = eventStore.SubmitMatchResult(ctx, booking.ID, "WRONGCOD", []events.PlayerScore{{Name: "Mario", Score: 10}})
	if !errors.Is(err, events.ErrInvalidBookingCredentials) {
		t.Fatalf("expected ErrInvalidBookingCredentials, got %v", err)
	}
}

func TestSubmitMatchResult_RejectsCancelledBooking(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}
	if _, err := eventStore.CancelBooking(ctx, booking.ID, booking.BookingCode); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	_, err = eventStore.SubmitMatchResult(ctx, booking.ID, booking.BookingCode, []events.PlayerScore{{Name: "Mario", Score: 10}})
	if !errors.Is(err, events.ErrBookingNotActive) {
		t.Fatalf("expected ErrBookingNotActive, got %v", err)
	}
}

func TestSubmitMatchResult_RejectsEmptyPlayers(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	_, err = eventStore.SubmitMatchResult(ctx, booking.ID, booking.BookingCode, nil)
	if !errors.Is(err, events.ErrEmptyPlayers) {
		t.Fatalf("expected ErrEmptyPlayers, got %v", err)
	}
}

func TestGetMatchResultForBooking_ReturnsNilWhenNoneSubmitted(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	found, err := eventStore.GetMatchResultForBooking(ctx, booking.ID)
	if err != nil {
		t.Fatalf("get match result: %v", err)
	}
	if found != nil {
		t.Fatalf("expected nil, got %+v", found)
	}
}
```

- [ ] **Step 2: Eseguire i test — devono fallire a compilare**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/events/... -v
```
Expected: FAIL — `undefined: events.PlayerScore` (o simili).

- [ ] **Step 3: Implementare `matches.go`**

`backend/internal/events/matches.go`:
```go
package events

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type PlayerScore struct {
	Name  string
	Score int
}

type MatchResult struct {
	ID          int64
	BookingID   int64
	SubmittedAt time.Time
	Players     []PlayerScore
}

var (
	ErrEmptyPlayers     = errors.New("at least one player is required")
	ErrBookingNotActive = errors.New("booking is not active")
)

// SubmitMatchResult creates or replaces the MatchResult for a booking. A
// booking can only ever have one MatchResult (booking_id is UNIQUE):
// calling this again for the same booking replaces the previously
// submitted players instead of creating a duplicate — this is how "il
// punteggio è sempre modificabile" (design spec) is implemented.
func (s *Store) SubmitMatchResult(ctx context.Context, bookingID int64, code string, players []PlayerScore) (MatchResult, error) {
	if len(players) == 0 {
		return MatchResult{}, ErrEmptyPlayers
	}
	b, err := s.getBookingByID(ctx, bookingID)
	if err != nil {
		return MatchResult{}, err
	}
	if strings.ToUpper(strings.TrimSpace(code)) != b.BookingCode {
		return MatchResult{}, ErrInvalidBookingCredentials
	}
	if b.Status != BookingStatusActive {
		return MatchResult{}, ErrBookingNotActive
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MatchResult{}, err
	}
	defer tx.Rollback()

	var matchResultID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM match_results WHERE booking_id = ?`, bookingID).Scan(&matchResultID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		res, err := tx.ExecContext(ctx, `INSERT INTO match_results (booking_id) VALUES (?)`, bookingID)
		if err != nil {
			return MatchResult{}, err
		}
		matchResultID, err = res.LastInsertId()
		if err != nil {
			return MatchResult{}, err
		}
	case err != nil:
		return MatchResult{}, err
	default:
		if _, err := tx.ExecContext(ctx, `UPDATE match_results SET submitted_at = datetime('now') WHERE id = ?`, matchResultID); err != nil {
			return MatchResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM match_player_scores WHERE match_result_id = ?`, matchResultID); err != nil {
			return MatchResult{}, err
		}
	}

	for _, p := range players {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO match_player_scores (match_result_id, player_name, score) VALUES (?, ?, ?)`,
			matchResultID, p.Name, p.Score,
		); err != nil {
			return MatchResult{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return MatchResult{}, err
	}
	return s.getMatchResultByID(ctx, matchResultID)
}

func (s *Store) getMatchResultByID(ctx context.Context, id int64) (MatchResult, error) {
	var m MatchResult
	var submittedAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, booking_id, submitted_at FROM match_results WHERE id = ?`, id,
	).Scan(&m.ID, &m.BookingID, &submittedAt)
	if err != nil {
		return MatchResult{}, err
	}
	m.SubmittedAt, _ = time.Parse("2006-01-02 15:04:05", submittedAt)

	players, err := playersForMatchResult(ctx, s.db, id)
	if err != nil {
		return MatchResult{}, err
	}
	m.Players = players
	return m, nil
}

func playersForMatchResult(ctx context.Context, q queryer, matchResultID int64) ([]PlayerScore, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT player_name, score FROM match_player_scores WHERE match_result_id = ? ORDER BY id`, matchResultID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PlayerScore
	for rows.Next() {
		var p PlayerScore
		if err := rows.Scan(&p.Name, &p.Score); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetMatchResultForBooking returns nil (with no error) if the booking has
// no MatchResult yet — "not played yet" is a normal, expected state here,
// unlike ErrNotFound elsewhere in this package.
func (s *Store) GetMatchResultForBooking(ctx context.Context, bookingID int64) (*MatchResult, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM match_results WHERE booking_id = ?`, bookingID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m, err := s.getMatchResultByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &m, nil
}
```

- [ ] **Step 4: Eseguire i test — devono passare**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/events/... -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/events/matches.go backend/internal/events/matches_test.go
git commit -m "feat: add MatchResult domain with upsert-on-resubmit semantics"
```

---

### Task 5: Includere il punteggio nella risposta di dettaglio prenotazione

**Files:**
- Modify: `backend/internal/httpapi/events_responses.go`
- Modify: `backend/internal/httpapi/events_bookings_handlers_test.go`

**Interfaces:**
- Consumes: `events.PlayerScore`, `events.MatchResult`, `s.Events.GetMatchResultForBooking` (Task 4).
- Produces: `toPlayerScores([]events.PlayerScore) []map[string]any`, `toMatchResultResponse(events.MatchResult) map[string]any` — riusate da Task 6 e 7. Il JSON di `toBookingDetailResponse` guadagna la chiave `"matchResult"`: `null` se non ancora inserito, altrimenti `{"players": [{"name": "...", "score": 0}]}`.

- [ ] **Step 1: Aggiungere gli helper e il campo `matchResult`**

In `backend/internal/httpapi/events_responses.go`, aggiungere in fondo al file:
```go
func toPlayerScores(players []events.PlayerScore) []map[string]any {
	out := make([]map[string]any, 0, len(players))
	for _, p := range players {
		out = append(out, map[string]any{"name": p.Name, "score": p.Score})
	}
	return out
}

func toMatchResultResponse(m events.MatchResult) map[string]any {
	return map[string]any{"players": toPlayerScores(m.Players)}
}
```

Sostituire il corpo di `toBookingDetailResponse`:
```go
func (s *Server) toBookingDetailResponse(ctx context.Context, b events.Booking) (map[string]any, error) {
	resp := toBookingResponse(b)
	event, err := s.Events.GetEvent(ctx, b.EventID)
	if err != nil {
		return nil, err
	}
	eventGame, err := s.Events.GetEventGame(ctx, b.EventGameID)
	if err != nil {
		return nil, err
	}
	game, err := s.Games.GetGame(ctx, eventGame.GameID)
	if err != nil {
		return nil, err
	}
	resp["eventTitle"] = event.Title
	resp["eventDate"] = event.EventDate
	resp["startTime"] = event.StartTime
	resp["gameName"] = game.Name
	return resp, nil
}
```
con:
```go
func (s *Server) toBookingDetailResponse(ctx context.Context, b events.Booking) (map[string]any, error) {
	resp := toBookingResponse(b)
	event, err := s.Events.GetEvent(ctx, b.EventID)
	if err != nil {
		return nil, err
	}
	eventGame, err := s.Events.GetEventGame(ctx, b.EventGameID)
	if err != nil {
		return nil, err
	}
	game, err := s.Games.GetGame(ctx, eventGame.GameID)
	if err != nil {
		return nil, err
	}
	resp["eventTitle"] = event.Title
	resp["eventDate"] = event.EventDate
	resp["startTime"] = event.StartTime
	resp["gameName"] = game.Name

	matchResult, err := s.Events.GetMatchResultForBooking(ctx, b.ID)
	if err != nil {
		return nil, err
	}
	if matchResult == nil {
		resp["matchResult"] = nil
	} else {
		resp["matchResult"] = toMatchResultResponse(*matchResult)
	}
	return resp, nil
}
```

- [ ] **Step 2: Aggiungere un test per il campo `matchResult`**

In `backend/internal/httpapi/events_bookings_handlers_test.go`, aggiungere:
```go
func TestLookupBooking_IncludesNullMatchResultWhenNoneSubmitted(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)

	createPayload, _ := json.Marshal(map[string]any{
		"eventGameId": eventGames[0].ID, "participantName": "Mario Rossi",
		"participantEmail": "mario@example.com", "participantPhone": "3331234567",
	})
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), bytes.NewReader(createPayload)))
	var created struct {
		BookingCode string `json:"bookingCode"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	lookupPayload, _ := json.Marshal(map[string]string{"bookingCode": created.BookingCode})
	lookupRec := httptest.NewRecorder()
	router.ServeHTTP(lookupRec, httptest.NewRequest(http.MethodPost, "/api/bookings/lookup", bytes.NewReader(lookupPayload)))
	var body map[string]any
	if err := json.NewDecoder(lookupRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode lookup: %v", err)
	}
	if v, ok := body["matchResult"]; !ok || v != nil {
		t.Fatalf("expected matchResult to be present and null, got %#v", v)
	}
}
```

- [ ] **Step 3: Eseguire i test**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/... -v
```
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/httpapi/events_responses.go backend/internal/httpapi/events_bookings_handlers_test.go
git commit -m "feat: include matchResult in booking lookup/cancel responses"
```

---

### Task 6: Endpoint pubblico — submit punteggio

**Files:**
- Create: `backend/internal/httpapi/match_result_handlers.go`
- Create: `backend/internal/httpapi/match_result_handlers_test.go`
- Modify: `backend/internal/httpapi/router.go`

**Interfaces:**
- Consumes: `s.Events.SubmitMatchResult` (Task 4), `toMatchResultResponse` (Task 5), `parseIDParam`, `writeError`/`writeJSON`.
- Produces: `POST /api/bookings/{id}/match-result` (rate-limited come lookup/cancel); `createTestBooking(t, router, eventID, eventGameID) testBooking{ID, BookingCode}` — helper di test riusato dai Task 7 e 9.

- [ ] **Step 1: Scrivere il test (fallirà: route inesistente)**

`backend/internal/httpapi/match_result_handlers_test.go`:
```go
package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/httpapi"
)

type testBooking struct {
	ID          int64  `json:"id"`
	BookingCode string `json:"bookingCode"`
}

func createTestBooking(t *testing.T, router http.Handler, eventID, eventGameID int64) testBooking {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{
		"eventGameId": eventGameID, "participantName": "Mario Rossi",
		"participantEmail": "mario@example.com", "participantPhone": "3331234567",
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), bytes.NewReader(payload)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create booking: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body testBooking
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body
}

func TestSubmitMatchResult_Succeeds(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	booking := createTestBooking(t, router, eventID, eventGames[0].ID)

	payload, _ := json.Marshal(map[string]any{
		"bookingCode": booking.BookingCode,
		"players": []map[string]any{
			{"name": "Mario", "score": 42},
			{"name": "Luigi", "score": 30},
		},
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/match-result", booking.ID), bytes.NewReader(payload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Players []struct {
			Name  string `json:"name"`
			Score int    `json:"score"`
		} `json:"players"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Players) != 2 || body.Players[0].Name != "Mario" || body.Players[0].Score != 42 {
		t.Fatalf("unexpected players: %+v", body.Players)
	}
}

func TestSubmitMatchResult_WrongCodeReturns404(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	booking := createTestBooking(t, router, eventID, eventGames[0].ID)

	payload, _ := json.Marshal(map[string]any{
		"bookingCode": "WRONGCOD",
		"players":     []map[string]any{{"name": "Mario", "score": 42}},
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/match-result", booking.ID), bytes.NewReader(payload)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestSubmitMatchResult_CancelledBookingReturns409(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	booking := createTestBooking(t, router, eventID, eventGames[0].ID)

	cancelPayload, _ := json.Marshal(map[string]string{"bookingCode": booking.BookingCode})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/cancel", booking.ID), bytes.NewReader(cancelPayload)))

	payload, _ := json.Marshal(map[string]any{
		"bookingCode": booking.BookingCode,
		"players":     []map[string]any{{"name": "Mario", "score": 42}},
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/match-result", booking.ID), bytes.NewReader(payload)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSubmitMatchResult_ResubmitReplacesPlayers(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	booking := createTestBooking(t, router, eventID, eventGames[0].ID)

	firstPayload, _ := json.Marshal(map[string]any{
		"bookingCode": booking.BookingCode,
		"players":     []map[string]any{{"name": "Mario", "score": 10}},
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/match-result", booking.ID), bytes.NewReader(firstPayload)))

	secondPayload, _ := json.Marshal(map[string]any{
		"bookingCode": booking.BookingCode,
		"players":     []map[string]any{{"name": "Mario", "score": 50}, {"name": "Luigi", "score": 20}},
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/match-result", booking.ID), bytes.NewReader(secondPayload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Players []struct {
			Name string `json:"name"`
		} `json:"players"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Players) != 2 {
		t.Fatalf("expected 2 players after resubmission, got %d", len(body.Players))
	}
}
```

- [ ] **Step 2: Eseguire i test — devono fallire a compilare**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/... -v
```
Expected: FAIL — `s.submitMatchResultHandler undefined`.

- [ ] **Step 3: Implementare l'handler**

`backend/internal/httpapi/match_result_handlers.go`:
```go
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"boardgames-manager/internal/events"
)

const maxPlayersPerMatch = 20

type playerScoreRequest struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
}

type matchResultRequest struct {
	BookingCode string               `json:"bookingCode"`
	Players     []playerScoreRequest `json:"players"`
}

func decodeMatchResultRequest(r *http.Request) ([]events.PlayerScore, string, bool) {
	var req matchResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, "", false
	}
	if req.BookingCode == "" || len(req.Players) == 0 || len(req.Players) > maxPlayersPerMatch {
		return nil, "", false
	}
	players := make([]events.PlayerScore, 0, len(req.Players))
	for _, p := range req.Players {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			return nil, "", false
		}
		players = append(players, events.PlayerScore{Name: name, Score: p.Score})
	}
	return players, req.BookingCode, true
}

func (s *Server) submitMatchResultHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid booking id")
		return
	}
	players, code, ok := decodeMatchResultRequest(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "bookingCode and at least one player with a name are required")
		return
	}
	result, err := s.Events.SubmitMatchResult(r.Context(), id, code, players)
	switch {
	case errors.Is(err, events.ErrNotFound), errors.Is(err, events.ErrInvalidBookingCredentials):
		writeError(w, http.StatusNotFound, "prenotazione non trovata")
	case errors.Is(err, events.ErrBookingNotActive):
		writeError(w, http.StatusConflict, "la prenotazione non è più attiva")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not submit match result")
	default:
		writeJSON(w, http.StatusOK, toMatchResultResponse(result))
	}
}
```

- [ ] **Step 4: Registrare la route**

In `backend/internal/httpapi/router.go`, aggiungere dopo la riga di `cancel`:
```go
	r.With(bookingCredentialsLimiter.middleware).Post("/api/bookings/{id}/cancel", s.cancelBookingHandler)
```
```go
	r.With(bookingCredentialsLimiter.middleware).Post("/api/bookings/{id}/match-result", s.submitMatchResultHandler)
```

- [ ] **Step 5: Eseguire i test — devono passare**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/... -v
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi/match_result_handlers.go backend/internal/httpapi/match_result_handlers_test.go backend/internal/httpapi/router.go
git commit -m "feat: add public match-result submission endpoint"
```

---

### Task 7: Vista admin di sola lettura sui risultati di un evento

**Files:**
- Modify: `backend/internal/events/matches.go`
- Modify: `backend/internal/httpapi/match_result_handlers.go`
- Modify: `backend/internal/httpapi/match_result_handlers_test.go`
- Modify: `backend/internal/httpapi/router.go`

**Interfaces:**
- Consumes: `toPlayerScores` (Task 5), `createTestBooking`, `bootstrapFirstAdmin` (helper già esistente in `auth_handlers_test.go`).
- Produces: `type BookingMatchResult struct { BookingID int64; ParticipantName string; GameName string; Players []PlayerScore }`, `func (s *Store) ListMatchResultsForEvent(ctx, eventID int64) ([]BookingMatchResult, error)`, route protetta `GET /api/events/{id}/match-results`.

- [ ] **Step 1: Scrivere i test**

In `backend/internal/httpapi/match_result_handlers_test.go`, aggiungere:
```go
func TestListEventMatchResults_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/events/1/match-results", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestListEventMatchResults_ReturnsSubmittedResults(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	booking := createTestBooking(t, router, eventID, eventGames[0].ID)

	submitPayload, _ := json.Marshal(map[string]any{
		"bookingCode": booking.BookingCode,
		"players":     []map[string]any{{"name": "Mario", "score": 42}},
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/match-result", booking.ID), bytes.NewReader(submitPayload)))

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/events/%d/match-results", eventID), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body []struct {
		GameName string `json:"gameName"`
		Players  []struct {
			Name  string `json:"name"`
			Score int    `json:"score"`
		} `json:"players"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 1 || body[0].GameName != "Catan" || len(body[0].Players) != 1 || body[0].Players[0].Name != "Mario" {
		t.Fatalf("unexpected body: %+v", body)
	}
}
```

- [ ] **Step 2: Eseguire i test — devono fallire a compilare**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/... -v
```
Expected: FAIL — `s.listEventMatchResultsHandler undefined`.

- [ ] **Step 3: Aggiungere `ListMatchResultsForEvent` al dominio**

In `backend/internal/events/matches.go`, aggiungere in fondo al file:
```go
type BookingMatchResult struct {
	BookingID       int64
	ParticipantName string
	GameName        string
	Players         []PlayerScore
}

// ListMatchResultsForEvent returns, for every booking of the event that has
// a MatchResult, the participant, the game and the players/scores
// submitted — read-only data for the admin event detail page.
func (s *Store) ListMatchResultsForEvent(ctx context.Context, eventID int64) ([]BookingMatchResult, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT b.id, b.participant_name, g.name, mps.player_name, mps.score
		 FROM bookings b
		 JOIN event_games eg ON b.event_game_id = eg.id
		 JOIN games g ON eg.game_id = g.id
		 JOIN match_results mr ON mr.booking_id = b.id
		 JOIN match_player_scores mps ON mps.match_result_id = mr.id
		 WHERE b.event_id = ?
		 ORDER BY b.id, mps.id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BookingMatchResult
	var current *BookingMatchResult
	for rows.Next() {
		var bookingID int64
		var participantName, gameName, playerName string
		var score int
		if err := rows.Scan(&bookingID, &participantName, &gameName, &playerName, &score); err != nil {
			return nil, err
		}
		if current == nil || current.BookingID != bookingID {
			out = append(out, BookingMatchResult{BookingID: bookingID, ParticipantName: participantName, GameName: gameName})
			current = &out[len(out)-1]
		}
		current.Players = append(current.Players, PlayerScore{Name: playerName, Score: score})
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Aggiungere l'handler**

In `backend/internal/httpapi/match_result_handlers.go`, aggiungere in fondo al file:
```go
func (s *Server) listEventMatchResultsHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	list, err := s.Events.ListMatchResultsForEvent(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list match results")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, m := range list {
		out = append(out, map[string]any{
			"bookingId": m.BookingID, "participantName": m.ParticipantName,
			"gameName": m.GameName, "players": toPlayerScores(m.Players),
		})
	}
	writeJSON(w, http.StatusOK, out)
}
```

- [ ] **Step 5: Registrare la route protetta**

In `backend/internal/httpapi/router.go`, nel blocco `protected`, dopo:
```go
		protected.Get("/api/events/{id}/bookings", s.listEventBookingsHandler)
```
aggiungere:
```go
		protected.Get("/api/events/{id}/match-results", s.listEventMatchResultsHandler)
```

- [ ] **Step 6: Eseguire i test — devono passare**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/... -v
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/events/matches.go backend/internal/httpapi/match_result_handlers.go backend/internal/httpapi/match_result_handlers_test.go backend/internal/httpapi/router.go
git commit -m "feat: add admin read-only match results endpoint"
```

---

### Task 8: Pacchetto leaderboard — aggregazione classifica

**Files:**
- Create: `backend/internal/leaderboard/leaderboard.go`
- Create: `backend/internal/leaderboard/leaderboard_test.go`

**Interfaces:**
- Consumes: tabelle `match_player_scores`/`match_results`/`bookings`/`event_games`/`events` (Task 1, 4); per i test, `events.Store`/`games.Store` (già esistenti) per creare i fixture.
- Produces:
  ```go
  type PlayerStats struct { Name string; GamesPlayed int; Wins int; AverageScore float64; TotalScore int }
  type PlayerResult struct { Name string; Score int; IsWinner bool }
  type MatchEntry struct { EventTitle string; EventDate string; StartTime string; Players []PlayerResult }
  type Leaderboard struct { Players []PlayerStats; Matches []MatchEntry }
  type Store struct{...}
  func NewStore(conn *sql.DB) *Store
  func (s *Store) GetLeaderboard(ctx context.Context, gameID int64) (Leaderboard, error)
  ```
  usate da `internal/httpapi` (Task 9).

- [ ] **Step 1: Scrivere i test**

`backend/internal/leaderboard/leaderboard_test.go`:
```go
package leaderboard_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"boardgames-manager/internal/db"
	"boardgames-manager/internal/events"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/leaderboard"
)

func newTestStores(t *testing.T) (*events.Store, *games.Store, *leaderboard.Store) {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return events.NewStore(conn), games.NewStore(conn), leaderboard.NewStore(conn)
}

func TestGetLeaderboard_AggregatesWinsAndAveragesAcrossEvents(t *testing.T) {
	eventStore, gameStore, lbStore := newTestStores(t)
	ctx := context.Background()

	game, err := gameStore.CreateGame(ctx, games.Game{Name: "Catan"})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}

	event1, err := eventStore.CreateEvent(ctx, "Serata 1", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: game.ID, Quantity: 2}})
	if err != nil {
		t.Fatalf("create event 1: %v", err)
	}
	event1Games, _ := eventStore.ListEventGames(ctx, event1.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	booking1, err := eventStore.CreateBooking(ctx, event1.ID, event1Games[0].ID, "Mario Rossi", "mario@example.com", "3331111111", now)
	if err != nil {
		t.Fatalf("create booking 1: %v", err)
	}
	if _, err := eventStore.SubmitMatchResult(ctx, booking1.ID, booking1.BookingCode,
		[]events.PlayerScore{{Name: "Mario", Score: 50}, {Name: "luigi", Score: 30}}); err != nil {
		t.Fatalf("submit match 1: %v", err)
	}

	event2, err := eventStore.CreateEvent(ctx, "Serata 2", nil, "2026-11-01", "20:00",
		[]events.EventGameInput{{GameID: game.ID, Quantity: 2}})
	if err != nil {
		t.Fatalf("create event 2: %v", err)
	}
	event2Games, _ := eventStore.ListEventGames(ctx, event2.ID)
	booking2, err := eventStore.CreateBooking(ctx, event2.ID, event2Games[0].ID, "Luigi Verdi", "luigi2@example.com", "3332222222", now)
	if err != nil {
		t.Fatalf("create booking 2: %v", err)
	}
	if _, err := eventStore.SubmitMatchResult(ctx, booking2.ID, booking2.BookingCode,
		[]events.PlayerScore{{Name: "Luigi", Score: 70}, {Name: "Mario", Score: 20}}); err != nil {
		t.Fatalf("submit match 2: %v", err)
	}

	lb, err := lbStore.GetLeaderboard(ctx, game.ID)
	if err != nil {
		t.Fatalf("get leaderboard: %v", err)
	}
	if len(lb.Matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(lb.Matches))
	}
	if len(lb.Players) != 2 {
		t.Fatalf("expected 2 distinct normalized players (mario/luigi case-insensitive), got %d: %+v", len(lb.Players), lb.Players)
	}

	byName := map[string]leaderboard.PlayerStats{}
	for _, p := range lb.Players {
		byName[strings.ToLower(p.Name)] = p
	}
	mario := byName["mario"]
	if mario.GamesPlayed != 2 || mario.Wins != 1 || mario.TotalScore != 70 {
		t.Fatalf("unexpected mario stats: %+v", mario)
	}
	luigi := byName["luigi"]
	if luigi.GamesPlayed != 2 || luigi.Wins != 1 || luigi.TotalScore != 100 {
		t.Fatalf("unexpected luigi stats: %+v", luigi)
	}

	// Ordered by wins desc, then average score desc; both have 1 win here, so
	// luigi (avg 50) must come before mario (avg 35).
	if lb.Players[0].Name != luigi.Name {
		t.Fatalf("expected luigi first (higher average with same wins), got %+v", lb.Players)
	}
}

func TestGetLeaderboard_TiedTopScoreBothCountAsWinners(t *testing.T) {
	eventStore, gameStore, lbStore := newTestStores(t)
	ctx := context.Background()
	game, err := gameStore.CreateGame(ctx, games.Game{Name: "Catan"})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	event, err := eventStore.CreateEvent(ctx, "Serata", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: game.ID, Quantity: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331111111", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}
	if _, err := eventStore.SubmitMatchResult(ctx, booking.ID, booking.BookingCode,
		[]events.PlayerScore{{Name: "Mario", Score: 40}, {Name: "Luigi", Score: 40}}); err != nil {
		t.Fatalf("submit match: %v", err)
	}

	lb, err := lbStore.GetLeaderboard(ctx, game.ID)
	if err != nil {
		t.Fatalf("get leaderboard: %v", err)
	}
	if len(lb.Matches) != 1 || len(lb.Matches[0].Players) != 2 {
		t.Fatalf("unexpected matches: %+v", lb.Matches)
	}
	for _, p := range lb.Matches[0].Players {
		if !p.IsWinner {
			t.Fatalf("expected both tied players to be winners, got %+v", lb.Matches[0].Players)
		}
	}
	for _, p := range lb.Players {
		if p.Wins != 1 {
			t.Fatalf("expected both players to have 1 win from the tie, got %+v", p)
		}
	}
}
```

- [ ] **Step 2: Eseguire i test — devono fallire a compilare**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/leaderboard/... -v
```
Expected: FAIL — pacchetto `leaderboard` non esiste.

- [ ] **Step 3: Implementare il pacchetto**

`backend/internal/leaderboard/leaderboard.go`:
```go
package leaderboard

import (
	"context"
	"database/sql"
	"sort"
	"strings"
)

type PlayerStats struct {
	Name         string
	GamesPlayed  int
	Wins         int
	AverageScore float64
	TotalScore   int
}

type PlayerResult struct {
	Name     string
	Score    int
	IsWinner bool
}

type MatchEntry struct {
	EventTitle string
	EventDate  string
	StartTime  string
	Players    []PlayerResult
}

type Leaderboard struct {
	Players []PlayerStats
	Matches []MatchEntry
}

type Store struct {
	db *sql.DB
}

func NewStore(conn *sql.DB) *Store {
	return &Store{db: conn}
}

type scoreRow struct {
	matchResultID int64
	eventTitle    string
	eventDate     string
	startTime     string
	playerName    string
	score         int
}

// GetLeaderboard aggregates every MatchResult ever submitted for a game,
// across all events it was played at. Winner and player-stats computation
// happens in Go rather than SQL: each match's winner(s) depend on comparing
// scores within that match only, which is awkward to express as a portable
// SQL aggregate.
func (s *Store) GetLeaderboard(ctx context.Context, gameID int64) (Leaderboard, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT mr.id, e.title, e.event_date, e.start_time, mps.player_name, mps.score
		 FROM match_player_scores mps
		 JOIN match_results mr ON mps.match_result_id = mr.id
		 JOIN bookings b ON mr.booking_id = b.id
		 JOIN event_games eg ON b.event_game_id = eg.id
		 JOIN events e ON b.event_id = e.id
		 WHERE eg.game_id = ?
		 ORDER BY mr.id, mps.id`, gameID)
	if err != nil {
		return Leaderboard{}, err
	}
	defer rows.Close()

	var raw []scoreRow
	for rows.Next() {
		var r scoreRow
		if err := rows.Scan(&r.matchResultID, &r.eventTitle, &r.eventDate, &r.startTime, &r.playerName, &r.score); err != nil {
			return Leaderboard{}, err
		}
		raw = append(raw, r)
	}
	if err := rows.Err(); err != nil {
		return Leaderboard{}, err
	}

	return buildLeaderboard(raw), nil
}

func buildLeaderboard(raw []scoreRow) Leaderboard {
	var matchOrder []int64
	matchRows := map[int64][]scoreRow{}
	matchInfo := map[int64]scoreRow{}
	for _, r := range raw {
		if _, seen := matchInfo[r.matchResultID]; !seen {
			matchOrder = append(matchOrder, r.matchResultID)
			matchInfo[r.matchResultID] = r
		}
		matchRows[r.matchResultID] = append(matchRows[r.matchResultID], r)
	}

	playerAgg := map[string]*PlayerStats{}
	var matches []MatchEntry
	for _, matchID := range matchOrder {
		playersInMatch := matchRows[matchID]
		maxScore := playersInMatch[0].score
		for _, p := range playersInMatch {
			if p.score > maxScore {
				maxScore = p.score
			}
		}

		entry := MatchEntry{
			EventTitle: matchInfo[matchID].eventTitle,
			EventDate:  matchInfo[matchID].eventDate,
			StartTime:  matchInfo[matchID].startTime,
		}
		for _, p := range playersInMatch {
			isWinner := p.score == maxScore
			entry.Players = append(entry.Players, PlayerResult{Name: p.playerName, Score: p.score, IsWinner: isWinner})

			key := strings.ToLower(strings.TrimSpace(p.playerName))
			agg, ok := playerAgg[key]
			if !ok {
				agg = &PlayerStats{Name: strings.TrimSpace(p.playerName)}
				playerAgg[key] = agg
			} else {
				agg.Name = strings.TrimSpace(p.playerName)
			}
			agg.GamesPlayed++
			agg.TotalScore += p.score
			if isWinner {
				agg.Wins++
			}
		}
		matches = append(matches, entry)
	}

	players := make([]PlayerStats, 0, len(playerAgg))
	for _, agg := range playerAgg {
		if agg.GamesPlayed > 0 {
			agg.AverageScore = float64(agg.TotalScore) / float64(agg.GamesPlayed)
		}
		players = append(players, *agg)
	}
	sort.Slice(players, func(i, j int) bool {
		if players[i].Wins != players[j].Wins {
			return players[i].Wins > players[j].Wins
		}
		return players[i].AverageScore > players[j].AverageScore
	})

	return Leaderboard{Players: players, Matches: matches}
}
```

- [ ] **Step 4: Eseguire i test — devono passare**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/leaderboard/... -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/leaderboard/
git commit -m "feat: add leaderboard package aggregating match results per game"
```

---

### Task 9: Endpoint pubblico — classifica per gioco

**Files:**
- Modify: `backend/internal/httpapi/router.go`
- Modify: `backend/internal/httpapi/testhelpers_test.go`
- Modify: `backend/internal/httpapi/match_result_handlers.go`
- Modify: `backend/internal/httpapi/match_result_handlers_test.go`

**Interfaces:**
- Consumes: `leaderboard.Store`/`leaderboard.Leaderboard` (Task 8).
- Produces: `GET /api/games/{id}/leaderboard` (pubblico, nessuna auth), `Server.Leaderboard *leaderboard.Store`.

- [ ] **Step 1: Scrivere il test**

In `backend/internal/httpapi/match_result_handlers_test.go`, aggiungere:
```go
func TestGetLeaderboard_ReturnsAggregatedStats(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	booking := createTestBooking(t, router, eventID, eventGames[0].ID)

	submitPayload, _ := json.Marshal(map[string]any{
		"bookingCode": booking.BookingCode,
		"players": []map[string]any{
			{"name": "Mario", "score": 42},
			{"name": "Luigi", "score": 10},
		},
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/match-result", booking.ID), bytes.NewReader(submitPayload)))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/games/%d/leaderboard", gameID), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Players []struct {
			Name string `json:"name"`
			Wins int    `json:"wins"`
		} `json:"players"`
		Matches []struct {
			Players []struct {
				Name     string `json:"name"`
				IsWinner bool   `json:"isWinner"`
			} `json:"players"`
		} `json:"matches"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Players) != 2 || len(body.Matches) != 1 {
		t.Fatalf("unexpected body: %+v", body)
	}
}
```

- [ ] **Step 2: Eseguire i test — devono fallire a compilare**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/... -v
```
Expected: FAIL — `s.Leaderboard undefined` / `s.getLeaderboardHandler undefined`.

- [ ] **Step 3: Aggiungere `Leaderboard` al `Server` e alla route pubblica**

In `backend/internal/httpapi/router.go`, aggiungere l'import `"boardgames-manager/internal/leaderboard"` e il campo:
```go
type Server struct {
	Users       *users.Store
	Sessions    *auth.SessionStore
	Settings    *settings.Store
	Games       *games.Store
	Events      *events.Store
	Leaderboard *leaderboard.Store
	Storage     *storage.Store
	BGG         bgg.Client
}
```
Aggiungere la route pubblica dopo `getEventHandler`:
```go
	r.Get("/api/events/{id}", s.getEventHandler)
```
```go
	r.Get("/api/games/{id}/leaderboard", s.getLeaderboardHandler)
```

- [ ] **Step 4: Wire il nuovo store nei test helper**

In `backend/internal/httpapi/testhelpers_test.go`, aggiungere l'import `"boardgames-manager/internal/leaderboard"` e il campo nella struct literal:
```go
	return &httpapi.Server{
		Users:    users.NewStore(conn),
		Sessions: auth.NewSessionStore(conn),
		Settings: settings.NewStore(conn),
		Games:    games.NewStore(conn),
		Events:   events.NewStore(conn),
		Storage:  storage.NewStore(t.TempDir()),
		BGG:      &fakeBGGClient{},
	}, conn
```
con:
```go
	return &httpapi.Server{
		Users:       users.NewStore(conn),
		Sessions:    auth.NewSessionStore(conn),
		Settings:    settings.NewStore(conn),
		Games:       games.NewStore(conn),
		Events:      events.NewStore(conn),
		Leaderboard: leaderboard.NewStore(conn),
		Storage:     storage.NewStore(t.TempDir()),
		BGG:         &fakeBGGClient{},
	}, conn
```

- [ ] **Step 5: Aggiungere handler e response builder**

In `backend/internal/httpapi/match_result_handlers.go`, aggiungere `"boardgames-manager/internal/leaderboard"` agli import e, in fondo al file:
```go
func toLeaderboardResponse(lb leaderboard.Leaderboard) map[string]any {
	playerStats := make([]map[string]any, 0, len(lb.Players))
	for _, p := range lb.Players {
		playerStats = append(playerStats, map[string]any{
			"name": p.Name, "gamesPlayed": p.GamesPlayed, "wins": p.Wins,
			"averageScore": p.AverageScore, "totalScore": p.TotalScore,
		})
	}
	matches := make([]map[string]any, 0, len(lb.Matches))
	for _, m := range lb.Matches {
		matchPlayers := make([]map[string]any, 0, len(m.Players))
		for _, p := range m.Players {
			matchPlayers = append(matchPlayers, map[string]any{"name": p.Name, "score": p.Score, "isWinner": p.IsWinner})
		}
		matches = append(matches, map[string]any{
			"eventTitle": m.EventTitle, "eventDate": m.EventDate, "startTime": m.StartTime, "players": matchPlayers,
		})
	}
	return map[string]any{"players": playerStats, "matches": matches}
}

func (s *Server) getLeaderboardHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid game id")
		return
	}
	lb, err := s.Leaderboard.GetLeaderboard(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not build leaderboard")
		return
	}
	writeJSON(w, http.StatusOK, toLeaderboardResponse(lb))
}
```

- [ ] **Step 6: Eseguire tutti i test backend — devono passare**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./... -v
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/httpapi/router.go backend/internal/httpapi/testhelpers_test.go backend/internal/httpapi/match_result_handlers.go backend/internal/httpapi/match_result_handlers_test.go
git commit -m "feat: add public per-game leaderboard endpoint"
```

---

### Task 10: Frontend — ManageBookingView con inserimento punteggio

**Files:**
- Modify: `frontend/src/views/ManageBookingView.vue`

**Interfaces:**
- Consumes: `POST /api/bookings/lookup` `{bookingCode}` → `{..., matchResult: {players:[{name,score}]} | null}`, `POST /api/bookings/{id}/cancel` `{bookingCode}`, `POST /api/bookings/{id}/match-result` `{bookingCode, players}` (Task 3, 5, 6).

- [ ] **Step 1: Riscrivere il file**

`frontend/src/views/ManageBookingView.vue`:
```vue
<script setup lang="ts">
import { ref } from 'vue'
import { api } from '../api/client'
import PublicHeader from '../components/PublicHeader.vue'

interface PlayerScore {
  name: string
  score: number
}

interface BookingResult {
  id: number
  eventId: number
  eventGameId: number
  participantName: string
  bookingCode: string
  status: 'active' | 'cancelled'
  eventTitle: string
  eventDate: string
  startTime: string
  gameName: string
  matchResult: { players: PlayerScore[] } | null
}

const bookingCode = ref('')
const booking = ref<BookingResult | null>(null)
const error = ref('')
const cancelMessage = ref('')
const scoreError = ref('')
const scoreMessage = ref('')
const players = ref<PlayerScore[]>([{ name: '', score: 0 }])

async function lookup() {
  error.value = ''
  cancelMessage.value = ''
  scoreMessage.value = ''
  try {
    booking.value = await api.post<BookingResult>('/bookings/lookup', {
      bookingCode: bookingCode.value,
    })
    players.value = booking.value.matchResult
      ? booking.value.matchResult.players.map((p) => ({ ...p }))
      : [{ name: '', score: 0 }]
  } catch (e) {
    booking.value = null
    error.value = (e as Error).message
  }
}

async function cancel() {
  if (!booking.value) {
    return
  }
  error.value = ''
  try {
    booking.value = await api.post<BookingResult>(`/bookings/${booking.value.id}/cancel`, {
      bookingCode: bookingCode.value,
    })
    cancelMessage.value = 'Prenotazione annullata.'
  } catch (e) {
    error.value = (e as Error).message
  }
}

function addPlayerRow() {
  players.value.push({ name: '', score: 0 })
}

function removePlayerRow(index: number) {
  if (players.value.length > 1) {
    players.value.splice(index, 1)
  }
}

async function submitScore() {
  if (!booking.value) {
    return
  }
  scoreError.value = ''
  scoreMessage.value = ''
  try {
    const result = await api.post<{ players: PlayerScore[] }>(
      `/bookings/${booking.value.id}/match-result`,
      { bookingCode: bookingCode.value, players: players.value },
    )
    booking.value.matchResult = result
    scoreMessage.value = 'Punteggio salvato.'
  } catch (e) {
    scoreError.value = (e as Error).message
  }
}
</script>

<template>
  <div>
    <PublicHeader />
    <div class="public-page">
      <h1>Gestisci prenotazione</h1>

      <form @submit.prevent="lookup">
        <label>
          Codice prenotazione
          <input v-model="bookingCode" required />
        </label>
        <button type="submit">Cerca</button>
      </form>
      <p v-if="error" class="error">{{ error }}</p>

      <div v-if="booking">
        <p>
          Prenotazione per {{ booking.participantName }} — {{ booking.gameName }} —
          {{ booking.eventTitle }} ({{ booking.eventDate }} · {{ booking.startTime }}) —
          stato: {{ booking.status }}
        </p>
        <button v-if="booking.status === 'active'" type="button" @click="cancel">
          Annulla prenotazione
        </button>
        <p v-if="cancelMessage" class="success">{{ cancelMessage }}</p>

        <form v-if="booking.status === 'active'" @submit.prevent="submitScore">
          <h2>Punteggio finale</h2>
          <div v-for="(p, index) in players" :key="index" class="player-score-row">
            <input v-model="p.name" placeholder="Nome giocatore" required />
            <input v-model.number="p.score" type="number" required />
            <button type="button" @click="removePlayerRow(index)">Rimuovi</button>
          </div>
          <button type="button" @click="addPlayerRow">Aggiungi giocatore</button>
          <button type="submit">
            {{ booking.matchResult ? 'Aggiorna punteggio' : 'Invia punteggio' }}
          </button>
          <p v-if="scoreMessage" class="success">{{ scoreMessage }}</p>
          <p v-if="scoreError" class="error">{{ scoreError }}</p>
        </form>
      </div>
    </div>
  </div>
</template>
```

- [ ] **Step 2: Build del frontend**

Run:
```bash
cd frontend && npm run build
```
Expected: build senza errori TypeScript.

- [ ] **Step 3: Verifica manuale in browser**

Avviare backend (`docker run ... go run ./cmd/server` o equivalente già in uso nel progetto) e frontend (`npm run dev`), poi:
1. Prenotare un gioco da un evento pubblico, annotando il `booking_code`.
2. Su `/manage-booking`, cercare con il solo codice (senza email): la prenotazione deve apparire.
3. Inserire due giocatori con punteggio, cliccare "Invia punteggio": deve apparire "Punteggio salvato."
4. Rifare la ricerca con lo stesso codice: le righe punteggio devono essere precompilate e il bottone deve dire "Aggiorna punteggio".
5. Modificare un punteggio e reinviare: deve salvare senza duplicare righe.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/ManageBookingView.vue
git commit -m "feat: add match score entry to the manage-booking page, drop email field"
```

---

### Task 11: Frontend — pagina classifica per gioco

**Files:**
- Create: `frontend/src/views/GameLeaderboardView.vue`
- Modify: `frontend/src/router/index.ts`
- Modify: `frontend/src/views/GameDetailView.vue`

**Interfaces:**
- Consumes: `GET /api/games/{id}/leaderboard` (Task 9).
- Produces: route pubblica `/games/:id/leaderboard`.

- [ ] **Step 1: Creare la vista**

`frontend/src/views/GameLeaderboardView.vue`:
```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import PublicHeader from '../components/PublicHeader.vue'

interface PlayerStats {
  name: string
  gamesPlayed: number
  wins: number
  averageScore: number
  totalScore: number
}

interface MatchPlayer {
  name: string
  score: number
  isWinner: boolean
}

interface MatchEntry {
  eventTitle: string
  eventDate: string
  startTime: string
  players: MatchPlayer[]
}

interface LeaderboardResponse {
  players: PlayerStats[]
  matches: MatchEntry[]
}

const route = useRoute()
const gameId = route.params.id as string

const leaderboard = ref<LeaderboardResponse | null>(null)
const error = ref('')

onMounted(async () => {
  try {
    leaderboard.value = await api.get<LeaderboardResponse>(`/games/${gameId}/leaderboard`)
  } catch (e) {
    error.value = (e as Error).message
  }
})
</script>

<template>
  <div>
    <PublicHeader />
    <div class="public-page">
      <h1>Classifica</h1>
      <p v-if="error" class="error">{{ error }}</p>

      <table v-if="leaderboard && leaderboard.players.length > 0">
        <thead>
          <tr>
            <th>Giocatore</th>
            <th>Partite</th>
            <th>Vittorie</th>
            <th>Punteggio medio</th>
            <th>Punteggio totale</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in leaderboard.players" :key="p.name">
            <td>{{ p.name }}</td>
            <td>{{ p.gamesPlayed }}</td>
            <td>{{ p.wins }}</td>
            <td>{{ p.averageScore.toFixed(1) }}</td>
            <td>{{ p.totalScore }}</td>
          </tr>
        </tbody>
      </table>
      <p v-else-if="leaderboard">Nessun punteggio ancora registrato per questo gioco.</p>

      <h2 v-if="leaderboard && leaderboard.matches.length > 0">Storico partite</h2>
      <ul>
        <li v-for="(m, index) in leaderboard?.matches" :key="index">
          {{ m.eventTitle }} ({{ m.eventDate }} · {{ m.startTime }}):
          <span v-for="(p, pIndex) in m.players" :key="p.name">
            {{ p.name }} {{ p.score }}{{ p.isWinner ? ' 🏆' : '' }}{{ pIndex < m.players.length - 1 ? ', ' : '' }}
          </span>
        </li>
      </ul>
    </div>
  </div>
</template>
```

- [ ] **Step 2: Registrare la route**

In `frontend/src/router/index.ts`, aggiungere l'import:
```ts
import GameLeaderboardView from '../views/GameLeaderboardView.vue'
```
e la route, dopo `game-detail`:
```ts
    { path: '/games/:id', name: 'game-detail', component: GameDetailView, meta: { public: true } },
```
```ts
    { path: '/games/:id/leaderboard', name: 'game-leaderboard', component: GameLeaderboardView, meta: { public: true } },
```

- [ ] **Step 3: Aggiungere il link dalla scheda gioco**

In `frontend/src/views/GameDetailView.vue`, subito dopo:
```vue
      <p v-if="game.owner">Proprietario: {{ game.owner }}</p>
```
aggiungere:
```vue
      <p><router-link :to="`/games/${game.id}/leaderboard`">Classifica</router-link></p>
```

- [ ] **Step 4: Build del frontend**

Run:
```bash
cd frontend && npm run build
```
Expected: build senza errori.

- [ ] **Step 5: Verifica manuale in browser**

Da `/games/{id}` cliccare "Classifica" e verificare che la tabella aggregata e lo storico partite mostrino i dati inseriti nel Task 10 (Mario/Luigi con vittorie e punteggio medio corretti).

- [ ] **Step 6: Commit**

```bash
git add frontend/src/views/GameLeaderboardView.vue frontend/src/router/index.ts frontend/src/views/GameDetailView.vue
git commit -m "feat: add public per-game leaderboard page"
```

---

### Task 12: Frontend — sezione risultati inseriti nell'admin evento

**Files:**
- Modify: `frontend/src/views/EventAdminDetailView.vue`

**Interfaces:**
- Consumes: `GET /api/events/{id}/match-results` (Task 7).

- [ ] **Step 1: Aggiungere stato e caricamento dati**

In `frontend/src/views/EventAdminDetailView.vue`, aggiungere le interfacce dopo `BookingAdminInfo`:
```ts
interface MatchResultPlayer {
  name: string
  score: number
}

interface MatchResultAdminInfo {
  bookingId: number
  participantName: string
  gameName: string
  players: MatchResultPlayer[]
}
```
Aggiungere lo stato accanto a `bookings`:
```ts
const matchResults = ref<MatchResultAdminInfo[]>([])
```
In `load()`, dopo la riga `bookings.value = await api.get<BookingAdminInfo[]>(...)`, aggiungere:
```ts
  matchResults.value = await api.get<MatchResultAdminInfo[]>(`/events/${eventId}/match-results`)
```

- [ ] **Step 2: Aggiungere la sezione al template**

Dopo la lista `<ul>` delle prenotazioni attive, aggiungere:
```vue
    <h2>Risultati inseriti</h2>
    <p v-if="matchResults.length === 0">Nessun punteggio inserito ancora.</p>
    <ul>
      <li v-for="m in matchResults" :key="m.bookingId">
        {{ m.participantName }} — {{ m.gameName }}:
        <span v-for="(p, index) in m.players" :key="p.name">
          {{ p.name }} {{ p.score }}{{ index < m.players.length - 1 ? ', ' : '' }}
        </span>
      </li>
    </ul>
```

- [ ] **Step 3: Build del frontend**

Run:
```bash
cd frontend && npm run build
```
Expected: build senza errori.

- [ ] **Step 4: Verifica manuale in browser**

Da `/admin/events/{id}` (evento con almeno un punteggio inserito nel Task 10), verificare che la sezione "Risultati inseriti" mostri prenotante, gioco e punteggi.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/views/EventAdminDetailView.vue
git commit -m "feat: show submitted match results in the admin event detail page"
```

---

### Task 13: Verifica end-to-end finale

**Files:** nessuno (solo verifica)

- [ ] **Step 1: Suite backend completa**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./... -v
```
Expected: PASS su tutti i pacchetti.

- [ ] **Step 2: Build frontend**

Run:
```bash
cd frontend && npm run build
```
Expected: build senza errori.

- [ ] **Step 3: Flusso manuale completo in browser**

1. Creare un evento admin con un gioco (quantità 2).
2. Prenotare due tavoli distinti dello stesso gioco (due `booking_code` diversi).
3. Per ciascuna prenotazione, su `/manage-booking`, inserire un punteggio diverso.
4. Verificare `/games/{id}/leaderboard`: due match nello storico, giocatori aggregati con vittorie/medie corrette (usare nomi giocatore con casing diverso tra le due partite per verificare la normalizzazione).
5. Modificare il punteggio di una delle due prenotazioni e verificare che la classifica si aggiorni senza duplicare la partita.
6. Verificare `/admin/events/{id}`: la sezione "Risultati inseriti" mostra entrambi i risultati.
7. Cancellare una prenotazione (senza email, solo codice) e verificare che non sia più possibile inserirne il punteggio (409).

- [ ] **Step 4: Nessun commit — task di sola verifica**
