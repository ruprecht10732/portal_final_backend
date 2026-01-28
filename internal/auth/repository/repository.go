package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

const (
	TokenTypeEmailVerify   = "EMAIL_VERIFY"
	TokenTypePasswordReset = "PASSWORD_RESET"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

type User struct {
	ID            uuid.UUID
	Email         string
	PasswordHash  string
	EmailVerified bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (r *Repository) CreateUser(ctx context.Context, email, passwordHash string) (User, error) {
	var user User
	err := r.pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, is_email_verified)
		VALUES ($1, $2, false)
		RETURNING id, email, password_hash, is_email_verified, created_at, updated_at
	`, email, passwordHash).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.EmailVerified, &user.CreatedAt, &user.UpdatedAt)
	return user, err
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := r.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, is_email_verified, created_at, updated_at
		FROM users WHERE email = $1
	`, email).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.EmailVerified, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return user, err
}

func (r *Repository) MarkEmailVerified(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE users SET is_email_verified = true, updated_at = now()
		WHERE id = $1
	`, userID)
	return err
}

func (r *Repository) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE users SET password_hash = $2, updated_at = now()
		WHERE id = $1
	`, userID, passwordHash)
	return err
}

func (r *Repository) CreateUserToken(ctx context.Context, userID uuid.UUID, tokenHash string, tokenType string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_tokens (user_id, token_hash, type, expires_at)
		VALUES ($1, $2, $3, $4)
	`, userID, tokenHash, tokenType, expiresAt)
	return err
}

func (r *Repository) GetUserToken(ctx context.Context, tokenHash string, tokenType string) (uuid.UUID, time.Time, error) {
	var userID uuid.UUID
	var expiresAt time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT user_id, expires_at FROM user_tokens
		WHERE token_hash = $1 AND type = $2 AND used_at IS NULL
	`, tokenHash, tokenType).Scan(&userID, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.UUID{}, time.Time{}, ErrNotFound
	}
	return userID, expiresAt, err
}

func (r *Repository) UseUserToken(ctx context.Context, tokenHash string, tokenType string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE user_tokens SET used_at = now()
		WHERE token_hash = $1 AND type = $2 AND used_at IS NULL
	`, tokenHash, tokenType)
	return err
}

func (r *Repository) CreateRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, tokenHash, expiresAt)
	return err
}

func (r *Repository) GetRefreshToken(ctx context.Context, tokenHash string) (uuid.UUID, time.Time, error) {
	var userID uuid.UUID
	var expiresAt time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT user_id, expires_at FROM refresh_tokens
		WHERE token_hash = $1 AND revoked_at IS NULL
	`, tokenHash).Scan(&userID, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.UUID{}, time.Time{}, ErrNotFound
	}
	return userID, expiresAt, err
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL
	`, tokenHash)
	return err
}

func (r *Repository) RevokeAllRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL
	`, userID)
	return err
}
