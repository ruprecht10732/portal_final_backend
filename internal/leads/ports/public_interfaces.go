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
	ID        uuid.UUID `json:"id"`
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
}

// OrganizationPublicViewer allows the lead portal to fetch organization contact info.
type OrganizationPublicViewer interface {
	GetPublicPhone(ctx context.Context, organizationID uuid.UUID) (string, error)
}

// PublicTimeSlot represents a single public-facing available time slot.
type PublicTimeSlot struct {
	UserID    uuid.UUID `json:"userId"`
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
}

// PublicDaySlots groups available slots by date.
type PublicDaySlots struct {
	Date  string           `json:"date"`
	Slots []PublicTimeSlot `json:"slots"`
}

// PublicAvailableSlotsResponse is returned by the public availability endpoint.
type PublicAvailableSlotsResponse struct {
	Days []PublicDaySlots `json:"days"`
}

// QuotePublicViewer allows the leads domain to fetch active quotes.
type QuotePublicViewer interface {
	GetActiveQuote(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (*PublicQuoteSummary, error)
}

// AppointmentPublicViewer allows the leads domain to fetch scheduled visits.
type AppointmentPublicViewer interface {
	GetUpcomingVisit(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (*PublicAppointmentSummary, error)
	GetPendingVisit(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (*PublicAppointmentSummary, error)
	ListVisits(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) ([]PublicAppointmentSummary, error)
}

// AppointmentSlotProvider exposes availability and booking for the public portal.
type AppointmentSlotProvider interface {
	HasAvailabilityRules(ctx context.Context, organizationID uuid.UUID) (bool, error)
	GetAvailableSlots(ctx context.Context, organizationID uuid.UUID, startDate string, endDate string, slotDuration int) (*PublicAvailableSlotsResponse, error)
	CreateRequestedAppointment(ctx context.Context, userID uuid.UUID, organizationID uuid.UUID, leadID uuid.UUID, leadServiceID uuid.UUID, startTime time.Time, endTime time.Time) (*PublicAppointmentSummary, error)
}
