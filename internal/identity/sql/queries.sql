-- Identity Domain SQL Queries

-- name: CreateOrganization :one
INSERT INTO RAC_organizations (name, created_by)
VALUES ($1, $2)
RETURNING id, name, email, phone, vat_number, kvk_number, address_line1, address_line2, postal_code, city, country,
  logo_file_key, logo_file_name, logo_content_type, logo_size_bytes,
  created_by, created_at, updated_at;

-- name: GetOrganization :one
SELECT id, name, email, phone, vat_number, kvk_number, address_line1, address_line2, postal_code, city, country,
  logo_file_key, logo_file_name, logo_content_type, logo_size_bytes,
  created_by, created_at, updated_at
FROM RAC_organizations
WHERE id = $1;

-- name: UpdateOrganizationProfile :one
UPDATE RAC_organizations
SET
  name = COALESCE(sqlc.narg('name')::text, name),
  email = COALESCE(sqlc.narg('email')::text, email),
  phone = COALESCE(sqlc.narg('phone')::text, phone),
  vat_number = COALESCE(sqlc.narg('vat_number')::text, vat_number),
  kvk_number = COALESCE(sqlc.narg('kvk_number')::text, kvk_number),
  address_line1 = COALESCE(sqlc.narg('address_line1')::text, address_line1),
  address_line2 = COALESCE(sqlc.narg('address_line2')::text, address_line2),
  postal_code = COALESCE(sqlc.narg('postal_code')::text, postal_code),
  city = COALESCE(sqlc.narg('city')::text, city),
  country = COALESCE(sqlc.narg('country')::text, country),
  updated_at = now()
WHERE id = sqlc.arg('id')::uuid
RETURNING id, name, email, phone, vat_number, kvk_number, address_line1, address_line2, postal_code, city, country,
  logo_file_key, logo_file_name, logo_content_type, logo_size_bytes,
  created_by, created_at, updated_at;

-- name: UpdateOrganizationLogo :one
UPDATE RAC_organizations
SET
  logo_file_key = $2,
  logo_file_name = $3,
  logo_content_type = $4,
  logo_size_bytes = $5,
  updated_at = now()
WHERE id = $1
RETURNING id, name, email, phone, vat_number, kvk_number, address_line1, address_line2, postal_code, city, country,
  logo_file_key, logo_file_name, logo_content_type, logo_size_bytes,
  created_by, created_at, updated_at;

-- name: ClearOrganizationLogo :one
UPDATE RAC_organizations
SET
  logo_file_key = NULL,
  logo_file_name = NULL,
  logo_content_type = NULL,
  logo_size_bytes = NULL,
  updated_at = now()
WHERE id = $1
RETURNING id, name, email, phone, vat_number, kvk_number, address_line1, address_line2, postal_code, city, country,
  logo_file_key, logo_file_name, logo_content_type, logo_size_bytes,
  created_by, created_at, updated_at;

-- name: GetOrganizationSettings :one
SELECT organization_id, quote_payment_days, quote_valid_days,
       ai_auto_disqualify_junk, ai_auto_dispatch, ai_auto_estimate, ai_confidence_gate_enabled,
       ai_adaptive_reasoning_enabled, ai_experience_memory_enabled, ai_council_enabled,
       ai_council_consensus_mode,
       catalog_gap_threshold, catalog_gap_lookback_days,
       notification_email, whatsapp_device_id, whatsapp_welcome_delay_minutes,
       smtp_host, smtp_port, smtp_username, smtp_password, smtp_from_email, smtp_from_name,
       created_at, updated_at
FROM RAC_organization_settings
WHERE organization_id = $1;

-- name: UpsertOrganizationSettings :one
INSERT INTO RAC_organization_settings (
  organization_id,
  quote_payment_days,
  quote_valid_days,
  ai_auto_disqualify_junk,
  ai_auto_dispatch,
  ai_auto_estimate,
  ai_confidence_gate_enabled,
  ai_adaptive_reasoning_enabled,
  ai_experience_memory_enabled,
  ai_council_enabled,
  ai_council_consensus_mode,
  catalog_gap_threshold,
  catalog_gap_lookback_days,
  notification_email,
  whatsapp_device_id,
  whatsapp_welcome_delay_minutes
)
VALUES (
  sqlc.arg('organization_id')::uuid,
  COALESCE(sqlc.narg('quote_payment_days')::int, 7),
  COALESCE(sqlc.narg('quote_valid_days')::int, 14),
  COALESCE(sqlc.narg('ai_auto_disqualify_junk')::boolean, true),
  COALESCE(sqlc.narg('ai_auto_dispatch')::boolean, false),
  COALESCE(sqlc.narg('ai_auto_estimate')::boolean, true),
  COALESCE(sqlc.narg('ai_confidence_gate_enabled')::boolean, false),
  COALESCE(sqlc.narg('ai_adaptive_reasoning_enabled')::boolean, true),
  COALESCE(sqlc.narg('ai_experience_memory_enabled')::boolean, true),
  COALESCE(sqlc.narg('ai_council_enabled')::boolean, true),
  COALESCE(NULLIF(sqlc.narg('ai_council_consensus_mode')::text, ''), 'weighted'),
  COALESCE(sqlc.narg('catalog_gap_threshold')::int, 3),
  COALESCE(sqlc.narg('catalog_gap_lookback_days')::int, 30),
  NULLIF(sqlc.narg('notification_email')::text, ''),
  NULLIF(sqlc.narg('whatsapp_device_id')::text, ''),
  COALESCE(sqlc.narg('whatsapp_welcome_delay_minutes')::int, 2)
)
ON CONFLICT (organization_id) DO UPDATE SET
  quote_payment_days = COALESCE(sqlc.narg('quote_payment_days')::int, RAC_organization_settings.quote_payment_days),
  quote_valid_days = COALESCE(sqlc.narg('quote_valid_days')::int, RAC_organization_settings.quote_valid_days),
  ai_auto_disqualify_junk = COALESCE(sqlc.narg('ai_auto_disqualify_junk')::boolean, RAC_organization_settings.ai_auto_disqualify_junk),
  ai_auto_dispatch = COALESCE(sqlc.narg('ai_auto_dispatch')::boolean, RAC_organization_settings.ai_auto_dispatch),
  ai_auto_estimate = COALESCE(sqlc.narg('ai_auto_estimate')::boolean, RAC_organization_settings.ai_auto_estimate),
  ai_confidence_gate_enabled = COALESCE(sqlc.narg('ai_confidence_gate_enabled')::boolean, RAC_organization_settings.ai_confidence_gate_enabled),
  ai_adaptive_reasoning_enabled = COALESCE(sqlc.narg('ai_adaptive_reasoning_enabled')::boolean, RAC_organization_settings.ai_adaptive_reasoning_enabled),
  ai_experience_memory_enabled = COALESCE(sqlc.narg('ai_experience_memory_enabled')::boolean, RAC_organization_settings.ai_experience_memory_enabled),
  ai_council_enabled = COALESCE(sqlc.narg('ai_council_enabled')::boolean, RAC_organization_settings.ai_council_enabled),
  ai_council_consensus_mode = COALESCE(NULLIF(sqlc.narg('ai_council_consensus_mode')::text, ''), RAC_organization_settings.ai_council_consensus_mode),
  catalog_gap_threshold = COALESCE(sqlc.narg('catalog_gap_threshold')::int, RAC_organization_settings.catalog_gap_threshold),
  catalog_gap_lookback_days = COALESCE(sqlc.narg('catalog_gap_lookback_days')::int, RAC_organization_settings.catalog_gap_lookback_days),
  notification_email = CASE WHEN sqlc.narg('notification_email')::text IS NULL THEN RAC_organization_settings.notification_email ELSE NULLIF(sqlc.narg('notification_email')::text, '') END,
  whatsapp_device_id = CASE WHEN sqlc.narg('whatsapp_device_id')::text IS NULL THEN RAC_organization_settings.whatsapp_device_id ELSE NULLIF(sqlc.narg('whatsapp_device_id')::text, '') END,
  whatsapp_welcome_delay_minutes = COALESCE(sqlc.narg('whatsapp_welcome_delay_minutes')::int, RAC_organization_settings.whatsapp_welcome_delay_minutes),
  updated_at = now()
RETURNING organization_id, quote_payment_days, quote_valid_days,
  ai_auto_disqualify_junk, ai_auto_dispatch, ai_auto_estimate, ai_confidence_gate_enabled,
  ai_adaptive_reasoning_enabled, ai_experience_memory_enabled, ai_council_enabled,
  ai_council_consensus_mode,
  catalog_gap_threshold, catalog_gap_lookback_days,
  notification_email, whatsapp_device_id, whatsapp_welcome_delay_minutes,
  smtp_host, smtp_port, smtp_username, smtp_password, smtp_from_email, smtp_from_name,
  created_at, updated_at;

-- name: UpsertOrganizationSMTP :one
INSERT INTO RAC_organization_settings (organization_id, smtp_host, smtp_port, smtp_username, smtp_password, smtp_from_email, smtp_from_name)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (organization_id) DO UPDATE SET
  smtp_host = $2,
  smtp_port = $3,
  smtp_username = $4,
  smtp_password = $5,
  smtp_from_email = $6,
  smtp_from_name = $7,
  updated_at = now()
RETURNING organization_id, quote_payment_days, quote_valid_days,
  ai_auto_disqualify_junk, ai_auto_dispatch, ai_auto_estimate, ai_confidence_gate_enabled,
  ai_adaptive_reasoning_enabled, ai_experience_memory_enabled, ai_council_enabled,
  ai_council_consensus_mode,
  catalog_gap_threshold, catalog_gap_lookback_days,
  notification_email, whatsapp_device_id, whatsapp_welcome_delay_minutes,
  smtp_host, smtp_port, smtp_username, smtp_password, smtp_from_email, smtp_from_name,
  created_at, updated_at;

-- name: ClearOrganizationSMTP :exec
UPDATE RAC_organization_settings
SET smtp_host = NULL, smtp_port = NULL, smtp_username = NULL, smtp_password = NULL,
    smtp_from_email = NULL, smtp_from_name = NULL, updated_at = now()
WHERE organization_id = $1;

-- name: AddMember :exec
INSERT INTO RAC_organization_members (organization_id, user_id)
VALUES ($1, $2);

-- name: GetUserOrganizationID :one
SELECT organization_id
FROM RAC_organization_members
WHERE user_id = $1;

-- name: CreateInvite :one
INSERT INTO RAC_organization_invites (organization_id, email, token_hash, expires_at, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, organization_id, email, token_hash, expires_at, created_by, created_at, used_at, used_by;

-- name: GetInviteByToken :one
SELECT id, organization_id, email, token_hash, expires_at, created_by, created_at, used_at, used_by
FROM RAC_organization_invites
WHERE token_hash = $1;

-- name: UseInvite :exec
UPDATE RAC_organization_invites
SET used_at = now(), used_by = $2
WHERE id = $1 AND used_at IS NULL;

-- name: ListInvites :many
SELECT id, organization_id, email, token_hash, expires_at, created_by, created_at, used_at, used_by
FROM RAC_organization_invites
WHERE organization_id = $1
ORDER BY created_at DESC;

-- name: UpdateInvite :one
UPDATE RAC_organization_invites
SET
  email = COALESCE(sqlc.narg('email')::text, email),
  token_hash = COALESCE(sqlc.narg('token_hash')::text, token_hash),
  expires_at = COALESCE(sqlc.narg('expires_at')::timestamptz, expires_at)
WHERE id = sqlc.arg('id')::uuid AND organization_id = sqlc.arg('organization_id')::uuid AND used_at IS NULL
RETURNING id, organization_id, email, token_hash, expires_at, created_by, created_at, used_at, used_by;

-- name: RevokeInvite :one
UPDATE RAC_organization_invites
SET expires_at = now()
WHERE id = $1 AND organization_id = $2 AND used_at IS NULL
RETURNING id, organization_id, email, token_hash, expires_at, created_by, created_at, used_at, used_by;

-- name: ListWorkflows :many
SELECT id, organization_id, workflow_key, name, description, enabled,
       quote_valid_days_override, quote_payment_days_override, created_at, updated_at
FROM RAC_workflows
WHERE organization_id = $1
ORDER BY workflow_key ASC;

-- name: ListWorkflowSteps :many
SELECT id, organization_id, workflow_id, trigger, channel, audience, action,
       step_order, delay_minutes, enabled, recipient_config, template_subject,
       template_body, stop_on_reply, created_at, updated_at
FROM RAC_workflow_steps
WHERE organization_id = $1
ORDER BY workflow_id ASC, trigger ASC, channel ASC, step_order ASC;

-- name: UpsertWorkflow :one
INSERT INTO RAC_workflows (
  id, organization_id, workflow_key, name, description, enabled,
  quote_valid_days_override, quote_payment_days_override
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (organization_id, workflow_key) DO UPDATE
SET
  name = EXCLUDED.name,
  description = EXCLUDED.description,
  enabled = EXCLUDED.enabled,
  quote_valid_days_override = EXCLUDED.quote_valid_days_override,
  quote_payment_days_override = EXCLUDED.quote_payment_days_override,
  updated_at = now()
RETURNING id;

-- name: DeleteWorkflowsByOrganization :exec
DELETE FROM RAC_workflows
WHERE organization_id = $1;

-- name: DeleteWorkflowsNotInList :exec
DELETE FROM RAC_workflows
WHERE organization_id = $1
  AND NOT (id = ANY($2::uuid[]));

-- name: UpsertWorkflowStep :one
INSERT INTO RAC_workflow_steps (
  id, organization_id, workflow_id, trigger, channel, audience, action,
  step_order, delay_minutes, enabled, recipient_config, template_subject,
  template_body, stop_on_reply
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, $12, $13, $14)
ON CONFLICT (workflow_id, trigger, channel, step_order) DO UPDATE
SET
  audience = EXCLUDED.audience,
  action = EXCLUDED.action,
  delay_minutes = EXCLUDED.delay_minutes,
  enabled = EXCLUDED.enabled,
  recipient_config = EXCLUDED.recipient_config,
  template_subject = EXCLUDED.template_subject,
  template_body = EXCLUDED.template_body,
  stop_on_reply = EXCLUDED.stop_on_reply,
  updated_at = now()
RETURNING id;

-- name: DeleteWorkflowStepsByWorkflow :exec
DELETE FROM RAC_workflow_steps
WHERE organization_id = $1 AND workflow_id = $2;

-- name: DeleteWorkflowStepsNotInList :exec
DELETE FROM RAC_workflow_steps
WHERE organization_id = $1
  AND workflow_id = $2
  AND NOT (id = ANY($3::uuid[]));

-- name: DefaultWorkflowAssignmentRuleExists :one
SELECT EXISTS (
  SELECT 1
  FROM RAC_workflow_assignment_rules
  WHERE organization_id = $1
    AND lead_source IS NULL
    AND lead_service_type IS NULL
    AND pipeline_stage IS NULL
) AS exists;

-- name: CreateWorkflowAssignmentRule :exec
INSERT INTO RAC_workflow_assignment_rules (
  id, organization_id, workflow_id, name, enabled, priority,
  lead_source, lead_service_type, pipeline_stage
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: CreateDefaultWorkflowAssignmentRule :exec
INSERT INTO RAC_workflow_assignment_rules (
  organization_id,
  workflow_id,
  name,
  enabled,
  priority,
  lead_source,
  lead_service_type,
  pipeline_stage
)
VALUES ($1, $2, $3, TRUE, $4, NULL, NULL, NULL);

-- name: ListWorkflowAssignmentRules :many
SELECT id, organization_id, workflow_id, name, enabled, priority,
       lead_source, lead_service_type, pipeline_stage, created_at, updated_at
FROM RAC_workflow_assignment_rules
WHERE organization_id = $1
ORDER BY priority ASC, created_at ASC;

-- name: DeleteWorkflowAssignmentRulesByOrganization :exec
DELETE FROM RAC_workflow_assignment_rules
WHERE organization_id = $1;

-- name: GetLeadWorkflowOverride :one
SELECT lead_id, organization_id, workflow_id, override_mode, reason, assigned_by, created_at, updated_at
FROM RAC_lead_workflow_overrides
WHERE lead_id = $1 AND organization_id = $2;

-- name: UpsertLeadWorkflowOverride :one
INSERT INTO RAC_lead_workflow_overrides (
  lead_id, organization_id, workflow_id, override_mode, reason, assigned_by
)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (lead_id) DO UPDATE SET
  workflow_id = EXCLUDED.workflow_id,
  override_mode = EXCLUDED.override_mode,
  reason = EXCLUDED.reason,
  assigned_by = EXCLUDED.assigned_by,
  updated_at = now()
RETURNING lead_id, organization_id, workflow_id, override_mode, reason, assigned_by, created_at, updated_at;

-- name: DeleteLeadWorkflowOverride :exec
DELETE FROM RAC_lead_workflow_overrides
WHERE lead_id = $1 AND organization_id = $2;

-- name: LeadExistsInOrganization :one
SELECT EXISTS (
  SELECT 1
  FROM RAC_leads
  WHERE id = $1 AND organization_id = $2
) AS exists;

-- name: WorkflowExistsInOrganization :one
SELECT EXISTS (
  SELECT 1
  FROM RAC_workflows
  WHERE id = $1 AND organization_id = $2
) AS exists;
