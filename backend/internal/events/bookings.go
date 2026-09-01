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
