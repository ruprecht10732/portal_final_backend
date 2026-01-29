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
	ServiceType           string
	Status                string
	AssignedAgentID       *uuid.UUID
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
	ServiceType        string
	AssignedAgentID    *uuid.UUID
}

func (r *Repository) Create(ctx context.Context, params CreateLeadParams) (Lead, error) {
	var lead Lead
	err := r.pool.QueryRow(ctx, `
		INSERT INTO leads (
			consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city,
			service_type, status, assigned_agent_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'New', $11)
		RETURNING id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city,
			service_type, status, assigned_agent_id, viewed_by_id, viewed_at,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
	`,
		params.ConsumerFirstName, params.ConsumerLastName, params.ConsumerPhone, params.ConsumerEmail, params.ConsumerRole,
		params.AddressStreet, params.AddressHouseNumber, params.AddressZipCode, params.AddressCity,
		params.ServiceType, params.AssignedAgentID,
	).Scan(
		&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity,
		&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ViewedByID, &lead.ViewedAt,
		&lead.VisitScheduledDate, &lead.VisitScoutID, &lead.VisitMeasurements, &lead.VisitAccessDifficulty, &lead.VisitNotes, &lead.VisitCompletedAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	return lead, err
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (Lead, error) {
	var lead Lead
	err := r.pool.QueryRow(ctx, `
		SELECT id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city,
			service_type, status, assigned_agent_id, viewed_by_id, viewed_at,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
		FROM leads WHERE id = $1 AND deleted_at IS NULL
	`, id).Scan(
		&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity,
		&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ViewedByID, &lead.ViewedAt,
		&lead.VisitScheduledDate, &lead.VisitScoutID, &lead.VisitMeasurements, &lead.VisitAccessDifficulty, &lead.VisitNotes, &lead.VisitCompletedAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	return lead, err
}

func (r *Repository) GetByPhone(ctx context.Context, phone string) (Lead, error) {
	var lead Lead
	err := r.pool.QueryRow(ctx, `
		SELECT id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city,
			service_type, status, assigned_agent_id, viewed_by_id, viewed_at,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
		FROM leads WHERE consumer_phone = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`, phone).Scan(
		&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity,
		&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ViewedByID, &lead.ViewedAt,
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
	ServiceType        *string
	Status             *string
	AssignedAgentID    *uuid.UUID
	AssignedAgentIDSet bool
}

func (r *Repository) Update(ctx context.Context, id uuid.UUID, params UpdateLeadParams) (Lead, error) {
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if params.ConsumerFirstName != nil {
		setClauses = append(setClauses, fmt.Sprintf("consumer_first_name = $%d", argIdx))
		args = append(args, *params.ConsumerFirstName)
		argIdx++
	}
	if params.ConsumerLastName != nil {
		setClauses = append(setClauses, fmt.Sprintf("consumer_last_name = $%d", argIdx))
		args = append(args, *params.ConsumerLastName)
		argIdx++
	}
	if params.ConsumerPhone != nil {
		setClauses = append(setClauses, fmt.Sprintf("consumer_phone = $%d", argIdx))
		args = append(args, *params.ConsumerPhone)
		argIdx++
	}
	if params.ConsumerEmail != nil {
		setClauses = append(setClauses, fmt.Sprintf("consumer_email = $%d", argIdx))
		args = append(args, *params.ConsumerEmail)
		argIdx++
	}
	if params.ConsumerRole != nil {
		setClauses = append(setClauses, fmt.Sprintf("consumer_role = $%d", argIdx))
		args = append(args, *params.ConsumerRole)
		argIdx++
	}
	if params.AddressStreet != nil {
		setClauses = append(setClauses, fmt.Sprintf("address_street = $%d", argIdx))
		args = append(args, *params.AddressStreet)
		argIdx++
	}
	if params.AddressHouseNumber != nil {
		setClauses = append(setClauses, fmt.Sprintf("address_house_number = $%d", argIdx))
		args = append(args, *params.AddressHouseNumber)
		argIdx++
	}
	if params.AddressZipCode != nil {
		setClauses = append(setClauses, fmt.Sprintf("address_zip_code = $%d", argIdx))
		args = append(args, *params.AddressZipCode)
		argIdx++
	}
	if params.AddressCity != nil {
		setClauses = append(setClauses, fmt.Sprintf("address_city = $%d", argIdx))
		args = append(args, *params.AddressCity)
		argIdx++
	}
	if params.ServiceType != nil {
		setClauses = append(setClauses, fmt.Sprintf("service_type = $%d", argIdx))
		args = append(args, *params.ServiceType)
		argIdx++
	}
	if params.AssignedAgentIDSet {
		setClauses = append(setClauses, fmt.Sprintf("assigned_agent_id = $%d", argIdx))
		args = append(args, params.AssignedAgentID)
		argIdx++
	}
	if params.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *params.Status)
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
			address_street, address_house_number, address_zip_code, address_city,
			service_type, status, assigned_agent_id, viewed_by_id, viewed_at,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
	`, strings.Join(setClauses, ", "), argIdx)

	var lead Lead
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity,
		&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ViewedByID, &lead.ViewedAt,
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
			address_street, address_house_number, address_zip_code, address_city,
			service_type, status, assigned_agent_id, viewed_by_id, viewed_at,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
	`, id, status).Scan(
		&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity,
		&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ViewedByID, &lead.ViewedAt,
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
			address_street, address_house_number, address_zip_code, address_city,
			service_type, status, assigned_agent_id, viewed_by_id, viewed_at,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
	`, id, scheduledDate, scoutID).Scan(
		&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity,
		&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ViewedByID, &lead.ViewedAt,
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
			address_street, address_house_number, address_zip_code, address_city,
			service_type, status, assigned_agent_id, viewed_by_id, viewed_at,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
	`, id, measurements, accessDifficulty, notesPtr).Scan(
		&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity,
		&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ViewedByID, &lead.ViewedAt,
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
			address_street, address_house_number, address_zip_code, address_city,
			service_type, status, assigned_agent_id, viewed_by_id, viewed_at,
			visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
			created_at, updated_at
	`, id, notesPtr).Scan(
		&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity,
		&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ViewedByID, &lead.ViewedAt,
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
				address_street, address_house_number, address_zip_code, address_city,
				service_type, status, assigned_agent_id, viewed_by_id, viewed_at,
				visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
				created_at, updated_at
		`, id, scheduledDate, scoutID, noShowNote).Scan(
			&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
			&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity,
			&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ViewedByID, &lead.ViewedAt,
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
				address_street, address_house_number, address_zip_code, address_city,
				service_type, status, assigned_agent_id, viewed_by_id, viewed_at,
				visit_scheduled_date, visit_scout_id, visit_measurements, visit_access_difficulty, visit_notes, visit_completed_at,
				created_at, updated_at
		`, id, scheduledDate, scoutID).Scan(
			&lead.ID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
			&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity,
			&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ViewedByID, &lead.ViewedAt,
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
	Status      *string
	ServiceType *string
	Search      string
	Offset      int
	Limit       int
	SortBy      string
	SortOrder   string
}

func (r *Repository) List(ctx context.Context, params ListParams) ([]Lead, int, error) {
	whereClauses := []string{"deleted_at IS NULL"}
	args := []interface{}{}
	argIdx := 1

	if params.Status != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *params.Status)
		argIdx++
	}
	if params.ServiceType != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("service_type = $%d", argIdx))
		args = append(args, *params.ServiceType)
		argIdx++
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

	whereClause := strings.Join(whereClauses, " AND ")

	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM leads WHERE %s", whereClause)
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortColumn := "created_at"
	switch params.SortBy {
	case "scheduledDate":
		sortColumn = "visit_scheduled_date"
	case "status":
		sortColumn = "status"
	case "firstName":
		sortColumn = "consumer_first_name"
	case "lastName":
		sortColumn = "consumer_last_name"
	}
	sortOrder := "DESC"
	if params.SortOrder == "asc" {
		sortOrder = "ASC"
	}

	args = append(args, params.Limit, params.Offset)

	query := fmt.Sprintf(`
		SELECT id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city,
			service_type, status, assigned_agent_id, viewed_by_id, viewed_at,
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
			&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity,
			&lead.ServiceType, &lead.Status, &lead.AssignedAgentID, &lead.ViewedByID, &lead.ViewedAt,
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
