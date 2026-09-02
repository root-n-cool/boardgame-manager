package events

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// testBookingCounter ensures unique booking_code and participant_phone values
// across repeated TestInsertBooking calls. Without it, multiple insertions with
// the same eventGameID and status would generate identical codes, violating:
// - booking_code UNIQUE constraint
// - idx_one_active_booking_per_phone_per_event partial unique index
// (The brief's original code used only eventGameID*10+len(status), which collides
// when called twice with identical parameters.)
var testBookingCounter int64

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

func (s *Store) GetEventGame(ctx context.Context, id int64) (EventGame, error) {
	var eg EventGame
	err := s.db.QueryRowContext(ctx,
		`SELECT id, event_id, game_id, quantity FROM event_games WHERE id = ?`, id,
	).Scan(&eg.ID, &eg.EventID, &eg.GameID, &eg.Quantity)
	if errors.Is(err, sql.ErrNoRows) {
		return EventGame{}, ErrNotFound
	}
	return eg, err
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
	// cancelled bookings for that game/event pair. Any match result for a
	// cancelled booking was already deleted by CancelBooking, so this
	// cascade only ever drops bookings/results that were correctly
	// considered void — it does not silently erase live historical data.
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

func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
