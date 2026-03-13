-- name: InsertAgentMessage :exec
INSERT INTO RAC_whatsapp_agent_messages (organization_id, phone_number, role, content)
VALUES ($1, $2, $3, $4);

-- name: GetRecentAgentMessages :many
SELECT id, organization_id, phone_number, role, content, created_at
FROM RAC_whatsapp_agent_messages
WHERE phone_number = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: GetAgentUserByPhone :one
SELECT phone_number, organization_id, display_name, created_at
FROM RAC_whatsapp_agent_users
WHERE phone_number = $1;

-- name: ListAgentUsersByOrganization :many
SELECT phone_number, organization_id, display_name, created_at
FROM RAC_whatsapp_agent_users
WHERE organization_id = $1
ORDER BY created_at DESC;

-- name: CreateAgentUser :exec
INSERT INTO RAC_whatsapp_agent_users (phone_number, organization_id, display_name)
VALUES ($1, $2, $3);

-- name: DeleteAgentUser :exec
DELETE FROM RAC_whatsapp_agent_users
WHERE phone_number = $1 AND organization_id = $2;

-- name: GetAgentConfig :one
SELECT id, device_id, account_jid, created_at, updated_at
FROM RAC_whatsapp_agent_config
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
