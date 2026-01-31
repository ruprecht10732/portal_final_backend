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
var ErrInvalidRole = errors.New("invalid role")

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
	FirstName     *string
	LastName      *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type UserWithRoles struct {
	ID        uuid.UUID
	Email     string
	FirstName *string
	LastName  *string
	Roles     []string
}

func (r *Repository) CreateUser(ctx context.Context, email, passwordHash string) (User, error) {
	var user User
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, is_email_verified)
		VALUES ($1, $2, false)
		RETURNING id, email, password_hash, is_email_verified, first_name, last_name, created_at, updated_at
	`, email, passwordHash).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.EmailVerified,
		&user.FirstName,
		&user.LastName,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return User{}, err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO user_settings (user_id)
		VALUES ($1)
		ON CONFLICT (user_id) DO NOTHING
	`, user.ID); err != nil {
		return User{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return User{}, err
	}

	return user, nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := r.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, is_email_verified, first_name, last_name, created_at, updated_at
		FROM users WHERE email = $1
	`, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.EmailVerified,
		&user.FirstName,
		&user.LastName,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return user, err
}

func (r *Repository) GetUserByID(ctx context.Context, userID uuid.UUID) (User, error) {
	var user User
	err := r.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, is_email_verified, first_name, last_name, created_at, updated_at
		FROM users WHERE id = $1
	`, userID).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.EmailVerified,
		&user.FirstName,
		&user.LastName,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
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

func (r *Repository) UpdateUserEmail(ctx context.Context, userID uuid.UUID, email string) (User, error) {
	var user User
	err := r.pool.QueryRow(ctx, `
		UPDATE users
		SET email = $2, is_email_verified = false, updated_at = now()
		WHERE id = $1
		RETURNING id, email, password_hash, is_email_verified, first_name, last_name, created_at, updated_at
	`, userID, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.EmailVerified,
		&user.FirstName,
		&user.LastName,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	return user, err
}

func (r *Repository) UpdateUserNames(ctx context.Context, userID uuid.UUID, firstName, lastName *string) (User, error) {
	var user User
	err := r.pool.QueryRow(ctx, `
		UPDATE users
		SET first_name = $2, last_name = $3, updated_at = now()
		WHERE id = $1
		RETURNING id, email, password_hash, is_email_verified, first_name, last_name, created_at, updated_at
	`, userID, firstName, lastName).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.EmailVerified,
		&user.FirstName,
		&user.LastName,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	return user, err
}

func (r *Repository) EnsureUserSettings(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_settings (user_id)
		VALUES ($1)
		ON CONFLICT (user_id) DO NOTHING
	`, userID)
	return err
}

func (r *Repository) GetUserSettings(ctx context.Context, userID uuid.UUID) (string, error) {
	var preferredLanguage string
	err := r.pool.QueryRow(ctx, `
		SELECT preferred_language
		FROM user_settings
		WHERE user_id = $1
	`, userID).Scan(&preferredLanguage)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return preferredLanguage, err
}

func (r *Repository) UpdateUserSettings(ctx context.Context, userID uuid.UUID, preferredLanguage string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if _, err = tx.Exec(ctx, `
		INSERT INTO user_settings (user_id, preferred_language)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE
		SET preferred_language = EXCLUDED.preferred_language, updated_at = now()
	`, userID, preferredLanguage); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
		UPDATE users SET updated_at = now() WHERE id = $1
	`, userID); err != nil {
		return err
	}

	return tx.Commit(ctx)
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

func (r *Repository) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT r.name
		FROM roles r
		JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1
		ORDER BY r.name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	roles := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		roles = append(roles, name)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return roles, nil
}

func (r *Repository) SetUserRoles(ctx context.Context, userID uuid.UUID, roles []string) error {
	if len(roles) == 0 {
		return ErrInvalidRole
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	rows, err := tx.Query(ctx, `SELECT name FROM roles WHERE name = ANY($1)`, roles)
	if err != nil {
		return err
	}
	defer rows.Close()

	valid := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		valid[name] = struct{}{}
	}
	if rows.Err() != nil {
		return rows.Err()
	}
	if len(valid) != len(uniqueStrings(roles)) {
		return ErrInvalidRole
	}

	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, userID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE name = ANY($2)
	`, userID, roles); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}

func (r *Repository) ListUsers(ctx context.Context) ([]UserWithRoles, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.email, u.first_name, u.last_name,
			COALESCE(array_agg(r.name) FILTER (WHERE r.name IS NOT NULL), '{}') AS roles
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		GROUP BY u.id
		ORDER BY u.email
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]UserWithRoles, 0)
	for rows.Next() {
		var user UserWithRoles
		if err := rows.Scan(&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.Roles); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return users, nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
