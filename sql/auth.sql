-- name: CreateUser :one
INSERT INTO RAC_users (email, password_hash, is_email_verified)
VALUES ($1, $2, false) RETURNING id, email, password_hash, is_email_verified, created_at, updated_at;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, is_email_verified, created_at, updated_at FROM RAC_users WHERE email = $1;

-- name: MarkEmailVerified :exec
UPDATE RAC_users SET is_email_verified = true, updated_at = now() WHERE id = $1;

-- name: UpdatePassword :exec
UPDATE RAC_users SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: CreateUserToken :exec
INSERT INTO RAC_user_tokens (user_id, token_hash, type, expires_at)
VALUES ($1, $2, $3, $4);

-- name: GetUserToken :one
SELECT user_id, expires_at FROM RAC_user_tokens
WHERE token_hash = $1 AND type = $2 AND used_at IS NULL;

-- name: UseUserToken :exec
UPDATE RAC_user_tokens SET used_at = now()
WHERE token_hash = $1 AND type = $2 AND used_at IS NULL;

-- name: CreateRefreshToken :exec
INSERT INTO RAC_refresh_tokens (user_id, token_hash, expires_at)
VALUES ($1, $2, $3);

-- name: GetRefreshToken :one
SELECT user_id, expires_at FROM RAC_refresh_tokens
WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: RevokeRefreshToken :exec
UPDATE RAC_refresh_tokens SET revoked_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: RevokeAllRefreshTokens :exec
UPDATE RAC_refresh_tokens SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL;
