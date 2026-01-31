package transport

import (
	"time"

	"github.com/google/uuid"
)

// Enum values
type ConsumerRole string

const (
	ConsumerRoleOwner    ConsumerRole = "Owner"
	ConsumerRoleTenant   ConsumerRole = "Tenant"
	ConsumerRoleLandlord ConsumerRole = "Landlord"
)

type ServiceType string

type LeadStatus string

const (
	LeadStatusNew               LeadStatus = "New"
	LeadStatusAttemptedContact  LeadStatus = "Attempted_Contact"
	LeadStatusScheduled         LeadStatus = "Scheduled"
	LeadStatusSurveyed          LeadStatus = "Surveyed"
	LeadStatusBadLead           LeadStatus = "Bad_Lead"
	LeadStatusNeedsRescheduling LeadStatus = "Needs_Rescheduling"
	LeadStatusClosed            LeadStatus = "Closed"
)

type AccessDifficulty string

const (
	AccessDifficultyLow    AccessDifficulty = "Low"
	AccessDifficultyMedium AccessDifficulty = "Medium"
	AccessDifficultyHigh   AccessDifficulty = "High"
)

// Request DTOs
type CreateLeadRequest struct {
	FirstName    string       `json:"firstName" validate:"required,min=1,max=100"`
	LastName     string       `json:"lastName" validate:"required,min=1,max=100"`
	Phone        string       `json:"phone" validate:"required,min=5,max=20"`
	Email        string       `json:"email,omitempty" validate:"omitempty,email"`
	ConsumerRole ConsumerRole `json:"consumerRole" validate:"required,oneof=Owner Tenant Landlord"`
	Street       string       `json:"street" validate:"required,min=1,max=200"`
	HouseNumber  string       `json:"houseNumber" validate:"required,min=1,max=20"`
	ZipCode      string       `json:"zipCode" validate:"required,min=1,max=20"`
	City         string       `json:"city" validate:"required,min=1,max=100"`
	Latitude     *float64     `json:"latitude,omitempty" validate:"omitempty,gte=-90,lte=90"`
	Longitude    *float64     `json:"longitude,omitempty" validate:"omitempty,gte=-180,lte=180"`
	ServiceType  ServiceType  `json:"serviceType" validate:"required,min=1,max=100"`
	AssigneeID   OptionalUUID `json:"assigneeId,omitempty" validate:"-"`
	ConsumerNote string       `json:"consumerNote,omitempty" validate:"max=2000"`
	Source       string       `json:"source,omitempty" validate:"max=50"`
}

type UpdateLeadRequest struct {
	FirstName    *string       `json:"firstName,omitempty" validate:"omitempty,min=1,max=100"`
	LastName     *string       `json:"lastName,omitempty" validate:"omitempty,min=1,max=100"`
	Phone        *string       `json:"phone,omitempty" validate:"omitempty,min=5,max=20"`
	Email        *string       `json:"email,omitempty" validate:"omitempty,email"`
	ConsumerRole *ConsumerRole `json:"consumerRole,omitempty" validate:"omitempty,oneof=Owner Tenant Landlord"`
	Street       *string       `json:"street,omitempty" validate:"omitempty,min=1,max=200"`
	HouseNumber  *string       `json:"houseNumber,omitempty" validate:"omitempty,min=1,max=20"`
	ZipCode      *string       `json:"zipCode,omitempty" validate:"omitempty,min=1,max=20"`
	City         *string       `json:"city,omitempty" validate:"omitempty,min=1,max=100"`
	Latitude     *float64      `json:"latitude,omitempty" validate:"omitempty,gte=-90,lte=90"`
	Longitude    *float64      `json:"longitude,omitempty" validate:"omitempty,gte=-180,lte=180"`
	AssigneeID   OptionalUUID  `json:"assigneeId,omitempty" validate:"-"`
}

type UpdateServiceStatusRequest struct {
	Status LeadStatus `json:"status" validate:"required,oneof=New Attempted_Contact Scheduled Surveyed Bad_Lead Needs_Rescheduling Closed"`
}

type AddServiceRequest struct {
	ServiceType        ServiceType `json:"serviceType" validate:"required,min=1,max=100"`
	CloseCurrentStatus bool        `json:"closeCurrentStatus"` // If true, auto-close current active service
	ConsumerNote       string      `json:"consumerNote,omitempty" validate:"max=2000"`
	Source             string      `json:"source,omitempty" validate:"max=50"`
}

type UpdateLeadStatusRequest struct {
	Status LeadStatus `json:"status" validate:"required,oneof=New Attempted_Contact Scheduled Surveyed Bad_Lead Needs_Rescheduling Closed"`
}

type AssignLeadRequest struct {
	AssigneeID *uuid.UUID `json:"assigneeId" validate:"omitempty"`
}

type ScheduleVisitRequest struct {
	ServiceID     uuid.UUID  `json:"serviceId" validate:"required"`
	ScheduledDate time.Time  `json:"scheduledDate" validate:"required"`
	ScoutID       *uuid.UUID `json:"scoutId,omitempty"`
	SendInvite    bool       `json:"sendInvite,omitempty"`
}

type CompleteSurveyRequest struct {
	ServiceID        uuid.UUID        `json:"serviceId" validate:"required"`
	Measurements     string           `json:"measurements" validate:"required,min=1,max=500"`
	AccessDifficulty AccessDifficulty `json:"accessDifficulty" validate:"required,oneof=Low Medium High"`
	Notes            string           `json:"notes,omitempty" validate:"max=2000"`
}

type MarkNoShowRequest struct {
	ServiceID uuid.UUID `json:"serviceId" validate:"required"`
	Notes     string    `json:"notes,omitempty" validate:"max=500"`
}

type RescheduleVisitRequest struct {
	ServiceID     uuid.UUID  `json:"serviceId" validate:"required"`
	NoShowNotes   string     `json:"noShowNotes,omitempty" validate:"max=500"`
	MarkAsNoShow  bool       `json:"markAsNoShow"`
	ScheduledDate time.Time  `json:"scheduledDate" validate:"required"`
	ScoutID       *uuid.UUID `json:"scoutId,omitempty"`
	SendInvite    bool       `json:"sendInvite,omitempty"`
}

type BulkDeleteLeadsRequest struct {
	IDs []uuid.UUID `json:"ids" validate:"required,min=1,dive,required"`
}

type ListLeadsRequest struct {
	Status          *LeadStatus   `form:"status" validate:"omitempty,oneof=New Attempted_Contact Scheduled Surveyed Bad_Lead Needs_Rescheduling Closed"`
	ServiceType     *ServiceType  `form:"serviceType" validate:"omitempty,min=1,max=100"`
	Search          string        `form:"search" validate:"max=100"`
	FirstName       string        `form:"firstName" validate:"omitempty,max=100"`
	LastName        string        `form:"lastName" validate:"omitempty,max=100"`
	Phone           string        `form:"phone" validate:"omitempty,max=20"`
	Email           string        `form:"email" validate:"omitempty,max=200"`
	Role            *ConsumerRole `form:"role" validate:"omitempty,oneof=Owner Tenant Landlord"`
	Street          string        `form:"street" validate:"omitempty,max=200"`
	HouseNumber     string        `form:"houseNumber" validate:"omitempty,max=20"`
	ZipCode         string        `form:"zipCode" validate:"omitempty,max=20"`
	City            string        `form:"city" validate:"omitempty,max=100"`
	AssignedAgentID *uuid.UUID    `form:"assignedAgentId" validate:"omitempty"`
	CreatedAtFrom   string        `form:"createdAtFrom" validate:"omitempty"`
	CreatedAtTo     string        `form:"createdAtTo" validate:"omitempty"`
	Page            int           `form:"page" validate:"min=1"`
	PageSize        int           `form:"pageSize" validate:"min=1,max=100"`
	SortBy          string        `form:"sortBy" validate:"omitempty,oneof=createdAt firstName lastName phone email role street houseNumber zipCode city assignedAgentId"`
	SortOrder       string        `form:"sortOrder" validate:"omitempty,oneof=asc desc"`
}

type LeadHeatmapRequest struct {
	StartDate string `form:"startDate"`
	EndDate   string `form:"endDate"`
}

type ActionItemsRequest struct {
	Page     int `form:"page" validate:"min=1"`
	PageSize int `form:"pageSize" validate:"min=1,max=50"`
}

// Response DTOs
type ConsumerResponse struct {
	FirstName string       `json:"firstName"`
	LastName  string       `json:"lastName"`
	Phone     string       `json:"phone"`
	Email     *string      `json:"email,omitempty"`
	Role      ConsumerRole `json:"role"`
}

type AddressResponse struct {
	Street      string   `json:"street"`
	HouseNumber string   `json:"houseNumber"`
	ZipCode     string   `json:"zipCode"`
	City        string   `json:"city"`
	Latitude    *float64 `json:"latitude,omitempty"`
	Longitude   *float64 `json:"longitude,omitempty"`
}

type VisitResponse struct {
	ScheduledDate    *time.Time        `json:"scheduledDate,omitempty"`
	ScoutID          *uuid.UUID        `json:"scoutId,omitempty"`
	Measurements     *string           `json:"measurements,omitempty"`
	AccessDifficulty *AccessDifficulty `json:"accessDifficulty,omitempty"`
	Notes            *string           `json:"notes,omitempty"`
	CompletedAt      *time.Time        `json:"completedAt,omitempty"`
}

type LeadServiceResponse struct {
	ID           uuid.UUID     `json:"id"`
	ServiceType  ServiceType   `json:"serviceType"`
	Status       LeadStatus    `json:"status"`
	ConsumerNote *string       `json:"consumerNote,omitempty"`
	Visit        VisitResponse `json:"visit"`
	CreatedAt    time.Time     `json:"createdAt"`
	UpdatedAt    time.Time     `json:"updatedAt"`
}

type LeadResponse struct {
	ID              uuid.UUID             `json:"id"`
	Consumer        ConsumerResponse      `json:"consumer"`
	Address         AddressResponse       `json:"address"`
	Services        []LeadServiceResponse `json:"services"`
	CurrentService  *LeadServiceResponse  `json:"currentService,omitempty"`
	AggregateStatus *LeadStatus           `json:"aggregateStatus,omitempty"` // Derived from current service
	AssignedAgentID *uuid.UUID            `json:"assignedAgentId,omitempty"`
	ViewedByID      *uuid.UUID            `json:"viewedById,omitempty"`
	ViewedAt        *time.Time            `json:"viewedAt,omitempty"`
	Source          *string               `json:"source,omitempty"`
	CreatedAt       time.Time             `json:"createdAt"`
	UpdatedAt       time.Time             `json:"updatedAt"`
}

type LeadHeatmapPointResponse struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type LeadHeatmapResponse struct {
	Points []LeadHeatmapPointResponse `json:"points"`
}

type ActionItemResponse struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	UrgencyReason *string   `json:"urgencyReason,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	IsUrgent      bool      `json:"isUrgent"`
}

type ActionItemsResponse struct {
	Items    []ActionItemResponse `json:"items"`
	Total    int                  `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"pageSize"`
}

type LeadListResponse struct {
	Items      []LeadResponse `json:"items"`
	Total      int            `json:"total"`
	Page       int            `json:"page"`
	PageSize   int            `json:"pageSize"`
	TotalPages int            `json:"totalPages"`
}

type DuplicateCheckResponse struct {
	IsDuplicate  bool          `json:"isDuplicate"`
	ExistingLead *LeadResponse `json:"existingLead,omitempty"`
}

// ReturningCustomerResponse provides information about an existing lead for returning customer detection
type ReturningCustomerResponse struct {
	Found         bool           `json:"found"`
	LeadID        *uuid.UUID     `json:"leadId,omitempty"`
	FullName      string         `json:"fullName,omitempty"`
	TotalServices int            `json:"totalServices"`
	Services      []ServiceBrief `json:"services,omitempty"` // Brief summary of past services
}

// ServiceBrief provides a brief summary of a service for returning customer detection
type ServiceBrief struct {
	ServiceType ServiceType `json:"serviceType"`
	Status      LeadStatus  `json:"status"`
	CreatedAt   time.Time   `json:"createdAt"`
}

type BulkDeleteLeadsResponse struct {
	DeletedCount int `json:"deletedCount"`
}

// LeadMetricsResponse provides aggregated KPIs for the dashboard.
type LeadMetricsResponse struct {
	TotalLeads          int     `json:"totalLeads"`
	ProjectedValueCents int64   `json:"projectedValueCents"`
	DisqualifiedRate    float64 `json:"disqualifiedRate"`
	TouchpointsPerLead  float64 `json:"touchpointsPerLead"`
}

// Visit history types
type VisitOutcome string

const (
	VisitOutcomeCompleted   VisitOutcome = "completed"
	VisitOutcomeNoShow      VisitOutcome = "no_show"
	VisitOutcomeRescheduled VisitOutcome = "rescheduled"
	VisitOutcomeCancelled   VisitOutcome = "cancelled"
)

type VisitHistoryResponse struct {
	ID               uuid.UUID         `json:"id"`
	LeadID           uuid.UUID         `json:"leadId"`
	ScheduledDate    time.Time         `json:"scheduledDate"`
	ScoutID          *uuid.UUID        `json:"scoutId,omitempty"`
	Outcome          VisitOutcome      `json:"outcome"`
	Measurements     *string           `json:"measurements,omitempty"`
	AccessDifficulty *AccessDifficulty `json:"accessDifficulty,omitempty"`
	Notes            *string           `json:"notes,omitempty"`
	CompletedAt      *time.Time        `json:"completedAt,omitempty"`
	CreatedAt        time.Time         `json:"createdAt"`
}

type VisitHistoryListResponse struct {
	Items []VisitHistoryResponse `json:"items"`
}
