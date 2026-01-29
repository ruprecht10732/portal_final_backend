// Package scheduling handles visit scheduling operations for leads.
// This is a vertically sliced feature package containing service logic
// for scheduling, rescheduling, completing, and managing lead visits.
package scheduling

import (
	"context"
	"errors"
	"time"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/management"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

// Repository defines the data access interface needed by the scheduling service.
// This is a consumer-driven interface - only what scheduling needs.
type Repository interface {
	repository.LeadReader
	repository.LeadServiceReader
	repository.VisitManager
	repository.ActivityLogger
	repository.VisitHistoryStore
}

// Service handles visit scheduling operations.
type Service struct {
	repo     Repository
	eventBus events.Bus
}

// New creates a new visit scheduling service.
func New(repo Repository, eventBus events.Bus) *Service {
	return &Service{repo: repo, eventBus: eventBus}
}

// ScheduleVisit schedules a new visit for a lead service.
func (s *Service) ScheduleVisit(ctx context.Context, leadID uuid.UUID, req transport.ScheduleVisitRequest) (transport.LeadResponse, error) {
	// Validate scheduled date is not in the past
	today := time.Now().Truncate(24 * time.Hour)
	scheduledDay := req.ScheduledDate.Truncate(24 * time.Hour)
	if scheduledDay.Before(today) {
		return transport.LeadResponse{}, apperr.Validation("cannot schedule a visit in the past")
	}

	// Schedule visit on the service
	_, err := s.repo.ScheduleServiceVisit(ctx, req.ServiceID, req.ScheduledDate, req.ScoutID)
	if err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) {
			return transport.LeadResponse{}, apperr.NotFound("lead service not found")
		}
		return transport.LeadResponse{}, err
	}

	// Return the lead with all services
	lead, services, err := s.repo.GetByIDWithServices(ctx, leadID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound("lead not found")
		}
		return transport.LeadResponse{}, err
	}

	// Publish event - notification module handles email sending
	s.eventBus.Publish(ctx, events.VisitScheduled{
		BaseEvent:          events.NewBaseEvent(),
		LeadID:             leadID,
		ServiceID:          req.ServiceID,
		ScheduledDate:      req.ScheduledDate,
		ScoutID:            req.ScoutID,
		ConsumerEmail:      lead.ConsumerEmail,
		ConsumerFirstName:  lead.ConsumerFirstName,
		ConsumerLastName:   lead.ConsumerLastName,
		AddressStreet:      lead.AddressStreet,
		AddressHouseNumber: lead.AddressHouseNumber,
		AddressZipCode:     lead.AddressZipCode,
		AddressCity:        lead.AddressCity,
		SendInvite:         req.SendInvite,
	})

	return management.ToLeadResponseWithServices(lead, services), nil
}

// RescheduleVisit reschedules an existing visit.
func (s *Service) RescheduleVisit(ctx context.Context, leadID uuid.UUID, req transport.RescheduleVisitRequest, actorID uuid.UUID) (transport.LeadResponse, error) {
	// Validate scheduled date is not in the past
	today := time.Now().Truncate(24 * time.Hour)
	scheduledDay := req.ScheduledDate.Truncate(24 * time.Hour)
	if scheduledDay.Before(today) {
		return transport.LeadResponse{}, apperr.Validation("cannot schedule a visit in the past")
	}

	// Get current service to capture old visit data for history
	currentService, err := s.repo.GetLeadServiceByID(ctx, req.ServiceID)
	if err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) {
			return transport.LeadResponse{}, apperr.NotFound("lead service not found")
		}
		return transport.LeadResponse{}, err
	}

	// Store the old visit in history if there was a scheduled date
	if currentService.VisitScheduledDate != nil {
		outcome := "rescheduled"
		if req.MarkAsNoShow {
			outcome = "no_show"
		}

		_, _ = s.repo.CreateVisitHistory(ctx, repository.CreateVisitHistoryParams{
			LeadID:           leadID,
			ScheduledDate:    *currentService.VisitScheduledDate,
			ScoutID:          currentService.VisitScoutID,
			Outcome:          outcome,
			Measurements:     currentService.VisitMeasurements,
			AccessDifficulty: currentService.VisitAccessDifficulty,
			Notes:            &req.NoShowNotes,
			CompletedAt:      currentService.VisitCompletedAt,
		})
	}

	// Perform the reschedule on the service
	_, err = s.repo.RescheduleServiceVisit(ctx, req.ServiceID, req.ScheduledDate, req.ScoutID, req.NoShowNotes, req.MarkAsNoShow)
	if err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) {
			return transport.LeadResponse{}, apperr.NotFound("lead service not found")
		}
		return transport.LeadResponse{}, err
	}

	// Log the no-show to activity only if marked as no-show
	if req.MarkAsNoShow {
		_ = s.repo.AddActivity(ctx, leadID, actorID, "no_show", map[string]interface{}{
			"serviceId":             req.ServiceID,
			"previousScheduledDate": currentService.VisitScheduledDate,
			"notes":                 req.NoShowNotes,
		})
	}

	// Log the reschedule to activity
	_ = s.repo.AddActivity(ctx, leadID, actorID, "rescheduled", map[string]interface{}{
		"serviceId":             req.ServiceID,
		"previousScheduledDate": currentService.VisitScheduledDate,
		"newScheduledDate":      req.ScheduledDate,
		"scoutId":               req.ScoutID,
	})

	// Return the lead with all services
	lead, services, err := s.repo.GetByIDWithServices(ctx, leadID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound("lead not found")
		}
		return transport.LeadResponse{}, err
	}

	// Publish event - notification module handles email sending
	s.eventBus.Publish(ctx, events.VisitRescheduled{
		BaseEvent:          events.NewBaseEvent(),
		LeadID:             leadID,
		ServiceID:          req.ServiceID,
		PreviousDate:       currentService.VisitScheduledDate,
		NewScheduledDate:   req.ScheduledDate,
		ScoutID:            req.ScoutID,
		MarkedAsNoShow:     req.MarkAsNoShow,
		ConsumerEmail:      lead.ConsumerEmail,
		ConsumerFirstName:  lead.ConsumerFirstName,
		ConsumerLastName:   lead.ConsumerLastName,
		AddressStreet:      lead.AddressStreet,
		AddressHouseNumber: lead.AddressHouseNumber,
		AddressZipCode:     lead.AddressZipCode,
		AddressCity:        lead.AddressCity,
		SendInvite:         req.SendInvite,
	})

	return management.ToLeadResponseWithServices(lead, services), nil
}

// CompleteSurvey marks a visit as completed with survey data.
func (s *Service) CompleteSurvey(ctx context.Context, leadID uuid.UUID, req transport.CompleteSurveyRequest) (transport.LeadResponse, error) {
	// Get current service to check scheduled date
	currentService, err := s.repo.GetLeadServiceByID(ctx, req.ServiceID)
	if err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) {
			return transport.LeadResponse{}, apperr.NotFound("lead service not found")
		}
		return transport.LeadResponse{}, err
	}

	// Check if visit is scheduled
	if currentService.VisitScheduledDate == nil {
		return transport.LeadResponse{}, apperr.Validation("visit is not scheduled")
	}

	// Check if scheduled date is in the future
	if currentService.VisitScheduledDate.After(time.Now()) {
		return transport.LeadResponse{}, apperr.Validation("cannot complete a visit scheduled in the future")
	}

	// Complete survey on the service
	_, err = s.repo.CompleteServiceSurvey(ctx, req.ServiceID, req.Measurements, string(req.AccessDifficulty), req.Notes)
	if err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) {
			return transport.LeadResponse{}, apperr.NotFound("lead service not found")
		}
		return transport.LeadResponse{}, err
	}

	// Return the lead with all services
	lead, services, err := s.repo.GetByIDWithServices(ctx, leadID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound("lead not found")
		}
		return transport.LeadResponse{}, err
	}

	return management.ToLeadResponseWithServices(lead, services), nil
}

// MarkNoShow marks a visit as a no-show.
func (s *Service) MarkNoShow(ctx context.Context, leadID uuid.UUID, req transport.MarkNoShowRequest) (transport.LeadResponse, error) {
	// Mark no-show on the service
	_, err := s.repo.MarkServiceNoShow(ctx, req.ServiceID, req.Notes)
	if err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) {
			return transport.LeadResponse{}, apperr.NotFound("lead service not found")
		}
		return transport.LeadResponse{}, err
	}

	// Return the lead with all services
	lead, services, err := s.repo.GetByIDWithServices(ctx, leadID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound("lead not found")
		}
		return transport.LeadResponse{}, err
	}

	return management.ToLeadResponseWithServices(lead, services), nil
}

// ListVisitHistory returns the visit history for a lead.
func (s *Service) ListVisitHistory(ctx context.Context, leadID uuid.UUID) (transport.VisitHistoryListResponse, error) {
	// Verify lead exists
	if _, err := s.repo.GetByID(ctx, leadID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.VisitHistoryListResponse{}, apperr.NotFound("lead not found")
		}
		return transport.VisitHistoryListResponse{}, err
	}

	history, err := s.repo.ListVisitHistory(ctx, leadID)
	if err != nil {
		return transport.VisitHistoryListResponse{}, err
	}

	items := make([]transport.VisitHistoryResponse, len(history))
	for i, h := range history {
		items[i] = toVisitHistoryResponse(h)
	}

	return transport.VisitHistoryListResponse{Items: items}, nil
}

// toVisitHistoryResponse converts a repository visit history to a transport response.
func toVisitHistoryResponse(vh repository.VisitHistory) transport.VisitHistoryResponse {
	resp := transport.VisitHistoryResponse{
		ID:            vh.ID,
		LeadID:        vh.LeadID,
		ScheduledDate: vh.ScheduledDate,
		ScoutID:       vh.ScoutID,
		Outcome:       transport.VisitOutcome(vh.Outcome),
		Measurements:  vh.Measurements,
		Notes:         vh.Notes,
		CompletedAt:   vh.CompletedAt,
		CreatedAt:     vh.CreatedAt,
	}

	if vh.AccessDifficulty != nil {
		difficulty := transport.AccessDifficulty(*vh.AccessDifficulty)
		resp.AccessDifficulty = &difficulty
	}

	return resp
}
