package waagent

import (
	"context"
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

// ToolHandler implements the function-calling tool handlers.
type ToolHandler struct {
	quotesReader       QuotesReader
	appointmentsReader AppointmentsReader
}

// HandleGetPendingQuotes retrieves quotes scoped to the org from context.
func (h *ToolHandler) HandleGetPendingQuotes(_ tool.Context, orgID uuid.UUID, input GetPendingQuotesInput) (GetPendingQuotesOutput, error) {
	var status *string
	if input.Status != "" {
		status = &input.Status
	}

	quotes, err := h.quotesReader.ListQuotesByOrganization(context.Background(), orgID, status)
	if err != nil {
		return GetPendingQuotesOutput{}, err
	}

	return GetPendingQuotesOutput{
		Quotes: quotes,
		Count:  len(quotes),
	}, nil
}

// HandleGetAppointments retrieves appointments scoped to the org from context.
func (h *ToolHandler) HandleGetAppointments(_ tool.Context, orgID uuid.UUID, input GetAppointmentsInput) (GetAppointmentsOutput, error) {
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

	return GetAppointmentsOutput{
		Appointments: appointments,
		Count:        len(appointments),
	}, nil
}
