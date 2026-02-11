-- +goose Up
-- Migration: 024_service_types_tenancy.sql
-- Purpose: Scope service types to organizations

-- Add organization_id column (nullable for legacy rows)
ALTER TABLE RAC_service_types ADD COLUMN IF NOT EXISTS organization_id UUID REFERENCES organizations(id);

-- Drop global uniqueness constraints
ALTER TABLE RAC_service_types DROP CONSTRAINT IF EXISTS service_types_name_key;
ALTER TABLE RAC_service_types DROP CONSTRAINT IF EXISTS service_types_slug_key;

-- Add tenant-scoped uniqueness and lookup indexes
CREATE UNIQUE INDEX IF NOT EXISTS idx_service_types_org_name ON RAC_service_types(organization_id, name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_service_types_org_slug ON RAC_service_types(organization_id, slug);
CREATE INDEX IF NOT EXISTS idx_service_types_org ON RAC_service_types(organization_id);
