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

// AccessDifficulty defines accessibility difficulty for visit reports
type AccessDifficulty string

const (
	AccessDifficultyLow    AccessDifficulty = "Low"
	AccessDifficultyMedium AccessDifficulty = "Medium"
	AccessDifficultyHigh   AccessDifficulty = "High"
)

// CreateAppointmentRequest is the request body for creating an appointment
type CreateAppointmentRequest struct {
	LeadID                *uuid.UUID      `json:"leadId,omitempty"`
	LeadServiceID         *uuid.UUID      `json:"leadServiceId,omitempty"`
	Type                  AppointmentType `json:"type" validate:"required,oneof=lead_visit standalone blocked"`
	Title                 string          `json:"title" validate:"required,min=1,max=200"`
	Description           string          `json:"description,omitempty" validate:"max=2000"`
	Location              string          `json:"location,omitempty" validate:"max=500"`
	MeetingLink           string          `json:"meetingLink,omitempty" validate:"max=500"`
	StartTime             time.Time       `json:"startTime" validate:"required"`
	EndTime               time.Time       `json:"endTime" validate:"required,gtfield=StartTime"`
	AllDay                bool            `json:"allDay"`
	SendConfirmationEmail *bool           `json:"sendConfirmationEmail,omitempty"` // If true, sends confirmation email to lead
}

// UpdateAppointmentRequest is the request body for updating an appointment
type UpdateAppointmentRequest struct {
	Title       *string    `json:"title,omitempty" validate:"omitempty,min=1,max=200"`
	Description *string    `json:"description,omitempty" validate:"omitempty,max=2000"`
	Location    *string    `json:"location,omitempty" validate:"omitempty,max=500"`
	MeetingLink *string    `json:"meetingLink,omitempty" validate:"omitempty,max=500"`
	StartTime   *time.Time `json:"startTime,omitempty"`
	EndTime     *time.Time `json:"endTime,omitempty"`
	AllDay      *bool      `json:"allDay,omitempty"`
}

// UpdateAppointmentStatusRequest is the request body for updating appointment status
type UpdateAppointmentStatusRequest struct {
	Status AppointmentStatus `json:"status" validate:"required,oneof=scheduled completed cancelled no_show"`
}

// ListAppointmentsRequest is the query parameters for listing RAC_appointments
type ListAppointmentsRequest struct {
	UserID    string             `form:"userId"`
	LeadID    string             `form:"leadId"`
	Type      *AppointmentType   `form:"type" validate:"omitempty,oneof=lead_visit standalone blocked"`
	Status    *AppointmentStatus `form:"status" validate:"omitempty,oneof=scheduled completed cancelled no_show"`
	StartFrom string             `form:"startFrom"` // ISO date
	StartTo   string             `form:"startTo"`   // ISO date
	Search    string             `form:"search"`    // Search term for title/location/meeting link
	SortBy    string             `form:"sortBy" validate:"omitempty,oneof=title type status startTime endTime createdAt"`
	SortOrder string             `form:"sortOrder" validate:"omitempty,oneof=asc desc"`
	Page      int                `form:"page" validate:"omitempty,min=1"`
	PageSize  int                `form:"pageSize" validate:"omitempty,min=1,max=100"`
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
	MeetingLink   *string           `json:"meetingLink,omitempty"`
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

// AppointmentListResponse is the paginated response for listing RAC_appointments
type AppointmentListResponse struct {
	Items      []AppointmentResponse `json:"items"`
	Total      int                   `json:"total"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"pageSize"`
	TotalPages int                   `json:"totalPages"`
}

// Visit report DTOs
type UpsertVisitReportRequest struct {
	Measurements     *string           `json:"measurements,omitempty" validate:"omitempty,max=5000"`
	AccessDifficulty *AccessDifficulty `json:"accessDifficulty,omitempty" validate:"omitempty,oneof=Low Medium High"`
	Notes            *string           `json:"notes,omitempty" validate:"omitempty,max=5000"`
}

type AppointmentVisitReportResponse struct {
	AppointmentID    uuid.UUID         `json:"appointmentId"`
	Measurements     *string           `json:"measurements,omitempty"`
	AccessDifficulty *AccessDifficulty `json:"accessDifficulty,omitempty"`
	Notes            *string           `json:"notes,omitempty"`
	CreatedAt        time.Time         `json:"createdAt"`
	UpdatedAt        time.Time         `json:"updatedAt"`
}

// Attachment DTOs
type CreateAppointmentAttachmentRequest struct {
	FileKey     string  `json:"fileKey" validate:"required,min=1,max=500"`
	FileName    string  `json:"fileName" validate:"required,min=1,max=255"`
	ContentType *string `json:"contentType,omitempty" validate:"omitempty,max=200"`
	SizeBytes   *int64  `json:"sizeBytes,omitempty" validate:"omitempty,min=0"`
}

type AppointmentAttachmentResponse struct {
	ID            uuid.UUID `json:"id"`
	AppointmentID uuid.UUID `json:"appointmentId"`
	FileKey       string    `json:"fileKey"`
	FileName      string    `json:"fileName"`
	ContentType   *string   `json:"contentType,omitempty"`
	SizeBytes     *int64    `json:"sizeBytes,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
}

// Availability DTOs
type CreateAvailabilityRuleRequest struct {
	UserID    *uuid.UUID `json:"userId,omitempty"`
	Weekday   int        `json:"weekday" validate:"min=0,max=6"`
	StartTime string     `json:"startTime" validate:"required"`
	EndTime   string     `json:"endTime" validate:"required"`
	Timezone  string     `json:"timezone,omitempty" validate:"omitempty,max=100"`
}

type UpdateAvailabilityRuleRequest struct {
	Weekday   *int    `json:"weekday,omitempty" validate:"omitempty,min=0,max=6"`
	StartTime *string `json:"startTime,omitempty"`
	EndTime   *string `json:"endTime,omitempty"`
	Timezone  *string `json:"timezone,omitempty" validate:"omitempty,max=100"`
}

type AvailabilityRuleResponse struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"userId"`
	Weekday   int       `json:"weekday"`
	StartTime string    `json:"startTime"`
	EndTime   string    `json:"endTime"`
	Timezone  string    `json:"timezone"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type CreateAvailabilityOverrideRequest struct {
	UserID      *uuid.UUID `json:"userId,omitempty"`
	Date        string     `json:"date" validate:"required"`
	IsAvailable bool       `json:"isAvailable"`
	StartTime   *string    `json:"startTime,omitempty"`
	EndTime     *string    `json:"endTime,omitempty"`
	Timezone    string     `json:"timezone,omitempty" validate:"omitempty,max=100"`
}

type UpdateAvailabilityOverrideRequest struct {
	Date        *string `json:"date,omitempty"`
	IsAvailable *bool   `json:"isAvailable,omitempty"`
	StartTime   *string `json:"startTime,omitempty"`
	EndTime     *string `json:"endTime,omitempty"`
	Timezone    *string `json:"timezone,omitempty" validate:"omitempty,max=100"`
}

type AvailabilityOverrideResponse struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"userId"`
	Date        string    `json:"date"`
	IsAvailable bool      `json:"isAvailable"`
	StartTime   *string   `json:"startTime,omitempty"`
	EndTime     *string   `json:"endTime,omitempty"`
	Timezone    string    `json:"timezone"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// GetAvailableSlotsRequest is the query parameters for getting available slots
type GetAvailableSlotsRequest struct {
	UserID       string `form:"userId"`
	StartDate    string `form:"startDate" validate:"required"` // ISO date YYYY-MM-DD
	EndDate      string `form:"endDate" validate:"required"`   // ISO date YYYY-MM-DD
	SlotDuration int    `form:"slotDuration"`                  // Duration in minutes (default: 60)
}

// TimeSlot represents a single available time slot
type TimeSlot struct {
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
}

// DaySlots represents available slots for a specific day
type DaySlots struct {
	Date  string     `json:"date"` // ISO date YYYY-MM-DD
	Slots []TimeSlot `json:"slots"`
}

// AvailableSlotsResponse is the response for available slots query
type AvailableSlotsResponse struct {
	Days []DaySlots `json:"days"`
}
