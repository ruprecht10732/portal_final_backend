CREATE EXTENSION IF NOT EXISTS cube;
CREATE EXTENSION IF NOT EXISTS earthdistance;

CREATE TYPE pipeline_stage AS ENUM (
    'Triage',
    'Nurturing',
    'Ready_For_Estimator',
    'Ready_For_Partner',
    'Partner_Matching',
    'Partner_Assigned',
    'Manual_Intervention',
    'Completed',
    'Lost'
);

ALTER TABLE RAC_lead_services
ADD COLUMN pipeline_stage pipeline_stage NOT NULL DEFAULT 'Triage';

CREATE INDEX idx_lead_services_pipeline ON RAC_lead_services(pipeline_stage);

CREATE TABLE lead_timeline_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES RAC_leads(id) ON DELETE CASCADE,
    service_id UUID REFERENCES RAC_lead_services(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL,
    actor_type TEXT NOT NULL,
    actor_name TEXT NOT NULL,
    event_type TEXT NOT NULL,
    title TEXT NOT NULL,
    summary TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_timeline_lookup ON lead_timeline_events(lead_id, created_at DESC);
