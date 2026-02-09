package transport

import (
	"encoding/json"
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

type PipelineStage string

const (
	PipelineStageTriage             PipelineStage = "Triage"
	PipelineStageNurturing          PipelineStage = "Nurturing"
	PipelineStageReadyForEstimator  PipelineStage = "Ready_For_Estimator"
	PipelineStageQuoteSent          PipelineStage = "Quote_Sent"
	PipelineStageReadyForPartner    PipelineStage = "Ready_For_Partner"
	PipelineStagePartnerMatching    PipelineStage = "Partner_Matching"
	PipelineStagePartnerAssigned    PipelineStage = "Partner_Assigned"
	PipelineStageManualIntervention PipelineStage = "Manual_Intervention"
	PipelineStageCompleted          PipelineStage = "Completed"
	PipelineStageLost               PipelineStage = "Lost"
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

type LeadServiceResponse struct {
	ID            uuid.UUID                `json:"id"`
	ServiceType   ServiceType              `json:"serviceType"`
	Status        LeadStatus               `json:"status"`
	PipelineStage PipelineStage            `json:"pipelineStage"`
	Preferences   *LeadPreferencesResponse `json:"preferences,omitempty"`
	ConsumerNote  *string                  `json:"consumerNote,omitempty"`
	CreatedAt     time.Time                `json:"createdAt"`
	UpdatedAt     time.Time                `json:"updatedAt"`
}

type LeadPreferencesResponse struct {
	Budget       *string `json:"budget,omitempty"`
	Timeframe    *string `json:"timeframe,omitempty"`
	Availability *string `json:"availability,omitempty"`
	ExtraNotes   *string `json:"extraNotes,omitempty"`
}

type EnergyLabelResponse struct {
	Energieklasse           string     `json:"energieklasse"`                     // Energy label class (A+++, A++, A+, A, B, C, D, E, F, G)
	EnergieIndex            *float64   `json:"energieIndex,omitempty"`            // Energy index value
	Bouwjaar                int        `json:"bouwjaar,omitempty"`                // Construction year
	GeldigTot               *time.Time `json:"geldigTot,omitempty"`               // Label validity end date
	Gebouwtype              string     `json:"gebouwtype,omitempty"`              // Building type
	Registratiedatum        *time.Time `json:"registratiedatum,omitempty"`        // When the label was registered
	PrimaireFossieleEnergie *float64   `json:"primaireFossieleEnergie,omitempty"` // Primary fossil energy use (kWh/m2·jaar)
}

type LeadEnrichmentResponse struct {
	Source                    *string    `json:"source,omitempty"`
	Postcode6                 *string    `json:"postcode6,omitempty"`
	Postcode4                 *string    `json:"postcode4,omitempty"`
	Buurtcode                 *string    `json:"buurtcode,omitempty"`
	DataYear                  *int       `json:"dataYear,omitempty"` // Year of CBS statistics data (e.g. 2022, 2023, 2024)
	GemAardgasverbruik        *float64   `json:"gemAardgasverbruik,omitempty"`
	GemElektriciteitsverbruik *float64   `json:"gemElektriciteitsverbruik,omitempty"`
	HuishoudenGrootte         *float64   `json:"huishoudenGrootte,omitempty"`
	KoopwoningenPct           *float64   `json:"koopwoningenPct,omitempty"`
	BouwjaarVanaf2000Pct      *float64   `json:"bouwjaarVanaf2000Pct,omitempty"`
	WOZWaarde                 *float64   `json:"wozWaarde,omitempty"` // Average WOZ property value in thousands
	MediaanVermogenX1000      *float64   `json:"mediaanVermogenX1000,omitempty"`
	GemInkomen                *float64   `json:"gemInkomen,omitempty"` // Average income in thousands
	PctHoogInkomen            *float64   `json:"pctHoogInkomen,omitempty"`
	PctLaagInkomen            *float64   `json:"pctLaagInkomen,omitempty"`
	HuishoudensMetKinderenPct *float64   `json:"huishoudensMetKinderenPct,omitempty"`
	Stedelijkheid             *int       `json:"stedelijkheid,omitempty"` // 1=very urban to 5=rural
	Confidence                *float64   `json:"confidence,omitempty"`
	FetchedAt                 *time.Time `json:"fetchedAt,omitempty"`
}

type LeadScoreResponse struct {
	Score     *int            `json:"score,omitempty"`
	PreAI     *int            `json:"preAi,omitempty"`
	Factors   json.RawMessage `json:"factors,omitempty"`
	Version   *string         `json:"version,omitempty"`
	UpdatedAt *time.Time      `json:"updatedAt,omitempty"`
}

type LeadResponse struct {
	ID              uuid.UUID               `json:"id"`
	Consumer        ConsumerResponse        `json:"consumer"`
	Address         AddressResponse         `json:"address"`
	Services        []LeadServiceResponse   `json:"services"`
	CurrentService  *LeadServiceResponse    `json:"currentService,omitempty"`
	AggregateStatus *LeadStatus             `json:"aggregateStatus,omitempty"` // Derived from current service
	EnergyLabel     *EnergyLabelResponse    `json:"energyLabel,omitempty"`     // Energy label data from EP-Online
	LeadEnrichment  *LeadEnrichmentResponse `json:"leadEnrichment,omitempty"`
	LeadScore       *LeadScoreResponse      `json:"leadScore,omitempty"`
	AssignedAgentID *uuid.UUID              `json:"assignedAgentId,omitempty"`
	ViewedByID      *uuid.UUID              `json:"viewedById,omitempty"`
	ViewedAt        *time.Time              `json:"viewedAt,omitempty"`
	Source          *string                 `json:"source,omitempty"`
	CreatedAt       time.Time               `json:"createdAt"`
	UpdatedAt       time.Time               `json:"updatedAt"`
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

// TimelineItem represents an entry in the lead timeline feed.
type TimelineItem struct {
	ID        uuid.UUID      `json:"id"`
	Type      string         `json:"type"` // 'ai', 'user', 'stage'
	Title     string         `json:"title"`
	Summary   string         `json:"summary"`
	Timestamp time.Time      `json:"timestamp"`
	Actor     string         `json:"actor"`
	Metadata  map[string]any `json:"metadata"`
}

// LogCallRequest is the request body for processing a post-call summary
type LogCallRequest struct {
	Summary string `json:"summary" validate:"required,min=1,max=5000"`
}

// ActivityFeedItem is a single entry in the dashboard activity feed (historical).
type ActivityFeedItem struct {
	ID              string         `json:"id"`
	Type            string         `json:"type"`
	Category        string         `json:"category"`
	Title           string         `json:"title"`
	Description     string         `json:"description,omitempty"`
	LeadName        string         `json:"leadName,omitempty"`
	Phone           string         `json:"phone,omitempty"`
	Email           string         `json:"email,omitempty"`
	LeadStatus      string         `json:"leadStatus,omitempty"`
	ServiceType     string         `json:"serviceType,omitempty"`
	LeadScore       *int           `json:"leadScore,omitempty"`
	Address         string         `json:"address,omitempty"`
	Latitude        *float64       `json:"latitude,omitempty"`
	Longitude       *float64       `json:"longitude,omitempty"`
	ScheduledAt     string         `json:"scheduledAt,omitempty"`
	Timestamp       string         `json:"timestamp"`
	Priority        int            `json:"priority,omitempty"`
	Link            []string       `json:"link,omitempty"`
	Sentiment       string         `json:"sentiment"`
	GroupCount      int            `json:"groupCount,omitempty"`
	ActorName       string         `json:"actorName,omitempty"`
	SuggestedAction string         `json:"suggestedAction,omitempty"`
	ActionLink      string         `json:"actionLink,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// FeedSeparator marks a position in the feed list for visual grouping (e.g. day headers).
type FeedSeparator struct {
	Index int    `json:"index"`
	Label string `json:"label"`
}

// ActivityFeedResponse wraps the list returned by GET /leads/activity-feed.
type ActivityFeedResponse struct {
	Items      []ActivityFeedItem `json:"items"`
	Separators []FeedSeparator    `json:"separators,omitempty"`
}

// LogCallResponse is the response for a processed call log
type LogCallResponse struct {
	NoteCreated            bool       `json:"noteCreated"`
	NoteBody               string     `json:"noteBody,omitempty"`
	AuthorEmail            string     `json:"authorEmail,omitempty"`
	CallOutcome            *string    `json:"callOutcome,omitempty"`
	StatusUpdated          *string    `json:"statusUpdated,omitempty"`
	PipelineStageUpdated   *string    `json:"pipelineStageUpdated,omitempty"`
	AppointmentBooked      *time.Time `json:"appointmentBooked,omitempty"`
	AppointmentRescheduled *time.Time `json:"appointmentRescheduled,omitempty"`
	AppointmentCancelled   bool       `json:"appointmentCancelled,omitempty"`
	Message                string     `json:"message"`
}
