package transport

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// --- Constants & Enums ---

type AppointmentType string

const (
	AppointmentTypeLeadVisit  AppointmentType = "lead_visit"
	AppointmentTypeStandalone AppointmentType = "standalone"
	AppointmentTypeBlocked    AppointmentType = "blocked"
)

type AppointmentStatus string

const (
	AppointmentStatusScheduled AppointmentStatus = "scheduled"
	AppointmentStatusRequested AppointmentStatus = "requested"
	AppointmentStatusCompleted AppointmentStatus = "completed"
	AppointmentStatusCancelled AppointmentStatus = "cancelled"
	AppointmentStatusNoShow    AppointmentStatus = "no_show"
)

type AccessDifficulty string

const (
	AccessDifficultyLow    AccessDifficulty = "Low"
	AccessDifficultyMedium AccessDifficulty = "Medium"
	AccessDifficultyHigh   AccessDifficulty = "High"
)

// --- Appointment DTOs ---

type CreateAppointmentRequest struct {
	// 24-byte fields
	StartTime time.Time `json:"startTime" validate:"required"`
	EndTime   time.Time `json:"endTime" validate:"required,gtfield=StartTime"`

	// 8-byte pointers
	LeadID                *uuid.UUID `json:"leadId,omitempty" validate:"omitempty,uuid"`
	LeadServiceID         *uuid.UUID `json:"leadServiceId,omitempty" validate:"omitempty,uuid"`
	SendConfirmationEmail *bool      `json:"sendConfirmationEmail,omitempty"`

	// 16-byte strings/UUIDs/Header types
	Type          AppointmentType   `json:"type" validate:"required,oneof=lead_visit standalone blocked"`
	Title         string            `json:"title" validate:"required,min=1,max=200"`
	Description   string            `json:"description,omitempty" validate:"max=2000"`
	Location      string            `json:"location,omitempty" validate:"max=500"`
	MeetingLink   string            `json:"meetingLink,omitempty" validate:"max=500"`
	InitialStatus AppointmentStatus `json:"-"` // Internal-only

	// 1-byte fields
	AllDay bool `json:"allDay"`
}

type UpdateAppointmentRequest struct {
	StartTime   *time.Time `json:"startTime,omitempty"`
	EndTime     *time.Time `json:"endTime,omitempty"`
	Title       *string    `json:"title,omitempty" validate:"omitempty,min=1,max=200"`
	Description *string    `json:"description,omitempty" validate:"omitempty,max=2000"`
	Location    *string    `json:"location,omitempty" validate:"omitempty,max=500"`
	MeetingLink *string    `json:"meetingLink,omitempty" validate:"omitempty,max=500"`
	AllDay      *bool      `json:"allDay,omitempty"`
}

type UpdateAppointmentStatusRequest struct {
	Status AppointmentStatus `json:"status" validate:"required,oneof=scheduled requested completed cancelled no_show"`
}

type ListAppointmentsRequest struct {
	// Strings/Enums (16 bytes)
	UserID    string             `form:"userId" validate:"omitempty,uuid"`
	LeadID    string             `form:"leadId" validate:"omitempty,uuid"`
	Type      *AppointmentType   `form:"type" validate:"omitempty,oneof=lead_visit standalone blocked"`
	Status    *AppointmentStatus `form:"status" validate:"omitempty,oneof=scheduled requested completed cancelled no_show"`
	StartFrom string             `form:"startFrom"`
	StartTo   string             `form:"startTo"`
	Search    string             `form:"search"`
	SortBy    string             `form:"sortBy" validate:"omitempty,oneof=title type status startTime endTime createdAt"`
	SortOrder string             `form:"sortOrder" validate:"omitempty,oneof=asc desc"`

	// Ints (8 bytes)
	Page     int `form:"page" validate:"omitempty,min=1"`
	PageSize int `form:"pageSize" validate:"omitempty,min=1,max=100"`
}

type AppointmentResponse struct {
	StartTime     time.Time            `json:"startTime"`
	EndTime       time.Time            `json:"endTime"`
	CreatedAt     time.Time            `json:"createdAt"`
	UpdatedAt     time.Time            `json:"updatedAt"`
	ID            uuid.UUID            `json:"id"`
	UserID        uuid.UUID            `json:"userId"`
	LeadID        *uuid.UUID           `json:"leadId,omitempty"`
	LeadServiceID *uuid.UUID           `json:"leadServiceId,omitempty"`
	Lead          *AppointmentLeadInfo `json:"lead,omitempty"`
	Type          AppointmentType      `json:"type"`
	Title         string               `json:"title"`
	Description   *string              `json:"description,omitempty"`
	Location      *string              `json:"location,omitempty"`
	MeetingLink   *string              `json:"meetingLink,omitempty"`
	Status        AppointmentStatus    `json:"status"`
	AllDay        bool                 `json:"allDay"`
}

type AppointmentLeadInfo struct {
	ID        uuid.UUID `json:"id"`
	FirstName string    `json:"firstName"`
	LastName  string    `json:"lastName"`
	Phone     string    `json:"phone"`
	Address   string    `json:"address"`
}

type AppointmentListResponse struct {
	Items      []AppointmentResponse `json:"items"`
	Total      int                   `json:"total"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"pageSize"`
	TotalPages int                   `json:"totalPages"`
}

// --- Visit Reports ---

type UpsertVisitReportRequest struct {
	MeasurementProducts json.RawMessage   `json:"measurementProducts,omitempty"`
	Measurements        *string           `json:"measurements,omitempty" validate:"omitempty,max=5000"`
	AccessDifficulty    *AccessDifficulty `json:"accessDifficulty,omitempty" validate:"omitempty,oneof=Low Medium High"`
	Notes               *string           `json:"notes,omitempty" validate:"omitempty,max=5000"`
}

type AppointmentVisitReportResponse struct {
	CreatedAt           time.Time         `json:"createdAt"`
	UpdatedAt           time.Time         `json:"updatedAt"`
	AppointmentID       uuid.UUID         `json:"appointmentId"`
	MeasurementProducts json.RawMessage   `json:"measurementProducts,omitempty"`
	Measurements        *string           `json:"measurements,omitempty"`
	AccessDifficulty    *AccessDifficulty `json:"accessDifficulty,omitempty"`
	Notes               *string           `json:"notes,omitempty"`
}

// --- Attachments ---

type PresignedUploadRequest struct {
	SizeBytes   int64  `json:"sizeBytes" validate:"required,min=1,max=104857600"`
	FileName    string `json:"fileName" validate:"required,min=1,max=255"`
	ContentType string `json:"contentType" validate:"required,min=1,max=200"`
}

type PresignedUploadResponse struct {
	ExpiresAt int64  `json:"expiresAt"`
	UploadURL string `json:"uploadUrl"`
	FileKey   string `json:"fileKey"`
}

type PresignedDownloadResponse struct {
	ExpiresAt   int64  `json:"expiresAt"`
	DownloadURL string `json:"downloadUrl"`
}

type CreateAppointmentAttachmentRequest struct {
	SizeBytes   *int64  `json:"sizeBytes,omitempty" validate:"omitempty,min=0"`
	FileKey     string  `json:"fileKey" validate:"required,min=1,max=500"`
	FileName    string  `json:"fileName" validate:"required,min=1,max=255"`
	ContentType *string `json:"contentType,omitempty" validate:"omitempty,max=200"`
}

type AppointmentAttachmentResponse struct {
	CreatedAt     time.Time `json:"createdAt"`
	SizeBytes     *int64    `json:"sizeBytes,omitempty"`
	ID            uuid.UUID `json:"id"`
	AppointmentID uuid.UUID `json:"appointmentId"`
	FileKey       string    `json:"fileKey"`
	FileName      string    `json:"fileName"`
	ContentType   *string   `json:"contentType,omitempty"`
}

// --- Availability ---

type CreateAvailabilityRuleRequest struct {
	UserID    *uuid.UUID `json:"userId,omitempty" validate:"omitempty,uuid"`
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
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"userId"`
	Weekday   int       `json:"weekday"`
	StartTime string    `json:"startTime"`
	EndTime   string    `json:"endTime"`
	Timezone  string    `json:"timezone"`
}

type CreateAvailabilityOverrideRequest struct {
	UserID      *uuid.UUID `json:"userId,omitempty" validate:"omitempty,uuid"`
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
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"userId"`
	Date        string    `json:"date"`
	StartTime   *string   `json:"startTime,omitempty"`
	EndTime     *string   `json:"endTime,omitempty"`
	Timezone    string    `json:"timezone"`
	IsAvailable bool      `json:"isAvailable"`
}

type GetAvailableSlotsRequest struct {
	StartDate    string `form:"startDate" validate:"required"`
	EndDate      string `form:"endDate" validate:"required"`
	UserID       string `form:"userId" validate:"omitempty,uuid"`
	SlotDuration int    `form:"slotDuration"`
}

type TimeSlot struct {
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
}

type DaySlots struct {
	Date  string     `json:"date"`
	Slots []TimeSlot `json:"slots"`
}

type AvailableSlotsResponse struct {
	Days []DaySlots `json:"days"`
}
