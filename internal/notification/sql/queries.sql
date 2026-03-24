-- name: GetNotificationOrganizationName :one
SELECT name
FROM rac_organizations
WHERE id = $1;

-- name: GetNotificationLeadDetails :one
SELECT l.consumer_first_name, l.consumer_last_name, l.consumer_phone, l.consumer_email,
	l.address_street, l.address_house_number, l.address_zip_code, l.address_city,
	l.public_token,
	COALESCE(st.name, '') AS service_type
FROM rac_leads l
LEFT JOIN LATERAL (
	SELECT ls.service_type_id
	FROM RAC_lead_services ls
	WHERE ls.lead_id = l.id AND ls.organization_id = l.organization_id
	ORDER BY ls.created_at DESC
	LIMIT 1
) latest_ls ON true
LEFT JOIN RAC_service_types st ON st.id = latest_ls.service_type_id AND st.organization_id = l.organization_id
WHERE l.id = $1 AND l.organization_id = $2;

-- name: CreateInAppNotification :one
INSERT INTO RAC_in_app_notifications (
	organization_id,
	user_id,
	title,
	content,
	resource_id,
	resource_type,
	category
) VALUES (
	sqlc.arg(organization_id)::uuid,
	sqlc.arg(user_id)::uuid,
	sqlc.arg(title)::text,
	sqlc.arg(content)::text,
	sqlc.narg(resource_id)::uuid,
	sqlc.narg(resource_type)::text,
	sqlc.arg(category)::text
)
RETURNING *;

-- name: CountInAppNotifications :one
SELECT COUNT(*)::bigint
FROM RAC_in_app_notifications
WHERE user_id = sqlc.arg(user_id)::uuid;

-- name: ListInAppNotifications :many
SELECT *
FROM RAC_in_app_notifications
WHERE user_id = sqlc.arg(user_id)::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;

-- name: CountUnreadInAppNotifications :one
SELECT COUNT(*)::bigint
FROM RAC_in_app_notifications
WHERE user_id = sqlc.arg(user_id)::uuid
  AND is_read = FALSE;

-- name: CountUnreadInAppNotificationsByResourceTypes :one
SELECT COUNT(*)::bigint
FROM RAC_in_app_notifications
WHERE user_id = sqlc.arg(user_id)::uuid
  AND is_read = FALSE
  AND resource_type = ANY(sqlc.arg(resource_types)::text[]);

-- name: MarkInAppNotificationRead :exec
UPDATE RAC_in_app_notifications
SET is_read = TRUE,
	read_at = now()
WHERE id = sqlc.arg(notification_id)::uuid
  AND user_id = sqlc.arg(user_id)::uuid;

-- name: MarkAllInAppNotificationsRead :exec
UPDATE RAC_in_app_notifications
SET is_read = TRUE,
	read_at = now()
WHERE user_id = sqlc.arg(user_id)::uuid
  AND is_read = FALSE;

-- name: DeleteInAppNotification :exec
DELETE FROM RAC_in_app_notifications
WHERE id = sqlc.arg(notification_id)::uuid
  AND user_id = sqlc.arg(user_id)::uuid;

-- name: InsertNotificationOutbox :one
INSERT INTO RAC_notification_outbox (
	tenant_id,
	lead_id,
	service_id,
	kind,
	template,
	payload,
	run_at,
	status,
	last_error
) VALUES (
	sqlc.arg(tenant_id)::uuid,
	sqlc.narg(lead_id)::uuid,
	sqlc.narg(service_id)::uuid,
	sqlc.arg(kind)::text,
	sqlc.arg(template)::text,
	sqlc.arg(payload)::jsonb,
	sqlc.arg(run_at)::timestamptz,
	sqlc.arg(status)::text,
	sqlc.narg(last_error)::text
)
RETURNING id;

-- name: GetNotificationOutboxByID :one
SELECT *
FROM RAC_notification_outbox
WHERE id = sqlc.arg(id)::uuid;

-- name: ClaimPendingNotificationOutbox :many
WITH cte AS (
	SELECT id
	FROM RAC_notification_outbox
	WHERE status = 'pending'
	ORDER BY run_at ASC
	LIMIT sqlc.arg(limit_count)::int
	FOR UPDATE SKIP LOCKED
)
UPDATE RAC_notification_outbox AS o
SET status = 'enqueued',
	updated_at = now()
FROM cte
WHERE o.id = cte.id
RETURNING o.*;

-- name: MarkNotificationOutboxPending :exec
UPDATE RAC_notification_outbox
SET status = 'pending',
	last_error = sqlc.narg(last_error)::text,
	updated_at = now()
WHERE id = sqlc.arg(id)::uuid;

-- name: ScheduleNotificationOutboxRetry :exec
UPDATE RAC_notification_outbox
SET status = 'pending',
	run_at = sqlc.arg(run_at)::timestamptz,
	last_error = sqlc.arg(last_error)::text,
	updated_at = now()
WHERE id = sqlc.arg(id)::uuid;

-- name: MarkNotificationOutboxProcessing :exec
UPDATE RAC_notification_outbox
SET status = 'processing',
	attempts = attempts + 1,
	updated_at = now()
WHERE id = sqlc.arg(id)::uuid;

-- name: MarkNotificationOutboxSucceeded :exec
UPDATE RAC_notification_outbox
SET status = 'succeeded',
	last_error = NULL,
	updated_at = now()
WHERE id = sqlc.arg(id)::uuid;

-- name: MarkNotificationOutboxFailed :exec
UPDATE RAC_notification_outbox
SET status = 'failed',
	last_error = sqlc.arg(last_error)::text,
	updated_at = now()
WHERE id = sqlc.arg(id)::uuid;

-- name: CancelPendingNotificationOutboxForLead :execrows
UPDATE RAC_notification_outbox
SET status = sqlc.arg(cancelled_status)::text,
	updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
  AND lead_id = sqlc.arg(lead_id)::uuid
  AND status = sqlc.arg(pending_status)::text;