package ports

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// PublicQuoteSummary represents what the lead portal needs to know about a quote.
type PublicQuoteSummary struct {
	ID          uuid.UUID
	QuoteNumber string
	Status      string
	PublicToken string
	TotalCents  int64
	PDFFileKey  *string
}

// PublicAppointmentSummary represents what the lead portal needs to know about a visit.
type PublicAppointmentSummary struct {
	ID        uuid.UUID
	StartTime time.Time
	EndTime   time.Time
	Title     string
}

// QuotePublicViewer allows the leads domain to fetch active quotes.
type QuotePublicViewer interface {
	GetActiveQuote(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (*PublicQuoteSummary, error)
}

// AppointmentPublicViewer allows the leads domain to fetch scheduled visits.
type AppointmentPublicViewer interface {
	GetUpcomingVisit(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (*PublicAppointmentSummary, error)
}
