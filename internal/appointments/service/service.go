package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"portal_final_backend/internal/appointments/repository"
	"portal_final_backend/internal/appointments/transport"
	"portal_final_backend/internal/email"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/sanitize"

	"github.com/google/uuid"
)

// LeadAssigner provides minimal lead assignment capabilities for lead visits.
type LeadAssigner interface {
	GetAssignedAgentID(ctx context.Context, leadID uuid.UUID, tenantID uuid.UUID) (*uuid.UUID, error)
	AssignLead(ctx context.Context, leadID uuid.UUID, agentID uuid.UUID, tenantID uuid.UUID) error
}

// Service provides business logic for appointments
type Service struct {
	repo         *repository.Repository
	leadAssigner LeadAssigner
	emailSender  email.Sender
}

// New creates a new appointments service
func New(repo *repository.Repository, leadAssigner LeadAssigner, emailSender email.Sender) *Service {
	return &Service{repo: repo, leadAssigner: leadAssigner, emailSender: emailSender}
}

// Create creates a new appointment
func (s *Service) Create(ctx context.Context, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, req transport.CreateAppointmentRequest) (*transport.AppointmentResponse, error) {
	// Validate lead_visit type has required fields
	if req.Type == transport.AppointmentTypeLeadVisit {
		if req.LeadID == nil || req.LeadServiceID == nil {
			return nil, apperr.BadRequest("lead_visit type requires leadId and leadServiceId")
		}
		if s.leadAssigner == nil {
			return nil, apperr.BadRequest("lead assignment not configured")
		}

		assignedAgentID, err := s.leadAssigner.GetAssignedAgentID(ctx, *req.LeadID, tenantID)
		if err != nil {
			return nil, err
		}

		if assignedAgentID != nil && *assignedAgentID != userID && !isAdmin {
			return nil, apperr.Forbidden("not authorized to schedule visits for this lead")
		}
		if assignedAgentID == nil && !isAdmin {
			if err := s.leadAssigner.AssignLead(ctx, *req.LeadID, userID, tenantID); err != nil {
				return nil, err
			}
		}
	}

	// Validate time range
	if !req.EndTime.After(req.StartTime) {
		return nil, apperr.BadRequest("endTime must be after startTime")
	}

	now := time.Now()
	appt := &repository.Appointment{
		ID:             uuid.New(),
		OrganizationID: tenantID,
		UserID:         userID,
		LeadID:         req.LeadID,
		LeadServiceID:  req.LeadServiceID,
		Type:           string(req.Type),
		Title:          sanitize.Text(req.Title),
		Description:    sanitize.TextPtr(nilIfEmpty(req.Description)),
		Location:       nilIfEmpty(req.Location),
		StartTime:      req.StartTime,
		EndTime:        req.EndTime,
		Status:         string(transport.AppointmentStatusScheduled),
		AllDay:         req.AllDay,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.repo.Create(ctx, appt); err != nil {
		return nil, err
	}

	// Get lead info if this is a lead visit
	var leadInfo *transport.AppointmentLeadInfo
	if appt.LeadID != nil {
		leadInfo = s.getLeadInfo(ctx, *appt.LeadID, tenantID)
	}

	// Send confirmation email if requested and this is a lead visit with lead info
	if req.SendConfirmationEmail != nil && *req.SendConfirmationEmail && leadInfo != nil && s.emailSender != nil {
		// Get consumer email from lead info (we need to fetch it separately)
		if consumerEmail := s.getLeadEmail(ctx, *appt.LeadID, tenantID); consumerEmail != "" {
			scheduledDate := appt.StartTime.Format("Monday, January 2, 2006 at 15:04")
			_ = s.emailSender.SendVisitInviteEmail(ctx, consumerEmail, leadInfo.FirstName, scheduledDate, leadInfo.Address)
			// We don't fail the appointment creation if email fails
		}
	}

	resp := appt.ToResponse(leadInfo)
	return &resp, nil
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

// Update updates an appointment
func (s *Service) Update(ctx context.Context, id uuid.UUID, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, req transport.UpdateAppointmentRequest) (*transport.AppointmentResponse, error) {
	appt, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	// Check ownership (admin can update any)
	if !isAdmin && appt.UserID != userID {
		return nil, apperr.Forbidden("not authorized to update this appointment")
	}

	// Apply updates (sanitize user input)
	if req.Title != nil {
		appt.Title = sanitize.Text(*req.Title)
	}
	if req.Description != nil {
		appt.Description = sanitize.TextPtr(req.Description)
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
		leadInfo = s.getLeadInfo(ctx, *appt.LeadID, tenantID)
	}

	resp := appt.ToResponse(leadInfo)
	return &resp, nil
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

	return s.repo.Delete(ctx, id, tenantID)
}

// List retrieves appointments with filtering
func (s *Service) List(ctx context.Context, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, req transport.ListAppointmentsRequest) (*transport.AppointmentListResponse, error) {
	// Apply pagination defaults
	page := req.Page
	if page == 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize == 0 {
		pageSize = 20
	}

	// Parse UUID filters
	var leadID *uuid.UUID
	if req.LeadID != "" {
		parsed, err := uuid.Parse(req.LeadID)
		if err != nil {
			return nil, apperr.BadRequest("invalid leadId format")
		}
		leadID = &parsed
	}

	var reqUserID *uuid.UUID
	if req.UserID != "" {
		parsed, err := uuid.Parse(req.UserID)
		if err != nil {
			return nil, apperr.BadRequest("invalid userId format")
		}
		reqUserID = &parsed
	}

	// Build params
	params := repository.ListParams{
		OrganizationID: tenantID,
		LeadID:         leadID,
		Page:           page,
		PageSize:       pageSize,
		Search:         req.Search,
		SortBy:         req.SortBy,
		SortOrder:      req.SortOrder,
	}

	// Non-admins can only see their own appointments
	if !isAdmin {
		params.UserID = &userID
	} else if reqUserID != nil {
		params.UserID = reqUserID
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
	leadInfoMap, err := s.repo.GetLeadInfoBatch(ctx, leadIDs, tenantID)
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
	if _, err := s.ensureAccess(ctx, appointmentID, userID, isAdmin, tenantID); err != nil {
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
	// Parse target user ID
	targetUserID, err := s.resolveTargetUserIDFromString(userID, isAdmin, req.UserID)
	if err != nil {
		return nil, err
	}

	// Parse dates
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return nil, apperr.BadRequest("invalid startDate format")
	}
	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		return nil, apperr.BadRequest("invalid endDate format")
	}

	// Validate date range
	if endDate.Before(startDate) {
		return nil, apperr.BadRequest("endDate must be after startDate")
	}
	maxDays := 14
	if endDate.Sub(startDate).Hours()/24 > float64(maxDays) {
		return nil, apperr.BadRequest(fmt.Sprintf("date range cannot exceed %d days", maxDays))
	}

	// Default slot duration
	slotDuration := req.SlotDuration
	if slotDuration <= 0 {
		slotDuration = 60
	}

	// Fetch availability rules for the user
	rules, err := s.repo.ListAvailabilityRules(ctx, tenantID, targetUserID)
	if err != nil {
		return nil, err
	}

	// Fetch overrides for the date range
	overrides, err := s.repo.ListAvailabilityOverrides(ctx, tenantID, targetUserID, &startDate, &endDate)
	if err != nil {
		return nil, err
	}

	// Build override map (date string -> override)
	overrideMap := make(map[string]*repository.AvailabilityOverride)
	for i := range overrides {
		dateKey := overrides[i].Date.Format("2006-01-02")
		overrideMap[dateKey] = &overrides[i]
	}

	// Fetch existing appointments in the date range (extend slightly to catch boundary overlaps)
	fetchStart := startDate.AddDate(0, 0, -1)
	fetchEnd := endDate.AddDate(0, 0, 2)
	appointments, err := s.repo.ListForDateRange(ctx, tenantID, targetUserID, fetchStart, fetchEnd)
	if err != nil {
		return nil, err
	}

	// Helper to process a time window in a specific timezone
	processWindow := func(d time.Time, tzName string, startClock, endClock time.Time, slotDurationMinutes int) []transport.TimeSlot {
		loc, err := time.LoadLocation(tzName)
		if err != nil {
			// Fallback to UTC if timezone is invalid
			loc = time.UTC
		}

		// Construct absolute start/end times in the rule's timezone
		windowStart := time.Date(d.Year(), d.Month(), d.Day(), startClock.Hour(), startClock.Minute(), 0, 0, loc)
		windowEnd := time.Date(d.Year(), d.Month(), d.Day(), endClock.Hour(), endClock.Minute(), 0, 0, loc)

		// Convert to UTC for conflict checking
		return generateSlotsForWindow(windowStart.UTC(), windowEnd.UTC(), slotDurationMinutes, appointments)
	}

	// Generate slots for each day
	var days []transport.DaySlots
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		dateKey := d.Format("2006-01-02")
		weekday := int(d.Weekday())

		daySlots := transport.DaySlots{
			Date:  dateKey,
			Slots: []transport.TimeSlot{},
		}

		// Check for override on this day
		if override, exists := overrideMap[dateKey]; exists {
			if !override.IsAvailable {
				// Day is blocked
				days = append(days, daySlots)
				continue
			}
			// Override with custom hours
			if override.StartTime != nil && override.EndTime != nil {
				slots := processWindow(d, override.Timezone, *override.StartTime, *override.EndTime, slotDuration)
				daySlots.Slots = slots
			}
			days = append(days, daySlots)
			continue
		}

		// Find rules for this weekday and generate slots
		for _, rule := range rules {
			if rule.Weekday == weekday {
				slots := processWindow(d, rule.Timezone, rule.StartTime, rule.EndTime, slotDuration)
				daySlots.Slots = append(daySlots.Slots, slots...)
			}
		}

		// Sort slots by start time
		sort.Slice(daySlots.Slots, func(i, j int) bool {
			return daySlots.Slots[i].StartTime.Before(daySlots.Slots[j].StartTime)
		})

		days = append(days, daySlots)
	}

	return &transport.AvailableSlotsResponse{Days: days}, nil
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
	for i, item := range items {
		resp[i] = *mapAvailabilityRule(&item)
	}

	return resp, nil
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

	// Apply partial updates
	if req.Weekday != nil {
		rule.Weekday = *req.Weekday
	}

	timezone := rule.Timezone
	if req.Timezone != nil {
		timezone = *req.Timezone
	}

	startTimeStr := rule.StartTime.Format("15:04")
	endTimeStr := rule.EndTime.Format("15:04")
	if req.StartTime != nil {
		startTimeStr = *req.StartTime
	}
	if req.EndTime != nil {
		endTimeStr = *req.EndTime
	}

	startTime, endTime, parsedTimezone, err := parseAvailabilityTimes(startTimeStr, endTimeStr, timezone)
	if err != nil {
		return nil, err
	}

	rule.StartTime = startTime
	rule.EndTime = endTime
	rule.Timezone = parsedTimezone

	saved, err := s.repo.UpdateAvailabilityRule(ctx, id, tenantID, *rule)
	if err != nil {
		return nil, err
	}

	return mapAvailabilityRule(saved), nil
}

func (s *Service) CreateAvailabilityOverride(ctx context.Context, userID uuid.UUID, isAdmin bool, tenantID uuid.UUID, req transport.CreateAvailabilityOverrideRequest) (*transport.AvailabilityOverrideResponse, error) {
	targetUserID, err := s.resolveTargetUserID(userID, isAdmin, req.UserID)
	if err != nil {
		return nil, err
	}

	date, err := time.Parse("2006-01-02", req.Date)
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
	for i, item := range items {
		resp[i] = *mapAvailabilityOverride(&item)
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
		date, err := time.Parse("2006-01-02", *req.Date)
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
		return time.Time{}, time.Time{}, "", apperr.BadRequest("endTime must be after startTime")
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
		return nil, nil, "", apperr.BadRequest("endTime must be after startTime")
	}
	return &start, &end, timezone, nil
}

func parseOptionalDateRange(startDate *string, endDate *string) (*time.Time, *time.Time, error) {
	var start *time.Time
	var end *time.Time

	if startDate != nil && *startDate != "" {
		parsed, err := time.Parse("2006-01-02", *startDate)
		if err != nil {
			return nil, nil, apperr.BadRequest("invalid startDate format")
		}
		start = &parsed
	}
	if endDate != nil && *endDate != "" {
		parsed, err := time.Parse("2006-01-02", *endDate)
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
		Date:        override.Date.Format("2006-01-02"),
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
