package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	appointmentsdb "portal_final_backend/internal/appointments/db"
	"portal_final_backend/internal/appointments/transport"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Appointment represents the appointment database model
type Appointment struct {
	ID             uuid.UUID  `db:"id"`
	OrganizationID uuid.UUID  `db:"organization_id"`
	UserID         uuid.UUID  `db:"user_id"`
	LeadID         *uuid.UUID `db:"lead_id"`
	LeadServiceID  *uuid.UUID `db:"lead_service_id"`
	Type           string     `db:"type"`
	Title          string     `db:"title"`
	Description    *string    `db:"description"`
	Location       *string    `db:"location"`
	MeetingLink    *string    `db:"meeting_link"`
	StartTime      time.Time  `db:"start_time"`
	EndTime        time.Time  `db:"end_time"`
	Status         string     `db:"status"`
	AllDay         bool       `db:"all_day"`
	CreatedAt      time.Time  `db:"created_at"`
	UpdatedAt      time.Time  `db:"updated_at"`
}

// LeadInfo represents basic lead information for embedding in appointment responses
type LeadInfo struct {
	ID          uuid.UUID `db:"id"`
	FirstName   string    `db:"first_name"`
	LastName    string    `db:"last_name"`
	Phone       string    `db:"phone"`
	Street      string    `db:"street"`
	HouseNumber string    `db:"house_number"`
	City        string    `db:"city"`
}

// Repository provides database operations for RAC_appointments
type Repository struct {
	pool    *pgxpool.Pool
	queries *appointmentsdb.Queries
}

const appointmentNotFoundMsg = "appointment not found"

// New creates a new RAC_appointments repository
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: appointmentsdb.New(pool)}
}

func toPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func toPgUUIDPtr(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return toPgUUID(*id)
}

func toPgUUIDSlice(ids []uuid.UUID) []pgtype.UUID {
	items := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		items = append(items, toPgUUID(id))
	}
	return items
}

func toPgText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func toPgTextValue(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

func toPgTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func toPgTimestampPtr(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return toPgTimestamp(*value)
}

func toPgDate(value time.Time) pgtype.Date {
	dateOnly := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
	return pgtype.Date{Time: dateOnly, Valid: true}
}

func toPgDatePtr(value *time.Time) pgtype.Date {
	if value == nil {
		return pgtype.Date{}
	}
	return toPgDate(*value)
}

func toPgTimeOfDay(value time.Time) pgtype.Time {
	microseconds := int64(value.Hour())*int64(time.Hour/time.Microsecond) +
		int64(value.Minute())*int64(time.Minute/time.Microsecond) +
		int64(value.Second())*int64(time.Second/time.Microsecond) +
		int64(value.Nanosecond()/1000)
	return pgtype.Time{Microseconds: microseconds, Valid: true}
}

func toPgTimeOfDayPtr(value *time.Time) pgtype.Time {
	if value == nil {
		return pgtype.Time{}
	}
	return toPgTimeOfDay(*value)
}

func optionalUUID(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := uuid.UUID(value.Bytes)
	return &id
}

func optionalString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func appointmentFromModel(model appointmentsdb.RacAppointment) Appointment {
	return Appointment{
		ID:             uuid.UUID(model.ID.Bytes),
		OrganizationID: uuid.UUID(model.OrganizationID.Bytes),
		UserID:         uuid.UUID(model.UserID.Bytes),
		LeadID:         optionalUUID(model.LeadID),
		LeadServiceID:  optionalUUID(model.LeadServiceID),
		Type:           model.Type,
		Title:          model.Title,
		Description:    optionalString(model.Description),
		Location:       optionalString(model.Location),
		MeetingLink:    optionalString(model.MeetingLink),
		StartTime:      model.StartTime.Time,
		EndTime:        model.EndTime.Time,
		Status:         model.Status,
		AllDay:         model.AllDay,
		CreatedAt:      model.CreatedAt.Time,
		UpdatedAt:      model.UpdatedAt.Time,
	}
}

func leadInfoFromModel(model appointmentsdb.RacLead) LeadInfo {
	return LeadInfo{
		ID:          uuid.UUID(model.ID.Bytes),
		FirstName:   model.ConsumerFirstName,
		LastName:    model.ConsumerLastName,
		Phone:       model.ConsumerPhone,
		Street:      model.AddressStreet,
		HouseNumber: model.AddressHouseNumber,
		City:        model.AddressCity,
	}
}

// Create inserts a new appointment
func (r *Repository) Create(ctx context.Context, appt *Appointment) error {
	err := r.queries.CreateAppointment(ctx, appointmentsdb.CreateAppointmentParams{
		ID:             toPgUUID(appt.ID),
		OrganizationID: toPgUUID(appt.OrganizationID),
		UserID:         toPgUUID(appt.UserID),
		LeadID:         toPgUUIDPtr(appt.LeadID),
		LeadServiceID:  toPgUUIDPtr(appt.LeadServiceID),
		Type:           appt.Type,
		Title:          appt.Title,
		Description:    toPgText(appt.Description),
		Location:       toPgText(appt.Location),
		MeetingLink:    toPgText(appt.MeetingLink),
		StartTime:      toPgTimestamp(appt.StartTime),
		EndTime:        toPgTimestamp(appt.EndTime),
		Status:         appt.Status,
		AllDay:         appt.AllDay,
		CreatedAt:      toPgTimestamp(appt.CreatedAt),
		UpdatedAt:      toPgTimestamp(appt.UpdatedAt),
	})
	if err != nil {
		return fmt.Errorf("failed to create appointment: %w", err)
	}

	return nil
}

// GetByID retrieves an appointment by its ID
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (*Appointment, error) {
	row, err := r.queries.GetAppointmentByID(ctx, appointmentsdb.GetAppointmentByIDParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound(appointmentNotFoundMsg)
		}
		return nil, fmt.Errorf("failed to get appointment: %w", err)
	}

	appt := appointmentFromModel(appointmentsdb.RacAppointment{ID: row.ID, UserID: row.UserID, LeadID: row.LeadID, LeadServiceID: row.LeadServiceID, Type: row.Type, Title: row.Title, Description: row.Description, Location: row.Location, StartTime: row.StartTime, EndTime: row.EndTime, Status: row.Status, AllDay: row.AllDay, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID, MeetingLink: row.MeetingLink})
	return &appt, nil
}

// GetByLeadServiceID retrieves an appointment by lead service ID (for sync)
func (r *Repository) GetByLeadServiceID(ctx context.Context, leadServiceID uuid.UUID, organizationID uuid.UUID) (*Appointment, error) {
	row, err := r.queries.GetAppointmentByLeadServiceID(ctx, appointmentsdb.GetAppointmentByLeadServiceIDParams{LeadServiceID: toPgUUID(leadServiceID), OrganizationID: toPgUUID(organizationID)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found is acceptable
		}
		return nil, fmt.Errorf("failed to get appointment by lead service: %w", err)
	}

	appt := appointmentFromModel(appointmentsdb.RacAppointment{ID: row.ID, UserID: row.UserID, LeadID: row.LeadID, LeadServiceID: row.LeadServiceID, Type: row.Type, Title: row.Title, Description: row.Description, Location: row.Location, StartTime: row.StartTime, EndTime: row.EndTime, Status: row.Status, AllDay: row.AllDay, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID, MeetingLink: row.MeetingLink})
	return &appt, nil
}

// GetNextScheduledVisit returns the next upcoming scheduled lead visit for a lead.
func (r *Repository) GetNextScheduledVisit(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (*Appointment, error) {
	row, err := r.queries.GetNextUpcomingScheduledVisitByLead(ctx, appointmentsdb.GetNextUpcomingScheduledVisitByLeadParams{LeadID: toPgUUID(leadID), OrganizationID: toPgUUID(organizationID)})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("failed to get next scheduled visit: %w", err)
		}
		latestRow, latestErr := r.queries.GetLatestScheduledVisitByLead(ctx, appointmentsdb.GetLatestScheduledVisitByLeadParams{LeadID: toPgUUID(leadID), OrganizationID: toPgUUID(organizationID)})
		err = latestErr
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to get latest scheduled visit: %w", err)
		}
		appt := appointmentFromModel(appointmentsdb.RacAppointment{ID: latestRow.ID, UserID: latestRow.UserID, LeadID: latestRow.LeadID, LeadServiceID: latestRow.LeadServiceID, Type: latestRow.Type, Title: latestRow.Title, Description: latestRow.Description, Location: latestRow.Location, StartTime: latestRow.StartTime, EndTime: latestRow.EndTime, Status: latestRow.Status, AllDay: latestRow.AllDay, CreatedAt: latestRow.CreatedAt, UpdatedAt: latestRow.UpdatedAt, OrganizationID: latestRow.OrganizationID, MeetingLink: latestRow.MeetingLink})
		return &appt, nil
	}

	appt := appointmentFromModel(appointmentsdb.RacAppointment{ID: row.ID, UserID: row.UserID, LeadID: row.LeadID, LeadServiceID: row.LeadServiceID, Type: row.Type, Title: row.Title, Description: row.Description, Location: row.Location, StartTime: row.StartTime, EndTime: row.EndTime, Status: row.Status, AllDay: row.AllDay, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID, MeetingLink: row.MeetingLink})
	return &appt, nil
}

// GetNextRequestedVisit returns the next upcoming requested lead visit for a lead.
func (r *Repository) GetNextRequestedVisit(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (*Appointment, error) {
	row, err := r.queries.GetNextRequestedVisitByLead(ctx, appointmentsdb.GetNextRequestedVisitByLeadParams{LeadID: toPgUUID(leadID), OrganizationID: toPgUUID(organizationID)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get requested appointment: %w", err)
	}

	appt := appointmentFromModel(appointmentsdb.RacAppointment{ID: row.ID, UserID: row.UserID, LeadID: row.LeadID, LeadServiceID: row.LeadServiceID, Type: row.Type, Title: row.Title, Description: row.Description, Location: row.Location, StartTime: row.StartTime, EndTime: row.EndTime, Status: row.Status, AllDay: row.AllDay, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID, MeetingLink: row.MeetingLink})
	return &appt, nil
}

// ListLeadVisitsByStatus returns lead visit appointments matching the provided statuses.
func (r *Repository) ListLeadVisitsByStatus(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID, statuses []string) ([]Appointment, error) {
	if len(statuses) == 0 {
		return []Appointment{}, nil
	}

	rows, err := r.queries.ListLeadVisitsByStatus(ctx, appointmentsdb.ListLeadVisitsByStatusParams{LeadID: toPgUUID(leadID), OrganizationID: toPgUUID(organizationID), Column3: statuses})
	if err != nil {
		return nil, fmt.Errorf("failed to list lead visits: %w", err)
	}

	items := make([]Appointment, 0, len(rows))
	for _, row := range rows {
		items = append(items, appointmentFromModel(appointmentsdb.RacAppointment{ID: row.ID, UserID: row.UserID, LeadID: row.LeadID, LeadServiceID: row.LeadServiceID, Type: row.Type, Title: row.Title, Description: row.Description, Location: row.Location, StartTime: row.StartTime, EndTime: row.EndTime, Status: row.Status, AllDay: row.AllDay, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID, MeetingLink: row.MeetingLink}))
	}

	return items, nil
}

// Update updates an existing appointment
func (r *Repository) Update(ctx context.Context, appt *Appointment) error {
	result, err := r.queries.UpdateAppointment(ctx, appointmentsdb.UpdateAppointmentParams{
		ID:             toPgUUID(appt.ID),
		Title:          appt.Title,
		Description:    toPgText(appt.Description),
		Location:       toPgText(appt.Location),
		MeetingLink:    toPgText(appt.MeetingLink),
		StartTime:      toPgTimestamp(appt.StartTime),
		EndTime:        toPgTimestamp(appt.EndTime),
		AllDay:         appt.AllDay,
		UpdatedAt:      toPgTimestamp(appt.UpdatedAt),
		OrganizationID: toPgUUID(appt.OrganizationID),
	})
	if err != nil {
		return fmt.Errorf("failed to update appointment: %w", err)
	}

	if result == 0 {
		return apperr.NotFound(appointmentNotFoundMsg)
	}

	return nil
}

// UpdateStatus updates the status of an appointment
func (r *Repository) UpdateStatus(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, status string) error {
	result, err := r.queries.UpdateAppointmentStatus(ctx, appointmentsdb.UpdateAppointmentStatusParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID), Status: status, UpdatedAt: toPgTimestamp(time.Now())})
	if err != nil {
		return fmt.Errorf("failed to update appointment status: %w", err)
	}

	if result == 0 {
		return apperr.NotFound(appointmentNotFoundMsg)
	}

	return nil
}

// Delete removes an appointment
func (r *Repository) Delete(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) error {
	result, err := r.queries.DeleteAppointment(ctx, appointmentsdb.DeleteAppointmentParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID)})
	if err != nil {
		return fmt.Errorf("failed to delete appointment: %w", err)
	}

	if result == 0 {
		return apperr.NotFound(appointmentNotFoundMsg)
	}

	return nil
}

// ListParams contains parameters for listing RAC_appointments
type ListParams struct {
	OrganizationID uuid.UUID
	UserID         *uuid.UUID
	LeadID         *uuid.UUID
	Type           *string
	Status         *string
	StartFrom      *time.Time
	StartTo        *time.Time
	Search         string
	SortBy         string
	SortOrder      string
	Page           int
	PageSize       int
}

// ListResult contains the result of listing RAC_appointments
type ListResult struct {
	Items      []Appointment
	Total      int
	Page       int
	PageSize   int
	TotalPages int
}

// List retrieves RAC_appointments with optional filtering
func (r *Repository) List(ctx context.Context, params ListParams) (*ListResult, error) {
	filters := appointmentListFilters{
		userID:    toPgUUIDPtr(params.UserID),
		leadID:    toPgUUIDPtr(params.LeadID),
		typeValue: toPgText(params.Type),
		status:    toPgText(params.Status),
		startFrom: toPgTimestampPtr(params.StartFrom),
		startTo:   toPgTimestampPtr(params.StartTo),
		search:    optionalSearchParam(params.Search),
	}

	sortBy, err := resolveAppointmentSortBy(params.SortBy)
	if err != nil {
		return nil, err
	}

	sortOrder, err := resolveAppointmentSortOrder(params.SortOrder)
	if err != nil {
		return nil, err
	}

	total, err := r.queries.CountAppointments(ctx, appointmentsdb.CountAppointmentsParams{
		OrganizationID: toPgUUID(params.OrganizationID),
		UserID:         filters.userID,
		LeadID:         filters.leadID,
		Type:           filters.typeValue,
		Status:         filters.status,
		StartFrom:      filters.startFrom,
		StartTo:        filters.startTo,
		Search:         filters.search,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to count RAC_appointments: %w", err)
	}

	totalPages := (int(total) + params.PageSize - 1) / params.PageSize
	offset := (params.Page - 1) * params.PageSize
	rows, err := r.queries.ListAppointments(ctx, appointmentsdb.ListAppointmentsParams{
		OrganizationID: toPgUUID(params.OrganizationID),
		UserID:         filters.userID,
		LeadID:         filters.leadID,
		Type:           filters.typeValue,
		Status:         filters.status,
		StartFrom:      filters.startFrom,
		StartTo:        filters.startTo,
		Search:         filters.search,
		SortBy:         sortBy,
		SortOrder:      sortOrder,
		OffsetCount:    int32(offset),
		LimitCount:     int32(params.PageSize),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list RAC_appointments: %w", err)
	}

	items := make([]Appointment, 0, len(rows))
	for _, row := range rows {
		items = append(items, appointmentFromModel(appointmentsdb.RacAppointment{ID: row.ID, UserID: row.UserID, LeadID: row.LeadID, LeadServiceID: row.LeadServiceID, Type: row.Type, Title: row.Title, Description: row.Description, Location: row.Location, StartTime: row.StartTime, EndTime: row.EndTime, Status: row.Status, AllDay: row.AllDay, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID, MeetingLink: row.MeetingLink}))
	}

	return &ListResult{
		Items:      items,
		Total:      int(total),
		Page:       params.Page,
		PageSize:   params.PageSize,
		TotalPages: totalPages,
	}, nil
}

type appointmentListFilters struct {
	userID    pgtype.UUID
	leadID    pgtype.UUID
	typeValue pgtype.Text
	status    pgtype.Text
	startFrom pgtype.Timestamptz
	startTo   pgtype.Timestamptz
	search    pgtype.Text
}

func optionalSearchParam(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: "%" + value + "%", Valid: true}
}

func resolveAppointmentSortBy(sortBy string) (string, error) {
	if sortBy == "" {
		return "startTime", nil
	}
	switch sortBy {
	case "title", "type", "status", "startTime", "endTime", "createdAt":
		return sortBy, nil
	default:
		return "", apperr.BadRequest("invalid sort field")
	}
}

func resolveAppointmentSortOrder(sortOrder string) (string, error) {
	if sortOrder == "" {
		return "asc", nil
	}
	switch sortOrder {
	case "asc", "desc":
		return sortOrder, nil
	default:
		return "", apperr.BadRequest("invalid sort order")
	}
}

// GetLeadInfo retrieves basic lead information for embedding in appointment responses
func (r *Repository) GetLeadInfo(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (*LeadInfo, error) {
	row, err := r.queries.GetAppointmentLeadInfo(ctx, appointmentsdb.GetAppointmentLeadInfoParams{
		ID:             toPgUUID(leadID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get lead info: %w", err)
	}

	info := leadInfoFromModel(appointmentsdb.RacLead{ID: row.ID, ConsumerFirstName: row.ConsumerFirstName, ConsumerLastName: row.ConsumerLastName, ConsumerPhone: row.ConsumerPhone, AddressStreet: row.AddressStreet, AddressHouseNumber: row.AddressHouseNumber, AddressCity: row.AddressCity})
	return &info, nil
}

// GetLeadEmail retrieves the email for a lead
func (r *Repository) GetLeadEmail(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (string, error) {
	email, err := r.queries.GetAppointmentLeadEmail(ctx, appointmentsdb.GetAppointmentLeadEmailParams{
		ID:             toPgUUID(leadID),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get lead email: %w", err)
	}

	return email, nil
}

// GetLeadInfoBatch retrieves lead info for multiple lead IDs
func (r *Repository) GetLeadInfoBatch(ctx context.Context, leadIDs []uuid.UUID, organizationID uuid.UUID) (map[uuid.UUID]*LeadInfo, error) {
	if len(leadIDs) == 0 {
		return make(map[uuid.UUID]*LeadInfo), nil
	}

	rows, err := r.queries.ListAppointmentLeadInfoByIDs(ctx, appointmentsdb.ListAppointmentLeadInfoByIDsParams{
		Column1:        toPgUUIDSlice(leadIDs),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get lead info batch: %w", err)
	}

	result := make(map[uuid.UUID]*LeadInfo)
	for _, row := range rows {
		info := leadInfoFromModel(appointmentsdb.RacLead{ID: row.ID, ConsumerFirstName: row.ConsumerFirstName, ConsumerLastName: row.ConsumerLastName, ConsumerPhone: row.ConsumerPhone, AddressStreet: row.AddressStreet, AddressHouseNumber: row.AddressHouseNumber, AddressCity: row.AddressCity})
		result[info.ID] = &info
	}

	return result, nil
}

// ListForDateRange retrieves all RAC_appointments for a user within a date range (for slots computation)
// Uses proper overlap detection: an appointment overlaps if it starts before the window ends AND ends after the window starts
func (r *Repository) ListForDateRange(ctx context.Context, organizationID uuid.UUID, userID uuid.UUID, startDate, endDate time.Time) ([]Appointment, error) {
	rows, err := r.queries.ListAppointmentsForDateRange(ctx, appointmentsdb.ListAppointmentsForDateRangeParams{
		OrganizationID: toPgUUID(organizationID),
		UserID:         toPgUUID(userID),
		EndTime:        toPgTimestamp(startDate),
		StartTime:      toPgTimestamp(endDate),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list RAC_appointments for date range: %w", err)
	}

	items := make([]Appointment, 0, len(rows))
	for _, row := range rows {
		items = append(items, appointmentFromModel(appointmentsdb.RacAppointment{ID: row.ID, UserID: row.UserID, LeadID: row.LeadID, LeadServiceID: row.LeadServiceID, Type: row.Type, Title: row.Title, Description: row.Description, Location: row.Location, StartTime: row.StartTime, EndTime: row.EndTime, Status: row.Status, AllDay: row.AllDay, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID, MeetingLink: row.MeetingLink}))
	}

	return items, nil
}

// ToResponse converts an Appointment to AppointmentResponse
func (a *Appointment) ToResponse(leadInfo *transport.AppointmentLeadInfo) transport.AppointmentResponse {
	resp := transport.AppointmentResponse{
		ID:            a.ID,
		UserID:        a.UserID,
		LeadID:        a.LeadID,
		LeadServiceID: a.LeadServiceID,
		Type:          transport.AppointmentType(a.Type),
		Title:         a.Title,
		Description:   a.Description,
		Location:      a.Location,
		MeetingLink:   a.MeetingLink,
		StartTime:     a.StartTime,
		EndTime:       a.EndTime,
		Status:        transport.AppointmentStatus(a.Status),
		AllDay:        a.AllDay,
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
		Lead:          leadInfo,
	}
	return resp
}
