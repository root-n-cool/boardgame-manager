package settings

import (
	"context"
	"database/sql"
)

type Settings struct {
	DefaultLanguage   string
	YouTubeAPIKey     string
	SearchAPIKey      string
	SearchAPIProvider string
}

type Store struct {
	db *sql.DB
}

func NewStore(conn *sql.DB) *Store {
	return &Store{db: conn}
}

func (s *Store) Get(ctx context.Context) (Settings, error) {
	var out Settings
	var youtubeKey, searchKey, provider sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT default_language, youtube_api_key, search_api_key, search_api_provider FROM app_settings WHERE id = 1`,
	).Scan(&out.DefaultLanguage, &youtubeKey, &searchKey, &provider)
	if err != nil {
		return Settings{}, err
	}
	out.YouTubeAPIKey = youtubeKey.String
	out.SearchAPIKey = searchKey.String
	out.SearchAPIProvider = provider.String
	return out, nil
}

func (s *Store) Update(ctx context.Context, in Settings) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE app_settings SET default_language = ?, youtube_api_key = ?, search_api_key = ?, search_api_provider = ? WHERE id = 1`,
		in.DefaultLanguage, nullIfEmpty(in.YouTubeAPIKey), nullIfEmpty(in.SearchAPIKey), nullIfEmpty(in.SearchAPIProvider),
	)
	return err
}

func nullIfEmpty(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}
