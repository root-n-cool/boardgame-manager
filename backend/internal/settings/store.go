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
	// I campi SMTP valgono solo con host, porta e indirizzo mittente
	// insieme; senza, l'app resta senza posta e non è un errore. Come
	// AIAPIKey, SMTPPassword è un segreto e non esce mai in chiaro
	// dall'API.
	SMTPHost        string
	SMTPPort        int
	SMTPUsername    string
	SMTPPassword    string
	SMTPFromAddress string
	SMTPFromName    string
	SMTPTLSMode     string
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
	var smtpHost, smtpUser, smtpPass, smtpFrom, smtpFromName, smtpTLS sql.NullString
	var smtpPort sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT default_language, public_base_url, bgg_api_token, ai_base_url, ai_api_key, ai_model,
		        smtp_host, smtp_port, smtp_username, smtp_password, smtp_from_address, smtp_from_name, smtp_tls_mode
		 FROM app_settings WHERE id = 1`,
	).Scan(&out.DefaultLanguage, &baseURL, &bggToken, &aiBaseURL, &aiAPIKey, &aiModel,
		&smtpHost, &smtpPort, &smtpUser, &smtpPass, &smtpFrom, &smtpFromName, &smtpTLS)
	if err != nil {
		return Settings{}, err
	}
	out.PublicBaseURL = baseURL.String
	out.BGGAPIToken = bggToken.String
	out.AIBaseURL = aiBaseURL.String
	out.AIAPIKey = aiAPIKey.String
	out.AIModel = aiModel.String
	out.SMTPHost = smtpHost.String
	out.SMTPPort = int(smtpPort.Int64)
	out.SMTPUsername = smtpUser.String
	out.SMTPPassword = smtpPass.String
	out.SMTPFromAddress = smtpFrom.String
	out.SMTPFromName = smtpFromName.String
	out.SMTPTLSMode = smtpTLS.String
	return out, nil
}

func (s *Store) Update(ctx context.Context, in Settings) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE app_settings SET default_language = ?, public_base_url = ?, bgg_api_token = ?,
		 ai_base_url = ?, ai_api_key = ?, ai_model = ?,
		 smtp_host = ?, smtp_port = ?, smtp_username = ?, smtp_password = ?,
		 smtp_from_address = ?, smtp_from_name = ?, smtp_tls_mode = ?
		 WHERE id = 1`,
		in.DefaultLanguage, nullIfEmpty(in.PublicBaseURL), nullIfEmpty(in.BGGAPIToken),
		nullIfEmpty(in.AIBaseURL), nullIfEmpty(in.AIAPIKey), nullIfEmpty(in.AIModel),
		nullIfEmpty(in.SMTPHost), nullIfZero(in.SMTPPort), nullIfEmpty(in.SMTPUsername),
		nullIfEmpty(in.SMTPPassword), nullIfEmpty(in.SMTPFromAddress),
		nullIfEmpty(in.SMTPFromName), nullIfEmpty(in.SMTPTLSMode),
	)
	return err
}

func nullIfEmpty(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}

func nullIfZero(v int) sql.NullInt64 {
	return sql.NullInt64{Int64: int64(v), Valid: v != 0}
}
