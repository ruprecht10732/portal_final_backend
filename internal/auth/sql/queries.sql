-- Auth Domain SQL Queries

-- name: CreateUser :one
INSERT INTO users (email, password_hash, is_email_verified)
VALUES ($1, $2, false)
RETURNING id, email, password_hash, is_email_verified, first_name, last_name, created_at, updated_at;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, is_email_verified, first_name, last_name, created_at, updated_at
FROM users WHERE email = $1;

-- name: GetUserByID :one
SELECT id, email, password_hash, is_email_verified, first_name, last_name, created_at, updated_at
FROM users WHERE id = $1;

-- name: MarkEmailVerified :exec
UPDATE users SET is_email_verified = true, updated_at = now() WHERE id = $1;

-- name: UpdatePassword :exec
UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: UpdateUserEmail :one
UPDATE users
SET email = $2, is_email_verified = false, updated_at = now()
WHERE id = $1
RETURNING id, email, password_hash, is_email_verified, first_name, last_name, created_at, updated_at;

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

-- name: GetUserRoles :many
SELECT r.name
FROM roles r
JOIN user_roles ur ON ur.role_id = r.id
WHERE ur.user_id = $1
ORDER BY r.name;

-- name: ListUsers :many
SELECT u.id, u.email,
    COALESCE(array_agg(r.name) FILTER (WHERE r.name IS NOT NULL), '{}') AS roles
FROM users u
LEFT JOIN user_roles ur ON ur.user_id = u.id
LEFT JOIN roles r ON r.id = ur.role_id
GROUP BY u.id
ORDER BY u.email;

-- name: DeleteUserRoles :exec
DELETE FROM user_roles WHERE user_id = $1;

-- name: InsertUserRoles :exec
INSERT INTO user_roles (user_id, role_id)
SELECT $1, id FROM roles WHERE name = ANY($2::text[]);

-- name: GetValidRoles :many
SELECT name FROM roles WHERE name = ANY($1::text[]);
