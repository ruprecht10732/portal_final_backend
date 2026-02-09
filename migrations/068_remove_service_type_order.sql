-- Migration: Remove display_order from RAC_service_types

DROP INDEX IF EXISTS idx_service_types_display_order;

ALTER TABLE RAC_service_types
    DROP COLUMN IF EXISTS display_order;
