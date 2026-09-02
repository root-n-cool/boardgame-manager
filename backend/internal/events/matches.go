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
