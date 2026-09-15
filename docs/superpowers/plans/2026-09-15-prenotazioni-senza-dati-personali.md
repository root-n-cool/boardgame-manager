# Prenotazioni senza dati personali persistiti — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop collecting the participant's phone number and stop persisting their email on a booking; replace the server-side "one booking per phone" rule with a client-side (localStorage) "you already booked this" reminder; require an explicit, server-verified terms/privacy consent before a booking can be created.

**Architecture:** One new forward-only migration drops `participant_phone` (and its unique index) and `participant_email` from `bookings`, and adds a `terms_accepted_at` column that takes the row's insert time as its default (mirroring `created_at`) — no Go code reads it back. `events.Store.CreateBooking` drops its `email`/`phone` parameters entirely. The HTTP layer accepts an optional email purely to pass, unpersisted, straight into the async confirmation-mail goroutine, and rejects any request missing `termsAccepted: true`. The cancellation-notice mail (`sendBookingCancelled`) is deleted outright, since without a persisted email there is no address left to send it to, in either the participant's own cancellation or the admin's. On the frontend, the booking form loses its phone field, makes email optional (with an explanatory hint) and gains a required terms/privacy checkbox; a new `frontend/src/utils/myBookings.ts` records confirmed bookings in `localStorage` so `EventDetailView.vue` can show a "Prenotato" chip (with "Annulla prenotazione" / "Aggiungi risultato" actions) instead of the "Prenota" button for a game this browser already booked. The admin event view and the loan desk lose the contact info / phone-prefill convenience that depended on the now-removed columns.

**Tech Stack:** Go 1.25 (chi router, modernc.org/sqlite, embedded SQL migrations), Vue 3 + `<script setup>` + TypeScript + Pinia/Vue Router, no frontend test runner (verification is `vue-tsc` via `npm run build` plus manual browser checks).

**Spec:** `docs/superpowers/specs/2026-09-15-prenotazioni-senza-dati-personali-design.md`

## Global Constraints

- Run every Go command in Docker, reusing the named volumes — never `go build`/`go test` on the host:
  ```bash
  docker run --rm -v "$(pwd)/backend:/app" \
    -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
    -w /app golang:1.25 go test ./...
  ```
- Migrations are forward-only: never edit a migration file that already exists (`0001`–`0019`); this feature only ever adds `0020_prenotazioni_senza_contatti.sql`.
- `npm run build` (from `frontend/`) runs on the host directly (no Docker) and also performs the TypeScript type-check via `vue-tsc -b`.
- No i18n: all UI strings are Italian, written directly in components.
- Keep dependencies at zero net new packages (Go or npm) — this feature needs none.
- Every task that touches `frontend/` ends with `npm run build` passing; every task that touches `backend/` ends with the relevant Go test command passing in Docker.
- The last task of this plan is a mandatory `/impeccable` pass on the changed frontend surface, per this repo's workflow rule for any frontend-touching change.

---

### Task 1: Migration `0020_prenotazioni_senza_contatti.sql`

**Files:**
- Create: `backend/internal/db/migrations/0020_prenotazioni_senza_contatti.sql`

**Interfaces:**
- Consumes: nothing (pure schema change).
- Produces: a `bookings` table without `participant_phone`/`participant_email` and without `idx_one_active_booking_per_phone_per_event`, with a new `terms_accepted_at TEXT NOT NULL` column (default `datetime('now')`) that later tasks depend on existing (even though no Go code reads it back).

- [ ] **Step 1: Write the migration file**

```sql
-- I contatti di chi prenota non servono più a niente lato server: il
-- telefono esisteva solo per il vincolo "una prenotazione attiva per
-- telefono per evento" (sostituito da un promemoria nel browser di chi
-- prenota, non più un vincolo server), e l'email non viene più
-- persistita — si usa solo al volo per la mail di conferma. Il
-- consenso a termini/privacy, prima non richiesto, ora è obbligatorio;
-- la prova resta sul booking come timestamp, con lo stesso default di
-- created_at (nessun codice Go la legge indietro).
DROP INDEX idx_one_active_booking_per_phone_per_event;
ALTER TABLE bookings DROP COLUMN participant_phone;
ALTER TABLE bookings DROP COLUMN participant_email;
ALTER TABLE bookings ADD COLUMN terms_accepted_at TEXT NOT NULL DEFAULT (datetime('now'));
```

- [ ] **Step 2: Verify the migration applies cleanly**

Run (this exercises every migration from `0001` through `0020` against a fresh in-memory DB, via any existing package test — `events` is a good one since it will fail loudly if the schema is broken):

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/events/... 2>&1 | head -50
```

Expected: the package still compiles fine (Go source doesn't reference SQL column names at compile time), but most tests fail at runtime with errors like `no such column: participant_email` — that is the **expected**, correct signal at this point: it means the migration dropped the columns successfully, and Task 2 (next) is exactly what updates the Go code to match. What must **not** appear is an error about the migration statements themselves — e.g. `near "DROP": syntax error`, `no such index: idx_one_active_booking_per_phone_per_event` (meaning the `DROP INDEX` line itself failed), or a migration-runner error on startup. If you see one of those instead of `no such column: participant_...`, the migration SQL is wrong; fix it before moving on.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/db/migrations/0020_prenotazioni_senza_contatti.sql
git commit -m "$(cat <<'EOF'
feat: drop booking phone/email columns, add terms consent timestamp

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Domain layer — `events` package and its consumers

**Files:**
- Modify: `backend/internal/events/bookings.go`
- Modify: `backend/internal/events/store.go` (only the `TestInsertBooking` helper and its doc comment, around line 613)
- Modify: `backend/internal/events/bookings_test.go`
- Modify: `backend/internal/events/matches_test.go`
- Modify: `backend/internal/leaderboard/leaderboard_test.go`

**Interfaces:**
- Consumes: the `bookings` table shape produced by Task 1.
- Produces: `func (s *Store) CreateBooking(ctx context.Context, eventID, eventGameID int64, name string, now time.Time) (Booking, error)` — dropped the `email, phone string` parameters that used to sit between `name` and `now`. `Booking` struct: `ID int64; EventID int64; EventGameID int64; ParticipantName string; BookingCode string; Status string; CreatedAt time.Time` (no `ParticipantEmail`/`ParticipantPhone`). `events.ErrDuplicatePhoneBooking` no longer exists — Task 3 must stop referencing it. `BookingWithGame` (embeds `Booking`) is unaffected beyond inheriting the struct change.

- [ ] **Step 1: Update the `Booking` struct and drop `ErrDuplicatePhoneBooking`**

In `backend/internal/events/bookings.go`, replace:

```go
type Booking struct {
	ID               int64
	EventID          int64
	EventGameID      int64
	ParticipantName  string
	ParticipantEmail string
	ParticipantPhone string
	BookingCode      string
	Status           string
	CreatedAt        time.Time
}
```

with:

```go
type Booking struct {
	ID              int64
	EventID         int64
	EventGameID     int64
	ParticipantName string
	BookingCode     string
	Status          string
	CreatedAt       time.Time
}
```

And replace:

```go
var (
	ErrEventAlreadyStarted       = errors.New("event already started")
	ErrGameSoldOut               = errors.New("game sold out")
	ErrDuplicatePhoneBooking     = errors.New("phone already has an active booking for this event")
	ErrInvalidBookingCredentials = errors.New("invalid email or booking code")
	ErrGameNotBookable           = errors.New("game is not bookable at this event")
)
```

with:

```go
var (
	ErrEventAlreadyStarted       = errors.New("event already started")
	ErrGameSoldOut               = errors.New("game sold out")
	ErrInvalidBookingCredentials = errors.New("invalid email or booking code")
	ErrGameNotBookable           = errors.New("game is not bookable at this event")
)
```

- [ ] **Step 2: Update `CreateBooking`**

Replace the entire function (from `func (s *Store) CreateBooking(...)` through its closing `}`) with:

```go
func (s *Store) CreateBooking(ctx context.Context, eventID, eventGameID int64, name string, now time.Time) (Booking, error) {
	event, err := s.GetEvent(ctx, eventID)
	if err != nil {
		return Booking{}, err
	}
	startsAt, err := time.Parse("2006-01-02 15:04", event.EventDate+" "+event.StartTime)
	if err != nil {
		return Booking{}, fmt.Errorf("parse event start: %w", err)
	}
	if !now.Before(startsAt) {
		return Booking{}, ErrEventAlreadyStarted
	}

	// La stessa lettura fa due lavori: dice se la copia appartiene
	// all'evento e se è prenotabile. Un gioco lasciato sul tavolo per chi
	// arriva senza prenotazione non deve poter essere prenotato nemmeno
	// da chi chiama l'endpoint a mano.
	var bookable int
	err = s.db.QueryRowContext(ctx,
		`SELECT bookable FROM event_games WHERE id = ? AND event_id = ?`, eventGameID, eventID,
	).Scan(&bookable)
	if errors.Is(err, sql.ErrNoRows) {
		return Booking{}, ErrNotFound
	}
	if err != nil {
		return Booking{}, err
	}
	if bookable == 0 {
		return Booking{}, ErrGameNotBookable
	}

	code, err := generateBookingCode()
	if err != nil {
		return Booking{}, err
	}

	// Single atomic statement: the WHERE clause re-checks capacity as part of
	// the same write, so SQLite's write-lock makes this race-safe against
	// concurrent bookings for the last remaining seat — no separate
	// check-then-insert window. terms_accepted_at takes the column's own
	// default (datetime('now')), exactly like created_at: the row is only
	// ever written after the handler has checked termsAccepted, so the
	// insert instant doubles as the consent instant.
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO bookings (event_id, event_game_id, participant_name, booking_code, status)
		 SELECT ?, ?, ?, ?, 'active'
		 WHERE (SELECT COUNT(*) FROM bookings WHERE event_game_id = ? AND status = 'active') <
		       (SELECT seats FROM event_games WHERE id = ?)`,
		eventID, eventGameID, name, code, eventGameID, eventGameID,
	)
	if err != nil {
		return Booking{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Booking{}, err
	}
	if affected == 0 {
		return Booking{}, ErrGameSoldOut
	}

	id, err := res.LastInsertId()
	if err != nil {
		return Booking{}, err
	}
	return s.getBookingByID(ctx, id)
}
```

- [ ] **Step 3: Update `getBookingByID` and `LookupBooking`**

Replace:

```go
func (s *Store) getBookingByID(ctx context.Context, id int64) (Booking, error) {
	var b Booking
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, event_id, event_game_id, participant_name, participant_email, participant_phone, booking_code, status, created_at
		 FROM bookings WHERE id = ?`, id,
	).Scan(&b.ID, &b.EventID, &b.EventGameID, &b.ParticipantName, &b.ParticipantEmail, &b.ParticipantPhone, &b.BookingCode, &b.Status, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Booking{}, ErrNotFound
	}
	if err != nil {
		return Booking{}, err
	}
	b.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return b, nil
}

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

with:

```go
func (s *Store) getBookingByID(ctx context.Context, id int64) (Booking, error) {
	var b Booking
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, event_id, event_game_id, participant_name, booking_code, status, created_at
		 FROM bookings WHERE id = ?`, id,
	).Scan(&b.ID, &b.EventID, &b.EventGameID, &b.ParticipantName, &b.BookingCode, &b.Status, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Booking{}, ErrNotFound
	}
	if err != nil {
		return Booking{}, err
	}
	b.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return b, nil
}

func (s *Store) LookupBooking(ctx context.Context, code string) (Booking, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	var b Booking
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, event_id, event_game_id, participant_name, booking_code, status, created_at
		 FROM bookings WHERE booking_code = ? AND status = 'active'`, code,
	).Scan(&b.ID, &b.EventID, &b.EventGameID, &b.ParticipantName, &b.BookingCode, &b.Status, &createdAt)
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

- [ ] **Step 4: Update `ListBookingsForEvent`**

Replace:

```go
func (s *Store) ListBookingsForEvent(ctx context.Context, eventID int64) ([]BookingWithGame, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT b.id, b.event_id, b.event_game_id, b.participant_name, b.participant_email, b.participant_phone,
		        b.booking_code, b.status, b.created_at, g.id, g.name, eg.copy_index, eg.seats
		 FROM bookings b
		 JOIN event_games eg ON b.event_game_id = eg.id
		 JOIN games g ON eg.game_id = g.id
		 WHERE b.event_id = ? AND b.status = 'active'
		 ORDER BY eg.game_id, eg.copy_index, b.created_at`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BookingWithGame
	for rows.Next() {
		var bg BookingWithGame
		var createdAt string
		if err := rows.Scan(&bg.ID, &bg.EventID, &bg.EventGameID, &bg.ParticipantName, &bg.ParticipantEmail,
			&bg.ParticipantPhone, &bg.BookingCode, &bg.Status, &createdAt, &bg.GameID, &bg.GameName,
			&bg.CopyIndex, &bg.Seats); err != nil {
			return nil, err
		}
		bg.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		out = append(out, bg)
	}
	return out, rows.Err()
}
```

with:

```go
func (s *Store) ListBookingsForEvent(ctx context.Context, eventID int64) ([]BookingWithGame, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT b.id, b.event_id, b.event_game_id, b.participant_name,
		        b.booking_code, b.status, b.created_at, g.id, g.name, eg.copy_index, eg.seats
		 FROM bookings b
		 JOIN event_games eg ON b.event_game_id = eg.id
		 JOIN games g ON eg.game_id = g.id
		 WHERE b.event_id = ? AND b.status = 'active'
		 ORDER BY eg.game_id, eg.copy_index, b.created_at`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BookingWithGame
	for rows.Next() {
		var bg BookingWithGame
		var createdAt string
		if err := rows.Scan(&bg.ID, &bg.EventID, &bg.EventGameID, &bg.ParticipantName,
			&bg.BookingCode, &bg.Status, &createdAt, &bg.GameID, &bg.GameName,
			&bg.CopyIndex, &bg.Seats); err != nil {
			return nil, err
		}
		bg.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		out = append(out, bg)
	}
	return out, rows.Err()
}
```

- [ ] **Step 5: Update the `TestInsertBooking` fixture helper**

In `backend/internal/events/store.go`, replace:

```go
// TestInsertBooking writes a booking row directly, bypassing all of
// CreateBooking's validation. It exists only so tests in this package (and
// the bookings/lookup/cancel tests added in later tasks) can set up booking
// fixtures without a circular dependency on CreateBooking's own tests.
func (s *Store) TestInsertBooking(eventID, eventGameID int64, status string) error {
	// Each call must generate a distinct booking_code and participant_phone.
	// The counter increment ensures uniqueness even when called multiple times with
	// identical eventGameID and status parameters. Without it, the formula
	// (eventGameID*10+len(status)) would collide, violating:
	// (a) booking_code UNIQUE constraint, and
	// (b) idx_one_active_booking_per_phone_per_event partial unique index.
	counter := atomic.AddInt64(&testBookingCounter, 1)
	code := fmt.Sprintf("TEST%04d%d", eventGameID*10+int64(len(status)), counter)
	phone := fmt.Sprintf("TEST%04d%d", eventGameID*10+int64(len(status)), counter)
	_, err := s.db.Exec(
		`INSERT INTO bookings (event_id, event_game_id, participant_name, participant_email, participant_phone, booking_code, status)
		 VALUES (?, ?, 'Test Participant', 'test@example.com', ?, ?, ?)`,
		eventID, eventGameID, phone, code, status,
	)
	return err
}
```

with:

```go
// TestInsertBooking writes a booking row directly, bypassing all of
// CreateBooking's validation. It exists only so tests in this package (and
// the bookings/lookup/cancel tests added in later tasks) can set up booking
// fixtures without a circular dependency on CreateBooking's own tests.
func (s *Store) TestInsertBooking(eventID, eventGameID int64, status string) error {
	// Each call must generate a distinct booking_code: the counter increment
	// ensures uniqueness even when called multiple times with identical
	// eventGameID and status parameters, which would otherwise collide on
	// the booking_code UNIQUE constraint.
	counter := atomic.AddInt64(&testBookingCounter, 1)
	code := fmt.Sprintf("TEST%04d%d", eventGameID*10+int64(len(status)), counter)
	_, err := s.db.Exec(
		`INSERT INTO bookings (event_id, event_game_id, participant_name, booking_code, status)
		 VALUES (?, ?, 'Test Participant', ?, ?)`,
		eventID, eventGameID, code, status,
	)
	return err
}
```

- [ ] **Step 6: Delete the four tests that assert on the removed phone constraint**

In `backend/internal/events/bookings_test.go`, delete these four functions in their entirety (each from its `func Test...(t *testing.T) {` line through its closing `}`, plus the blank line that follows):

1. `TestCreateBooking_RejectsDuplicatePhoneForSameEvent`:

```go
func TestCreateBooking_RejectsDuplicatePhoneForSameEvent(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	// Plenty of seats on this one copy so the duplicate-phone rule is
	// exercised on its own, not masked by the copy selling out.
	gameID := mustCreateGameWithSeats(t, gameStore, "Catan", 5)
	event, err := eventStore.CreateEvent(ctx, events.EventInput{
		Title:     "Serata giochi",
		EventDate: "2026-10-01",
		StartTime: "20:00",
		Games:     []events.EventGameInput{{GameID: gameID, Copies: 1}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	if _, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now); err != nil {
		t.Fatalf("first booking: %v", err)
	}
	_, err = eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario2@example.com", "3331234567", now)
	if !errors.Is(err, events.ErrDuplicatePhoneBooking) {
		t.Fatalf("expected ErrDuplicatePhoneBooking, got %v", err)
	}
}
```

2. `TestCreateBooking_AllowsSamePhoneAfterCancellation`:

```go
func TestCreateBooking_AllowsSamePhoneAfterCancellation(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, events.EventInput{
		Title:     "Serata giochi",
		EventDate: "2026-10-01",
		StartTime: "20:00",
		Games:     []events.EventGameInput{{GameID: gameID, Copies: 5}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	first, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("first booking: %v", err)
	}
	if _, err := eventStore.CancelBooking(ctx, first.ID, first.BookingCode); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	if _, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now); err != nil {
		t.Fatalf("expected the same phone to be able to book again after cancelling, got %v", err)
	}
}
```

3. `TestCreateBooking_PhoneConstraintHoldsInsideATable` (including its trailing blank line and the comment block that immediately follows it, which explains the *next* deleted test):

```go
func TestCreateBooking_PhoneConstraintHoldsInsideATable(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGameWithSeats(t, gameStore, "D&D", 5)
	event := mustCreateEvent(t, eventStore, "Serata", "2026-10-01", "20:00", gameID)
	eventGames, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	if _, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
		"Mario", "mario@example.com", "3331111111", now); err != nil {
		t.Fatalf("first booking: %v", err)
	}
	_, err = eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
		"Mario di nuovo", "mario@example.com", "3331111111", now)
	if !errors.Is(err, events.ErrDuplicatePhoneBooking) {
		t.Fatalf("expected ErrDuplicatePhoneBooking, got %v", err)
	}
}

// TestCreateBooking_PhoneConstraintHoldsAcrossCopiesOfTheSameEvent pins the
// actual shape of the partial unique index:
// idx_one_active_booking_per_phone_per_event is scoped to (event_id, phone),
// tables included, not to event_game_id. Two copies of the *same* game are
// used on purpose rather than two different games: that keeps event_game_id
// as the only thing that differs between the two bookings, so a regression
// that accidentally scoped the constraint to event_game_id instead of
// event_id — which would let one phone hold a seat at every table of an
// evening, one booking per table — cannot hide behind a difference in
// game_id.
```

4. `TestCreateBooking_PhoneConstraintHoldsAcrossCopiesOfTheSameEvent`:

```go
func TestCreateBooking_PhoneConstraintHoldsAcrossCopiesOfTheSameEvent(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGameWithSeats(t, gameStore, "D&D", 5)
	event, err := eventStore.CreateEvent(ctx, events.EventInput{
		Title:     "Serata",
		EventDate: "2026-10-01",
		StartTime: "20:00",
		Games:     []events.EventGameInput{{GameID: gameID, Copies: 2}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	if _, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
		"Mario", "mario@example.com", "3331111111", now); err != nil {
		t.Fatalf("first table booking: %v", err)
	}
	_, err = eventStore.CreateBooking(ctx, event.ID, eventGames[1].ID,
		"Mario di nuovo", "mario@example.com", "3331111111", now)
	if !errors.Is(err, events.ErrDuplicatePhoneBooking) {
		t.Fatalf("expected ErrDuplicatePhoneBooking on the second table, got %v", err)
	}
}
```

- [ ] **Step 7: Compile and mechanically fix every remaining call site**

Run:

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go build ./... 2>&1
```

This fails with a `too many arguments in call to ...CreateBooking` (or similar) error for every remaining call site in `backend/internal/events/bookings_test.go`, `backend/internal/events/matches_test.go`, and `backend/internal/leaderboard/leaderboard_test.go`. For every flagged line, delete the two arguments that come right after the participant-name argument and right before the trailing `now` argument (the removed email and phone) — nothing else about the call changes. Three shapes appear; apply the same rule to each:

Single-line, literal strings (most common shape, e.g. `bookings_test.go`):
```go
// before
booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
// after
booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", now)
```

Two-line, literal strings (e.g. `bookings_test.go`'s `TestCreateBooking_FillsAllSeatsOfATable`, `matches_test.go`'s `TestSubmitMatchResult_IsSharedByTheWholeTable`):
```go
// before
first, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
	"Mario", "mario@example.com", "3331111111", now)
// after
first, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
	"Mario", now)
```

Two-line, computed with `fmt.Sprintf` (e.g. `bookings_test.go`'s `TestCreateBooking_FillsAllSeatsOfATable` loop, `matches_test.go`'s `TestListMatchResultsForEvent_OneRowPerCopy`):
```go
// before
b, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
	fmt.Sprintf("Giocatore %d", i+1), fmt.Sprintf("p%d@example.com", i+1), phone, now)
// after
b, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID,
	fmt.Sprintf("Giocatore %d", i+1), now)
```
(and where the loop variable that supplied the phone, e.g. `for i, phone := range []string{...}`, is no longer used anywhere else in the loop body, simplify the range to drop it, e.g. `for i := range []string{"3330000001", "3330000002", "3330000003"}` — or just replace the slice with `for i := 0; i < 3; i++`, whichever reads more clearly at that call site.)

`leaderboard_test.go`'s four call sites (lines ~51, ~70, ~128, ~179 as of this writing) follow the same single-line and `fmt.Sprintf`-loop shapes above.

Re-run `go build ./...` after each file until it exits with no output (success). Do not use a blind find-and-replace across files — some of these strings share substrings (e.g. `mario@example.com` appears with different surrounding phones) and a global regex risks corrupting an unrelated line; fix each flagged compiler location individually.

- [ ] **Step 8: Run the affected package tests**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./internal/events/... ./internal/leaderboard/... -v 2>&1 | tail -100
```

Expected: all tests pass (`ok  	boardgames-manager/internal/events`, `ok  	boardgames-manager/internal/leaderboard`). If any test still references `ErrDuplicatePhoneBooking`, `ParticipantEmail`, or `ParticipantPhone`, it was missed in Step 6/7 — find it with:

```bash
grep -rn "ErrDuplicatePhoneBooking\|ParticipantEmail\|ParticipantPhone" backend/internal/events backend/internal/leaderboard
```

and fix it before proceeding.

- [ ] **Step 9: Commit**

```bash
git add backend/internal/events/bookings.go backend/internal/events/store.go \
  backend/internal/events/bookings_test.go backend/internal/events/matches_test.go \
  backend/internal/leaderboard/leaderboard_test.go
git commit -m "$(cat <<'EOF'
feat: drop phone/email from the booking domain layer

CreateBooking no longer takes an email or phone: the phone-based
duplicate-booking rule is gone (replaced client-side later in this
change), and the email is never persisted.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: HTTP layer — booking handlers, responses, and mail

**Files:**
- Modify: `backend/internal/httpapi/events_bookings_handlers.go`
- Modify: `backend/internal/httpapi/events_responses.go`
- Modify: `backend/internal/httpapi/loans_responses.go`
- Modify: `backend/internal/httpapi/mail.go`
- Modify: `backend/internal/httpapi/mail_templates.go`
- Modify: `backend/internal/httpapi/events_bookings_handlers_test.go`
- Modify: `backend/internal/httpapi/mail_templates_test.go`
- Modify: `backend/internal/httpapi/loans_handlers_test.go`
- Modify: `backend/internal/httpapi/events_handlers_test.go`
- Modify: `backend/internal/httpapi/ask_handler_test.go`
- Modify: `backend/internal/httpapi/match_result_handlers_test.go`

**Interfaces:**
- Consumes: `events.Store.CreateBooking(ctx, eventID, eventGameID int64, name string, now time.Time) (events.Booking, error)` from Task 2.
- Produces: `POST /api/events/{id}/bookings` now requires JSON body `{eventGameId, participantName, participantEmail?, termsAccepted}` (`participantPhone` no longer accepted/required; `participantEmail` optional; `termsAccepted` must be `true` or the request is rejected with 400). The create-booking response drops nothing new (still `{id, eventId, eventGameId, participantName, bookingCode, status, mailQueued}`), but `mailQueued` is now `true` only when SMTP is configured **and** an email was given. `GET /api/events/{id}/bookings` (admin) responses drop `participantEmail`/`participantPhone`. The loan-desk response's `activeBookings` rows drop `phone`. No booking cancellation (self or admin) sends an email any more.

- [ ] **Step 1: Update `createBookingRequest` and `createBookingHandler`**

In `backend/internal/httpapi/events_bookings_handlers.go`, replace:

```go
type createBookingRequest struct {
	EventGameID int64  `json:"eventGameId"`
	Name        string `json:"participantName"`
	Email       string `json:"participantEmail"`
	Phone       string `json:"participantPhone"`
}

func (s *Server) createBookingHandler(w http.ResponseWriter, r *http.Request) {
	eventID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	var req createBookingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Email == "" || req.Phone == "" {
		writeError(w, http.StatusBadRequest, "participantName, participantEmail and participantPhone are required")
		return
	}

	booking, err := s.Events.CreateBooking(r.Context(), eventID, req.EventGameID, req.Name, req.Email, req.Phone, time.Now())
	switch {
	case errors.Is(err, events.ErrNotFound):
		writeError(w, http.StatusNotFound, "event or game not found")
	case errors.Is(err, events.ErrEventAlreadyStarted):
		writeError(w, http.StatusConflict, "l'evento è già iniziato")
	case errors.Is(err, events.ErrGameSoldOut):
		writeError(w, http.StatusConflict, "non ci sono più posti prenotabili su questa copia")
	case errors.Is(err, events.ErrDuplicatePhoneBooking):
		writeError(w, http.StatusConflict, "hai già una prenotazione attiva per questo evento")
	case errors.Is(err, events.ErrGameNotBookable):
		writeError(w, http.StatusConflict, "questo gioco non è prenotabile: è a disposizione al tavolo")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not create booking")
	default:
		s.sendBookingConfirmation(r, booking)
		resp := toBookingResponse(booking)
		// mailQueued dice alla pagina se promettere una mail: senza SMTP
		// il codice a schermo è l'unica cosa che il partecipante si porta
		// via, e la pagina lo dice così com'è sempre stato.
		resp["mailQueued"] = s.mailEnabled(r.Context())
		writeJSON(w, http.StatusCreated, resp)
	}
}
```

with:

```go
type createBookingRequest struct {
	EventGameID   int64  `json:"eventGameId"`
	Name          string `json:"participantName"`
	Email         string `json:"participantEmail"`
	TermsAccepted bool   `json:"termsAccepted"`
}

func (s *Server) createBookingHandler(w http.ResponseWriter, r *http.Request) {
	eventID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	var req createBookingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || !req.TermsAccepted {
		writeError(w, http.StatusBadRequest, "participantName is required and termsAccepted must be true")
		return
	}

	booking, err := s.Events.CreateBooking(r.Context(), eventID, req.EventGameID, req.Name, time.Now())
	switch {
	case errors.Is(err, events.ErrNotFound):
		writeError(w, http.StatusNotFound, "event or game not found")
	case errors.Is(err, events.ErrEventAlreadyStarted):
		writeError(w, http.StatusConflict, "l'evento è già iniziato")
	case errors.Is(err, events.ErrGameSoldOut):
		writeError(w, http.StatusConflict, "non ci sono più posti prenotabili su questa copia")
	case errors.Is(err, events.ErrGameNotBookable):
		writeError(w, http.StatusConflict, "questo gioco non è prenotabile: è a disposizione al tavolo")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not create booking")
	default:
		s.sendBookingConfirmation(r, booking, req.Email)
		resp := toBookingResponse(booking)
		// mailQueued dice alla pagina se promettere una mail: vero solo se
		// SMTP è configurato E chi prenota ha lasciato un'email — altrimenti
		// non parte niente, e prometterla sarebbe peggio che non dirla.
		resp["mailQueued"] = s.mailEnabled(r.Context()) && req.Email != ""
		writeJSON(w, http.StatusCreated, resp)
	}
}
```

- [ ] **Step 2: Stop sending a cancellation mail**

Still in `events_bookings_handlers.go`, in `cancelBookingHandler`, remove this line (right after the successful `CancelBooking` call):

```go
	s.sendBookingCancelled(r, booking, false)
```

In `adminCancelBookingHandler`, remove this line and the comment directly above it:

```go
	// La prenotazione serve per sapere chi avvisare: è l'unico modo in cui
	// il partecipante scopre che il suo posto è stato liberato.
	s.sendBookingCancelled(r, booking, true)
```

- [ ] **Step 3: Update `toBookingAdminResponse`**

In `backend/internal/httpapi/events_responses.go`, replace:

```go
func toBookingAdminResponse(b events.BookingWithGame) map[string]any {
	return map[string]any{
		"id": b.ID, "eventGameId": b.EventGameID, "gameId": b.GameID, "gameName": b.GameName,
		"copyIndex": b.CopyIndex, "seats": b.Seats,
		"participantName": b.ParticipantName, "participantEmail": b.ParticipantEmail,
		"participantPhone": b.ParticipantPhone,
		"createdAt":        b.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
```

with:

```go
func toBookingAdminResponse(b events.BookingWithGame) map[string]any {
	return map[string]any{
		"id": b.ID, "eventGameId": b.EventGameID, "gameId": b.GameID, "gameName": b.GameName,
		"copyIndex": b.CopyIndex, "seats": b.Seats,
		"participantName": b.ParticipantName,
		"createdAt":       b.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
```

- [ ] **Step 4: Update the loan-desk response's booking rows**

In `backend/internal/httpapi/loans_responses.go`, replace:

```go
	bookingsByCopy := map[int64][]map[string]any{}
	for _, b := range bookings {
		bookingsByCopy[b.EventGameID] = append(bookingsByCopy[b.EventGameID], map[string]any{
			"id": b.ID, "name": b.ParticipantName, "phone": b.ParticipantPhone,
		})
	}
```

with:

```go
	bookingsByCopy := map[int64][]map[string]any{}
	for _, b := range bookings {
		bookingsByCopy[b.EventGameID] = append(bookingsByCopy[b.EventGameID], map[string]any{
			"id": b.ID, "name": b.ParticipantName,
		})
	}
```

- [ ] **Step 5: Update `mail.go` — `bookingMailDataFor` and `sendBookingConfirmation`, delete `sendBookingCancelled`**

Replace:

```go
	return bookingMailData{
		ParticipantName:  b.ParticipantName,
		ParticipantEmail: b.ParticipantEmail,
		BookingCode:      b.BookingCode,
		GameLabel:        label,
		EventTitle:       event.Title,
		EventDate:        event.EventDate,
		StartTime:        event.StartTime,
		EventID:          b.EventID,
		SharedTable:      eventGame.Seats > 1,
	}, nil
}
```

with:

```go
	return bookingMailData{
		ParticipantName: b.ParticipantName,
		BookingCode:     b.BookingCode,
		GameLabel:       label,
		EventTitle:      event.Title,
		EventDate:       event.EventDate,
		StartTime:       event.StartTime,
		EventID:         b.EventID,
		SharedTable:     eventGame.Seats > 1,
	}, nil
}
```

Replace the whole `sendBookingConfirmation` function through the whole `sendBookingCancelled` function (i.e. from the `// sendBookingConfirmation manda la conferma...` comment down to the closing `}` of `sendBookingCancelled`, which is the rest of the file) with just the updated confirmation function:

```go
// sendBookingConfirmation manda la conferma, o non fa niente se non c'è
// posta o non c'è un indirizzo a cui mandarla — l'email non si salva più sul
// booking, quindi arriva qui come parametro esplicito, letto dalla richiesta
// che ha appena creato la prenotazione.
//
// Raccoglie i dati in modo sincrono — servono il context della richiesta e
// il database — e spedisce in modo asincrono.
//
// Un errore nel raccogliere i dati non risale: la prenotazione è già
// fatta, e non mandare una mail è meglio che rispondere con un errore
// per qualcosa che è andato bene.
//
// Deriva subito un context senza cancellazione (context.WithoutCancel) e
// riusa r con quello: un partecipante che perde il segnale subito dopo che
// la prenotazione è stata scritta non deve perdere anche la mail col
// codice, che è esattamente quello che gli servirebbe al posto della
// risposta HTTP che non è arrivata. Solo la cancellazione è tolta: una
// scadenza a monte (se mai aggiunta) resterebbe valida. mailEnabled,
// mailSender, publicBaseURL (via r) e bookingMailDataFor leggono tutte da
// qui, così l'intera raccolta — non solo l'invio — sopravvive alla
// disconnessione del client.
func (s *Server) sendBookingConfirmation(r *http.Request, b events.Booking, participantEmail string) {
	r = r.WithContext(context.WithoutCancel(r.Context()))
	if !s.mailEnabled(r.Context()) || participantEmail == "" {
		return
	}
	data, err := s.bookingMailDataFor(r.Context(), b)
	if err != nil {
		log.Printf("mail: could not gather booking %d data: %v", b.ID, err)
		return
	}
	data.ParticipantEmail = participantEmail
	base := s.publicBaseURL(r)
	s.sendMailAsync(s.mailSender(r.Context()), bookingConfirmationMail(
		data,
		bookingManageURL(base, b.BookingCode),
		bookingScoreURL(base, b.BookingCode),
	))
}
```

(`bookingScoreURL` stays used, so no import becomes unused; `events` is still imported for `events.Booking`.)

- [ ] **Step 6: Update `mail_templates.go` — delete `bookingCancelledMail`, fix `smtpTestMail` copy**

Delete the entire `bookingCancelledMail` function:

```go
func bookingCancelledMail(d bookingMailData, eventURL string, byAdmin bool) mailer.Message {
	// Testo e HTML dicono la stessa frase: una variabile sola, così non
	// possono divergere a una modifica futura.
	opening := fmt.Sprintf("la tua prenotazione per %s è stata annullata, come hai chiesto.", d.GameLabel)
	closing := "Se hai cambiato idea puoi prenotare di nuovo, se restano posti."
	if byAdmin {
		opening = fmt.Sprintf("la tua prenotazione per %s è stata annullata dall'organizzazione.", d.GameLabel)
		closing = "Il posto è tornato libero: puoi prenotare un altro gioco della serata."
	}

	text := strings.Join([]string{
		fmt.Sprintf("Ciao %s,", d.ParticipantName),
		"",
		opening,
		"",
		"Evento:   " + d.EventTitle,
		"Data:     " + d.EventDate,
		"Gioco:    " + d.GameLabel,
		"",
		closing,
		eventURL,
		"",
		"Il codice " + d.BookingCode + " non è più valido.",
	}, "\n")

	facts := `<table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 0 18px;border-collapse:collapse;">` +
		mailFactRow("Evento", d.EventTitle) +
		mailFactRow("Data", d.EventDate) +
		mailFactRow("Gioco", d.GameLabel) +
		`</table>`

	body := mailParagraph(fmt.Sprintf("Ciao %s,", d.ParticipantName)) +
		mailParagraph(opening) +
		facts +
		mailParagraph(closing) +
		mailButton("Vedi la serata", eventURL) +
		mailNote("Il codice " + d.BookingCode + " non è più valido.")

	return mailer.Message{
		To:       d.ParticipantEmail,
		ToName:   d.ParticipantName,
		Subject:  fmt.Sprintf("Prenotazione annullata: %s — %s", d.GameLabel, d.EventDate),
		TextBody: text,
		HTMLBody: mailShell("Prenotazione annullata", body),
	}
}
```

Then, in `smtpTestMail`, replace both occurrences of the sentence (once in the `text` slice, once in the `body` paragraph — they must stay identical to each other):

```
"Da qui in poi partiranno da sole: l'invito di un amministratore, la conferma di una prenotazione con il codice e i link, e l'avviso di annullamento."
```

with:

```
"Da qui in poi partiranno da sole: l'invito di un amministratore e la conferma di una prenotazione con il codice e i link."
```

- [ ] **Step 7: Fix `backend/internal/httpapi/events_bookings_handlers_test.go`**

Every JSON payload map in this file that includes `"participantEmail": "...", "participantPhone": "...",` needs `"participantPhone": "..."` replaced with `"termsAccepted": true`. This exact two-key pattern (`"participantEmail": "<email>", "participantPhone": "<digits>",`) appears in the `map[string]any{...}` payload literal of: `TestCreateBooking_Succeeds`, `TestCreateBooking_SoldOutReturns409`, `TestCreateBooking_SoldOutOnATableReturnsATruthfulMessage`, `TestLookupAndCancelBooking_FullFlow`, `TestLookupBooking_IncludesNullMatchResultWhenNoneSubmitted` (twice — Mario's and Luigi's payloads), `TestCreateBooking_EmailsTheCodeAndBothLinks`, `TestCreateBooking_WithoutSMTPBehavesExactlyAsBefore`, `TestCreateBooking_SMTPFailureKeepsTheBooking`, `TestCreateBooking_MailLabelsTheCopyOnlyWithMoreThanOne`, and the shared `bookForMailTest` helper. For each, change:

```go
		"eventGameId": eventGames[0].ID, "participantName": "Mario Rossi",
		"participantEmail": "mario@example.com", "participantPhone": "3331234567",
```

(or the equivalent with a different variable/name/email, e.g. `tableEventGameID`/`"Luigi Verdi"`/`"luigi@example.com"`/`"3339876543"` in the Luigi payload, or `eventGames[1].ID` in `TestCreateBooking_MailLabelsTheCopyOnlyWithMoreThanOne`) to:

```go
		"eventGameId": eventGames[0].ID, "participantName": "Mario Rossi",
		"participantEmail": "mario@example.com", "termsAccepted": true,
```

i.e. **only** the `"participantPhone": "<digits>",` fragment changes, to `"termsAccepted": true,` — everything else on those two lines (`eventGameId`, `participantName`, `participantEmail`) is untouched.

Then fix the three direct `server.Events.CreateBooking(...)` calls (no HTTP involved, so no JSON/termsAccepted concern — just the Go signature) in `TestAdminCancelBooking_RemovesItFromTheEventBookings`, `TestAdminCancelBooking_UnknownOrAlreadyCancelledReturns404`, and `TestAdminCancelBooking_RequiresAuth`:

```go
// before (all three call sites are identical apart from surrounding context)
booking, err := server.Events.CreateBooking(context.Background(), eventID, eventGames[0].ID,
	"Mario Rossi", "mario@example.com", "3331234567", time.Now())
// after
booking, err := server.Events.CreateBooking(context.Background(), eventID, eventGames[0].ID,
	"Mario Rossi", time.Now())
```

Finally, delete these three tests entirely (they assert on the now-removed cancellation mail):

- `TestCancelBooking_EmailsTheParticipantAReceipt`
- `TestAdminCancelBooking_EmailsTheParticipantThatTheSeatIsFreed`
- `TestAdminCancelBooking_UnknownSendsNoMail`

And simplify `TestCancelBooking_WithoutSMTPStillCancels` only if it references mail — re-check after the deletions: it uses `newTestServer(t)` (mail nil already) and only asserts on cancel status codes, not mail, so it needs **no** change beyond whatever `bookForMailTest`/payload fix Step 7 already applied to its helper.

- [ ] **Step 8: Fix `backend/internal/httpapi/mail_templates_test.go`**

Delete `TestBookingCancelledMail_DistinguishesWhoCancelled` in its entirety (it calls the now-deleted `bookingCancelledMail`):

```go
func TestBookingCancelledMail_DistinguishesWhoCancelled(t *testing.T) {
	byParticipant := bookingCancelledMail(testBookingData(), "https://giochi.example.org/events/7", false)
	byAdmin := bookingCancelledMail(testBookingData(), "https://giochi.example.org/events/7", true)

	if byParticipant.TextBody == byAdmin.TextBody {
		t.Fatal("le due varianti devono dire cose diverse: chi ha annullato cambia il senso della mail")
	}
	if !strings.Contains(byAdmin.TextBody, "organizzazione") {
		t.Errorf("la variante admin deve dire chi ha annullato:\n%s", byAdmin.TextBody)
	}
	for _, body := range []string{
		byParticipant.TextBody, byParticipant.HTMLBody, byAdmin.TextBody, byAdmin.HTMLBody,
	} {
		if !strings.Contains(body, "https://giochi.example.org/events/7") {
			t.Errorf("manca il link all'evento per riprenotare:\n%s", body)
		}
		if !strings.Contains(body, "Catan #2") {
			t.Errorf("manca il gioco annullato:\n%s", body)
		}
	}
}
```

`testBookingData()` itself needs **no change** — `bookingMailData` still has a `ParticipantEmail` field (Step 5 only stopped `bookingMailDataFor` from populating it out of the removed `Booking.ParticipantEmail`; the type itself, and this fixture, are unaffected).

- [ ] **Step 9: Fix the remaining `participantPhone` JSON literals**

In `backend/internal/httpapi/ask_handler_test.go`, `backend/internal/httpapi/events_handlers_test.go`, and `backend/internal/httpapi/loans_handlers_test.go`, these use the raw-JSON-string shape (`fmt.Sprintf` with no spaces after `:`), e.g.:

```go
// before
`{"eventGameId":%d,"participantName":"Anna","participantEmail":"anna@example.com","participantPhone":"3331234567"}`,
// after
`{"eventGameId":%d,"participantName":"Anna","participantEmail":"anna@example.com","termsAccepted":true}`,
```

Apply this same `"participantPhone":"<digits>"` → `"termsAccepted":true` substitution (last field before the closing `}`/backtick, no trailing comma either side) to every occurrence:
- `ask_handler_test.go`: the one payload in the function around line 720 (`"Ada"`/`"ada@example.com"`/`"3330000000"`).
- `events_handlers_test.go`: both payloads (lines ~805 and ~827, both `"Anna"`/`"anna@example.com"`/`"3331234567"`).
- `loans_handlers_test.go`: both booking payloads (lines ~328 and ~348, both `"Anna"`/`"anna@example.com"`/`"3331234567"`) — **not** the `borrowerPhone` payloads elsewhere in this file (lines ~139, ~190, ~241, ~253, ~262, ~357), which belong to the separate, untouched loan-creation endpoint and must be left exactly as they are.

- [ ] **Step 10: Fix `backend/internal/httpapi/match_result_handlers_test.go`**

In `createTestBooking`, replace:

```go
	payload, _ := json.Marshal(map[string]any{
		"eventGameId": eventGameID, "participantName": "Mario Rossi",
		"participantEmail": "mario@example.com", "participantPhone": "3331234567",
	})
```

with:

```go
	payload, _ := json.Marshal(map[string]any{
		"eventGameId": eventGameID, "participantName": "Mario Rossi",
		"participantEmail": "mario@example.com", "termsAccepted": true,
	})
```

- [ ] **Step 11: Remove the phone assertion and field from the loan-desk test fixture**

In `backend/internal/httpapi/loans_handlers_test.go`, remove the `Phone` field from the `deskBookingBody` struct:

```go
// before
type deskBookingBody struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Phone string `json:"phone"`
}
// after
type deskBookingBody struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
```

And in `TestLoanDesk_ShowsActiveBookingsOfACopy`, replace:

```go
	row := desk.Copies[0].ActiveBookings[0]
	if row.Name != "Anna" || row.Phone != "3331234567" {
		t.Errorf("activeBookings[0] = %+v, want Anna / 3331234567", row)
	}
```

with:

```go
	row := desk.Copies[0].ActiveBookings[0]
	if row.Name != "Anna" {
		t.Errorf("activeBookings[0] = %+v, want Anna", row)
	}
```

Also update the stale comment two lines above (`// La prenotazione entra dall'endpoint pubblico, così nome e telefono / sono quelli veri.`) to drop the phone mention, e.g. `// La prenotazione entra dall'endpoint pubblico, così il nome è quello vero.`

- [ ] **Step 12: Build and run the full backend suite**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go build ./... 2>&1
```

Expected: no output. Then:

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./... 2>&1 | tail -100
```

Expected: every package `ok`. If `TestCreateBooking_EmailsTheCodeAndBothLinks` fails because `mailQueued` is false, check that its payload (Step 7) still sends a non-empty `participantEmail` (it does — only `participantPhone` was replaced). If any test still greps for "annullamento"/cancellation-mail behavior, it was missed in Step 7/8 — search with `grep -rn "sendBookingCancelled\|bookingCancelledMail" backend/internal` (should return nothing).

- [ ] **Step 13: Commit**

```bash
git add backend/internal/httpapi/events_bookings_handlers.go backend/internal/httpapi/events_responses.go \
  backend/internal/httpapi/loans_responses.go backend/internal/httpapi/mail.go \
  backend/internal/httpapi/mail_templates.go backend/internal/httpapi/events_bookings_handlers_test.go \
  backend/internal/httpapi/mail_templates_test.go backend/internal/httpapi/loans_handlers_test.go \
  backend/internal/httpapi/events_handlers_test.go backend/internal/httpapi/ask_handler_test.go \
  backend/internal/httpapi/match_result_handlers_test.go
git commit -m "$(cat <<'EOF'
feat: require terms consent, drop cancellation mail and stored contacts

The booking API now requires termsAccepted:true and treats email as
optional and ephemeral (never persisted); the admin booking list and
loan desk no longer expose participant contact info; with no email
stored there is no address left to send a cancellation notice to, so
that mail is removed for both self- and admin-initiated cancellation.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Frontend — booking form consent/email, and "Le mie prenotazioni" (localStorage)

**Files:**
- Create: `frontend/src/utils/myBookings.ts`
- Modify: `frontend/src/views/EventDetailView.vue`

**Interfaces:**
- Consumes: `POST /api/events/{id}/bookings` now returns `{id, eventId, eventGameId, participantName, bookingCode, status, mailQueued}` and requires `{eventGameId, participantName, participantEmail?, termsAccepted}` (Task 3). Router already has named routes `terms`, `privacy`, `booking-score` (params: `{ code }`) — no router changes needed.
- Produces: `frontend/src/utils/myBookings.ts` exports `interface MyBooking { id: number; bookingCode: string; eventId: number; eventGameId: number; gameLabel: string; multiSeat: boolean }`, `listMyBookings(): MyBooking[]`, `saveMyBooking(b: MyBooking): void`, `removeMyBooking(id: number): void`. Later tasks (none in this plan) could reuse these from other views if ever needed.

- [ ] **Step 1: Create `frontend/src/utils/myBookings.ts`**

```ts
/**
 * Le prenotazioni fatte da questo browser, per mostrare "Prenotato" senza
 * doverle rimatchare per telefono o email: nessuno dei due si salva più
 * lato server (vedi il design in docs/superpowers/specs), quindi il
 * promemoria vive solo qui — sparisce cambiando dispositivo o browser, e
 * chi lo perde ricade comunque su "Gestisci prenotazione" col codice.
 */
export interface MyBooking {
  id: number
  bookingCode: string
  eventId: number
  eventGameId: number
  gameLabel: string
  multiSeat: boolean
}

const STORAGE_KEY = 'bgm:my-bookings'

function readAll(): MyBooking[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) {
      return []
    }
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed : []
  } catch {
    // Navigazione privata, storage pieno o disabilitato: nessuna
    // prenotazione ricordata, mai un errore in pagina.
    return []
  }
}

function writeAll(bookings: MyBooking[]) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(bookings))
  } catch {
    // Stesso discorso di readAll: il promemoria semplicemente non
    // sopravvive.
  }
}

export function listMyBookings(): MyBooking[] {
  return readAll()
}

export function saveMyBooking(booking: MyBooking) {
  const bookings = readAll().filter((b) => b.id !== booking.id)
  bookings.push(booking)
  writeAll(bookings)
}

export function removeMyBooking(id: number) {
  writeAll(readAll().filter((b) => b.id !== id))
}
```

- [ ] **Step 2: Update the script block of `EventDetailView.vue`**

`onMounted` stays in the import (it's still used lower in the file, in the `onMounted(async () => { await load() ... })` block) — only add one new import line below the existing ones:

```ts
import { computed, defineAsyncComponent, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import BookingConfirmation from '../components/BookingConfirmation.vue'
import GameDifficulty from '../components/GameDifficulty.vue'
import MarkdownText from '../components/MarkdownText.vue'
import ModalDialog from '../components/ModalDialog.vue'
import { formatEventDateTime } from '../utils/dates'
import { listMyBookings, removeMyBooking, saveMyBooking, type MyBooking } from '../utils/myBookings'
```

Remove the `participantPhone` ref and add `termsAccepted`, plus the new "my bookings" state and a chip-action error ref. Replace:

```ts
const selectedEventGameId = ref<number | null>(null)
const bookingOpen = ref(false)
const participantName = ref('')
const participantEmail = ref('')
const participantPhone = ref('')
const bookingError = ref('')
const bookingResult = ref<BookingResult | null>(null)
```

with:

```ts
const selectedEventGameId = ref<number | null>(null)
const bookingOpen = ref(false)
const participantName = ref('')
const participantEmail = ref('')
const termsAccepted = ref(false)
const bookingError = ref('')
const bookingResult = ref<BookingResult | null>(null)
const chipActionError = ref('')
```

Add the "my bookings" state right after the `confirmed` ref (below its doc comment block), and a helper to keep it in sync:

```ts
const confirmed = ref<ConfirmedBooking[]>([])

/**
 * Le prenotazioni di questo evento che il browser ricorda di aver già
 * fatto: sostituiscono, lato client, il vecchio vincolo server "un
 * booking attivo per telefono" — qui non è un'enforcement, solo un
 * promemoria per non riprenotare lo stesso tavolo per errore.
 */
const myBookingsForEvent = ref<MyBooking[]>(
  listMyBookings().filter((b) => b.eventId === Number(eventId)),
)

function refreshMyBookings() {
  myBookingsForEvent.value = listMyBookings().filter((b) => b.eventId === Number(eventId))
}

function myBookingFor(g: EventGameInfo): MyBooking | null {
  return myBookingsForEvent.value.find((b) => b.eventGameId === g.eventGameId) ?? null
}
```

Update `startBooking` to also reset `termsAccepted`. Replace:

```ts
/**
 * Il form riparte vuoto a ogni tavolo: chi prenota una seconda copia è
 * un'altra persona — un solo booking attivo per telefono — e ritrovare i
 * dati del compagno precompilati porta solo a prenotare a nome suo.
 */
function startBooking(eventGameId: number) {
  selectedEventGameId.value = eventGameId
  participantName.value = ''
  participantEmail.value = ''
  participantPhone.value = ''
  bookingError.value = ''
  bookingResult.value = null
  bookingOpen.value = true
}
```

with:

```ts
/**
 * Il form riparte vuoto a ogni tavolo: chi prenota una seconda copia è
 * un'altra persona, e ritrovare i dati del compagno precompilati (nome,
 * email, consenso già spuntato) porta solo a prenotare a nome suo.
 */
function startBooking(eventGameId: number) {
  selectedEventGameId.value = eventGameId
  participantName.value = ''
  participantEmail.value = ''
  termsAccepted.value = false
  bookingError.value = ''
  bookingResult.value = null
  bookingOpen.value = true
}
```

Replace `submitBooking` entirely:

```ts
// before
async function submitBooking() {
  bookingError.value = ''
  if (selectedEventGameId.value === null) {
    return
  }
  try {
    const result = await api.post<BookingResult>(`/events/${eventId}/bookings`, {
      eventGameId: selectedEventGameId.value,
      participantName: participantName.value,
      participantEmail: participantEmail.value,
      participantPhone: participantPhone.value,
    })
    bookingResult.value = result
    confirmed.value.push({
      code: result.bookingCode,
      label: selectedLabel.value,
      multiSeat: !!selectedGame.value && selectedGame.value.seats > 1,
      mailed: result.mailQueued,
    })
    await load()
  } catch (e) {
    bookingError.value = (e as Error).message
  }
}
```

```ts
// after
async function submitBooking() {
  bookingError.value = ''
  const eventGameId = selectedEventGameId.value
  if (eventGameId === null) {
    return
  }
  try {
    const result = await api.post<BookingResult>(`/events/${eventId}/bookings`, {
      eventGameId,
      participantName: participantName.value,
      participantEmail: participantEmail.value,
      termsAccepted: termsAccepted.value,
    })
    bookingResult.value = result
    const multiSeat = !!selectedGame.value && selectedGame.value.seats > 1
    confirmed.value.push({
      code: result.bookingCode,
      label: selectedLabel.value,
      multiSeat,
      mailed: result.mailQueued,
    })
    saveMyBooking({
      id: result.id,
      bookingCode: result.bookingCode,
      eventId: Number(eventId),
      eventGameId,
      gameLabel: selectedLabel.value,
      multiSeat,
    })
    refreshMyBookings()
    await load()
  } catch (e) {
    bookingError.value = (e as Error).message
  }
}
```

`BookingResult` needs an `id` field for this to type-check — check the interface near the top of the file:

```ts
// before
interface BookingResult {
  id: number
  bookingCode: string
  mailQueued: boolean
}
```

It already has `id: number` — no change needed here.

Add the cancel-from-chip function, right after `submitBooking`:

```ts
/** Annulla dalla pastiglia "Prenotato" sulla scheda evento, non dalla modale. */
async function cancelMyBooking(entry: MyBooking) {
  if (!window.confirm(`Annullare la prenotazione per ${entry.gameLabel}?`)) {
    return
  }
  chipActionError.value = ''
  try {
    await api.post(`/bookings/${entry.id}/cancel`, { bookingCode: entry.bookingCode })
    removeMyBooking(entry.id)
    refreshMyBookings()
    await load()
  } catch (e) {
    chipActionError.value = (e as Error).message
  }
}
```

- [ ] **Step 3: Update the template's booking form**

Replace:

```html
    <form v-else @submit.prevent="submitBooking">
      <label>
        Nome
        <!-- Il <dialog> nativo rispetta autofocus: al tavolo, da telefono,
             si prenota con la tastiera già aperta sul primo campo. -->
        <input v-model="participantName" autofocus required />
      </label>
      <label>
        Email
        <input v-model="participantEmail" type="email" required />
      </label>
      <label>
        Telefono
        <input v-model="participantPhone" required />
      </label>
      <p v-if="bookingError" class="error">{{ bookingError }}</p>
      <div class="form-actions">
        <button type="button" class="btn-secondary" @click="bookingOpen = false">Annulla</button>
        <button type="submit">Conferma prenotazione</button>
      </div>
    </form>
```

with:

```html
    <form v-else @submit.prevent="submitBooking">
      <label>
        Nome
        <!-- Il <dialog> nativo rispetta autofocus: al tavolo, da telefono,
             si prenota con la tastiera già aperta sul primo campo. -->
        <input v-model="participantName" autofocus required />
      </label>
      <label>
        Email
        <input v-model="participantEmail" type="email" />
      </label>
      <p class="field-hint">
        Facoltativa: verrà usata solo per inviarti la conferma della prenotazione.
      </p>
      <label class="booking-consent">
        <input v-model="termsAccepted" type="checkbox" required />
        Accetto i
        <router-link :to="{ name: 'terms' }" target="_blank">termini e condizioni</router-link>
        e ho letto l'<router-link :to="{ name: 'privacy' }" target="_blank">informativa privacy</router-link>
      </label>
      <p v-if="bookingError" class="error">{{ bookingError }}</p>
      <div class="form-actions">
        <button type="button" class="btn-secondary" @click="bookingOpen = false">Annulla</button>
        <button type="submit">Conferma prenotazione</button>
      </div>
    </form>
```

- [ ] **Step 4: Update the template's per-game actions to show the "Prenotato" chip**

Add the chip-action error paragraph right before the games list. Replace:

```html
    <h2 class="table-heading">Al tavolo</h2>
    <p v-if="hasStarted" class="table-note">
      Questo evento è già iniziato: non è più possibile prenotare.
    </p>

    <ul class="event-games">
```

with:

```html
    <h2 class="table-heading">Al tavolo</h2>
    <p v-if="hasStarted" class="table-note">
      Questo evento è già iniziato: non è più possibile prenotare.
    </p>
    <p v-if="chipActionError" class="error">{{ chipActionError }}</p>

    <ul class="event-games">
```

Then replace the actions block:

```html
        <div class="event-game-actions">
          <button
            v-if="!hasStarted && !isFull(g) && !tableOnly(g)"
            type="button"
            @click="startBooking(g.eventGameId)"
          >
            Prenota
          </button>
          <router-link class="detail-link" :to="`/games/${g.gameId}`">
            Dettagli
            <span aria-hidden="true">&rarr;</span>
            <span class="visually-hidden">di {{ copyLabel(g) }}</span>
          </router-link>
        </div>
```

with:

```html
        <div class="event-game-actions">
          <template v-if="myBookingFor(g)">
            <span class="status-badge status-active">Prenotato</span>
            <button type="button" class="btn-danger" @click="cancelMyBooking(myBookingFor(g)!)">
              Annulla prenotazione
            </button>
            <router-link
              class="detail-link"
              :to="{ name: 'booking-score', params: { code: myBookingFor(g)!.bookingCode } }"
            >
              Aggiungi risultato
            </router-link>
          </template>
          <button
            v-else-if="!hasStarted && !isFull(g) && !tableOnly(g)"
            type="button"
            @click="startBooking(g.eventGameId)"
          >
            Prenota
          </button>
          <router-link class="detail-link" :to="`/games/${g.gameId}`">
            Dettagli
            <span aria-hidden="true">&rarr;</span>
            <span class="visually-hidden">di {{ copyLabel(g) }}</span>
          </router-link>
        </div>
```

- [ ] **Step 5: Add minimal CSS for the new consent label**

In `frontend/src/app.css`, near the other booking-form rules (search for `.booking-recap` to find the neighborhood), add:

```css
.booking-consent {
  display: flex;
  align-items: flex-start;
  gap: 0.5em;
  font-weight: 400;
}

.booking-consent input[type='checkbox'] {
  margin-top: 0.2em;
}
```

(This is intentionally minimal — the mandatory `/impeccable` pass in the final task of this plan is where alignment, spacing, and polish against `DESIGN.md` get finished.)

- [ ] **Step 6: Build and manually verify**

```bash
cd frontend && npm run build
```

Expected: builds with no TypeScript errors. Then, with the app running (`docker compose up -d --build`, or `npm run dev` against a running backend), use Claude in Chrome to:
1. Open an event's page, start a booking, confirm the "Conferma prenotazione" button is inert/blocked until the terms checkbox is checked (try submitting with it unchecked — the native `required` validation should stop it).
2. Confirm a booking leaving Email blank — it should succeed and show "Prenotato" for that game after the modal closes and the page reloads its data.
3. Reload the page — the "Prenotato" chip must still show (proves the localStorage read-on-mount works).
4. Click "Annulla prenotazione" on the chip, confirm the browser `confirm()` dialog, and verify the card reverts to showing "Prenota".
5. Book again, then click "Aggiungi risultato" and confirm it navigates to the score-entry page for that booking's code.
6. Check both desktop and a mobile viewport width, and read the browser console for errors.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/utils/myBookings.ts frontend/src/views/EventDetailView.vue frontend/src/app.css
git commit -m "$(cat <<'EOF'
feat: require terms consent, make email optional, remember bookings

The public booking form drops the phone field, makes email optional
with an explanatory hint, and requires a terms/privacy checkbox.
Confirmed bookings are now remembered in localStorage so the event
page can show a "Prenotato" chip (cancel / add-result actions) for
games this browser already booked, instead of relying on a server-side
phone-based duplicate check.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Frontend — admin event view and loan desk lose contact info

**Files:**
- Modify: `frontend/src/views/EventAdminDetailView.vue`
- Modify: `frontend/src/views/LoanDeskView.vue`

**Interfaces:**
- Consumes: `GET /api/events/{id}/bookings` (admin) no longer returns `participantEmail`/`participantPhone` (Task 3); `GET /api/events/{id}/loans` no longer returns `phone` on `activeBookings` rows (Task 3).
- Produces: nothing new consumed elsewhere.

- [ ] **Step 1: Update `EventAdminDetailView.vue`**

Replace the `BookingAdminInfo` interface:

```ts
// before
interface BookingAdminInfo {
  id: number
  eventGameId: number
  gameId: number
  gameName: string
  copyIndex: number
  seats: number
  participantName: string
  participantEmail: string
  participantPhone: string
  createdAt: string
}
```

```ts
// after
interface BookingAdminInfo {
  id: number
  eventGameId: number
  gameId: number
  gameName: string
  copyIndex: number
  seats: number
  participantName: string
  createdAt: string
}
```

Replace the row markup:

```html
              <div class="admin-row">
                <span class="admin-pawn" aria-hidden="true">{{ initial(b.participantName) }}</span>
                <span class="admin-email booking-who">
                  {{ b.participantName }}
                  <span class="row-meta">{{ b.participantEmail }} · {{ b.participantPhone }}</span>
                </span>
                <div class="admin-row-actions">
                  <button type="button" @click="cancelBooking(b)">Annulla</button>
                </div>
              </div>
```

with:

```html
              <div class="admin-row">
                <span class="admin-pawn" aria-hidden="true">{{ initial(b.participantName) }}</span>
                <span class="admin-email booking-who">{{ b.participantName }}</span>
                <div class="admin-row-actions">
                  <button type="button" @click="cancelBooking(b)">Annulla</button>
                </div>
              </div>
```

- [ ] **Step 2: Update `LoanDeskView.vue`**

Replace the `CopyBooking` interface:

```ts
// before
interface CopyBooking {
  id: number
  name: string
  phone: string
}
```

```ts
// after
interface CopyBooking {
  id: number
  name: string
}
```

Update `startLending` (single-booking prefill no longer includes phone):

```ts
// before
function startLending(copy: DeskCopy) {
  lending.value = copy
  lendError.value = ''
  lendNotes.value = ''
  // Con una prenotazione sola non c'è niente da scegliere: si precompila.
  if (copy.activeBookings.length === 1) {
    borrowerName.value = copy.activeBookings[0].name
    borrowerPhone.value = copy.activeBookings[0].phone
  } else {
    borrowerName.value = ''
    borrowerPhone.value = ''
  }
}
```

```ts
// after
function startLending(copy: DeskCopy) {
  lending.value = copy
  lendError.value = ''
  lendNotes.value = ''
  // Con una prenotazione sola non c'è niente da scegliere: si precompila il
  // nome. Il telefono non viene più dalla prenotazione (non lo raccoglie
  // più): va sempre digitato a mano, qui come quando ci sono più
  // prenotazioni tra cui scegliere.
  if (copy.activeBookings.length === 1) {
    borrowerName.value = copy.activeBookings[0].name
  } else {
    borrowerName.value = ''
  }
  borrowerPhone.value = ''
}
```

Update `pickBooking`:

```ts
// before
function pickBooking(booking: CopyBooking) {
  borrowerName.value = booking.name
  borrowerPhone.value = booking.phone
}
```

```ts
// after
function pickBooking(booking: CopyBooking) {
  borrowerName.value = booking.name
}
```

Remove the phone display in the booking picker. Replace:

```html
            <li v-for="b in lending.activeBookings" :key="b.id">
              <button type="button" @click="pickBooking(b)">
                {{ b.name }}
                <span class="row-meta">{{ b.phone }}</span>
              </button>
            </li>
```

with:

```html
            <li v-for="b in lending.activeBookings" :key="b.id">
              <button type="button" @click="pickBooking(b)">
                {{ b.name }}
              </button>
            </li>
```

- [ ] **Step 3: Build and manually verify**

```bash
cd frontend && npm run build
```

Then, with the app running, use Claude in Chrome to open an event's loan desk (banco prestiti) for an event with at least one active booking, confirm the booking picker shows only names (no phone), confirm choosing a booking prefills only the borrower name (phone field stays empty and must be typed), and confirm lending still succeeds end-to-end. Also open the admin event detail page and confirm each booking row shows only the participant's name.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/EventAdminDetailView.vue frontend/src/views/LoanDeskView.vue
git commit -m "$(cat <<'EOF'
feat: drop participant contact info from admin views and loan desk

The admin event detail no longer shows a booking's email/phone (the
API stopped returning them), and the loan desk no longer prefills a
borrower's phone from their booking — it always has to be typed.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Documentation and UI copy alignment

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`
- Modify: `PRODUCT.md`
- Modify: `backend/internal/mailer/mailer.go` (doc comment only)
- Modify: `frontend/src/views/SettingsView.vue` (SMTP section copy only)

**Interfaces:**
- Consumes: nothing (documentation only).
- Produces: nothing consumed by other tasks.

- [ ] **Step 1: Update `CLAUDE.md`**

In the "Prenotazioni & punteggi" section, replace:

```
- Prenotazioni anonime: nome, email, telefono. Vincolo: un solo booking
  attivo per coppia `(event_id, telefono)`.
- Alla prenotazione si genera un `booking_code` mostrato a schermo.
  L'invio email è **opzionale**: configurando un server SMTP nelle
  impostazioni partono la conferma di prenotazione (col codice e i link
  diretti a disdetta e punteggi), l'invito di un amministratore e
  l'avviso di annullamento. Senza SMTP l'app funziona per intero come
  prima — il codice resta a schermo e il link d'invito si copia a mano.
  Vale la stessa regola del provider AI: nessun campo obbligatorio,
  nessun errore in UI perché la posta manca.
```

with:

```
- Prenotazioni anonime: solo nome, più un consenso obbligatorio a
  termini e privacy (checkbox in fase di prenotazione, verificato anche
  lato server; la prova è un timestamp sul booking). Niente telefono:
  non si raccoglie più. L'email è facoltativa e non viene mai salvata —
  si usa solo al volo per la mail di conferma, se lasciata. Il "non hai
  già prenotato questo tavolo" non è più un vincolo server: il browser
  di chi prenota lo ricorda da sé (localStorage) e mostra una pastiglia
  "Prenotato" al posto del bottone, con le azioni annulla/segna
  punteggio.
- Alla prenotazione si genera un `booking_code` mostrato a schermo.
  L'invio email è **opzionale**: configurando un server SMTP nelle
  impostazioni partono la conferma di prenotazione (col codice e i link
  diretti a disdetta e punteggi, solo se è stata lasciata un'email) e
  l'invito di un amministratore. Senza SMTP l'app funziona per intero
  come prima — il codice resta a schermo e il link d'invito si copia a
  mano. Non esiste più un avviso di annullamento via mail: senza
  l'email salvata sul booking non c'è più un indirizzo a cui mandarlo,
  né quando annulla il partecipante né quando annulla l'admin.
```

- [ ] **Step 2: Update `README.md`**

Replace (around what is currently line ~251-256):

```
- **Email (SMTP)**: se configurato, l'app manda da sé l'invito di un
  amministratore, la conferma di una prenotazione e l'avviso di
  annullamento. Se assente, l'app funziona esattamente come prima: il
  codice di prenotazione resta a schermo e il link d'invito si copia a
  mano. Dettagli sotto.
```

with:

```
- **Email (SMTP)**: se configurato, l'app manda da sé l'invito di un
  amministratore e la conferma di una prenotazione (se chi prenota ha
  lasciato un'email — è facoltativa e non viene mai salvata). Se
  assente, l'app funziona esattamente come prima: il codice di
  prenotazione resta a schermo e il link d'invito si copia a mano.
  Dettagli sotto.
```

Replace (around what is currently line ~390-396):

```
Senza un server SMTP configurato l'app funziona esattamente come prima:
il `booking_code` resta a schermo dopo la prenotazione e il link
d'invito di un amministratore si copia e recapita a mano. Configurando
un server nella sezione "Configurazione Email (SMTP)" della pagina
Impostazioni partono da sole tre email:

- **invito di un amministratore**, con il link per attivare l'accesso;
- **conferma di prenotazione**, col codice, il link per gestirla o
  disdirla e quello per inserire il punteggio a fine partita;
- **avviso di annullamento**, sia quando è il partecipante a disdire sia
  quando lo fa un organizzatore.
```

with:

```
Senza un server SMTP configurato l'app funziona esattamente come prima:
il `booking_code` resta a schermo dopo la prenotazione e il link
d'invito di un amministratore si copia e recapita a mano. Configurando
un server nella sezione "Configurazione Email (SMTP)" della pagina
Impostazioni partono da sole due email:

- **invito di un amministratore**, con il link per attivare l'accesso;
- **conferma di prenotazione**, col codice, il link per gestirla o
  disdirla e quello per inserire il punteggio a fine partita — solo se
  chi prenota ha lasciato un'email: è facoltativa e non viene mai
  salvata sul database.
```

- [ ] **Step 3: Update `PRODUCT.md`**

Replace:

```
- Un solo ruolo autenticato (admin); i partecipanti non hanno mai un
  account, si identificano solo con nome + telefono alla prenotazione
  e successivamente con il solo `booking_code`.
```

with:

```
- Un solo ruolo autenticato (admin); i partecipanti non hanno mai un
  account, si identificano solo con il nome alla prenotazione (email
  facoltativa, mai salvata; telefono non richiesto) e successivamente
  con il solo `booking_code`.
```

Replace:

```
- Email/SMTP opzionale: senza configurazione il `booking_code` resta
  l'unico strumento di gestione post-prenotazione, mostrato a schermo;
  configurando un server SMTP nelle impostazioni l'app manda anche una
  conferma di prenotazione, un avviso di annullamento e l'invito di un
  amministratore.
```

with:

```
- Email/SMTP opzionale: senza configurazione il `booking_code` resta
  l'unico strumento di gestione post-prenotazione, mostrato a schermo;
  configurando un server SMTP nelle impostazioni l'app manda anche una
  conferma di prenotazione (se chi prenota ha lasciato un'email) e
  l'invito di un amministratore.
```

- [ ] **Step 4: Update `backend/internal/mailer/mailer.go`'s package doc comment**

Replace:

```go
// Package mailer manda email via SMTP. Nel progetto serve alle tre
// comunicazioni verso l'esterno: l'invito di un amministratore, la
// conferma di una prenotazione e l'avviso di annullamento.
```

with:

```go
// Package mailer manda email via SMTP. Nel progetto serve alle due
// comunicazioni verso l'esterno: l'invito di un amministratore e la
// conferma di una prenotazione.
```

- [ ] **Step 5: Update `frontend/src/views/SettingsView.vue`'s SMTP section copy**

Replace:

```html
        <p class="field-hint">
          Se lo configuri, l'app manda da sé l'invito di un amministratore, la
          conferma di una prenotazione — con il codice e i link per disdire o
          segnare i punti — e l'avviso di annullamento. Lasciandolo vuoto
          funziona come prima: il codice resta solo a schermo e il link di
          invito si copia a mano.
        </p>
        <p v-if="smtpConfigured && !publicBaseUrl" class="field-hint">
          Manca l'indirizzo pubblico, qui sopra in "Generale": senza,
          l'invito di un amministratore e l'avviso di annullamento portano
          un link composto dall'indirizzo con cui stai navigando adesso, che
          chi lo riceve potrebbe non riuscire a raggiungere.
        </p>
```

with:

```html
        <p class="field-hint">
          Se lo configuri, l'app manda da sé l'invito di un amministratore e
          la conferma di una prenotazione — con il codice e i link per
          disdire o segnare i punti — quando chi prenota lascia un'email
          (è facoltativa). Lasciandolo vuoto funziona come prima: il codice
          resta solo a schermo e il link di invito si copia a mano.
        </p>
        <p v-if="smtpConfigured && !publicBaseUrl" class="field-hint">
          Manca l'indirizzo pubblico, qui sopra in "Generale": senza,
          l'invito di un amministratore porta un link composto
          dall'indirizzo con cui stai navigando adesso, che chi lo riceve
          potrebbe non riuscire a raggiungere.
        </p>
```

- [ ] **Step 6: Verify the frontend still builds**

```bash
cd frontend && npm run build
```

Expected: no errors (this task only changed static template text).

- [ ] **Step 7: Commit**

```bash
git add CLAUDE.md README.md PRODUCT.md backend/internal/mailer/mailer.go frontend/src/views/SettingsView.vue
git commit -m "$(cat <<'EOF'
docs: describe the phone-free, consent-based booking flow

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Final verification and polish

**Files:** none (verification only, plus whatever `/impeccable` touches).

**Interfaces:** none.

- [ ] **Step 1: Full backend suite**

```bash
docker run --rm -v "$(pwd)/backend:/app" \
  -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
  -w /app golang:1.25 go test ./... 2>&1 | tail -100
```

Expected: every package `ok`.

- [ ] **Step 2: Full frontend build**

```bash
cd frontend && npm run build
```

Expected: no TypeScript errors.

- [ ] **Step 3: Full app manual pass with Claude in Chrome**

`docker compose up -d --build`, then open `http://localhost:8080` and, on an event with at least one bookable game:
1. Book a game leaving email blank and accepting terms — confirm success, confirm the confirmation screen does not claim a mail was sent.
2. Book a second game providing an email and accepting terms (requires SMTP configured in Impostazioni to actually see `mailQueued: true`; if SMTP is not configured in this environment, just confirm the request still succeeds and the screen correctly does not claim a mail was sent).
3. Try to submit the booking form with the terms checkbox unchecked — confirm the browser blocks submission.
4. Reload the event page — confirm both bookings still show as "Prenotato" with working "Annulla prenotazione" and "Aggiungi risultato" actions.
5. Check the admin event detail page for that event — confirm booking rows show only names.
6. Check the loan desk for that event — confirm the booking picker shows only names and lending still works with a manually-typed phone.
7. Check both a desktop and a ~400px-wide mobile viewport, and read the browser console for errors on each page touched.

- [ ] **Step 4: Run `/impeccable` on the changed frontend surface**

Per this repo's workflow rule, run `/impeccable` (e.g. `polish` or `audit`) against the booking flow (`EventDetailView.vue`'s form and new chip UI), the admin event detail rows, and the loan desk changes, and apply whatever it recommends before considering this plan done.
