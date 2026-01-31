package transport

import (
	"time"

	"github.com/google/uuid"
)

// AppointmentType defines the type of appointment
type AppointmentType string

const (
	AppointmentTypeLeadVisit  AppointmentType = "lead_visit"
	AppointmentTypeStandalone AppointmentType = "standalone"
	AppointmentTypeBlocked    AppointmentType = "blocked"
)

// AppointmentStatus defines the status of an appointment
type AppointmentStatus string

const (
	AppointmentStatusScheduled AppointmentStatus = "scheduled"
	AppointmentStatusCompleted AppointmentStatus = "completed"
	AppointmentStatusCancelled AppointmentStatus = "cancelled"
	AppointmentStatusNoShow    AppointmentStatus = "no_show"
)

// CreateAppointmentRequest is the request body for creating an appointment
type CreateAppointmentRequest struct {
	LeadID        *uuid.UUID      `json:"leadId,omitempty"`
	LeadServiceID *uuid.UUID      `json:"leadServiceId,omitempty"`
	Type          AppointmentType `json:"type" validate:"required,oneof=lead_visit standalone blocked"`
	Title         string          `json:"title" validate:"required,min=1,max=200"`
	Description   string          `json:"description,omitempty" validate:"max=2000"`
	Location      string          `json:"location,omitempty" validate:"max=500"`
	StartTime     time.Time       `json:"startTime" validate:"required"`
	EndTime       time.Time       `json:"endTime" validate:"required,gtfield=StartTime"`
	AllDay        bool            `json:"allDay"`
}

// UpdateAppointmentRequest is the request body for updating an appointment
type UpdateAppointmentRequest struct {
	Title       *string    `json:"title,omitempty" validate:"omitempty,min=1,max=200"`
	Description *string    `json:"description,omitempty" validate:"omitempty,max=2000"`
	Location    *string    `json:"location,omitempty" validate:"omitempty,max=500"`
	StartTime   *time.Time `json:"startTime,omitempty"`
	EndTime     *time.Time `json:"endTime,omitempty"`
	AllDay      *bool      `json:"allDay,omitempty"`
}

// UpdateAppointmentStatusRequest is the request body for updating appointment status
type UpdateAppointmentStatusRequest struct {
	Status AppointmentStatus `json:"status" validate:"required,oneof=scheduled completed cancelled no_show"`
}

// ListAppointmentsRequest is the query parameters for listing appointments
type ListAppointmentsRequest struct {
	UserID    *uuid.UUID        `form:"userId"`
	LeadID    *uuid.UUID        `form:"leadId"`
	Type      *AppointmentType  `form:"type" validate:"omitempty,oneof=lead_visit standalone blocked"`
	Status    *AppointmentStatus `form:"status" validate:"omitempty,oneof=scheduled completed cancelled no_show"`
	StartFrom string            `form:"startFrom"` // ISO date
	StartTo   string            `form:"startTo"`   // ISO date
	Page      int               `form:"page" validate:"min=1"`
	PageSize  int               `form:"pageSize" validate:"min=1,max=100"`
}

// AppointmentResponse is the response body for an appointment
type AppointmentResponse struct {
	ID            uuid.UUID         `json:"id"`
	UserID        uuid.UUID         `json:"userId"`
	LeadID        *uuid.UUID        `json:"leadId,omitempty"`
	LeadServiceID *uuid.UUID        `json:"leadServiceId,omitempty"`
	Type          AppointmentType   `json:"type"`
	Title         string            `json:"title"`
	Description   *string           `json:"description,omitempty"`
	Location      *string           `json:"location,omitempty"`
	StartTime     time.Time         `json:"startTime"`
	EndTime       time.Time         `json:"endTime"`
	Status        AppointmentStatus `json:"status"`
	AllDay        bool              `json:"allDay"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
	// Embedded lead info for lead_visit type (populated by service)
	Lead *AppointmentLeadInfo `json:"lead,omitempty"`
}

// AppointmentLeadInfo is embedded lead info for appointment responses
type AppointmentLeadInfo struct {
	ID        uuid.UUID `json:"id"`
	FirstName string    `json:"firstName"`
	LastName  string    `json:"lastName"`
	Phone     string    `json:"phone"`
	Address   string    `json:"address"`
}

// AppointmentListResponse is the paginated response for listing appointments
type AppointmentListResponse struct {
	Items      []AppointmentResponse `json:"items"`
	Total      int                   `json:"total"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"pageSize"`
	TotalPages int                   `json:"totalPages"`
}
