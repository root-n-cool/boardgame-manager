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
// across all events it was played at. One MatchResult is one table, so a
// game of D&D booked by six people still counts as a single match.
// Winner and player-stats computation happens in Go rather than SQL: each
// match's winner(s) depend on comparing scores within that match only,
// which is awkward to express as a portable SQL aggregate.
func (s *Store) GetLeaderboard(ctx context.Context, gameID int64) (Leaderboard, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT mr.id, e.title, e.event_date, e.start_time, mps.player_name, mps.score
		 FROM match_player_scores mps
		 JOIN match_results mr ON mps.match_result_id = mr.id
		 JOIN event_games eg ON mr.event_game_id = eg.id
		 JOIN events e ON eg.event_id = e.id
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
