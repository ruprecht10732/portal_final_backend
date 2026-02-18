-- +goose Up
-- Big-bang cutover: remove legacy pipeline stages and lifecycle-specific statuses.
-- Final model:
--   pipeline_stage: Triage, Nurturing, Estimation, Proposal, Fulfillment, Manual_Intervention, Completed, Lost
--   status: New, Pending, In_Progress, Appointment_Scheduled, Attempted_Contact, Needs_Rescheduling, Disqualified

-- 1) Backfill pipeline stage values on service events (text column, safe to update directly)
UPDATE RAC_lead_service_events SET pipeline_stage = 'Estimation'
WHERE pipeline_stage IN ('Ready_For_Estimator', 'Quote_Draft');

UPDATE RAC_lead_service_events SET pipeline_stage = 'Proposal'
WHERE pipeline_stage = 'Quote_Sent';

UPDATE RAC_lead_service_events SET pipeline_stage = 'Fulfillment'
WHERE pipeline_stage IN ('Ready_For_Partner', 'Partner_Matching', 'Partner_Assigned');

-- 2) Replace pipeline_stage enum with condensed version.
--    Drop any leftover v2 type from a failed previous run, then create fresh.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'pipeline_stage_v2') THEN
    EXECUTE 'DROP TYPE pipeline_stage_v2';
  END IF;
END $$;
-- +goose StatementEnd

CREATE TYPE pipeline_stage_v2 AS ENUM (
  'Triage',
  'Nurturing',
  'Estimation',
  'Proposal',
  'Fulfillment',
  'Manual_Intervention',
  'Completed',
  'Lost'
);

-- Drop the column default first; it references the old enum type and blocks the cast.
ALTER TABLE RAC_lead_services ALTER COLUMN pipeline_stage DROP DEFAULT;

ALTER TABLE RAC_lead_services
  ALTER COLUMN pipeline_stage TYPE pipeline_stage_v2
  USING (
    CASE
      WHEN pipeline_stage::text IN ('Ready_For_Estimator', 'Quote_Draft') THEN 'Estimation'
      WHEN pipeline_stage::text = 'Quote_Sent'                            THEN 'Proposal'
      WHEN pipeline_stage::text IN ('Ready_For_Partner', 'Partner_Matching', 'Partner_Assigned') THEN 'Fulfillment'
      ELSE pipeline_stage::text
    END
  )::pipeline_stage_v2;

-- Re-apply the default using the new enum type.
ALTER TABLE RAC_lead_services ALTER COLUMN pipeline_stage SET DEFAULT 'Triage'::pipeline_stage_v2;

DROP TYPE pipeline_stage;
ALTER TYPE pipeline_stage_v2 RENAME TO pipeline_stage;

-- 3) Backfill statuses on services (legacy -> generic activity indicators)
UPDATE RAC_lead_services SET status = 'New'
WHERE status IN ('Survey_Completed', 'Quote_Draft', 'Quote_Accepted');

UPDATE RAC_lead_services SET status = 'Pending'
WHERE status = 'Quote_Sent';

UPDATE RAC_lead_services SET status = 'In_Progress'
WHERE status = 'Partner_Assigned';

UPDATE RAC_lead_services SET status = 'In_Progress'
WHERE status = 'Completed';

UPDATE RAC_lead_services SET status = 'New'
WHERE status = 'Lost';

-- 4) Backfill statuses on service events
UPDATE RAC_lead_service_events SET status = 'New'
WHERE status IN ('Survey_Completed', 'Quote_Draft', 'Quote_Accepted');

UPDATE RAC_lead_service_events SET status = 'Pending'
WHERE status = 'Quote_Sent';

UPDATE RAC_lead_service_events SET status = 'In_Progress'
WHERE status = 'Partner_Assigned';

UPDATE RAC_lead_service_events SET status = 'In_Progress'
WHERE status = 'Completed';

UPDATE RAC_lead_service_events SET status = 'New'
WHERE status = 'Lost';

-- 5) Tighten status CHECK constraints
ALTER TABLE RAC_lead_services DROP CONSTRAINT IF EXISTS rac_lead_services_status_check;
ALTER TABLE RAC_lead_services DROP CONSTRAINT IF EXISTS lead_services_status_check;
ALTER TABLE RAC_lead_services
  ADD CONSTRAINT rac_lead_services_status_check CHECK (
    status IN (
      'New',
      'Pending',
      'In_Progress',
      'Attempted_Contact',
      'Appointment_Scheduled',
      'Needs_Rescheduling',
      'Disqualified'
    )
  );

-- Conditionally tighten status on RAC_leads if that column exists.
-- Uses single-quoted SQL strings to avoid nested dollar-quoting.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name   = 'rac_leads'
      AND column_name  = 'status'
  ) THEN
    EXECUTE 'ALTER TABLE RAC_leads DROP CONSTRAINT IF EXISTS leads_status_check';
    EXECUTE 'ALTER TABLE RAC_leads ADD CONSTRAINT leads_status_check CHECK ('
         || 'status IN (''New'', ''Pending'', ''In_Progress'', ''Attempted_Contact'', '
         || '''Appointment_Scheduled'', ''Needs_Rescheduling'', ''Disqualified''))';
  END IF;
END $$;
-- +goose StatementEnd

-- 6) Allow visit_completed event type
ALTER TABLE RAC_lead_service_events DROP CONSTRAINT IF EXISTS rac_lead_service_events_event_type_check;
ALTER TABLE RAC_lead_service_events
  ADD CONSTRAINT rac_lead_service_events_event_type_check
  CHECK (event_type IN ('status_changed', 'pipeline_stage_changed', 'service_created', 'visit_completed'));

-- 7) Assert no legacy values remain (fail hard if backfill missed any rows)
-- +goose StatementBegin
DO $$
DECLARE
  legacy_stage_count  bigint;
  legacy_status_count bigint;
BEGIN
  SELECT COUNT(*) INTO legacy_stage_count
  FROM RAC_lead_services
  WHERE pipeline_stage::text IN (
    'Ready_For_Estimator', 'Quote_Draft', 'Quote_Sent',
    'Ready_For_Partner', 'Partner_Matching', 'Partner_Assigned'
  );

  IF legacy_stage_count > 0 THEN
    RAISE EXCEPTION 'legacy pipeline stages remain in RAC_lead_services: %', legacy_stage_count;
  END IF;

  SELECT COUNT(*) INTO legacy_status_count
  FROM RAC_lead_services
  WHERE status IN (
    'Survey_Completed', 'Quote_Draft', 'Quote_Sent',
    'Quote_Accepted', 'Partner_Assigned', 'Completed', 'Lost'
  );

  IF legacy_status_count > 0 THEN
    RAISE EXCEPTION 'legacy statuses remain in RAC_lead_services: %', legacy_status_count;
  END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- Best-effort rollback: reintroduces legacy enums and widens status constraints.

-- Restore event_type constraint (drop visit_completed)
ALTER TABLE RAC_lead_service_events DROP CONSTRAINT IF EXISTS rac_lead_service_events_event_type_check;
ALTER TABLE RAC_lead_service_events
  ADD CONSTRAINT rac_lead_service_events_event_type_check
  CHECK (event_type IN ('status_changed', 'pipeline_stage_changed', 'service_created'));

-- Widen status constraints back to the pre-110 set
ALTER TABLE RAC_lead_services DROP CONSTRAINT IF EXISTS rac_lead_services_status_check;
ALTER TABLE RAC_lead_services
  ADD CONSTRAINT rac_lead_services_status_check CHECK (
    status IN (
      'New',
      'Attempted_Contact',
      'Appointment_Scheduled',
      'Survey_Completed',
      'Quote_Draft',
      'Quote_Sent',
      'Quote_Accepted',
      'Partner_Assigned',
      'Needs_Rescheduling',
      'Completed',
      'Lost',
      'Disqualified'
    )
  );

-- Recreate legacy pipeline_stage enum (v1), drop leftover if present
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'pipeline_stage_v1') THEN
    EXECUTE 'DROP TYPE pipeline_stage_v1';
  END IF;
END $$;
-- +goose StatementEnd

CREATE TYPE pipeline_stage_v1 AS ENUM (
  'Triage',
  'Nurturing',
  'Ready_For_Estimator',
  'Quote_Draft',
  'Quote_Sent',
  'Ready_For_Partner',
  'Partner_Matching',
  'Partner_Assigned',
  'Manual_Intervention',
  'Completed',
  'Lost'
);

ALTER TABLE RAC_lead_services
  ALTER COLUMN pipeline_stage TYPE pipeline_stage_v1
  USING (
    CASE
      WHEN pipeline_stage::text = 'Estimation'  THEN 'Ready_For_Estimator'
      WHEN pipeline_stage::text = 'Proposal'    THEN 'Quote_Sent'
      WHEN pipeline_stage::text = 'Fulfillment' THEN 'Partner_Matching'
      ELSE pipeline_stage::text
    END
  )::pipeline_stage_v1;

DROP TYPE pipeline_stage;
ALTER TYPE pipeline_stage_v1 RENAME TO pipeline_stage;
