# Fase 3 — Eventi e Prenotazioni Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Aggiungere al BoardGames Manager la gestione degli eventi: un admin crea un evento e vi associa giochi del catalogo con una quantità di copie; i partecipanti, senza login, vedono l'elenco pubblico degli eventi futuri, prenotano una copia di un gioco (nome, email, telefono), ricevono un `booking_code`, e possono annullare la prenotazione con email + `booking_code`.

**Architecture:** Nuovo pacchetto Go `internal/events` (repository `Event`/`EventGame`/`Booking`, stesso pattern di `internal/games`). `internal/httpapi` guadagna handler pubblici (lettura eventi, creazione/lookup/cancellazione booking, con rate-limit minimale sugli ultimi due) e protetti (CRUD evento, elenco booking di un evento). Il frontend introduce le prime route pubbliche reali dell'app (finora ogni route richiedeva login): un `meta.public` sulle route bypassa il redirect a `/login`, e un componente `PublicHeader` sostituisce `DashboardLayout` sulle pagine pubbliche.

**Tech Stack:** Go (`database/sql`, `crypto/rand`), Vue 3 + TypeScript (già in uso), SQLite.

**Spec:** `docs/superpowers/specs/2026-09-01-eventi-prenotazioni-design.md`

## Global Constraints

- Ogni comando Go va eseguito via Docker (il toolchain locale è rotto su questa macchina):
  ```
  docker run --rm -v "$(pwd)/backend:/app" \
    -v bgm-gomodcache:/root/go/pkg/mod -v bgm-gocache:/root/.cache/go-build \
    -w /app golang:1.25 go test ./...
  ```
  Riusa sempre i volumi nominati `bgm-gomodcache`/`bgm-gocache`. Il frontend (`npm`) funziona regolarmente in locale, non serve Docker.
- Ogni link/router-link Vue verso una route non ancora registrata nello STESSO task deve usare una stringa di path semplice (es. `to="/events/new"`), mai un oggetto `{name: '...'}` — una `RouterLink` con nome non registrato lancia un errore che blocca il rendering dell'intera pagina.
- Nessuno stato esplicito sull'evento: "prenotabile" è sempre derivato confrontando `event_date`+`start_time` con l'istante della richiesta, lato server.
- `PUT /api/events/{id}` sostituisce l'intero elenco `EventGame`, ma senza mai eliminare/ricreare righe `event_games` che hanno booking attivi collegati (la FK `bookings.event_game_id` è `ON DELETE CASCADE`: cancellare la riga `event_games` sbagliata cancellerebbe silenziosamente le prenotazioni). Rimuovere un gioco o ridurne la quantità sotto il numero di booking attivi è rifiutato con 409.
- Le risposte di lookup/cancellazione booking falliti sono sempre generiche (404, "prenotazione non trovata") — mai distinguere "codice sbagliato" da "email sbagliata", per non facilitare l'enumerazione.
- Fuori scope in questa fase: inserimento punteggio, classifica, invio email, azioni admin sui booking di altri, stato manuale dell'evento.

---

### Task 1: Migrazione eventi e prenotazioni

**Files:**
- Create: `backend/internal/db/migrations/0004_events.sql`

- [ ] **Step 1: Scrivere la migrazione**

`backend/internal/db/migrations/0004_events.sql`:
```sql
CREATE TABLE events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    description TEXT,
    event_date TEXT NOT NULL,
    start_time TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE event_games (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    game_id INTEGER NOT NULL REFERENCES games(id),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    UNIQUE(event_id, game_id)
);

CREATE TABLE bookings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    event_game_id INTEGER NOT NULL REFERENCES event_games(id) ON DELETE CASCADE,
    participant_name TEXT NOT NULL,
    participant_email TEXT NOT NULL,
    participant_phone TEXT NOT NULL,
    booking_code TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL CHECK (status IN ('active', 'cancelled')),
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX idx_one_active_booking_per_phone_per_event
    ON bookings(event_id, participant_phone) WHERE status = 'active';
```

- [ ] **Step 2: Verificare che la migrazione si applichi**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/db/... -v
```
Expected: PASS (i test esistenti di `internal/db` verificano solo `users`/`sessions`/`app_settings`, ma devono continuare a passare — nessuna tabella pre-esistente è stata toccata).

- [ ] **Step 3: Commit**

```bash
git add backend/internal/db/migrations/0004_events.sql
git commit -m "feat: add events, event_games and bookings tables"
```

---

### Task 2: Repository eventi — Event + EventGame

**Files:**
- Create: `backend/internal/events/store.go`
- Create: `backend/internal/events/store_test.go`

**Interfaces:**
- Produces: `events.Event{ID int64; Title string; Description *string; EventDate, StartTime string; CreatedAt time.Time}`, `events.EventGame{ID, EventID, GameID int64; Quantity int}`, `events.EventGameInput{GameID int64; Quantity int}`, `events.Store`, `events.NewStore(conn *sql.DB) *Store`, `(*Store).CreateEvent(ctx, title string, description *string, eventDate, startTime string, games []EventGameInput) (Event, error)`, `GetEvent(ctx, id int64) (Event, error)`, `ListEvents(ctx, includePast bool, now time.Time) ([]Event, error)`, `ListEventGames(ctx, eventID int64) ([]EventGame, error)`, `RemainingCapacity(ctx, eventGameID int64) (int, error)`, `UpdateEvent(ctx, id int64, title string, description *string, eventDate, startTime string, games []EventGameInput) (Event, error)`, `DeleteEvent(ctx, id int64) error`, `events.ErrNotFound`, `events.ErrGameNotFound`, `events.ErrQuantityBelowActiveBookings`.

- [ ] **Step 1: Scrivere i test**

`backend/internal/events/store_test.go`:
```go
package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"boardgames-manager/internal/db"
	"boardgames-manager/internal/events"
	"boardgames-manager/internal/games"
)

func newTestStore(t *testing.T) (*events.Store, *games.Store) {
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
	return events.NewStore(conn), games.NewStore(conn)
}

func strPtr(v string) *string { return &v }

func mustCreateGame(t *testing.T, store *games.Store, name string) int64 {
	t.Helper()
	g, err := store.CreateGame(context.Background(), games.Game{Name: name})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	return g.ID
}

func TestCreateAndGetEvent(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")

	created, err := eventStore.CreateEvent(ctx, "Serata giochi", strPtr("Una serata di board game"),
		"2026-10-01", "20:00", []events.EventGameInput{{GameID: gameID, Quantity: 2}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected a non-zero id")
	}

	found, err := eventStore.GetEvent(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if found.Title != "Serata giochi" || found.EventDate != "2026-10-01" || found.StartTime != "20:00" {
		t.Fatalf("unexpected event: %+v", found)
	}

	eventGames, err := eventStore.ListEventGames(ctx, created.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if len(eventGames) != 1 || eventGames[0].GameID != gameID || eventGames[0].Quantity != 2 {
		t.Fatalf("unexpected event games: %+v", eventGames)
	}
}

func TestGetEvent_NotFound(t *testing.T) {
	eventStore, _ := newTestStore(t)
	_, err := eventStore.GetEvent(context.Background(), 999)
	if !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCreateEvent_RejectsUnknownGame(t *testing.T) {
	eventStore, _ := newTestStore(t)
	_, err := eventStore.CreateEvent(context.Background(), "Serata giochi", nil,
		"2026-10-01", "20:00", []events.EventGameInput{{GameID: 999, Quantity: 1}})
	if !errors.Is(err, events.ErrGameNotFound) {
		t.Fatalf("expected ErrGameNotFound, got %v", err)
	}
}

func TestListEvents_FiltersPastEventsUnlessIncluded(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	if _, err := eventStore.CreateEvent(ctx, "Passato", nil, "2026-08-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}}); err != nil {
		t.Fatalf("create past event: %v", err)
	}
	if _, err := eventStore.CreateEvent(ctx, "Futuro", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}}); err != nil {
		t.Fatalf("create future event: %v", err)
	}

	futureOnly, err := eventStore.ListEvents(ctx, false, now)
	if err != nil {
		t.Fatalf("list future only: %v", err)
	}
	if len(futureOnly) != 1 || futureOnly[0].Title != "Futuro" {
		t.Fatalf("expected only the future event, got %+v", futureOnly)
	}

	all, err := eventStore.ListEvents(ctx, true, now)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 events, got %d", len(all))
	}
}

func TestRemainingCapacity_DecreasesWithActiveBookingsOnly(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")

	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 2}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	eventGameID := eventGames[0].ID

	remaining, err := eventStore.RemainingCapacity(ctx, eventGameID)
	if err != nil {
		t.Fatalf("remaining: %v", err)
	}
	if remaining != 2 {
		t.Fatalf("expected 2, got %d", remaining)
	}

	insertBooking(t, eventStore, event.ID, eventGameID, "active")
	insertBooking(t, eventStore, event.ID, eventGameID, "cancelled")

	remaining, err = eventStore.RemainingCapacity(ctx, eventGameID)
	if err != nil {
		t.Fatalf("remaining after bookings: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("expected 1 (only the active booking counts), got %d", remaining)
	}
}

func TestUpdateEvent_ChangesFieldsAndReplacesGames(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameA := mustCreateGame(t, gameStore, "Catan")
	gameB := mustCreateGame(t, gameStore, "Azul")

	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameA, Quantity: 2}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := eventStore.UpdateEvent(ctx, event.ID, "Serata rinnovata", strPtr("Nuova descrizione"),
		"2026-10-02", "21:00", []events.EventGameInput{{GameID: gameB, Quantity: 3}})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Title != "Serata rinnovata" || updated.EventDate != "2026-10-02" || updated.StartTime != "21:00" {
		t.Fatalf("unexpected updated event: %+v", updated)
	}

	eventGames, err := eventStore.ListEventGames(ctx, event.ID)
	if err != nil {
		t.Fatalf("list event games: %v", err)
	}
	if len(eventGames) != 1 || eventGames[0].GameID != gameB || eventGames[0].Quantity != 3 {
		t.Fatalf("expected only game B with quantity 3, got %+v", eventGames)
	}
}

func TestUpdateEvent_RejectsQuantityBelowActiveBookings(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")

	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 2}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	insertBooking(t, eventStore, event.ID, eventGames[0].ID, "active")
	insertBooking(t, eventStore, event.ID, eventGames[0].ID, "active")

	_, err = eventStore.UpdateEvent(ctx, event.ID, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
	if !errors.Is(err, events.ErrQuantityBelowActiveBookings) {
		t.Fatalf("expected ErrQuantityBelowActiveBookings, got %v", err)
	}

	_, err = eventStore.UpdateEvent(ctx, event.ID, "Serata giochi", nil, "2026-10-01", "20:00", nil)
	if !errors.Is(err, events.ErrQuantityBelowActiveBookings) {
		t.Fatalf("expected ErrQuantityBelowActiveBookings when removing the game entirely, got %v", err)
	}

	remaining, err := eventStore.RemainingCapacity(ctx, eventGames[0].ID)
	if err != nil {
		t.Fatalf("remaining: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("a rejected update must not have touched event_games; expected remaining 0, got %d", remaining)
	}
}

func TestDeleteEvent_CascadesToEventGamesAndBookings(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")

	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := eventStore.DeleteEvent(ctx, event.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = eventStore.GetEvent(ctx, event.ID)
	if !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteEvent_NotFound(t *testing.T) {
	eventStore, _ := newTestStore(t)
	err := eventStore.DeleteEvent(context.Background(), 999)
	if !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// insertBooking writes a booking row directly (bypassing the not-yet-written
// CreateBooking, added in a later task) so RemainingCapacity/UpdateEvent
// guard tests can set up booking state on their own.
func insertBooking(t *testing.T, store *events.Store, eventID, eventGameID int64, status string) {
	t.Helper()
	if err := store.TestInsertBooking(eventID, eventGameID, status); err != nil {
		t.Fatalf("insert booking fixture: %v", err)
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/events/... -v
```
Expected: FAIL (pacchetto `events` non esiste)

- [ ] **Step 3: Implementare il repository**

`backend/internal/events/store.go`:
```go
package events

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Event struct {
	ID          int64
	Title       string
	Description *string
	EventDate   string
	StartTime   string
	CreatedAt   time.Time
}

type EventGame struct {
	ID       int64
	EventID  int64
	GameID   int64
	Quantity int
}

type EventGameInput struct {
	GameID   int64
	Quantity int
}

var (
	ErrNotFound                    = errors.New("not found")
	ErrGameNotFound                = errors.New("referenced game not found")
	ErrQuantityBelowActiveBookings = errors.New("quantity below active bookings")
)

type Store struct {
	db *sql.DB
}

func NewStore(conn *sql.DB) *Store {
	return &Store{db: conn}
}

// queryer is satisfied by both *sql.DB and *sql.Tx, so read helpers can run
// either against the pool or inside an in-flight transaction.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// execer is satisfied by both *sql.DB and *sql.Tx.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (s *Store) CreateEvent(ctx context.Context, title string, description *string, eventDate, startTime string, gamesInput []EventGameInput) (Event, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Event{}, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO events (title, description, event_date, start_time) VALUES (?, ?, ?, ?)`,
		title, description, eventDate, startTime,
	)
	if err != nil {
		return Event{}, err
	}
	eventID, err := res.LastInsertId()
	if err != nil {
		return Event{}, err
	}

	if err := insertEventGames(ctx, tx, eventID, gamesInput); err != nil {
		return Event{}, err
	}

	if err := tx.Commit(); err != nil {
		return Event{}, err
	}
	return s.GetEvent(ctx, eventID)
}

func insertEventGames(ctx context.Context, tx execQueryer, eventID int64, gamesInput []EventGameInput) error {
	for _, g := range gamesInput {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM games WHERE id = ?`, g.GameID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return ErrGameNotFound
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO event_games (event_id, game_id, quantity) VALUES (?, ?, ?)`,
			eventID, g.GameID, g.Quantity,
		); err != nil {
			return err
		}
	}
	return nil
}

// execQueryer is what insertEventGames needs; *sql.Tx satisfies it.
type execQueryer interface {
	execer
	queryer
}

func (s *Store) GetEvent(ctx context.Context, id int64) (Event, error) {
	return getEvent(ctx, s.db, id)
}

func getEvent(ctx context.Context, q queryer, id int64) (Event, error) {
	var e Event
	var createdAt string
	err := q.QueryRowContext(ctx,
		`SELECT id, title, description, event_date, start_time, created_at FROM events WHERE id = ?`, id,
	).Scan(&e.ID, &e.Title, &e.Description, &e.EventDate, &e.StartTime, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Event{}, ErrNotFound
	}
	if err != nil {
		return Event{}, err
	}
	e.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return e, nil
}

func (s *Store) ListEvents(ctx context.Context, includePast bool, now time.Time) ([]Event, error) {
	query := `SELECT id, title, description, event_date, start_time, created_at FROM events`
	args := []any{}
	if !includePast {
		query += ` WHERE event_date || ' ' || start_time >= ?`
		args = append(args, now.Format("2006-01-02 15:04"))
	}
	query += ` ORDER BY event_date, start_time`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var e Event
		var createdAt string
		if err := rows.Scan(&e.ID, &e.Title, &e.Description, &e.EventDate, &e.StartTime, &createdAt); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) ListEventGames(ctx context.Context, eventID int64) ([]EventGame, error) {
	return listEventGames(ctx, s.db, eventID)
}

func listEventGames(ctx context.Context, q queryer, eventID int64) ([]EventGame, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, event_id, game_id, quantity FROM event_games WHERE event_id = ? ORDER BY id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EventGame
	for rows.Next() {
		var eg EventGame
		if err := rows.Scan(&eg.ID, &eg.EventID, &eg.GameID, &eg.Quantity); err != nil {
			return nil, err
		}
		out = append(out, eg)
	}
	return out, rows.Err()
}

func (s *Store) RemainingCapacity(ctx context.Context, eventGameID int64) (int, error) {
	var remaining int
	err := s.db.QueryRowContext(ctx,
		`SELECT eg.quantity - (
			SELECT COUNT(*) FROM bookings b WHERE b.event_game_id = eg.id AND b.status = 'active'
		 ) FROM event_games eg WHERE eg.id = ?`, eventGameID,
	).Scan(&remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return remaining, err
}

func (s *Store) UpdateEvent(ctx context.Context, id int64, title string, description *string, eventDate, startTime string, gamesInput []EventGameInput) (Event, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Event{}, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE events SET title = ?, description = ?, event_date = ?, start_time = ? WHERE id = ?`,
		title, description, eventDate, startTime, id,
	)
	if err != nil {
		return Event{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Event{}, err
	}
	if affected == 0 {
		return Event{}, ErrNotFound
	}

	existing, err := listEventGames(ctx, tx, id)
	if err != nil {
		return Event{}, err
	}
	existingByGame := map[int64]EventGame{}
	for _, eg := range existing {
		existingByGame[eg.GameID] = eg
	}

	activeCounts, err := activeBookingCountsByGame(ctx, tx, id)
	if err != nil {
		return Event{}, err
	}

	newByGame := map[int64]int{}
	for _, g := range gamesInput {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM games WHERE id = ?`, g.GameID).Scan(&exists); err != nil {
			return Event{}, err
		}
		if exists == 0 {
			return Event{}, ErrGameNotFound
		}
		newByGame[g.GameID] = g.Quantity
	}
	for gameID, activeCount := range activeCounts {
		if activeCount > 0 && newByGame[gameID] < activeCount {
			return Event{}, ErrQuantityBelowActiveBookings
		}
	}

	// Games no longer present: safe to drop (the guard above already ensured
	// zero active bookings for any of them). This also cascades away any
	// cancelled bookings for that game/event pair, which is fine — there is
	// no historical reporting on Booking/EventGame in this phase.
	for gameID, eg := range existingByGame {
		if _, stillPresent := newByGame[gameID]; !stillPresent {
			if _, err := tx.ExecContext(ctx, `DELETE FROM event_games WHERE id = ?`, eg.ID); err != nil {
				return Event{}, err
			}
		}
	}
	// Games kept: update the quantity in place (never delete+recreate — that
	// would cascade-delete their bookings too). Games newly added: insert.
	for _, g := range gamesInput {
		if eg, ok := existingByGame[g.GameID]; ok {
			if _, err := tx.ExecContext(ctx, `UPDATE event_games SET quantity = ? WHERE id = ?`, g.Quantity, eg.ID); err != nil {
				return Event{}, err
			}
		} else {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO event_games (event_id, game_id, quantity) VALUES (?, ?, ?)`, id, g.GameID, g.Quantity,
			); err != nil {
				return Event{}, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return Event{}, err
	}
	return s.GetEvent(ctx, id)
}

func activeBookingCountsByGame(ctx context.Context, tx *sql.Tx, eventID int64) (map[int64]int, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT eg.game_id, COUNT(b.id) FROM event_games eg
		 LEFT JOIN bookings b ON b.event_game_id = eg.id AND b.status = 'active'
		 WHERE eg.event_id = ? GROUP BY eg.game_id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int64]int{}
	for rows.Next() {
		var gameID int64
		var count int
		if err := rows.Scan(&gameID, &count); err != nil {
			return nil, err
		}
		out[gameID] = count
	}
	return out, rows.Err()
}

func (s *Store) DeleteEvent(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE id = ?`, id)
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

// TestInsertBooking writes a booking row directly, bypassing all of
// CreateBooking's validation. It exists only so tests in this package (and
// the bookings/lookup/cancel tests added in later tasks) can set up booking
// fixtures without a circular dependency on CreateBooking's own tests.
func (s *Store) TestInsertBooking(eventID, eventGameID int64, status string) error {
	code := fmt.Sprintf("TEST%04d", eventGameID*10+int64(len(status)))
	_, err := s.db.Exec(
		`INSERT INTO bookings (event_id, event_game_id, participant_name, participant_email, participant_phone, booking_code, status)
		 VALUES (?, ?, 'Test Participant', 'test@example.com', ?, ?, ?)`,
		eventID, eventGameID, code, code, status,
	)
	return err
}
```

- [ ] **Step 4: Eseguire i test e verificare che passino**

Run: (stesso comando dello Step 2)
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/events/store.go backend/internal/events/store_test.go
git commit -m "feat: add events and event_games repository"
```

---

### Task 3: Repository prenotazioni — booking_code e CreateBooking

**Files:**
- Create: `backend/internal/events/bookings.go`
- Create: `backend/internal/events/bookings_test.go`
- Modify: `backend/internal/events/store.go` (aggiungere `isUniqueConstraintErr`)

**Interfaces:**
- Consumes: `events.Store`, `events.ErrNotFound` (Task 2).
- Produces: `events.Booking{ID, EventID, EventGameID int64; ParticipantName, ParticipantEmail, ParticipantPhone, BookingCode, Status string; CreatedAt time.Time}`, `events.BookingStatusActive`, `events.BookingStatusCancelled` (costanti stringa), `(*Store).CreateBooking(ctx, eventID, eventGameID int64, name, email, phone string, now time.Time) (Booking, error)`, `events.ErrEventAlreadyStarted`, `events.ErrGameSoldOut`, `events.ErrDuplicatePhoneBooking`.

- [ ] **Step 1: Scrivere i test**

`backend/internal/events/bookings_test.go`:
```go
package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"boardgames-manager/internal/events"
)

func TestCreateBooking_Succeeds(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 2}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	booking, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}
	if booking.ID == 0 {
		t.Fatal("expected a non-zero id")
	}
	if len(booking.BookingCode) != 8 {
		t.Fatalf("expected an 8-char booking code, got %q", booking.BookingCode)
	}
	if booking.Status != events.BookingStatusActive {
		t.Fatalf("expected status active, got %q", booking.Status)
	}

	remaining, err := eventStore.RemainingCapacity(ctx, eventGames[0].ID)
	if err != nil {
		t.Fatalf("remaining: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("expected remaining capacity 1, got %d", remaining)
	}
}

func TestCreateBooking_RejectsWhenEventAlreadyStarted(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-09-01", "10:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 2}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) // after the 10:00 start

	_, err = eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if !errors.Is(err, events.ErrEventAlreadyStarted) {
		t.Fatalf("expected ErrEventAlreadyStarted, got %v", err)
	}
}

func TestCreateBooking_RejectsWhenSoldOut(t *testing.T) {
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

	if _, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331111111", now); err != nil {
		t.Fatalf("first booking: %v", err)
	}
	_, err = eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Luigi Verdi", "luigi@example.com", "3332222222", now)
	if !errors.Is(err, events.ErrGameSoldOut) {
		t.Fatalf("expected ErrGameSoldOut, got %v", err)
	}
}

func TestCreateBooking_RejectsDuplicatePhoneForSameEvent(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 5}})
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

func TestCreateBooking_AllowsSamePhoneAfterCancellation(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 5}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	first, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("first booking: %v", err)
	}
	if _, err := eventStore.CancelBooking(ctx, first.ID, first.ParticipantEmail, first.BookingCode); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	if _, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331234567", now); err != nil {
		t.Fatalf("expected the same phone to be able to book again after cancelling, got %v", err)
	}
}

func TestCreateBooking_UnknownEventGameReturnsNotFound(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	_, err = eventStore.CreateBooking(ctx, event.ID, 999, "Mario Rossi", "mario@example.com", "3331234567", now)
	if !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/events/... -run Booking -v
```
Expected: FAIL (`Booking`/`CreateBooking`/`CancelBooking` non esistono)

- [ ] **Step 3: Aggiungere `isUniqueConstraintErr` a store.go**

In `backend/internal/events/store.go`, aggiungere l'import `"strings"` e, in coda al file:
```go
func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
```

- [ ] **Step 4: Implementare booking_code e CreateBooking**

`backend/internal/events/bookings.go`:
```go
package events

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Booking struct {
	ID                int64
	EventID           int64
	EventGameID       int64
	ParticipantName   string
	ParticipantEmail  string
	ParticipantPhone  string
	BookingCode       string
	Status            string
	CreatedAt         time.Time
}

const (
	BookingStatusActive    = "active"
	BookingStatusCancelled = "cancelled"
)

var (
	ErrEventAlreadyStarted       = errors.New("event already started")
	ErrGameSoldOut               = errors.New("game sold out")
	ErrDuplicatePhoneBooking     = errors.New("phone already has an active booking for this event")
	ErrInvalidBookingCredentials = errors.New("invalid email or booking code")
)

// bookingCodeAlphabet excludes visually ambiguous characters (0/O, 1/I).
const bookingCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// generateBookingCode is not perfectly uniform (256 % 33 != 0) but the bias
// is negligible for an 8-character identifier that is not a security
// credential on its own (it is always paired with the participant's email).
func generateBookingCode() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	code := make([]byte, 8)
	for i, b := range buf {
		code[i] = bookingCodeAlphabet[int(b)%len(bookingCodeAlphabet)]
	}
	return string(code), nil
}

func (s *Store) CreateBooking(ctx context.Context, eventID, eventGameID int64, name, email, phone string, now time.Time) (Booking, error) {
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

	var eventGameCount int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM event_games WHERE id = ? AND event_id = ?`, eventGameID, eventID,
	).Scan(&eventGameCount); err != nil {
		return Booking{}, err
	}
	if eventGameCount == 0 {
		return Booking{}, ErrNotFound
	}

	code, err := generateBookingCode()
	if err != nil {
		return Booking{}, err
	}

	// Single atomic statement: the WHERE clause re-checks capacity as part of
	// the same write, so SQLite's write-lock makes this race-safe against
	// concurrent bookings for the last remaining copy — no separate
	// check-then-insert window. A collision on the (event_id, phone) unique
	// index (a duplicate booking that slipped past an earlier read) surfaces
	// here too and is mapped to ErrDuplicatePhoneBooking below.
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO bookings (event_id, event_game_id, participant_name, participant_email, participant_phone, booking_code, status)
		 SELECT ?, ?, ?, ?, ?, ?, 'active'
		 WHERE (SELECT COUNT(*) FROM bookings WHERE event_game_id = ? AND status = 'active') <
		       (SELECT quantity FROM event_games WHERE id = ?)`,
		eventID, eventGameID, name, email, phone, code, eventGameID, eventGameID,
	)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return Booking{}, ErrDuplicatePhoneBooking
		}
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
```

`TestCreateBooking_AllowsSamePhoneAfterCancellation` chiama già `eventStore.CancelBooking`, che arriva nel Task 4: questo test va spostato in `bookings_test.go` del Task 4, oppure — più semplice — lasciarlo qui ma **il pacchetto non compilerà finché il Task 4 non è completato**. Per tenere questo task verde da solo, sposta quella singola funzione di test in fondo al file e aggiungi il commento `// Covered together with CancelBooking in the next task.` sopra la sua definizione, oppure spostala fisicamente in `bookings_test.go` del Task 4 seguente. Questo piano la definisce nel Task 3 per tenere il file di test completo in un unico posto da leggere, ma va spostata prima di eseguire Step 5 qui:

- [ ] **Step 4b: Spostare temporaneamente il test che dipende da CancelBooking**

Rimuovi la funzione `TestCreateBooking_AllowsSamePhoneAfterCancellation` da `bookings_test.go` per ora (taglia il blocco), tienila da parte: la reincollerai nel Task 4 dopo aver implementato `CancelBooking`.

- [ ] **Step 5: Eseguire i test e verificare che passino**

Run: (stesso comando dello Step 2)
Expected: PASS (con `TestCreateBooking_AllowsSamePhoneAfterCancellation` temporaneamente rimosso)

- [ ] **Step 6: Commit**

```bash
git add backend/internal/events/bookings.go backend/internal/events/bookings_test.go backend/internal/events/store.go
git commit -m "feat: add booking creation with atomic availability guard"
```

---

### Task 4: Repository prenotazioni — LookupBooking, CancelBooking, ListBookingsForEvent

**Files:**
- Modify: `backend/internal/events/bookings.go`
- Modify: `backend/internal/events/bookings_test.go`

**Interfaces:**
- Consumes: `events.Store`, `events.Booking`, `events.ErrNotFound` (Task 2/3).
- Produces: `(*Store).LookupBooking(ctx, email, code string) (Booking, error)`, `(*Store).CancelBooking(ctx, id int64, email, code string) (Booking, error)`, `events.BookingWithGame{Booking; GameName string}`, `(*Store).ListBookingsForEvent(ctx, eventID int64) ([]BookingWithGame, error)`, `events.ErrInvalidBookingCredentials` (già dichiarato nel Task 3).

- [ ] **Step 1: Reincollare il test rimandato e aggiungere i nuovi test**

Riaggiungi in fondo a `backend/internal/events/bookings_test.go` la funzione `TestCreateBooking_AllowsSamePhoneAfterCancellation` rimossa nel Task 3, poi aggiungi:
```go
func TestLookupBooking_FindsActiveBookingByEmailAndCode(t *testing.T) {
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

	created, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "Mario@Example.com", "3331234567", now)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	found, err := eventStore.LookupBooking(ctx, "mario@example.com", created.BookingCode)
	if err != nil {
		t.Fatalf("lookup with lowercase email: %v", err)
	}
	if found.ID != created.ID {
		t.Fatalf("expected booking %d, got %d", created.ID, found.ID)
	}
}

func TestLookupBooking_WrongEmailOrCodeReturnsGenericError(t *testing.T) {
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

	if _, err := eventStore.LookupBooking(ctx, "wrong@example.com", created.BookingCode); !errors.Is(err, events.ErrInvalidBookingCredentials) {
		t.Fatalf("expected ErrInvalidBookingCredentials for wrong email, got %v", err)
	}
	if _, err := eventStore.LookupBooking(ctx, "mario@example.com", "WRONGCOD"); !errors.Is(err, events.ErrInvalidBookingCredentials) {
		t.Fatalf("expected ErrInvalidBookingCredentials for wrong code, got %v", err)
	}
}

func TestLookupBooking_CancelledBookingIsNotFound(t *testing.T) {
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
	if _, err := eventStore.CancelBooking(ctx, created.ID, created.ParticipantEmail, created.BookingCode); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	if _, err := eventStore.LookupBooking(ctx, "mario@example.com", created.BookingCode); !errors.Is(err, events.ErrInvalidBookingCredentials) {
		t.Fatalf("expected a cancelled booking to be unreachable by lookup, got %v", err)
	}
}

func TestCancelBooking_RejectsWrongCredentials(t *testing.T) {
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

	_, err = eventStore.CancelBooking(ctx, created.ID, "wrong@example.com", created.BookingCode)
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

func TestListBookingsForEvent_ReturnsOnlyActiveWithGameName(t *testing.T) {
	eventStore, gameStore := newTestStore(t)
	ctx := context.Background()
	gameID := mustCreateGame(t, gameStore, "Catan")
	event, err := eventStore.CreateEvent(ctx, "Serata giochi", nil, "2026-10-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 2}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := eventStore.ListEventGames(ctx, event.ID)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	active, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Mario Rossi", "mario@example.com", "3331111111", now)
	if err != nil {
		t.Fatalf("create active booking: %v", err)
	}
	toCancel, err := eventStore.CreateBooking(ctx, event.ID, eventGames[0].ID, "Luigi Verdi", "luigi@example.com", "3332222222", now)
	if err != nil {
		t.Fatalf("create booking to cancel: %v", err)
	}
	if _, err := eventStore.CancelBooking(ctx, toCancel.ID, toCancel.ParticipantEmail, toCancel.BookingCode); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	list, err := eventStore.ListBookingsForEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("list bookings: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 active booking, got %d", len(list))
	}
	if list[0].ID != active.ID || list[0].GameName != "Catan" {
		t.Fatalf("unexpected booking: %+v", list[0])
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

Run: (stesso comando del Task 3, Step 2)
Expected: FAIL (`LookupBooking`/`CancelBooking`/`ListBookingsForEvent` non esistono)

- [ ] **Step 3: Implementare**

Aggiungi in coda a `backend/internal/events/bookings.go` (serve anche l'import `"strings"`):
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

type BookingWithGame struct {
	Booking
	GameName string
}

func (s *Store) ListBookingsForEvent(ctx context.Context, eventID int64) ([]BookingWithGame, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT b.id, b.event_id, b.event_game_id, b.participant_name, b.participant_email, b.participant_phone,
		        b.booking_code, b.status, b.created_at, g.name
		 FROM bookings b
		 JOIN event_games eg ON b.event_game_id = eg.id
		 JOIN games g ON eg.game_id = g.id
		 WHERE b.event_id = ? AND b.status = 'active'
		 ORDER BY b.created_at`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BookingWithGame
	for rows.Next() {
		var bg BookingWithGame
		var createdAt string
		if err := rows.Scan(&bg.ID, &bg.EventID, &bg.EventGameID, &bg.ParticipantName, &bg.ParticipantEmail,
			&bg.ParticipantPhone, &bg.BookingCode, &bg.Status, &createdAt, &bg.GameName); err != nil {
			return nil, err
		}
		bg.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		out = append(out, bg)
	}
	return out, rows.Err()
}
```

Aggiungi `"strings"` all'elenco import di `bookings.go`.

- [ ] **Step 4: Eseguire i test e verificare che passino**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/events/... -v
```
Expected: PASS (tutti i test del pacchetto `events`, inclusi quelli dei Task 2 e 3)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/events/bookings.go backend/internal/events/bookings_test.go
git commit -m "feat: add booking lookup, cancellation and admin listing"
```

---

### Task 5: Rate limiter HTTP minimale

**Files:**
- Create: `backend/internal/httpapi/ratelimit.go`
- Create: `backend/internal/httpapi/ratelimit_test.go`

**Interfaces:**
- Produces: `httpapi.newRateLimiter(max int, window time.Duration) *rateLimiter` (non esportato, usato solo internamente da `router.go`), `(*rateLimiter).middleware(next http.Handler) http.Handler`.

- [ ] **Step 1: Scrivere i test**

`backend/internal/httpapi/ratelimit_test.go`:
```go
package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterAllow_BlocksAfterMaxWithinWindow(t *testing.T) {
	rl := newRateLimiter(2, time.Minute)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	if !rl.allow("1.2.3.4", now) {
		t.Fatal("expected first attempt to be allowed")
	}
	if !rl.allow("1.2.3.4", now) {
		t.Fatal("expected second attempt to be allowed")
	}
	if rl.allow("1.2.3.4", now) {
		t.Fatal("expected third attempt within the window to be blocked")
	}
}

func TestRateLimiterAllow_ResetsAfterWindow(t *testing.T) {
	rl := newRateLimiter(1, time.Minute)
	start := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	if !rl.allow("1.2.3.4", start) {
		t.Fatal("expected first attempt to be allowed")
	}
	if rl.allow("1.2.3.4", start.Add(30*time.Second)) {
		t.Fatal("expected attempt still within the window to be blocked")
	}
	if !rl.allow("1.2.3.4", start.Add(90*time.Second)) {
		t.Fatal("expected attempt after the window to be allowed again")
	}
}

func TestRateLimiterAllow_TracksKeysIndependently(t *testing.T) {
	rl := newRateLimiter(1, time.Minute)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	if !rl.allow("1.2.3.4", now) {
		t.Fatal("expected first IP's first attempt to be allowed")
	}
	if !rl.allow("5.6.7.8", now) {
		t.Fatal("expected a different IP's first attempt to be allowed regardless of the first IP's count")
	}
}

func TestRateLimiterMiddleware_Returns429WhenExceeded(t *testing.T) {
	rl := newRateLimiter(1, time.Minute)
	handler := rl.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "9.9.9.9:12345"

	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected first request to pass through, got %d", rec1.Code)
	}

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec2.Code)
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/... -run RateLimiter -v
```
Expected: FAIL (`newRateLimiter` non esiste)

- [ ] **Step 3: Implementare**

`backend/internal/httpapi/ratelimit.go`:
```go
package httpapi

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// rateLimiter is a minimal per-key sliding-window limiter, in-memory and
// unbounded in size. That is an acceptable trade-off for a self-hosted,
// single-instance deployment with a small user base; it is not meant to
// survive a restart or to scale past one process.
type rateLimiter struct {
	mu       sync.Mutex
	max      int
	window   time.Duration
	attempts map[string][]time.Time
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{max: max, window: window, attempts: make(map[string][]time.Time)}
}

func (rl *rateLimiter) allow(key string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := now.Add(-rl.window)
	kept := rl.attempts[key][:0]
	for _, t := range rl.attempts[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= rl.max {
		rl.attempts[key] = kept
		return false
	}
	rl.attempts[key] = append(kept, now)
	return true
}

func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if !rl.allow(host, time.Now()) {
			writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 4: Eseguire i test e verificare che passino**

Run: (stesso comando dello Step 2, senza `-run`)
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/... -run RateLimiter -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/httpapi/ratelimit.go backend/internal/httpapi/ratelimit_test.go
git commit -m "feat: add minimal in-memory per-IP rate limiter"
```

---

### Task 6: HTTP — Server wiring + lettura pubblica eventi

**Files:**
- Modify: `backend/internal/httpapi/router.go`
- Modify: `backend/internal/httpapi/middleware_auth.go`
- Modify: `backend/internal/httpapi/testhelpers_test.go`
- Create: `backend/internal/httpapi/events_responses.go`
- Create: `backend/internal/httpapi/events_handlers.go`
- Create: `backend/internal/httpapi/events_handlers_test.go`

**Interfaces:**
- Consumes: `events.Store`, `events.Event`, `events.EventGame`, `events.ErrNotFound` (Task 2); `games.Store.GetGame` (già esistente).
- Produces: `Server.Events *events.Store` (nuovo campo), `(*Server).hasAdminSession(r *http.Request) bool`, `toEventSummary(e events.Event) map[string]any`, `(*Server).toEventDetail(ctx, e events.Event) (map[string]any, error)`, handler `listEventsHandler`, `getEventHandler` registrati su `GET /api/events` e `GET /api/events/{id}` (pubblici).

- [ ] **Step 1: Aggiungere il campo Events al Server e wiring nei test**

In `backend/internal/httpapi/router.go`, aggiungi l'import `"boardgames-manager/internal/events"` e il campo:
```go
type Server struct {
	Users    *users.Store
	Sessions *auth.SessionStore
	Settings *settings.Store
	Games    *games.Store
	Events   *events.Store
	Storage  *storage.Store
	BGG      bgg.Client
}
```

In `backend/internal/httpapi/testhelpers_test.go`, aggiungi l'import `"boardgames-manager/internal/events"` e il campo nella costruzione del server:
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

- [ ] **Step 2: Aggiungere l'helper di autenticazione opzionale**

In `backend/internal/httpapi/middleware_auth.go`, aggiungi in coda al file:
```go
// hasAdminSession reports whether the request carries a currently valid
// admin session, without requiring one (unlike requireAuth, it never writes
// an error response). Used by handlers that serve different data to the
// public than to an authenticated admin on the same route.
func (s *Server) hasAdminSession(r *http.Request) bool {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		return false
	}
	tokenHash := auth.HashToken(cookie.Value)
	_, err = s.Sessions.GetValidByTokenHash(r.Context(), tokenHash)
	return err == nil
}
```

- [ ] **Step 3: Scrivere i test degli handler**

`backend/internal/httpapi/events_handlers_test.go`:
```go
package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/httpapi"
)

func createTestGameForEvent(t *testing.T, gamesStore *games.Store, name string) int64 {
	t.Helper()
	g, err := gamesStore.CreateGame(context.Background(), games.Game{Name: name})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	return g.ID
}

func TestListEvents_PublicSeesOnlyFutureEvents(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")

	if _, err := server.Events.CreateEvent(context.Background(), "Passato", nil, "2020-01-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}}); err != nil {
		t.Fatalf("create past event: %v", err)
	}
	if _, err := server.Events.CreateEvent(context.Background(), "Futuro", nil, "2099-01-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}}); err != nil {
		t.Fatalf("create future event: %v", err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/events", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body []struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 1 || body[0].Title != "Futuro" {
		t.Fatalf("expected only the future event for an anonymous request, got %+v", body)
	}
}

func TestListEvents_AdminSeesPastEventsToo(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Catan")

	if _, err := server.Events.CreateEvent(context.Background(), "Passato", nil, "2020-01-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}}); err != nil {
		t.Fatalf("create past event: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body []struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 1 || body[0].Title != "Passato" {
		t.Fatalf("expected the past event to be visible to an admin, got %+v", body)
	}
}

func TestGetEvent_ReturnsGamesWithRemainingCapacity(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")

	event, err := server.Events.CreateEvent(context.Background(), "Serata giochi", nil, "2099-01-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 3}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/events/%d", event.ID), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Title string `json:"title"`
		Games []struct {
			Name      string `json:"name"`
			Quantity  int    `json:"quantity"`
			Remaining int    `json:"remaining"`
		} `json:"games"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Title != "Serata giochi" || len(body.Games) != 1 {
		t.Fatalf("unexpected body: %+v", body)
	}
	if body.Games[0].Name != "Catan" || body.Games[0].Quantity != 3 || body.Games[0].Remaining != 3 {
		t.Fatalf("unexpected game entry: %+v", body.Games[0])
	}
}

func TestGetEvent_NotFound(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/events/999", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
```

- [ ] **Step 4: Eseguire i test e verificare che falliscano**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/... -run "Events" -v
```
Expected: FAIL (compilazione: `listEventsHandler`/`getEventHandler` non esistono)

- [ ] **Step 5: Implementare le response e gli handler**

`backend/internal/httpapi/events_responses.go`:
```go
package httpapi

import (
	"context"

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/games"
)

func toEventSummary(e events.Event) map[string]any {
	return map[string]any{
		"id": e.ID, "title": e.Title, "description": e.Description,
		"eventDate": e.EventDate, "startTime": e.StartTime,
	}
}

func toEventGameSummary(eventGameID int64, g games.Game, quantity, remaining int) map[string]any {
	return map[string]any{
		"eventGameId": eventGameID, "gameId": g.ID, "name": g.Name, "coverPath": g.CoverPath,
		"quantity": quantity, "remaining": remaining,
	}
}

func (s *Server) toEventDetail(ctx context.Context, e events.Event) (map[string]any, error) {
	eventGames, err := s.Events.ListEventGames(ctx, e.ID)
	if err != nil {
		return nil, err
	}

	gamesOut := make([]map[string]any, 0, len(eventGames))
	for _, eg := range eventGames {
		game, err := s.Games.GetGame(ctx, eg.GameID)
		if err != nil {
			return nil, err
		}
		remaining, err := s.Events.RemainingCapacity(ctx, eg.ID)
		if err != nil {
			return nil, err
		}
		gamesOut = append(gamesOut, toEventGameSummary(eg.ID, game, eg.Quantity, remaining))
	}

	detail := toEventSummary(e)
	detail["games"] = gamesOut
	return detail, nil
}

func toBookingResponse(b events.Booking) map[string]any {
	return map[string]any{
		"id": b.ID, "eventId": b.EventID, "eventGameId": b.EventGameID,
		"participantName": b.ParticipantName, "bookingCode": b.BookingCode, "status": b.Status,
	}
}

func toBookingAdminResponse(b events.BookingWithGame) map[string]any {
	return map[string]any{
		"id": b.ID, "gameName": b.GameName, "participantName": b.ParticipantName,
		"participantEmail": b.ParticipantEmail, "participantPhone": b.ParticipantPhone,
		"createdAt": b.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
```

`backend/internal/httpapi/events_handlers.go`:
```go
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"boardgames-manager/internal/events"
)

func (s *Server) listEventsHandler(w http.ResponseWriter, r *http.Request) {
	includePast := s.hasAdminSession(r)
	list, err := s.Events.ListEvents(r.Context(), includePast, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list events")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, e := range list {
		out = append(out, toEventSummary(e))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getEventHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	event, err := s.Events.GetEvent(r.Context(), id)
	if errors.Is(err, events.ErrNotFound) {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load event")
		return
	}
	detail, err := s.toEventDetail(r.Context(), event)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not build response")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

type eventGameRequest struct {
	GameID   int64 `json:"gameId"`
	Quantity int   `json:"quantity"`
}

type eventRequest struct {
	Title       string             `json:"title"`
	Description *string            `json:"description"`
	EventDate   string             `json:"eventDate"`
	StartTime   string             `json:"startTime"`
	Games       []eventGameRequest `json:"games"`
}

func toEventGameInputs(in []eventGameRequest) []events.EventGameInput {
	out := make([]events.EventGameInput, 0, len(in))
	for _, g := range in {
		out = append(out, events.EventGameInput{GameID: g.GameID, Quantity: g.Quantity})
	}
	return out
}

func decodeEventRequest(r *http.Request) (eventRequest, bool) {
	var req eventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return eventRequest{}, false
	}
	return req, req.Title != "" && req.EventDate != "" && req.StartTime != ""
}
```

Nota: `createEventHandler`, `updateEventHandler`, `deleteEventHandler`, `listEventBookingsHandler` (protetti) e la loro registrazione nel router arrivano nel Task 7 — questo task si ferma alla lettura pubblica.

- [ ] **Step 6: Registrare le route pubbliche nel router**

In `backend/internal/httpapi/router.go`, dentro `NewRouter`, subito dopo `r.Get("/api/uploads/{filename}", s.getUploadHandler)`:
```go
	r.Get("/api/events", s.listEventsHandler)
	r.Get("/api/events/{id}", s.getEventHandler)
```

- [ ] **Step 7: Eseguire i test e verificare che passino**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/... -v
```
Expected: PASS (tutta la suite `httpapi`, non solo `-run Events`, per assicurarsi che il wiring del `Server` non abbia rotto nulla)

- [ ] **Step 8: Commit**

```bash
git add backend/internal/httpapi/router.go backend/internal/httpapi/middleware_auth.go backend/internal/httpapi/testhelpers_test.go backend/internal/httpapi/events_responses.go backend/internal/httpapi/events_handlers.go backend/internal/httpapi/events_handlers_test.go
git commit -m "feat: add public event listing and detail endpoints"
```

---

### Task 7: HTTP — CRUD evento admin + elenco prenotazioni

**Files:**
- Modify: `backend/internal/httpapi/events_handlers.go`
- Modify: `backend/internal/httpapi/events_handlers_test.go`
- Modify: `backend/internal/httpapi/router.go`

**Interfaces:**
- Consumes: `events.Store.CreateEvent/UpdateEvent/DeleteEvent/ListBookingsForEvent`, `events.ErrGameNotFound`, `events.ErrQuantityBelowActiveBookings` (Task 2/4); `eventRequest`, `decodeEventRequest`, `toEventGameInputs` (Task 6).
- Produces: handler `createEventHandler`, `updateEventHandler`, `deleteEventHandler`, `listEventBookingsHandler`, registrati protetti su `POST/PUT/DELETE /api/events[/{id}]` e `GET /api/events/{id}/bookings`.

- [ ] **Step 1: Aggiungere i test**

Aggiungi `"bytes"` all'elenco import di `backend/internal/httpapi/events_handlers_test.go` (i nuovi test inviano un body con `bytes.NewReader`), poi aggiungi in fondo al file:
```go
func TestCreateEvent_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	payload, _ := json.Marshal(map[string]any{"title": "Serata", "eventDate": "2099-01-01", "startTime": "20:00"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/events", bytes.NewReader(payload)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCreateEvent_Succeeds(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Catan")

	payload, _ := json.Marshal(map[string]any{
		"title": "Serata giochi", "eventDate": "2099-01-01", "startTime": "20:00",
		"games": []map[string]any{{"gameId": gameID, "quantity": 2}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/events", bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateEvent_RejectsQuantityBelowActiveBookings(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Catan")

	event, err := server.Events.CreateEvent(context.Background(), "Serata giochi", nil, "2099-01-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 2}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := server.Events.ListEventGames(context.Background(), event.ID)
	if err := server.Events.TestInsertBooking(event.ID, eventGames[0].ID, "active"); err != nil {
		t.Fatalf("insert booking fixture: %v", err)
	}
	if err := server.Events.TestInsertBooking(event.ID, eventGames[0].ID, "active"); err != nil {
		t.Fatalf("insert booking fixture: %v", err)
	}

	payload, _ := json.Marshal(map[string]any{
		"title": "Serata giochi", "eventDate": "2099-01-01", "startTime": "20:00",
		"games": []map[string]any{{"gameId": gameID, "quantity": 1}},
	})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/events/%d", event.ID), bytes.NewReader(payload))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteEvent_Succeeds(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Catan")

	event, err := server.Events.CreateEvent(context.Background(), "Serata giochi", nil, "2099-01-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/events/%d", event.ID), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestListEventBookings_RequiresAuth(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/events/1/bookings", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestListEventBookings_ReturnsActiveBookings(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	cookie := bootstrapFirstAdmin(t, router, "admin@example.com", "supersecret1")
	gameID := createTestGameForEvent(t, server.Games, "Catan")

	event, err := server.Events.CreateEvent(context.Background(), "Serata giochi", nil, "2099-01-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: 1}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventGames, _ := server.Events.ListEventGames(context.Background(), event.ID)
	if err := server.Events.TestInsertBooking(event.ID, eventGames[0].ID, "active"); err != nil {
		t.Fatalf("insert booking fixture: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/events/%d/bookings", event.ID), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body []struct {
		GameName string `json:"gameName"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 1 || body[0].GameName != "Catan" {
		t.Fatalf("unexpected body: %+v", body)
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/... -run "Event" -v
```
Expected: FAIL (404/routing: gli handler protetti non sono ancora registrati)

- [ ] **Step 3: Implementare gli handler**

Aggiungi in fondo a `backend/internal/httpapi/events_handlers.go`:
```go
func (s *Server) createEventHandler(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeEventRequest(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "title, eventDate and startTime are required")
		return
	}
	event, err := s.Events.CreateEvent(r.Context(), req.Title, req.Description, req.EventDate, req.StartTime, toEventGameInputs(req.Games))
	if errors.Is(err, events.ErrGameNotFound) {
		writeError(w, http.StatusBadRequest, "one of the selected games does not exist")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create event")
		return
	}
	writeJSON(w, http.StatusCreated, toEventSummary(event))
}

func (s *Server) updateEventHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	req, ok := decodeEventRequest(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "title, eventDate and startTime are required")
		return
	}
	event, err := s.Events.UpdateEvent(r.Context(), id, req.Title, req.Description, req.EventDate, req.StartTime, toEventGameInputs(req.Games))
	switch {
	case errors.Is(err, events.ErrNotFound):
		writeError(w, http.StatusNotFound, "event not found")
	case errors.Is(err, events.ErrGameNotFound):
		writeError(w, http.StatusBadRequest, "one of the selected games does not exist")
	case errors.Is(err, events.ErrQuantityBelowActiveBookings):
		writeError(w, http.StatusConflict, "quantity is below the number of active bookings for that game")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not update event")
	default:
		writeJSON(w, http.StatusOK, toEventSummary(event))
	}
}

func (s *Server) deleteEventHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	if err := s.Events.DeleteEvent(r.Context(), id); errors.Is(err, events.ErrNotFound) {
		writeError(w, http.StatusNotFound, "event not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete event")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) listEventBookingsHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return
	}
	list, err := s.Events.ListBookingsForEvent(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list bookings")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, b := range list {
		out = append(out, toBookingAdminResponse(b))
	}
	writeJSON(w, http.StatusOK, out)
}
```

- [ ] **Step 4: Registrare le route protette**

In `backend/internal/httpapi/router.go`, dentro `r.Group(func(protected chi.Router) { ... })`, in coda al blocco esistente:
```go
		protected.Post("/api/events", s.createEventHandler)
		protected.Put("/api/events/{id}", s.updateEventHandler)
		protected.Delete("/api/events/{id}", s.deleteEventHandler)
		protected.Get("/api/events/{id}/bookings", s.listEventBookingsHandler)
```

- [ ] **Step 5: Eseguire i test e verificare che passino**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/... -v
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi/events_handlers.go backend/internal/httpapi/events_handlers_test.go backend/internal/httpapi/router.go
git commit -m "feat: add admin event CRUD and booking list endpoints"
```

---

### Task 8: HTTP — prenotazione pubblica (create/lookup/cancel)

**Files:**
- Create: `backend/internal/httpapi/events_bookings_handlers.go`
- Create: `backend/internal/httpapi/events_bookings_handlers_test.go`
- Modify: `backend/internal/httpapi/router.go`

**Interfaces:**
- Consumes: `events.Store.CreateBooking/LookupBooking/CancelBooking`, `events.Err*` (Task 3/4); `toBookingResponse` (Task 6); `newRateLimiter` (Task 5).
- Produces: handler `createBookingHandler`, `lookupBookingHandler`, `cancelBookingHandler`, registrati pubblici su `POST /api/events/{id}/bookings`, `POST /api/bookings/lookup`, `POST /api/bookings/{id}/cancel` (gli ultimi due dietro rate limiter).

- [ ] **Step 1: Scrivere i test**

`backend/internal/httpapi/events_bookings_handlers_test.go`:
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

	"boardgames-manager/internal/events"
	"boardgames-manager/internal/httpapi"
)

func createTestEvent(t *testing.T, server *httpapi.Server, gameID int64, quantity int) int64 {
	t.Helper()
	event, err := server.Events.CreateEvent(context.Background(), "Serata giochi", nil, "2099-01-01", "20:00",
		[]events.EventGameInput{{GameID: gameID, Quantity: quantity}})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	return event.ID
}

func TestCreateBooking_Succeeds(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 2)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)

	payload, _ := json.Marshal(map[string]any{
		"eventGameId": eventGames[0].ID, "participantName": "Mario Rossi",
		"participantEmail": "mario@example.com", "participantPhone": "3331234567",
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), bytes.NewReader(payload)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		BookingCode string `json:"bookingCode"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.BookingCode) != 8 {
		t.Fatalf("expected an 8-char booking code, got %q", body.BookingCode)
	}
}

func TestCreateBooking_SoldOutReturns409(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)
	gameID := createTestGameForEvent(t, server.Games, "Catan")
	eventID := createTestEvent(t, server, gameID, 1)
	eventGames, _ := server.Events.ListEventGames(context.Background(), eventID)
	if err := server.Events.TestInsertBooking(eventID, eventGames[0].ID, "active"); err != nil {
		t.Fatalf("insert booking fixture: %v", err)
	}

	payload, _ := json.Marshal(map[string]any{
		"eventGameId": eventGames[0].ID, "participantName": "Mario Rossi",
		"participantEmail": "mario@example.com", "participantPhone": "3331234567",
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/events/%d/bookings", eventID), bytes.NewReader(payload)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLookupAndCancelBooking_FullFlow(t *testing.T) {
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
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		ID          int64  `json:"id"`
		BookingCode string `json:"bookingCode"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	lookupPayload, _ := json.Marshal(map[string]string{"email": "mario@example.com", "bookingCode": created.BookingCode})
	lookupRec := httptest.NewRecorder()
	router.ServeHTTP(lookupRec, httptest.NewRequest(http.MethodPost, "/api/bookings/lookup", bytes.NewReader(lookupPayload)))
	if lookupRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", lookupRec.Code, lookupRec.Body.String())
	}

	cancelPayload, _ := json.Marshal(map[string]string{"email": "mario@example.com", "bookingCode": created.BookingCode})
	cancelRec := httptest.NewRecorder()
	router.ServeHTTP(cancelRec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/bookings/%d/cancel", created.ID), bytes.NewReader(cancelPayload)))
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", cancelRec.Code, cancelRec.Body.String())
	}
	var cancelled struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(cancelRec.Body).Decode(&cancelled); err != nil {
		t.Fatalf("decode cancel: %v", err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("expected status cancelled, got %q", cancelled.Status)
	}
}

func TestLookupBooking_WrongCredentialsReturns404(t *testing.T) {
	server := newTestServer(t)
	router := httpapi.NewRouter(server)

	payload, _ := json.Marshal(map[string]string{"email": "nobody@example.com", "bookingCode": "AAAAAAAA"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/bookings/lookup", bytes.NewReader(payload)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Eseguire i test e verificare che falliscano**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./internal/httpapi/... -run "Booking" -v
```
Expected: FAIL (404: gli handler non sono ancora registrati)

- [ ] **Step 3: Implementare**

`backend/internal/httpapi/events_bookings_handlers.go`:
```go
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"boardgames-manager/internal/events"
)

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
		writeError(w, http.StatusConflict, "non ci sono più copie disponibili per questo gioco")
	case errors.Is(err, events.ErrDuplicatePhoneBooking):
		writeError(w, http.StatusConflict, "hai già una prenotazione attiva per questo evento")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not create booking")
	default:
		writeJSON(w, http.StatusCreated, toBookingResponse(booking))
	}
}

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
	if errors.Is(err, events.ErrInvalidBookingCredentials) {
		writeError(w, http.StatusNotFound, "prenotazione non trovata")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not look up booking")
		return
	}
	writeJSON(w, http.StatusOK, toBookingResponse(booking))
}

func (s *Server) cancelBookingHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid booking id")
		return
	}
	var req bookingCredentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.BookingCode == "" {
		writeError(w, http.StatusBadRequest, "email and bookingCode are required")
		return
	}
	booking, err := s.Events.CancelBooking(r.Context(), id, req.Email, req.BookingCode)
	if errors.Is(err, events.ErrInvalidBookingCredentials) || errors.Is(err, events.ErrNotFound) {
		writeError(w, http.StatusNotFound, "prenotazione non trovata")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not cancel booking")
		return
	}
	writeJSON(w, http.StatusOK, toBookingResponse(booking))
}
```

- [ ] **Step 4: Registrare le route pubbliche, con rate limit su lookup/cancel**

In `backend/internal/httpapi/router.go`, aggiungi l'import `"time"` e dichiara il limiter dentro `NewRouter`, prima di `r.Get("/api/health", ...)`:
```go
	bookingCredentialsLimiter := newRateLimiter(10, time.Minute)
```
Poi, subito dopo le due route `GET /api/events...` aggiunte nel Task 6:
```go
	r.Post("/api/events/{id}/bookings", s.createBookingHandler)
	r.With(bookingCredentialsLimiter.middleware).Post("/api/bookings/lookup", s.lookupBookingHandler)
	r.With(bookingCredentialsLimiter.middleware).Post("/api/bookings/{id}/cancel", s.cancelBookingHandler)
```

- [ ] **Step 5: Eseguire i test e verificare che passino**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./... -v
```
Expected: PASS (intera suite backend)

- [ ] **Step 6: Commit**

```bash
git add backend/internal/httpapi/events_bookings_handlers.go backend/internal/httpapi/events_bookings_handlers_test.go backend/internal/httpapi/router.go
git commit -m "feat: add public booking creation, lookup and cancellation endpoints"
```

---

### Task 9: Wiring cmd/server + router frontend per le route pubbliche

**Files:**
- Modify: `backend/cmd/server/main.go`
- Modify: `frontend/src/router/index.ts`
- Create: `frontend/src/components/PublicHeader.vue`
- Modify: `frontend/src/app.css`

**Interfaces:**
- Consumes: `events.NewStore` (Task 2); `useAuthStore` (già esistente).
- Produces: `Server.Events` cablato in produzione; route Vue con `meta: { public: true }`; componente `PublicHeader.vue` riusato dalle view pubbliche dei task successivi.

- [ ] **Step 1: Wiring in cmd/server/main.go**

In `backend/cmd/server/main.go`, aggiungi l'import `"boardgames-manager/internal/events"` e il campo nel costruttore del server:
```go
	server := &httpapi.Server{
		Users:    users.NewStore(conn),
		Sessions: auth.NewSessionStore(conn),
		Settings: settings.NewStore(conn),
		Games:    games.NewStore(conn),
		Events:   events.NewStore(conn),
		Storage:  storage.NewStore(dataDir + "/uploads"),
		BGG:      bgg.NewHTTPClient(),
	}
```

- [ ] **Step 2: Verificare che il backend compili**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go build ./...
```
Expected: nessun errore

- [ ] **Step 3: Creare PublicHeader.vue**

`frontend/src/components/PublicHeader.vue`:
```vue
<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const router = useRouter()

async function logout() {
  try {
    await auth.logout()
  } catch (e) {
    console.error('logout request failed', e)
  }
  router.push({ name: 'events' })
}
</script>

<template>
  <header class="public-header">
    <router-link :to="{ name: 'events' }" class="brand">BoardGames Manager</router-link>
    <nav>
      <router-link :to="{ name: 'manage-booking' }">Gestisci prenotazione</router-link>
      <template v-if="auth.user">
        <router-link to="/games">Area admin</router-link>
        <button type="button" @click="logout">Esci</button>
      </template>
    </nav>
  </header>
</template>
```

- [ ] **Step 4: Aggiornare il router**

Sostituisci l'intero contenuto di `frontend/src/router/index.ts`:
```ts
import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import SetupView from '../views/SetupView.vue'
import LoginView from '../views/LoginView.vue'
import DashboardLayout from '../views/DashboardLayout.vue'
import UsersView from '../views/UsersView.vue'
import SettingsView from '../views/SettingsView.vue'
import GamesView from '../views/GamesView.vue'
import GameNewView from '../views/GameNewView.vue'
import GameDetailView from '../views/GameDetailView.vue'
import EventsView from '../views/EventsView.vue'
import EventDetailView from '../views/EventDetailView.vue'
import ManageBookingView from '../views/ManageBookingView.vue'
import EventsAdminView from '../views/EventsAdminView.vue'
import EventNewView from '../views/EventNewView.vue'
import EventAdminDetailView from '../views/EventAdminDetailView.vue'

declare module 'vue-router' {
  interface RouteMeta {
    public?: boolean
  }
}

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/setup', name: 'setup', component: SetupView },
    { path: '/login', name: 'login', component: LoginView },
    { path: '/', name: 'events', component: EventsView, meta: { public: true } },
    { path: '/events/:id', name: 'event-detail', component: EventDetailView, meta: { public: true } },
    { path: '/manage-booking', name: 'manage-booking', component: ManageBookingView, meta: { public: true } },
    { path: '/games/:id', name: 'game-detail', component: GameDetailView, meta: { public: true } },
    {
      path: '/admin',
      component: DashboardLayout,
      children: [
        { path: '', redirect: '/users' },
        { path: 'events', name: 'admin-events', component: EventsAdminView },
        { path: 'events/new', name: 'admin-event-new', component: EventNewView },
        { path: 'events/:id', name: 'admin-event-detail', component: EventAdminDetailView },
      ],
    },
    {
      path: '/',
      component: DashboardLayout,
      children: [
        { path: 'users', name: 'users', component: UsersView },
        { path: 'games', name: 'games', component: GamesView },
        { path: 'games/new', name: 'game-new', component: GameNewView },
        { path: 'settings', name: 'settings', component: SettingsView },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (!auth.checked) {
    // checkStatus already swallows a backend outage, but a rejected guard
    // promise means a blank page, so never let one escape from here.
    try {
      await auth.checkStatus()
    } catch (e) {
      console.error('auth status check failed', e)
    }
  }

  if (auth.needsSetup && to.name !== 'setup') {
    return { name: 'setup' }
  }
  if (!auth.needsSetup && to.name === 'setup') {
    return { name: 'users' }
  }
  if (to.meta.public) {
    return true
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

Questo piano registra `EventsView`/`EventDetailView`/`ManageBookingView`/`EventsAdminView`/`EventNewView`/`EventAdminDetailView` prima ancora che i file esistano (arrivano nei Task 10–16): finché non sono creati, `npm run build` fallirà con "file not found" — è atteso, e viene risolto mano a mano che i task successivi creano quei file. Per tenere QUESTO task verificabile da solo, crea nel frattempo dei placeholder minimi (verranno sovrascritti dai task successivi):

- [ ] **Step 5: Placeholder temporanei per le view non ancora create**

Crea ciascuno di questi sei file con esattamente questo contenuto (verranno sovrascritti dai Task 11–16, qui servono solo a far compilare il router):
```vue
<template>
  <div>Placeholder</div>
</template>
```
- `frontend/src/views/EventsView.vue`
- `frontend/src/views/EventDetailView.vue`
- `frontend/src/views/ManageBookingView.vue`
- `frontend/src/views/EventsAdminView.vue`
- `frontend/src/views/EventNewView.vue`
- `frontend/src/views/EventAdminDetailView.vue`

- [ ] **Step 6: Aggiungere gli stili base per le pagine pubbliche**

In coda a `frontend/src/app.css`:
```css
/* ---------- Public shell (PublicHeader.vue + top-level public views) ---------- */

.public-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 0 1.5rem;
  height: 3.25rem;
  background: var(--surface);
  border-bottom: 1px solid var(--border);
}

.public-header .brand {
  font-weight: 600;
  text-decoration: none;
  color: var(--text);
}

.public-header nav {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.public-header nav a {
  padding: 0.4rem 0.75rem;
  border-radius: var(--radius);
  color: var(--text-muted);
  text-decoration: none;
  font-weight: 500;
}

.public-header nav a:hover {
  background: var(--bg);
  color: var(--text);
}

.public-header nav button {
  background: transparent;
  color: var(--text-muted);
  border: 1px solid var(--border);
}

.public-header nav button:hover {
  background: var(--bg);
  color: var(--text);
}

.public-page {
  width: 100%;
  max-width: 52rem;
  margin: 0 auto;
  padding: 2rem 1.5rem 3rem;
}

.event-list,
.event-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 1rem;
  list-style: none;
  padding: 0;
}

.event-list li,
.event-grid li {
  border: 1px solid #d0d5dd;
  border-radius: 8px;
  padding: 1rem;
}

.event-list a,
.event-grid a {
  text-decoration: none;
  color: inherit;
}

.event-games {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 1rem;
  list-style: none;
  padding: 0;
  margin: 1rem 0;
}

.event-games li {
  border: 1px solid #d0d5dd;
  border-radius: 8px;
  padding: 1rem;
  text-align: center;
}

.event-games img {
  max-width: 100%;
  border-radius: 4px;
  margin-bottom: 0.5rem;
}

.booking-code {
  font-size: 1.75rem;
  font-weight: 700;
  letter-spacing: 0.15em;
}

.game-select-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.game-select-row input[type='number'] {
  width: 5rem;
}
```

- [ ] **Step 7: Verificare che il frontend compili**

Run (dalla directory `frontend/`):
```bash
npm run build
```
Expected: build riuscita (i placeholder rendono valide tutte le nuove route)

- [ ] **Step 8: Commit**

```bash
git add backend/cmd/server/main.go frontend/src/router/index.ts frontend/src/components/PublicHeader.vue frontend/src/app.css frontend/src/views/EventsView.vue frontend/src/views/EventDetailView.vue frontend/src/views/ManageBookingView.vue frontend/src/views/EventsAdminView.vue frontend/src/views/EventNewView.vue frontend/src/views/EventAdminDetailView.vue
git commit -m "feat: wire Events store into production server, add public routing shell"
```

---

### Task 10: GameDetailView.vue — lettura pubblica per anonimi

**Files:**
- Modify: `frontend/src/views/GameDetailView.vue`

**Interfaces:**
- Consumes: `useAuthStore` (già esistente), `PublicHeader.vue` (Task 9).
- Produces: `game-detail` (`/games/:id`) pienamente utilizzabile in lettura da un visitatore anonimo (nessun controllo di modifica visibile), invariato per un admin autenticato.

- [ ] **Step 1: Sostituire l'intero file**

`frontend/src/views/GameDetailView.vue` (file completo aggiornato — aggiunge `useAuthStore`, `PublicHeader`, e nasconde dietro `v-if="auth.user"` ogni controllo di modifica: upload copertina, elimina gioco, form di modifica nome/descrizione (sostituito da testo semplice per anonimi), aggiungi lingua, upload manuale, aggiungi media, rimuovi media):
```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import { useAuthStore } from '../stores/auth'
import PublicHeader from '../components/PublicHeader.vue'

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
const auth = useAuthStore()
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

const coverFile = ref<File | null>(null)
const coverError = ref('')

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

function onCoverFileSelected(event: Event) {
  const target = event.target as HTMLInputElement
  coverFile.value = target.files?.[0] || null
}

async function uploadCover() {
  coverError.value = ''
  if (!coverFile.value) {
    coverError.value = 'Seleziona un immagine'
    return
  }
  const formData = new FormData()
  formData.append('file', coverFile.value)
  try {
    await api.post(`/games/${gameId}/cover`, formData)
    coverFile.value = null
    await load()
  } catch (e) {
    coverError.value = (e as Error).message
  }
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
  <div>
    <PublicHeader />
    <div class="public-page" v-if="game">
      <h1>{{ game.name }}</h1>
      <img v-if="game.coverPath" :src="`/api/uploads/${game.coverPath}`" :alt="game.name" class="cover" />

      <form v-if="auth.user" @submit.prevent="uploadCover">
        <label>
          Copertina (JPEG, PNG o WebP, max 5MB)
          <input type="file" accept="image/jpeg,image/png,image/webp" @change="onCoverFileSelected" />
        </label>
        <button type="submit">Carica copertina</button>
        <p v-if="coverError" class="error">{{ coverError }}</p>
      </form>

      <p v-if="game.owner">Proprietario: {{ game.owner }}</p>
      <button v-if="auth.user" type="button" @click="deleteGame">Elimina gioco</button>

      <nav class="language-tabs">
        <button
          v-for="l in game.languages"
          :key="l.code"
          type="button"
          :class="{ active: l.code === activeLangCode }"
          @click="selectLanguage(l.code)"
        >
          {{ l.code }}{{ l.isBaseLanguage ? ' ★' : '' }}
        </button>
      </nav>

      <template v-if="auth.user">
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
      </template>
      <template v-else>
        <h2>{{ activeLanguage()?.name }}</h2>
        <p v-if="activeLanguage()?.description">{{ activeLanguage()?.description }}</p>
      </template>

      <h2>Manuale e tutorial ({{ activeLangCode }})</h2>
      <ul>
        <li v-for="m in activeLanguage()?.media || []" :key="m.id">
          <a v-if="m.type === 'file'" :href="`/api/uploads/${m.url}`" target="_blank">{{ m.title || 'Manuale' }}</a>
          <a v-else :href="m.url" target="_blank">{{ m.title || m.url }}</a>
          ({{ m.type }})
          <button v-if="auth.user" type="button" @click="removeMedia(m.id)">Rimuovi</button>
        </li>
      </ul>

      <template v-if="auth.user">
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
      </template>

      <p v-if="error" class="error">{{ error }}</p>
    </div>
  </div>
</template>
```

- [ ] **Step 2: Verificare che il frontend compili**

Run (da `frontend/`):
```bash
npm run build
```
Expected: build riuscita

- [ ] **Step 3: Commit**

```bash
git add frontend/src/views/GameDetailView.vue
git commit -m "feat: make GameDetailView readable by anonymous visitors"
```

---

### Task 11: EventsView.vue — home pubblica

**Files:**
- Modify: `frontend/src/views/EventsView.vue` (sostituisce il placeholder del Task 9)

**Interfaces:**
- Consumes: `GET /api/events` (Task 6), `PublicHeader.vue` (Task 9).

- [ ] **Step 1: Implementare**

`frontend/src/views/EventsView.vue`:
```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'
import PublicHeader from '../components/PublicHeader.vue'

interface EventSummary {
  id: number
  title: string
  description: string | null
  eventDate: string
  startTime: string
}

const events = ref<EventSummary[]>([])
const error = ref('')

async function loadEvents() {
  try {
    events.value = await api.get<EventSummary[]>('/events')
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(loadEvents)
</script>

<template>
  <div>
    <PublicHeader />
    <div class="public-page">
      <h1>Prossimi eventi</h1>
      <p v-if="error" class="error">{{ error }}</p>
      <p v-if="!error && events.length === 0">Nessun evento in programma.</p>
      <ul class="event-list">
        <li v-for="e in events" :key="e.id">
          <router-link :to="`/events/${e.id}`">
            <h2>{{ e.title }}</h2>
            <p>{{ e.eventDate }} · {{ e.startTime }}</p>
          </router-link>
        </li>
      </ul>
    </div>
  </div>
</template>
```

- [ ] **Step 2: Verificare che il frontend compili**

Run (da `frontend/`): `npm run build`
Expected: build riuscita

- [ ] **Step 3: Commit**

```bash
git add frontend/src/views/EventsView.vue
git commit -m "feat: add public events home page"
```

---

### Task 12: EventDetailView.vue — dettaglio evento + prenotazione

**Files:**
- Modify: `frontend/src/views/EventDetailView.vue` (sostituisce il placeholder del Task 9)

**Interfaces:**
- Consumes: `GET /api/events/{id}`, `POST /api/events/{id}/bookings` (Task 6/8), `PublicHeader.vue` (Task 9).

- [ ] **Step 1: Implementare**

`frontend/src/views/EventDetailView.vue`:
```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import PublicHeader from '../components/PublicHeader.vue'

interface EventGameInfo {
  eventGameId: number
  gameId: number
  name: string
  coverPath: string | null
  remaining: number
}

interface EventDetail {
  id: number
  title: string
  description: string | null
  eventDate: string
  startTime: string
  games: EventGameInfo[]
}

interface BookingResult {
  id: number
  bookingCode: string
}

const route = useRoute()
const eventId = route.params.id as string

const event = ref<EventDetail | null>(null)
const error = ref('')

const selectedEventGameId = ref<number | null>(null)
const participantName = ref('')
const participantEmail = ref('')
const participantPhone = ref('')
const bookingError = ref('')
const bookingResult = ref<BookingResult | null>(null)

async function load() {
  event.value = await api.get<EventDetail>(`/events/${eventId}`)
}

function startBooking(eventGameId: number) {
  selectedEventGameId.value = eventGameId
  bookingError.value = ''
  bookingResult.value = null
}

async function submitBooking() {
  bookingError.value = ''
  if (selectedEventGameId.value === null) {
    return
  }
  try {
    bookingResult.value = await api.post<BookingResult>(`/events/${eventId}/bookings`, {
      eventGameId: selectedEventGameId.value,
      participantName: participantName.value,
      participantEmail: participantEmail.value,
      participantPhone: participantPhone.value,
    })
    await load()
  } catch (e) {
    bookingError.value = (e as Error).message
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
    <PublicHeader />
    <div class="public-page" v-if="event">
      <h1>{{ event.title }}</h1>
      <p v-if="event.description">{{ event.description }}</p>
      <p>{{ event.eventDate }} · {{ event.startTime }}</p>

      <ul class="event-games">
        <li v-for="g in event.games" :key="g.eventGameId">
          <img v-if="g.coverPath" :src="`/api/uploads/${g.coverPath}`" :alt="g.name" />
          <router-link :to="`/games/${g.gameId}`">{{ g.name }}</router-link>
          <p>Disponibilità: {{ g.remaining }}</p>
          <button type="button" :disabled="g.remaining <= 0" @click="startBooking(g.eventGameId)">
            Prenota
          </button>
        </li>
      </ul>

      <form v-if="selectedEventGameId !== null && !bookingResult" @submit.prevent="submitBooking">
        <label>
          Nome
          <input v-model="participantName" required />
        </label>
        <label>
          Email
          <input v-model="participantEmail" type="email" required />
        </label>
        <label>
          Telefono
          <input v-model="participantPhone" required />
        </label>
        <button type="submit">Conferma prenotazione</button>
        <p v-if="bookingError" class="error">{{ bookingError }}</p>
      </form>

      <div v-if="bookingResult" class="success">
        <p>Prenotazione confermata! Il tuo codice è:</p>
        <p class="booking-code">{{ bookingResult.bookingCode }}</p>
        <p>Conservalo insieme alla tua email per gestire la prenotazione da "Gestisci prenotazione".</p>
      </div>

      <p v-if="error" class="error">{{ error }}</p>
    </div>
  </div>
</template>
```

- [ ] **Step 2: Verificare che il frontend compili**

Run (da `frontend/`): `npm run build`
Expected: build riuscita

- [ ] **Step 3: Commit**

```bash
git add frontend/src/views/EventDetailView.vue
git commit -m "feat: add public event detail page with booking form"
```

---

### Task 13: ManageBookingView.vue — gestione prenotazione

**Files:**
- Modify: `frontend/src/views/ManageBookingView.vue` (sostituisce il placeholder del Task 9)

**Interfaces:**
- Consumes: `POST /api/bookings/lookup`, `POST /api/bookings/{id}/cancel` (Task 8), `PublicHeader.vue` (Task 9).

- [ ] **Step 1: Implementare**

`frontend/src/views/ManageBookingView.vue`:
```vue
<script setup lang="ts">
import { ref } from 'vue'
import { api } from '../api/client'
import PublicHeader from '../components/PublicHeader.vue'

interface BookingResult {
  id: number
  eventId: number
  eventGameId: number
  participantName: string
  bookingCode: string
  status: 'active' | 'cancelled'
}

const email = ref('')
const bookingCode = ref('')
const booking = ref<BookingResult | null>(null)
const error = ref('')
const cancelMessage = ref('')

async function lookup() {
  error.value = ''
  cancelMessage.value = ''
  try {
    booking.value = await api.post<BookingResult>('/bookings/lookup', {
      email: email.value,
      bookingCode: bookingCode.value,
    })
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
      email: email.value,
      bookingCode: bookingCode.value,
    })
    cancelMessage.value = 'Prenotazione annullata.'
  } catch (e) {
    error.value = (e as Error).message
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
          Email
          <input v-model="email" type="email" required />
        </label>
        <label>
          Codice prenotazione
          <input v-model="bookingCode" required />
        </label>
        <button type="submit">Cerca</button>
      </form>
      <p v-if="error" class="error">{{ error }}</p>

      <div v-if="booking">
        <p>Prenotazione per {{ booking.participantName }} — stato: {{ booking.status }}</p>
        <button v-if="booking.status === 'active'" type="button" @click="cancel">
          Annulla prenotazione
        </button>
        <p v-if="cancelMessage" class="success">{{ cancelMessage }}</p>
      </div>
    </div>
  </div>
</template>
```

- [ ] **Step 2: Verificare che il frontend compili**

Run (da `frontend/`): `npm run build`
Expected: build riuscita

- [ ] **Step 3: Commit**

```bash
git add frontend/src/views/ManageBookingView.vue
git commit -m "feat: add manage-booking page (lookup and cancellation)"
```

---

### Task 14: Admin — EventsAdminView.vue (elenco)

**Files:**
- Modify: `frontend/src/views/EventsAdminView.vue` (sostituisce il placeholder del Task 9)

**Interfaces:**
- Consumes: `GET /api/events` (Task 6, versione admin con eventi passati).

- [ ] **Step 1: Implementare**

`frontend/src/views/EventsAdminView.vue`:
```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'

interface EventSummary {
  id: number
  title: string
  eventDate: string
  startTime: string
}

const events = ref<EventSummary[]>([])
const error = ref('')

async function loadEvents() {
  try {
    events.value = await api.get<EventSummary[]>('/events')
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(loadEvents)
</script>

<template>
  <div>
    <h1>Eventi</h1>
    <router-link to="/admin/events/new">Crea evento</router-link>
    <p v-if="error" class="error">{{ error }}</p>
    <ul class="event-grid">
      <li v-for="e in events" :key="e.id">
        <router-link :to="`/admin/events/${e.id}`">
          <h2>{{ e.title }}</h2>
          <p>{{ e.eventDate }} · {{ e.startTime }}</p>
        </router-link>
      </li>
    </ul>
  </div>
</template>
```

- [ ] **Step 2: Verificare che il frontend compili**

Run (da `frontend/`): `npm run build`
Expected: build riuscita

- [ ] **Step 3: Commit**

```bash
git add frontend/src/views/EventsAdminView.vue
git commit -m "feat: add admin events list page"
```

---

### Task 15: Admin — EventNewView.vue (creazione)

**Files:**
- Modify: `frontend/src/views/EventNewView.vue` (sostituisce il placeholder del Task 9)

**Interfaces:**
- Consumes: `GET /api/games` (già esistente), `POST /api/events` (Task 7).

- [ ] **Step 1: Implementare**

`frontend/src/views/EventNewView.vue`:
```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client'

interface GameSummary {
  id: number
  name: string
}

interface SelectedGame {
  gameId: number
  quantity: number
}

const router = useRouter()

const title = ref('')
const description = ref('')
const eventDate = ref('')
const startTime = ref('')
const error = ref('')

const availableGames = ref<GameSummary[]>([])
const selectedGames = ref<SelectedGame[]>([])

async function loadGames() {
  availableGames.value = await api.get<GameSummary[]>('/games')
}

function isSelected(gameId: number) {
  return selectedGames.value.some((g) => g.gameId === gameId)
}

function toggleGame(gameId: number, checked: boolean) {
  if (checked) {
    selectedGames.value.push({ gameId, quantity: 1 })
  } else {
    selectedGames.value = selectedGames.value.filter((g) => g.gameId !== gameId)
  }
}

function quantityFor(gameId: number) {
  return selectedGames.value.find((g) => g.gameId === gameId)?.quantity ?? 1
}

function setQuantity(gameId: number, quantity: number) {
  const entry = selectedGames.value.find((g) => g.gameId === gameId)
  if (entry) {
    entry.quantity = quantity
  }
}

async function createEvent() {
  error.value = ''
  try {
    const event = await api.post<{ id: number }>('/events', {
      title: title.value,
      description: description.value || null,
      eventDate: eventDate.value,
      startTime: startTime.value,
      games: selectedGames.value,
    })
    router.push(`/admin/events/${event.id}`)
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(loadGames)
</script>

<template>
  <div>
    <h1>Crea evento</h1>
    <form @submit.prevent="createEvent">
      <label>
        Titolo
        <input v-model="title" required />
      </label>
      <label>
        Descrizione
        <textarea v-model="description"></textarea>
      </label>
      <label>
        Data
        <input v-model="eventDate" type="date" required />
      </label>
      <label>
        Ora
        <input v-model="startTime" type="time" required />
      </label>

      <fieldset>
        <legend>Giochi</legend>
        <div v-for="g in availableGames" :key="g.id" class="game-select-row">
          <label>
            <input
              type="checkbox"
              :checked="isSelected(g.id)"
              @change="toggleGame(g.id, ($event.target as HTMLInputElement).checked)"
            />
            {{ g.name }}
          </label>
          <input
            v-if="isSelected(g.id)"
            type="number"
            min="1"
            :value="quantityFor(g.id)"
            @input="setQuantity(g.id, Number(($event.target as HTMLInputElement).value))"
          />
        </div>
      </fieldset>

      <button type="submit">Crea</button>
      <p v-if="error" class="error">{{ error }}</p>
    </form>
  </div>
</template>
```

- [ ] **Step 2: Verificare che il frontend compili**

Run (da `frontend/`): `npm run build`
Expected: build riuscita

- [ ] **Step 3: Commit**

```bash
git add frontend/src/views/EventNewView.vue
git commit -m "feat: add admin event creation page"
```

---

### Task 16: Admin — EventAdminDetailView.vue (modifica + prenotazioni)

**Files:**
- Modify: `frontend/src/views/EventAdminDetailView.vue` (sostituisce il placeholder del Task 9)

**Interfaces:**
- Consumes: `GET /api/events/{id}` (versione admin, con `quantity`), `PUT /api/events/{id}`, `GET /api/events/{id}/bookings` (Task 6/7).

- [ ] **Step 1: Implementare**

`frontend/src/views/EventAdminDetailView.vue`:
```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'

interface GameSummary {
  id: number
  name: string
}

interface SelectedGame {
  gameId: number
  quantity: number
}

interface EventGameInfo {
  eventGameId: number
  gameId: number
  name: string
  quantity: number
  remaining: number
}

interface EventDetail {
  id: number
  title: string
  description: string | null
  eventDate: string
  startTime: string
  games: EventGameInfo[]
}

interface BookingAdminInfo {
  id: number
  gameName: string
  participantName: string
  participantEmail: string
  participantPhone: string
  createdAt: string
}

const route = useRoute()
const eventId = route.params.id as string

const title = ref('')
const description = ref('')
const eventDate = ref('')
const startTime = ref('')
const error = ref('')
const saveMessage = ref('')

const availableGames = ref<GameSummary[]>([])
const selectedGames = ref<SelectedGame[]>([])
const bookings = ref<BookingAdminInfo[]>([])

function isSelected(gameId: number) {
  return selectedGames.value.some((g) => g.gameId === gameId)
}

function toggleGame(gameId: number, checked: boolean) {
  if (checked) {
    selectedGames.value.push({ gameId, quantity: 1 })
  } else {
    selectedGames.value = selectedGames.value.filter((g) => g.gameId !== gameId)
  }
}

function quantityFor(gameId: number) {
  return selectedGames.value.find((g) => g.gameId === gameId)?.quantity ?? 1
}

function setQuantity(gameId: number, quantity: number) {
  const entry = selectedGames.value.find((g) => g.gameId === gameId)
  if (entry) {
    entry.quantity = quantity
  }
}

async function load() {
  const [event, games] = await Promise.all([
    api.get<EventDetail>(`/events/${eventId}`),
    api.get<GameSummary[]>('/games'),
  ])
  title.value = event.title
  description.value = event.description || ''
  eventDate.value = event.eventDate
  startTime.value = event.startTime
  availableGames.value = games
  selectedGames.value = event.games.map((g) => ({ gameId: g.gameId, quantity: g.quantity }))
  bookings.value = await api.get<BookingAdminInfo[]>(`/events/${eventId}/bookings`)
}

async function saveEvent() {
  error.value = ''
  saveMessage.value = ''
  try {
    await api.put(`/events/${eventId}`, {
      title: title.value,
      description: description.value || null,
      eventDate: eventDate.value,
      startTime: startTime.value,
      games: selectedGames.value,
    })
    saveMessage.value = 'Salvato'
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
    <h1>Modifica evento</h1>
    <form @submit.prevent="saveEvent">
      <label>
        Titolo
        <input v-model="title" required />
      </label>
      <label>
        Descrizione
        <textarea v-model="description"></textarea>
      </label>
      <label>
        Data
        <input v-model="eventDate" type="date" required />
      </label>
      <label>
        Ora
        <input v-model="startTime" type="time" required />
      </label>

      <fieldset>
        <legend>Giochi</legend>
        <div v-for="g in availableGames" :key="g.id" class="game-select-row">
          <label>
            <input
              type="checkbox"
              :checked="isSelected(g.id)"
              @change="toggleGame(g.id, ($event.target as HTMLInputElement).checked)"
            />
            {{ g.name }}
          </label>
          <input
            v-if="isSelected(g.id)"
            type="number"
            min="1"
            :value="quantityFor(g.id)"
            @input="setQuantity(g.id, Number(($event.target as HTMLInputElement).value))"
          />
        </div>
      </fieldset>

      <button type="submit">Salva</button>
      <p v-if="saveMessage" class="success">{{ saveMessage }}</p>
      <p v-if="error" class="error">{{ error }}</p>
    </form>

    <h2>Prenotazioni attive</h2>
    <p v-if="bookings.length === 0">Nessuna prenotazione ancora.</p>
    <ul>
      <li v-for="b in bookings" :key="b.id">
        {{ b.participantName }} — {{ b.gameName }} — {{ b.participantEmail }} — {{ b.participantPhone }}
      </li>
    </ul>
  </div>
</template>
```

- [ ] **Step 2: Verificare che il frontend compili**

Run (da `frontend/`): `npm run build`
Expected: build riuscita

- [ ] **Step 3: Commit**

```bash
git add frontend/src/views/EventAdminDetailView.vue
git commit -m "feat: add admin event edit page with bookings list"
```

---

### Task 17: Verifica end-to-end finale

**Files:** nessuno (solo verifica)

- [ ] **Step 1: Eseguire l'intera suite backend**

Run:
```bash
docker run --rm -v "$(pwd)/backend":/app -v bgm-gomodcache:/go/pkg/mod -v bgm-gocache:/root/.cache/go-build -w /app golang:1.25 go test ./... -v
```
Expected: PASS

- [ ] **Step 2: Ricostruire ed avviare l'applicazione**

```bash
docker compose build && docker compose up -d
curl -s http://localhost:8080/api/health
```
Expected: `{"status":"ok"}`

- [ ] **Step 3: Verificare il flusso completo in browser**

1. Visita `/` **senza** essere loggato: deve mostrare l'elenco eventi pubblico (vuoto), non un redirect a `/login`.
2. Login come admin (o bootstrap se è il primo avvio) su `/login`.
3. Vai su `/games`, crea un gioco manualmente (senza token BGG) se non ce n'è già uno.
4. Vai su `/admin/events` → "Crea evento": titolo, data futura, ora, seleziona il gioco con quantità 1 → conferma redirect al dettaglio admin dell'evento.
5. Apri una finestra anonima (o fai logout): vai su `/`, verifica che l'evento appena creato compaia.
6. Apri il dettaglio pubblico dell'evento, verifica copertina/nome/disponibilità del gioco, clicca sul nome del gioco → verifica che porti al dettaglio pubblico del gioco (sola lettura, nessun controllo di modifica visibile).
7. Torna al dettaglio evento, clicca "Prenota", compila nome/email/telefono, conferma → verifica che appaia il `booking_code` a schermo.
8. Ricarica il dettaglio evento: verifica che la disponibilità sia scesa a 0 e il pulsante "Prenota" sia disabilitato.
9. Prova a prenotare di nuovo con lo stesso telefono ma un altro gioco (se disponibile) o lo stesso gioco: verifica il messaggio di conflitto (una prenotazione attiva per telefono per evento).
10. Vai su "Gestisci prenotazione", inserisci l'email e il `booking_code` salvati al passo 7 → verifica che i dettagli della prenotazione appaiano.
11. Clicca "Annulla prenotazione" → verifica il messaggio di conferma.
12. Torna al dettaglio evento: verifica che la disponibilità sia tornata a 1.
13. Da loggato come admin, apri il dettaglio evento in `/admin/events/{id}`: verifica che l'elenco prenotazioni attive rifletta lo stato corrente (vuoto, dato che l'unica prenotazione è stata annullata).
14. Controlla la console del browser: nessun errore oltre agli avvisi cosmetici già noti.

- [ ] **Step 4: Arrestare l'ambiente e ripulire i dati di test**

```bash
docker compose down
rm -rf data/
```

- [ ] **Step 5: Annotare l'esito**

Se tutti i passaggi sono verificati con successo, la Fase 3 è completa.
