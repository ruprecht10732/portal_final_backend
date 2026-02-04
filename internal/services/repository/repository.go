package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"portal_final_backend/platform/apperr"
)

const serviceTypeNotFoundMessage = "service type not found"

// Repo implements the Repository interface with PostgreSQL.
type Repo struct {
	pool *pgxpool.Pool
}

// New creates a new service types repository.
func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// Compile-time check that Repo implements Repository.
var _ Repository = (*Repo)(nil)

// GetByID retrieves a service type by its ID.
func (r *Repo) GetByID(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (ServiceType, error) {
	query := `
		SELECT id, organization_id, name, slug, description, intake_guidelines, icon, color, is_active, display_order, created_at, updated_at
		FROM RAC_service_types
		WHERE id = $1 AND organization_id = $2`

	var st ServiceType
	var createdAt, updatedAt time.Time

	err := r.pool.QueryRow(ctx, query, id, organizationID).Scan(
		&st.ID, &st.OrganizationID, &st.Name, &st.Slug, &st.Description, &st.IntakeGuidelines, &st.Icon, &st.Color,
		&st.IsActive, &st.DisplayOrder, &createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ServiceType{}, apperr.NotFound(serviceTypeNotFoundMessage)
		}
		return ServiceType{}, fmt.Errorf("get service type by id: %w", err)
	}

	st.CreatedAt = createdAt.Format(time.RFC3339)
	st.UpdatedAt = updatedAt.Format(time.RFC3339)

	return st, nil
}

// GetBySlug retrieves a service type by its slug.
func (r *Repo) GetBySlug(ctx context.Context, organizationID uuid.UUID, slug string) (ServiceType, error) {
	query := `
		SELECT id, organization_id, name, slug, description, intake_guidelines, icon, color, is_active, display_order, created_at, updated_at
		FROM RAC_service_types
		WHERE slug = $1 AND organization_id = $2`

	var st ServiceType
	var createdAt, updatedAt time.Time

	err := r.pool.QueryRow(ctx, query, slug, organizationID).Scan(
		&st.ID, &st.OrganizationID, &st.Name, &st.Slug, &st.Description, &st.IntakeGuidelines, &st.Icon, &st.Color,
		&st.IsActive, &st.DisplayOrder, &createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ServiceType{}, apperr.NotFound(serviceTypeNotFoundMessage)
		}
		return ServiceType{}, fmt.Errorf("get service type by slug: %w", err)
	}

	st.CreatedAt = createdAt.Format(time.RFC3339)
	st.UpdatedAt = updatedAt.Format(time.RFC3339)

	return st, nil
}

// List retrieves all service types ordered by display_order.
func (r *Repo) List(ctx context.Context, organizationID uuid.UUID) ([]ServiceType, error) {
	query := `
		SELECT id, organization_id, name, slug, description, intake_guidelines, icon, color, is_active, display_order, created_at, updated_at
		FROM RAC_service_types
		WHERE organization_id = $1
		ORDER BY display_order ASC, name ASC`

	rows, err := r.pool.Query(ctx, query, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list service types: %w", err)
	}
	defer rows.Close()

	return scanServiceTypes(rows)
}

// ListActive retrieves only active service types ordered by display_order.
func (r *Repo) ListActive(ctx context.Context, organizationID uuid.UUID) ([]ServiceType, error) {
	query := `
		SELECT id, organization_id, name, slug, description, intake_guidelines, icon, color, is_active, display_order, created_at, updated_at
		FROM RAC_service_types
		WHERE organization_id = $1 AND is_active = true
		ORDER BY display_order ASC, name ASC`

	rows, err := r.pool.Query(ctx, query, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list active service types: %w", err)
	}
	defer rows.Close()

	return scanServiceTypes(rows)
}

// ListWithFilters retrieves service types with search, active filter, pagination, and sorting.
// Service type sort column mappings.
var serviceTypeSortColumns = map[string]string{
	"name":         "name",
	"slug":         "slug",
	"displayOrder": "display_order",
	"isActive":     "is_active",
	"createdAt":    "created_at",
	"updatedAt":    "updated_at",
}

// mapServiceTypeSortColumn returns the DB column for sorting.
func mapServiceTypeSortColumn(sortBy string) (string, error) {
	if sortBy == "" {
		return "display_order", nil
	}
	if col, ok := serviceTypeSortColumns[sortBy]; ok {
		return col, nil
	}
	return "", apperr.BadRequest("invalid sort field")
}

func (r *Repo) ListWithFilters(ctx context.Context, params ListParams) ([]ServiceType, int, error) {
	whereClauses, args := r.buildServiceTypeFilters(params)
	whereClause := strings.Join(whereClauses, " AND ")

	total, err := r.countServiceTypes(ctx, whereClause, args)
	if err != nil {
		return nil, 0, err
	}

	sortColumn, err := mapServiceTypeSortColumn(params.SortBy)
	if err != nil {
		return nil, 0, err
	}

	sortOrder, err := mapSortOrderASC(params.SortOrder)
	if err != nil {
		return nil, 0, err
	}

	items, err := r.fetchServiceTypes(ctx, whereClause, sortColumn, sortOrder, args, params.Limit, params.Offset)
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// buildServiceTypeFilters constructs WHERE clause predicates.
func (r *Repo) buildServiceTypeFilters(params ListParams) ([]string, []interface{}) {
	whereClauses := []string{"organization_id = $1"}
	args := []interface{}{params.OrganizationID}
	argIdx := 2

	if params.IsActive != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *params.IsActive)
		argIdx++
	}
	if params.Search != "" {
		searchPattern := "%" + params.Search + "%"
		whereClauses = append(whereClauses, fmt.Sprintf("(name ILIKE $%d OR slug ILIKE $%d)", argIdx, argIdx))
		args = append(args, searchPattern)
	}
	return whereClauses, args
}

// countServiceTypes returns the total count matching the filter.
func (r *Repo) countServiceTypes(ctx context.Context, whereClause string, args []interface{}) (int, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM RAC_service_types WHERE %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count service types: %w", err)
	}
	return total, nil
}

// mapSortOrderASC returns validated sort order, defaulting to ASC.
func mapSortOrderASC(sortOrder string) (string, error) {
	switch sortOrder {
	case "", "asc":
		return "ASC", nil
	case "desc":
		return "DESC", nil
	default:
		return "", apperr.BadRequest("invalid sort order")
	}
}

// fetchServiceTypes executes the paginated query.
func (r *Repo) fetchServiceTypes(ctx context.Context, whereClause, sortColumn, sortOrder string, args []interface{}, limit, offset int) ([]ServiceType, error) {
	argIdx := len(args) + 1
	args = append(args, limit, offset)

	query := fmt.Sprintf(`
		SELECT id, organization_id, name, slug, description, intake_guidelines, icon, color, is_active, display_order, created_at, updated_at
		FROM RAC_service_types
		WHERE %s
		ORDER BY %s %s, name ASC
		LIMIT $%d OFFSET $%d
	`, whereClause, sortColumn, sortOrder, argIdx, argIdx+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list service types: %w", err)
	}
	defer rows.Close()

	return scanServiceTypes(rows)
}

// Exists checks if a service type exists by ID.
func (r *Repo) Exists(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM RAC_service_types WHERE id = $1 AND organization_id = $2)`

	var exists bool
	err := r.pool.QueryRow(ctx, query, id, organizationID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check service type exists: %w", err)
	}

	return exists, nil
}

// HasLeadServices checks if a service type is referenced by RAC_lead_services.
func (r *Repo) HasLeadServices(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM RAC_lead_services WHERE service_type_id = $1 AND organization_id = $2)`

	var exists bool
	if err := r.pool.QueryRow(ctx, query, id, organizationID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check service type lead services: %w", err)
	}

	return exists, nil
}

// Create creates a new service type.
func (r *Repo) Create(ctx context.Context, params CreateParams) (ServiceType, error) {
	query := `
		INSERT INTO RAC_service_types (organization_id, name, slug, description, intake_guidelines, icon, color, display_order)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, organization_id, name, slug, description, intake_guidelines, icon, color, is_active, display_order, created_at, updated_at`

	var st ServiceType
	var createdAt, updatedAt time.Time

	err := r.pool.QueryRow(ctx, query,
		params.OrganizationID, params.Name, params.Slug, params.Description, params.IntakeGuidelines, params.Icon, params.Color, params.DisplayOrder,
	).Scan(
		&st.ID, &st.OrganizationID, &st.Name, &st.Slug, &st.Description, &st.IntakeGuidelines, &st.Icon, &st.Color,
		&st.IsActive, &st.DisplayOrder, &createdAt, &updatedAt,
	)
	if err != nil {
		return ServiceType{}, fmt.Errorf("create service type: %w", err)
	}

	st.CreatedAt = createdAt.Format(time.RFC3339)
	st.UpdatedAt = updatedAt.Format(time.RFC3339)

	return st, nil
}

// Update updates an existing service type.
func (r *Repo) Update(ctx context.Context, params UpdateParams) (ServiceType, error) {
	// Build dynamic update query
	query := `
		UPDATE RAC_service_types SET
			name = COALESCE($2, name),
			slug = COALESCE($3, slug),
			description = COALESCE($4, description),
			intake_guidelines = COALESCE($5, intake_guidelines),
			icon = COALESCE($6, icon),
			color = COALESCE($7, color),
			display_order = COALESCE($8, display_order),
			updated_at = now()
		WHERE id = $1 AND organization_id = $9
		RETURNING id, organization_id, name, slug, description, intake_guidelines, icon, color, is_active, display_order, created_at, updated_at`

	var st ServiceType
	var createdAt, updatedAt time.Time

	err := r.pool.QueryRow(ctx, query,
		params.ID, params.Name, params.Slug, params.Description, params.IntakeGuidelines, params.Icon, params.Color, params.DisplayOrder, params.OrganizationID,
	).Scan(
		&st.ID, &st.OrganizationID, &st.Name, &st.Slug, &st.Description, &st.IntakeGuidelines, &st.Icon, &st.Color,
		&st.IsActive, &st.DisplayOrder, &createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ServiceType{}, apperr.NotFound(serviceTypeNotFoundMessage)
		}
		return ServiceType{}, fmt.Errorf("update service type: %w", err)
	}

	st.CreatedAt = createdAt.Format(time.RFC3339)
	st.UpdatedAt = updatedAt.Format(time.RFC3339)

	return st, nil
}

// Delete removes a service type by ID (hard delete).
// Use SetActive(false) for soft delete.
func (r *Repo) Delete(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) error {
	query := `DELETE FROM RAC_service_types WHERE id = $1 AND organization_id = $2`

	result, err := r.pool.Exec(ctx, query, id, organizationID)
	if err != nil {
		return fmt.Errorf("delete service type: %w", err)
	}

	if result.RowsAffected() == 0 {
		return apperr.NotFound(serviceTypeNotFoundMessage)
	}

	return nil
}

// SetActive sets the is_active flag for a service type.
func (r *Repo) SetActive(ctx context.Context, organizationID uuid.UUID, id uuid.UUID, isActive bool) error {
	query := `UPDATE RAC_service_types SET is_active = $3, updated_at = now() WHERE id = $1 AND organization_id = $2`

	result, err := r.pool.Exec(ctx, query, id, organizationID, isActive)
	if err != nil {
		return fmt.Errorf("set service type active: %w", err)
	}

	if result.RowsAffected() == 0 {
		return apperr.NotFound(serviceTypeNotFoundMessage)
	}

	return nil
}

// Reorder updates the display_order of multiple service types in a single transaction.
func (r *Repo) Reorder(ctx context.Context, organizationID uuid.UUID, items []ReorderItem) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	query := `UPDATE RAC_service_types SET display_order = $2, updated_at = now() WHERE id = $1 AND organization_id = $3`

	for _, item := range items {
		_, err := tx.Exec(ctx, query, item.ID, item.DisplayOrder, organizationID)
		if err != nil {
			return fmt.Errorf("update display order for %s: %w", item.ID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// scanServiceTypes is a helper to scan multiple rows into ServiceType slice.
func scanServiceTypes(rows pgx.Rows) ([]ServiceType, error) {
	var results []ServiceType

	for rows.Next() {
		var st ServiceType
		var createdAt, updatedAt time.Time

		err := rows.Scan(
			&st.ID, &st.OrganizationID, &st.Name, &st.Slug, &st.Description, &st.IntakeGuidelines, &st.Icon, &st.Color,
			&st.IsActive, &st.DisplayOrder, &createdAt, &updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan service type: %w", err)
		}

		st.CreatedAt = createdAt.Format(time.RFC3339)
		st.UpdatedAt = updatedAt.Format(time.RFC3339)

		results = append(results, st)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate service types: %w", err)
	}

	return results, nil
}
