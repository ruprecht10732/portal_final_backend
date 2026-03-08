-- Leads Domain SQL Queries
-- Add sqlc-annotated queries here as you migrate raw SQL from repository files.

-- name: GetLeadByID :one
SELECT * FROM rac_leads WHERE id = $1 AND organization_id = $2;

-- name: GetLeadByPublicToken :one
SELECT *
FROM RAC_leads
WHERE public_token = $1
	AND deleted_at IS NULL
	AND (public_token_expires_at IS NULL OR public_token_expires_at > now());

-- name: SetLeadPublicToken :exec
UPDATE RAC_leads
SET public_token = $3, public_token_expires_at = $4, updated_at = now()
WHERE id = $1 AND organization_id = $2;

-- name: CreateLeadService :one
WITH resolved_service_type AS (
	SELECT COALESCE(
		(SELECT st.id FROM RAC_service_types st WHERE (st.name = $3 OR st.slug = $3) AND st.organization_id = $2 LIMIT 1),
		(SELECT st.id FROM RAC_service_types st WHERE st.organization_id = $2 AND st.is_active = true ORDER BY st.name ASC LIMIT 1),
		(SELECT st.id FROM RAC_service_types st WHERE st.organization_id = $2 ORDER BY st.name ASC LIMIT 1)
	) AS id
), inserted AS (
	INSERT INTO RAC_lead_services (lead_id, organization_id, service_type_id, status, consumer_note, source)
	SELECT $1, $2, id, 'New', $4, $5
	FROM resolved_service_type
	WHERE id IS NOT NULL
	RETURNING *
), event AS (
	INSERT INTO RAC_lead_service_events (organization_id, lead_id, lead_service_id, event_type, status, pipeline_stage, occurred_at)
	SELECT i.organization_id, i.lead_id, i.id, 'service_created', i.status, i.pipeline_stage, i.created_at
	FROM inserted i
)
SELECT i.id, i.lead_id, i.organization_id, st.name AS service_type, i.status, i.pipeline_stage, i.consumer_note, i.source,
	i.customer_preferences, i.gatekeeper_nurturing_loop_count, i.gatekeeper_nurturing_loop_fingerprint,
	i.created_at, i.updated_at
FROM inserted i
JOIN RAC_service_types st ON st.id = i.service_type_id AND st.organization_id = i.organization_id;

-- name: GetLeadServiceByID :one
SELECT ls.id, ls.lead_id, ls.organization_id, st.name AS service_type, ls.status, ls.pipeline_stage, ls.consumer_note, ls.source,
	ls.customer_preferences, ls.gatekeeper_nurturing_loop_count, ls.gatekeeper_nurturing_loop_fingerprint,
	ls.created_at, ls.updated_at
FROM RAC_lead_services ls
JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
WHERE ls.id = $1 AND ls.organization_id = $2;

-- name: ListLeadServices :many
SELECT ls.id, ls.lead_id, ls.organization_id, st.name AS service_type, ls.status, ls.pipeline_stage, ls.consumer_note, ls.source,
	ls.customer_preferences, ls.gatekeeper_nurturing_loop_count, ls.gatekeeper_nurturing_loop_fingerprint,
	ls.created_at, ls.updated_at
FROM RAC_lead_services ls
JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
WHERE ls.lead_id = $1 AND ls.organization_id = $2
ORDER BY ls.created_at DESC;

-- name: GetCurrentActiveLeadService :one
SELECT ls.id, ls.lead_id, ls.organization_id, st.name AS service_type, ls.status, ls.pipeline_stage, ls.consumer_note, ls.source,
	ls.customer_preferences, ls.gatekeeper_nurturing_loop_count, ls.gatekeeper_nurturing_loop_fingerprint,
	ls.created_at, ls.updated_at
FROM RAC_lead_services ls
JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
WHERE ls.lead_id = $1 AND ls.organization_id = $2 AND ls.pipeline_stage NOT IN ('Completed', 'Lost')
ORDER BY ls.created_at DESC
LIMIT 1;

-- name: GetLatestLeadService :one
SELECT ls.id, ls.lead_id, ls.organization_id, st.name AS service_type, ls.status, ls.pipeline_stage, ls.consumer_note, ls.source,
	ls.customer_preferences, ls.gatekeeper_nurturing_loop_count, ls.gatekeeper_nurturing_loop_fingerprint,
	ls.created_at, ls.updated_at
FROM RAC_lead_services ls
JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
WHERE ls.lead_id = $1 AND ls.organization_id = $2
ORDER BY ls.created_at DESC
LIMIT 1;

-- name: UpdateServiceStatusAndPipelineStage :one
WITH current AS (
	SELECT status AS old_status, pipeline_stage AS old_stage
	FROM RAC_lead_services ls
	WHERE ls.id = $1 AND ls.organization_id = $2
), updated AS (
	UPDATE RAC_lead_services ls
	SET status = $3, pipeline_stage = $4, updated_at = now()
	WHERE ls.id = $1 AND ls.organization_id = $2
		AND (status IS DISTINCT FROM $3 OR pipeline_stage IS DISTINCT FROM $4)
	RETURNING *
), selected AS (
	SELECT updated.* FROM updated
	UNION ALL
	SELECT ls.*
	FROM RAC_lead_services ls
	WHERE ls.id = $1 AND ls.organization_id = $2 AND NOT EXISTS (SELECT 1 FROM updated)
), status_event AS (
	INSERT INTO RAC_lead_service_events (organization_id, lead_id, lead_service_id, event_type, status, pipeline_stage, occurred_at)
	SELECT u.organization_id, u.lead_id, u.id, 'status_changed', u.status, u.pipeline_stage, u.updated_at
	FROM updated u
	WHERE (SELECT old_status FROM current) IS DISTINCT FROM $3
), stage_event AS (
	INSERT INTO RAC_lead_service_events (organization_id, lead_id, lead_service_id, event_type, status, pipeline_stage, occurred_at)
	SELECT u.organization_id, u.lead_id, u.id, 'pipeline_stage_changed', u.status, u.pipeline_stage, u.updated_at
	FROM updated u
	WHERE (SELECT old_stage FROM current) IS DISTINCT FROM $4
)
SELECT u.id, u.lead_id, u.organization_id, st.name AS service_type, u.status, u.pipeline_stage, u.consumer_note, u.source,
	u.customer_preferences, u.gatekeeper_nurturing_loop_count, u.gatekeeper_nurturing_loop_fingerprint,
	u.created_at, u.updated_at
FROM selected u
JOIN RAC_service_types st ON st.id = u.service_type_id AND st.organization_id = u.organization_id;

-- name: UpdateLeadServiceType :one
WITH target AS (
	SELECT st.id FROM RAC_service_types st
	WHERE (st.name = $3 OR st.slug = $3)
		AND st.organization_id = $2
		AND st.is_active = true
	LIMIT 1
), updated AS (
	UPDATE RAC_lead_services ls
	SET service_type_id = (SELECT target.id FROM target), updated_at = now()
	WHERE ls.id = $1 AND ls.organization_id = $2 AND EXISTS (SELECT 1 FROM target)
	RETURNING *
)
SELECT u.id, u.lead_id, u.organization_id, st.name AS service_type, u.status, u.pipeline_stage, u.consumer_note, u.source,
	u.customer_preferences, u.gatekeeper_nurturing_loop_count, u.gatekeeper_nurturing_loop_fingerprint,
	u.created_at, u.updated_at
FROM updated u
JOIN RAC_service_types st ON st.id = u.service_type_id AND st.organization_id = u.organization_id;

-- name: UpdateServiceStatus :one
WITH updated AS (
	UPDATE RAC_lead_services ls SET status = $3, updated_at = now()
	WHERE ls.id = $1 AND ls.organization_id = $2 AND ls.status IS DISTINCT FROM $3
	RETURNING *
), selected AS (
	SELECT updated.* FROM updated
	UNION ALL
	SELECT ls.*
	FROM RAC_lead_services ls
	WHERE ls.id = $1 AND ls.organization_id = $2 AND NOT EXISTS (SELECT 1 FROM updated)
), event AS (
	INSERT INTO RAC_lead_service_events (organization_id, lead_id, lead_service_id, event_type, status, pipeline_stage, occurred_at)
	SELECT u.organization_id, u.lead_id, u.id, 'status_changed', u.status, u.pipeline_stage, u.updated_at
	FROM updated u
)
SELECT u.id, u.lead_id, u.organization_id, st.name AS service_type, u.status, u.pipeline_stage, u.consumer_note, u.source,
	u.customer_preferences, u.gatekeeper_nurturing_loop_count, u.gatekeeper_nurturing_loop_fingerprint,
	u.created_at, u.updated_at
FROM selected u
JOIN RAC_service_types st ON st.id = u.service_type_id AND st.organization_id = u.organization_id;

-- name: UpdatePipelineStage :one
WITH updated AS (
	UPDATE RAC_lead_services ls SET pipeline_stage = $3, updated_at = now()
	WHERE ls.id = $1 AND ls.organization_id = $2 AND ls.pipeline_stage IS DISTINCT FROM $3
	RETURNING *
), selected AS (
	SELECT updated.* FROM updated
	UNION ALL
	SELECT ls.*
	FROM RAC_lead_services ls
	WHERE ls.id = $1 AND ls.organization_id = $2 AND NOT EXISTS (SELECT 1 FROM updated)
), event AS (
	INSERT INTO RAC_lead_service_events (organization_id, lead_id, lead_service_id, event_type, status, pipeline_stage, occurred_at)
	SELECT u.organization_id, u.lead_id, u.id, 'pipeline_stage_changed', u.status, u.pipeline_stage, u.updated_at
	FROM updated u
)
SELECT u.id, u.lead_id, u.organization_id, st.name AS service_type, u.status, u.pipeline_stage, u.consumer_note, u.source,
	u.customer_preferences, u.gatekeeper_nurturing_loop_count, u.gatekeeper_nurturing_loop_fingerprint,
	u.created_at, u.updated_at
FROM selected u
JOIN RAC_service_types st ON st.id = u.service_type_id AND st.organization_id = u.organization_id;

-- name: SetGatekeeperNurturingLoopState :exec
UPDATE RAC_lead_services
SET gatekeeper_nurturing_loop_count = $3,
	gatekeeper_nurturing_loop_fingerprint = $4,
	updated_at = now()
WHERE id = $1 AND organization_id = $2;

-- name: ResetGatekeeperNurturingLoopState :exec
UPDATE RAC_lead_services
SET gatekeeper_nurturing_loop_count = 0,
	gatekeeper_nurturing_loop_fingerprint = NULL,
	updated_at = now()
WHERE id = $1 AND organization_id = $2
	AND (gatekeeper_nurturing_loop_count <> 0 OR gatekeeper_nurturing_loop_fingerprint IS NOT NULL);

-- name: CloseAllActiveServices :exec
WITH updated AS (
	UPDATE RAC_lead_services ls
	SET pipeline_stage = 'Completed', updated_at = now()
	WHERE ls.lead_id = $1 AND ls.organization_id = $2 AND ls.pipeline_stage NOT IN ('Completed', 'Lost')
	RETURNING id, lead_id, organization_id, status, pipeline_stage, updated_at
)
INSERT INTO RAC_lead_service_events (organization_id, lead_id, lead_service_id, event_type, status, pipeline_stage, occurred_at)
SELECT u.organization_id, u.lead_id, u.id, 'pipeline_stage_changed', u.status, u.pipeline_stage, u.updated_at
FROM updated u;

-- name: UpdateServicePreferences :exec
UPDATE RAC_lead_services
SET customer_preferences = $3, updated_at = now()
WHERE id = $1 AND organization_id = $2;

-- name: InsertLeadServiceEvent :exec
INSERT INTO RAC_lead_service_events (
	organization_id, lead_id, lead_service_id,
	event_type, status, pipeline_stage, occurred_at
)
SELECT $1, $2, $3, $4, $5, $6, $7
WHERE NOT EXISTS (
	SELECT 1 FROM RAC_lead_service_events
	WHERE lead_service_id = $3 AND event_type = $4
);

-- name: CreateLeadNote :one
WITH inserted AS (
	INSERT INTO RAC_lead_notes (lead_id, organization_id, author_id, type, body, service_id)
	VALUES ($1, $2, $3, $4, $5, $6)
	RETURNING id, lead_id, organization_id, author_id, type, body, service_id, created_at, updated_at
)
SELECT inserted.id, inserted.lead_id, inserted.organization_id, inserted.author_id, u.email, inserted.type, inserted.body, inserted.service_id, inserted.created_at, inserted.updated_at
FROM inserted
JOIN RAC_users u ON u.id = inserted.author_id;

-- name: ListLeadNotes :many
SELECT ln.id, ln.lead_id, ln.organization_id, ln.author_id, u.email, ln.type, ln.body, ln.service_id, ln.created_at, ln.updated_at
FROM RAC_lead_notes ln
JOIN RAC_users u ON u.id = ln.author_id
WHERE ln.lead_id = $1 AND ln.organization_id = $2
ORDER BY ln.created_at DESC;

-- name: ListNotesByService :many
SELECT ln.id, ln.lead_id, ln.organization_id, ln.author_id, u.email, ln.type, ln.body, ln.service_id, ln.created_at, ln.updated_at
FROM RAC_lead_notes ln
JOIN RAC_users u ON u.id = ln.author_id
WHERE ln.lead_id = $1 AND ln.organization_id = $2 AND (ln.service_id = $3 OR ln.service_id IS NULL)
ORDER BY ln.created_at DESC;

-- name: CreateAttachment :one
INSERT INTO RAC_lead_service_attachments (lead_service_id, organization_id, file_key, file_name, content_type, size_bytes, uploaded_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, lead_service_id, organization_id, file_key, file_name, content_type, size_bytes, uploaded_by, created_at;

-- name: GetAttachmentByID :one
SELECT id, lead_service_id, organization_id, file_key, file_name, content_type, size_bytes, uploaded_by, created_at
FROM RAC_lead_service_attachments
WHERE id = $1 AND organization_id = $2;

-- name: ListAttachmentsByService :many
SELECT id, lead_service_id, organization_id, file_key, file_name, content_type, size_bytes, uploaded_by, created_at
FROM RAC_lead_service_attachments
WHERE lead_service_id = $1 AND organization_id = $2
ORDER BY created_at DESC;

-- name: DeleteAttachment :execrows
DELETE FROM RAC_lead_service_attachments
WHERE id = $1 AND organization_id = $2;

-- name: CreateAIAnalysis :one
INSERT INTO RAC_lead_ai_analysis (
	lead_id, organization_id, lead_service_id, urgency_level, urgency_reason,
	lead_quality, recommended_action, missing_information,
	preferred_contact_channel, suggested_contact_message, summary,
	composite_confidence, confidence_breakdown, risk_flags
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING id, lead_id, organization_id, lead_service_id, urgency_level, urgency_reason,
	lead_quality, recommended_action, missing_information,
	preferred_contact_channel, suggested_contact_message, summary,
	composite_confidence, confidence_breakdown, risk_flags, created_at;

-- name: GetLatestAIAnalysis :one
SELECT id, lead_id, organization_id, lead_service_id, urgency_level, urgency_reason,
	lead_quality, recommended_action, missing_information,
	preferred_contact_channel, suggested_contact_message, summary,
	composite_confidence, confidence_breakdown, risk_flags, created_at
FROM RAC_lead_ai_analysis
WHERE lead_service_id = $1 AND organization_id = $2
ORDER BY created_at DESC
LIMIT 1;

-- name: ListAIAnalyses :many
SELECT id, lead_id, organization_id, lead_service_id, urgency_level, urgency_reason,
	lead_quality, recommended_action, missing_information,
	preferred_contact_channel, suggested_contact_message, summary,
	composite_confidence, confidence_breakdown, risk_flags, created_at
FROM RAC_lead_ai_analysis
WHERE lead_service_id = $1 AND organization_id = $2
ORDER BY created_at DESC;

-- name: CreatePhotoAnalysis :one
INSERT INTO RAC_lead_photo_analyses (
	lead_id, service_id, org_id, summary, observations, scope_assessment, cost_indicators,
	safety_concerns, additional_info, confidence_level, photo_count,
	measurements, needs_onsite_measurement, discrepancies, extracted_text, suggested_search_terms
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING id, lead_id, service_id, org_id, summary, observations, scope_assessment, cost_indicators,
	safety_concerns, additional_info, confidence_level, photo_count,
	measurements, needs_onsite_measurement, discrepancies, extracted_text, suggested_search_terms,
	created_at, updated_at;

-- name: GetPhotoAnalysisByID :one
SELECT id, lead_id, service_id, org_id, summary, observations, scope_assessment, cost_indicators,
	safety_concerns, additional_info, confidence_level, photo_count,
	measurements, needs_onsite_measurement, discrepancies, extracted_text, suggested_search_terms,
	created_at, updated_at
FROM RAC_lead_photo_analyses
WHERE id = $1 AND org_id = $2;

-- name: GetLatestPhotoAnalysis :one
SELECT id, lead_id, service_id, org_id, summary, observations, scope_assessment, cost_indicators,
	safety_concerns, additional_info, confidence_level, photo_count,
	measurements, needs_onsite_measurement, discrepancies, extracted_text, suggested_search_terms,
	created_at, updated_at
FROM RAC_lead_photo_analyses
WHERE service_id = $1 AND org_id = $2
ORDER BY created_at DESC
LIMIT 1;

-- name: ListPhotoAnalysesByService :many
SELECT id, lead_id, service_id, org_id, summary, observations, scope_assessment, cost_indicators,
	safety_concerns, additional_info, confidence_level, photo_count,
	measurements, needs_onsite_measurement, discrepancies, extracted_text, suggested_search_terms,
	created_at, updated_at
FROM RAC_lead_photo_analyses
WHERE service_id = $1 AND org_id = $2
ORDER BY created_at DESC;

-- name: ListPhotoAnalysesByLead :many
SELECT id, lead_id, service_id, org_id, summary, observations, scope_assessment, cost_indicators,
	safety_concerns, additional_info, confidence_level, photo_count,
	measurements, needs_onsite_measurement, discrepancies, extracted_text, suggested_search_terms,
	created_at, updated_at
FROM RAC_lead_photo_analyses
WHERE lead_id = $1 AND org_id = $2
ORDER BY created_at DESC;

-- name: GetLeadAppointmentStats :one
SELECT
	COUNT(*) AS total,
	COUNT(*) FILTER (WHERE status = 'Scheduled') AS scheduled,
	COUNT(*) FILTER (WHERE status = 'Completed') AS completed,
	COUNT(*) FILTER (WHERE status = 'Cancelled') AS cancelled,
	EXISTS(
		SELECT 1 FROM RAC_appointments upcoming
		WHERE upcoming.lead_id = $1 AND upcoming.organization_id = $2
			AND status = 'Scheduled' AND start_time > NOW()
	) AS has_upcoming
FROM RAC_appointments a
WHERE a.lead_id = $1 AND a.organization_id = $2;

-- name: GetLatestAppointment :one
SELECT start_time, status
FROM RAC_appointments
WHERE lead_id = $1 AND organization_id = $2
ORDER BY start_time DESC
LIMIT 1;

-- name: GetServiceStateAggregates :one
WITH quote_stats AS (
	SELECT
		COUNT(*) FILTER (WHERE status = 'Accepted') AS accepted,
		COUNT(*) FILTER (WHERE status = 'Sent') AS sent,
		COUNT(*) FILTER (WHERE status = 'Draft') AS draft,
		COUNT(*) FILTER (WHERE status = 'Rejected') AS rejected,
		MAX(updated_at) AS latest_at
	FROM rac_quotes
		WHERE rac_quotes.lead_service_id = $1 AND rac_quotes.organization_id = $2
),
offer_counts AS (
	SELECT
		COUNT(*) FILTER (WHERE status = 'accepted') AS accepted,
		COUNT(*) FILTER (WHERE status IN ('pending', 'sent')) AS pending,
		MAX(
			GREATEST(
				created_at,
				updated_at,
				COALESCE(accepted_at, created_at),
				COALESCE(rejected_at, created_at)
			)
		) AS latest_at
	FROM rac_partner_offers
		WHERE rac_partner_offers.lead_service_id = $1 AND rac_partner_offers.organization_id = $2
),
appt_counts AS (
	SELECT
		COUNT(*) FILTER (WHERE status IN ('scheduled', 'requested') AND start_time > NOW()) AS scheduled,
		COUNT(*) FILTER (WHERE status = 'completed') AS completed,
		COUNT(*) FILTER (WHERE status IN ('cancelled', 'no_show')) AS cancelled,
		MAX(updated_at) AS latest_at
	FROM rac_appointments
	WHERE lead_service_id = $1 AND organization_id = $2
),
report_check AS (
	SELECT EXISTS(
		SELECT 1
		FROM rac_appointment_visit_reports avr
		JOIN rac_appointments a ON a.id = avr.appointment_id
		WHERE a.lead_service_id = $1 AND a.organization_id = $2 AND avr.organization_id = $2
	) AS has_report
),
ai_data AS (
	SELECT recommended_action
	FROM rac_lead_ai_analysis
	WHERE lead_service_id = $1 AND organization_id = $2
	ORDER BY created_at DESC
	LIMIT 1
),
terminal_event AS (
	SELECT MAX(occurred_at) AS terminal_at
	FROM rac_lead_service_events
	WHERE lead_service_id = $1 AND organization_id = $2
		AND pipeline_stage IN ('Completed', 'Lost')
)
SELECT
	COALESCE(q.accepted, 0),
	COALESCE(q.sent, 0),
	COALESCE(q.draft, 0),
	COALESCE(q.rejected, 0),
	q.latest_at,
	COALESCE(o.accepted, 0),
	COALESCE(o.pending, 0),
	o.latest_at,
	COALESCE(ap.scheduled, 0),
	COALESCE(ap.completed, 0),
	COALESCE(ap.cancelled, 0),
	ap.latest_at,
	COALESCE(rc.has_report, false),
	a.recommended_action,
	t.terminal_at
FROM quote_stats q
CROSS JOIN offer_counts o
CROSS JOIN appt_counts ap
CROSS JOIN report_check rc
LEFT JOIN ai_data a ON true
CROSS JOIN terminal_event t;

-- name: GetAppointmentVisitReport :one
SELECT appointment_id, organization_id, measurements, access_difficulty, notes, created_at, updated_at
FROM RAC_appointment_visit_reports
WHERE appointment_id = $1 AND organization_id = $2;

-- name: CreateTimelineEvent :one
INSERT INTO lead_timeline_events (
	lead_id,
	service_id,
	organization_id,
	actor_type,
	actor_name,
	event_type,
	title,
	summary,
	metadata,
	visibility
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, lead_id, service_id, organization_id, actor_type, actor_name, event_type, title, summary, metadata, visibility, created_at;

-- name: FindRecentDuplicateTimelineEvent :one
SELECT id, lead_id, service_id, organization_id, actor_type, actor_name, event_type, title, summary, metadata, visibility, created_at
FROM lead_timeline_events
WHERE lead_id = $1
	AND organization_id = $2
	AND (service_id = $3 OR (service_id IS NULL AND $3 IS NULL))
	AND actor_type = $4
	AND actor_name = $5
	AND event_type = $6
	AND title = $7
	AND (($8 = '' AND summary IS NULL) OR ($8 <> '' AND summary = $8))
	AND visibility = $9
	AND created_at >= now() - make_interval(secs => $10)
	AND (
		$6 <> $11 OR (
			COALESCE(metadata->>'oldStage', '') = CAST($12 AS text)
			AND COALESCE(metadata->>'newStage', '') = CAST($13 AS text)
		)
	)
ORDER BY created_at DESC
LIMIT 1;

-- name: ListTimelineEvents :many
SELECT id, lead_id, service_id, organization_id, actor_type, actor_name, event_type, title, summary, metadata, visibility, created_at
FROM lead_timeline_events
WHERE lead_id = $1 AND organization_id = $2
ORDER BY created_at DESC;

-- name: ListTimelineEventsByService :many
SELECT id, lead_id, service_id, organization_id, actor_type, actor_name, event_type, title, summary, metadata, visibility, created_at
FROM lead_timeline_events
WHERE lead_id = $1 AND organization_id = $2 AND service_id = $3
ORDER BY created_at DESC;

-- name: DeleteFeedReaction :execrows
DELETE FROM RAC_feed_reactions
WHERE event_id = $1
	AND event_source = $2
	AND reaction_type = $3
	AND user_id = $4
	AND org_id = $5;

-- name: CreateFeedReaction :exec
INSERT INTO RAC_feed_reactions (event_id, event_source, reaction_type, user_id, org_id)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT DO NOTHING;

-- name: ListReactionsByEvent :many
SELECT fr.id, fr.event_id, fr.event_source, fr.reaction_type, fr.user_id, u.email, fr.created_at
FROM RAC_feed_reactions fr
JOIN RAC_users u ON u.id = fr.user_id
WHERE fr.event_id = $1
	AND fr.event_source = $2
	AND fr.org_id = $3
ORDER BY fr.created_at;

-- name: ListReactionsByEvents :many
SELECT fr.id, fr.event_id, fr.event_source, fr.reaction_type, fr.user_id, u.email, fr.created_at
FROM RAC_feed_reactions fr
JOIN RAC_users u ON u.id = fr.user_id
WHERE fr.event_id = ANY($1::text[])
	AND fr.org_id = $2
ORDER BY fr.event_id, fr.created_at;

-- name: CreateFeedComment :one
INSERT INTO RAC_feed_comments (event_id, event_source, user_id, org_id, body)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, event_id, event_source, user_id, org_id, body, created_at, updated_at;

-- name: CreateFeedCommentMention :exec
INSERT INTO RAC_feed_comment_mentions (comment_id, mentioned_user_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: ListCommentsByEvent :many
SELECT c.id, c.event_id, c.event_source, c.user_id, u.email, c.body, c.created_at, c.updated_at
FROM RAC_feed_comments c
JOIN RAC_users u ON u.id = c.user_id
WHERE c.event_id = $1
	AND c.event_source = $2
	AND c.org_id = $3
ORDER BY c.created_at ASC;

-- name: DeleteFeedComment :execrows
DELETE FROM RAC_feed_comments
WHERE id = $1
	AND user_id = $2
	AND org_id = $3;

-- name: ListCommentCountsByEvents :many
SELECT event_id, COUNT(*)::int AS comment_count
FROM RAC_feed_comments
WHERE event_id = ANY($1::text[])
	AND org_id = $2
GROUP BY event_id;

-- name: ListMentionsByComments :many
SELECT m.comment_id, m.mentioned_user_id, u.email
FROM RAC_feed_comment_mentions m
JOIN RAC_users u ON u.id = m.mentioned_user_id
WHERE m.comment_id = ANY(sqlc.arg(commentIDs)::uuid[])
ORDER BY m.created_at;

-- name: ListLeadOrgMembers :many
SELECT
	u.id,
	u.email,
	COALESCE(array_agg(r.name) FILTER (WHERE r.name IS NOT NULL), '{}') AS roles
FROM RAC_organization_members om
JOIN RAC_users u ON u.id = om.user_id
LEFT JOIN RAC_user_roles ur ON ur.user_id = u.id
LEFT JOIN RAC_roles r ON r.id = ur.role_id
WHERE om.organization_id = $1
GROUP BY u.id, u.email
ORDER BY u.email;

-- name: CreateHumanFeedback :one
INSERT INTO RAC_human_feedback (
	organization_id, quote_id, lead_service_id,
	field_changed, ai_value, human_value, delta_percentage
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, organization_id, quote_id, lead_service_id,
	field_changed, ai_value, human_value, delta_percentage,
	context_embedding_id, applied_to_memory, created_at;

-- name: ListRecentAppliedHumanFeedbackByServiceType :many
SELECT hf.id, hf.organization_id, hf.quote_id, hf.lead_service_id,
	hf.field_changed, hf.ai_value, hf.human_value, hf.delta_percentage,
	hf.context_embedding_id, hf.applied_to_memory, hf.created_at
FROM RAC_human_feedback hf
JOIN RAC_lead_services ls
	ON ls.id = hf.lead_service_id
	AND ls.organization_id = hf.organization_id
JOIN RAC_service_types st
	ON st.id = ls.service_type_id
	AND st.organization_id = ls.organization_id
WHERE hf.organization_id = $1
	AND hf.applied_to_memory = true
	AND st.name = $2
ORDER BY hf.created_at DESC
LIMIT $3;

-- name: GetHumanFeedbackByID :one
SELECT id, organization_id, quote_id, lead_service_id,
	field_changed, ai_value, human_value, delta_percentage,
	context_embedding_id, applied_to_memory, created_at
FROM RAC_human_feedback
WHERE id = $1 AND organization_id = $2;

-- name: MarkHumanFeedbackApplied :one
UPDATE RAC_human_feedback
SET applied_to_memory = true,
	context_embedding_id = COALESCE($3, context_embedding_id)
WHERE id = $1 AND organization_id = $2
RETURNING id, organization_id, quote_id, lead_service_id,
	field_changed, ai_value, human_value, delta_percentage,
	context_embedding_id, applied_to_memory, created_at;

-- name: GetLeadMetricsSummary :one
SELECT
	(
		SELECT COUNT(DISTINCT l.id)
		FROM RAC_leads l
		JOIN RAC_lead_services ls ON ls.lead_id = l.id
		WHERE l.organization_id = $1 AND l.deleted_at IS NULL
			AND ls.pipeline_stage NOT IN ('Completed', 'Lost')
			AND ls.status != 'Disqualified'
	)::int AS active_leads,
	(
		SELECT COUNT(*)
		FROM RAC_quotes q
		WHERE q.organization_id = $1
			AND q.status::text IN ('Accepted', 'Quote_Accepted')
	)::int AS accepted_quotes,
	(
		SELECT COUNT(*)
		FROM RAC_quotes q
		WHERE q.organization_id = $1
			AND q.status::text IN ('Sent', 'Quote_Sent')
	)::int AS sent_quotes,
	(
		SELECT COALESCE(SUM(q.total_cents), 0)
		FROM RAC_quotes q
		WHERE q.organization_id = $1
			AND q.status::text IN ('Sent', 'Accepted', 'Quote_Sent', 'Quote_Accepted')
	)::bigint AS quote_pipeline_cents,
	(
		SELECT COALESCE(AVG(q.total_cents)::bigint, 0)
		FROM RAC_quotes q
		WHERE q.organization_id = $1
			AND q.status::text IN ('Sent', 'Accepted', 'Quote_Sent', 'Quote_Accepted')
	)::bigint AS avg_quote_value_cents;

-- name: ListActiveLeadsTrend :many
WITH weeks AS (
	SELECT generate_series(
		date_trunc('week', NOW()) - (($2 - 1) * INTERVAL '1 week'),
		date_trunc('week', NOW()),
		INTERVAL '1 week'
	) AS week_start
)
SELECT COALESCE(COUNT(DISTINCT CASE WHEN ls.pipeline_stage NOT IN ('Completed', 'Lost') AND ls.status != 'Disqualified' THEN l.id END), 0)::int AS activeLeadsCount
FROM weeks w
LEFT JOIN RAC_leads l
	ON l.organization_id = $1
	AND l.deleted_at IS NULL
	AND l.created_at >= w.week_start
	AND l.created_at < w.week_start + INTERVAL '1 week'
LEFT JOIN RAC_lead_services ls ON ls.lead_id = l.id
GROUP BY w.week_start
ORDER BY w.week_start;

-- name: ListQuoteOutcomeTrend :many
WITH weeks AS (
	SELECT generate_series(
		date_trunc('week', NOW()) - (($2 - 1) * INTERVAL '1 week'),
		date_trunc('week', NOW()),
		INTERVAL '1 week'
	) AS week_start
)
SELECT
	COALESCE(COUNT(*) FILTER (WHERE q.status::text IN ('Accepted', 'Quote_Accepted')), 0)::int AS accepted_quotes,
	COALESCE(COUNT(*) FILTER (WHERE q.status::text IN ('Sent', 'Quote_Sent')), 0)::int AS sent_quotes
FROM weeks w
LEFT JOIN RAC_quotes q
	ON q.organization_id = $1
	AND q.created_at >= w.week_start
	AND q.created_at < w.week_start + INTERVAL '1 week'
GROUP BY w.week_start
ORDER BY w.week_start;

-- name: ListQuotePipelineTrend :many
WITH weeks AS (
	SELECT generate_series(
		date_trunc('week', NOW()) - (($2 - 1) * INTERVAL '1 week'),
		date_trunc('week', NOW()),
		INTERVAL '1 week'
	) AS week_start
)
SELECT COALESCE(SUM(CASE WHEN q.status::text IN ('Sent', 'Accepted', 'Quote_Sent', 'Quote_Accepted') THEN q.total_cents ELSE 0 END), 0)::bigint AS quotePipelineCentsValue
FROM weeks w
LEFT JOIN RAC_quotes q
	ON q.organization_id = $1
	AND q.created_at >= w.week_start
	AND q.created_at < w.week_start + INTERVAL '1 week'
GROUP BY w.week_start
ORDER BY w.week_start;

-- name: ListAvgQuoteValueTrend :many
WITH weeks AS (
	SELECT generate_series(
		date_trunc('week', NOW()) - (($2 - 1) * INTERVAL '1 week'),
		date_trunc('week', NOW()),
		INTERVAL '1 week'
	) AS week_start
)
SELECT COALESCE(AVG(CASE WHEN q.status::text IN ('Sent', 'Accepted', 'Quote_Sent', 'Quote_Accepted') THEN q.total_cents END)::bigint, 0)::bigint AS avgQuoteValueCentsValue
FROM weeks w
LEFT JOIN RAC_quotes q
	ON q.organization_id = $1
	AND q.created_at >= w.week_start
	AND q.created_at < w.week_start + INTERVAL '1 week'
GROUP BY w.week_start
ORDER BY w.week_start;

-- name: FindMatchingPartnersByCoordinates :many
SELECT p.id, p.business_name, p.contact_email,
	CAST(earth_distance(ll_to_earth($2, $1), ll_to_earth(p.latitude, p.longitude)) / 1000.0 AS double precision) AS dist_km
FROM RAC_partners p
JOIN RAC_partner_service_types pst ON pst.partner_id = p.id
JOIN RAC_service_types st ON st.id = pst.service_type_id AND st.organization_id = p.organization_id
WHERE p.organization_id = $3
	AND st.is_active = true
	AND (st.name = $4 OR st.slug = $4)
	AND p.latitude IS NOT NULL AND p.longitude IS NOT NULL
	AND earth_distance(ll_to_earth($2, $1), ll_to_earth(p.latitude, p.longitude)) <= ($5 * 1000.0)
	AND (CARDINALITY($6::uuid[]) = 0 OR p.id != ALL($6::uuid[]))
ORDER BY dist_km ASC
LIMIT 5;

-- name: GetPartnerOfferStatsSince :many
SELECT partner_id,
	COUNT(*) FILTER (WHERE status = 'rejected')::int AS rejected_count,
	COUNT(*) FILTER (WHERE status = 'accepted')::int AS accepted_count,
	COUNT(*) FILTER (WHERE status IN ('pending', 'sent'))::int AS open_count
FROM RAC_partner_offers
WHERE organization_id = $1
	AND partner_id = ANY($2::uuid[])
	AND created_at >= $3
GROUP BY partner_id;

-- name: GetLeadCity :one
SELECT address_city AS cityValue
FROM RAC_leads
WHERE organization_id = $1
	AND id = $2
LIMIT 1;

-- name: FindPartnersByServiceTypeAndCity :many
SELECT p.id, p.business_name, p.contact_email,
	0.0::double precision AS dist_km
FROM RAC_partners p
JOIN RAC_partner_service_types pst ON pst.partner_id = p.id
JOIN RAC_service_types st ON st.id = pst.service_type_id AND st.organization_id = p.organization_id
WHERE p.organization_id = $1
	AND st.is_active = true
	AND (st.name = $2 OR st.slug = $2)
	AND lower(p.city) = lower($3)
	AND (CARDINALITY($4::uuid[]) = 0 OR p.id != ALL($4::uuid[]))
ORDER BY p.updated_at DESC
LIMIT 5;

-- name: FindPartnersByServiceType :many
SELECT p.id, p.business_name, p.contact_email,
	0.0::double precision AS dist_km
FROM RAC_partners p
JOIN RAC_partner_service_types pst ON pst.partner_id = p.id
JOIN RAC_service_types st ON st.id = pst.service_type_id AND st.organization_id = p.organization_id
WHERE p.organization_id = $1
	AND st.is_active = true
	AND (st.name = $2 OR st.slug = $2)
	AND (CARDINALITY($3::uuid[]) = 0 OR p.id != ALL($3::uuid[]))
ORDER BY p.updated_at DESC
LIMIT 5;

-- name: GetLeadCoordinates :one
SELECT latitude, longitude
FROM RAC_leads
WHERE organization_id = $1
	AND id = $2
	AND latitude IS NOT NULL
	AND longitude IS NOT NULL
LIMIT 1;

-- name: ListInvitedPartnerIDs :many
SELECT partner_id AS partnerIDValue
FROM RAC_partner_offers
WHERE lead_service_id = $1
	AND status IN ('rejected', 'expired', 'sent', 'pending');

-- name: HasLinkedPartners :one
SELECT EXISTS(
	SELECT 1
	FROM RAC_partner_leads
	WHERE organization_id = $1
		AND lead_id = $2
);

-- name: GetZipCoordinates :one
SELECT latitude, longitude
FROM RAC_leads
WHERE organization_id = $1
	AND address_zip_code = $2
	AND latitude IS NOT NULL
	AND longitude IS NOT NULL
ORDER BY created_at DESC
LIMIT 1;

-- name: ListActiveServiceTypes :many
SELECT name, description, intake_guidelines, estimation_guidelines
FROM RAC_service_types
WHERE organization_id = $1 AND is_active = true
ORDER BY name ASC;

-- name: CreateLead :one
INSERT INTO RAC_leads (
	organization_id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
	address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
	assigned_agent_id, source,
	gclid, utm_source, utm_medium, utm_campaign, utm_content, utm_term, ad_landing_page, referrer_url,
	whatsapp_opted_in
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
RETURNING *;

-- name: GetLeadByPhone :one
SELECT *
FROM RAC_leads
WHERE consumer_phone = $1 AND organization_id = $2 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT 1;

-- name: GetLeadWhatsAppOptIn :one
SELECT whatsapp_opted_in AS whatsappOptInValue
FROM RAC_leads
WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL;

-- name: GetLeadSummaryByPhoneOrEmail :one
SELECT
	l.id,
	l.organization_id,
	l.consumer_first_name || ' ' || l.consumer_last_name AS consumer_name,
	l.consumer_phone,
	l.consumer_email,
	l.address_city,
	COUNT(ls.id)::int AS service_count,
	COALESCE((
		SELECT st.name
		FROM RAC_lead_services ls2
		JOIN RAC_service_types st ON st.id = ls2.service_type_id AND st.organization_id = l.organization_id
		WHERE ls2.lead_id = l.id
		ORDER BY ls2.created_at DESC
		LIMIT 1
	), '') AS last_service_type,
	COALESCE((
		SELECT ls2.status
		FROM RAC_lead_services ls2
		WHERE ls2.lead_id = l.id
		ORDER BY ls2.created_at DESC
		LIMIT 1
	), '') AS last_status,
	l.created_at
FROM RAC_leads l
LEFT JOIN RAC_lead_services ls ON ls.lead_id = l.id
WHERE l.deleted_at IS NULL
	AND l.organization_id = $3
	AND (($1 != '' AND l.consumer_phone = $1) OR ($2 != '' AND l.consumer_email = $2))
GROUP BY l.id
ORDER BY l.created_at DESC
LIMIT 1;

-- name: GetLatestAcceptedQuoteIDForService :one
SELECT id
FROM RAC_quotes
WHERE lead_service_id = $1
	AND organization_id = $2
	AND status = 'Accepted'
	AND total_cents > 0
ORDER BY created_at DESC
LIMIT 1;

-- name: HasNonDraftQuote :one
SELECT EXISTS (
	SELECT 1
	FROM RAC_quotes
	WHERE lead_service_id = $1
		AND organization_id = $2
		AND status <> 'Draft'
);

-- name: GetLatestDraftQuoteID :one
SELECT id
FROM RAC_quotes
WHERE lead_service_id = $1
	AND organization_id = $2
	AND status = 'Draft'
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateLead :one
UPDATE RAC_leads
SET
	consumer_first_name = COALESCE(sqlc.narg(consumer_first_name)::text, consumer_first_name),
	consumer_last_name = COALESCE(sqlc.narg(consumer_last_name)::text, consumer_last_name),
	consumer_phone = COALESCE(sqlc.narg(consumer_phone)::text, consumer_phone),
	consumer_email = COALESCE(sqlc.narg(consumer_email)::text, consumer_email),
	consumer_role = COALESCE(sqlc.narg(consumer_role)::text, consumer_role),
	address_street = COALESCE(sqlc.narg(address_street)::text, address_street),
	address_house_number = COALESCE(sqlc.narg(address_house_number)::text, address_house_number),
	address_zip_code = COALESCE(sqlc.narg(address_zip_code)::text, address_zip_code),
	address_city = COALESCE(sqlc.narg(address_city)::text, address_city),
	latitude = COALESCE(sqlc.narg(latitude)::double precision, latitude),
	longitude = COALESCE(sqlc.narg(longitude)::double precision, longitude),
	assigned_agent_id = CASE WHEN sqlc.arg(assigned_agent_id_set)::bool THEN sqlc.narg(assigned_agent_id)::uuid ELSE assigned_agent_id END,
	whatsapp_opted_in = CASE WHEN sqlc.arg(whatsapp_opted_in_set)::bool THEN sqlc.arg(whatsapp_opted_in)::bool ELSE whatsapp_opted_in END,
	updated_at = now()
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL
RETURNING *;

-- name: CountLeads :one
SELECT COUNT(DISTINCT l.id)::int
FROM RAC_leads l
LEFT JOIN LATERAL (
	SELECT ls.id, ls.status, ls.service_type_id
	FROM RAC_lead_services ls
	WHERE ls.lead_id = l.id AND ls.pipeline_stage NOT IN ('Completed', 'Lost') AND ls.status != 'Disqualified'
	ORDER BY ls.created_at DESC
	LIMIT 1
) cs ON true
LEFT JOIN RAC_service_types st ON st.id = cs.service_type_id AND st.organization_id = l.organization_id
WHERE l.organization_id = sqlc.arg(organization_id)
	AND l.deleted_at IS NULL
	AND (sqlc.narg(status)::text IS NULL OR cs.status = sqlc.narg(status)::text)
	AND (sqlc.narg(service_type)::text IS NULL OR st.name = sqlc.narg(service_type)::text)
	AND (sqlc.narg(search)::text IS NULL OR (
		l.consumer_first_name ILIKE sqlc.narg(search)::text OR l.consumer_last_name ILIKE sqlc.narg(search)::text OR l.consumer_phone ILIKE sqlc.narg(search)::text OR l.consumer_email ILIKE sqlc.narg(search)::text OR l.address_city ILIKE sqlc.narg(search)::text
	))
	AND (sqlc.narg(first_name)::text IS NULL OR l.consumer_first_name ILIKE sqlc.narg(first_name)::text)
	AND (sqlc.narg(last_name)::text IS NULL OR l.consumer_last_name ILIKE sqlc.narg(last_name)::text)
	AND (sqlc.narg(phone)::text IS NULL OR l.consumer_phone ILIKE sqlc.narg(phone)::text)
	AND (sqlc.narg(email)::text IS NULL OR l.consumer_email ILIKE sqlc.narg(email)::text)
	AND (sqlc.narg(role)::text IS NULL OR l.consumer_role = sqlc.narg(role)::text)
	AND (sqlc.narg(street)::text IS NULL OR l.address_street ILIKE sqlc.narg(street)::text)
	AND (sqlc.narg(house_number)::text IS NULL OR l.address_house_number ILIKE sqlc.narg(house_number)::text)
	AND (sqlc.narg(zip_code)::text IS NULL OR l.address_zip_code ILIKE sqlc.narg(zip_code)::text)
	AND (sqlc.narg(city)::text IS NULL OR l.address_city ILIKE sqlc.narg(city)::text)
	AND (sqlc.narg(assigned_agent_id)::uuid IS NULL OR l.assigned_agent_id = sqlc.narg(assigned_agent_id)::uuid)
	AND (sqlc.narg(created_at_from)::timestamptz IS NULL OR l.created_at >= sqlc.narg(created_at_from)::timestamptz)
	AND (sqlc.narg(created_at_to)::timestamptz IS NULL OR l.created_at < sqlc.narg(created_at_to)::timestamptz);

-- name: ListLeads :many
SELECT * FROM (
	SELECT DISTINCT l.*
	FROM RAC_leads l
	LEFT JOIN LATERAL (
		SELECT ls.id, ls.status, ls.service_type_id
		FROM RAC_lead_services ls
		WHERE ls.lead_id = l.id AND ls.pipeline_stage NOT IN ('Completed', 'Lost') AND ls.status != 'Disqualified'
		ORDER BY ls.created_at DESC
		LIMIT 1
	) cs ON true
	LEFT JOIN RAC_service_types st ON st.id = cs.service_type_id AND st.organization_id = l.organization_id
	WHERE l.organization_id = sqlc.arg(organization_id)
		AND l.deleted_at IS NULL
		AND (sqlc.narg(status)::text IS NULL OR cs.status = sqlc.narg(status)::text)
		AND (sqlc.narg(service_type)::text IS NULL OR st.name = sqlc.narg(service_type)::text)
		AND (sqlc.narg(search)::text IS NULL OR (
			l.consumer_first_name ILIKE sqlc.narg(search)::text OR l.consumer_last_name ILIKE sqlc.narg(search)::text OR l.consumer_phone ILIKE sqlc.narg(search)::text OR l.consumer_email ILIKE sqlc.narg(search)::text OR l.address_city ILIKE sqlc.narg(search)::text
		))
		AND (sqlc.narg(first_name)::text IS NULL OR l.consumer_first_name ILIKE sqlc.narg(first_name)::text)
		AND (sqlc.narg(last_name)::text IS NULL OR l.consumer_last_name ILIKE sqlc.narg(last_name)::text)
		AND (sqlc.narg(phone)::text IS NULL OR l.consumer_phone ILIKE sqlc.narg(phone)::text)
		AND (sqlc.narg(email)::text IS NULL OR l.consumer_email ILIKE sqlc.narg(email)::text)
		AND (sqlc.narg(role)::text IS NULL OR l.consumer_role = sqlc.narg(role)::text)
		AND (sqlc.narg(street)::text IS NULL OR l.address_street ILIKE sqlc.narg(street)::text)
		AND (sqlc.narg(house_number)::text IS NULL OR l.address_house_number ILIKE sqlc.narg(house_number)::text)
		AND (sqlc.narg(zip_code)::text IS NULL OR l.address_zip_code ILIKE sqlc.narg(zip_code)::text)
		AND (sqlc.narg(city)::text IS NULL OR l.address_city ILIKE sqlc.narg(city)::text)
		AND (sqlc.narg(assigned_agent_id)::uuid IS NULL OR l.assigned_agent_id = sqlc.narg(assigned_agent_id)::uuid)
		AND (sqlc.narg(created_at_from)::timestamptz IS NULL OR l.created_at >= sqlc.narg(created_at_from)::timestamptz)
		AND (sqlc.narg(created_at_to)::timestamptz IS NULL OR l.created_at < sqlc.narg(created_at_to)::timestamptz)
) leads
ORDER BY
	CASE WHEN sqlc.arg(sort_by)::text = 'createdAt' AND sqlc.arg(sort_order)::text = 'asc' THEN leads.created_at END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'createdAt' AND sqlc.arg(sort_order)::text = 'desc' THEN leads.created_at END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'firstName' AND sqlc.arg(sort_order)::text = 'asc' THEN leads.consumer_first_name END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'firstName' AND sqlc.arg(sort_order)::text = 'desc' THEN leads.consumer_first_name END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'lastName' AND sqlc.arg(sort_order)::text = 'asc' THEN leads.consumer_last_name END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'lastName' AND sqlc.arg(sort_order)::text = 'desc' THEN leads.consumer_last_name END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'phone' AND sqlc.arg(sort_order)::text = 'asc' THEN leads.consumer_phone END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'phone' AND sqlc.arg(sort_order)::text = 'desc' THEN leads.consumer_phone END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'email' AND sqlc.arg(sort_order)::text = 'asc' THEN leads.consumer_email END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'email' AND sqlc.arg(sort_order)::text = 'desc' THEN leads.consumer_email END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'role' AND sqlc.arg(sort_order)::text = 'asc' THEN leads.consumer_role END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'role' AND sqlc.arg(sort_order)::text = 'desc' THEN leads.consumer_role END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'street' AND sqlc.arg(sort_order)::text = 'asc' THEN leads.address_street END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'street' AND sqlc.arg(sort_order)::text = 'desc' THEN leads.address_street END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'houseNumber' AND sqlc.arg(sort_order)::text = 'asc' THEN leads.address_house_number END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'houseNumber' AND sqlc.arg(sort_order)::text = 'desc' THEN leads.address_house_number END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'zipCode' AND sqlc.arg(sort_order)::text = 'asc' THEN leads.address_zip_code END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'zipCode' AND sqlc.arg(sort_order)::text = 'desc' THEN leads.address_zip_code END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'city' AND sqlc.arg(sort_order)::text = 'asc' THEN leads.address_city END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'city' AND sqlc.arg(sort_order)::text = 'desc' THEN leads.address_city END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'assignedAgentId' AND sqlc.arg(sort_order)::text = 'asc' THEN leads.assigned_agent_id END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'assignedAgentId' AND sqlc.arg(sort_order)::text = 'desc' THEN leads.assigned_agent_id END DESC,
	leads.created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: UpdateEnergyLabel :execrows
UPDATE RAC_leads
SET energy_class = $3,
	energy_index = $4,
	energy_bouwjaar = $5,
	energy_gebouwtype = $6,
	energy_label_valid_until = $7,
	energy_label_registered_at = $8,
	energy_primair_fossiel = $9,
	energy_bag_verblijfsobject_id = $10,
	energy_label_fetched_at = $11,
	updated_at = $12
WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL;

-- name: UpdateLeadEnrichment :execrows
UPDATE RAC_leads
SET lead_enrichment_source = $3,
	lead_enrichment_postcode6 = $4,
	lead_enrichment_postcode4 = $5,
	lead_enrichment_buurtcode = $6,
	lead_enrichment_data_year = $7,
	lead_enrichment_gem_aardgasverbruik = $8,
	lead_enrichment_gem_elektriciteitsverbruik = $9,
	lead_enrichment_huishouden_grootte = $10,
	lead_enrichment_koopwoningen_pct = $11,
	lead_enrichment_bouwjaar_vanaf2000_pct = $12,
	lead_enrichment_woz_waarde = $13,
	lead_enrichment_mediaan_vermogen_x1000 = $14,
	lead_enrichment_gem_inkomen = $15,
	lead_enrichment_pct_hoog_inkomen = $16,
	lead_enrichment_pct_laag_inkomen = $17,
	lead_enrichment_huishoudens_met_kinderen_pct = $18,
	lead_enrichment_stedelijkheid = $19,
	lead_enrichment_confidence = $20,
	lead_enrichment_fetched_at = $21,
	lead_score = $22,
	lead_score_pre_ai = $23,
	lead_score_factors = $24,
	lead_score_version = $25,
	lead_score_updated_at = $26,
	updated_at = $27
WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL;

-- name: UpdateLeadScore :execrows
UPDATE RAC_leads
SET lead_score = $3,
	lead_score_pre_ai = $4,
	lead_score_factors = $5,
	lead_score_version = $6,
	lead_score_updated_at = $7,
	updated_at = $8
WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL;

-- name: UpdateProjectedValueCents :execrows
UPDATE RAC_leads
SET projected_value_cents = $3, updated_at = now()
WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL;

-- name: SetLeadViewedBy :exec
UPDATE RAC_leads
SET viewed_by_id = $3, viewed_at = now(), updated_at = now()
WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL;

-- name: AddLeadActivity :exec
INSERT INTO RAC_lead_activity (lead_id, organization_id, user_id, action, meta)
VALUES ($1, $2, $3, $4, $5);

-- name: ListHeatmapPoints :many
SELECT latitude, longitude
FROM RAC_leads
WHERE organization_id = $1
	AND deleted_at IS NULL
	AND latitude IS NOT NULL
	AND longitude IS NOT NULL
	AND ($2::timestamptz IS NULL OR created_at >= $2)
	AND ($3::timestamptz IS NULL OR created_at < $3);

-- name: CountActionItems :one
SELECT COUNT(*)::int
FROM RAC_leads l
LEFT JOIN (
	SELECT DISTINCT ON (lead_id) lead_id, urgency_level, urgency_reason, created_at
	FROM RAC_lead_ai_analysis
	ORDER BY lead_id, created_at DESC
) ai ON ai.lead_id = l.id
WHERE l.organization_id = $1
	AND l.deleted_at IS NULL
	AND (ai.urgency_level = 'High' OR l.created_at >= now() - ($2::int || ' days')::interval);

-- name: ListActionItems :many
SELECT l.id, l.consumer_first_name, l.consumer_last_name, ai.urgency_level, ai.urgency_reason, l.created_at
FROM RAC_leads l
LEFT JOIN (
	SELECT DISTINCT ON (lead_id) lead_id, urgency_level, urgency_reason, created_at
	FROM RAC_lead_ai_analysis
	ORDER BY lead_id, created_at DESC
) ai ON ai.lead_id = l.id
WHERE l.organization_id = $1
	AND l.deleted_at IS NULL
	AND (ai.urgency_level = 'High' OR l.created_at >= now() - ($2::int || ' days')::interval)
ORDER BY
	CASE WHEN ai.urgency_level = 'High' THEN 0 ELSE 1 END,
	l.created_at DESC
LIMIT $3 OFFSET $4;

-- name: DeleteLead :execrows
UPDATE RAC_leads
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL;

-- name: BulkDeleteLeads :execrows
UPDATE RAC_leads
SET deleted_at = now(), updated_at = now()
WHERE id = ANY($1::uuid[])
	AND organization_id = $2
	AND deleted_at IS NULL;

-- name: ListRecentActivity :many
WITH unified AS (
	SELECT
		la.id,
		'leads'::text AS category,
		la.action AS event_type,
		la.action AS title,
		''::text AS description,
		la.lead_id AS entity_id,
		COALESCE(NULLIF(trim(concat_ws(' ', l.consumer_first_name, l.consumer_last_name)), ''), '') AS lead_name,
		COALESCE(l.consumer_phone, '') AS phone,
		COALESCE(l.consumer_email, '') AS email,
		COALESCE(svc.status, '') AS lead_status,
		COALESCE(svc.name, '') AS service_type,
		l.lead_score,
		NULL::text AS address,
		NULL::double precision AS latitude,
		NULL::double precision AS longitude,
		NULL::timestamptz AS scheduled_at,
		la.created_at,
		COALESCE(NULLIF(trim(concat_ws(' ', u.first_name, u.last_name)), ''), 'Systeem') AS actor_name,
		la.meta AS raw_metadata,
		NULL::uuid AS service_id
	FROM RAC_lead_activity la
	LEFT JOIN RAC_leads l ON l.id = la.lead_id AND l.organization_id = la.organization_id
	LEFT JOIN RAC_users u ON u.id = la.user_id
	LEFT JOIN LATERAL (
		SELECT ls.status, st.name
		FROM RAC_lead_services ls
		LEFT JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = l.organization_id
		WHERE ls.lead_id = l.id
		ORDER BY ls.created_at DESC
		LIMIT 1
	) svc ON true
	WHERE la.organization_id = $1
		AND la.action != 'lead_viewed'

	UNION ALL

	SELECT
		qa.id,
		'quotes'::text AS category,
		qa.event_type,
		qa.message AS title,
		''::text AS description,
		qa.quote_id AS entity_id,
		COALESCE(NULLIF(trim(concat_ws(' ', l.consumer_first_name, l.consumer_last_name)), ''), '') AS lead_name,
		COALESCE(l.consumer_phone, '') AS phone,
		COALESCE(l.consumer_email, '') AS email,
		COALESCE(ls.status, '') AS lead_status,
		COALESCE(st.name, '') AS service_type,
		l.lead_score,
		NULL::text AS address,
		NULL::double precision AS latitude,
		NULL::double precision AS longitude,
		NULL::timestamptz AS scheduled_at,
		qa.created_at,
		'Systeem'::text AS actor_name,
		qa.metadata AS raw_metadata,
		NULL::uuid AS service_id
	FROM RAC_quote_activity qa
	LEFT JOIN RAC_quotes q ON q.id = qa.quote_id
	LEFT JOIN RAC_lead_services ls ON ls.id = q.lead_service_id
	LEFT JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = qa.organization_id
	LEFT JOIN RAC_leads l ON l.id = ls.lead_id AND l.organization_id = qa.organization_id
	WHERE qa.organization_id = $1

	UNION ALL

	SELECT
		a.id,
		'appointments'::text AS category,
		CASE
			WHEN a.created_at = a.updated_at THEN 'appointment_created'::text
			ELSE 'appointment_updated'::text
		END AS event_type,
		a.title,
		COALESCE(a.description, '') AS description,
		a.id AS entity_id,
		COALESCE(NULLIF(trim(concat_ws(' ', l.consumer_first_name, l.consumer_last_name)), ''), '') AS lead_name,
		COALESCE(l.consumer_phone, '') AS phone,
		COALESCE(l.consumer_email, '') AS email,
		COALESCE(als.status, svc.status, '') AS lead_status,
		COALESCE(ast.name, svc.name, '') AS service_type,
		l.lead_score,
		COALESCE(
			NULLIF(a.location, ''),
			concat_ws(', ',
				concat_ws(' ', l.address_street, l.address_house_number),
				concat_ws(' ', l.address_zip_code, l.address_city)
			)
		) AS address,
		l.latitude,
		l.longitude,
		a.start_time AS scheduled_at,
		a.updated_at AS created_at,
		'Systeem'::text AS actor_name,
		NULL::jsonb AS raw_metadata,
		NULL::uuid AS service_id
	FROM RAC_appointments a
	LEFT JOIN RAC_leads l ON l.id = a.lead_id AND l.organization_id = a.organization_id
	LEFT JOIN RAC_lead_services als ON als.id = a.lead_service_id
	LEFT JOIN RAC_service_types ast ON ast.id = als.service_type_id AND ast.organization_id = a.organization_id
	LEFT JOIN LATERAL (
		SELECT ls.status, st.name
		FROM RAC_lead_services ls
		LEFT JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = l.organization_id
		WHERE ls.lead_id = l.id
		ORDER BY ls.created_at DESC
		LIMIT 1
	) svc ON true
	WHERE a.organization_id = $1

	UNION ALL

	SELECT
		te.id,
		'ai'::text AS category,
		te.event_type,
		te.title,
		COALESCE(te.summary, '') AS description,
		te.lead_id AS entity_id,
		COALESCE(NULLIF(trim(concat_ws(' ', l.consumer_first_name, l.consumer_last_name)), ''), '') AS lead_name,
		COALESCE(l.consumer_phone, '') AS phone,
		COALESCE(l.consumer_email, '') AS email,
		COALESCE(svc.status, '') AS lead_status,
		COALESCE(svc.name, '') AS service_type,
		l.lead_score,
		NULL::text AS address,
		NULL::double precision AS latitude,
		NULL::double precision AS longitude,
		NULL::timestamptz AS scheduled_at,
		te.created_at,
		COALESCE(te.actor_name, 'AI') AS actor_name,
		te.metadata AS raw_metadata,
		te.service_id
	FROM lead_timeline_events te
	LEFT JOIN RAC_leads l ON l.id = te.lead_id AND l.organization_id = te.organization_id
	LEFT JOIN LATERAL (
		SELECT ls.status, st.name
		FROM RAC_lead_services ls
		LEFT JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = l.organization_id
		WHERE ls.lead_id = l.id
		ORDER BY ls.created_at DESC
		LIMIT 1
	) svc ON true
	WHERE te.organization_id = $1
		AND te.visibility <> 'debug'
		AND te.event_type IN ('ai', 'photo_analysis_completed')
),
with_gap AS (
	SELECT *,
		CASE
			WHEN created_at - LAG(created_at) OVER (
				PARTITION BY entity_id, event_type, category
				ORDER BY created_at
			) <= interval '15 minutes' THEN 0
			ELSE 1
		END AS is_new_cluster
	FROM unified
),
clustered AS (
	SELECT *,
		SUM(is_new_cluster) OVER (
			PARTITION BY entity_id, event_type, category
			ORDER BY created_at
		) AS cluster_id
	FROM with_gap
),
with_count AS (
	SELECT *,
		COUNT(*) OVER (
			PARTITION BY entity_id, event_type, category, cluster_id
		)::int AS group_count
	FROM clustered
),
deduped AS (
	SELECT DISTINCT ON (entity_id, event_type, category, cluster_id)
		id, category, event_type, title, description, entity_id,
		service_id,
		lead_name, phone, email, lead_status, service_type, lead_score,
		COALESCE(address, '') AS address, latitude, longitude,
		scheduled_at, created_at, 0::int AS priority,
		group_count, COALESCE(actor_name, '') AS actor_name, raw_metadata
	FROM with_count
	ORDER BY entity_id, event_type, category, cluster_id, created_at DESC
)
SELECT * FROM deduped
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListUpcomingAppointments :many
SELECT
	a.id,
	'appointments'::text AS category,
	'appointment_upcoming'::text AS event_type,
	a.title,
	COALESCE(a.description, '') AS description,
	a.id AS entity_id,
	COALESCE(NULLIF(trim(concat_ws(' ', l.consumer_first_name, l.consumer_last_name)), ''), '') AS lead_name,
	COALESCE(l.consumer_phone, '') AS phone,
	COALESCE(l.consumer_email, '') AS email,
	COALESCE(als.status, svc.status, '') AS lead_status,
	COALESCE(ast.name, svc.name, '') AS service_type,
	l.lead_score,
	COALESCE(
		NULLIF(a.location, ''),
		concat_ws(', ',
			concat_ws(' ', l.address_street, l.address_house_number),
			concat_ws(' ', l.address_zip_code, l.address_city)
		)
	) AS address,
	l.latitude,
	l.longitude,
	a.start_time AS scheduled_at,
	now() AS created_at,
	2::int AS priority
FROM RAC_appointments a
LEFT JOIN RAC_leads l ON l.id = a.lead_id AND l.organization_id = a.organization_id
LEFT JOIN RAC_lead_services als ON als.id = a.lead_service_id
LEFT JOIN RAC_service_types ast ON ast.id = als.service_type_id AND ast.organization_id = a.organization_id
LEFT JOIN LATERAL (
	SELECT ls.status, st.name
	FROM RAC_lead_services ls
	LEFT JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = l.organization_id
	WHERE ls.lead_id = l.id
	ORDER BY ls.created_at DESC
	LIMIT 1
) svc ON true
WHERE a.organization_id = $1
	AND a.status = 'scheduled'
	AND a.start_time > now()
	AND a.start_time <= now() + interval '48 hours'
ORDER BY a.start_time ASC
LIMIT $2;

-- name: GetLatestAppointmentVisitReportByService :one
SELECT avr.appointment_id, avr.organization_id, avr.measurements, avr.access_difficulty, avr.notes, avr.created_at, avr.updated_at
FROM RAC_appointment_visit_reports avr
JOIN RAC_appointments a ON a.id = avr.appointment_id AND a.organization_id = avr.organization_id
WHERE a.lead_service_id = $1 AND avr.organization_id = $2 AND a.status != 'cancelled'
ORDER BY a.start_time DESC, avr.updated_at DESC
LIMIT 1;

-- name: CreateAIDecisionMemory :one
INSERT INTO RAC_ai_decision_memory (
	organization_id, lead_id, lead_service_id, service_type, decision_type, outcome,
	confidence, context_summary, action_summary
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, organization_id, lead_id, lead_service_id, service_type, decision_type, outcome,
	confidence, context_summary, action_summary, created_at;

-- name: ListRecentAIDecisionMemories :many
SELECT id, organization_id, lead_id, lead_service_id, service_type, decision_type, outcome,
	confidence, context_summary, action_summary, created_at
FROM RAC_ai_decision_memory
WHERE organization_id = $1
	AND (sqlc.narg(service_type)::text IS NULL OR service_type = sqlc.narg(service_type)::text)
ORDER BY created_at DESC
LIMIT $2;

-- name: ListFrequentAdHocQuoteItems :many
WITH items AS (
	SELECT
		LOWER(REGEXP_REPLACE(TRIM(qi.description), '\s+', ' ', 'g')) AS dnorm,
		qi.description,
		q.created_at
	FROM RAC_quote_items qi
	JOIN RAC_quotes q ON q.id = qi.quote_id
	WHERE qi.organization_id = $1
		AND q.organization_id = $1
		AND q.status != 'Draft'
		AND qi.catalog_product_id IS NULL
		AND qi.description IS NOT NULL
		AND TRIM(qi.description) != ''
		AND q.created_at >= (NOW() - ($2::int || ' days')::interval)
		AND (qi.is_optional = false OR qi.is_selected = true)
)
SELECT
	MIN(description) AS representative_description,
	COUNT(*)::int AS cnt,
	MAX(created_at) AS last_seen
FROM items
GROUP BY dnorm
HAVING COUNT(*) >= $3
ORDER BY cnt DESC, last_seen DESC
LIMIT $4;

-- name: CreateCatalogSearchLog :exec
INSERT INTO RAC_catalog_search_log (
	organization_id, lead_service_id, query, collection, result_count, top_score, created_at
)
VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,
	$6,
	COALESCE(sqlc.narg(created_at)::timestamptz, NOW())
);

-- name: ListFrequentCatalogSearchMisses :many
WITH misses AS (
	SELECT
		LOWER(REGEXP_REPLACE(TRIM(query), '\s+', ' ', 'g')) AS qnorm,
		query,
		collection,
		created_at
	FROM RAC_catalog_search_log
	WHERE organization_id = $1
		AND result_count = 0
		AND created_at >= (NOW() - ($2::int || ' days')::interval)
)
SELECT
	MIN(query) AS representative_query,
	COUNT(*)::int AS cnt,
	MAX(created_at) AS last_seen,
	ARRAY_AGG(DISTINCT collection)::text[] AS collections
FROM misses
GROUP BY qnorm
HAVING COUNT(*) >= $3
ORDER BY cnt DESC, last_seen DESC
LIMIT $4;
