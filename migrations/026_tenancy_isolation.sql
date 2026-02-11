-- +goose Up
-- Migration: 023_tenancy_isolation.sql
-- Purpose: Add organization_id to all tenant-owned tables for strict multi-tenant isolation
-- Note: This migration truncates tables (dev mode) to allow NOT NULL constraints

-- ============================================
-- LEADS DOMAIN
-- ============================================

-- Truncate RAC_leads-related tables (dev mode - no data preservation needed)
TRUNCATE TABLE RAC_lead_ai_analysis CASCADE;
TRUNCATE TABLE RAC_lead_notes CASCADE;
TRUNCATE TABLE RAC_lead_activity CASCADE;
TRUNCATE TABLE RAC_lead_services CASCADE;
TRUNCATE TABLE RAC_leads CASCADE;

-- Add organization_id to RAC_leads table
ALTER TABLE RAC_leads ADD COLUMN organization_id UUID NOT NULL REFERENCES organizations(id);

-- Add organization_id to RAC_lead_services table
ALTER TABLE RAC_lead_services ADD COLUMN organization_id UUID NOT NULL REFERENCES organizations(id);

-- Add organization_id to RAC_lead_activity table
ALTER TABLE RAC_lead_activity ADD COLUMN organization_id UUID NOT NULL REFERENCES organizations(id);

-- Add organization_id to RAC_lead_notes table
ALTER TABLE RAC_lead_notes ADD COLUMN organization_id UUID NOT NULL REFERENCES organizations(id);

-- Add organization_id to RAC_lead_ai_analysis table
ALTER TABLE RAC_lead_ai_analysis ADD COLUMN organization_id UUID NOT NULL REFERENCES organizations(id);

-- ============================================
-- APPOINTMENTS DOMAIN
-- ============================================

-- Truncate appointments-related tables (dev mode - no data preservation needed)
TRUNCATE TABLE appointment_attachments CASCADE;
TRUNCATE TABLE appointment_visit_reports CASCADE;
TRUNCATE TABLE appointment_availability_overrides CASCADE;
TRUNCATE TABLE appointment_availability_rules CASCADE;
TRUNCATE TABLE appointments CASCADE;

-- Add organization_id to appointments table
ALTER TABLE appointments ADD COLUMN organization_id UUID NOT NULL REFERENCES organizations(id);

-- Add organization_id to appointment_visit_reports table
ALTER TABLE appointment_visit_reports ADD COLUMN organization_id UUID NOT NULL REFERENCES organizations(id);

-- Add organization_id to appointment_attachments table
ALTER TABLE appointment_attachments ADD COLUMN organization_id UUID NOT NULL REFERENCES organizations(id);

-- Add organization_id to appointment_availability_rules table
ALTER TABLE appointment_availability_rules ADD COLUMN organization_id UUID NOT NULL REFERENCES organizations(id);

-- Add organization_id to appointment_availability_overrides table
ALTER TABLE appointment_availability_overrides ADD COLUMN organization_id UUID NOT NULL REFERENCES organizations(id);

-- ============================================
-- INDEXES (Crucial for performance and isolation)
-- ============================================

-- Leads domain indexes
CREATE INDEX idx_leads_org ON RAC_leads(organization_id);
CREATE INDEX idx_leads_org_deleted ON RAC_leads(organization_id, deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX idx_lead_services_org ON RAC_lead_services(organization_id);
CREATE INDEX idx_lead_activity_org ON RAC_lead_activity(organization_id);
CREATE INDEX idx_lead_notes_org ON RAC_lead_notes(organization_id);
CREATE INDEX idx_lead_ai_analysis_org ON RAC_lead_ai_analysis(organization_id);

-- Appointments domain indexes
CREATE INDEX idx_appointments_org ON appointments(organization_id);
CREATE INDEX idx_appointments_org_user ON appointments(organization_id, user_id);
CREATE INDEX idx_appointment_visit_reports_org ON appointment_visit_reports(organization_id);
CREATE INDEX idx_appointment_attachments_org ON appointment_attachments(organization_id);
CREATE INDEX idx_appointment_availability_rules_org ON appointment_availability_rules(organization_id);
CREATE INDEX idx_appointment_availability_overrides_org ON appointment_availability_overrides(organization_id);

-- Composite indexes for common query patterns
CREATE INDEX idx_leads_org_assigned ON RAC_leads(organization_id, assigned_agent_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_appointments_org_time ON appointments(organization_id, start_time, end_time);

-- +goose Down
DROP INDEX IF EXISTS idx_appointments_org_time;
DROP INDEX IF EXISTS idx_leads_org_assigned;
DROP INDEX IF EXISTS idx_appointment_availability_overrides_org;
DROP INDEX IF EXISTS idx_appointment_availability_rules_org;
DROP INDEX IF EXISTS idx_appointment_attachments_org;
DROP INDEX IF EXISTS idx_appointment_visit_reports_org;
DROP INDEX IF EXISTS idx_appointments_org_user;
DROP INDEX IF EXISTS idx_appointments_org;
DROP INDEX IF EXISTS idx_lead_ai_analysis_org;
DROP INDEX IF EXISTS idx_lead_notes_org;
DROP INDEX IF EXISTS idx_lead_activity_org;
DROP INDEX IF EXISTS idx_lead_services_org;
DROP INDEX IF EXISTS idx_leads_org_deleted;
DROP INDEX IF EXISTS idx_leads_org;

ALTER TABLE appointment_availability_overrides DROP COLUMN IF EXISTS organization_id;
ALTER TABLE appointment_availability_rules DROP COLUMN IF EXISTS organization_id;
ALTER TABLE appointment_attachments DROP COLUMN IF EXISTS organization_id;
ALTER TABLE appointment_visit_reports DROP COLUMN IF EXISTS organization_id;
ALTER TABLE appointments DROP COLUMN IF EXISTS organization_id;
ALTER TABLE RAC_lead_ai_analysis DROP COLUMN IF EXISTS organization_id;
ALTER TABLE RAC_lead_notes DROP COLUMN IF EXISTS organization_id;
ALTER TABLE RAC_lead_activity DROP COLUMN IF EXISTS organization_id;
ALTER TABLE RAC_lead_services DROP COLUMN IF EXISTS organization_id;
ALTER TABLE RAC_leads DROP COLUMN IF EXISTS organization_id;
