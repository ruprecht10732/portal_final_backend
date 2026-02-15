-- +goose Up
-- Backfill historical services that were marked Quote_Sent while all quotes are still Draft.
UPDATE RAC_lead_services ls
SET pipeline_stage = 'Quote_Draft'
WHERE ls.pipeline_stage = 'Quote_Sent'
  AND EXISTS (
    SELECT 1
    FROM RAC_quotes q
    WHERE q.lead_service_id = ls.id
      AND q.organization_id = ls.organization_id
  )
  AND NOT EXISTS (
    SELECT 1
    FROM RAC_quotes q
    WHERE q.lead_service_id = ls.id
      AND q.organization_id = ls.organization_id
      AND q.status <> 'Draft'
  );

-- +goose Down
SELECT 1;
