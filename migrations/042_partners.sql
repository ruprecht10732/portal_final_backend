-- +goose Up
-- Migration: 038_partners.sql
-- Purpose: Add partners domain with tenant isolation and invite support

CREATE TABLE IF NOT EXISTS RAC_partners (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  business_name TEXT NOT NULL,
  kvk_number TEXT NOT NULL,
  vat_number TEXT NOT NULL,
  address_line1 TEXT NOT NULL,
  address_line2 TEXT,
  postal_code TEXT NOT NULL,
  city TEXT NOT NULL,
  country TEXT NOT NULL,
  contact_name TEXT NOT NULL,
  contact_email TEXT NOT NULL,
  contact_phone TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_partners_org_business_name
  ON RAC_partners(organization_id, lower(business_name));

CREATE UNIQUE INDEX IF NOT EXISTS idx_partners_org_kvk
  ON RAC_partners(organization_id, kvk_number);

CREATE UNIQUE INDEX IF NOT EXISTS idx_partners_org_vat
  ON RAC_partners(organization_id, vat_number);

CREATE INDEX IF NOT EXISTS idx_partners_org
  ON RAC_partners(organization_id);

CREATE INDEX IF NOT EXISTS idx_partners_contact_email
  ON RAC_partners(organization_id, lower(contact_email));

CREATE TABLE IF NOT EXISTS RAC_partner_leads (
  organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  partner_id UUID NOT NULL REFERENCES RAC_partners(id) ON DELETE CASCADE,
  lead_id UUID NOT NULL REFERENCES RAC_leads(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, partner_id, lead_id)
);

CREATE INDEX IF NOT EXISTS idx_partner_leads_org
  ON RAC_partner_leads(organization_id);

CREATE INDEX IF NOT EXISTS idx_partner_leads_partner
  ON RAC_partner_leads(partner_id);

CREATE INDEX IF NOT EXISTS idx_partner_leads_lead
  ON RAC_partner_leads(lead_id);

CREATE TABLE IF NOT EXISTS RAC_partner_invites (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  partner_id UUID NOT NULL REFERENCES RAC_partners(id) ON DELETE CASCADE,
  email TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_by UUID NOT NULL REFERENCES RAC_users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  used_at TIMESTAMPTZ,
  used_by UUID REFERENCES RAC_users(id) ON DELETE RESTRICT,
  lead_id UUID REFERENCES RAC_leads(id) ON DELETE SET NULL,
  lead_service_id UUID REFERENCES RAC_lead_services(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_partner_invites_active_email
  ON RAC_partner_invites(organization_id, partner_id, lower(email))
  WHERE used_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_partner_invites_org
  ON RAC_partner_invites(organization_id);

CREATE INDEX IF NOT EXISTS idx_partner_invites_partner
  ON RAC_partner_invites(partner_id);

CREATE INDEX IF NOT EXISTS idx_partner_invites_lead
  ON RAC_partner_invites(lead_id);

-- +goose Down
DROP TABLE IF EXISTS RAC_partner_invites;
DROP TABLE IF EXISTS RAC_partner_leads;
DROP TABLE IF EXISTS RAC_partners;
