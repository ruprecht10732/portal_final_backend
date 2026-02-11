-- +goose Up
-- 1. Enable earthdistance for partner search (if not enabled)
CREATE EXTENSION IF NOT EXISTS cube;
CREATE EXTENSION IF NOT EXISTS earthdistance;

-- 2. Define the strict Lifecycle Stages
CREATE TYPE pipeline_stage AS ENUM (
    'Triage',              -- New data arrived. Gatekeeper Agent is analyzing.
    'Nurturing',           -- Missing info. Message sent to customer. Waiting.
    'Ready_For_Estimator', -- Intake valid. Technical Agent needs to scope/price.
    'Ready_For_Partner',   -- Scoped & Priced. Dispatcher Agent needs to find match.
    'Partner_Matching',    -- Partners found and invited. Waiting for acceptance.
    'Partner_Assigned',    -- Partner accepted job.
    'Manual_Intervention', -- AI Failure (e.g. 0 partners found). Human must act.
    'Completed',           -- Job done.
    'Lost'                 -- Dead lead / Rejected.
);

-- 3. Add Stage to Service (Current State)
ALTER TABLE RAC_lead_services 
ADD COLUMN pipeline_stage pipeline_stage NOT NULL DEFAULT 'Triage';

CREATE INDEX idx_lead_services_pipeline ON RAC_lead_services(pipeline_stage);

-- 4. Create the Immutable Timeline (History Ledger)
CREATE TABLE lead_timeline_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES RAC_leads(id) ON DELETE CASCADE,
    service_id UUID REFERENCES RAC_lead_services(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL,
    
    -- WHO did it?
    actor_type TEXT NOT NULL, -- 'AI', 'User', 'System'
    actor_name TEXT NOT NULL, -- 'Gatekeeper', 'John Agent'
    
    -- WHAT happened?
    event_type TEXT NOT NULL, -- 'analysis', 'stage_change', 'note', 'call_log', 'partner_search'
    
    -- The Content
    title TEXT NOT NULL,      -- e.g. "Moved to Manual Intervention"
    summary TEXT,             -- e.g. "No partners found within 25km radius."
    
    -- Technical Data (Snapshots)
    metadata JSONB DEFAULT '{}', 
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_timeline_lookup ON lead_timeline_events(lead_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS lead_timeline_events;
DROP INDEX IF EXISTS idx_lead_services_pipeline;
ALTER TABLE RAC_lead_services DROP COLUMN IF EXISTS pipeline_stage;
DROP TYPE IF EXISTS pipeline_stage;
DROP EXTENSION IF EXISTS earthdistance;
DROP EXTENSION IF EXISTS cube;

