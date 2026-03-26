-- Auth Domain SQL Queries

-- name: CreateUser :one
INSERT INTO RAC_users (email, password_hash, is_email_verified)
VALUES ($1, $2, false)
RETURNING id, email, password_hash, is_email_verified, first_name, last_name, phone, onboarding_completed_at, created_at, updated_at;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, is_email_verified, first_name, last_name, phone, onboarding_completed_at, created_at, updated_at FROM RAC_users WHERE email = $1;

-- name: GetUserByID :one
SELECT id, email, password_hash, is_email_verified, first_name, last_name, phone, onboarding_completed_at, created_at, updated_at FROM RAC_users WHERE id = $1;

-- name: MarkEmailVerified :exec
UPDATE RAC_users SET is_email_verified = true, updated_at = now() WHERE id = $1;

-- name: UpdatePassword :exec
UPDATE RAC_users SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: UpdateUserEmail :one
UPDATE RAC_users
SET email = $2, is_email_verified = false, updated_at = now()
WHERE id = $1
RETURNING id, email, password_hash, is_email_verified, first_name, last_name, phone, onboarding_completed_at, created_at, updated_at;

-- name: UpdateUserNames :one
UPDATE RAC_users
SET first_name = $2, last_name = $3, updated_at = now()
WHERE id = $1
RETURNING id, email, password_hash, is_email_verified, first_name, last_name, phone, onboarding_completed_at, created_at, updated_at;

-- name: UpdateUserPhone :one
UPDATE RAC_users
SET phone = CASE
		WHEN NULLIF(BTRIM(sqlc.arg(phone)::text), '') IS NULL THEN NULL
		ELSE sqlc.arg(phone)::text
	END,
	updated_at = now()
WHERE id = $1
RETURNING id, email, password_hash, is_email_verified, first_name, last_name, phone, onboarding_completed_at, created_at, updated_at;

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

-- name: GetUserRoles :many
SELECT r.name FROM RAC_roles r
JOIN RAC_user_roles ur ON ur.role_id = r.id
WHERE ur.user_id = $1
ORDER BY r.name;

-- name: ListUsers :many
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

-- name: DeleteUserRoles :exec
DELETE FROM RAC_user_roles WHERE user_id = $1;

-- name: InsertUserRoles :exec
INSERT INTO RAC_user_roles (user_id, role_id)
SELECT $1, id FROM RAC_roles WHERE name = ANY($2::text[]);

-- name: GetValidRoles :many
SELECT name FROM RAC_roles WHERE name = ANY(sqlc.arg(roleNames)::text[]);

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
SET preferred_language = EXCLUDED.preferred_language, updated_at = now();

-- name: TouchUserUpdatedAt :exec
UPDATE RAC_users SET updated_at = now() WHERE id = $1;

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

-- name: MarkOnboardingComplete :exec
UPDATE RAC_users SET onboarding_completed_at = now(), updated_at = now()
WHERE id = $1 AND onboarding_completed_at IS NULL;

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
UPDATE RAC_webauthn_credentials
SET sign_count = $2, clone_warning = $3, last_used_at = now()
WHERE id = $1;

-- name: UpdateWebAuthnCredentialNickname :exec
UPDATE RAC_webauthn_credentials
SET nickname = $2
WHERE id = $1 AND user_id = $3;

-- name: DeleteWebAuthnCredential :exec
DELETE FROM RAC_webauthn_credentials
WHERE id = $1 AND user_id = $2;

-- name: GetUserByWebAuthnCredentialID :one
SELECT u.id, u.email, u.password_hash, u.is_email_verified,
       u.first_name, u.last_name, u.phone, u.onboarding_completed_at,
       u.created_at, u.updated_at
FROM RAC_users u
JOIN RAC_webauthn_credentials wc ON wc.user_id = u.id
WHERE wc.id = $1;
