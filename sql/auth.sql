-- name: CreateUser :one
INSERT INTO users (email, password_hash, is_email_verified)
VALUES ($1, $2, false)
RETURNING id, email, password_hash, is_email_verified, created_at, updated_at;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, is_email_verified, created_at, updated_at
FROM users WHERE email = $1;

-- name: MarkEmailVerified :exec
UPDATE users SET is_email_verified = true, updated_at = now() WHERE id = $1;

-- name: UpdatePassword :exec
UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: CreateUserToken :exec
INSERT INTO user_tokens (user_id, token_hash, type, expires_at)
VALUES ($1, $2, $3, $4);

-- name: GetUserToken :one
SELECT user_id, expires_at FROM user_tokens
WHERE token_hash = $1 AND type = $2 AND used_at IS NULL;

-- name: UseUserToken :exec
UPDATE user_tokens SET used_at = now()
WHERE token_hash = $1 AND type = $2 AND used_at IS NULL;

-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
VALUES ($1, $2, $3);

-- name: GetRefreshToken :one
SELECT user_id, expires_at FROM refresh_tokens
WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens SET revoked_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: RevokeAllRefreshTokens :exec
UPDATE refresh_tokens SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL;
