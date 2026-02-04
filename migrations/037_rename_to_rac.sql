-- Migration: Rename remaining tables to use RAC_ prefix for consistency with Semgrep rules

-- Organizations
ALTER TABLE organizations RENAME TO RAC_organizations;
ALTER TABLE organization_members RENAME TO RAC_organization_members;
ALTER TABLE organization_invites RENAME TO RAC_organization_invites;

-- Appointments
ALTER TABLE appointments RENAME TO RAC_appointments;
ALTER TABLE appointment_attachments RENAME TO RAC_appointment_attachments;
ALTER TABLE appointment_availability_overrides RENAME TO RAC_appointment_availability_overrides;
ALTER TABLE appointment_availability_rules RENAME TO RAC_appointment_availability_rules;
ALTER TABLE appointment_visit_reports RENAME TO RAC_appointment_visit_reports;

-- Assets & Analysis
ALTER TABLE catalog_product_assets RENAME TO RAC_catalog_product_assets;
ALTER TABLE lead_photo_analyses RENAME TO RAC_lead_photo_analyses;
ALTER TABLE lead_service_attachments RENAME TO RAC_lead_service_attachments;

-- Update foreign keys if necessary? 
-- Postgres handles renaming tables by updating references automatically in existing constraints.
