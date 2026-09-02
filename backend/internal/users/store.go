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
	// InviteToken is the plaintext token of the invite link, non-nil only
	// until the admin has chosen their own password. Clearing it is what
	// makes the link unusable.
	InviteToken *string
	CreatedAt   time.Time
}

// Pending reports whether the admin still has an invite to accept.
// invite_token is the only "active" criterion the code uses: password_hash
// stays an internal detail of the store.
func (u User) Pending() bool { return u.InviteToken != nil }

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

// CreateInvite creates a pending admin: no password, one invite token. The
// password is written by the invitee themselves through the link (see
// Activate), so whoever invited them never knows it.
func (s *Store) CreateInvite(ctx context.Context, email, token string) (User, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO users (email, password_hash, invite_token) VALUES (?, '', ?)`, email, token)
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

// Activate writes the password the invitee chose and clears the token.
//
// The `invite_token IS NOT NULL` condition in the WHERE is what makes the link
// single-use: two concurrent POSTs on the same invite cannot set two different
// passwords — the second one matches zero rows and gets ErrNotFound.
func (s *Store) Activate(ctx context.Context, id int64, passwordHash string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, invite_token = NULL WHERE id = ? AND invite_token IS NOT NULL`,
		passwordHash, id)
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

func (s *Store) GetByEmail(ctx context.Context, email string) (User, error) {
	return s.scanOne(ctx, `SELECT id, email, password_hash, invite_token, created_at FROM users WHERE email = ?`, email)
}

func (s *Store) GetByID(ctx context.Context, id int64) (User, error) {
	return s.scanOne(ctx, `SELECT id, email, password_hash, invite_token, created_at FROM users WHERE id = ?`, id)
}

func (s *Store) GetByInviteToken(ctx context.Context, token string) (User, error) {
	return s.scanOne(ctx, `SELECT id, email, password_hash, invite_token, created_at FROM users WHERE invite_token = ?`, token)
}

func (s *Store) scanOne(ctx context.Context, query string, arg any) (User, error) {
	var u User
	var inviteToken sql.NullString
	var createdAt string
	err := s.db.QueryRowContext(ctx, query, arg).Scan(&u.ID, &u.Email, &u.PasswordHash, &inviteToken, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	if inviteToken.Valid {
		token := inviteToken.String
		u.InviteToken = &token
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return u, nil
}

func (s *Store) List(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, email, password_hash, invite_token, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []User
	for rows.Next() {
		var u User
		var inviteToken sql.NullString
		var createdAt string
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &inviteToken, &createdAt); err != nil {
			return nil, err
		}
		if inviteToken.Valid {
			token := inviteToken.String
			u.InviteToken = &token
		}
		u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		out = append(out, u)
	}
	return out, rows.Err()
}

// DeleteIfNotLast removes a user, refusing to empty the pool of active admins.
//
// The count and the delete share one transaction on purpose. Doing them as
// two separate statements is a TOCTOU race whose consequence is instance
// takeover: two admins deleting each other concurrently can both see the
// same count of other active admins, both delete, and leave zero — at which
// point POST /api/bootstrap is unauthenticated-open again (it only checks
// count == 0 on the full table) and the next visitor claims the instance
// as admin.
//
// Returns ErrCannotDeleteLastUser if the user is the last active admin, and
// ErrNotFound if no user has that id.
func (s *Store) DeleteIfNotLast(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Count the ACTIVE admins other than the target: if none is left, the
	// deletion would leave the instance with nobody able to sign in, while
	// POST /api/bootstrap stays closed (it looks at the total COUNT(*)).
	// A pending invite is not a working way in, so it does not count as an
	// admin — and for the same reason it is always revocable.
	var otherActive int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE invite_token IS NULL AND id != ?`, id).Scan(&otherActive); err != nil {
		return err
	}
	if otherActive == 0 {
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
