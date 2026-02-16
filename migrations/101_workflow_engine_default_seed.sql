-- +goose Up
WITH ranked_default_rules AS (
  SELECT
    id,
    ROW_NUMBER() OVER (
      PARTITION BY organization_id
      ORDER BY created_at ASC, id ASC
    ) AS rn
  FROM RAC_workflow_assignment_rules
  WHERE lead_source IS NULL
    AND lead_service_type IS NULL
    AND pipeline_stage IS NULL
)
DELETE FROM RAC_workflow_assignment_rules r
USING ranked_default_rules rr
WHERE r.id = rr.id
  AND rr.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_assignment_rules_one_default_per_org
  ON RAC_workflow_assignment_rules(organization_id)
  WHERE lead_source IS NULL
    AND lead_service_type IS NULL
    AND pipeline_stage IS NULL;

INSERT INTO RAC_workflows (organization_id, workflow_key, name, enabled)
SELECT
  o.id,
  'default',
  'Default workflow',
  TRUE
FROM RAC_organizations o
WHERE NOT EXISTS (
  SELECT 1
  FROM RAC_workflows w
  WHERE w.organization_id = o.id
    AND w.workflow_key = 'default'
);

INSERT INTO RAC_workflow_steps (
  organization_id,
  workflow_id,
  trigger,
  channel,
  audience,
  action,
  step_order,
  delay_minutes,
  enabled,
  recipient_config,
  template_subject,
  template_body,
  stop_on_reply
)
SELECT
  w.organization_id,
  w.id,
  s.trigger,
  s.channel,
  s.audience,
  'send_message',
  s.step_order,
  0,
  TRUE,
  s.recipient_config,
  s.template_subject,
  s.template_body,
  FALSE
FROM RAC_workflows w
CROSS JOIN (
  VALUES
    ('lead_welcome', 'whatsapp', 'lead', 1, '{"includeLeadContact": true}'::jsonb, NULL::text, 'Hallo {{lead.name}}, welkom bij {{org.name}}. We hebben je aanvraag ontvangen en nemen snel contact op.'::text),
    ('lead_welcome', 'email', 'lead', 2, '{"includeLeadContact": true}'::jsonb, 'Welkom bij {{org.name}}'::text, 'Hallo {{lead.name}},\n\nWelkom bij {{org.name}}. We hebben je aanvraag ontvangen en nemen snel contact op.\n\nMet vriendelijke groet,\n{{org.name}}'::text),
    ('quote_sent', 'whatsapp', 'lead', 3, '{"includeLeadContact": true}'::jsonb, NULL::text, 'Hallo {{lead.name}}, je offerte {{quote.number}} staat klaar. Bekijk deze hier: {{quote.previewUrl}}'::text),
    ('quote_sent', 'email', 'lead', 4, '{"includeLeadContact": true}'::jsonb, 'Je offerte {{quote.number}} staat klaar'::text, 'Hallo {{lead.name}},\n\nJe offerte {{quote.number}} staat klaar. Je kunt deze bekijken via {{quote.previewUrl}}.\n\nMet vriendelijke groet,\n{{org.name}}'::text),
    ('quote_accepted', 'whatsapp', 'lead', 5, '{"includeLeadContact": true}'::jsonb, NULL::text, 'Bedankt {{lead.name}}! Je hebt offerte {{quote.number}} geaccepteerd. Je downloadlink: {{links.download}}'::text),
    ('quote_accepted', 'email', 'lead', 6, '{"includeLeadContact": true}'::jsonb, 'Bevestiging offerte {{quote.number}}'::text, 'Hallo {{lead.name}},\n\nBedankt voor je akkoord op offerte {{quote.number}}. Je kunt de documenten downloaden via {{links.download}}.\n\nMet vriendelijke groet,\n{{org.name}}'::text),
    ('quote_accepted', 'email', 'partner', 7, '{"includePartner": true}'::jsonb, 'Offerte {{quote.number}} is geaccepteerd'::text, 'Hallo {{partner.name}},\n\nOfferte {{quote.number}} voor {{lead.name}} is geaccepteerd.\n\nMet vriendelijke groet,\n{{org.name}}'::text),
    ('quote_rejected', 'whatsapp', 'lead', 8, '{"includeLeadContact": true}'::jsonb, NULL::text, 'Hallo {{lead.name}}, jammer dat offerte {{quote.number}} niet is doorgegaan. Reden: {{quote.reason}}'::text),
    ('quote_rejected', 'email', 'lead', 9, '{"includeLeadContact": true}'::jsonb, 'Offerte {{quote.number}} niet doorgegaan'::text, 'Hallo {{lead.name}},\n\nJammer dat offerte {{quote.number}} niet is doorgegaan. Reden: {{quote.reason}}.\n\nMet vriendelijke groet,\n{{org.name}}'::text),
    ('appointment_created', 'whatsapp', 'lead', 10, '{"includeLeadContact": true}'::jsonb, NULL::text, 'Hallo {{lead.name}}, je afspraak staat gepland op {{appointment.date}} om {{appointment.time}}.'::text),
    ('appointment_created', 'email', 'lead', 11, '{"includeLeadContact": true}'::jsonb, 'Afspraak bevestigd op {{appointment.date}}'::text, 'Hallo {{lead.name}},\n\nJe afspraak staat gepland op {{appointment.date}} om {{appointment.time}}.\n\nMet vriendelijke groet,\n{{org.name}}'::text),
    ('appointment_reminder', 'whatsapp', 'lead', 12, '{"includeLeadContact": true}'::jsonb, NULL::text, 'Hallo {{lead.name}}, herinnering: je afspraak is op {{appointment.date}} om {{appointment.time}}.'::text),
    ('appointment_reminder', 'email', 'lead', 13, '{"includeLeadContact": true}'::jsonb, 'Herinnering afspraak {{appointment.date}}'::text, 'Hallo {{lead.name}},\n\nHerinnering: je afspraak is op {{appointment.date}} om {{appointment.time}}.\n\nMet vriendelijke groet,\n{{org.name}}'::text),
    ('partner_offer_created', 'whatsapp', 'partner', 14, '{"includePartner": true}'::jsonb, NULL::text, 'Hallo {{partner.name}}, er staat een nieuw werkaanbod voor je klaar. Bekijk het aanbod via {{links.accept}}.'::text),
    ('partner_offer_created', 'email', 'partner', 15, '{"includePartner": true}'::jsonb, 'Nieuw werkaanbod beschikbaar'::text, 'Hallo {{partner.name}},\n\nEr staat een nieuw werkaanbod voor je klaar. Bekijk het aanbod via {{links.accept}}.\n\nMet vriendelijke groet,\n{{org.name}}'::text)
) AS s(trigger, channel, audience, step_order, recipient_config, template_subject, template_body)
WHERE w.workflow_key = 'default'
ON CONFLICT (workflow_id, trigger, channel, step_order) DO UPDATE
SET
  audience = EXCLUDED.audience,
  action = EXCLUDED.action,
  delay_minutes = EXCLUDED.delay_minutes,
  enabled = EXCLUDED.enabled,
  recipient_config = EXCLUDED.recipient_config,
  template_subject = EXCLUDED.template_subject,
  template_body = EXCLUDED.template_body,
  stop_on_reply = EXCLUDED.stop_on_reply,
  updated_at = now();

INSERT INTO RAC_workflow_assignment_rules (
  organization_id,
  workflow_id,
  name,
  enabled,
  priority,
  lead_source,
  lead_service_type,
  pipeline_stage
)
SELECT
  w.organization_id,
  w.id,
  'Default workflow',
  TRUE,
  1000000,
  NULL,
  NULL,
  NULL
FROM RAC_workflows w
WHERE w.workflow_key = 'default'
  AND NOT EXISTS (
    SELECT 1
    FROM RAC_workflow_assignment_rules r
    WHERE r.organization_id = w.organization_id
      AND r.lead_source IS NULL
      AND r.lead_service_type IS NULL
      AND r.pipeline_stage IS NULL
  );

-- +goose Down
DROP INDEX IF EXISTS idx_workflow_assignment_rules_one_default_per_org;

DELETE FROM RAC_workflow_assignment_rules r
USING RAC_workflows w
WHERE r.workflow_id = w.id
  AND w.workflow_key = 'default'
  AND r.lead_source IS NULL
  AND r.lead_service_type IS NULL
  AND r.pipeline_stage IS NULL;

DELETE FROM RAC_workflows
WHERE workflow_key = 'default';
