package transport

import (
	"time"

	"github.com/google/uuid"
)

// PublicPreferencesRequest is the DTO for updating lead preferences via the public portal.
type PublicPreferencesRequest struct {
	Budget       string `json:"budget" validate:"omitempty,max=200"`
	Timeframe    string `json:"timeframe" validate:"omitempty,max=200"`
	Availability string `json:"availability" validate:"omitempty,max=2000"`
	ExtraNotes   string `json:"extraNotes" validate:"omitempty,max=2000"`
}

// PublicAvailabilitySlotsQuery is the query DTO for public availability slots.
type PublicAvailabilitySlotsQuery struct {
	StartDate    string `form:"startDate" validate:"required"`
	EndDate      string `form:"endDate" validate:"required"`
	SlotDuration int    `form:"slotDuration"`
}

// PublicAppointmentRequest is the DTO for requesting an appointment via the public portal.
type PublicAppointmentRequest struct {
	UserID    uuid.UUID `json:"userId" validate:"required"`
	StartTime time.Time `json:"startTime" validate:"required"`
	EndTime   time.Time `json:"endTime" validate:"required,gtfield=StartTime"`
}
