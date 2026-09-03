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
	EventGameID int64
	SubmittedAt time.Time
	Players     []PlayerScore
}

var (
	ErrEmptyPlayers     = errors.New("at least one player is required")
	ErrBookingNotActive = errors.New("booking is not active")
)

// SubmitMatchResult creates or replaces the MatchResult for the copy the
// booking sits on. A copy can only ever have one MatchResult
// (event_game_id is UNIQUE): calling this again for any booking at the
// same table replaces the previously submitted players instead of adding
// a second result — this is both "il punteggio è sempre modificabile"
// (design spec) and what keeps the leaderboard from counting one game of
// D&D once per participant.
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
	err = tx.QueryRowContext(ctx, `SELECT id FROM match_results WHERE event_game_id = ?`, b.EventGameID).Scan(&matchResultID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		res, err := tx.ExecContext(ctx, `INSERT INTO match_results (event_game_id) VALUES (?)`, b.EventGameID)
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
		`SELECT id, event_game_id, submitted_at FROM match_results WHERE id = ?`, id,
	).Scan(&m.ID, &m.EventGameID, &submittedAt)
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

// GetMatchResultForEventGame returns nil (with no error) if nobody at that
// table has submitted a result yet — "not played yet" is a normal,
// expected state here, unlike ErrNotFound elsewhere in this package.
func (s *Store) GetMatchResultForEventGame(ctx context.Context, eventGameID int64) (*MatchResult, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM match_results WHERE event_game_id = ?`, eventGameID).Scan(&id)
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

type EventGameMatchResult struct {
	EventGameID int64
	GameID      int64
	GameName    string
	CopyIndex   int
	Players     []PlayerScore
}

// ListMatchResultsForEvent returns one row per copy of the event that has a
// result, with the game, its copy number and the players/scores submitted —
// read-only data for the admin event detail page. It is keyed by copy and
// not by participant because the result belongs to the table: who typed it
// in is not a fact worth storing.
func (s *Store) ListMatchResultsForEvent(ctx context.Context, eventID int64) ([]EventGameMatchResult, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT eg.id, g.id, g.name, eg.copy_index, mps.player_name, mps.score
		 FROM match_results mr
		 JOIN event_games eg ON mr.event_game_id = eg.id
		 JOIN games g ON eg.game_id = g.id
		 JOIN match_player_scores mps ON mps.match_result_id = mr.id
		 WHERE eg.event_id = ?
		 ORDER BY eg.game_id, eg.copy_index, mps.id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EventGameMatchResult
	var current *EventGameMatchResult
	for rows.Next() {
		var eventGameID, gameID int64
		var gameName, playerName string
		var copyIndex, score int
		if err := rows.Scan(&eventGameID, &gameID, &gameName, &copyIndex, &playerName, &score); err != nil {
			return nil, err
		}
		if current == nil || current.EventGameID != eventGameID {
			out = append(out, EventGameMatchResult{EventGameID: eventGameID, GameID: gameID,
				GameName: gameName, CopyIndex: copyIndex})
			current = &out[len(out)-1]
		}
		current.Players = append(current.Players, PlayerScore{Name: playerName, Score: score})
	}
	return out, rows.Err()
}
