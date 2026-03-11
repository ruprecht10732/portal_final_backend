package ports

type ReplySuggestionScenario string

type ReplySuggestionDraft struct {
	Text              string
	EffectiveScenario ReplySuggestionScenario
}

const (
	ReplySuggestionScenarioGeneric             ReplySuggestionScenario = "generic"
	ReplySuggestionScenarioFollowUp            ReplySuggestionScenario = "follow_up"
	ReplySuggestionScenarioAppointmentReminder ReplySuggestionScenario = "appointment_reminder"
	ReplySuggestionScenarioAppointmentConfirm  ReplySuggestionScenario = "appointment_confirmation"
	ReplySuggestionScenarioRescheduleRequest   ReplySuggestionScenario = "reschedule_request"
	ReplySuggestionScenarioQuoteReminder       ReplySuggestionScenario = "quote_reminder"
	ReplySuggestionScenarioQuoteExpiry         ReplySuggestionScenario = "quote_expiry"
	ReplySuggestionScenarioMissingInformation  ReplySuggestionScenario = "missing_information"
	ReplySuggestionScenarioPhotosOrDocuments   ReplySuggestionScenario = "photos_or_documents"
	ReplySuggestionScenarioPostVisitFollowUp   ReplySuggestionScenario = "post_visit_follow_up"
	ReplySuggestionScenarioAcceptedQuoteNext   ReplySuggestionScenario = "accepted_quote_next_steps"
	ReplySuggestionScenarioDelayUpdate         ReplySuggestionScenario = "delay_update"
	ReplySuggestionScenarioComplaintRecovery   ReplySuggestionScenario = "complaint_recovery"
)

func NormalizeReplySuggestionScenario(value string) ReplySuggestionScenario {
	switch ReplySuggestionScenario(value) {
	case ReplySuggestionScenarioFollowUp,
		ReplySuggestionScenarioAppointmentReminder,
		ReplySuggestionScenarioAppointmentConfirm,
		ReplySuggestionScenarioRescheduleRequest,
		ReplySuggestionScenarioQuoteReminder,
		ReplySuggestionScenarioQuoteExpiry,
		ReplySuggestionScenarioMissingInformation,
		ReplySuggestionScenarioPhotosOrDocuments,
		ReplySuggestionScenarioPostVisitFollowUp,
		ReplySuggestionScenarioAcceptedQuoteNext,
		ReplySuggestionScenarioDelayUpdate,
		ReplySuggestionScenarioComplaintRecovery:
		return ReplySuggestionScenario(value)
	default:
		return ReplySuggestionScenarioGeneric
	}
}

func (s ReplySuggestionScenario) IsGeneric() bool {
	return s == "" || s == ReplySuggestionScenarioGeneric
}
