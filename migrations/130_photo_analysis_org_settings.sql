-- +goose Up
ALTER TABLE RAC_organization_settings
  ADD COLUMN IF NOT EXISTS photo_analysis_preprocessing_enabled BOOLEAN NOT NULL DEFAULT true,
  ADD COLUMN IF NOT EXISTS photo_analysis_ocr_assist_enabled BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS photo_analysis_ocr_assist_service_types TEXT[] NOT NULL DEFAULT '{}'::text[],
  ADD COLUMN IF NOT EXISTS photo_analysis_lens_correction_enabled BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS photo_analysis_lens_correction_service_types TEXT[] NOT NULL DEFAULT '{}'::text[],
  ADD COLUMN IF NOT EXISTS photo_analysis_perspective_normalization_enabled BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS photo_analysis_perspective_normalization_service_types TEXT[] NOT NULL DEFAULT '{}'::text[];

UPDATE RAC_organization_settings
SET
  photo_analysis_preprocessing_enabled = COALESCE(photo_analysis_preprocessing_enabled, true),
  photo_analysis_ocr_assist_enabled = COALESCE(photo_analysis_ocr_assist_enabled, false),
  photo_analysis_ocr_assist_service_types = COALESCE(photo_analysis_ocr_assist_service_types, '{}'::text[]),
  photo_analysis_lens_correction_enabled = COALESCE(photo_analysis_lens_correction_enabled, false),
  photo_analysis_lens_correction_service_types = COALESCE(photo_analysis_lens_correction_service_types, '{}'::text[]),
  photo_analysis_perspective_normalization_enabled = COALESCE(photo_analysis_perspective_normalization_enabled, false),
  photo_analysis_perspective_normalization_service_types = COALESCE(photo_analysis_perspective_normalization_service_types, '{}'::text[])
WHERE TRUE;

-- +goose Down
ALTER TABLE RAC_organization_settings
  DROP COLUMN IF EXISTS photo_analysis_perspective_normalization_service_types,
  DROP COLUMN IF EXISTS photo_analysis_perspective_normalization_enabled,
  DROP COLUMN IF EXISTS photo_analysis_lens_correction_service_types,
  DROP COLUMN IF EXISTS photo_analysis_lens_correction_enabled,
  DROP COLUMN IF EXISTS photo_analysis_ocr_assist_service_types,
  DROP COLUMN IF EXISTS photo_analysis_ocr_assist_enabled,
  DROP COLUMN IF EXISTS photo_analysis_preprocessing_enabled;