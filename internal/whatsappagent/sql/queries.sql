-- name: InsertAgentMessage :exec
INSERT INTO RAC_whatsapp_agent_messages (organization_id, phone_number, role, content, external_message_id, metadata)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: UpdateAgentMessageByExternalID :exec
UPDATE RAC_whatsapp_agent_messages
SET content = $3,
    metadata = $4
WHERE organization_id = $1
    AND external_message_id = $2;

-- name: DeleteAgentMessagesByPhone :exec
DELETE FROM RAC_whatsapp_agent_messages
WHERE organization_id = $1
    AND phone_number = $2;

-- name: GetRecentAgentMessages :many
SELECT id, organization_id, phone_number, role, content, external_message_id, metadata, created_at
FROM RAC_whatsapp_agent_messages
WHERE organization_id = $1
    AND phone_number = $2
ORDER BY created_at DESC, id DESC
LIMIT $3;

-- name: GetRecentInboundAgentMessages :many
SELECT id, organization_id, phone_number, role, content, external_message_id, metadata, created_at
FROM RAC_whatsapp_agent_messages
WHERE organization_id = $1
    AND phone_number = $2
    AND role = 'user'
ORDER BY created_at DESC, id DESC
LIMIT $3;

-- name: GetAgentMessageByExternalID :one
SELECT id, organization_id, phone_number, role, content, external_message_id, metadata, created_at
FROM RAC_whatsapp_agent_messages
WHERE organization_id = $1
    AND external_message_id = $2
LIMIT 1;

-- name: UpsertAgentVoiceTranscription :exec
INSERT INTO RAC_whatsapp_agent_voice_transcriptions (
    organization_id,
    external_message_id,
    phone_number,
    status,
    provider,
    error_message
)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (organization_id, external_message_id) DO UPDATE SET
    phone_number = EXCLUDED.phone_number,
    status = EXCLUDED.status,
    provider = EXCLUDED.provider,
    error_message = EXCLUDED.error_message,
    updated_at = now();

-- name: GetAgentVoiceTranscriptionByExternalID :one
SELECT id, organization_id, external_message_id, phone_number, status, storage_bucket, storage_key, content_type, provider, language, confidence_score, transcript_text, error_message, created_at, updated_at, completed_at
FROM RAC_whatsapp_agent_voice_transcriptions
WHERE organization_id = $1
    AND external_message_id = $2
LIMIT 1;

-- name: MarkAgentVoiceTranscriptionProcessing :exec
UPDATE RAC_whatsapp_agent_voice_transcriptions
SET status = 'processing',
    error_message = NULL,
    updated_at = now()
WHERE organization_id = $1
    AND external_message_id = $2;

-- name: UpdateAgentVoiceTranscriptionStorage :exec
UPDATE RAC_whatsapp_agent_voice_transcriptions
SET storage_bucket = $3,
    storage_key = $4,
    content_type = $5,
    updated_at = now()
WHERE organization_id = $1
    AND external_message_id = $2;

-- name: MarkAgentVoiceTranscriptionCompleted :exec
UPDATE RAC_whatsapp_agent_voice_transcriptions
SET status = 'completed',
    transcript_text = $3,
    provider = $4,
    language = $5,
    confidence_score = $6,
    error_message = NULL,
    updated_at = now(),
    completed_at = now()
WHERE organization_id = $1
    AND external_message_id = $2;

-- name: MarkAgentVoiceTranscriptionFailed :exec
UPDATE RAC_whatsapp_agent_voice_transcriptions
SET status = 'failed',
    error_message = $3,
    updated_at = now()
WHERE organization_id = $1
    AND external_message_id = $2;

-- name: GetAgentUserByPhone :one
SELECT phone_number, organization_id, display_name, user_type, partner_id, created_at
FROM RAC_whatsapp_agent_users
WHERE phone_number = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: GetAllAgentUsersByPhone :many
SELECT phone_number, organization_id, display_name, user_type, partner_id, created_at
FROM RAC_whatsapp_agent_users
WHERE phone_number = $1
ORDER BY created_at DESC;

-- name: ListAgentUsersByOrganization :many
SELECT phone_number, organization_id, display_name, user_type, partner_id, created_at
FROM RAC_whatsapp_agent_users
WHERE organization_id = $1
ORDER BY created_at DESC;

-- name: CreateAgentUser :exec
INSERT INTO RAC_whatsapp_agent_users (phone_number, organization_id, display_name, user_type, partner_id)
VALUES ($1, $2, $3, $4, $5);

-- name: DeleteAgentUser :exec
DELETE FROM RAC_whatsapp_agent_users
WHERE phone_number = $1 AND organization_id = $2;

-- name: GetAgentConfig :one
SELECT id, device_id, account_jid, created_at, updated_at
FROM RAC_whatsapp_agent_config
ORDER BY created_at DESC
LIMIT 1;

-- name: UpsertAgentConfig :one
INSERT INTO RAC_whatsapp_agent_config (device_id, account_jid)
VALUES ($1, $2)
ON CONFLICT (device_id) DO UPDATE SET
    account_jid = EXCLUDED.account_jid,
    updated_at = now()
RETURNING id, device_id, account_jid, created_at, updated_at;

-- name: DeleteAgentConfig :exec
DELETE FROM RAC_whatsapp_agent_config;

-- name: GetAgentConfigByDeviceID :one
SELECT id, device_id, account_jid, created_at, updated_at
FROM RAC_whatsapp_agent_config
WHERE device_id = $1;