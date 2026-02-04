-- Migration: Add service types for leads schema alignment
-- This keeps sqlc schema in sync with the app queries.

CREATE TABLE IF NOT EXISTS RAC_service_types (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT,
    icon TEXT,
    color TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    display_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_service_types_slug ON RAC_service_types(slug);
CREATE UNIQUE INDEX IF NOT EXISTS idx_service_types_name ON RAC_service_types(name);

ALTER TABLE RAC_lead_services
    ADD COLUMN IF NOT EXISTS service_type_id UUID REFERENCES RAC_service_types(id);

CREATE INDEX IF NOT EXISTS idx_lead_services_service_type_id
    ON RAC_lead_services(service_type_id);
