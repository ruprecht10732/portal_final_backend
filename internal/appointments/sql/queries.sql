-- Appointments Domain SQL Queries

-- name: CreateAppointment :exec
INSERT INTO RAC_appointments (
	id, organization_id, user_id, lead_id, lead_service_id, type, title, description,
	location, meeting_link, start_time, end_time, status, all_day, created_at, updated_at
)
VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8,
	$9, $10, $11, $12, $13, $14, $15, $16
);

-- name: GetAppointmentByID :one
SELECT id, organization_id, user_id, lead_id, lead_service_id, type, title, description,
	location, meeting_link, start_time, end_time, status, all_day, created_at, updated_at
FROM RAC_appointments
WHERE id = $1 AND organization_id = $2;

-- name: GetAppointmentByLeadServiceID :one
SELECT id, organization_id, user_id, lead_id, lead_service_id, type, title, description,
	location, meeting_link, start_time, end_time, status, all_day, created_at, updated_at
FROM RAC_appointments
WHERE lead_service_id = $1 AND organization_id = $2 AND status != 'cancelled'
ORDER BY created_at DESC
LIMIT 1;

-- name: GetNextUpcomingScheduledVisitByLead :one
SELECT id, organization_id, user_id, lead_id, lead_service_id, type, title, description,
	location, meeting_link, start_time, end_time, status, all_day, created_at, updated_at
FROM RAC_appointments
WHERE lead_id = $1
	AND organization_id = $2
	AND type = 'lead_visit'
	AND status = 'scheduled'
	AND start_time > now()
ORDER BY start_time ASC
LIMIT 1;

-- name: GetLatestScheduledVisitByLead :one
SELECT id, organization_id, user_id, lead_id, lead_service_id, type, title, description,
	location, meeting_link, start_time, end_time, status, all_day, created_at, updated_at
FROM RAC_appointments
WHERE lead_id = $1
	AND organization_id = $2
	AND type = 'lead_visit'
	AND status = 'scheduled'
ORDER BY start_time DESC
LIMIT 1;

-- name: GetNextRequestedVisitByLead :one
SELECT id, organization_id, user_id, lead_id, lead_service_id, type, title, description,
	location, meeting_link, start_time, end_time, status, all_day, created_at, updated_at
FROM RAC_appointments
WHERE lead_id = $1
	AND organization_id = $2
	AND type = 'lead_visit'
	AND status = 'requested'
ORDER BY start_time ASC
LIMIT 1;

-- name: ListLeadVisitsByStatus :many
SELECT id, organization_id, user_id, lead_id, lead_service_id, type, title, description,
	location, meeting_link, start_time, end_time, status, all_day, created_at, updated_at
FROM RAC_appointments
WHERE lead_id = $1
	AND organization_id = $2
	AND type = 'lead_visit'
	AND status = ANY($3::text[])
ORDER BY start_time ASC;

-- name: UpdateAppointment :execrows
UPDATE RAC_appointments
SET title = $2,
	description = $3,
	location = $4,
	meeting_link = $5,
	start_time = $6,
	end_time = $7,
	all_day = $8,
	updated_at = $9
WHERE id = $1 AND organization_id = $10;

-- name: UpdateAppointmentStatus :execrows
UPDATE RAC_appointments
SET status = $3, updated_at = $4
WHERE id = $1 AND organization_id = $2;

-- name: DeleteAppointment :execrows
DELETE FROM RAC_appointments
WHERE id = $1 AND organization_id = $2;

-- name: CountAppointments :one
SELECT COUNT(*)::int AS countValue
FROM RAC_appointments
WHERE organization_id = sqlc.arg(organization_id)
	AND (sqlc.narg(user_id)::uuid IS NULL OR user_id = sqlc.narg(user_id)::uuid)
	AND (sqlc.narg(lead_id)::uuid IS NULL OR lead_id = sqlc.narg(lead_id)::uuid)
	AND (sqlc.narg(type)::text IS NULL OR type = sqlc.narg(type)::text)
	AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
	AND (sqlc.narg(start_from)::timestamptz IS NULL OR start_time >= sqlc.narg(start_from)::timestamptz)
	AND (sqlc.narg(start_to)::timestamptz IS NULL OR start_time <= sqlc.narg(start_to)::timestamptz)
	AND (sqlc.narg(search)::text IS NULL OR title ILIKE sqlc.narg(search)::text OR location ILIKE sqlc.narg(search)::text OR meeting_link ILIKE sqlc.narg(search)::text);

-- name: ListAppointments :many
SELECT id, organization_id, user_id, lead_id, lead_service_id, type, title, description,
	location, meeting_link, start_time, end_time, status, all_day, created_at, updated_at
FROM RAC_appointments
WHERE organization_id = sqlc.arg(organization_id)
	AND (sqlc.narg(user_id)::uuid IS NULL OR user_id = sqlc.narg(user_id)::uuid)
	AND (sqlc.narg(lead_id)::uuid IS NULL OR lead_id = sqlc.narg(lead_id)::uuid)
	AND (sqlc.narg(type)::text IS NULL OR type = sqlc.narg(type)::text)
	AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
	AND (sqlc.narg(start_from)::timestamptz IS NULL OR start_time >= sqlc.narg(start_from)::timestamptz)
	AND (sqlc.narg(start_to)::timestamptz IS NULL OR start_time <= sqlc.narg(start_to)::timestamptz)
	AND (sqlc.narg(search)::text IS NULL OR title ILIKE sqlc.narg(search)::text OR location ILIKE sqlc.narg(search)::text OR meeting_link ILIKE sqlc.narg(search)::text)
ORDER BY
	CASE WHEN sqlc.arg(sort_by)::text = 'title' AND sqlc.arg(sort_order)::text = 'asc' THEN title END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'title' AND sqlc.arg(sort_order)::text = 'desc' THEN title END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'type' AND sqlc.arg(sort_order)::text = 'asc' THEN type END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'type' AND sqlc.arg(sort_order)::text = 'desc' THEN type END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'status' AND sqlc.arg(sort_order)::text = 'asc' THEN status END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'status' AND sqlc.arg(sort_order)::text = 'desc' THEN status END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'startTime' AND sqlc.arg(sort_order)::text = 'asc' THEN start_time END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'startTime' AND sqlc.arg(sort_order)::text = 'desc' THEN start_time END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'endTime' AND sqlc.arg(sort_order)::text = 'asc' THEN end_time END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'endTime' AND sqlc.arg(sort_order)::text = 'desc' THEN end_time END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'createdAt' AND sqlc.arg(sort_order)::text = 'asc' THEN created_at END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'createdAt' AND sqlc.arg(sort_order)::text = 'desc' THEN created_at END DESC,
	start_time ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: GetAppointmentLeadInfo :one
SELECT id, consumer_first_name, consumer_last_name, consumer_phone, address_street, address_house_number, address_city
FROM RAC_leads
WHERE id = $1 AND organization_id = $2;

-- name: GetAppointmentLeadEmail :one
SELECT COALESCE(consumer_email, '') AS consumerEmailValue
FROM RAC_leads
WHERE id = $1 AND organization_id = $2;

-- name: ListAppointmentLeadInfoByIDs :many
SELECT id, consumer_first_name, consumer_last_name, consumer_phone, address_street, address_house_number, address_city
FROM RAC_leads
WHERE id = ANY($1::uuid[]) AND organization_id = $2;

-- name: ListAppointmentsForDateRange :many
SELECT id, organization_id, user_id, lead_id, lead_service_id, type, title, description,
	location, meeting_link, start_time, end_time, status, all_day, created_at, updated_at
FROM RAC_appointments
WHERE organization_id = $1 AND user_id = $2
	AND start_time < $4 AND end_time > $3
	AND status = 'scheduled'
ORDER BY start_time ASC;

-- name: CreateAvailabilityRule :one
INSERT INTO RAC_appointment_availability_rules
	(id, organization_id, user_id, weekday, start_time, end_time, timezone)
VALUES
	($1, $2, $3, $4, $5, $6, $7)
RETURNING id, organization_id, user_id, weekday, start_time, end_time, timezone, created_at, updated_at;

-- name: ListAvailabilityRules :many
SELECT id, organization_id, user_id, weekday, start_time, end_time, timezone, created_at, updated_at
FROM RAC_appointment_availability_rules
WHERE organization_id = $1 AND user_id = $2
ORDER BY weekday, start_time;

-- name: ListAvailabilityRuleUserIDs :many
SELECT DISTINCT user_id AS userIDValue
FROM RAC_appointment_availability_rules
WHERE organization_id = $1;

-- name: GetAvailabilityRuleByID :one
SELECT id, organization_id, user_id, weekday, start_time, end_time, timezone, created_at, updated_at
FROM RAC_appointment_availability_rules
WHERE id = $1 AND organization_id = $2;

-- name: DeleteAvailabilityRule :execrows
DELETE FROM RAC_appointment_availability_rules
WHERE id = $1 AND organization_id = $2;

-- name: UpdateAvailabilityRule :one
UPDATE RAC_appointment_availability_rules
SET weekday = $3,
	start_time = $4,
	end_time = $5,
	timezone = $6,
	updated_at = now()
WHERE id = $1 AND organization_id = $2
RETURNING id, organization_id, user_id, weekday, start_time, end_time, timezone, created_at, updated_at;

-- name: CreateAvailabilityOverride :one
INSERT INTO RAC_appointment_availability_overrides
	(id, organization_id, user_id, date, is_available, start_time, end_time, timezone)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, organization_id, user_id, date, is_available, start_time, end_time, timezone, created_at, updated_at;

-- name: ListAvailabilityOverrides :many
SELECT id, organization_id, user_id, date, is_available, start_time, end_time, timezone, created_at, updated_at
FROM RAC_appointment_availability_overrides
WHERE organization_id = $1 AND user_id = $2
	AND ($3::date IS NULL OR date >= $3)
	AND ($4::date IS NULL OR date <= $4)
ORDER BY date ASC;

-- name: GetAvailabilityOverrideByID :one
SELECT id, organization_id, user_id, date, is_available, start_time, end_time, timezone, created_at, updated_at
FROM RAC_appointment_availability_overrides
WHERE id = $1 AND organization_id = $2;

-- name: DeleteAvailabilityOverride :execrows
DELETE FROM RAC_appointment_availability_overrides
WHERE id = $1 AND organization_id = $2;

-- name: UpdateAvailabilityOverride :one
UPDATE RAC_appointment_availability_overrides
SET date = $3,
	is_available = $4,
	start_time = $5,
	end_time = $6,
	timezone = $7,
	updated_at = now()
WHERE id = $1 AND organization_id = $2
RETURNING id, organization_id, user_id, date, is_available, start_time, end_time, timezone, created_at, updated_at;

-- name: GetAppointmentVisitReport :one
SELECT appointment_id, organization_id, measurements, access_difficulty, notes, created_at, updated_at
FROM RAC_appointment_visit_reports
WHERE appointment_id = $1 AND organization_id = $2;

-- name: UpsertAppointmentVisitReport :one
INSERT INTO RAC_appointment_visit_reports
	(appointment_id, organization_id, measurements, access_difficulty, notes, created_at, updated_at)
VALUES
	($1, $2, $3, $4, $5, now(), now())
ON CONFLICT (appointment_id)
DO UPDATE SET
	measurements = EXCLUDED.measurements,
	access_difficulty = EXCLUDED.access_difficulty,
	notes = EXCLUDED.notes,
	updated_at = now()
RETURNING appointment_id, organization_id, measurements, access_difficulty, notes, created_at, updated_at;

-- name: CreateAppointmentAttachment :one
INSERT INTO RAC_appointment_attachments
	(id, appointment_id, organization_id, file_key, file_name, content_type, size_bytes)
VALUES
	($1, $2, $3, $4, $5, $6, $7)
RETURNING id, appointment_id, organization_id, file_key, file_name, content_type, size_bytes, created_at;

-- name: ListAppointmentAttachments :many
SELECT id, appointment_id, organization_id, file_key, file_name, content_type, size_bytes, created_at
FROM RAC_appointment_attachments
WHERE appointment_id = $1 AND organization_id = $2
ORDER BY created_at ASC;