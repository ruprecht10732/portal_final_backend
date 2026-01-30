-- Leads Domain SQL Queries

-- name: CreateLead :one
INSERT INTO leads (
    consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
    address_street, address_house_number, address_zip_code, address_city,
    service_type, status, assigned_agent_id,
    consumer_note, source
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'New', $11, $12, $13)
RETURNING *;

-- name: GetLeadByID :one
SELECT * FROM leads WHERE id = $1 AND deleted_at IS NULL;

-- name: GetLeadByPhone :one
SELECT * FROM leads 
WHERE consumer_phone = $1 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateLeadStatus :one
UPDATE leads SET status = $2, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SetLeadViewedBy :exec
UPDATE leads SET viewed_by_id = $2, viewed_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: ScheduleLeadVisit :one
UPDATE leads SET 
    visit_scheduled_date = $2, 
    visit_scout_id = $3,
    status = 'Scheduled',
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: CompleteLeadSurvey :one
UPDATE leads SET 
    visit_measurements = $2,
    visit_access_difficulty = $3,
    visit_notes = $4,
    visit_completed_at = now(),
    status = 'Surveyed',
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteLead :exec
UPDATE leads SET deleted_at = now(), updated_at = now() 
WHERE id = $1 AND deleted_at IS NULL;

-- name: BulkSoftDeleteLeads :execresult
UPDATE leads SET deleted_at = now(), updated_at = now() 
WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL;

-- name: CountLeads :one
SELECT COUNT(*) FROM leads WHERE deleted_at IS NULL;

-- Lead Activity Queries

-- name: CreateLeadActivity :exec
INSERT INTO lead_activity (lead_id, user_id, action, meta)
VALUES ($1, $2, $3, $4);

-- name: ListLeadActivities :many
SELECT * FROM lead_activity
WHERE lead_id = $1
ORDER BY created_at DESC;

-- Lead Notes Queries

-- name: CreateLeadNote :one
INSERT INTO lead_notes (lead_id, author_id, body, type)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetLeadNote :one
SELECT * FROM lead_notes WHERE id = $1;

-- name: UpdateLeadNote :one
UPDATE lead_notes SET body = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteLeadNote :exec
DELETE FROM lead_notes WHERE id = $1;

-- name: ListLeadNotes :many
SELECT * FROM lead_notes
WHERE lead_id = $1
ORDER BY created_at DESC;

-- Lead Services Queries

-- name: CreateLeadService :one
WITH inserted AS (
    INSERT INTO lead_services (lead_id, service_type_id, status)
    VALUES (
        $1,
        (SELECT id FROM service_types WHERE name = $2 OR slug = $2 LIMIT 1),
        'New'
    )
    RETURNING *
)
SELECT i.id, i.lead_id, st.name AS service_type, i.status,
    i.visit_scheduled_date, i.visit_scout_id, i.visit_measurements, i.visit_access_difficulty, i.visit_notes, i.visit_completed_at,
    i.created_at, i.updated_at
FROM inserted i
JOIN service_types st ON st.id = i.service_type_id;

-- name: GetLeadService :one
SELECT * FROM lead_services WHERE id = $1;

-- name: UpdateLeadServiceStatus :one
UPDATE lead_services SET status = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ListLeadServices :many
SELECT * FROM lead_services
WHERE lead_id = $1
ORDER BY created_at;

-- name: ScheduleLeadServiceVisit :one
UPDATE lead_services SET 
    visit_scheduled_date = $2, 
    visit_scout_id = $3,
    status = 'Scheduled',
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CompleteLeadServiceSurvey :one
UPDATE lead_services SET 
    visit_measurements = $2,
    visit_access_difficulty = $3,
    visit_notes = $4,
    visit_completed_at = now(),
    status = 'Surveyed',
    updated_at = now()
WHERE id = $1
RETURNING *;

-- Visit History Queries

-- name: CreateVisitHistory :one
INSERT INTO visit_history (lead_id, scheduled_date, scout_id, outcome, measurements, access_difficulty, notes, completed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListVisitHistory :many
SELECT * FROM visit_history
WHERE lead_id = $1
ORDER BY scheduled_date DESC;

-- Lead AI Analysis Queries

-- name: CreateLeadAIAnalysis :one
INSERT INTO lead_ai_analysis (lead_id, urgency_level, urgency_reason, talking_points, objection_handling, upsell_opportunities, summary)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetLatestLeadAIAnalysis :one
SELECT * FROM lead_ai_analysis
WHERE lead_id = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: ListLeadAIAnalysis :many
SELECT * FROM lead_ai_analysis
WHERE lead_id = $1
ORDER BY created_at DESC;
