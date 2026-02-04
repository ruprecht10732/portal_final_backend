-- +goose Up
ALTER TABLE RAC_lead_ai_analysis ADD COLUMN suggested_whatsapp_message TEXT;

-- +goose Down
ALTER TABLE RAC_lead_ai_analysis DROP COLUMN IF EXISTS suggested_whatsapp_message;
