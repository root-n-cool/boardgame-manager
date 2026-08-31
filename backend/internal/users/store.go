package users

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type User struct {
	ID           int64
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

var ErrNotFound = errors.New("user not found")
var ErrDuplicateEmail = errors.New("email already in use")

type Store struct {
	db *sql.DB
}

func NewStore(conn *sql.DB) *Store {
	return &Store{db: conn}
}

func (s *Store) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) Create(ctx context.Context, email, passwordHash string) (User, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO users (email, password_hash) VALUES (?, ?)`, email, passwordHash)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return User{}, ErrDuplicateEmail
		}
		return User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *Store) GetByEmail(ctx context.Context, email string) (User, error) {
	return s.scanOne(ctx, `SELECT id, email, password_hash, created_at FROM users WHERE email = ?`, email)
}

func (s *Store) GetByID(ctx context.Context, id int64) (User, error) {
	return s.scanOne(ctx, `SELECT id, email, password_hash, created_at FROM users WHERE id = ?`, id)
}

func (s *Store) scanOne(ctx context.Context, query string, arg any) (User, error) {
	var u User
	var createdAt string
	err := s.db.QueryRowContext(ctx, query, arg).Scan(&u.ID, &u.Email, &u.PasswordHash, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return u, nil
}

func (s *Store) List(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, email, password_hash, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []User
	for rows.Next() {
		var u User
		var createdAt string
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &createdAt); err != nil {
			return nil, err
		}
		u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
