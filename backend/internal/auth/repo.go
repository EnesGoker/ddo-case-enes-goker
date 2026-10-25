package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("kayıt bulunamadı")

type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	FullName     string `json:"full_name"`
	Role         Role   `json:"role"`
	IsActive     bool   `json:"-"`
	PasswordHash string `json:"-"`
}

type refreshRow struct {
	ID        int64
	UserID    int64
	ExpiresAt time.Time
	RevokedAt *time.Time
}

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const userCols = `id, email, full_name, role, is_active, password_hash`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.FullName, &u.Role, &u.IsActive, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (r *Repo) UserByEmail(ctx context.Context, email string) (User, error) {
	return scanUser(r.pool.QueryRow(ctx,
		`SELECT `+userCols+` FROM users WHERE lower(email) = lower($1)`, email))
}

func (r *Repo) UserByID(ctx context.Context, id int64) (User, error) {
	return scanUser(r.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
}

func insertRefresh(ctx context.Context, q pgx.Tx, userID int64, hash string, exp time.Time) error {
	_, err := q.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, hash, exp)
	return err
}

func (r *Repo) CreateRefresh(ctx context.Context, userID int64, hash string, exp time.Time) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		return insertRefresh(ctx, tx, userID, hash, exp)
	})
}

// lockRefresh token satırını FOR UPDATE ile kilitler: aynı token'la eşzamanlı iki
// yenileme isteğinden yalnızca biri başarılı olur.
func lockRefresh(ctx context.Context, tx pgx.Tx, hash string) (refreshRow, error) {
	var t refreshRow
	err := tx.QueryRow(ctx,
		`SELECT id, user_id, expires_at, revoked_at FROM refresh_tokens WHERE token_hash = $1 FOR UPDATE`,
		hash).Scan(&t.ID, &t.UserID, &t.ExpiresAt, &t.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

func revokeRefresh(ctx context.Context, tx pgx.Tx, id int64) error {
	_, err := tx.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, id)
	return err
}

func revokeAllForUser(ctx context.Context, tx pgx.Tx, userID int64) error {
	_, err := tx.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return err
}
