// Package management handles lead CRUD operations.
// This is a vertically sliced feature package containing service logic
// for creating, reading, updating, and deleting leads.
package management

import (
	"context"
	"errors"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/phone"

	"github.com/google/uuid"
)

// Repository defines the data access interface needed by the management service.
// This is a consumer-driven interface - only what management needs.
type Repository interface {
	repository.LeadReader
	repository.LeadWriter
	repository.LeadViewTracker
	repository.ActivityLogger
	repository.LeadServiceReader
	repository.LeadServiceWriter
}

// Service handles lead management operations (CRUD).
type Service struct {
	repo Repository
}

// New creates a new lead management service.
func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create creates a new lead.
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

	services, _ := s.repo.ListLeadServices(ctx, lead.ID)
	return ToLeadResponseWithServices(lead, services), nil
}

// GetByID retrieves a lead by ID.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (transport.LeadResponse, error) {
	lead, services, err := s.repo.GetByIDWithServices(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound("lead not found")
		}
		return transport.LeadResponse{}, err
	}

	return ToLeadResponseWithServices(lead, services), nil
}

// Update updates a lead's information.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req transport.UpdateLeadRequest, actorID uuid.UUID, actorRoles []string) (transport.LeadResponse, error) {
	params := repository.UpdateLeadParams{}
	var current repository.Lead
	loadedCurrent := false

	if req.AssigneeID.Set {
		lead, err := s.repo.GetByID(ctx, id)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return transport.LeadResponse{}, apperr.NotFound("lead not found")
			}
			return transport.LeadResponse{}, err
		}
		current = lead
		loadedCurrent = true

		if !hasRole(actorRoles, "admin") {
			if current.AssignedAgentID == nil || *current.AssignedAgentID != actorID {
				return transport.LeadResponse{}, apperr.Forbidden("forbidden")
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

	lead, err := s.repo.Update(ctx, id, params)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound("lead not found")
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

	services, _ := s.repo.ListLeadServices(ctx, lead.ID)
	return ToLeadResponseWithServices(lead, services), nil
}

// Delete soft-deletes a lead.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	err := s.repo.Delete(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return apperr.NotFound("lead not found")
		}
		return err
	}
	return nil
}

// BulkDelete deletes multiple leads.
func (s *Service) BulkDelete(ctx context.Context, ids []uuid.UUID) (int, error) {
	deletedCount, err := s.repo.BulkDelete(ctx, ids)
	if err != nil {
		return 0, err
	}
	if deletedCount == 0 {
		return 0, apperr.NotFound("no leads found to delete")
	}
	return deletedCount, nil
}

// List retrieves a paginated list of leads.
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
		services, _ := s.repo.ListLeadServices(ctx, lead.ID)
		items[i] = ToLeadResponseWithServices(lead, services)
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

// CheckDuplicate checks if a lead with the given phone already exists.
func (s *Service) CheckDuplicate(ctx context.Context, phoneNumber string) (transport.DuplicateCheckResponse, error) {
	lead, err := s.repo.GetByPhone(ctx, phoneNumber)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.DuplicateCheckResponse{IsDuplicate: false}, nil
		}
		return transport.DuplicateCheckResponse{}, err
	}

	resp := ToLeadResponse(lead)
	return transport.DuplicateCheckResponse{
		IsDuplicate:  true,
		ExistingLead: &resp,
	}, nil
}

// Assign assigns or unassigns a lead to an agent.
func (s *Service) Assign(ctx context.Context, id uuid.UUID, assigneeID *uuid.UUID, actorID uuid.UUID, actorRoles []string) (transport.LeadResponse, error) {
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound("lead not found")
		}
		return transport.LeadResponse{}, err
	}

	if !hasRole(actorRoles, "admin") {
		if current.AssignedAgentID == nil || *current.AssignedAgentID != actorID {
			return transport.LeadResponse{}, apperr.Forbidden("forbidden")
		}
	}

	params := repository.UpdateLeadParams{
		AssignedAgentID:    assigneeID,
		AssignedAgentIDSet: true,
	}
	updated, err := s.repo.Update(ctx, id, params)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound("lead not found")
		}
		return transport.LeadResponse{}, err
	}

	_ = s.repo.AddActivity(ctx, id, actorID, "assigned", map[string]interface{}{
		"from": current.AssignedAgentID,
		"to":   assigneeID,
	})

	return ToLeadResponse(updated), nil
}

// SetViewedBy marks a lead as viewed by a user.
func (s *Service) SetViewedBy(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	return s.repo.SetViewedBy(ctx, id, userID)
}

// AddService adds a new service to an existing lead.
func (s *Service) AddService(ctx context.Context, leadID uuid.UUID, req transport.AddServiceRequest) (transport.LeadResponse, error) {
	lead, err := s.repo.GetByID(ctx, leadID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound("lead not found")
		}
		return transport.LeadResponse{}, err
	}

	if req.CloseCurrentStatus {
		if err := s.repo.CloseAllActiveServices(ctx, leadID); err != nil {
			return transport.LeadResponse{}, err
		}
	}

	_, err = s.repo.CreateLeadService(ctx, repository.CreateLeadServiceParams{
		LeadID:      leadID,
		ServiceType: string(req.ServiceType),
	})
	if err != nil {
		return transport.LeadResponse{}, err
	}

	services, _ := s.repo.ListLeadServices(ctx, leadID)
	return ToLeadResponseWithServices(lead, services), nil
}

// UpdateServiceStatus updates the status of a specific service.
func (s *Service) UpdateServiceStatus(ctx context.Context, leadID uuid.UUID, serviceID uuid.UUID, req transport.UpdateServiceStatusRequest) (transport.LeadResponse, error) {
	svc, err := s.repo.GetLeadServiceByID(ctx, serviceID)
	if err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) {
			return transport.LeadResponse{}, apperr.NotFound("lead service not found")
		}
		return transport.LeadResponse{}, err
	}
	if svc.LeadID != leadID {
		return transport.LeadResponse{}, apperr.NotFound("lead service not found")
	}

	_, err = s.repo.UpdateServiceStatus(ctx, serviceID, string(req.Status))
	if err != nil {
		return transport.LeadResponse{}, err
	}

	return s.GetByID(ctx, leadID)
}

// UpdateStatus updates the status of the lead's current service.
func (s *Service) UpdateStatus(ctx context.Context, id uuid.UUID, req transport.UpdateLeadStatusRequest) (transport.LeadResponse, error) {
	lead, err := s.repo.UpdateStatus(ctx, id, string(req.Status))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound("lead not found")
		}
		return transport.LeadResponse{}, err
	}

	return ToLeadResponse(lead), nil
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
