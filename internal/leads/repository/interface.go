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

// LeadViewTracker tracks which users have viewed leads.
type LeadViewTracker interface {
	SetViewedBy(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, userID uuid.UUID) error
}

// ActivityLogger records activity/audit trail on leads.
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

// AIAnalysisStore manages AI-generated analyses for leads.
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

// =====================================
// Composite Interface (for backward compatibility)
// =====================================

// LeadsRepository defines the complete interface for leads data operations.
// Composed of smaller, focused interfaces for better testability and flexibility.
type LeadsRepository interface {
	LeadReader
	LeadWriter
	LeadViewTracker
	ActivityLogger
	MetricsReader
	LeadServiceReader
	LeadServiceWriter
	NoteStore
	AIAnalysisStore
	AttachmentStore
	PhotoAnalysisStore
	ServiceTypeContextReader
}

// Ensure Repository implements LeadsRepository
var _ LeadsRepository = (*Repository)(nil)
