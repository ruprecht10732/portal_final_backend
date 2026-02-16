package service

import (
	"context"

	"portal_final_backend/internal/identity/repository"

	"github.com/google/uuid"
)

const (
	defaultWorkflowKey      = "default"
	defaultWorkflowName     = "Default workflow"
	defaultWorkflowRuleName = "Default workflow"
	defaultWorkflowPriority = 1000000
)

func (s *Service) SeedDefaultWorkflow(ctx context.Context, organizationID uuid.UUID) error {
	workflow := repository.WorkflowUpsert{
		WorkflowKey: defaultWorkflowKey,
		Name:        defaultWorkflowName,
		Enabled:     true,
		Steps:       buildDefaultWorkflowSteps(),
	}

	return s.repo.EnsureDefaultWorkflowSeed(
		ctx,
		organizationID,
		workflow,
		defaultWorkflowRuleName,
		defaultWorkflowPriority,
	)
}

func buildDefaultWorkflowSteps() []repository.WorkflowStepUpsert {
	leadRecipients := map[string]any{"includeLeadContact": true}
	partnerRecipients := map[string]any{"includePartner": true}

	return []repository.WorkflowStepUpsert{
		newDefaultWorkflowStep(1, "lead_welcome", "whatsapp", "lead", leadRecipients, nil,
			"Hallo {{lead.name}}, welkom bij {{org.name}}. We hebben je aanvraag ontvangen en nemen snel contact op."),
		newDefaultWorkflowStep(2, "lead_welcome", "email", "lead", leadRecipients,
			stringPtr("Welkom bij {{org.name}}"),
			"Hallo {{lead.name}},\n\nWelkom bij {{org.name}}. We hebben je aanvraag ontvangen en nemen snel contact op.\n\nMet vriendelijke groet,\n{{org.name}}"),
		newDefaultWorkflowStep(3, "quote_sent", "whatsapp", "lead", leadRecipients, nil,
			"Hallo {{lead.name}}, je offerte {{quote.number}} staat klaar. Bekijk deze hier: {{quote.previewUrl}}"),
		newDefaultWorkflowStep(4, "quote_sent", "email", "lead", leadRecipients,
			stringPtr("Je offerte {{quote.number}} staat klaar"),
			"Hallo {{lead.name}},\n\nJe offerte {{quote.number}} staat klaar. Je kunt deze bekijken via {{quote.previewUrl}}.\n\nMet vriendelijke groet,\n{{org.name}}"),
		newDefaultWorkflowStep(5, "quote_accepted", "whatsapp", "lead", leadRecipients, nil,
			"Bedankt {{lead.name}}! Je hebt offerte {{quote.number}} geaccepteerd. Je downloadlink: {{links.download}}"),
		newDefaultWorkflowStep(6, "quote_accepted", "email", "lead", leadRecipients,
			stringPtr("Bevestiging offerte {{quote.number}}"),
			"Hallo {{lead.name}},\n\nBedankt voor je akkoord op offerte {{quote.number}}. Je kunt de documenten downloaden via {{links.download}}.\n\nMet vriendelijke groet,\n{{org.name}}"),
		newDefaultWorkflowStep(7, "quote_accepted", "email", "partner", partnerRecipients,
			stringPtr("Offerte {{quote.number}} is geaccepteerd"),
			"Hallo {{partner.name}},\n\nOfferte {{quote.number}} voor {{lead.name}} is geaccepteerd.\n\nMet vriendelijke groet,\n{{org.name}}"),
		newDefaultWorkflowStep(8, "quote_rejected", "whatsapp", "lead", leadRecipients, nil,
			"Hallo {{lead.name}}, jammer dat offerte {{quote.number}} niet is doorgegaan. Reden: {{quote.reason}}"),
		newDefaultWorkflowStep(9, "quote_rejected", "email", "lead", leadRecipients,
			stringPtr("Offerte {{quote.number}} niet doorgegaan"),
			"Hallo {{lead.name}},\n\nJammer dat offerte {{quote.number}} niet is doorgegaan. Reden: {{quote.reason}}.\n\nMet vriendelijke groet,\n{{org.name}}"),
		newDefaultWorkflowStep(10, "appointment_created", "whatsapp", "lead", leadRecipients, nil,
			"Hallo {{lead.name}}, je afspraak staat gepland op {{appointment.date}} om {{appointment.time}}."),
		newDefaultWorkflowStep(11, "appointment_created", "email", "lead", leadRecipients,
			stringPtr("Afspraak bevestigd op {{appointment.date}}"),
			"Hallo {{lead.name}},\n\nJe afspraak staat gepland op {{appointment.date}} om {{appointment.time}}.\n\nMet vriendelijke groet,\n{{org.name}}"),
		newDefaultWorkflowStep(12, "appointment_reminder", "whatsapp", "lead", leadRecipients, nil,
			"Hallo {{lead.name}}, herinnering: je afspraak is op {{appointment.date}} om {{appointment.time}}."),
		newDefaultWorkflowStep(13, "appointment_reminder", "email", "lead", leadRecipients,
			stringPtr("Herinnering afspraak {{appointment.date}}"),
			"Hallo {{lead.name}},\n\nHerinnering: je afspraak is op {{appointment.date}} om {{appointment.time}}.\n\nMet vriendelijke groet,\n{{org.name}}"),
		newDefaultWorkflowStep(14, "partner_offer_created", "whatsapp", "partner", partnerRecipients, nil,
			"Hallo {{partner.name}}, er staat een nieuw werkaanbod voor je klaar. Bekijk het aanbod via {{links.accept}}."),
		newDefaultWorkflowStep(15, "partner_offer_created", "email", "partner", partnerRecipients,
			stringPtr("Nieuw werkaanbod beschikbaar"),
			"Hallo {{partner.name}},\n\nEr staat een nieuw werkaanbod voor je klaar. Bekijk het aanbod via {{links.accept}}.\n\nMet vriendelijke groet,\n{{org.name}}"),
	}
}

func newDefaultWorkflowStep(
	stepOrder int,
	trigger string,
	channel string,
	audience string,
	recipientConfig map[string]any,
	templateSubject *string,
	templateBody string,
) repository.WorkflowStepUpsert {
	return repository.WorkflowStepUpsert{
		Trigger:         trigger,
		Channel:         channel,
		Audience:        audience,
		Action:          "send_message",
		StepOrder:       stepOrder,
		DelayMinutes:    0,
		Enabled:         true,
		RecipientConfig: recipientConfig,
		TemplateSubject: templateSubject,
		TemplateBody:    stringPtr(templateBody),
		StopOnReply:     false,
	}
}

func stringPtr(value string) *string {
	return &value
}
