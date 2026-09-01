package games

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type Game struct {
	ID              int64
	BGGID           *string
	Name            string
	Year            *int
	MinPlayers      *int
	MaxPlayers      *int
	PlaytimeMinutes *int
	Owner           *string
	CoverPath       *string
	CreatedAt       time.Time
}

type GameLanguage struct {
	ID             int64
	GameID         int64
	LanguageCode   string
	IsBaseLanguage bool
	Name           string
	Description    *string
}

type GameUpdate struct {
	Owner           *string
	Year            *int
	MinPlayers      *int
	MaxPlayers      *int
	PlaytimeMinutes *int
}

var ErrNotFound = errors.New("not found")
var ErrDuplicateLanguage = errors.New("language already exists for this game")
var ErrGameInUse = errors.New("game is used by one or more events")

type Store struct {
	db *sql.DB
}

func NewStore(conn *sql.DB) *Store {
	return &Store{db: conn}
}

func (s *Store) CreateGame(ctx context.Context, g Game) (Game, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO games (bgg_id, name, year, min_players, max_players, playtime_minutes, owner, cover_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		g.BGGID, g.Name, g.Year, g.MinPlayers, g.MaxPlayers, g.PlaytimeMinutes, g.Owner, g.CoverPath,
	)
	if err != nil {
		return Game{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Game{}, err
	}
	return s.GetGame(ctx, id)
}

func (s *Store) GetGame(ctx context.Context, id int64) (Game, error) {
	var g Game
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, bgg_id, name, year, min_players, max_players, playtime_minutes, owner, cover_path, created_at
		 FROM games WHERE id = ?`, id,
	).Scan(&g.ID, &g.BGGID, &g.Name, &g.Year, &g.MinPlayers, &g.MaxPlayers, &g.PlaytimeMinutes, &g.Owner, &g.CoverPath, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Game{}, ErrNotFound
	}
	if err != nil {
		return Game{}, err
	}
	g.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return g, nil
}

func (s *Store) ListGames(ctx context.Context) ([]Game, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, bgg_id, name, year, min_players, max_players, playtime_minutes, owner, cover_path, created_at
		 FROM games ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Game
	for rows.Next() {
		var g Game
		var createdAt string
		if err := rows.Scan(&g.ID, &g.BGGID, &g.Name, &g.Year, &g.MinPlayers, &g.MaxPlayers, &g.PlaytimeMinutes, &g.Owner, &g.CoverPath, &createdAt); err != nil {
			return nil, err
		}
		g.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) UpdateGame(ctx context.Context, id int64, upd GameUpdate) (Game, error) {
	current, err := s.GetGame(ctx, id)
	if err != nil {
		return Game{}, err
	}
	if upd.Owner != nil {
		current.Owner = upd.Owner
	}
	if upd.Year != nil {
		current.Year = upd.Year
	}
	if upd.MinPlayers != nil {
		current.MinPlayers = upd.MinPlayers
	}
	if upd.MaxPlayers != nil {
		current.MaxPlayers = upd.MaxPlayers
	}
	if upd.PlaytimeMinutes != nil {
		current.PlaytimeMinutes = upd.PlaytimeMinutes
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE games SET owner = ?, year = ?, min_players = ?, max_players = ?, playtime_minutes = ? WHERE id = ?`,
		current.Owner, current.Year, current.MinPlayers, current.MaxPlayers, current.PlaytimeMinutes, id,
	)
	if err != nil {
		return Game{}, err
	}
	return s.GetGame(ctx, id)
}

func (s *Store) UpdateCoverPath(ctx context.Context, id int64, path string) (Game, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE games SET cover_path = ? WHERE id = ?`, path, id)
	if err != nil {
		return Game{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Game{}, err
	}
	if affected == 0 {
		return Game{}, ErrNotFound
	}
	return s.GetGame(ctx, id)
}

func (s *Store) DeleteGame(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM games WHERE id = ?`, id)
	if err != nil {
		if isForeignKeyConstraintErr(err) {
			return ErrGameInUse
		}
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

func (s *Store) CreateLanguage(ctx context.Context, gl GameLanguage) (GameLanguage, error) {
	isBase := 0
	if gl.IsBaseLanguage {
		isBase = 1
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO game_languages (game_id, language_code, is_base_language, name, description)
		 VALUES (?, ?, ?, ?, ?)`,
		gl.GameID, gl.LanguageCode, isBase, gl.Name, gl.Description,
	)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return GameLanguage{}, ErrDuplicateLanguage
		}
		return GameLanguage{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return GameLanguage{}, err
	}
	return s.getLanguageByID(ctx, id)
}

func (s *Store) getLanguageByID(ctx context.Context, id int64) (GameLanguage, error) {
	var gl GameLanguage
	var isBase int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, game_id, language_code, is_base_language, name, description FROM game_languages WHERE id = ?`, id,
	).Scan(&gl.ID, &gl.GameID, &gl.LanguageCode, &isBase, &gl.Name, &gl.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return GameLanguage{}, ErrNotFound
	}
	if err != nil {
		return GameLanguage{}, err
	}
	gl.IsBaseLanguage = isBase != 0
	return gl, nil
}

func (s *Store) GetLanguage(ctx context.Context, gameID int64, code string) (GameLanguage, error) {
	var gl GameLanguage
	var isBase int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, game_id, language_code, is_base_language, name, description
		 FROM game_languages WHERE game_id = ? AND language_code = ?`, gameID, code,
	).Scan(&gl.ID, &gl.GameID, &gl.LanguageCode, &isBase, &gl.Name, &gl.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return GameLanguage{}, ErrNotFound
	}
	if err != nil {
		return GameLanguage{}, err
	}
	gl.IsBaseLanguage = isBase != 0
	return gl, nil
}

func (s *Store) ListLanguages(ctx context.Context, gameID int64) ([]GameLanguage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, game_id, language_code, is_base_language, name, description
		 FROM game_languages WHERE game_id = ? ORDER BY id`, gameID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GameLanguage
	for rows.Next() {
		var gl GameLanguage
		var isBase int
		if err := rows.Scan(&gl.ID, &gl.GameID, &gl.LanguageCode, &isBase, &gl.Name, &gl.Description); err != nil {
			return nil, err
		}
		gl.IsBaseLanguage = isBase != 0
		out = append(out, gl)
	}
	return out, rows.Err()
}

func (s *Store) UpdateLanguage(ctx context.Context, gameID int64, code string, name string, description *string) (GameLanguage, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE game_languages SET name = ?, description = ? WHERE game_id = ? AND language_code = ?`,
		name, description, gameID, code,
	)
	if err != nil {
		return GameLanguage{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return GameLanguage{}, err
	}
	if affected == 0 {
		return GameLanguage{}, ErrNotFound
	}
	return s.GetLanguage(ctx, gameID, code)
}

func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func isForeignKeyConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}
