package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// LeadsRepository defines the interface for leads data operations.
// This allows services to depend on an abstraction rather than concrete implementation,
// improving testability and modularity.
type LeadsRepository interface {
	// Lead CRUD operations
	Create(ctx context.Context, params CreateLeadParams) (Lead, error)
	GetByID(ctx context.Context, id uuid.UUID) (Lead, error)
	GetByIDWithServices(ctx context.Context, id uuid.UUID) (Lead, []LeadService, error)
	GetByPhone(ctx context.Context, phone string) (Lead, error)
	Update(ctx context.Context, id uuid.UUID, params UpdateLeadParams) (Lead, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) (Lead, error)
	Delete(ctx context.Context, id uuid.UUID) error
	BulkDelete(ctx context.Context, ids []uuid.UUID) (int, error)
	List(ctx context.Context, params ListParams) ([]Lead, int, error)

	// Lead viewing
	SetViewedBy(ctx context.Context, id uuid.UUID, userID uuid.UUID) error

	// Activity logging
	AddActivity(ctx context.Context, leadID uuid.UUID, userID uuid.UUID, action string, meta map[string]interface{}) error

	// Legacy visit operations (on lead)
	ScheduleVisit(ctx context.Context, id uuid.UUID, scheduledDate time.Time, scoutID *uuid.UUID) (Lead, error)
	CompleteSurvey(ctx context.Context, id uuid.UUID, measurements string, accessDifficulty string, notes string) (Lead, error)
	MarkNoShow(ctx context.Context, id uuid.UUID, notes string) (Lead, error)
	RescheduleVisit(ctx context.Context, id uuid.UUID, scheduledDate time.Time, scoutID *uuid.UUID, noShowNotes string, markAsNoShow bool) (Lead, error)

	// Lead services operations
	CreateLeadService(ctx context.Context, params CreateLeadServiceParams) (LeadService, error)
	GetLeadServiceByID(ctx context.Context, id uuid.UUID) (LeadService, error)
	ListLeadServices(ctx context.Context, leadID uuid.UUID) ([]LeadService, error)
	GetCurrentLeadService(ctx context.Context, leadID uuid.UUID) (LeadService, error)
	UpdateLeadService(ctx context.Context, id uuid.UUID, params UpdateLeadServiceParams) (LeadService, error)
	UpdateServiceStatus(ctx context.Context, id uuid.UUID, status string) (LeadService, error)
	ScheduleServiceVisit(ctx context.Context, id uuid.UUID, scheduledDate time.Time, scoutID *uuid.UUID) (LeadService, error)
	CompleteServiceSurvey(ctx context.Context, id uuid.UUID, measurements string, accessDifficulty string, notes string) (LeadService, error)
	MarkServiceNoShow(ctx context.Context, id uuid.UUID, notes string) (LeadService, error)
	RescheduleServiceVisit(ctx context.Context, id uuid.UUID, scheduledDate time.Time, scoutID *uuid.UUID, noShowNotes string, markAsNoShow bool) (LeadService, error)
	CloseAllActiveServices(ctx context.Context, leadID uuid.UUID) error

	// Notes operations
	CreateLeadNote(ctx context.Context, params CreateLeadNoteParams) (LeadNote, error)
	ListLeadNotes(ctx context.Context, leadID uuid.UUID) ([]LeadNote, error)

	// Visit history operations
	CreateVisitHistory(ctx context.Context, params CreateVisitHistoryParams) (VisitHistory, error)
	ListVisitHistory(ctx context.Context, leadID uuid.UUID) ([]VisitHistory, error)
	GetVisitHistoryByID(ctx context.Context, id uuid.UUID) (VisitHistory, error)
}

// Ensure Repository implements LeadsRepository
var _ LeadsRepository = (*Repository)(nil)
