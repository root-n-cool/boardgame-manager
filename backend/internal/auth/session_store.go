package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Session struct {
	ID        int64
	UserID    int64
	TokenHash string
	ExpiresAt time.Time
}

var ErrSessionNotFound = errors.New("session not found")

type SessionStore struct {
	db *sql.DB
}

func NewSessionStore(conn *sql.DB) *SessionStore {
	return &SessionStore{db: conn}
}

func (s *SessionStore) Create(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (user_id, token_hash, expires_at) VALUES (?, ?, ?)`,
		userID, tokenHash, expiresAt.UTC().Format(time.RFC3339))
	return err
}

func (s *SessionStore) GetValidByTokenHash(ctx context.Context, tokenHash string) (Session, error) {
	var sess Session
	var expiresAtStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, expires_at FROM sessions WHERE token_hash = ?`,
		tokenHash).Scan(&sess.ID, &sess.UserID, &sess.TokenHash, &expiresAtStr)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, err
	}
	sess.ExpiresAt, err = time.Parse(time.RFC3339, expiresAtStr)
	if err != nil {
		return Session{}, err
	}
	if sess.ExpiresAt.Before(time.Now()) {
		return Session{}, ErrSessionNotFound
	}
	return sess, nil
}

func (s *SessionStore) Delete(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}
