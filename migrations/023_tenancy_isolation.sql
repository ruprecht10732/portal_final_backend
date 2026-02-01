-- Migration: 023_tenancy_isolation.sql
-- Purpose: Add organization_id to all tenant-owned tables for strict multi-tenant isolation
-- Note: This migration truncates tables (dev mode) to allow NOT NULL constraints

-- ============================================
-- LEADS DOMAIN
-- ============================================

-- Truncate leads-related tables (dev mode - no data preservation needed)
TRUNCATE TABLE lead_ai_analysis CASCADE;
TRUNCATE TABLE lead_notes CASCADE;
TRUNCATE TABLE lead_activity CASCADE;
TRUNCATE TABLE lead_services CASCADE;
TRUNCATE TABLE leads CASCADE;

-- Add organization_id to leads table
ALTER TABLE leads ADD COLUMN organization_id UUID NOT NULL REFERENCES organizations(id);

-- Add organization_id to lead_services table
ALTER TABLE lead_services ADD COLUMN organization_id UUID NOT NULL REFERENCES organizations(id);

-- Add organization_id to lead_activity table
ALTER TABLE lead_activity ADD COLUMN organization_id UUID NOT NULL REFERENCES organizations(id);

-- Add organization_id to lead_notes table
ALTER TABLE lead_notes ADD COLUMN organization_id UUID NOT NULL REFERENCES organizations(id);

-- Add organization_id to lead_ai_analysis table
ALTER TABLE lead_ai_analysis ADD COLUMN organization_id UUID NOT NULL REFERENCES organizations(id);

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
CREATE INDEX idx_leads_org ON leads(organization_id);
CREATE INDEX idx_leads_org_deleted ON leads(organization_id, deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX idx_lead_services_org ON lead_services(organization_id);
CREATE INDEX idx_lead_activity_org ON lead_activity(organization_id);
CREATE INDEX idx_lead_notes_org ON lead_notes(organization_id);
CREATE INDEX idx_lead_ai_analysis_org ON lead_ai_analysis(organization_id);

-- Appointments domain indexes
CREATE INDEX idx_appointments_org ON appointments(organization_id);
CREATE INDEX idx_appointments_org_user ON appointments(organization_id, user_id);
CREATE INDEX idx_appointment_visit_reports_org ON appointment_visit_reports(organization_id);
CREATE INDEX idx_appointment_attachments_org ON appointment_attachments(organization_id);
CREATE INDEX idx_appointment_availability_rules_org ON appointment_availability_rules(organization_id);
CREATE INDEX idx_appointment_availability_overrides_org ON appointment_availability_overrides(organization_id);

-- Composite indexes for common query patterns
CREATE INDEX idx_leads_org_assigned ON leads(organization_id, assigned_agent_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_appointments_org_time ON appointments(organization_id, start_time, end_time);
