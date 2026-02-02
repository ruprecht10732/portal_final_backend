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

// ListActiveServiceTypes returns active service types with intake guidelines for AI context.
func (r *Repository) ListActiveServiceTypes(ctx context.Context, organizationID uuid.UUID) ([]ServiceContextDefinition, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT name, description, intake_guidelines
		FROM service_types
		WHERE organization_id = $1 AND is_active = true
		ORDER BY display_order ASC, name ASC
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ServiceContextDefinition, 0)
	for rows.Next() {
		var item ServiceContextDefinition
		if err := rows.Scan(&item.Name, &item.Description, &item.IntakeGuidelines); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return items, nil
}

type Lead struct {
	ID                 uuid.UUID
	OrganizationID     uuid.UUID
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
	AssignedAgentID    *uuid.UUID
	Source             *string
	ViewedByID         *uuid.UUID
	ViewedAt           *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// LeadSummary is a lightweight lead representation for returning customer detection
type LeadSummary struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	ConsumerName    string
	ConsumerPhone   string
	ConsumerEmail   *string
	AddressCity     string
	ServiceCount    int
	LastServiceType *string
	LastStatus      *string
	CreatedAt       time.Time
}

type CreateLeadParams struct {
	OrganizationID     uuid.UUID
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
	AssignedAgentID    *uuid.UUID
	Source             *string
}

func (r *Repository) Create(ctx context.Context, params CreateLeadParams) (Lead, error) {
	var lead Lead
	err := r.pool.QueryRow(ctx, `
		INSERT INTO leads (
			organization_id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			assigned_agent_id, source
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, organization_id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			assigned_agent_id, source, viewed_by_id, viewed_at, created_at, updated_at
	`,
		params.OrganizationID, params.ConsumerFirstName, params.ConsumerLastName, params.ConsumerPhone, params.ConsumerEmail, params.ConsumerRole,
		params.AddressStreet, params.AddressHouseNumber, params.AddressZipCode, params.AddressCity, params.Latitude, params.Longitude,
		params.AssignedAgentID, params.Source,
	).Scan(
		&lead.ID, &lead.OrganizationID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
		&lead.AssignedAgentID, &lead.Source, &lead.ViewedByID, &lead.ViewedAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if err != nil {
		return Lead{}, err
	}

	return lead, nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (Lead, error) {
	var lead Lead
	err := r.pool.QueryRow(ctx, `
		SELECT id, organization_id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			assigned_agent_id, source, viewed_by_id, viewed_at, created_at, updated_at
		FROM leads WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, organizationID).Scan(
		&lead.ID, &lead.OrganizationID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
		&lead.AssignedAgentID, &lead.Source, &lead.ViewedByID, &lead.ViewedAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	return lead, err
}

// GetByIDWithServices returns a lead with all its services populated
func (r *Repository) GetByIDWithServices(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (Lead, []LeadService, error) {
	lead, err := r.GetByID(ctx, id, organizationID)
	if err != nil {
		return Lead{}, nil, err
	}

	services, err := r.ListLeadServices(ctx, id, organizationID)
	if err != nil {
		return Lead{}, nil, err
	}

	return lead, services, nil
}

func (r *Repository) GetByPhone(ctx context.Context, phone string, organizationID uuid.UUID) (Lead, error) {
	var lead Lead
	err := r.pool.QueryRow(ctx, `
		SELECT id, organization_id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			assigned_agent_id, source, viewed_by_id, viewed_at, created_at, updated_at
		FROM leads WHERE consumer_phone = $1 AND organization_id = $2 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`, phone, organizationID).Scan(
		&lead.ID, &lead.OrganizationID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
		&lead.AssignedAgentID, &lead.Source, &lead.ViewedByID, &lead.ViewedAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	return lead, err
}

// GetByPhoneOrEmail finds a lead matching the given phone or email for returning customer detection.
// Returns the first matching lead with its services, or nil if not found.
func (r *Repository) GetByPhoneOrEmail(ctx context.Context, phone string, email string, organizationID uuid.UUID) (*LeadSummary, []LeadService, error) {
	if phone == "" && email == "" {
		return nil, nil, nil
	}

	var summary LeadSummary
	err := r.pool.QueryRow(ctx, `
		SELECT 
			l.id,
			l.organization_id,
			l.consumer_first_name || ' ' || l.consumer_last_name AS consumer_name,
			l.consumer_phone,
			l.consumer_email,
			l.address_city,
			COUNT(ls.id) AS service_count,
			(SELECT st.name FROM lead_services ls2 
			 JOIN service_types st ON st.id = ls2.service_type_id AND st.organization_id = l.organization_id
			 WHERE ls2.lead_id = l.id ORDER BY ls2.created_at DESC LIMIT 1) AS last_service_type,
			(SELECT ls2.status FROM lead_services ls2 
			 WHERE ls2.lead_id = l.id ORDER BY ls2.created_at DESC LIMIT 1) AS last_status,
			l.created_at
		FROM leads l
		LEFT JOIN lead_services ls ON ls.lead_id = l.id
		WHERE l.deleted_at IS NULL 
		  AND l.organization_id = $3
		  AND (($1 != '' AND l.consumer_phone = $1) OR ($2 != '' AND l.consumer_email = $2))
		GROUP BY l.id
		ORDER BY l.created_at DESC
		LIMIT 1
	`, phone, email, organizationID).Scan(
		&summary.ID, &summary.OrganizationID, &summary.ConsumerName, &summary.ConsumerPhone, &summary.ConsumerEmail,
		&summary.AddressCity, &summary.ServiceCount, &summary.LastServiceType, &summary.LastStatus,
		&summary.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	// Fetch services for the found lead
	services, err := r.ListLeadServices(ctx, summary.ID, organizationID)
	if err != nil {
		return nil, nil, err
	}

	return &summary, services, nil
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

func (r *Repository) Update(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, params UpdateLeadParams) (Lead, error) {
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
		{params.AssignedAgentIDSet, "assigned_agent_id", params.AssignedAgentID},
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
		return r.GetByID(ctx, id, organizationID)
	}

	setClauses = append(setClauses, "updated_at = now()")
	args = append(args, id, organizationID)

	query := fmt.Sprintf(`
		UPDATE leads SET %s
		WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL
		RETURNING id, organization_id, consumer_first_name, consumer_last_name, consumer_phone, consumer_email, consumer_role,
			address_street, address_house_number, address_zip_code, address_city, latitude, longitude,
			assigned_agent_id, source, viewed_by_id, viewed_at, created_at, updated_at
	`, strings.Join(setClauses, ", "), argIdx, argIdx+1)

	var lead Lead
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&lead.ID, &lead.OrganizationID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
		&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
		&lead.AssignedAgentID, &lead.Source, &lead.ViewedByID, &lead.ViewedAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	return lead, err
}

func (r *Repository) SetViewedBy(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE leads SET viewed_by_id = $3, viewed_at = now(), updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`, id, organizationID, userID)
	return err
}

func (r *Repository) AddActivity(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID, userID uuid.UUID, action string, meta map[string]interface{}) error {
	var metaJSON []byte
	if meta != nil {
		encoded, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		metaJSON = encoded
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO lead_activity (lead_id, organization_id, user_id, action, meta)
		VALUES ($1, $2, $3, $4, $5)
	`, leadID, organizationID, userID, action, metaJSON)
	return err
}

type ListParams struct {
	OrganizationID  uuid.UUID
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
	whereClause, joinClause, args, argIdx := buildLeadListWhere(params)

	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(DISTINCT l.id) FROM leads l %s WHERE %s", joinClause, whereClause)
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
		SELECT DISTINCT l.id, l.organization_id, l.consumer_first_name, l.consumer_last_name, l.consumer_phone, l.consumer_email, l.consumer_role,
			l.address_street, l.address_house_number, l.address_zip_code, l.address_city, l.latitude, l.longitude,
			l.assigned_agent_id, l.source, l.viewed_by_id, l.viewed_at, l.created_at, l.updated_at
		FROM leads l
		%s
		WHERE %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, joinClause, whereClause, sortColumn, sortOrder, argIdx, argIdx+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	leads := make([]Lead, 0)
	for rows.Next() {
		var lead Lead
		if err := rows.Scan(
			&lead.ID, &lead.OrganizationID, &lead.ConsumerFirstName, &lead.ConsumerLastName, &lead.ConsumerPhone, &lead.ConsumerEmail, &lead.ConsumerRole,
			&lead.AddressStreet, &lead.AddressHouseNumber, &lead.AddressZipCode, &lead.AddressCity, &lead.Latitude, &lead.Longitude,
			&lead.AssignedAgentID, &lead.Source, &lead.ViewedByID, &lead.ViewedAt,
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

func buildLeadListWhere(params ListParams) (string, string, []interface{}, int) {
	// Organization ID is always the first filter (mandatory for tenant isolation)
	whereClauses := []string{"l.organization_id = $1", "l.deleted_at IS NULL"}
	args := []interface{}{params.OrganizationID}
	argIdx := 2
	needsServiceJoin := false

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

	// Status and ServiceType filtering now requires joining with lead_services (current service)
	if params.Status != nil {
		needsServiceJoin = true
		whereClauses = append(whereClauses, fmt.Sprintf("cs.status = $%d", argIdx))
		args = append(args, *params.Status)
		argIdx++
	}
	if params.ServiceType != nil {
		needsServiceJoin = true
		whereClauses = append(whereClauses, fmt.Sprintf("st.name = $%d", argIdx))
		args = append(args, *params.ServiceType)
		argIdx++
	}
	if params.Search != "" {
		searchPattern := "%" + params.Search + "%"
		whereClauses = append(whereClauses, fmt.Sprintf(
			"(l.consumer_first_name ILIKE $%d OR l.consumer_last_name ILIKE $%d OR l.consumer_phone ILIKE $%d OR l.consumer_email ILIKE $%d OR l.address_city ILIKE $%d)",
			argIdx, argIdx, argIdx, argIdx, argIdx,
		))
		args = append(args, searchPattern)
		argIdx++
	}
	if params.FirstName != nil {
		addILike("l.consumer_first_name", *params.FirstName)
	}
	if params.LastName != nil {
		addILike("l.consumer_last_name", *params.LastName)
	}
	if params.Phone != nil {
		addILike("l.consumer_phone", *params.Phone)
	}
	if params.Email != nil {
		addILike("l.consumer_email", *params.Email)
	}
	if params.Role != nil {
		addEquals("l.consumer_role", *params.Role)
	}
	if params.Street != nil {
		addILike("l.address_street", *params.Street)
	}
	if params.HouseNumber != nil {
		addILike("l.address_house_number", *params.HouseNumber)
	}
	if params.ZipCode != nil {
		addILike("l.address_zip_code", *params.ZipCode)
	}
	if params.City != nil {
		addILike("l.address_city", *params.City)
	}
	if params.AssignedAgentID != nil {
		addEquals("l.assigned_agent_id", *params.AssignedAgentID)
	}
	if params.CreatedAtFrom != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("l.created_at >= $%d", argIdx))
		args = append(args, *params.CreatedAtFrom)
		argIdx++
	}
	if params.CreatedAtTo != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("l.created_at < $%d", argIdx))
		args = append(args, *params.CreatedAtTo)
		argIdx++
	}

	// Build join clause for current service if needed
	joinClause := ""
	if needsServiceJoin {
		joinClause = `
		LEFT JOIN LATERAL (
			SELECT ls.id, ls.status, ls.service_type_id
			FROM lead_services ls
			WHERE ls.lead_id = l.id AND ls.status NOT IN ('Closed', 'Bad_Lead', 'Surveyed')
			ORDER BY ls.created_at DESC
			LIMIT 1
		) cs ON true
		LEFT JOIN service_types st ON st.id = cs.service_type_id AND st.organization_id = l.organization_id`
	}

	return strings.Join(whereClauses, " AND "), joinClause, args, argIdx
}

func mapLeadSortColumn(sortBy string) string {
	sortColumn := "l.created_at"
	switch sortBy {
	case "firstName":
		return "l.consumer_first_name"
	case "lastName":
		return "l.consumer_last_name"
	case "phone":
		return "l.consumer_phone"
	case "email":
		return "l.consumer_email"
	case "role":
		return "l.consumer_role"
	case "street":
		return "l.address_street"
	case "houseNumber":
		return "l.address_house_number"
	case "zipCode":
		return "l.address_zip_code"
	case "city":
		return "l.address_city"
	case "assignedAgentId":
		return "l.assigned_agent_id"
	default:
		return sortColumn
	}
}

type HeatmapPoint struct {
	Latitude  float64
	Longitude float64
}

func (r *Repository) ListHeatmapPoints(ctx context.Context, organizationID uuid.UUID, startDate *time.Time, endDate *time.Time) ([]HeatmapPoint, error) {
	whereClauses := []string{"organization_id = $1", "deleted_at IS NULL", "latitude IS NOT NULL", "longitude IS NOT NULL"}
	args := []interface{}{organizationID}
	argIdx := 2

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

func (r *Repository) ListActionItems(ctx context.Context, organizationID uuid.UUID, newLeadDays int, limit int, offset int) (ActionItemListResult, error) {
	whereClauses := []string{"l.organization_id = $1", "l.deleted_at IS NULL"}
	args := []interface{}{organizationID, newLeadDays}
	argIdx := 3

	whereClauses = append(whereClauses, "(ai.urgency_level = 'High' OR l.created_at >= now() - ($2::int || ' days')::interval)")

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

func (r *Repository) Delete(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) error {
	result, err := r.pool.Exec(ctx, "UPDATE leads SET deleted_at = now(), updated_at = now() WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL", id, organizationID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) BulkDelete(ctx context.Context, ids []uuid.UUID, organizationID uuid.UUID) (int, error) {
	result, err := r.pool.Exec(ctx, "UPDATE leads SET deleted_at = now(), updated_at = now() WHERE id = ANY($1) AND organization_id = $2 AND deleted_at IS NULL", ids, organizationID)
	if err != nil {
		return 0, err
	}
	return int(result.RowsAffected()), nil
}
