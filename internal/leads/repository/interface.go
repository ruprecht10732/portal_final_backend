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
	GetByID(ctx context.Context, id uuid.UUID) (Lead, error)
	GetByIDWithServices(ctx context.Context, id uuid.UUID) (Lead, []LeadService, error)
	GetByPhone(ctx context.Context, phone string) (Lead, error)
	List(ctx context.Context, params ListParams) ([]Lead, int, error)
}

// LeadWriter provides write operations for lead management.
type LeadWriter interface {
	Create(ctx context.Context, params CreateLeadParams) (Lead, error)
	Update(ctx context.Context, id uuid.UUID, params UpdateLeadParams) (Lead, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) (Lead, error)
	Delete(ctx context.Context, id uuid.UUID) error
	BulkDelete(ctx context.Context, ids []uuid.UUID) (int, error)
}

// LeadViewTracker tracks which users have viewed leads.
type LeadViewTracker interface {
	SetViewedBy(ctx context.Context, id uuid.UUID, userID uuid.UUID) error
}

// ActivityLogger records activity/audit trail on leads.
type ActivityLogger interface {
	AddActivity(ctx context.Context, leadID uuid.UUID, userID uuid.UUID, action string, meta map[string]interface{}) error
}

// MetricsReader provides access to lead KPI metrics.
type MetricsReader interface {
	GetMetrics(ctx context.Context) (LeadMetrics, error)
}

// LeadServiceReader provides read access to lead services.
type LeadServiceReader interface {
	GetLeadServiceByID(ctx context.Context, id uuid.UUID) (LeadService, error)
	ListLeadServices(ctx context.Context, leadID uuid.UUID) ([]LeadService, error)
	GetCurrentLeadService(ctx context.Context, leadID uuid.UUID) (LeadService, error)
}

// LeadServiceWriter provides write operations for lead services.
type LeadServiceWriter interface {
	CreateLeadService(ctx context.Context, params CreateLeadServiceParams) (LeadService, error)
	UpdateLeadService(ctx context.Context, id uuid.UUID, params UpdateLeadServiceParams) (LeadService, error)
	UpdateServiceStatus(ctx context.Context, id uuid.UUID, status string) (LeadService, error)
	CloseAllActiveServices(ctx context.Context, leadID uuid.UUID) error
}

// VisitManager handles visit scheduling and completion on services.
type VisitManager interface {
	ScheduleServiceVisit(ctx context.Context, id uuid.UUID, scheduledDate time.Time, scoutID *uuid.UUID) (LeadService, error)
	CompleteServiceSurvey(ctx context.Context, id uuid.UUID, measurements string, accessDifficulty string, notes string) (LeadService, error)
	MarkServiceNoShow(ctx context.Context, id uuid.UUID, notes string) (LeadService, error)
	RescheduleServiceVisit(ctx context.Context, id uuid.UUID, scheduledDate time.Time, scoutID *uuid.UUID, noShowNotes string, markAsNoShow bool) (LeadService, error)
}

// LegacyVisitManager handles legacy visit operations directly on leads (deprecated).
// Prefer using VisitManager with lead services instead.
type LegacyVisitManager interface {
	ScheduleVisit(ctx context.Context, id uuid.UUID, scheduledDate time.Time, scoutID *uuid.UUID) (Lead, error)
	CompleteSurvey(ctx context.Context, id uuid.UUID, measurements string, accessDifficulty string, notes string) (Lead, error)
	MarkNoShow(ctx context.Context, id uuid.UUID, notes string) (Lead, error)
	RescheduleVisit(ctx context.Context, id uuid.UUID, scheduledDate time.Time, scoutID *uuid.UUID, noShowNotes string, markAsNoShow bool) (Lead, error)
}

// NoteStore manages lead notes.
type NoteStore interface {
	CreateLeadNote(ctx context.Context, params CreateLeadNoteParams) (LeadNote, error)
	ListLeadNotes(ctx context.Context, leadID uuid.UUID) ([]LeadNote, error)
}

// VisitHistoryStore manages visit history records.
type VisitHistoryStore interface {
	CreateVisitHistory(ctx context.Context, params CreateVisitHistoryParams) (VisitHistory, error)
	ListVisitHistory(ctx context.Context, leadID uuid.UUID) ([]VisitHistory, error)
	GetVisitHistoryByID(ctx context.Context, id uuid.UUID) (VisitHistory, error)
}

// AIAnalysisStore manages AI-generated analyses for leads.
type AIAnalysisStore interface {
	CreateAIAnalysis(ctx context.Context, params CreateAIAnalysisParams) (AIAnalysis, error)
	GetLatestAIAnalysis(ctx context.Context, leadID uuid.UUID) (AIAnalysis, error)
	ListAIAnalyses(ctx context.Context, leadID uuid.UUID) ([]AIAnalysis, error)
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
	VisitManager
	LegacyVisitManager
	NoteStore
	VisitHistoryStore
	AIAnalysisStore
}

// Ensure Repository implements LeadsRepository
var _ LeadsRepository = (*Repository)(nil)
