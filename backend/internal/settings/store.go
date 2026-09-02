package settings

import (
	"context"
	"database/sql"
)

type Settings struct {
	DefaultLanguage string
	// PublicBaseURL is the address the club reaches this instance at, without a
	// trailing slash, or empty when it has not been configured. It is not a
	// secret: unlike BGGAPIToken it is served in clear.
	PublicBaseURL string
	BGGAPIToken   string
}

type Store struct {
	db *sql.DB
}

func NewStore(conn *sql.DB) *Store {
	return &Store{db: conn}
}

func (s *Store) Get(ctx context.Context) (Settings, error) {
	var out Settings
	var baseURL, bggToken sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT default_language, public_base_url, bgg_api_token FROM app_settings WHERE id = 1`,
	).Scan(&out.DefaultLanguage, &baseURL, &bggToken)
	if err != nil {
		return Settings{}, err
	}
	out.PublicBaseURL = baseURL.String
	out.BGGAPIToken = bggToken.String
	return out, nil
}

func (s *Store) Update(ctx context.Context, in Settings) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE app_settings SET default_language = ?, public_base_url = ?, bgg_api_token = ? WHERE id = 1`,
		in.DefaultLanguage, nullIfEmpty(in.PublicBaseURL), nullIfEmpty(in.BGGAPIToken),
	)
	return err
}

func nullIfEmpty(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}
