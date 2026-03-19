-- +goose Up
UPDATE RAC_workflow_steps
SET
  template_body = 'Hallo {{lead.name}},\n\nBedankt voor je akkoord op offerte {{quote.number}}. De getekende offerte is als pdf-bijlage toegevoegd voor je administratie. Je kunt de offerte ook online bekijken via {{links.view}}.\n\nMet vriendelijke groet,\n{{org.name}}'::text,
  updated_at = now()
WHERE trigger = 'quote_accepted'
  AND channel = 'email'
  AND audience = 'lead'
  AND template_body = 'Hallo {{lead.name}},\n\nBedankt voor je akkoord op offerte {{quote.number}}. Je kunt de documenten downloaden via {{links.download}}.\n\nMet vriendelijke groet,\n{{org.name}}'::text;

-- +goose Down
UPDATE RAC_workflow_steps
SET
  template_body = 'Hallo {{lead.name}},\n\nBedankt voor je akkoord op offerte {{quote.number}}. Je kunt de documenten downloaden via {{links.download}}.\n\nMet vriendelijke groet,\n{{org.name}}'::text,
  updated_at = now()
WHERE trigger = 'quote_accepted'
  AND channel = 'email'
  AND audience = 'lead'
  AND template_body = 'Hallo {{lead.name}},\n\nBedankt voor je akkoord op offerte {{quote.number}}. De getekende offerte is als pdf-bijlage toegevoegd voor je administratie. Je kunt de offerte ook online bekijken via {{links.view}}.\n\nMet vriendelijke groet,\n{{org.name}}'::text;