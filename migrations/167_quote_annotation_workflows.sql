-- +goose Up
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
    ('quote_question_asked', 'whatsapp', 'partner', 16, '{"includePartner": true}'::jsonb, NULL::text, 'Hallo {{partner.name}}, {{lead.name}} heeft een vraag gesteld over offerte {{quote.number}}: "{{annotation.text}}". Bekijk de offerte via {{quote.previewUrl}}.'::text),
    ('quote_question_asked', 'email', 'partner', 17, '{"includePartner": true}'::jsonb, 'Nieuwe vraag over offerte {{quote.number}}'::text, 'Hallo {{partner.name}},\n\n{{lead.name}} heeft een vraag gesteld over offerte {{quote.number}}.\n\nVraag: {{annotation.text}}\n\nBekijk de offerte via {{quote.previewUrl}}.\n\nMet vriendelijke groet,\n{{org.name}}'::text),
    ('quote_question_answered', 'whatsapp', 'lead', 18, '{"includeLeadContact": true}'::jsonb, NULL::text, 'Hallo {{lead.name}}, je vraag over offerte {{quote.number}} is beantwoord: "{{annotation.text}}". Bekijk de offerte via {{quote.previewUrl}}.'::text),
    ('quote_question_answered', 'email', 'lead', 19, '{"includeLeadContact": true}'::jsonb, 'Antwoord op je vraag over offerte {{quote.number}}'::text, 'Hallo {{lead.name}},\n\nJe vraag over offerte {{quote.number}} is beantwoord.\n\nAntwoord: {{annotation.text}}\n\nBekijk de offerte via {{quote.previewUrl}}.\n\nMet vriendelijke groet,\n{{org.name}}'::text)
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

-- +goose Down
DELETE FROM RAC_workflow_steps
WHERE trigger IN ('quote_question_asked', 'quote_question_answered');