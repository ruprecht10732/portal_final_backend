-- ============================================================================
-- Auth Domain SQL Queries (sqlc)
-- ============================================================================

-- name: CreateUser :one
-- Complexity: O(1) Time. Requires UNIQUE index on email.
INSERT INTO RAC_users (email, password_hash, is_email_verified)
VALUES ($1, $2, false)
RETURNING id, email, password_hash, is_email_verified, first_name, last_name, phone, onboarding_completed_at, created_at, updated_at;

-- name: GetUserByEmail :one
-- Complexity: O(log N) Time. Requires UNIQUE index on email.
SELECT id, email, password_hash, is_email_verified, first_name, last_name, phone, onboarding_completed_at, created_at, updated_at 
FROM RAC_users 
WHERE email = $1;

-- name: GetUserByID :one
-- Complexity: O(1) Time (Primary Key lookup).
SELECT id, email, password_hash, is_email_verified, first_name, last_name, phone, onboarding_completed_at, created_at, updated_at 
FROM RAC_users 
WHERE id = $1;

-- name: MarkEmailVerified :exec
UPDATE RAC_users 
SET is_email_verified = true, updated_at = CURRENT_TIMESTAMP 
WHERE id = $1;

-- name: UpdatePassword :exec
UPDATE RAC_users 
SET password_hash = $2, updated_at = CURRENT_TIMESTAMP 
WHERE id = $1;

-- name: UpdateUserEmail :one
-- Security: Implicitly resets is_email_verified to false to prevent account takeover.
UPDATE RAC_users
SET email = $2, is_email_verified = false, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING id, email, password_hash, is_email_verified, first_name, last_name, phone, onboarding_completed_at, created_at, updated_at;

-- name: UpdateUserNames :one
UPDATE RAC_users
SET first_name = $2, last_name = $3, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING id, email, password_hash, is_email_verified, first_name, last_name, phone, onboarding_completed_at, created_at, updated_at;

-- name: UpdateUserPhone :one
-- Optimization: Eliminated verbose CASE/WHEN block. NULLIF(TRIM()) safely coalesces empty/whitespace strings to NULL.
UPDATE RAC_users
SET phone = NULLIF(BTRIM(sqlc.arg(phone)::text), ''),
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING id, email, password_hash, is_email_verified, first_name, last_name, phone, onboarding_completed_at, created_at, updated_at;

-- ============================================================================
-- Token Management
-- ============================================================================

-- name: CreateUserToken :exec
INSERT INTO RAC_user_tokens (user_id, token_hash, type, expires_at)
VALUES ($1, $2, $3, $4);

-- name: GetUserToken :one
-- Complexity: O(log N) Time. Requires Index on (token_hash, type) WHERE used_at IS NULL.
SELECT user_id, expires_at 
FROM RAC_user_tokens
WHERE token_hash = $1 AND type = $2 AND used_at IS NULL;

-- name: UseUserToken :exec
-- Security: Atomic invalidation prevents TOCTOU replay attacks on verification links.
UPDATE RAC_user_tokens 
SET used_at = CURRENT_TIMESTAMP
WHERE token_hash = $1 AND type = $2 AND used_at IS NULL;

-- name: CreateRefreshToken :exec
INSERT INTO RAC_refresh_tokens (user_id, token_hash, expires_at)
VALUES ($1, $2, $3);

-- name: GetRefreshToken :one
SELECT user_id, expires_at 
FROM RAC_refresh_tokens
WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: RevokeRefreshToken :exec
UPDATE RAC_refresh_tokens 
SET revoked_at = CURRENT_TIMESTAMP
WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: RevokeAllRefreshTokens :exec
UPDATE RAC_refresh_tokens 
SET revoked_at = CURRENT_TIMESTAMP
WHERE user_id = $1 AND revoked_at IS NULL;

-- ============================================================================
-- Role Management (RBAC)
-- ============================================================================

-- name: GetUserRoles :many
SELECT r.name 
FROM RAC_roles r
JOIN RAC_user_roles ur ON ur.role_id = r.id
WHERE ur.user_id = $1
ORDER BY r.name;

-- name: DeleteUserRoles :exec
DELETE FROM RAC_user_roles WHERE user_id = $1;

-- name: InsertUserRoles :exec
INSERT INTO RAC_user_roles (user_id, role_id)
SELECT $1, id FROM RAC_roles WHERE name = ANY($2::text[]);

-- name: GetValidRoles :many
SELECT name FROM RAC_roles WHERE name = ANY(sqlc.arg(roleNames)::text[]);

-- ============================================================================
-- User Settings & Onboarding
-- ============================================================================

-- name: EnsureUserSettings :exec
INSERT INTO RAC_user_settings (user_id)
VALUES ($1)
ON CONFLICT (user_id) DO NOTHING;

-- name: GetUserSettings :one
SELECT preferred_language AS preferredLanguage
FROM RAC_user_settings
WHERE user_id = $1;

-- name: UpsertUserSettings :exec
INSERT INTO RAC_user_settings (user_id, preferred_language)
VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE
SET preferred_language = EXCLUDED.preferred_language, updated_at = CURRENT_TIMESTAMP;

-- name: TouchUserUpdatedAt :exec
UPDATE RAC_users SET updated_at = CURRENT_TIMESTAMP WHERE id = $1;

-- name: MarkOnboardingComplete :exec
UPDATE RAC_users 
SET onboarding_completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = $1 AND onboarding_completed_at IS NULL;

-- ============================================================================
-- Bulk Read Operations (WARNING: O(N) Complexity Risks)
-- ============================================================================

-- name: ListUsers :many
-- WARNING: This requires pagination (LIMIT/OFFSET) in V2 to prevent memory exhaustion.
SELECT
    u.id,
    u.email,
    u.first_name,
    u.last_name,
    COALESCE(array_agg(r.name) FILTER (WHERE r.name IS NOT NULL), ARRAY[]::text[])::text[] AS roles
FROM RAC_users u
LEFT JOIN RAC_user_roles ur ON ur.user_id = u.id
LEFT JOIN RAC_roles r ON r.id = ur.role_id
GROUP BY u.id
ORDER BY u.email;

-- name: ListUsersByOrganization :many
SELECT
    u.id,
    u.email,
    u.first_name,
    u.last_name,
    COALESCE(array_agg(r.name) FILTER (WHERE r.name IS NOT NULL), ARRAY[]::text[])::text[] AS roles
FROM RAC_organization_members om
JOIN RAC_users u ON u.id = om.user_id
LEFT JOIN RAC_user_roles ur ON ur.user_id = u.id
LEFT JOIN RAC_roles r ON r.id = ur.role_id
WHERE om.organization_id = $1
GROUP BY u.id
ORDER BY u.email;

-- ============================================================================
-- WebAuthn Credential Queries
-- ============================================================================

-- name: CreateWebAuthnCredential :exec
INSERT INTO RAC_webauthn_credentials (
    id, user_id, public_key, attestation_type, transport,
    flags_json, aaguid, sign_count, clone_warning, nickname
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: ListWebAuthnCredentialsByUser :many
SELECT id, user_id, public_key, attestation_type, transport,
       flags_json, aaguid, sign_count, clone_warning, nickname,
       created_at, last_used_at
FROM RAC_webauthn_credentials
WHERE user_id = $1
ORDER BY created_at;

-- name: GetWebAuthnCredential :one
SELECT id, user_id, public_key, attestation_type, transport,
       flags_json, aaguid, sign_count, clone_warning, nickname,
       created_at, last_used_at
FROM RAC_webauthn_credentials
WHERE id = $1;

-- name: UpdateWebAuthnCredentialSignCount :exec
-- Security: Critical for clone detection during passkey login phase.
UPDATE RAC_webauthn_credentials
SET sign_count = $2, clone_warning = $3, last_used_at = CURRENT_TIMESTAMP
WHERE id = $1;

-- name: UpdateWebAuthnCredentialNickname :exec
UPDATE RAC_webauthn_credentials
SET nickname = $2
WHERE id = $1 AND user_id = $3;

-- name: DeleteWebAuthnCredential :exec
DELETE FROM RAC_webauthn_credentials
WHERE id = $1 AND user_id = $2;

-- name: GetUserByWebAuthnCredentialID :one
-- Complexity: O(log N) Time. Requires Primary Key index on WebAuthn id.
SELECT u.id, u.email, u.password_hash, u.is_email_verified,
       u.first_name, u.last_name, u.phone, u.onboarding_completed_at,
       u.created_at, u.updated_at
FROM RAC_users u
JOIN RAC_webauthn_credentials wc ON wc.user_id = u.id
WHERE wc.id = $1;