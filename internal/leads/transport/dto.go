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

const (
	ServiceTypeWindows    ServiceType = "Windows"
	ServiceTypeInsulation ServiceType = "Insulation"
	ServiceTypeSolar      ServiceType = "Solar"
)

type LeadStatus string

const (
	LeadStatusNew               LeadStatus = "New"
	LeadStatusAttemptedContact  LeadStatus = "Attempted_Contact"
	LeadStatusScheduled         LeadStatus = "Scheduled"
	LeadStatusSurveyed          LeadStatus = "Surveyed"
	LeadStatusBadLead           LeadStatus = "Bad_Lead"
	LeadStatusNeedsRescheduling LeadStatus = "Needs_Rescheduling"
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
	ServiceType  ServiceType  `json:"serviceType" validate:"required,oneof=Windows Insulation Solar"`
	AssigneeID   OptionalUUID `json:"assigneeId,omitempty" validate:"-"`
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
	ServiceType  *ServiceType  `json:"serviceType,omitempty" validate:"omitempty,oneof=Windows Insulation Solar"`
	Status       *LeadStatus   `json:"status,omitempty" validate:"omitempty,oneof=New Attempted_Contact Scheduled Surveyed Bad_Lead Needs_Rescheduling"`
	AssigneeID   OptionalUUID  `json:"assigneeId,omitempty" validate:"-"`
}

type UpdateLeadStatusRequest struct {
	Status LeadStatus `json:"status" validate:"required,oneof=New Attempted_Contact Scheduled Surveyed Bad_Lead Needs_Rescheduling"`
}

type AssignLeadRequest struct {
	AssigneeID *uuid.UUID `json:"assigneeId" validate:"omitempty"`
}

type ScheduleVisitRequest struct {
	ScheduledDate time.Time  `json:"scheduledDate" validate:"required"`
	ScoutID       *uuid.UUID `json:"scoutId,omitempty"`
}

type CompleteSurveyRequest struct {
	Measurements     string           `json:"measurements" validate:"required,min=1,max=500"`
	AccessDifficulty AccessDifficulty `json:"accessDifficulty" validate:"required,oneof=Low Medium High"`
	Notes            string           `json:"notes,omitempty" validate:"max=2000"`
}

type MarkNoShowRequest struct {
	Notes string `json:"notes,omitempty" validate:"max=500"`
}

type ListLeadsRequest struct {
	Status      *LeadStatus  `form:"status" validate:"omitempty,oneof=New Attempted_Contact Scheduled Surveyed Bad_Lead Needs_Rescheduling"`
	ServiceType *ServiceType `form:"serviceType" validate:"omitempty,oneof=Windows Insulation Solar"`
	Search      string       `form:"search" validate:"max=100"`
	Page        int          `form:"page" validate:"min=1"`
	PageSize    int          `form:"pageSize" validate:"min=1,max=100"`
	SortBy      string       `form:"sortBy" validate:"omitempty,oneof=createdAt scheduledDate status firstName lastName"`
	SortOrder   string       `form:"sortOrder" validate:"omitempty,oneof=asc desc"`
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
	Street      string `json:"street"`
	HouseNumber string `json:"houseNumber"`
	ZipCode     string `json:"zipCode"`
	City        string `json:"city"`
}

type VisitResponse struct {
	ScheduledDate    *time.Time        `json:"scheduledDate,omitempty"`
	ScoutID          *uuid.UUID        `json:"scoutId,omitempty"`
	Measurements     *string           `json:"measurements,omitempty"`
	AccessDifficulty *AccessDifficulty `json:"accessDifficulty,omitempty"`
	Notes            *string           `json:"notes,omitempty"`
	CompletedAt      *time.Time        `json:"completedAt,omitempty"`
}

type LeadResponse struct {
	ID              uuid.UUID        `json:"id"`
	Consumer        ConsumerResponse `json:"consumer"`
	Address         AddressResponse  `json:"address"`
	ServiceType     ServiceType      `json:"serviceType"`
	Status          LeadStatus       `json:"status"`
	AssignedAgentID *uuid.UUID       `json:"assignedAgentId,omitempty"`
	ViewedByID      *uuid.UUID       `json:"viewedById,omitempty"`
	ViewedAt        *time.Time       `json:"viewedAt,omitempty"`
	Visit           VisitResponse    `json:"visit"`
	CreatedAt       time.Time        `json:"createdAt"`
	UpdatedAt       time.Time        `json:"updatedAt"`
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
