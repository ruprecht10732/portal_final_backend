package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"portal_final_backend/internal/appointments/repository"
	"portal_final_backend/internal/appointments/transport"
	"portal_final_backend/internal/email"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/sanitize"

	"github.com/google/uuid"
)

// Date/time format and error message constants.
const (
	dateFormat           = "2006-01-02"
	errEndTimeAfterStart = "endTime must be after startTime"
)

// LeadAssigner provides minimal lead assignment capabilities for lead visits.
type LeadAssigner interface {
	GetAssignedAgentID(ctx context.Context, leadID uuid.UUID, tenantID uuid.UUID) (*uuid.UUID, error)
	AssignLead(ctx context.Context, leadID uuid.UUID, agentID uuid.UUID, tenantID uuid.UUID) error
}

// Service provides business logic for appointments
type Service struct {
	repo              *repository.Repository
	leadAssigner      LeadAssigner
	emailSender       email.Sender
	sseService        *sse.Service
	eventBus          events.Bus
	reminderScheduler scheduler.ReminderScheduler
}

// New creates a new appointments service
func New(repo *repository.Repository, leadAssigner LeadAssigner, emailSender email.Sender, eventBus events.Bus, reminderScheduler scheduler.ReminderScheduler) *Service {
	return &Service{
		repo:              repo,
		leadAssigner:      leadAssigner,
		emailSender:       emailSender,
		eventBus:          eventBus,
		reminderScheduler: reminderScheduler,
	}
}

// SetSSE sets the SSE service for real-time event broadcasting.
func (s *Service) SetSSE(sseService *sse.Service) {
	s.sseService = sseService
}

// GetNextScheduledVisit returns the next upcoming scheduled lead visit for a lead.
func (s *Service) GetNextScheduledVisit(ctx context.Context, leadID uuid.UUID, tenantID uuid.UUID) (*repository.Appointment, error) {
	return s.repo.GetNextScheduledVisit(ctx, leadID, tenantID)
}

// GetNextRequestedVisit returns the next upcoming requested lead visit for a lead.
func (s *Service) GetNextRequestedVisit(ctx context.Context, leadID uuid.UUID, tenantID uuid.UUID) (*repository.Appointment, error) {
	return s.repo.GetNextRequestedVisit(ctx, leadID, tenantID)
}

// ListLeadVisitsByStatus returns lead visit appointments matching the provided statuses.
func (s *Service) ListLeadVisitsByStatus(ctx context.Context, leadID uuid.UUID, tenantID uuid.UUID, statuses []transport.AppointmentStatus) ([]repository.Appointment, error) {
	if len(statuses) == 0 {
		return []repository.Appointment{}, nil
	}

	statusValues := make([]string, 0, len(statuses))
	for _, status := range statuses {
		statusValues = append(statusValues, string(status))
	}

	return s.repo.ListLeadVisitsByStatus(ctx, leadID, tenantID, statusValues)
}

// publishSSE publishes an SSE event to all org members if the SSE service is available.
func (s *Service) publishSSE(orgID uuid.UUID, event sse.Event) {
	if s.sseService != nil {
		s.sseService.PublishToOrganization(orgID, event)
	}
}

// publishLeadSSE publishes an SSE event to public lead viewers if available.
func (s *Service) publishLeadSSE(leadID *uuid.UUID, event sse.Event) {
	if s.sseService == nil || leadID == nil {
		return
	}
	event.LeadID = *leadID
	s.sseService.PublishToLead(*leadID, event)
}

// Create creates a new appointment
func (s *Service) Create(ctx context.Context, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, req transport.CreateAppointmentRequest) (*transport.AppointmentResponse, error) {
	if err := s.validateLeadVisit(ctx, req, userID, isAdmin, tenantID); err != nil {
		return nil, err
	}

	if !req.EndTime.After(req.StartTime) {
		return nil, apperr.BadRequest(errEndTimeAfterStart)
	}

	if err := s.checkTimeConflict(ctx, tenantID, userID, req.StartTime, req.EndTime, uuid.Nil); err != nil {
		return nil, err
	}

	appt := s.buildAppointment(userID, tenantID, req)
	if err := s.repo.Create(ctx, appt); err != nil {
		return nil, err
	}

	leadInfo := s.getLeadInfoIfPresent(ctx, appt.LeadID, tenantID)
	s.sendConfirmationEmailIfNeeded(ctx, req.SendConfirmationEmail, appt, leadInfo, tenantID)

	resp := appt.ToResponse(leadInfo)

	// Broadcast appointment creation via SSE
	s.publishSSE(tenantID, sse.Event{
		Type:    sse.EventAppointmentCreated,
		Message: fmt.Sprintf("Nieuwe afspraak: %s", appt.Title),
		Data: map[string]interface{}{
			"appointmentId": appt.ID,
			"title":         appt.Title,
			"type":          appt.Type,
			"startTime":     appt.StartTime,
			"endTime":       appt.EndTime,
			"lead":          leadInfo,
		},
	})

	// Notify public lead tracking page (minimal payload)
	s.publishLeadSSE(appt.LeadID, sse.Event{
		Type: sse.EventAppointmentCreated,
		Data: map[string]interface{}{
			"appointmentId": appt.ID,
			"status":        string(appt.Status),
			"startTime":     appt.StartTime,
			"endTime":       appt.EndTime,
		},
	})

	if s.eventBus != nil {
		evt := events.AppointmentCreated{
			BaseEvent:      events.NewBaseEvent(),
			AppointmentID:  appt.ID,
			OrganizationID: appt.OrganizationID,
			LeadID:         appt.LeadID,
			LeadServiceID:  appt.LeadServiceID,
			UserID:         appt.UserID,
			Type:           appt.Type,
			Title:          appt.Title,
			StartTime:      appt.StartTime,
			EndTime:        appt.EndTime,
			Location:       getOptionalString(appt.Location),
		}
		if leadInfo != nil {
			evt.ConsumerName = formatConsumerName(leadInfo.FirstName, leadInfo.LastName)
			evt.ConsumerPhone = leadInfo.Phone
		}
		if appt.LeadID != nil {
			evt.ConsumerEmail = s.getLeadEmail(ctx, *appt.LeadID, tenantID)
		}
		s.eventBus.Publish(ctx, evt)
	}

	if s.reminderScheduler != nil && appt.Type == string(transport.AppointmentTypeLeadVisit) && leadInfo != nil && leadInfo.Phone != "" {
		reminderAt := appt.StartTime.Add(-24 * time.Hour)
		if reminderAt.After(time.Now()) {
			_ = s.reminderScheduler.ScheduleAppointmentReminder(ctx, scheduler.AppointmentReminderPayload{
				AppointmentID:  appt.ID.String(),
				OrganizationID: appt.OrganizationID.String(),
			}, reminderAt)
		}
	}

	return &resp, nil
}

// validateLeadVisit validates lead_visit type requirements.
func (s *Service) validateLeadVisit(ctx context.Context, req transport.CreateAppointmentRequest, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID) error {
	if req.Type != transport.AppointmentTypeLeadVisit {
		return nil
	}

	if req.LeadID == nil || req.LeadServiceID == nil {
		return apperr.BadRequest("lead_visit type requires leadId and leadServiceId")
	}
	if s.leadAssigner == nil {
		return apperr.BadRequest("lead assignment not configured")
	}

	assignedAgentID, err := s.leadAssigner.GetAssignedAgentID(ctx, *req.LeadID, tenantID)
	if err != nil {
		return err
	}

	if assignedAgentID != nil && *assignedAgentID != userID && !isAdmin {
		return apperr.Forbidden("not authorized to schedule visits for this lead")
	}
	if assignedAgentID == nil && !isAdmin {
		return s.leadAssigner.AssignLead(ctx, *req.LeadID, userID, tenantID)
	}
	return nil
}

// checkTimeConflict checks for overlapping appointments, excluding excludeID if non-nil.
func (s *Service) checkTimeConflict(ctx context.Context, tenantID, userID uuid.UUID, startTime, endTime time.Time, excludeID uuid.UUID) error {
	existing, err := s.repo.ListForDateRange(ctx, tenantID, userID, startTime, endTime)
	if err != nil {
		return err
	}
	for _, appt := range existing {
		if excludeID != uuid.Nil && appt.ID == excludeID {
			continue
		}
		if startTime.Before(appt.EndTime) && endTime.After(appt.StartTime) {
			return apperr.Conflict("timeslot already booked")
		}
	}
	return nil
}

// buildAppointment creates a new Appointment from the request.
func (s *Service) buildAppointment(userID, tenantID uuid.UUID, req transport.CreateAppointmentRequest) *repository.Appointment {
	now := time.Now()
	return &repository.Appointment{
		ID:             uuid.New(),
		OrganizationID: tenantID,
		UserID:         userID,
		LeadID:         req.LeadID,
		LeadServiceID:  req.LeadServiceID,
		Type:           string(req.Type),
		Title:          sanitize.Text(req.Title),
		Description:    sanitize.TextPtr(nilIfEmpty(req.Description)),
		Location:       nilIfEmpty(req.Location),
		MeetingLink:    sanitize.TextPtr(nilIfEmpty(req.MeetingLink)),
		StartTime:      req.StartTime,
		EndTime:        req.EndTime,
		Status:         string(transport.AppointmentStatusScheduled),
		AllDay:         req.AllDay,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// getLeadInfoIfPresent returns lead info if leadID is not nil.
func (s *Service) getLeadInfoIfPresent(ctx context.Context, leadID *uuid.UUID, tenantID uuid.UUID) *transport.AppointmentLeadInfo {
	if leadID == nil {
		return nil
	}
	return s.getLeadInfo(ctx, *leadID, tenantID)
}

// sendConfirmationEmailIfNeeded sends confirmation email if conditions are met.
func (s *Service) sendConfirmationEmailIfNeeded(ctx context.Context, sendEmail *bool, appt *repository.Appointment, leadInfo *transport.AppointmentLeadInfo, tenantID uuid.UUID) {
	if sendEmail == nil || !*sendEmail || leadInfo == nil || s.emailSender == nil || appt.LeadID == nil {
		return
	}
	if consumerEmail := s.getLeadEmail(ctx, *appt.LeadID, tenantID); consumerEmail != "" {
		scheduledDate := appt.StartTime.Format("Monday, January 2, 2006 at 15:04")
		_ = s.emailSender.SendVisitInviteEmail(ctx, consumerEmail, leadInfo.FirstName, scheduledDate, leadInfo.Address)
	}
}

// GetByID retrieves an appointment by ID
func (s *Service) GetByID(ctx context.Context, id uuid.UUID, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID) (*transport.AppointmentResponse, error) {
	appt, err := s.ensureAccess(ctx, id, userID, isAdmin, tenantID)
	if err != nil {
		return nil, err
	}

	var leadInfo *transport.AppointmentLeadInfo
	if appt.LeadID != nil {
		leadInfo = s.getLeadInfo(ctx, *appt.LeadID, tenantID)
	}

	resp := appt.ToResponse(leadInfo)
	return &resp, nil
}

// GetByLeadServiceID retrieves the latest non-cancelled appointment for a lead service.
func (s *Service) GetByLeadServiceID(ctx context.Context, leadServiceID uuid.UUID, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID) (*transport.AppointmentResponse, error) {
	appt, err := s.repo.GetByLeadServiceID(ctx, leadServiceID, tenantID)
	if err != nil {
		return nil, err
	}
	if appt == nil {
		return nil, apperr.NotFound("appointment not found")
	}
	if !isAdmin && appt.UserID != userID {
		return nil, apperr.Forbidden("not authorized to access this appointment")
	}

	leadInfo := s.getLeadInfoIfPresent(ctx, appt.LeadID, tenantID)
	resp := appt.ToResponse(leadInfo)
	return &resp, nil
}

// Update updates an appointment
func (s *Service) Update(ctx context.Context, id uuid.UUID, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, req transport.UpdateAppointmentRequest) (*transport.AppointmentResponse, error) {
	appt, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	if !isAdmin && appt.UserID != userID {
		return nil, apperr.Forbidden("not authorized to update this appointment")
	}

	applyAppointmentUpdates(appt, req)

	if !appt.EndTime.After(appt.StartTime) {
		return nil, apperr.BadRequest(errEndTimeAfterStart)
	}

	if req.StartTime != nil || req.EndTime != nil {
		if err := s.checkTimeConflict(ctx, tenantID, appt.UserID, appt.StartTime, appt.EndTime, appt.ID); err != nil {
			return nil, err
		}
	}

	appt.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, appt); err != nil {
		return nil, err
	}

	leadInfo := s.getLeadInfoIfPresent(ctx, appt.LeadID, tenantID)
	resp := appt.ToResponse(leadInfo)

	// Broadcast appointment update via SSE
	s.publishSSE(tenantID, sse.Event{
		Type:    sse.EventAppointmentUpdated,
		Message: fmt.Sprintf("Afspraak bijgewerkt: %s", appt.Title),
		Data: map[string]interface{}{
			"appointmentId": appt.ID,
			"title":         appt.Title,
			"type":          appt.Type,
			"startTime":     appt.StartTime,
			"endTime":       appt.EndTime,
			"lead":          leadInfo,
		},
	})

	// Notify public lead tracking page (minimal payload)
	s.publishLeadSSE(appt.LeadID, sse.Event{
		Type: sse.EventAppointmentUpdated,
		Data: map[string]interface{}{
			"appointmentId": appt.ID,
			"status":        string(appt.Status),
			"startTime":     appt.StartTime,
			"endTime":       appt.EndTime,
		},
	})

	return &resp, nil
}

// applyAppointmentUpdates applies partial updates from the request to the appointment.
func applyAppointmentUpdates(appt *repository.Appointment, req transport.UpdateAppointmentRequest) {
	if req.Title != nil {
		appt.Title = sanitize.Text(*req.Title)
	}
	if req.Description != nil {
		appt.Description = sanitize.TextPtr(req.Description)
	}
	if req.Location != nil {
		appt.Location = req.Location
	}
	if req.MeetingLink != nil {
		appt.MeetingLink = sanitize.TextPtr(nilIfEmpty(*req.MeetingLink))
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
}

// UpdateStatus updates the status of an appointment
func (s *Service) UpdateStatus(ctx context.Context, id uuid.UUID, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, req transport.UpdateAppointmentStatusRequest) (*transport.AppointmentResponse, error) {
	appt, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	// Check ownership (admin can update any)
	if !isAdmin && appt.UserID != userID {
		return nil, apperr.Forbidden("not authorized to update this appointment")
	}

	oldStatus := string(appt.Status)
	if string(req.Status) == oldStatus {
		leadInfo := s.getLeadInfoIfPresent(ctx, appt.LeadID, tenantID)
		resp := appt.ToResponse(leadInfo)
		return &resp, nil
	}

	if err := s.repo.UpdateStatus(ctx, id, tenantID, string(req.Status)); err != nil {
		return nil, err
	}

	// Refetch to get updated data
	appt, err = s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	var leadInfo *transport.AppointmentLeadInfo
	if appt.LeadID != nil {
		leadInfo = s.getLeadInfo(ctx, *appt.LeadID, tenantID)
	}

	resp := appt.ToResponse(leadInfo)

	// Broadcast status change via SSE
	s.publishSSE(tenantID, sse.Event{
		Type:    sse.EventAppointmentStatusChanged,
		Message: fmt.Sprintf("Afspraak status: %s → %s", appt.Title, req.Status),
		Data: map[string]interface{}{
			"appointmentId": appt.ID,
			"title":         appt.Title,
			"type":          appt.Type,
			"status":        string(req.Status),
			"startTime":     appt.StartTime,
			"endTime":       appt.EndTime,
			"lead":          leadInfo,
		},
	})

	// Notify public lead tracking page (minimal payload)
	s.publishLeadSSE(appt.LeadID, sse.Event{
		Type: sse.EventAppointmentStatusChanged,
		Data: map[string]interface{}{
			"appointmentId": appt.ID,
			"status":        string(req.Status),
			"startTime":     appt.StartTime,
			"endTime":       appt.EndTime,
		},
	})

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.AppointmentStatusChanged{
			BaseEvent:      events.NewBaseEvent(),
			AppointmentID:  appt.ID,
			OrganizationID: appt.OrganizationID,
			LeadID:         appt.LeadID,
			LeadServiceID:  appt.LeadServiceID,
			UserID:         appt.UserID,
			OldStatus:      oldStatus,
			NewStatus:      string(req.Status),
		})
	}

	return &resp, nil
}

// Delete removes an appointment
func (s *Service) Delete(ctx context.Context, id uuid.UUID, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID) error {
	appt, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return err
	}

	// Check ownership (admin can delete any)
	if !isAdmin && appt.UserID != userID {
		return apperr.Forbidden("not authorized to delete this appointment")
	}

	if err := s.repo.Delete(ctx, id, tenantID); err != nil {
		return err
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, events.AppointmentDeleted{
			BaseEvent:      events.NewBaseEvent(),
			AppointmentID:  id,
			OrganizationID: tenantID,
			LeadID:         appt.LeadID,
			LeadServiceID:  appt.LeadServiceID,
			UserID:         userID,
		})
	}

	return nil
}

// List retrieves appointments with filtering
func (s *Service) List(ctx context.Context, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, req transport.ListAppointmentsRequest) (*transport.AppointmentListResponse, error) {
	filters, err := parseListFilters(req)
	if err != nil {
		return nil, err
	}

	params := buildListParams(tenantID, req, filters, userID, isAdmin)
	result, err := s.repo.List(ctx, params)
	if err != nil {
		return nil, err
	}

	return s.buildListResponse(ctx, result, tenantID)
}

type listFilterValues struct {
	leadID    *uuid.UUID
	reqUserID *uuid.UUID
	startFrom *time.Time
	startTo   *time.Time
}

func parseListFilters(req transport.ListAppointmentsRequest) (listFilterValues, error) {
	leadID, err := parseUUIDFilter(req.LeadID, "leadId")
	if err != nil {
		return listFilterValues{}, err
	}
	reqUserID, err := parseUUIDFilter(req.UserID, "userId")
	if err != nil {
		return listFilterValues{}, err
	}

	startFrom, err := parseDateFilter(req.StartFrom, "startFrom")
	if err != nil {
		return listFilterValues{}, err
	}
	startTo, err := parseDateFilter(req.StartTo, "startTo")
	if err != nil {
		return listFilterValues{}, err
	}
	if startTo != nil {
		endOfDay := startTo.Add(24*time.Hour - time.Nanosecond)
		startTo = &endOfDay
	}

	return listFilterValues{
		leadID:    leadID,
		reqUserID: reqUserID,
		startFrom: startFrom,
		startTo:   startTo,
	}, nil
}

func buildListParams(tenantID uuid.UUID, req transport.ListAppointmentsRequest, filters listFilterValues, userID uuid.UUID, isAdmin bool) repository.ListParams {
	params := repository.ListParams{
		OrganizationID: tenantID,
		LeadID:         filters.leadID,
		Page:           max(req.Page, 1),
		PageSize:       clampPageSize(req.PageSize),
		Search:         req.Search,
		SortBy:         req.SortBy,
		SortOrder:      req.SortOrder,
		StartFrom:      filters.startFrom,
		StartTo:        filters.startTo,
	}

	params.UserID = resolveUserIDFilter(userID, isAdmin, filters.reqUserID)

	if req.Type != nil {
		t := string(*req.Type)
		params.Type = &t
	}
	if req.Status != nil {
		st := string(*req.Status)
		params.Status = &st
	}

	return params
}

// resolveUserIDFilter determines which user to filter by based on admin status.
func resolveUserIDFilter(currentUser uuid.UUID, isAdmin bool, requestedUser *uuid.UUID) *uuid.UUID {
	if !isAdmin {
		return &currentUser // Non-admins can only see own appointments
	}
	return requestedUser // Admins can filter by any user (or nil for all)
}

// clampPageSize ensures page size is within valid range.
func clampPageSize(size int) int {
	if size < 1 || size > 100 {
		return 50
	}
	return size
}

// buildListResponse converts repository results to transport response.
func (s *Service) buildListResponse(ctx context.Context, result *repository.ListResult, tenantID uuid.UUID) (*transport.AppointmentListResponse, error) {
	// Build lead ID list for batch fetching
	leadIDs := make([]uuid.UUID, 0, len(result.Items))
	for _, appt := range result.Items {
		if appt.LeadID != nil {
			leadIDs = append(leadIDs, *appt.LeadID)
		}
	}

	// Batch fetch lead info
	leadInfoMap, err := s.repo.GetLeadInfoBatch(ctx, leadIDs, tenantID)
	if err != nil {
		return nil, err
	}

	// Convert to responses
	items := make([]transport.AppointmentResponse, len(result.Items))
	for i, appt := range result.Items {
		leadInfo := buildLeadInfo(appt.LeadID, leadInfoMap)
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

// buildLeadInfo constructs lead info from the map if available.
func buildLeadInfo(leadID *uuid.UUID, leadInfoMap map[uuid.UUID]*repository.LeadInfo) *transport.AppointmentLeadInfo {
	if leadID == nil {
		return nil
	}
	info, ok := leadInfoMap[*leadID]
	if !ok || info == nil {
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

// Visit reports
func (s *Service) GetVisitReport(ctx context.Context, appointmentID uuid.UUID, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID) (*transport.AppointmentVisitReportResponse, error) {
	if _, err := s.ensureAccess(ctx, appointmentID, userID, isAdmin, tenantID); err != nil {
		return nil, err
	}

	report, err := s.repo.GetVisitReport(ctx, appointmentID, tenantID)
	if err != nil {
		return nil, err
	}

	return &transport.AppointmentVisitReportResponse{
		AppointmentID:    report.AppointmentID,
		Measurements:     report.Measurements,
		AccessDifficulty: toAccessDifficulty(report.AccessDifficulty),
		Notes:            report.Notes,
		CreatedAt:        report.CreatedAt,
		UpdatedAt:        report.UpdatedAt,
	}, nil
}

func (s *Service) UpsertVisitReport(ctx context.Context, appointmentID uuid.UUID, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, req transport.UpsertVisitReportRequest) (*transport.AppointmentVisitReportResponse, error) {
	appt, err := s.ensureAccess(ctx, appointmentID, userID, isAdmin, tenantID)
	if err != nil {
		return nil, err
	}

	existing, _ := s.repo.GetVisitReport(ctx, appointmentID, tenantID)
	measurements := mergeString(existing, func(r *repository.VisitReport) *string { return r.Measurements }, sanitize.TextPtr(req.Measurements))
	accessDifficulty := mergeString(existing, func(r *repository.VisitReport) *string { return r.AccessDifficulty }, toAccessDifficultyString(req.AccessDifficulty))
	notes := mergeString(existing, func(r *repository.VisitReport) *string { return r.Notes }, sanitize.TextPtr(req.Notes))

	saved, err := s.repo.UpsertVisitReport(ctx, repository.VisitReport{
		AppointmentID:    appointmentID,
		OrganizationID:   tenantID,
		Measurements:     measurements,
		AccessDifficulty: accessDifficulty,
		Notes:            notes,
	})
	if err != nil {
		return nil, err
	}

	leadID := appt.LeadID
	leadServiceID := appt.LeadServiceID
	if s.eventBus != nil && leadID != nil && leadServiceID != nil {
		s.eventBus.Publish(ctx, events.LeadDataChanged{
			BaseEvent:     events.NewBaseEvent(),
			LeadID:        *leadID,
			LeadServiceID: *leadServiceID,
			TenantID:      tenantID,
			Source:        "visit_report",
		})
	}

	return &transport.AppointmentVisitReportResponse{
		AppointmentID:    saved.AppointmentID,
		Measurements:     saved.Measurements,
		AccessDifficulty: toAccessDifficulty(saved.AccessDifficulty),
		Notes:            saved.Notes,
		CreatedAt:        saved.CreatedAt,
		UpdatedAt:        saved.UpdatedAt,
	}, nil
}

// Attachments
func (s *Service) CreateAttachment(ctx context.Context, appointmentID uuid.UUID, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, req transport.CreateAppointmentAttachmentRequest) (*transport.AppointmentAttachmentResponse, error) {
	if _, err := s.ensureAccess(ctx, appointmentID, userID, isAdmin, tenantID); err != nil {
		return nil, err
	}

	attachment := repository.AppointmentAttachment{
		ID:             uuid.New(),
		AppointmentID:  appointmentID,
		OrganizationID: tenantID,
		FileKey:        req.FileKey,
		FileName:       req.FileName,
		ContentType:    req.ContentType,
		SizeBytes:      req.SizeBytes,
	}

	saved, err := s.repo.CreateAttachment(ctx, attachment)
	if err != nil {
		return nil, err
	}

	return &transport.AppointmentAttachmentResponse{
		ID:            saved.ID,
		AppointmentID: saved.AppointmentID,
		FileKey:       saved.FileKey,
		FileName:      saved.FileName,
		ContentType:   saved.ContentType,
		SizeBytes:     saved.SizeBytes,
		CreatedAt:     saved.CreatedAt,
	}, nil
}

func (s *Service) ListAttachments(ctx context.Context, appointmentID uuid.UUID, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID) ([]transport.AppointmentAttachmentResponse, error) {
	if _, err := s.ensureAccess(ctx, appointmentID, userID, isAdmin, tenantID); err != nil {
		return nil, err
	}

	items, err := s.repo.ListAttachments(ctx, appointmentID, tenantID)
	if err != nil {
		return nil, err
	}

	resp := make([]transport.AppointmentAttachmentResponse, len(items))
	for i, item := range items {
		resp[i] = transport.AppointmentAttachmentResponse{
			ID:            item.ID,
			AppointmentID: item.AppointmentID,
			FileKey:       item.FileKey,
			FileName:      item.FileName,
			ContentType:   item.ContentType,
			SizeBytes:     item.SizeBytes,
			CreatedAt:     item.CreatedAt,
		}
	}

	return resp, nil
}

// GetAvailableSlots computes available time slots for a user within a date range
func (s *Service) GetAvailableSlots(ctx context.Context, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, req transport.GetAvailableSlotsRequest) (*transport.AvailableSlotsResponse, error) {
	// Parse and validate inputs
	targetUserID, err := s.resolveTargetUserIDFromString(userID, isAdmin, req.UserID)
	if err != nil {
		return nil, err
	}

	startDate, endDate, err := parseAndValidateDateRange(req.StartDate, req.EndDate, 14)
	if err != nil {
		return nil, err
	}

	slotDuration := max(req.SlotDuration, 60)

	// Fetch availability data
	rules, overrideMap, appointments, err := s.fetchAvailabilityData(ctx, tenantID, targetUserID, startDate, endDate)
	if err != nil {
		return nil, err
	}

	// Generate slots for each day
	days := s.generateDaySlots(startDate, endDate, rules, overrideMap, appointments, slotDuration)

	return &transport.AvailableSlotsResponse{Days: days}, nil
}

// parseAndValidateDateRange parses dates and validates the range.
func parseAndValidateDateRange(startStr, endStr string, maxDays int) (time.Time, time.Time, error) {
	startDate, err := time.Parse(dateFormat, startStr)
	if err != nil {
		return time.Time{}, time.Time{}, apperr.BadRequest("invalid startDate format")
	}
	endDate, err := time.Parse(dateFormat, endStr)
	if err != nil {
		return time.Time{}, time.Time{}, apperr.BadRequest("invalid endDate format")
	}
	if endDate.Before(startDate) {
		return time.Time{}, time.Time{}, apperr.BadRequest("endDate must be after startDate")
	}
	if endDate.Sub(startDate).Hours()/24 > float64(maxDays) {
		return time.Time{}, time.Time{}, apperr.BadRequest(fmt.Sprintf("date range cannot exceed %d days", maxDays))
	}
	return startDate, endDate, nil
}

// fetchAvailabilityData fetches rules, overrides, and appointments for slot generation.
func (s *Service) fetchAvailabilityData(ctx context.Context, tenantID, userID uuid.UUID, startDate, endDate time.Time) ([]repository.AvailabilityRule, map[string]*repository.AvailabilityOverride, []repository.Appointment, error) {
	rules, err := s.repo.ListAvailabilityRules(ctx, tenantID, userID)
	if err != nil {
		return nil, nil, nil, err
	}

	overrides, err := s.repo.ListAvailabilityOverrides(ctx, tenantID, userID, &startDate, &endDate)
	if err != nil {
		return nil, nil, nil, err
	}

	overrideMap := make(map[string]*repository.AvailabilityOverride)
	for i := range overrides {
		overrideMap[overrides[i].Date.Format(dateFormat)] = &overrides[i]
	}

	fetchStart := startDate.AddDate(0, 0, -1)
	fetchEnd := endDate.AddDate(0, 0, 2)
	appointments, err := s.repo.ListForDateRange(ctx, tenantID, userID, fetchStart, fetchEnd)
	if err != nil {
		return nil, nil, nil, err
	}

	return rules, overrideMap, appointments, nil
}

// generateDaySlots generates time slots for each day in the range.
func (s *Service) generateDaySlots(startDate, endDate time.Time, rules []repository.AvailabilityRule, overrideMap map[string]*repository.AvailabilityOverride, appointments []repository.Appointment, slotDuration int) []transport.DaySlots {
	var days []transport.DaySlots

	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		daySlots := s.generateDaySlotsForDate(d, rules, overrideMap, appointments, slotDuration)
		days = append(days, daySlots)
	}

	return days
}

// generateDaySlotsForDate generates slots for a single day.
func (s *Service) generateDaySlotsForDate(d time.Time, rules []repository.AvailabilityRule, overrideMap map[string]*repository.AvailabilityOverride, appointments []repository.Appointment, slotDuration int) transport.DaySlots {
	dateKey := d.Format(dateFormat)
	daySlots := transport.DaySlots{Date: dateKey, Slots: []transport.TimeSlot{}}

	// Check for override
	if override, exists := overrideMap[dateKey]; exists {
		if !override.IsAvailable {
			return daySlots // Day blocked
		}
		if override.StartTime != nil && override.EndTime != nil {
			daySlots.Slots = processTimeWindow(d, override.Timezone, *override.StartTime, *override.EndTime, slotDuration, appointments)
		}
		return daySlots
	}

	// Apply rules for this weekday
	weekday := int(d.Weekday())
	for _, rule := range rules {
		if rule.Weekday == weekday {
			slots := processTimeWindow(d, rule.Timezone, rule.StartTime, rule.EndTime, slotDuration, appointments)
			daySlots.Slots = append(daySlots.Slots, slots...)
		}
	}

	// Sort slots by start time
	sort.Slice(daySlots.Slots, func(i, j int) bool {
		return daySlots.Slots[i].StartTime.Before(daySlots.Slots[j].StartTime)
	})

	return daySlots
}

// processTimeWindow generates slots for a time window on a given date.
func processTimeWindow(d time.Time, tzName string, startClock, endClock time.Time, slotDurationMinutes int, appointments []repository.Appointment) []transport.TimeSlot {
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		loc = time.UTC
	}

	windowStart := time.Date(d.Year(), d.Month(), d.Day(), startClock.Hour(), startClock.Minute(), 0, 0, loc)
	windowEnd := time.Date(d.Year(), d.Month(), d.Day(), endClock.Hour(), endClock.Minute(), 0, 0, loc)

	return generateSlotsForWindow(windowStart.UTC(), windowEnd.UTC(), slotDurationMinutes, appointments)
}

// generateSlotsForWindow generates available slots within a time window (UTC), excluding existing appointments
func generateSlotsForWindow(windowStart, windowEnd time.Time, slotDurationMinutes int, appointments []repository.Appointment) []transport.TimeSlot {
	var slots []transport.TimeSlot
	slotDuration := time.Duration(slotDurationMinutes) * time.Minute

	// Generate slots
	for slotStart := windowStart; slotStart.Add(slotDuration).Before(windowEnd) || slotStart.Add(slotDuration).Equal(windowEnd); slotStart = slotStart.Add(slotDuration) {
		slotEnd := slotStart.Add(slotDuration)

		// Check if slot conflicts with any appointment
		conflicts := false
		for _, appt := range appointments {
			// Check for overlap: slot overlaps if it starts before appt ends AND ends after appt starts
			if slotStart.Before(appt.EndTime) && slotEnd.After(appt.StartTime) {
				conflicts = true
				break
			}
		}

		if !conflicts {
			slots = append(slots, transport.TimeSlot{
				StartTime: slotStart,
				EndTime:   slotEnd,
			})
		}
	}

	return slots
}

func (s *Service) resolveTargetUserIDFromString(userID uuid.UUID, isAdmin bool, target string) (uuid.UUID, error) {
	if target == "" {
		return userID, nil
	}
	parsed, err := uuid.Parse(target)
	if err != nil {
		return uuid.UUID{}, apperr.BadRequest("invalid userId format")
	}
	if !isAdmin && parsed != userID {
		return uuid.UUID{}, apperr.Forbidden("not authorized to view availability for this user")
	}
	return parsed, nil
}

// Availability
func (s *Service) CreateAvailabilityRule(ctx context.Context, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, req transport.CreateAvailabilityRuleRequest) (*transport.AvailabilityRuleResponse, error) {
	targetUserID, err := s.resolveTargetUserID(userID, isAdmin, req.UserID)
	if err != nil {
		return nil, err
	}

	startTime, endTime, timezone, err := parseAvailabilityTimes(req.StartTime, req.EndTime, req.Timezone)
	if err != nil {
		return nil, err
	}

	saved, err := s.repo.CreateAvailabilityRule(ctx, repository.AvailabilityRule{
		ID:             uuid.New(),
		OrganizationID: tenantID,
		UserID:         targetUserID,
		Weekday:        req.Weekday,
		StartTime:      startTime,
		EndTime:        endTime,
		Timezone:       timezone,
	})
	if err != nil {
		return nil, err
	}

	return mapAvailabilityRule(saved), nil
}

func (s *Service) ListAvailabilityRules(ctx context.Context, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, targetUserID *uuid.UUID) ([]transport.AvailabilityRuleResponse, error) {
	resolvedUserID, err := s.resolveTargetUserID(userID, isAdmin, targetUserID)
	if err != nil {
		return nil, err
	}

	items, err := s.repo.ListAvailabilityRules(ctx, tenantID, resolvedUserID)
	if err != nil {
		return nil, err
	}

	resp := make([]transport.AvailabilityRuleResponse, len(items))
	for i := range items {
		resp[i] = *mapAvailabilityRule(&items[i])
	}

	return resp, nil
}

func (s *Service) ListAvailabilityRuleUserIDs(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error) {
	return s.repo.ListAvailabilityRuleUserIDs(ctx, tenantID)
}

func (s *Service) DeleteAvailabilityRule(ctx context.Context, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, id uuid.UUID) error {
	rule, err := s.repo.GetAvailabilityRuleByID(ctx, id, tenantID)
	if err != nil {
		return err
	}
	if !isAdmin && rule.UserID != userID {
		return apperr.Forbidden("not authorized to delete this availability rule")
	}

	return s.repo.DeleteAvailabilityRule(ctx, id, tenantID)
}

func (s *Service) UpdateAvailabilityRule(ctx context.Context, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, id uuid.UUID, req transport.UpdateAvailabilityRuleRequest) (*transport.AvailabilityRuleResponse, error) {
	rule, err := s.repo.GetAvailabilityRuleByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if !isAdmin && rule.UserID != userID {
		return nil, apperr.Forbidden("not authorized to update this availability rule")
	}

	updated, err := applyAvailabilityRuleUpdates(rule, req)
	if err != nil {
		return nil, err
	}

	saved, err := s.repo.UpdateAvailabilityRule(ctx, id, tenantID, updated)
	if err != nil {
		return nil, err
	}

	return mapAvailabilityRule(saved), nil
}

func applyAvailabilityRuleUpdates(rule *repository.AvailabilityRule, req transport.UpdateAvailabilityRuleRequest) (repository.AvailabilityRule, error) {
	updated := *rule
	if req.Weekday != nil {
		updated.Weekday = *req.Weekday
	}

	timezone := updated.Timezone
	if req.Timezone != nil {
		timezone = *req.Timezone
	}

	startTimeStr := updated.StartTime.Format("15:04")
	endTimeStr := updated.EndTime.Format("15:04")
	if req.StartTime != nil {
		startTimeStr = *req.StartTime
	}
	if req.EndTime != nil {
		endTimeStr = *req.EndTime
	}

	startTime, endTime, parsedTimezone, err := parseAvailabilityTimes(startTimeStr, endTimeStr, timezone)
	if err != nil {
		return repository.AvailabilityRule{}, err
	}

	updated.StartTime = startTime
	updated.EndTime = endTime
	updated.Timezone = parsedTimezone
	return updated, nil
}

func (s *Service) CreateAvailabilityOverride(ctx context.Context, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, req transport.CreateAvailabilityOverrideRequest) (*transport.AvailabilityOverrideResponse, error) {
	targetUserID, err := s.resolveTargetUserID(userID, isAdmin, req.UserID)
	if err != nil {
		return nil, err
	}

	date, err := time.Parse(dateFormat, req.Date)
	if err != nil {
		return nil, apperr.BadRequest("invalid date format")
	}

	startTime, endTime, timezone, err := parseAvailabilityOptionalTimes(req.StartTime, req.EndTime, req.Timezone)
	if err != nil {
		return nil, err
	}

	saved, err := s.repo.CreateAvailabilityOverride(ctx, repository.AvailabilityOverride{
		ID:             uuid.New(),
		OrganizationID: tenantID,
		UserID:         targetUserID,
		Date:           date,
		IsAvailable:    req.IsAvailable,
		StartTime:      startTime,
		EndTime:        endTime,
		Timezone:       timezone,
	})
	if err != nil {
		return nil, err
	}

	return mapAvailabilityOverride(saved), nil
}

func (s *Service) ListAvailabilityOverrides(ctx context.Context, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, targetUserID *uuid.UUID, startDate *string, endDate *string) ([]transport.AvailabilityOverrideResponse, error) {
	resolvedUserID, err := s.resolveTargetUserID(userID, isAdmin, targetUserID)
	if err != nil {
		return nil, err
	}

	start, end, err := parseOptionalDateRange(startDate, endDate)
	if err != nil {
		return nil, err
	}

	items, err := s.repo.ListAvailabilityOverrides(ctx, tenantID, resolvedUserID, start, end)
	if err != nil {
		return nil, err
	}

	resp := make([]transport.AvailabilityOverrideResponse, len(items))
	for i := range items {
		resp[i] = *mapAvailabilityOverride(&items[i])
	}

	return resp, nil
}

func (s *Service) DeleteAvailabilityOverride(ctx context.Context, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, id uuid.UUID) error {
	override, err := s.repo.GetAvailabilityOverrideByID(ctx, id, tenantID)
	if err != nil {
		return err
	}
	if !isAdmin && override.UserID != userID {
		return apperr.Forbidden("not authorized to delete this availability override")
	}

	return s.repo.DeleteAvailabilityOverride(ctx, id, tenantID)
}

func (s *Service) UpdateAvailabilityOverride(ctx context.Context, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, id uuid.UUID, req transport.UpdateAvailabilityOverrideRequest) (*transport.AvailabilityOverrideResponse, error) {
	override, err := s.repo.GetAvailabilityOverrideByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if !isAdmin && override.UserID != userID {
		return nil, apperr.Forbidden("not authorized to update this availability override")
	}

	// Apply partial updates
	if req.Date != nil {
		date, err := time.Parse(dateFormat, *req.Date)
		if err != nil {
			return nil, apperr.BadRequest("invalid date format")
		}
		override.Date = date
	}
	if req.IsAvailable != nil {
		override.IsAvailable = *req.IsAvailable
	}

	timezone := override.Timezone
	if req.Timezone != nil {
		timezone = *req.Timezone
	}

	// Handle time updates
	var startTimeStr *string
	var endTimeStr *string
	if override.StartTime != nil {
		str := override.StartTime.Format("15:04")
		startTimeStr = &str
	}
	if override.EndTime != nil {
		str := override.EndTime.Format("15:04")
		endTimeStr = &str
	}
	if req.StartTime != nil {
		startTimeStr = req.StartTime
	}
	if req.EndTime != nil {
		endTimeStr = req.EndTime
	}

	startTime, endTime, parsedTimezone, err := parseAvailabilityOptionalTimes(startTimeStr, endTimeStr, timezone)
	if err != nil {
		return nil, err
	}

	override.StartTime = startTime
	override.EndTime = endTime
	override.Timezone = parsedTimezone

	saved, err := s.repo.UpdateAvailabilityOverride(ctx, id, tenantID, *override)
	if err != nil {
		return nil, err
	}

	return mapAvailabilityOverride(saved), nil
}

// Helper functions

func (s *Service) getLeadInfo(ctx context.Context, leadID uuid.UUID, tenantID uuid.UUID) *transport.AppointmentLeadInfo {
	info, err := s.repo.GetLeadInfo(ctx, leadID, tenantID)
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

func (s *Service) getLeadEmail(ctx context.Context, leadID uuid.UUID, tenantID uuid.UUID) string {
	email, err := s.repo.GetLeadEmail(ctx, leadID, tenantID)
	if err != nil {
		return ""
	}
	return email
}

func (s *Service) ensureAccess(ctx context.Context, appointmentID uuid.UUID, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID) (*repository.Appointment, error) {
	appt, err := s.repo.GetByID(ctx, appointmentID, tenantID)
	if err != nil {
		return nil, err
	}
	if !isAdmin && appt.UserID != userID {
		return nil, apperr.Forbidden("not authorized to access this appointment")
	}
	return appt, nil
}

func toAccessDifficulty(value *string) *transport.AccessDifficulty {
	if value == nil {
		return nil
	}
	converted := transport.AccessDifficulty(*value)
	return &converted
}

func toAccessDifficultyString(value *transport.AccessDifficulty) *string {
	if value == nil {
		return nil
	}
	converted := string(*value)
	return &converted
}

func mergeString(existing *repository.VisitReport, getExisting func(*repository.VisitReport) *string, next *string) *string {
	if next != nil {
		return next
	}
	if existing == nil {
		return nil
	}
	return getExisting(existing)
}

func parseAvailabilityTimes(startTime string, endTime string, timezone string) (time.Time, time.Time, string, error) {
	start, err := time.Parse("15:04", startTime)
	if err != nil {
		return time.Time{}, time.Time{}, "", apperr.BadRequest("invalid startTime format")
	}
	end, err := time.Parse("15:04", endTime)
	if err != nil {
		return time.Time{}, time.Time{}, "", apperr.BadRequest("invalid endTime format")
	}
	if !end.After(start) {
		return time.Time{}, time.Time{}, "", apperr.BadRequest(errEndTimeAfterStart)
	}
	if timezone == "" {
		timezone = "Europe/Amsterdam"
	}
	return start, end, timezone, nil
}

func parseAvailabilityOptionalTimes(startTime *string, endTime *string, timezone string) (*time.Time, *time.Time, string, error) {
	if timezone == "" {
		timezone = "Europe/Amsterdam"
	}
	if startTime == nil && endTime == nil {
		return nil, nil, timezone, nil
	}
	if startTime == nil || endTime == nil {
		return nil, nil, "", apperr.BadRequest("startTime and endTime must both be provided")
	}
	start, err := time.Parse("15:04", *startTime)
	if err != nil {
		return nil, nil, "", apperr.BadRequest("invalid startTime format")
	}
	end, err := time.Parse("15:04", *endTime)
	if err != nil {
		return nil, nil, "", apperr.BadRequest("invalid endTime format")
	}
	if !end.After(start) {
		return nil, nil, "", apperr.BadRequest(errEndTimeAfterStart)
	}
	return &start, &end, timezone, nil
}

func parseOptionalDateRange(startDate *string, endDate *string) (*time.Time, *time.Time, error) {
	var start *time.Time
	var end *time.Time

	if startDate != nil && *startDate != "" {
		parsed, err := time.Parse(dateFormat, *startDate)
		if err != nil {
			return nil, nil, apperr.BadRequest("invalid startDate format")
		}
		start = &parsed
	}
	if endDate != nil && *endDate != "" {
		parsed, err := time.Parse(dateFormat, *endDate)
		if err != nil {
			return nil, nil, apperr.BadRequest("invalid endDate format")
		}
		end = &parsed
	}
	if start != nil && end != nil && start.After(*end) {
		return nil, nil, apperr.BadRequest("startDate must be before or equal to endDate")
	}
	return start, end, nil
}

func (s *Service) resolveTargetUserID(userID uuid.UUID, isAdmin bool, target *uuid.UUID) (uuid.UUID, error) {
	if target == nil {
		return userID, nil
	}
	if !isAdmin && *target != userID {
		return uuid.UUID{}, apperr.Forbidden("not authorized to manage availability for this user")
	}
	return *target, nil
}

func mapAvailabilityRule(rule *repository.AvailabilityRule) *transport.AvailabilityRuleResponse {
	return &transport.AvailabilityRuleResponse{
		ID:        rule.ID,
		UserID:    rule.UserID,
		Weekday:   rule.Weekday,
		StartTime: rule.StartTime.Format("15:04"),
		EndTime:   rule.EndTime.Format("15:04"),
		Timezone:  rule.Timezone,
		CreatedAt: rule.CreatedAt,
		UpdatedAt: rule.UpdatedAt,
	}
}

func mapAvailabilityOverride(override *repository.AvailabilityOverride) *transport.AvailabilityOverrideResponse {
	var startTime *string
	var endTime *string
	if override.StartTime != nil {
		value := override.StartTime.Format("15:04")
		startTime = &value
	}
	if override.EndTime != nil {
		value := override.EndTime.Format("15:04")
		endTime = &value
	}

	return &transport.AvailabilityOverrideResponse{
		ID:          override.ID,
		UserID:      override.UserID,
		Date:        override.Date.Format(dateFormat),
		IsAvailable: override.IsAvailable,
		StartTime:   startTime,
		EndTime:     endTime,
		Timezone:    override.Timezone,
		CreatedAt:   override.CreatedAt,
		UpdatedAt:   override.UpdatedAt,
	}
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func getOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func formatConsumerName(firstName, lastName string) string {
	name := strings.TrimSpace(firstName + " " + lastName)
	if name == "" {
		return "klant"
	}
	return name
}

// parseUUIDFilter parses an optional UUID string filter.
// Returns nil if empty, error if invalid format.
func parseUUIDFilter(s string, fieldName string) (*uuid.UUID, error) {
	if s == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(s)
	if err != nil {
		return nil, apperr.BadRequest(fmt.Sprintf("invalid %s format", fieldName))
	}
	return &parsed, nil
}

// parseDateFilter parses date string in 2006-01-02 format.
// Returns nil if empty, error if invalid format.
func parseDateFilter(s string, fieldName string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(dateFormat, s)
	if err != nil {
		return nil, apperr.BadRequest(fmt.Sprintf("invalid %s date format: %s", fieldName, s))
	}
	return &t, nil
}
