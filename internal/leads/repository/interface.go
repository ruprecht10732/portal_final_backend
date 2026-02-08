package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// =====================================
// Segregated Interfaces (Interface Segregation Principle)
// =====================================

// LeadReader provides read-only access to lead data.
type LeadReader interface {
	GetByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (Lead, error)
	GetByIDWithServices(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (Lead, []LeadService, error)
	GetByPhone(ctx context.Context, phone string, organizationID uuid.UUID) (Lead, error)
	GetByPhoneOrEmail(ctx context.Context, phone string, email string, organizationID uuid.UUID) (*LeadSummary, []LeadService, error)
	List(ctx context.Context, params ListParams) ([]Lead, int, error)
	ListHeatmapPoints(ctx context.Context, organizationID uuid.UUID, startDate *time.Time, endDate *time.Time) ([]HeatmapPoint, error)
	ListActionItems(ctx context.Context, organizationID uuid.UUID, newLeadDays int, limit int, offset int) (ActionItemListResult, error)
}

// LeadWriter provides write operations for lead management.
type LeadWriter interface {
	Create(ctx context.Context, params CreateLeadParams) (Lead, error)
	Update(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params UpdateLeadParams) (Lead, error)
	Delete(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) error
	BulkDelete(ctx context.Context, ids []uuid.UUID, organizationID uuid.UUID) (int, error)
}

// LeadEnrichmentWriter updates enrichment and scoring data for RAC_leads.
type LeadEnrichmentWriter interface {
	UpdateLeadEnrichment(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params UpdateLeadEnrichmentParams) error
	UpdateLeadScore(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params UpdateLeadScoreParams) error
}

// LeadViewTracker tracks which RAC_users have viewed RAC_leads.
type LeadViewTracker interface {
	SetViewedBy(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, userID uuid.UUID) error
}

// ActivityLogger records activity/audit trail on RAC_leads.
type ActivityLogger interface {
	AddActivity(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID, userID uuid.UUID, action string, meta map[string]interface{}) error
}

// MetricsReader provides access to lead KPI metrics.
type MetricsReader interface {
	GetMetrics(ctx context.Context, organizationID uuid.UUID) (LeadMetrics, error)
}

// LeadServiceReader provides read access to lead services.
type LeadServiceReader interface {
	GetLeadServiceByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (LeadService, error)
	ListLeadServices(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) ([]LeadService, error)
	GetCurrentLeadService(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (LeadService, error)
}

// LeadServiceWriter provides write operations for lead services.
type LeadServiceWriter interface {
	CreateLeadService(ctx context.Context, params CreateLeadServiceParams) (LeadService, error)
	UpdateLeadService(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params UpdateLeadServiceParams) (LeadService, error)
	UpdateLeadServiceType(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, serviceType string) (LeadService, error)
	UpdateServiceStatus(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, status string) (LeadService, error)
	UpdatePipelineStage(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, stage string) (LeadService, error)
	CloseAllActiveServices(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) error
}

// ServiceContextDefinition provides context for the AI gatekeeper.
type ServiceContextDefinition struct {
	Name             string
	Description      *string
	IntakeGuidelines *string
}

// ServiceTypeContextReader provides read access to active service definitions for AI context.
type ServiceTypeContextReader interface {
	ListActiveServiceTypes(ctx context.Context, organizationID uuid.UUID) ([]ServiceContextDefinition, error)
}

// NoteStore manages lead notes.
type NoteStore interface {
	CreateLeadNote(ctx context.Context, params CreateLeadNoteParams) (LeadNote, error)
	ListLeadNotes(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) ([]LeadNote, error)
}

// TimelineEventStore manages immutable lead timeline events.
type TimelineEventStore interface {
	CreateTimelineEvent(ctx context.Context, params CreateTimelineEventParams) (TimelineEvent, error)
	ListTimelineEvents(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) ([]TimelineEvent, error)
}

// AIAnalysisStore manages AI-generated analyses for RAC_leads.
type AIAnalysisStore interface {
	CreateAIAnalysis(ctx context.Context, params CreateAIAnalysisParams) (AIAnalysis, error)
	GetLatestAIAnalysis(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) (AIAnalysis, error)
	ListAIAnalyses(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) ([]AIAnalysis, error)
}

// AttachmentStore manages file attachments for lead services.
type AttachmentStore interface {
	CreateAttachment(ctx context.Context, params CreateAttachmentParams) (Attachment, error)
	GetAttachmentByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (Attachment, error)
	ListAttachmentsByService(ctx context.Context, leadServiceID uuid.UUID, organizationID uuid.UUID) ([]Attachment, error)
	DeleteAttachment(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) error
}

// PhotoAnalysisStore manages AI photo analyses for lead services.
type PhotoAnalysisStore interface {
	CreatePhotoAnalysis(ctx context.Context, params CreatePhotoAnalysisParams) (PhotoAnalysis, error)
	GetPhotoAnalysisByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (PhotoAnalysis, error)
	GetLatestPhotoAnalysis(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) (PhotoAnalysis, error)
	ListPhotoAnalysesByService(ctx context.Context, serviceID uuid.UUID, organizationID uuid.UUID) ([]PhotoAnalysis, error)
	ListPhotoAnalysesByLead(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) ([]PhotoAnalysis, error)
}

// LeadAppointmentStats holds appointment statistics for scoring.
type LeadAppointmentStats struct {
	Total       int
	Scheduled   int
	Completed   int
	Cancelled   int
	HasUpcoming bool
}

// AppointmentStatsReader provides appointment stats for RAC_leads (for scoring).
type AppointmentStatsReader interface {
	GetLeadAppointmentStats(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (LeadAppointmentStats, error)
}

// PartnerMatcher provides partner search based on service type and location.
type PartnerMatcher interface {
	FindMatchingPartners(ctx context.Context, organizationID uuid.UUID, serviceType string, zipCode string, radiusKm int) ([]PartnerMatch, error)
}

// ActivityFeedEntry represents a unified activity entry from multiple sources.
type ActivityFeedEntry struct {
	ID          uuid.UUID
	Category    string // leads, quotes, appointments, ai
	EventType   string
	Title       string
	Description string
	EntityID    uuid.UUID
	CreatedAt   time.Time
}

// ActivityFeedReader provides recent org-wide activity for the dashboard feed.
type ActivityFeedReader interface {
	ListRecentActivity(ctx context.Context, organizationID uuid.UUID, limit int) ([]ActivityFeedEntry, error)
}

// =====================================
// Composite Interface (for backward compatibility)
// =====================================

// LeadsRepository defines the complete interface for RAC_leads data operations.
// Composed of smaller, focused interfaces for better testability and flexibility.
type LeadsRepository interface {
	LeadReader
	LeadWriter
	LeadEnrichmentWriter
	LeadViewTracker
	ActivityLogger
	MetricsReader
	LeadServiceReader
	LeadServiceWriter
	NoteStore
	TimelineEventStore
	AIAnalysisStore
	AttachmentStore
	PhotoAnalysisStore
	ServiceTypeContextReader
	AppointmentStatsReader
	PartnerMatcher
	ActivityFeedReader
}

// Ensure Repository implements LeadsRepository
var _ LeadsRepository = (*Repository)(nil)
