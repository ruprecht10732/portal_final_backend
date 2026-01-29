package service

import (
	"context"
	"errors"
	"time"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/internal/phone"

	"github.com/google/uuid"
)

var (
	ErrLeadNotFound      = errors.New("lead not found")
	ErrDuplicatePhone    = errors.New("a lead with this phone number already exists")
	ErrInvalidTransition = errors.New("invalid status transition")
	ErrForbidden         = errors.New("forbidden")
	ErrInvalidNote       = errors.New("invalid note")
	ErrVisitNotScheduled = errors.New("visit is not scheduled")
	ErrVisitInFuture     = errors.New("cannot complete a visit scheduled in the future")
)

type Service struct {
	repo *repository.Repository
}

func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, req transport.CreateLeadRequest) (transport.LeadResponse, error) {
	req.Phone = phone.NormalizeE164(req.Phone)

	params := repository.CreateLeadParams{
		ConsumerFirstName:  req.FirstName,
		ConsumerLastName:   req.LastName,
		ConsumerPhone:      req.Phone,
		ConsumerRole:       string(req.ConsumerRole),
		AddressStreet:      req.Street,
		AddressHouseNumber: req.HouseNumber,
		AddressZipCode:     req.ZipCode,
		AddressCity:        req.City,
		ServiceType:        string(req.ServiceType),
	}

	if req.AssigneeID.Set {
		params.AssignedAgentID = req.AssigneeID.Value
	}

	if req.Email != "" {
		params.ConsumerEmail = &req.Email
	}

	lead, err := s.repo.Create(ctx, params)
	if err != nil {
		return transport.LeadResponse{}, err
	}

	return toLeadResponse(lead), nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (transport.LeadResponse, error) {
	lead, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, ErrLeadNotFound
		}
		return transport.LeadResponse{}, err
	}

	return toLeadResponse(lead), nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req transport.UpdateLeadRequest, actorID uuid.UUID, actorRoles []string) (transport.LeadResponse, error) {
	params := repository.UpdateLeadParams{}
	var current repository.Lead
	loadedCurrent := false

	if req.AssigneeID.Set {
		lead, err := s.repo.GetByID(ctx, id)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return transport.LeadResponse{}, ErrLeadNotFound
			}
			return transport.LeadResponse{}, err
		}
		current = lead
		loadedCurrent = true

		if !hasRole(actorRoles, "admin") {
			if current.AssignedAgentID == nil || *current.AssignedAgentID != actorID {
				return transport.LeadResponse{}, ErrForbidden
			}
		}

		params.AssignedAgentID = req.AssigneeID.Value
		params.AssignedAgentIDSet = true
	}

	if req.FirstName != nil {
		params.ConsumerFirstName = req.FirstName
	}
	if req.LastName != nil {
		params.ConsumerLastName = req.LastName
	}
	if req.Phone != nil {
		normalized := phone.NormalizeE164(*req.Phone)
		params.ConsumerPhone = &normalized
	}
	if req.Email != nil {
		params.ConsumerEmail = req.Email
	}
	if req.ConsumerRole != nil {
		role := string(*req.ConsumerRole)
		params.ConsumerRole = &role
	}
	if req.Street != nil {
		params.AddressStreet = req.Street
	}
	if req.HouseNumber != nil {
		params.AddressHouseNumber = req.HouseNumber
	}
	if req.ZipCode != nil {
		params.AddressZipCode = req.ZipCode
	}
	if req.City != nil {
		params.AddressCity = req.City
	}
	if req.ServiceType != nil {
		serviceType := string(*req.ServiceType)
		params.ServiceType = &serviceType
	}
	if req.Status != nil {
		status := string(*req.Status)
		params.Status = &status
	}

	lead, err := s.repo.Update(ctx, id, params)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, ErrLeadNotFound
		}
		return transport.LeadResponse{}, err
	}

	if req.AssigneeID.Set && loadedCurrent {
		if !equalUUIDPtrs(current.AssignedAgentID, req.AssigneeID.Value) {
			_ = s.repo.AddActivity(ctx, id, actorID, "assigned", map[string]interface{}{
				"from": current.AssignedAgentID,
				"to":   req.AssigneeID.Value,
			})
		}
	}

	return toLeadResponse(lead), nil
}

func (s *Service) Assign(ctx context.Context, id uuid.UUID, assigneeID *uuid.UUID, actorID uuid.UUID, actorRoles []string) (transport.LeadResponse, error) {
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, ErrLeadNotFound
		}
		return transport.LeadResponse{}, err
	}

	if !hasRole(actorRoles, "admin") {
		if current.AssignedAgentID == nil || *current.AssignedAgentID != actorID {
			return transport.LeadResponse{}, ErrForbidden
		}
	}

	params := repository.UpdateLeadParams{
		AssignedAgentID:    assigneeID,
		AssignedAgentIDSet: true,
	}
	updated, err := s.repo.Update(ctx, id, params)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, ErrLeadNotFound
		}
		return transport.LeadResponse{}, err
	}

	_ = s.repo.AddActivity(ctx, id, actorID, "assigned", map[string]interface{}{
		"from": current.AssignedAgentID,
		"to":   assigneeID,
	})

	return toLeadResponse(updated), nil
}

func (s *Service) UpdateStatus(ctx context.Context, id uuid.UUID, req transport.UpdateLeadStatusRequest) (transport.LeadResponse, error) {
	lead, err := s.repo.UpdateStatus(ctx, id, string(req.Status))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, ErrLeadNotFound
		}
		return transport.LeadResponse{}, err
	}

	return toLeadResponse(lead), nil
}

func (s *Service) ScheduleVisit(ctx context.Context, id uuid.UUID, req transport.ScheduleVisitRequest) (transport.LeadResponse, error) {
	lead, err := s.repo.ScheduleVisit(ctx, id, req.ScheduledDate, req.ScoutID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, ErrLeadNotFound
		}
		return transport.LeadResponse{}, err
	}

	return toLeadResponse(lead), nil
}

func (s *Service) CompleteSurvey(ctx context.Context, id uuid.UUID, req transport.CompleteSurveyRequest) (transport.LeadResponse, error) {
	// Get current lead to check scheduled date
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, ErrLeadNotFound
		}
		return transport.LeadResponse{}, err
	}

	// Check if visit is scheduled
	if current.VisitScheduledDate == nil {
		return transport.LeadResponse{}, ErrVisitNotScheduled
	}

	// Check if scheduled date is in the future
	if current.VisitScheduledDate.After(time.Now()) {
		return transport.LeadResponse{}, ErrVisitInFuture
	}

	lead, err := s.repo.CompleteSurvey(ctx, id, req.Measurements, string(req.AccessDifficulty), req.Notes)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, ErrLeadNotFound
		}
		return transport.LeadResponse{}, err
	}

	return toLeadResponse(lead), nil
}

func (s *Service) MarkNoShow(ctx context.Context, id uuid.UUID, req transport.MarkNoShowRequest) (transport.LeadResponse, error) {
	lead, err := s.repo.MarkNoShow(ctx, id, req.Notes)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, ErrLeadNotFound
		}
		return transport.LeadResponse{}, err
	}

	return toLeadResponse(lead), nil
}

func (s *Service) RescheduleVisit(ctx context.Context, id uuid.UUID, req transport.RescheduleVisitRequest, actorID uuid.UUID) (transport.LeadResponse, error) {
	// Get current lead to capture old visit data for history
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, ErrLeadNotFound
		}
		return transport.LeadResponse{}, err
	}

	// Store the old visit in history if there was a scheduled date
	if current.VisitScheduledDate != nil {
		outcome := "rescheduled"
		if req.MarkAsNoShow {
			outcome = "no_show"
		}

		_, _ = s.repo.CreateVisitHistory(ctx, repository.CreateVisitHistoryParams{
			LeadID:           id,
			ScheduledDate:    *current.VisitScheduledDate,
			ScoutID:          current.VisitScoutID,
			Outcome:          outcome,
			Measurements:     current.VisitMeasurements,
			AccessDifficulty: current.VisitAccessDifficulty,
			Notes:            &req.NoShowNotes,
			CompletedAt:      current.VisitCompletedAt,
		})
	}

	// Perform the reschedule
	lead, err := s.repo.RescheduleVisit(ctx, id, req.ScheduledDate, req.ScoutID, req.NoShowNotes, req.MarkAsNoShow)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, ErrLeadNotFound
		}
		return transport.LeadResponse{}, err
	}

	// Log the no-show to activity only if marked as no-show
	if req.MarkAsNoShow {
		_ = s.repo.AddActivity(ctx, id, actorID, "no_show", map[string]interface{}{
			"previousScheduledDate": current.VisitScheduledDate,
			"notes":                 req.NoShowNotes,
		})
	}

	// Log the reschedule to activity
	_ = s.repo.AddActivity(ctx, id, actorID, "rescheduled", map[string]interface{}{
		"previousScheduledDate": current.VisitScheduledDate,
		"newScheduledDate":      req.ScheduledDate,
		"scoutId":               req.ScoutID,
	})

	return toLeadResponse(lead), nil
}

func (s *Service) SetViewedBy(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	return s.repo.SetViewedBy(ctx, id, userID)
}

func (s *Service) CheckDuplicate(ctx context.Context, phone string) (transport.DuplicateCheckResponse, error) {
	lead, err := s.repo.GetByPhone(ctx, phone)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.DuplicateCheckResponse{IsDuplicate: false}, nil
		}
		return transport.DuplicateCheckResponse{}, err
	}

	resp := toLeadResponse(lead)
	return transport.DuplicateCheckResponse{
		IsDuplicate:  true,
		ExistingLead: &resp,
	}, nil
}

func (s *Service) List(ctx context.Context, req transport.ListLeadsRequest) (transport.LeadListResponse, error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 {
		req.PageSize = 20
	}
	if req.PageSize > 100 {
		req.PageSize = 100
	}

	params := repository.ListParams{
		Search:    req.Search,
		Offset:    (req.Page - 1) * req.PageSize,
		Limit:     req.PageSize,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}

	if req.Status != nil {
		status := string(*req.Status)
		params.Status = &status
	}
	if req.ServiceType != nil {
		serviceType := string(*req.ServiceType)
		params.ServiceType = &serviceType
	}

	leads, total, err := s.repo.List(ctx, params)
	if err != nil {
		return transport.LeadListResponse{}, err
	}

	items := make([]transport.LeadResponse, len(leads))
	for i, lead := range leads {
		items[i] = toLeadResponse(lead)
	}

	totalPages := (total + req.PageSize - 1) / req.PageSize

	return transport.LeadListResponse{
		Items:      items,
		Total:      total,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	err := s.repo.Delete(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrLeadNotFound
		}
		return err
	}
	return nil
}

func (s *Service) BulkDelete(ctx context.Context, ids []uuid.UUID) (int, error) {
	deletedCount, err := s.repo.BulkDelete(ctx, ids)
	if err != nil {
		return 0, err
	}
	if deletedCount == 0 {
		return 0, ErrLeadNotFound
	}
	return deletedCount, nil
}

func toLeadResponse(lead repository.Lead) transport.LeadResponse {
	resp := transport.LeadResponse{
		ID:              lead.ID,
		ServiceType:     transport.ServiceType(lead.ServiceType),
		Status:          transport.LeadStatus(lead.Status),
		AssignedAgentID: lead.AssignedAgentID,
		ViewedByID:      lead.ViewedByID,
		ViewedAt:        lead.ViewedAt,
		CreatedAt:       lead.CreatedAt,
		UpdatedAt:       lead.UpdatedAt,
		Consumer: transport.ConsumerResponse{
			FirstName: lead.ConsumerFirstName,
			LastName:  lead.ConsumerLastName,
			Phone:     lead.ConsumerPhone,
			Email:     lead.ConsumerEmail,
			Role:      transport.ConsumerRole(lead.ConsumerRole),
		},
		Address: transport.AddressResponse{
			Street:      lead.AddressStreet,
			HouseNumber: lead.AddressHouseNumber,
			ZipCode:     lead.AddressZipCode,
			City:        lead.AddressCity,
		},
		Visit: transport.VisitResponse{
			ScheduledDate: lead.VisitScheduledDate,
			ScoutID:       lead.VisitScoutID,
			Measurements:  lead.VisitMeasurements,
			Notes:         lead.VisitNotes,
			CompletedAt:   lead.VisitCompletedAt,
		},
	}

	if lead.VisitAccessDifficulty != nil {
		difficulty := transport.AccessDifficulty(*lead.VisitAccessDifficulty)
		resp.Visit.AccessDifficulty = &difficulty
	}

	return resp
}

func hasRole(roles []string, target string) bool {
	for _, role := range roles {
		if role == target {
			return true
		}
	}
	return false
}

func equalUUIDPtrs(a *uuid.UUID, b *uuid.UUID) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
