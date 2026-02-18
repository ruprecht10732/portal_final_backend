-- +goose Up
-- Store Google Ads export password encrypted so admins can re-view it in the UI without rotating credentials.

ALTER TABLE RAC_google_ads_export_credentials
  ADD COLUMN IF NOT EXISTS password_encrypted TEXT;

-- +goose Down
ALTER TABLE RAC_google_ads_export_credentials
  DROP COLUMN IF EXISTS password_encrypted;
