-- +goose Up
-- Migration: Remove display_order from RAC_service_types

DROP INDEX IF EXISTS idx_service_types_display_order;

ALTER TABLE RAC_service_types
    DROP COLUMN IF EXISTS display_order;

-- +goose Down
ALTER TABLE RAC_service_types
    ADD COLUMN IF NOT EXISTS display_order INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_service_types_display_order ON RAC_service_types(display_order);
