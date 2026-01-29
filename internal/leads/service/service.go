package service

import (
	"context"
	"errors"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/transport"

	"github.com/google/uuid"
)

var (
	ErrLeadNotFound      = errors.New("lead not found")
	ErrDuplicatePhone    = errors.New("a lead with this phone number already exists")
	ErrInvalidTransition = errors.New("invalid status transition")
)

type Service struct {
	repo *repository.Repository
}

func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, req transport.CreateLeadRequest) (transport.LeadResponse, error) {
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

func (s *Service) Update(ctx context.Context, id uuid.UUID, req transport.UpdateLeadRequest) (transport.LeadResponse, error) {
	params := repository.UpdateLeadParams{}

	if req.FirstName != nil {
		params.ConsumerFirstName = req.FirstName
	}
	if req.LastName != nil {
		params.ConsumerLastName = req.LastName
	}
	if req.Phone != nil {
		params.ConsumerPhone = req.Phone
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

	lead, err := s.repo.Update(ctx, id, params)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, ErrLeadNotFound
		}
		return transport.LeadResponse{}, err
	}

	return toLeadResponse(lead), nil
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
