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
	// AIBaseURL, AIAPIKey e AIModel descrivono un provider
	// OpenAI-compatible. Valgono solo tutti e tre insieme: con uno vuoto
	// l'app resta senza AI, e non è un errore.
	AIBaseURL string
	AIAPIKey  string
	AIModel   string
}

type Store struct {
	db *sql.DB
}

func NewStore(conn *sql.DB) *Store {
	return &Store{db: conn}
}

func (s *Store) Get(ctx context.Context) (Settings, error) {
	var out Settings
	var baseURL, bggToken, aiBaseURL, aiAPIKey, aiModel sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT default_language, public_base_url, bgg_api_token, ai_base_url, ai_api_key, ai_model
		 FROM app_settings WHERE id = 1`,
	).Scan(&out.DefaultLanguage, &baseURL, &bggToken, &aiBaseURL, &aiAPIKey, &aiModel)
	if err != nil {
		return Settings{}, err
	}
	out.PublicBaseURL = baseURL.String
	out.BGGAPIToken = bggToken.String
	out.AIBaseURL = aiBaseURL.String
	out.AIAPIKey = aiAPIKey.String
	out.AIModel = aiModel.String
	return out, nil
}

func (s *Store) Update(ctx context.Context, in Settings) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE app_settings SET default_language = ?, public_base_url = ?, bgg_api_token = ?,
		 ai_base_url = ?, ai_api_key = ?, ai_model = ? WHERE id = 1`,
		in.DefaultLanguage, nullIfEmpty(in.PublicBaseURL), nullIfEmpty(in.BGGAPIToken),
		nullIfEmpty(in.AIBaseURL), nullIfEmpty(in.AIAPIKey), nullIfEmpty(in.AIModel),
	)
	return err
}

func nullIfEmpty(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}
