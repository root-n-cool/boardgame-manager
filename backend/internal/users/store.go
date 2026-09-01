package users

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type User struct {
	ID    int64
	Email string
	// PasswordHash must never reach a response body. Handlers hand-build
	// their own response maps today; the tag is defence in depth against a
	// future writeJSON(w, 200, user) leaking bcrypt hashes.
	PasswordHash string `json:"-"`
	CreatedAt    time.Time
}

var ErrNotFound = errors.New("user not found")
var ErrDuplicateEmail = errors.New("email already in use")
var ErrCannotDeleteLastUser = errors.New("cannot delete the last remaining user")

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

// DeleteIfNotLast removes a user, refusing to empty the users table.
//
// The count and the delete share one transaction on purpose. Doing them as
// two separate statements is a TOCTOU race whose consequence is instance
// takeover: two admins deleting each other concurrently can both see
// count == 2, both delete, and leave zero users — at which point
// POST /api/bootstrap is unauthenticated-open again (it only checks
// count == 0) and the next visitor claims the instance as admin.
//
// Returns ErrCannotDeleteLastUser if the user is the only one left, and
// ErrNotFound if no user has that id.
func (s *Store) DeleteIfNotLast(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count <= 1 {
		return ErrCannotDeleteLastUser
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
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

	return tx.Commit()
}

func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
