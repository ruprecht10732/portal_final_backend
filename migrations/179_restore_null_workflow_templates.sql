-- +goose Up
-- Restore workflow step templates that were accidentally set to NULL.
-- Bug: the frontend normalizeTrigger() was missing 'job_completed', causing
-- every workflow-settings save to overwrite job_completed template_body with NULL.
-- Other triggers (e.g. quote_accepted email) may also have been cleared by users
-- who saw garbled template content and emptied the field.
-- Condition: only updates rows where template_body IS NULL, so customized
-- templates are never overwritten.

-- job_completed whatsapp lead
UPDATE RAC_workflow_steps
SET template_body = 'Hallo {{lead.name}}, het werk is afgerond! We horen graag hoe je de ervaring vond. Laat je review achter via: {{org.reviewUrl}}'
WHERE trigger = 'job_completed' AND channel = 'whatsapp' AND audience = 'lead'
  AND template_body IS NULL;

-- job_completed email lead
UPDATE RAC_workflow_steps
SET template_subject = 'Het werk is afgerond – laat een review achter',
    template_body = E'Hallo {{lead.name}},\n\nHet werk is afgerond! We hopen dat je tevreden bent met het resultaat.\n\nWe zouden het erg waarderen als je een review achterlaat via: {{org.reviewUrl}}\n\nMet vriendelijke groet,\n{{org.name}}'
WHERE trigger = 'job_completed' AND channel = 'email' AND audience = 'lead'
  AND template_body IS NULL;

-- quote_accepted whatsapp lead
UPDATE RAC_workflow_steps
SET template_body = 'Bedankt {{lead.name}}! Je hebt offerte {{quote.number}} geaccepteerd. Je downloadlink: {{links.download}}\n\nPlan hier een afspraak in: {{links.scheduling}}'
WHERE trigger = 'quote_accepted' AND channel = 'whatsapp' AND audience = 'lead'
  AND template_body IS NULL;

-- quote_accepted email lead
UPDATE RAC_workflow_steps
SET template_subject = 'Bevestiging offerte {{quote.number}}',
    template_body = E'Hallo {{lead.name}},\n\nBedankt voor je akkoord op offerte {{quote.number}}. De getekende offerte is als pdf-bijlage toegevoegd voor je administratie. Je kunt de offerte ook online bekijken via {{links.view}}.\n\nPlan hier een afspraak in voor de vakman: {{links.scheduling}}\n\nMet vriendelijke groet,\n{{org.name}}'
WHERE trigger = 'quote_accepted' AND channel = 'email' AND audience = 'lead'
  AND template_body IS NULL;

-- quote_accepted email partner (advisor)
UPDATE RAC_workflow_steps
SET template_subject = 'Offerte {{quote.number}} is geaccepteerd',
    template_body = E'Hallo {{partner.name}},\n\nOfferte {{quote.number}} voor {{lead.name}} is geaccepteerd.\n\nMet vriendelijke groet,\n{{org.name}}'
WHERE trigger = 'quote_accepted' AND channel = 'email' AND audience = 'partner'
  AND template_body IS NULL;

-- lead_welcome whatsapp lead
UPDATE RAC_workflow_steps
SET template_body = 'Hallo {{lead.name}}, welkom bij {{org.name}}. We hebben je aanvraag ontvangen en nemen snel contact op.'
WHERE trigger = 'lead_welcome' AND channel = 'whatsapp' AND audience = 'lead'
  AND template_body IS NULL;

-- lead_welcome email lead
UPDATE RAC_workflow_steps
SET template_subject = 'Welkom bij {{org.name}}',
    template_body = E'Hallo {{lead.name}},\n\nWelkom bij {{org.name}}. We hebben je aanvraag ontvangen en nemen snel contact op.\n\nMet vriendelijke groet,\n{{org.name}}'
WHERE trigger = 'lead_welcome' AND channel = 'email' AND audience = 'lead'
  AND template_body IS NULL;

-- quote_sent whatsapp lead
UPDATE RAC_workflow_steps
SET template_body = 'Hallo {{lead.name}}, je offerte {{quote.number}} staat klaar. Bekijk deze hier: {{quote.previewUrl}}'
WHERE trigger = 'quote_sent' AND channel = 'whatsapp' AND audience = 'lead'
  AND template_body IS NULL;

-- quote_sent email lead
UPDATE RAC_workflow_steps
SET template_subject = 'Je offerte {{quote.number}} staat klaar',
    template_body = E'Hallo {{lead.name}},\n\nJe offerte {{quote.number}} staat klaar. Je kunt deze bekijken via {{quote.previewUrl}}.\n\nMet vriendelijke groet,\n{{org.name}}'
WHERE trigger = 'quote_sent' AND channel = 'email' AND audience = 'lead'
  AND template_body IS NULL;

-- quote_rejected whatsapp lead
UPDATE RAC_workflow_steps
SET template_body = 'Hallo {{lead.name}}, jammer dat offerte {{quote.number}} niet is doorgegaan. Reden: {{quote.reason}}'
WHERE trigger = 'quote_rejected' AND channel = 'whatsapp' AND audience = 'lead'
  AND template_body IS NULL;

-- quote_rejected email lead
UPDATE RAC_workflow_steps
SET template_subject = 'Offerte {{quote.number}} niet doorgegaan',
    template_body = E'Hallo {{lead.name}},\n\nJammer dat offerte {{quote.number}} niet is doorgegaan. Reden: {{quote.reason}}.\n\nMet vriendelijke groet,\n{{org.name}}'
WHERE trigger = 'quote_rejected' AND channel = 'email' AND audience = 'lead'
  AND template_body IS NULL;

-- appointment_created whatsapp lead
UPDATE RAC_workflow_steps
SET template_body = 'Hallo {{lead.name}}, je afspraak staat gepland op {{appointment.date}} om {{appointment.time}}.'
WHERE trigger = 'appointment_created' AND channel = 'whatsapp' AND audience = 'lead'
  AND template_body IS NULL;

-- appointment_created email lead
UPDATE RAC_workflow_steps
SET template_subject = 'Afspraak bevestigd op {{appointment.date}}',
    template_body = E'Hallo {{lead.name}},\n\nJe afspraak staat gepland op {{appointment.date}} om {{appointment.time}}.\n\nMet vriendelijke groet,\n{{org.name}}'
WHERE trigger = 'appointment_created' AND channel = 'email' AND audience = 'lead'
  AND template_body IS NULL;

-- appointment_reminder whatsapp lead
UPDATE RAC_workflow_steps
SET template_body = 'Hallo {{lead.name}}, herinnering: je afspraak is op {{appointment.date}} om {{appointment.time}}.'
WHERE trigger = 'appointment_reminder' AND channel = 'whatsapp' AND audience = 'lead'
  AND template_body IS NULL;

-- appointment_reminder email lead
UPDATE RAC_workflow_steps
SET template_subject = 'Herinnering afspraak {{appointment.date}}',
    template_body = E'Hallo {{lead.name}},\n\nHerinnering: je afspraak is op {{appointment.date}} om {{appointment.time}}.\n\nMet vriendelijke groet,\n{{org.name}}'
WHERE trigger = 'appointment_reminder' AND channel = 'email' AND audience = 'lead'
  AND template_body IS NULL;

-- partner_offer_created whatsapp partner
UPDATE RAC_workflow_steps
SET template_body = 'Hallo {{partner.name}}, er staat een nieuw werkaanbod voor je klaar. Bekijk het aanbod via {{links.accept}}.'
WHERE trigger = 'partner_offer_created' AND channel = 'whatsapp' AND audience = 'partner'
  AND template_body IS NULL;

-- partner_offer_created email partner
UPDATE RAC_workflow_steps
SET template_subject = 'Nieuw werkaanbod beschikbaar',
    template_body = E'Hallo {{partner.name}},\n\nEr staat een nieuw werkaanbod voor je klaar. Bekijk het aanbod via {{links.accept}}.\n\nMet vriendelijke groet,\n{{org.name}}'
WHERE trigger = 'partner_offer_created' AND channel = 'email' AND audience = 'partner'
  AND template_body IS NULL;

-- quote_question_asked whatsapp partner
UPDATE RAC_workflow_steps
SET template_body = 'Hallo {{partner.name}}, {{lead.name}} heeft een vraag gesteld over offerte {{quote.number}}: "{{annotation.text}}". Bekijk de offerte via {{quote.previewUrl}}.'
WHERE trigger = 'quote_question_asked' AND channel = 'whatsapp' AND audience = 'partner'
  AND template_body IS NULL;

-- quote_question_asked email partner
UPDATE RAC_workflow_steps
SET template_subject = 'Nieuwe vraag over offerte {{quote.number}}',
    template_body = E'Hallo {{partner.name}},\n\n{{lead.name}} heeft een vraag gesteld over offerte {{quote.number}}.\n\nVraag: {{annotation.text}}\n\nBekijk de offerte via {{quote.previewUrl}}.\n\nMet vriendelijke groet,\n{{org.name}}'
WHERE trigger = 'quote_question_asked' AND channel = 'email' AND audience = 'partner'
  AND template_body IS NULL;

-- quote_question_answered whatsapp lead
UPDATE RAC_workflow_steps
SET template_body = 'Hallo {{lead.name}}, je vraag over offerte {{quote.number}} is beantwoord: "{{annotation.text}}". Bekijk de offerte via {{quote.previewUrl}}.'
WHERE trigger = 'quote_question_answered' AND channel = 'whatsapp' AND audience = 'lead'
  AND template_body IS NULL;

-- quote_question_answered email lead
UPDATE RAC_workflow_steps
SET template_subject = 'Antwoord op je vraag over offerte {{quote.number}}',
    template_body = E'Hallo {{lead.name}},\n\nJe vraag over offerte {{quote.number}} is beantwoord.\n\nAntwoord: {{annotation.text}}\n\nBekijk de offerte via {{quote.previewUrl}}.\n\nMet vriendelijke groet,\n{{org.name}}'
WHERE trigger = 'quote_question_answered' AND channel = 'email' AND audience = 'lead'
  AND template_body IS NULL;

-- +goose Down
-- No-op: cannot distinguish intentionally empty templates from accidentally cleared ones.
