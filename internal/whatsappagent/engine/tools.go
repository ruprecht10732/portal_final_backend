package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/adk/tool"
)

// GetQuotesInput — no organization field (compile-time safety).
type GetQuotesInput struct {
	Status string `json:"status,omitempty"`
}

// GetQuotesOutput is the tool output returned to the LLM.
type GetQuotesOutput struct {
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
	RememberQuotes(orgID, phoneKey string, quotes []QuoteSummary)
	RememberAppointments(orgID, phoneKey string, appointments []AppointmentSummary)
	Clear(orgID, phoneKey string)
}

// ToolHandler implements the function-calling tool handlers.
type ToolHandler struct {
	quotesReader                 QuotesReader
	appointmentsReader           AppointmentsReader
	leadSearchReader             LeadSearchReader
	leadHintStore                LeadHintStore
	leadDetailsReader            LeadDetailsReader
	navigationLinkReader         NavigationLinkReader
	catalogSearchReader          CatalogSearchReader
	leadMutationWriter           LeadMutationWriter
	taskWriter                   TaskWriter
	quoteWorkflowWriter          QuoteWorkflowWriter
	currentInboundPhotoAttacher  CurrentInboundPhotoAttacher
	sender                       *Sender
	visitSlotReader              VisitSlotReader
	visitMutationWriter          VisitMutationWriter
	partnerJobReader             PartnerJobReader
	appointmentVisitReportWriter AppointmentVisitReportWriter
	appointmentStatusWriter      AppointmentStatusWriter
}

// HandleGetQuotes retrieves quotes scoped to the org from context.
func (h *ToolHandler) HandleGetQuotes(ctx tool.Context, orgID uuid.UUID, input GetQuotesInput) (GetQuotesOutput, error) {
	var status *string
	if input.Status != "" {
		normalized := normalizeQuoteStatusFilter(input.Status)
		status = &normalized
	}

	quotes, err := h.quotesReader.ListQuotesByOrganization(context.Background(), orgID, status)
	if err != nil {
		return GetQuotesOutput{}, err
	}
	h.recordLeadHintFromQuotes(ctx, orgID, quotes)

	return GetQuotesOutput{
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
		t, err := parseAppointmentDateInput(input.DateFrom)
		if err == nil {
			from = &t
		}
	}
	if input.DateTo != "" {
		t, err := parseAppointmentDateInput(input.DateTo)
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
		return GetAppointmentsOutput{}, fmt.Errorf("ik kan de afspraken nu niet ophalen. probeer het later opnieuw")
	}
	if partnerID, ok := partnerIDFromToolContext(ctx); ok {
		appointments, err = h.filterPartnerAppointments(orgID, partnerID, appointments)
		if err != nil {
			return GetAppointmentsOutput{}, err
		}
	}
	h.recordLeadHintFromAppointments(ctx, orgID, appointments)

	return GetAppointmentsOutput{
		Appointments: appointments,
		Count:        len(appointments),
	}, nil
}

func (h *ToolHandler) filterPartnerAppointments(orgID, partnerID uuid.UUID, appointments []AppointmentSummary) ([]AppointmentSummary, error) {
	if h.partnerJobReader == nil {
		return nil, fmt.Errorf(errPartnerJobReaderNotConfigured)
	}
	jobs, err := h.partnerJobReader.ListPartnerJobs(context.Background(), orgID, partnerID)
	if err != nil {
		return nil, err
	}
	allowedAppointments := make(map[string]struct{}, len(jobs))
	allowedServices := make(map[string]struct{}, len(jobs))
	for _, job := range jobs {
		if strings.TrimSpace(job.AppointmentID) != "" {
			allowedAppointments[strings.TrimSpace(job.AppointmentID)] = struct{}{}
		}
		if strings.TrimSpace(job.LeadServiceID) != "" {
			allowedServices[strings.TrimSpace(job.LeadServiceID)] = struct{}{}
		}
	}
	filtered := make([]AppointmentSummary, 0, len(appointments))
	for _, appointment := range appointments {
		if _, ok := allowedAppointments[strings.TrimSpace(appointment.AppointmentID)]; ok {
			filtered = append(filtered, appointment)
			continue
		}
		if _, ok := allowedServices[strings.TrimSpace(appointment.LeadServiceID)]; ok {
			filtered = append(filtered, appointment)
		}
	}
	return filtered, nil
}

func (h *ToolHandler) recordLeadHintFromQuotes(ctx tool.Context, orgID uuid.UUID, quotes []QuoteSummary) {
	if h == nil || h.leadHintStore == nil {
		return
	}
	if phoneKey, ok := phoneKeyFromToolContext(ctx); ok {
		h.leadHintStore.RememberQuotes(orgID.String(), phoneKey, quotes)
	}
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
	if h == nil || h.leadHintStore == nil {
		return
	}
	if phoneKey, ok := phoneKeyFromToolContext(ctx); ok {
		h.leadHintStore.RememberAppointments(orgID.String(), phoneKey, appointments)
	}
	if len(appointments) != 1 {
		return
	}
	appointment := appointments[0]
	if strings.TrimSpace(appointment.LeadID) == "" {
		return
	}
	h.recordLeadHint(ctx, orgID, appointment.LeadID, "", appointment.LeadServiceID)
}

func parseAppointmentDateInput(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	if parsed, err := time.Parse("2006-01-02", trimmed); err == nil {
		return parsed, nil
	}
	if parsed := parseDateFact(trimmed); parsed != nil {
		return time.Date(parsed.year, time.Month(parsed.month), parsed.day, 0, 0, 0, 0, time.UTC), nil
	}
	return time.Time{}, fmt.Errorf("unsupported date format")
}
