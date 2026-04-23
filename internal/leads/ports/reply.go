package ports

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────
// Shared reply types
// ──────────────────────────────────────────────────

type ReplySuggestionScenario string

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
	ReplySuggestionScenarioStaleFollowUp       ReplySuggestionScenario = "stale_follow_up"
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
		ReplySuggestionScenarioComplaintRecovery,
		ReplySuggestionScenarioStaleFollowUp:
		return ReplySuggestionScenario(value)
	default:
		return ReplySuggestionScenarioGeneric
	}
}

func (s ReplySuggestionScenario) IsGeneric() bool {
	return s == "" || s == ReplySuggestionScenarioGeneric
}

type ReplySuggestionDraft struct {
	Text              string
	EffectiveScenario ReplySuggestionScenario
}

// ReplyUserProfile is the minimal user data needed for reply context.
type ReplyUserProfile struct {
	ID        uuid.UUID
	Email     string
	FirstName *string
	LastName  *string
}

// ReplyUserReader provides user context for reply generation.
type ReplyUserReader interface {
	GetUserProfile(ctx context.Context, userID uuid.UUID) (*ReplyUserProfile, error)
}

// ReplyQuoteReader provides quote context for reply generation.
type ReplyQuoteReader interface {
	GetAcceptedQuote(ctx context.Context, leadServiceID uuid.UUID, organizationID uuid.UUID) (*PublicQuoteSummary, error)
}

// ──────────────────────────────────────────────────
// Email reply
// ──────────────────────────────────────────────────

var ErrEmailReplyLeadContextUnavailable = errors.New("email reply lead context unavailable")

type EmailReplyFeedback struct {
	AIReply    string
	HumanReply string
	CreatedAt  time.Time
}

type EmailReplyExample struct {
	CustomerMessage string
	Reply           string
	CreatedAt       time.Time
}

type EmailReplyInput struct {
	OrganizationID  uuid.UUID
	RequesterUserID uuid.UUID
	LeadID          *uuid.UUID
	LeadServiceID   *uuid.UUID
	Scenario        ReplySuggestionScenario
	ScenarioNotes   string
	CustomerEmail   string
	CustomerName    string
	Subject         string
	MessageBody     string
	Feedback        []EmailReplyFeedback
	Examples        []EmailReplyExample
}

type EmailReplyGenerator interface {
	SuggestEmailReply(ctx context.Context, input EmailReplyInput) (ReplySuggestionDraft, error)
}

// ──────────────────────────────────────────────────
// WhatsApp reply
// ──────────────────────────────────────────────────

type WhatsAppReplyMessage struct {
	Direction string
	Body      string
	CreatedAt time.Time
}

type WhatsAppReplyExample struct {
	CustomerMessage string
	Reply           string
	CreatedAt       time.Time
}

type WhatsAppReplyFeedback struct {
	AIReply    string
	HumanReply string
	CreatedAt  time.Time
}

type WhatsAppReplyInput struct {
	OrganizationID  uuid.UUID
	RequesterUserID uuid.UUID
	LeadID          *uuid.UUID
	ConversationID  uuid.UUID
	Scenario        ReplySuggestionScenario
	ScenarioNotes   string
	PhoneNumber     string
	DisplayName     string
	Messages        []WhatsAppReplyMessage
	Examples        []WhatsAppReplyExample
	Feedback        []WhatsAppReplyFeedback
}

type WhatsAppReplyGenerator interface {
	SuggestWhatsAppReply(ctx context.Context, input WhatsAppReplyInput) (ReplySuggestionDraft, error)
}
