-- +goose Up
-- Migration 073: Make all service types tenant-scoped
-- 1. Drop leftover global unique constraints (migration 024 used wrong names).
-- 2. Copy global (NULL org) service types into every existing org.
-- 3. Seed "Algemeen" for every org.
-- 4. Delete orphan global rows.
-- 5. Make organization_id NOT NULL.

BEGIN;

-- Step 1: Drop global unique constraints that block per-org duplicates.
ALTER TABLE rac_service_types DROP CONSTRAINT IF EXISTS rac_service_types_name_key;
ALTER TABLE rac_service_types DROP CONSTRAINT IF EXISTS rac_service_types_slug_key;

-- Step 2: Copy each global service type into every org that doesn't already have it.
INSERT INTO rac_service_types (id, organization_id, name, slug, description, icon, color, is_active)
SELECT
    gen_random_uuid(),
    o.id,
    g.name,
    g.slug,
    g.description,
    g.icon,
    g.color,
    g.is_active
FROM rac_service_types g
CROSS JOIN rac_organizations o
WHERE g.organization_id IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM rac_service_types existing
      WHERE existing.organization_id = o.id AND existing.slug = g.slug
  );

-- Step 3: Seed "Algemeen" for every org that doesn't already have it.
INSERT INTO rac_service_types (id, organization_id, name, slug, description, icon, color, is_active)
SELECT
    gen_random_uuid(),
    o.id,
    'Algemeen',
    'algemeen',
    'Algemene aanvragen en niet-gecategoriseerde verzoeken',
    'inbox',
    '#9CA3AF',
    true
FROM rac_organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM rac_service_types st
    WHERE st.organization_id = o.id AND st.slug = 'algemeen'
);

-- Step 4: Delete global (NULL org) service types — they are now copied into each org.
DELETE FROM rac_service_types WHERE organization_id IS NULL;

-- Step 5: Make organization_id NOT NULL so no global types can be created.
ALTER TABLE rac_service_types ALTER COLUMN organization_id SET NOT NULL;

COMMIT;

-- +goose Down
BEGIN;

ALTER TABLE rac_service_types ALTER COLUMN organization_id DROP NOT NULL;

-- Collapse tenant-scoped rows back into a single global row per slug.
WITH ranked AS (
    SELECT
        id,
        slug,
        row_number() OVER (PARTITION BY slug ORDER BY created_at ASC NULLS LAST, id) AS rn
    FROM rac_service_types
)
UPDATE rac_service_types st
SET organization_id = NULL
FROM ranked r
WHERE st.id = r.id AND r.rn = 1;

DELETE FROM rac_service_types st
USING (
    SELECT id
    FROM (
        SELECT
            id,
            row_number() OVER (PARTITION BY slug ORDER BY created_at ASC NULLS LAST, id) AS rn
        FROM rac_service_types
    ) t
    WHERE t.rn > 1
) d
WHERE st.id = d.id;

-- Remove per-org seeding from the up migration.
UPDATE rac_lead_services ls
SET service_type_id = COALESCE(
    (SELECT id FROM rac_service_types WHERE slug = 'windows' AND organization_id IS NULL LIMIT 1),
    (SELECT id FROM rac_service_types WHERE organization_id IS NULL ORDER BY created_at ASC NULLS LAST, id LIMIT 1)
)
WHERE ls.service_type_id IN (SELECT id FROM rac_service_types WHERE slug = 'algemeen');

DELETE FROM rac_service_types WHERE slug = 'algemeen';
ALTER TABLE rac_service_types DROP CONSTRAINT IF EXISTS rac_service_types_name_key;
ALTER TABLE rac_service_types DROP CONSTRAINT IF EXISTS rac_service_types_slug_key;
ALTER TABLE rac_service_types ADD CONSTRAINT rac_service_types_name_key UNIQUE (name);
ALTER TABLE rac_service_types ADD CONSTRAINT rac_service_types_slug_key UNIQUE (slug);
COMMIT;
