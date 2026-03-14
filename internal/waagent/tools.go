package waagent

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/adk/tool"
)

// GetPendingQuotesInput — no organization field (compile-time safety).
type GetPendingQuotesInput struct {
	Status string `json:"status,omitempty"`
}

// GetPendingQuotesOutput is the tool output returned to the LLM.
type GetPendingQuotesOutput struct {
	Quotes []QuoteSummary `json:"quotes"`
	Count  int            `json:"count"`
}

// GetAppointmentsInput — no organization field (compile-time safety).
type GetAppointmentsInput struct {
	DateFrom string `json:"date_from,omitempty"`
	DateTo   string `json:"date_to,omitempty"`
}

// GetAppointmentsOutput is the tool output returned to the LLM.
type GetAppointmentsOutput struct {
	Appointments []AppointmentSummary `json:"appointments"`
	Count        int                  `json:"count"`
}

type LeadHintStore interface {
	Get(orgID, phoneKey string) (*ConversationLeadHint, bool)
	Set(orgID, phoneKey string, hint ConversationLeadHint)
}

// ToolHandler implements the function-calling tool handlers.
type ToolHandler struct {
	quotesReader                QuotesReader
	appointmentsReader          AppointmentsReader
	leadSearchReader            LeadSearchReader
	leadHintStore               LeadHintStore
	leadDetailsReader           LeadDetailsReader
	navigationLinkReader        NavigationLinkReader
	catalogSearchReader         CatalogSearchReader
	leadMutationWriter          LeadMutationWriter
	quoteWorkflowWriter         QuoteWorkflowWriter
	currentInboundPhotoAttacher CurrentInboundPhotoAttacher
	sender                      *Sender
	visitSlotReader             VisitSlotReader
	visitMutationWriter         VisitMutationWriter
}

// HandleGetPendingQuotes retrieves quotes scoped to the org from context.
func (h *ToolHandler) HandleGetPendingQuotes(ctx tool.Context, orgID uuid.UUID, input GetPendingQuotesInput) (GetPendingQuotesOutput, error) {
	var status *string
	if input.Status != "" {
		normalized := normalizeQuoteStatusFilter(input.Status)
		status = &normalized
	}

	quotes, err := h.quotesReader.ListQuotesByOrganization(context.Background(), orgID, status)
	if err != nil {
		return GetPendingQuotesOutput{}, err
	}
	h.recordLeadHintFromQuotes(ctx, orgID, quotes)

	return GetPendingQuotesOutput{
		Quotes: quotes,
		Count:  len(quotes),
	}, nil
}

func normalizeQuoteStatusFilter(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "draft", "concept", "conceptofferte":
		return "Draft"
	case "sent", "pending", "open", "openstaand", "openstaande", "verstuurd":
		return "Sent"
	case "accepted", "accept", "goedgekeurd", "geaccepteerd", "akkoord":
		return "Accepted"
	case "rejected", "afgewezen", "geweigerd":
		return "Rejected"
	case "expired", "verlopen", "expired_quote":
		return "Expired"
	default:
		if normalized == "" {
			return ""
		}
		return strings.ToUpper(normalized[:1]) + normalized[1:]
	}
}

// HandleGetAppointments retrieves appointments scoped to the org from context.
func (h *ToolHandler) HandleGetAppointments(ctx tool.Context, orgID uuid.UUID, input GetAppointmentsInput) (GetAppointmentsOutput, error) {
	var from, to *time.Time

	if input.DateFrom != "" {
		t, err := time.Parse("2006-01-02", input.DateFrom)
		if err == nil {
			from = &t
		}
	}
	if input.DateTo != "" {
		t, err := time.Parse("2006-01-02", input.DateTo)
		if err == nil {
			to = &t
		}
	}

	// Default: today to 30 days from now
	if from == nil {
		t := time.Now().Truncate(24 * time.Hour)
		from = &t
	}
	if to == nil {
		t := from.Add(30 * 24 * time.Hour)
		to = &t
	}

	appointments, err := h.appointmentsReader.ListAppointmentsByOrganization(context.Background(), orgID, from, to)
	if err != nil {
		return GetAppointmentsOutput{}, err
	}
	h.recordLeadHintFromAppointments(ctx, orgID, appointments)

	return GetAppointmentsOutput{
		Appointments: appointments,
		Count:        len(appointments),
	}, nil
}

func (h *ToolHandler) recordLeadHintFromQuotes(ctx tool.Context, orgID uuid.UUID, quotes []QuoteSummary) {
	if len(quotes) != 1 {
		return
	}
	quote := quotes[0]
	if strings.TrimSpace(quote.LeadID) == "" {
		return
	}
	h.recordLeadHint(ctx, orgID, quote.LeadID, quote.ClientName, quote.LeadServiceID)
}

func (h *ToolHandler) recordLeadHintFromAppointments(ctx tool.Context, orgID uuid.UUID, appointments []AppointmentSummary) {
	if len(appointments) != 1 {
		return
	}
	appointment := appointments[0]
	if strings.TrimSpace(appointment.LeadID) == "" {
		return
	}
	h.recordLeadHint(ctx, orgID, appointment.LeadID, "", appointment.LeadServiceID)
}
