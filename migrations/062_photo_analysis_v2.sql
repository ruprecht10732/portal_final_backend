-- PhotoAnalyzer v2: forensic analysis with measurements, OCR, discrepancies, and product identification.
-- All columns have defaults so existing rows remain valid.

ALTER TABLE RAC_lead_photo_analyses
    ADD COLUMN IF NOT EXISTS measurements              JSONB DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS needs_onsite_measurement  JSONB DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS discrepancies             JSONB DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS extracted_text             JSONB DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS suggested_search_terms     JSONB DEFAULT '[]'::jsonb;

COMMENT ON COLUMN RAC_lead_photo_analyses.measurements IS
    'Array of {description, value, unit, type, confidence, photoRef} measurement objects';
COMMENT ON COLUMN RAC_lead_photo_analyses.needs_onsite_measurement IS
    'Array of strings: items that need physical on-site measurement';
COMMENT ON COLUMN RAC_lead_photo_analyses.discrepancies IS
    'Array of strings: contradictions between user claims and photo evidence';
COMMENT ON COLUMN RAC_lead_photo_analyses.extracted_text IS
    'Array of strings: OCR text from labels, stickers, screens, type plates';
COMMENT ON COLUMN RAC_lead_photo_analyses.suggested_search_terms IS
    'Array of strings: specific product/material names for catalog search';
