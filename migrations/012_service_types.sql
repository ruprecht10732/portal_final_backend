-- +goose Up
-- Migration: Create RAC_service_types table for dynamic service management
-- This replaces the hardcoded ServiceType enum (Windows, Insulation, Solar)

CREATE TABLE IF NOT EXISTS RAC_service_types (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    slug TEXT NOT NULL UNIQUE,
    description TEXT,
    icon TEXT,  -- icon name (e.g., "wrench", "flame") or URL
    color TEXT, -- hex color code for UI
    is_active BOOLEAN NOT NULL DEFAULT true,
    display_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Indexes for common queries
CREATE INDEX idx_service_types_slug ON RAC_service_types(slug);
CREATE INDEX idx_service_types_active ON RAC_service_types(is_active) WHERE is_active = true;
CREATE INDEX idx_service_types_display_order ON RAC_service_types(display_order);

-- Seed initial service types (migrate from existing hardcoded values)
INSERT INTO RAC_service_types (name, slug, description, icon, color, display_order) VALUES
    ('Windows', 'windows', 'Window and door installation, replacement, and repairs', 'window', '#3B82F6', 1),
    ('Insulation', 'insulation', 'Home insulation services including roof, wall, and floor insulation', 'home', '#10B981', 2),
    ('Solar', 'solar', 'Solar panel installation and maintenance', 'sun', '#F59E0B', 3),
    -- New service types for the expanded marketplace
    ('Plumbing', 'plumbing', 'Plumbing repairs, installations, and drain services', 'droplet', '#0EA5E9', 4),
    ('HVAC', 'hvac', 'Heating, ventilation, air conditioning, and heat pumps', 'flame', '#EF4444', 5),
    ('Electrical', 'electrical', 'Electrical installations, repairs, and upgrades', 'zap', '#8B5CF6', 6),
    ('Carpentry', 'carpentry', 'Woodwork, doors, floors, and furniture repairs', 'hammer', '#D97706', 7),
    ('Handyman', 'handyman', 'General repairs and small home improvement tasks', 'tool', '#6B7280', 8);

-- Add service_type_id column to RAC_lead_services (keep service_type TEXT temporarily for migration)
ALTER TABLE RAC_lead_services ADD COLUMN IF NOT EXISTS service_type_id UUID REFERENCES RAC_service_types(id);

-- Migrate existing RAC_lead_services data
UPDATE RAC_lead_services ls 
SET service_type_id = st.id
FROM RAC_service_types st 
WHERE LOWER(st.slug) = LOWER(ls.service_type)
  AND ls.service_type_id IS NULL;

-- For any unmapped service types, default to 'handyman'
UPDATE RAC_lead_services 
SET service_type_id = (SELECT id FROM RAC_service_types WHERE slug = 'handyman')
WHERE service_type_id IS NULL;

-- Now make service_type_id required
ALTER TABLE RAC_lead_services ALTER COLUMN service_type_id SET NOT NULL;

-- Drop the old CHECK constraint and TEXT column
ALTER TABLE RAC_lead_services DROP CONSTRAINT IF EXISTS lead_services_service_type_check;
ALTER TABLE RAC_lead_services DROP COLUMN IF EXISTS service_type;
