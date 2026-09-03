package events

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
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
	// concurrent bookings for the last remaining seat — no separate
	// check-then-insert window. A collision on the (event_id, phone) unique
	// index (a duplicate booking that slipped past an earlier read) surfaces
	// here too and is mapped to ErrDuplicatePhoneBooking below.
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO bookings (event_id, event_game_id, participant_name, participant_email, participant_phone, booking_code, status)
		 SELECT ?, ?, ?, ?, ?, ?, 'active'
		 WHERE (SELECT COUNT(*) FROM bookings WHERE event_game_id = ? AND status = 'active') <
		       (SELECT seats FROM event_games WHERE id = ?)`,
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

func (s *Store) CancelBooking(ctx context.Context, id int64, code string) (Booking, error) {
	b, err := s.getBookingByID(ctx, id)
	if err != nil {
		return Booking{}, err
	}
	if b.Status != BookingStatusActive || strings.ToUpper(strings.TrimSpace(code)) != b.BookingCode {
		return Booking{}, ErrInvalidBookingCredentials
	}
	return s.cancelBooking(ctx, b)
}

// AdminCancelBooking cancels a booking on the organiser's behalf: same effect
// as the participant cancelling with their own code, minus the code check —
// an admin is already authenticated. A booking that does not exist or was
// already cancelled is ErrNotFound, so the caller can answer 404 either way.
func (s *Store) AdminCancelBooking(ctx context.Context, id int64) (Booking, error) {
	b, err := s.getBookingByID(ctx, id)
	if err != nil {
		return Booking{}, err
	}
	if b.Status != BookingStatusActive {
		return Booking{}, ErrNotFound
	}
	return s.cancelBooking(ctx, b)
}

// cancelBooking is the single cancellation path, shared by the participant's
// code-authenticated cancel and the admin's: both must free the copy and drop
// the score in the same transaction, so neither can drift from the other.
func (s *Store) cancelBooking(ctx context.Context, b Booking) (Booking, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Booking{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE bookings SET status = ? WHERE id = ?`, BookingStatusCancelled, b.ID); err != nil {
		return Booking{}, err
	}
	// Un tavolo condiviso non deve poter essere azzerato da chi si sfila:
	// il risultato è di tutti quelli che ci sono ancora seduti. Solo quando
	// il tavolo si svuota il punteggio non ha più senso e va via — e questo
	// evita anche il doppio conteggio se il posto liberato viene riprenotato
	// e riscritto.
	var remainingActive int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bookings WHERE event_game_id = ? AND status = 'active' AND id != ?`,
		b.EventGameID, b.ID,
	).Scan(&remainingActive); err != nil {
		return Booking{}, err
	}
	if remainingActive == 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM match_results WHERE event_game_id = ?`, b.EventGameID); err != nil {
			return Booking{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return Booking{}, err
	}
	b.Status = BookingStatusCancelled
	return b, nil
}

type BookingWithGame struct {
	Booking
	GameID   int64
	GameName string
	// CopyIndex e Seats servono a chi legge per tavolo: l'organizzatore
	// vuole vedere "D&D #2 — 3 di 5 posti prenotabili", non una lista piatta.
	CopyIndex int
	Seats     int
}

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

// CountActiveBookingsForEventGame dice quante persone siedono a un tavolo.
// La pagina pubblica se ne serve per spiegare che il punteggio è condiviso.
func (s *Store) CountActiveBookingsForEventGame(ctx context.Context, eventGameID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bookings WHERE event_game_id = ? AND status = 'active'`, eventGameID,
	).Scan(&count)
	return count, err
}

// CountEventGameCopies says how many copies of a game the event carries. A page
// that only ever sees one booking cannot count them itself, and it needs the
// number to decide whether the copy index is worth showing: on an evening with
// one copy per game, "#1" is noise.
func (s *Store) CountEventGameCopies(ctx context.Context, eventID, gameID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM event_games WHERE event_id = ? AND game_id = ?`, eventID, gameID,
	).Scan(&count)
	return count, err
}
