package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"portal_final_backend/internal/appointments/transport"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// Repository provides database operations for appointments
type Repository struct {
	pool *pgxpool.Pool
}

const appointmentNotFoundMsg = "appointment not found"

// New creates a new appointments repository
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create inserts a new appointment
func (r *Repository) Create(ctx context.Context, appt *Appointment) error {
	query := `
		INSERT INTO appointments (
			id, organization_id, user_id, lead_id, lead_service_id, type, title, description,
			location, meeting_link, start_time, end_time, status, all_day, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
		)`

	_, err := r.pool.Exec(ctx, query,
		appt.ID, appt.OrganizationID, appt.UserID, appt.LeadID, appt.LeadServiceID, appt.Type,
		appt.Title, appt.Description, appt.Location, appt.MeetingLink, appt.StartTime,
		appt.EndTime, appt.Status, appt.AllDay, appt.CreatedAt, appt.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create appointment: %w", err)
	}

	return nil
}

// GetByID retrieves an appointment by its ID
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (*Appointment, error) {
	var appt Appointment
	query := `SELECT id, organization_id, user_id, lead_id, lead_service_id, type, title, description,
		location, meeting_link, start_time, end_time, status, all_day, created_at, updated_at
		FROM appointments WHERE id = $1 AND organization_id = $2`

	err := r.pool.QueryRow(ctx, query, id, organizationID).Scan(
		&appt.ID, &appt.OrganizationID, &appt.UserID, &appt.LeadID, &appt.LeadServiceID, &appt.Type,
		&appt.Title, &appt.Description, &appt.Location, &appt.MeetingLink, &appt.StartTime,
		&appt.EndTime, &appt.Status, &appt.AllDay, &appt.CreatedAt, &appt.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound(appointmentNotFoundMsg)
		}
		return nil, fmt.Errorf("failed to get appointment: %w", err)
	}

	return &appt, nil
}

// GetByLeadServiceID retrieves an appointment by lead service ID (for sync)
func (r *Repository) GetByLeadServiceID(ctx context.Context, leadServiceID uuid.UUID, organizationID uuid.UUID) (*Appointment, error) {
	var appt Appointment
	query := `SELECT id, organization_id, user_id, lead_id, lead_service_id, type, title, description,
		location, meeting_link, start_time, end_time, status, all_day, created_at, updated_at
		FROM appointments WHERE lead_service_id = $1 AND organization_id = $2 AND status != 'cancelled' ORDER BY created_at DESC LIMIT 1`

	err := r.pool.QueryRow(ctx, query, leadServiceID, organizationID).Scan(
		&appt.ID, &appt.OrganizationID, &appt.UserID, &appt.LeadID, &appt.LeadServiceID, &appt.Type,
		&appt.Title, &appt.Description, &appt.Location, &appt.MeetingLink, &appt.StartTime,
		&appt.EndTime, &appt.Status, &appt.AllDay, &appt.CreatedAt, &appt.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found is acceptable
		}
		return nil, fmt.Errorf("failed to get appointment by lead service: %w", err)
	}

	return &appt, nil
}

// Update updates an existing appointment
func (r *Repository) Update(ctx context.Context, appt *Appointment) error {
	query := `
		UPDATE appointments SET
			title = $2,
			description = $3,
			location = $4,
			meeting_link = $5,
			start_time = $6,
			end_time = $7,
			all_day = $8,
			updated_at = $9
		WHERE id = $1 AND organization_id = $10`

	result, err := r.pool.Exec(ctx, query,
		appt.ID, appt.Title, appt.Description, appt.Location, appt.MeetingLink,
		appt.StartTime, appt.EndTime, appt.AllDay, appt.UpdatedAt, appt.OrganizationID,
	)
	if err != nil {
		return fmt.Errorf("failed to update appointment: %w", err)
	}

	if result.RowsAffected() == 0 {
		return apperr.NotFound(appointmentNotFoundMsg)
	}

	return nil
}

// UpdateStatus updates the status of an appointment
func (r *Repository) UpdateStatus(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, status string) error {
	query := `UPDATE appointments SET status = $3, updated_at = $4 WHERE id = $1 AND organization_id = $2`

	result, err := r.pool.Exec(ctx, query, id, organizationID, status, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update appointment status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return apperr.NotFound(appointmentNotFoundMsg)
	}

	return nil
}

// Delete removes an appointment
func (r *Repository) Delete(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) error {
	query := `DELETE FROM appointments WHERE id = $1 AND organization_id = $2`

	result, err := r.pool.Exec(ctx, query, id, organizationID)
	if err != nil {
		return fmt.Errorf("failed to delete appointment: %w", err)
	}

	if result.RowsAffected() == 0 {
		return apperr.NotFound(appointmentNotFoundMsg)
	}

	return nil
}

// ListParams contains parameters for listing appointments
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

// ListResult contains the result of listing appointments
type ListResult struct {
	Items      []Appointment
	Total      int
	Page       int
	PageSize   int
	TotalPages int
}

// List retrieves appointments with optional filtering
func (r *Repository) List(ctx context.Context, params ListParams) (*ListResult, error) {
	// Build query
	baseQuery := `FROM appointments WHERE organization_id = $1`
	args := []interface{}{params.OrganizationID}
	argIndex := 2

	addFilter(&baseQuery, &args, &argIndex, params.UserID != nil, " AND user_id = $%d", derefUUID(params.UserID))
	addFilter(&baseQuery, &args, &argIndex, params.LeadID != nil, " AND lead_id = $%d", derefUUID(params.LeadID))
	addFilter(&baseQuery, &args, &argIndex, params.Type != nil, " AND type = $%d", derefString(params.Type))
	addFilter(&baseQuery, &args, &argIndex, params.Status != nil, " AND status = $%d", derefString(params.Status))
	addFilter(&baseQuery, &args, &argIndex, params.StartFrom != nil, " AND start_time >= $%d", derefTime(params.StartFrom))
	addFilter(&baseQuery, &args, &argIndex, params.StartTo != nil, " AND start_time <= $%d", derefTime(params.StartTo))
	addFilter(
		&baseQuery,
		&args,
		&argIndex,
		params.Search != "",
		" AND (title ILIKE $%d OR location ILIKE $%d OR meeting_link ILIKE $%d)",
		"%"+params.Search+"%",
	)

	// Count total
	var total int
	countQuery := "SELECT COUNT(*) " + baseQuery
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to count appointments: %w", err)
	}

	// Calculate pagination
	totalPages := (total + params.PageSize - 1) / params.PageSize
	offset := (params.Page - 1) * params.PageSize

	// Build ORDER BY clause
	orderBy := "start_time"
	if params.SortBy != "" {
		// Map frontend column names to database columns
		columnMap := map[string]string{
			"title":     "title",
			"type":      "type",
			"status":    "status",
			"startTime": "start_time",
			"endTime":   "end_time",
			"createdAt": "created_at",
		}
		col, ok := columnMap[params.SortBy]
		if !ok {
			return nil, apperr.BadRequest("invalid sort field")
		}
		orderBy = col
	}
	sortDir := "ASC"
	if params.SortOrder != "" {
		switch params.SortOrder {
		case "asc":
			sortDir = "ASC"
		case "desc":
			sortDir = "DESC"
		default:
			return nil, apperr.BadRequest("invalid sort order")
		}
	}

	// Fetch items
	selectQuery := fmt.Sprintf(`SELECT id, organization_id, user_id, lead_id, lead_service_id, type, title, description,
		location, meeting_link, start_time, end_time, status, all_day, created_at, updated_at %s ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		baseQuery, orderBy, sortDir, argIndex, argIndex+1)
	args = append(args, params.PageSize, offset)

	rows, err := r.pool.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list appointments: %w", err)
	}
	defer rows.Close()

	var items []Appointment
	for rows.Next() {
		var appt Appointment
		if err := rows.Scan(
			&appt.ID, &appt.OrganizationID, &appt.UserID, &appt.LeadID, &appt.LeadServiceID, &appt.Type,
			&appt.Title, &appt.Description, &appt.Location, &appt.MeetingLink, &appt.StartTime,
			&appt.EndTime, &appt.Status, &appt.AllDay, &appt.CreatedAt, &appt.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan appointment: %w", err)
		}
		items = append(items, appt)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate appointments: %w", err)
	}

	return &ListResult{
		Items:      items,
		Total:      total,
		Page:       params.Page,
		PageSize:   params.PageSize,
		TotalPages: totalPages,
	}, nil
}

// GetLeadInfo retrieves basic lead information for embedding in appointment responses
func (r *Repository) GetLeadInfo(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (*LeadInfo, error) {
	var info LeadInfo
	query := `SELECT id, consumer_first_name, consumer_last_name, consumer_phone, address_street, address_house_number, address_city 
		FROM RAC_leads WHERE id = $1 AND organization_id = $2`

	err := r.pool.QueryRow(ctx, query, leadID, organizationID).Scan(
		&info.ID, &info.FirstName, &info.LastName, &info.Phone,
		&info.Street, &info.HouseNumber, &info.City,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get lead info: %w", err)
	}

	return &info, nil
}

// GetLeadEmail retrieves the email for a lead
func (r *Repository) GetLeadEmail(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (string, error) {
	var email string
	query := `SELECT COALESCE(consumer_email, '') FROM RAC_leads WHERE id = $1 AND organization_id = $2`

	err := r.pool.QueryRow(ctx, query, leadID, organizationID).Scan(&email)
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

	query := `SELECT id, consumer_first_name, consumer_last_name, consumer_phone, address_street, address_house_number, address_city 
		FROM RAC_leads WHERE id = ANY($1) AND organization_id = $2`

	rows, err := r.pool.Query(ctx, query, leadIDs, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get lead info batch: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID]*LeadInfo)
	for rows.Next() {
		var info LeadInfo
		if err := rows.Scan(
			&info.ID, &info.FirstName, &info.LastName, &info.Phone,
			&info.Street, &info.HouseNumber, &info.City,
		); err != nil {
			return nil, fmt.Errorf("failed to scan lead info: %w", err)
		}
		result[info.ID] = &info
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate RAC_leads: %w", err)
	}

	return result, nil
}

// ListForDateRange retrieves all appointments for a user within a date range (for slots computation)
// Uses proper overlap detection: an appointment overlaps if it starts before the window ends AND ends after the window starts
func (r *Repository) ListForDateRange(ctx context.Context, organizationID uuid.UUID, userID uuid.UUID, startDate, endDate time.Time) ([]Appointment, error) {
	query := `SELECT id, organization_id, user_id, lead_id, lead_service_id, type, title, description,
		location, meeting_link, start_time, end_time, status, all_day, created_at, updated_at
		FROM appointments 
		WHERE organization_id = $1 AND user_id = $2 
		AND start_time < $4 AND end_time > $3
		AND status = 'scheduled'
		ORDER BY start_time ASC`

	rows, err := r.pool.Query(ctx, query, organizationID, userID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to list appointments for date range: %w", err)
	}
	defer rows.Close()

	var items []Appointment
	for rows.Next() {
		var appt Appointment
		if err := rows.Scan(
			&appt.ID, &appt.OrganizationID, &appt.UserID, &appt.LeadID, &appt.LeadServiceID, &appt.Type,
			&appt.Title, &appt.Description, &appt.Location, &appt.MeetingLink, &appt.StartTime,
			&appt.EndTime, &appt.Status, &appt.AllDay, &appt.CreatedAt, &appt.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan appointment: %w", err)
		}
		items = append(items, appt)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate appointments: %w", err)
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

func addFilter(baseQuery *string, args *[]interface{}, argIndex *int, apply bool, clause string, value interface{}) {
	if !apply {
		return
	}
	*baseQuery += fmt.Sprintf(clause, *argIndex)
	*args = append(*args, value)
	*argIndex++
}

func derefUUID(value *uuid.UUID) uuid.UUID {
	if value == nil {
		return uuid.UUID{}
	}
	return *value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func derefTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}
