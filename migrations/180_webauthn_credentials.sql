-- +goose Up
-- WebAuthn (Passkey) credentials: one-to-many relationship with RAC_users.
CREATE TABLE RAC_webauthn_credentials (
    id              BYTEA        PRIMARY KEY,            -- Credential ID from authenticator
    user_id         UUID         NOT NULL REFERENCES RAC_users(id) ON DELETE CASCADE,
    public_key      BYTEA        NOT NULL,               -- COSE-encoded public key
    attestation_type TEXT        NOT NULL DEFAULT 'none', -- e.g. "packed", "none"
    transport       TEXT[]       NOT NULL DEFAULT '{}',   -- e.g. {"internal","hybrid"}
    flags_json      JSONB        NOT NULL DEFAULT '{}',   -- CredentialFlags (UP, UV, BE, BS)
    aaguid          BYTEA        NOT NULL DEFAULT '\x00000000000000000000000000000000', -- Authenticator AAGUID
    sign_count      BIGINT       NOT NULL DEFAULT 0,      -- Counter for clone detection
    clone_warning   BOOLEAN      NOT NULL DEFAULT FALSE,
    nickname        TEXT         NOT NULL DEFAULT '',      -- User-friendly label
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    last_used_at    TIMESTAMPTZ
);

CREATE INDEX idx_webauthn_credentials_user_id ON RAC_webauthn_credentials(user_id);
