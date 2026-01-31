package service

import (
	"context"
	"fmt"
	"time"

	"portal_final_backend/internal/appointments/repository"
	"portal_final_backend/internal/appointments/transport"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

// Service provides business logic for appointments
type Service struct {
	repo *repository.Repository
}

// New creates a new appointments service
func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

// Create creates a new appointment
func (s *Service) Create(ctx context.Context, userID uuid.UUID, req transport.CreateAppointmentRequest) (*transport.AppointmentResponse, error) {
	// Validate lead_visit type has required fields
	if req.Type == transport.AppointmentTypeLeadVisit {
		if req.LeadID == nil || req.LeadServiceID == nil {
			return nil, apperr.BadRequest("lead_visit type requires leadId and leadServiceId")
		}
	}

	// Validate time range
	if !req.EndTime.After(req.StartTime) {
		return nil, apperr.BadRequest("endTime must be after startTime")
	}

	now := time.Now()
	appt := &repository.Appointment{
		ID:            uuid.New(),
		UserID:        userID,
		LeadID:        req.LeadID,
		LeadServiceID: req.LeadServiceID,
		Type:          string(req.Type),
		Title:         req.Title,
		Description:   nilIfEmpty(req.Description),
		Location:      nilIfEmpty(req.Location),
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		Status:        string(transport.AppointmentStatusScheduled),
		AllDay:        req.AllDay,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.repo.Create(ctx, appt); err != nil {
		return nil, err
	}

	// Get lead info if this is a lead visit
	var leadInfo *transport.AppointmentLeadInfo
	if appt.LeadID != nil {
		leadInfo = s.getLeadInfo(ctx, *appt.LeadID)
	}

	resp := appt.ToResponse(leadInfo)
	return &resp, nil
}

// GetByID retrieves an appointment by ID
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*transport.AppointmentResponse, error) {
	appt, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	var leadInfo *transport.AppointmentLeadInfo
	if appt.LeadID != nil {
		leadInfo = s.getLeadInfo(ctx, *appt.LeadID)
	}

	resp := appt.ToResponse(leadInfo)
	return &resp, nil
}

// Update updates an appointment
func (s *Service) Update(ctx context.Context, id uuid.UUID, userID uuid.UUID, isAdmin bool, req transport.UpdateAppointmentRequest) (*transport.AppointmentResponse, error) {
	appt, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Check ownership (admin can update any)
	if !isAdmin && appt.UserID != userID {
		return nil, apperr.Forbidden("not authorized to update this appointment")
	}

	// Apply updates
	if req.Title != nil {
		appt.Title = *req.Title
	}
	if req.Description != nil {
		appt.Description = req.Description
	}
	if req.Location != nil {
		appt.Location = req.Location
	}
	if req.StartTime != nil {
		appt.StartTime = *req.StartTime
	}
	if req.EndTime != nil {
		appt.EndTime = *req.EndTime
	}
	if req.AllDay != nil {
		appt.AllDay = *req.AllDay
	}

	// Validate time range after updates
	if !appt.EndTime.After(appt.StartTime) {
		return nil, apperr.BadRequest("endTime must be after startTime")
	}

	appt.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, appt); err != nil {
		return nil, err
	}

	var leadInfo *transport.AppointmentLeadInfo
	if appt.LeadID != nil {
		leadInfo = s.getLeadInfo(ctx, *appt.LeadID)
	}

	resp := appt.ToResponse(leadInfo)
	return &resp, nil
}

// UpdateStatus updates the status of an appointment
func (s *Service) UpdateStatus(ctx context.Context, id uuid.UUID, userID uuid.UUID, isAdmin bool, req transport.UpdateAppointmentStatusRequest) (*transport.AppointmentResponse, error) {
	appt, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Check ownership (admin can update any)
	if !isAdmin && appt.UserID != userID {
		return nil, apperr.Forbidden("not authorized to update this appointment")
	}

	if err := s.repo.UpdateStatus(ctx, id, string(req.Status)); err != nil {
		return nil, err
	}

	// Refetch to get updated data
	appt, err = s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	var leadInfo *transport.AppointmentLeadInfo
	if appt.LeadID != nil {
		leadInfo = s.getLeadInfo(ctx, *appt.LeadID)
	}

	resp := appt.ToResponse(leadInfo)
	return &resp, nil
}

// Delete removes an appointment
func (s *Service) Delete(ctx context.Context, id uuid.UUID, userID uuid.UUID, isAdmin bool) error {
	appt, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Check ownership (admin can delete any)
	if !isAdmin && appt.UserID != userID {
		return apperr.Forbidden("not authorized to delete this appointment")
	}

	return s.repo.Delete(ctx, id)
}

// List retrieves appointments with filtering
func (s *Service) List(ctx context.Context, userID uuid.UUID, isAdmin bool, req transport.ListAppointmentsRequest) (*transport.AppointmentListResponse, error) {
	// Build params
	params := repository.ListParams{
		LeadID:   req.LeadID,
		Page:     req.Page,
		PageSize: req.PageSize,
	}

	// Non-admins can only see their own appointments
	if !isAdmin {
		params.UserID = &userID
	} else if req.UserID != nil {
		params.UserID = req.UserID
	}

	if req.Type != nil {
		t := string(*req.Type)
		params.Type = &t
	}

	if req.Status != nil {
		st := string(*req.Status)
		params.Status = &st
	}

	// Parse date filters
	if req.StartFrom != "" {
		t, err := time.Parse("2006-01-02", req.StartFrom)
		if err != nil {
			return nil, apperr.BadRequest(fmt.Sprintf("invalid startFrom date format: %s", req.StartFrom))
		}
		params.StartFrom = &t
	}

	if req.StartTo != "" {
		t, err := time.Parse("2006-01-02", req.StartTo)
		if err != nil {
			return nil, apperr.BadRequest(fmt.Sprintf("invalid startTo date format: %s", req.StartTo))
		}
		// Add a day to include the full end date
		endOfDay := t.Add(24*time.Hour - time.Nanosecond)
		params.StartTo = &endOfDay
	}

	// Default pagination
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 || params.PageSize > 100 {
		params.PageSize = 50
	}

	result, err := s.repo.List(ctx, params)
	if err != nil {
		return nil, err
	}

	// Build lead ID list for batch fetching
	leadIDs := make([]uuid.UUID, 0)
	for _, appt := range result.Items {
		if appt.LeadID != nil {
			leadIDs = append(leadIDs, *appt.LeadID)
		}
	}

	// Batch fetch lead info
	leadInfoMap, err := s.repo.GetLeadInfoBatch(ctx, leadIDs)
	if err != nil {
		return nil, err
	}

	// Convert to responses
	items := make([]transport.AppointmentResponse, len(result.Items))
	for i, appt := range result.Items {
		var leadInfo *transport.AppointmentLeadInfo
		if appt.LeadID != nil {
			if info, ok := leadInfoMap[*appt.LeadID]; ok && info != nil {
				leadInfo = &transport.AppointmentLeadInfo{
					ID:        info.ID,
					FirstName: info.FirstName,
					LastName:  info.LastName,
					Phone:     info.Phone,
					Address:   fmt.Sprintf("%s %s, %s", info.Street, info.HouseNumber, info.City),
				}
			}
		}
		items[i] = appt.ToResponse(leadInfo)
	}

	return &transport.AppointmentListResponse{
		Items:      items,
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
	}, nil
}

// CreateFromLeadVisit creates an appointment from a lead visit scheduling (for sync)
func (s *Service) CreateFromLeadVisit(ctx context.Context, userID uuid.UUID, leadID uuid.UUID, leadServiceID uuid.UUID, scheduledDate time.Time, title string) error {
	// Check if appointment already exists for this service
	existing, err := s.repo.GetByLeadServiceID(ctx, leadServiceID)
	if err != nil {
		return err
	}

	if existing != nil {
		// Update the existing appointment instead
		existing.StartTime = scheduledDate
		existing.EndTime = scheduledDate.Add(1 * time.Hour) // Default 1 hour duration
		existing.UpdatedAt = time.Now()
		return s.repo.Update(ctx, existing)
	}

	// Create new appointment
	now := time.Now()
	appt := &repository.Appointment{
		ID:            uuid.New(),
		UserID:        userID,
		LeadID:        &leadID,
		LeadServiceID: &leadServiceID,
		Type:          string(transport.AppointmentTypeLeadVisit),
		Title:         title,
		Description:   nil,
		Location:      nil,
		StartTime:     scheduledDate,
		EndTime:       scheduledDate.Add(1 * time.Hour), // Default 1 hour duration
		Status:        string(transport.AppointmentStatusScheduled),
		AllDay:        false,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	return s.repo.Create(ctx, appt)
}

// Helper functions

func (s *Service) getLeadInfo(ctx context.Context, leadID uuid.UUID) *transport.AppointmentLeadInfo {
	info, err := s.repo.GetLeadInfo(ctx, leadID)
	if err != nil || info == nil {
		return nil
	}
	return &transport.AppointmentLeadInfo{
		ID:        info.ID,
		FirstName: info.FirstName,
		LastName:  info.LastName,
		Phone:     info.Phone,
		Address:   fmt.Sprintf("%s %s, %s", info.Street, info.HouseNumber, info.City),
	}
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
