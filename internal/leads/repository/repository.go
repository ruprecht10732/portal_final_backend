package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("lead not found")

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

type Lead struct {
	ID                    uuid.UUID
	ConsumerFirstName     string
	ConsumerLastName      string
	ConsumerPhone         string
	ConsumerEmail         *string
	ConsumerRole          string
	AddressStreet         string
	AddressHouseNumber    string
	AddressZipCode        string
	AddressCity           string
	Latitude              *float64
	Longitude             *float64
	ServiceType           string
	Status                string
	AssignedAgentID       *uuid.UUID
	ConsumerNote          *string
	Source                *string
	ViewedByID            *uuid.UUID
	ViewedAt              *time.Time
	VisitScheduledDate    *time.Time
	VisitScoutID          *uuid.UUID
	VisitMeasurements     *string
	VisitAccessDifficulty *string
	VisitNotes            *string
	VisitCompletedAt      *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type CreateLeadParams struct {
	ConsumerFirstName  string
	ConsumerLastName   string
	ConsumerPhone      string
	ConsumerEmail      *string
	ConsumerRole       string
	AddressStreet      string
	AddressHouseNumber string
	AddressZipCode     string
	AddressCity        string
	Latitude           *float64
	Longitude          *float64
	ServiceType        string
	AssignedAgentID    *uuid.UUID
	ConsumerNote       *string
	Source             *string
}

func (r *Repository) Create(ctx context.Context, params CreateLeadParams) (Lead, error) {
	var lead Lead
	err := r.pool.QueryRow(ctx, `
		INSERT INTO leads (
			consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			service_type, status, assigned_agent_id,
			consumer_note, source
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 'New', $13, $14, $15)
		RETURNING id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			service_type, status, assigned_agent_id, consumer_note, source, viewed_by_id, viewed_at,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
	`,
		params.ConsumerFirstName, params.ConsumerLastName, params.ConsumerPhone, params.ConsumerEmail, params.ConsumerRole,
		params.AddressStreet, params.AddressHouseNumber, params.AddressZipCode, params.AddressCity, params.Latitude, params.Longitude,
		params.ServiceType, params.AssignedAgentID, params.ConsumerNote, params.Source,
	).Scan(
		&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
		&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ConsumerNote, &lead.Source, &lead.ViewedByID, &lead.ViewedAt,
		&lead.VisitScheduledDate, &lead.VisitScoutID, &lead.VisitMeasurements, &lead.VisitAccessDifficulty, &lead.VisitNotes, &lead.VisitCompletedAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if err != nil {
		return Lead{}, err
	}

	// Also create a corresponding lead_service entry
	_, err = r.CreateLeadService(ctx, CreateLeadServiceParams{
		LeadID:      lead.ID,
		ServiceType: params.ServiceType,
	})
	if err != nil {
		return Lead{}, err
	}

	return lead, nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (Lead, error) {
	var lead Lead
	err := r.pool.QueryRow(ctx, `
		SELECT id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			service_type, status, assigned_agent_id, consumer_note, source, viewed_by_id, viewed_at,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
		FROM leads WHERE id = $1 AND deleted_at IS NULL
	`, id).Scan(
		&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
		&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ConsumerNote, &lead.Source, &lead.ViewedByID, &lead.ViewedAt,
		&lead.VisitScheduledDate, &lead.VisitScoutID, &lead.VisitMeasurements, &lead.VisitAccessDifficulty, &lead.VisitNotes, &lead.VisitCompletedAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	return lead, err
}

// GetByIDWithServices returns a lead with all its services populated
func (r *Repository) GetByIDWithServices(ctx context.Context, id uuid.UUID) (Lead, []LeadService, error) {
	lead, err := r.GetByID(ctx, id)
	if err != nil {
		return Lead{}, nil, err
	}

	services, err := r.ListLeadServices(ctx, id)
	if err != nil {
		return Lead{}, nil, err
	}

	return lead, services, nil
}

func (r *Repository) GetByPhone(ctx context.Context, phone string) (Lead, error) {
	var lead Lead
	err := r.pool.QueryRow(ctx, `
		SELECT id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			service_type, status, assigned_agent_id, consumer_note, source, viewed_by_id, viewed_at,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
		FROM leads WHERE consumer_phone = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`, phone).Scan(
		&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
		&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ConsumerNote, &lead.Source, &lead.ViewedByID, &lead.ViewedAt,
		&lead.VisitScheduledDate, &lead.VisitScoutID, &lead.VisitMeasurements, &lead.VisitAccessDifficulty, &lead.VisitNotes, &lead.VisitCompletedAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	return lead, err
}

type UpdateLeadParams struct {
	ConsumerFirstName  *string
	ConsumerLastName   *string
	ConsumerPhone      *string
	ConsumerEmail      *string
	ConsumerRole       *string
	AddressStreet      *string
	AddressHouseNumber *string
	AddressZipCode     *string
	AddressCity        *string
	Latitude           *float64
	Longitude          *float64
	ServiceType        *string
	Status             *string
	AssignedAgentID    *uuid.UUID
	AssignedAgentIDSet bool
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func derefFloat(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func (r *Repository) Update(ctx context.Context, id uuid.UUID, params UpdateLeadParams) (Lead, error) {
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	fields := []struct {
		enabled bool
		column  string
		value   interface{}
	}{
		{params.ConsumerFirstName != nil, "consumer_first_name", derefString(params.ConsumerFirstName)},
		{params.ConsumerLastName != nil, "consumer_last_name", derefString(params.ConsumerLastName)},
		{params.ConsumerPhone != nil, "consumer_phone", derefString(params.ConsumerPhone)},
		{params.ConsumerEmail != nil, "consumer_email", derefString(params.ConsumerEmail)},
		{params.ConsumerRole != nil, "consumer_role", derefString(params.ConsumerRole)},
		{params.AddressStreet != nil, "address_street", derefString(params.AddressStreet)},
		{params.AddressHouseNumber != nil, "address_house_number", derefString(params.AddressHouseNumber)},
		{params.AddressZipCode != nil, "address_zip_code", derefString(params.AddressZipCode)},
		{params.AddressCity != nil, "address_city", derefString(params.AddressCity)},
		{params.Latitude != nil, "latitude", derefFloat(params.Latitude)},
		{params.Longitude != nil, "longitude", derefFloat(params.Longitude)},
		{params.ServiceType != nil, "service_type", derefString(params.ServiceType)},
		{params.AssignedAgentIDSet, "assigned_agent_id", params.AssignedAgentID},
		{params.Status != nil, "status", derefString(params.Status)},
	}

	for _, field := range fields {
		if !field.enabled {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", field.column, argIdx))
		args = append(args, field.value)
		argIdx++
	}

	if len(setClauses) == 0 {
		return r.GetByID(ctx, id)
	}

	setClauses = append(setClauses, "updated_at = now()")
	args = append(args, id)

	query := fmt.Sprintf(`
		UPDATE leads SET %s
		WHERE id = $%d AND deleted_at IS NULL
		RETURNING id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			service_type, status, assigned_agent_id, consumer_note, source, viewed_by_id, viewed_at,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
	`, strings.Join(setClauses, ", "), argIdx)

	var lead Lead
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
		&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ConsumerNote, &lead.Source, &lead.ViewedByID, &lead.ViewedAt,
		&lead.VisitScheduledDate, &lead.VisitScoutID, &lead.VisitMeasurements, &lead.VisitAccessDifficulty, &lead.VisitNotes, &lead.VisitCompletedAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	return lead, err
}

func (r *Repository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) (Lead, error) {
	var lead Lead
	err := r.pool.QueryRow(ctx, `
		UPDATE leads SET status = $2, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			service_type, status, assigned_agent_id, consumer_note, source, viewed_by_id, viewed_at,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
	`, id, status).Scan(
		&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
		&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ConsumerNote, &lead.Source, &lead.ViewedByID, &lead.ViewedAt,
		&lead.VisitScheduledDate, &lead.VisitScoutID, &lead.VisitMeasurements, &lead.VisitAccessDifficulty, &lead.VisitNotes, &lead.VisitCompletedAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	return lead, err
}

func (r *Repository) SetViewedBy(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE leads SET viewed_by_id = $2, viewed_at = now(), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
	`, id, userID)
	return err
}

func (r *Repository) AddActivity(ctx context.Context, leadID uuid.UUID, userID uuid.UUID, action string, meta map[string]interface{}) error {
	var metaJSON []byte
	if meta != nil {
		encoded, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		metaJSON = encoded
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO lead_activity (lead_id, user_id, action, meta)
		VALUES ($1, $2, $3, $4)
	`, leadID, userID, action, metaJSON)
	return err
}

func (r *Repository) ScheduleVisit(ctx context.Context, id uuid.UUID, scheduledDate time.Time, scoutID *uuid.UUID) (Lead, error) {
	var lead Lead
	err := r.pool.QueryRow(ctx, `
		UPDATE leads SET 
			visit_scheduled_date = $2, 
			visit_scout_id = $3,
			status = 'Scheduled',
			updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			service_type, status, assigned_agent_id, consumer_note, source, viewed_by_id, viewed_at,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
	`, id, scheduledDate, scoutID).Scan(
		&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
		&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ConsumerNote, &lead.Source, &lead.ViewedByID, &lead.ViewedAt,
		&lead.VisitScheduledDate, &lead.VisitScoutID, &lead.VisitMeasurements, &lead.VisitAccessDifficulty, &lead.VisitNotes, &lead.VisitCompletedAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	return lead, err
}

func (r *Repository) CompleteSurvey(ctx context.Context, id uuid.UUID, measurements string, accessDifficulty string, notes string) (Lead, error) {
	var lead Lead
	var notesPtr *string
	if notes != "" {
		notesPtr = &notes
	}
	err := r.pool.QueryRow(ctx, `
		UPDATE leads SET 
			visit_measurements = $2,
			visit_access_difficulty = $3,
			visit_notes = $4,
			visit_completed_at = now(),
			status = 'Surveyed',
			updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			service_type, status, assigned_agent_id, consumer_note, source, viewed_by_id, viewed_at,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
	`, id, measurements, accessDifficulty, notesPtr).Scan(
		&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
		&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ConsumerNote, &lead.Source, &lead.ViewedByID, &lead.ViewedAt,
		&lead.VisitScheduledDate, &lead.VisitScoutID, &lead.VisitMeasurements, &lead.VisitAccessDifficulty, &lead.VisitNotes, &lead.VisitCompletedAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	return lead, err
}

func (r *Repository) MarkNoShow(ctx context.Context, id uuid.UUID, notes string) (Lead, error) {
	var lead Lead
	var notesPtr *string
	if notes != "" {
		notesPtr = &notes
	}
	err := r.pool.QueryRow(ctx, `
		UPDATE leads SET 
			visit_notes = COALESCE(visit_notes || E'\n', '') || COALESCE($2, 'No show'),
			status = 'Needs_Rescheduling',
			updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			service_type, status, assigned_agent_id, consumer_note, source, viewed_by_id, viewed_at,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
	`, id, notesPtr).Scan(
		&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
		&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ConsumerNote, &lead.Source, &lead.ViewedByID, &lead.ViewedAt,
		&lead.VisitScheduledDate, &lead.VisitScoutID, &lead.VisitMeasurements, &lead.VisitAccessDifficulty, &lead.VisitNotes, &lead.VisitCompletedAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	return lead, err
}

func (r *Repository) RescheduleVisit(ctx context.Context, id uuid.UUID, scheduledDate time.Time, scoutID *uuid.UUID, noShowNotes string, markAsNoShow bool) (Lead, error) {
	var lead Lead
	var err error

	if markAsNoShow {
		// Build no-show note
		noShowNote := "No show"
		if noShowNotes != "" {
			noShowNote = "No show: " + noShowNotes
		}

		err = r.pool.QueryRow(ctx, `
			UPDATE leads SET 
				visit_notes = COALESCE(visit_notes || E'\n', '') || $4,
				visit_scheduled_date = $2,
				visit_scout_id = $3,
				visit_measurements = NULL,
				visit_access_difficulty = NULL,
				visit_completed_at = NULL,
				status = 'Scheduled',
				updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL
			RETURNING id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
				address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
				service_type, status, assigned_agent_id, consumer_note, source, viewed_by_id, viewed_at,
				visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
				created_at, updated_at
		`, id, scheduledDate, scoutID, noShowNote).Scan(
			&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
			&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
			&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ConsumerNote, &lead.Source, &lead.ViewedByID, &lead.ViewedAt,
			&lead.VisitScheduledDate, &lead.VisitScoutID, &lead.VisitMeasurements, &lead.VisitAccessDifficulty, &lead.VisitNotes, &lead.VisitCompletedAt,
			&lead.CreatedAt, &lead.UpdatedAt,
		)
	} else {
		// Simple reschedule without no-show
		err = r.pool.QueryRow(ctx, `
			UPDATE leads SET 
				visit_scheduled_date = $2,
				visit_scout_id = $3,
				visit_measurements = NULL,
				visit_access_difficulty = NULL,
				visit_completed_at = NULL,
				status = 'Scheduled',
				updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL
			RETURNING id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
				address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
				service_type, status, assigned_agent_id, consumer_note, source, viewed_by_id, viewed_at,
				visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
				created_at, updated_at
		`, id, scheduledDate, scoutID).Scan(
			&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
			&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
			&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ConsumerNote, &lead.Source, &lead.ViewedByID, &lead.ViewedAt,
			&lead.VisitScheduledDate, &lead.VisitScoutID, &lead.VisitMeasurements, &lead.VisitAccessDifficulty, &lead.VisitNotes, &lead.VisitCompletedAt,
			&lead.CreatedAt, &lead.UpdatedAt,
		)
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	return lead, err
}

type ListParams struct {
	Status          *string
	ServiceType     *string
	Search          string
	FirstName       *string
	LastName        *string
	Phone           *string
	Email           *string
	Role            *string
	Street          *string
	HouseNumber     *string
	ZipCode         *string
	City            *string
	AssignedAgentID *uuid.UUID
	CreatedAtFrom   *time.Time
	CreatedAtTo     *time.Time
	Offset          int
	Limit           int
	SortBy          string
	SortOrder       string
}

func (r *Repository) List(ctx context.Context, params ListParams) ([]Lead, int, error) {
	whereClause, args, argIdx := buildLeadListWhere(params)

	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM leads WHERE %s", whereClause)
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortColumn := mapLeadSortColumn(params.SortBy)
	sortOrder := "DESC"
	if params.SortOrder == "asc" {
		sortOrder = "ASC"
	}

	args = append(args, params.Limit, params.Offset)

	query := fmt.Sprintf(`
		SELECT id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			service_type, status, assigned_agent_id, consumer_note, source, viewed_by_id, viewed_at,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
		FROM leads
		WHERE %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, whereClause, sortColumn, sortOrder, argIdx, argIdx+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	leads := make([]Lead, 0)
	for rows.Next() {
		var lead Lead
		if err := rows.Scan(
			&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
			&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
			&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ConsumerNote, &lead.Source, &lead.ViewedByID, &lead.ViewedAt,
			&lead.VisitScheduledDate, &lead.VisitScoutID, &lead.VisitMeasurements, &lead.VisitAccessDifficulty, &lead.VisitNotes, &lead.VisitCompletedAt,
			&lead.CreatedAt, &lead.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		leads = append(leads, lead)
	}

	if rows.Err() != nil {
		return nil, 0, rows.Err()
	}

	return leads, total, nil
}

func buildLeadListWhere(params ListParams) (string, []interface{}, int) {
	whereClauses := []string{"deleted_at IS NULL"}
	args := []interface{}{}
	argIdx := 1

	addEquals := func(column string, value interface{}) {
		whereClauses = append(whereClauses, fmt.Sprintf("%s = $%d", column, argIdx))
		args = append(args, value)
		argIdx++
	}
	addILike := func(column string, value string) {
		whereClauses = append(whereClauses, fmt.Sprintf("%s ILIKE $%d", column, argIdx))
		args = append(args, "%"+value+"%")
		argIdx++
	}

	if params.Status != nil {
		addEquals("status", *params.Status)
	}
	if params.ServiceType != nil {
		addEquals("service_type", *params.ServiceType)
	}
	if params.Search != "" {
		searchPattern := "%" + params.Search + "%"
		whereClauses = append(whereClauses, fmt.Sprintf(
			"(consumer_first_name ILIKE $%d OR consumer_last_name ILIKE $%d OR consumer_phone ILIKE $%d OR consumer_email ILIKE $%d OR address_city ILIKE $%d)",
			argIdx, argIdx, argIdx, argIdx, argIdx,
		))
		args = append(args, searchPattern)
		argIdx++
	}
	if params.FirstName != nil {
		addILike("consumer_first_name", *params.FirstName)
	}
	if params.LastName != nil {
		addILike("consumer_last_name", *params.LastName)
	}
	if params.Phone != nil {
		addILike("consumer_phone", *params.Phone)
	}
	if params.Email != nil {
		addILike("consumer_email", *params.Email)
	}
	if params.Role != nil {
		addEquals("consumer_role", *params.Role)
	}
	if params.Street != nil {
		addILike("address_street", *params.Street)
	}
	if params.HouseNumber != nil {
		addILike("address_house_number", *params.HouseNumber)
	}
	if params.ZipCode != nil {
		addILike("address_zip_code", *params.ZipCode)
	}
	if params.City != nil {
		addILike("address_city", *params.City)
	}
	if params.AssignedAgentID != nil {
		addEquals("assigned_agent_id", *params.AssignedAgentID)
	}
	if params.CreatedAtFrom != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, *params.CreatedAtFrom)
		argIdx++
	}
	if params.CreatedAtTo != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at < $%d", argIdx))
		args = append(args, *params.CreatedAtTo)
		argIdx++
	}

	return strings.Join(whereClauses, " AND "), args, argIdx
}

func mapLeadSortColumn(sortBy string) string {
	sortColumn := "created_at"
	switch sortBy {
	case "scheduledDate":
		return "visit_scheduled_date"
	case "status":
		return "status"
	case "firstName":
		return "consumer_first_name"
	case "lastName":
		return "consumer_last_name"
	case "phone":
		return "consumer_phone"
	case "email":
		return "consumer_email"
	case "role":
		return "consumer_role"
	case "street":
		return "address_street"
	case "houseNumber":
		return "address_house_number"
	case "zipCode":
		return "address_zip_code"
	case "city":
		return "address_city"
	case "serviceType":
		return "service_type"
	case "assignedAgentId":
		return "assigned_agent_id"
	default:
		return sortColumn
	}
}

type HeatmapPoint struct {
	Latitude  float64
	Longitude float64
}

func (r *Repository) ListHeatmapPoints(ctx context.Context, startDate *time.Time, endDate *time.Time) ([]HeatmapPoint, error) {
	whereClauses := []string{"deleted_at IS NULL", "latitude IS NOT NULL", "longitude IS NOT NULL"}
	args := []interface{}{}
	argIdx := 1

	if startDate != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, *startDate)
		argIdx++
	}
	if endDate != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at < $%d", argIdx))
		args = append(args, *endDate)
		argIdx++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	query := fmt.Sprintf(`
		SELECT latitude, longitude
		FROM leads
		WHERE %s
	`, whereClause)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := make([]HeatmapPoint, 0)
	for rows.Next() {
		var point HeatmapPoint
		if err := rows.Scan(&point.Latitude, &point.Longitude); err != nil {
			return nil, err
		}
		points = append(points, point)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return points, nil
}

type ActionItem struct {
	ID            uuid.UUID
	FirstName     string
	LastName      string
	UrgencyLevel  *string
	UrgencyReason *string
	CreatedAt     time.Time
}

type ActionItemListResult struct {
	Items []ActionItem
	Total int
}

func (r *Repository) ListActionItems(ctx context.Context, newLeadDays int, limit int, offset int) (ActionItemListResult, error) {
	whereClauses := []string{"l.deleted_at IS NULL"}
	args := []interface{}{newLeadDays}
	argIdx := 2

	whereClauses = append(whereClauses, "(ai.urgency_level = 'High' OR l.created_at >= now() - ($1::int || ' days')::interval)")

	whereClause := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM leads l
		LEFT JOIN (
			SELECT DISTINCT ON (lead_id) lead_id, urgency_level, urgency_reason, created_at
			FROM lead_ai_analysis
			ORDER BY lead_id, created_at DESC
		) ai ON ai.lead_id = l.id
		WHERE %s
	`, whereClause)

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return ActionItemListResult{}, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf(`
		SELECT l.id, l.consumer_first_name, l.consumer_last_name, ai.urgency_level, ai.urgency_reason, l.created_at
		FROM leads l
		LEFT JOIN (
			SELECT DISTINCT ON (lead_id) lead_id, urgency_level, urgency_reason, created_at
			FROM lead_ai_analysis
			ORDER BY lead_id, created_at DESC
		) ai ON ai.lead_id = l.id
		WHERE %s
		ORDER BY
			CASE WHEN ai.urgency_level = 'High' THEN 0 ELSE 1 END,
			l.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return ActionItemListResult{}, err
	}
	defer rows.Close()

	items := make([]ActionItem, 0)
	for rows.Next() {
		var item ActionItem
		if err := rows.Scan(&item.ID, &item.FirstName, &item.LastName, &item.UrgencyLevel, &item.UrgencyReason, &item.CreatedAt); err != nil {
			return ActionItemListResult{}, err
		}
		items = append(items, item)
	}

	if rows.Err() != nil {
		return ActionItemListResult{}, rows.Err()
	}

	return ActionItemListResult{Items: items, Total: total}, nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.pool.Exec(ctx, "UPDATE leads SET deleted_at = now(), updated_at = now() WHERE id = $1 AND deleted_at IS NULL", id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) BulkDelete(ctx context.Context, ids []uuid.UUID) (int, error) {
	result, err := r.pool.Exec(ctx, "UPDATE leads SET deleted_at = now(), updated_at = now() WHERE id = ANY($1) AND deleted_at IS NULL", ids)
	if err != nil {
		return 0, err
	}
	return int(result.RowsAffected()), nil
}
