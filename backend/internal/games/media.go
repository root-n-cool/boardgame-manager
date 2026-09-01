package games

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const (
	MediaTypeFile    = "file"
	MediaTypeLink    = "link"
	MediaTypeYoutube = "youtube"
)

type GameMedia struct {
	ID             int64
	GameLanguageID int64
	Type           string
	URLOrPath      string
	Title          *string
	CreatedAt      time.Time
}

func (s *Store) CreateMedia(ctx context.Context, m GameMedia) (GameMedia, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO game_media (game_language_id, type, url_or_path, title) VALUES (?, ?, ?, ?)`,
		m.GameLanguageID, m.Type, m.URLOrPath, m.Title,
	)
	if err != nil {
		return GameMedia{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return GameMedia{}, err
	}
	return s.getMediaByID(ctx, id)
}

func (s *Store) getMediaByID(ctx context.Context, id int64) (GameMedia, error) {
	var m GameMedia
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, game_language_id, type, url_or_path, title, created_at FROM game_media WHERE id = ?`, id,
	).Scan(&m.ID, &m.GameLanguageID, &m.Type, &m.URLOrPath, &m.Title, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GameMedia{}, ErrNotFound
	}
	if err != nil {
		return GameMedia{}, err
	}
	m.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return m, nil
}

func (s *Store) ListMedia(ctx context.Context, gameLanguageID int64) ([]GameMedia, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, game_language_id, type, url_or_path, title, created_at FROM game_media WHERE game_language_id = ? ORDER BY id`,
		gameLanguageID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GameMedia
	for rows.Next() {
		var m GameMedia
		var createdAt string
		if err := rows.Scan(&m.ID, &m.GameLanguageID, &m.Type, &m.URLOrPath, &m.Title, &createdAt); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) DeleteMedia(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM game_media WHERE id = ?`, id)
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
