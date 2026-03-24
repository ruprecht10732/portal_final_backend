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
    ('job_completed', 'whatsapp', 'lead', 20, '{"includeLeadContact": true}'::jsonb, NULL::text, 'Hallo {{lead.name}}, het werk is afgerond! We horen graag hoe je de ervaring vond. Laat je review achter via: {{org.reviewUrl}}'::text),
    ('job_completed', 'email', 'lead', 21, '{"includeLeadContact": true}'::jsonb, 'Het werk is afgerond – laat een review achter'::text, E'Hallo {{lead.name}},\n\nHet werk is afgerond! We hopen dat je tevreden bent met het resultaat.\n\nWe zouden het erg waarderen als je een review achterlaat via: {{org.reviewUrl}}\n\nMet vriendelijke groet,\n{{org.name}}'::text)
) AS s(trigger, channel, audience, step_order, recipient_config, template_subject, template_body)
WHERE w.workflow_key = 'default'
ON CONFLICT (workflow_id, trigger, channel, step_order) DO NOTHING;

-- +goose Down
DELETE FROM RAC_workflow_steps WHERE trigger = 'job_completed';
