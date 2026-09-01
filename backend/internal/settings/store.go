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
	BGGAPIToken       string
}

type Store struct {
	db *sql.DB
}

func NewStore(conn *sql.DB) *Store {
	return &Store{db: conn}
}

func (s *Store) Get(ctx context.Context) (Settings, error) {
	var out Settings
	var youtubeKey, searchKey, provider, bggToken sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT default_language, youtube_api_key, search_api_key, search_api_provider, bgg_api_token FROM app_settings WHERE id = 1`,
	).Scan(&out.DefaultLanguage, &youtubeKey, &searchKey, &provider, &bggToken)
	if err != nil {
		return Settings{}, err
	}
	out.YouTubeAPIKey = youtubeKey.String
	out.SearchAPIKey = searchKey.String
	out.SearchAPIProvider = provider.String
	out.BGGAPIToken = bggToken.String
	return out, nil
}

func (s *Store) Update(ctx context.Context, in Settings) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE app_settings SET default_language = ?, youtube_api_key = ?, search_api_key = ?, search_api_provider = ?, bgg_api_token = ? WHERE id = 1`,
		in.DefaultLanguage, nullIfEmpty(in.YouTubeAPIKey), nullIfEmpty(in.SearchAPIKey), nullIfEmpty(in.SearchAPIProvider), nullIfEmpty(in.BGGAPIToken),
	)
	return err
}

func nullIfEmpty(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}
