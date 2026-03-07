package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	servicesdb "portal_final_backend/internal/services/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"portal_final_backend/platform/apperr"
)

const serviceTypeNotFoundMessage = "service type not found"

// Repo implements the Repository interface with PostgreSQL.
type Repo struct {
	pool    *pgxpool.Pool
	queries *servicesdb.Queries
}

// New creates a new service types repository.
func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool, queries: servicesdb.New(pool)}
}

// Compile-time check that Repo implements Repository.
var _ Repository = (*Repo)(nil)

func toPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func toPgText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func optionalString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func toPgBoolPtr(value *bool) pgtype.Bool {
	if value == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *value, Valid: true}
}

func serviceSearchParam(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: "%" + value + "%", Valid: true}
}

func serviceTypeFromModel(model servicesdb.RacServiceType) ServiceType {
	return ServiceType{
		ID:                   uuid.UUID(model.ID.Bytes),
		OrganizationID:       uuid.UUID(model.OrganizationID.Bytes),
		Name:                 model.Name,
		Slug:                 model.Slug,
		Description:          optionalString(model.Description),
		IntakeGuidelines:     optionalString(model.IntakeGuidelines),
		EstimationGuidelines: optionalString(model.EstimationGuidelines),
		Icon:                 optionalString(model.Icon),
		Color:                optionalString(model.Color),
		IsActive:             model.IsActive,
		CreatedAt:            model.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:            model.UpdatedAt.Time.Format(time.RFC3339),
	}
}

// GetByID retrieves a service type by its ID.
func (r *Repo) GetByID(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (ServiceType, error) {
	row, err := r.queries.GetServiceTypeByID(ctx, servicesdb.GetServiceTypeByIDParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ServiceType{}, apperr.NotFound(serviceTypeNotFoundMessage)
		}
		return ServiceType{}, fmt.Errorf("get service type by id: %w", err)
	}

	return serviceTypeFromModel(servicesdb.RacServiceType{ID: row.ID, Name: row.Name, Slug: row.Slug, Description: row.Description, Icon: row.Icon, Color: row.Color, IsActive: row.IsActive, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID, IntakeGuidelines: row.IntakeGuidelines, EstimationGuidelines: row.EstimationGuidelines}), nil
}

// GetBySlug retrieves a service type by its slug.
func (r *Repo) GetBySlug(ctx context.Context, organizationID uuid.UUID, slug string) (ServiceType, error) {
	row, err := r.queries.GetServiceTypeBySlug(ctx, servicesdb.GetServiceTypeBySlugParams{Slug: slug, OrganizationID: toPgUUID(organizationID)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ServiceType{}, apperr.NotFound(serviceTypeNotFoundMessage)
		}
		return ServiceType{}, fmt.Errorf("get service type by slug: %w", err)
	}

	return serviceTypeFromModel(servicesdb.RacServiceType{ID: row.ID, Name: row.Name, Slug: row.Slug, Description: row.Description, Icon: row.Icon, Color: row.Color, IsActive: row.IsActive, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID, IntakeGuidelines: row.IntakeGuidelines, EstimationGuidelines: row.EstimationGuidelines}), nil
}

// List retrieves all service types ordered by name.
func (r *Repo) List(ctx context.Context, organizationID uuid.UUID) ([]ServiceType, error) {
	rows, err := r.queries.ListServiceTypes(ctx, toPgUUID(organizationID))
	if err != nil {
		return nil, fmt.Errorf("list service types: %w", err)
	}

	items := make([]ServiceType, 0, len(rows))
	for _, row := range rows {
		items = append(items, serviceTypeFromModel(servicesdb.RacServiceType{ID: row.ID, Name: row.Name, Slug: row.Slug, Description: row.Description, Icon: row.Icon, Color: row.Color, IsActive: row.IsActive, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID, IntakeGuidelines: row.IntakeGuidelines, EstimationGuidelines: row.EstimationGuidelines}))
	}

	return items, nil
}

// ListActive retrieves only active service types ordered by name.
func (r *Repo) ListActive(ctx context.Context, organizationID uuid.UUID) ([]ServiceType, error) {
	rows, err := r.queries.ListActiveServiceTypes(ctx, toPgUUID(organizationID))
	if err != nil {
		return nil, fmt.Errorf("list active service types: %w", err)
	}

	items := make([]ServiceType, 0, len(rows))
	for _, row := range rows {
		items = append(items, serviceTypeFromModel(servicesdb.RacServiceType{ID: row.ID, Name: row.Name, Slug: row.Slug, Description: row.Description, Icon: row.Icon, Color: row.Color, IsActive: row.IsActive, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID, IntakeGuidelines: row.IntakeGuidelines, EstimationGuidelines: row.EstimationGuidelines}))
	}

	return items, nil
}

// ListWithFilters retrieves service types with search, active filter, pagination, and sorting.
func (r *Repo) ListWithFilters(ctx context.Context, params ListParams) ([]ServiceType, int, error) {
	sortBy := "name"
	if params.SortBy != "" {
		switch params.SortBy {
		case "name", "slug", "isActive", "createdAt", "updatedAt":
			sortBy = params.SortBy
		default:
			return nil, 0, apperr.BadRequest("invalid sort field")
		}
	}

	sortOrder := "asc"
	if params.SortOrder != "" {
		switch params.SortOrder {
		case "asc", "desc":
			sortOrder = params.SortOrder
		default:
			return nil, 0, apperr.BadRequest("invalid sort order")
		}
	}

	total, err := r.queries.CountServiceTypes(ctx, servicesdb.CountServiceTypesParams{
		OrganizationID: toPgUUID(params.OrganizationID),
		Search:         serviceSearchParam(params.Search),
		IsActive:       toPgBoolPtr(params.IsActive),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count service types: %w", err)
	}
	rows, err := r.queries.ListServiceTypesWithFilters(ctx, servicesdb.ListServiceTypesWithFiltersParams{
		OrganizationID: toPgUUID(params.OrganizationID),
		Search:         serviceSearchParam(params.Search),
		IsActive:       toPgBoolPtr(params.IsActive),
		SortBy:         sortBy,
		SortOrder:      sortOrder,
		Offset:         int32(params.Offset),
		Limit:          int32(params.Limit),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list service types: %w", err)
	}

	items := make([]ServiceType, 0, len(rows))
	for _, row := range rows {
		items = append(items, serviceTypeFromModel(servicesdb.RacServiceType{ID: row.ID, Name: row.Name, Slug: row.Slug, Description: row.Description, Icon: row.Icon, Color: row.Color, IsActive: row.IsActive, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID, IntakeGuidelines: row.IntakeGuidelines, EstimationGuidelines: row.EstimationGuidelines}))
	}

	return items, int(total), nil
}

// Exists checks if a service type exists by ID.
func (r *Repo) Exists(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (bool, error) {
	exists, err := r.queries.ServiceTypeExists(ctx, servicesdb.ServiceTypeExistsParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID)})
	if err != nil {
		return false, fmt.Errorf("check service type exists: %w", err)
	}

	return exists, nil
}

// HasLeadServices checks if a service type is referenced by RAC_lead_services.
func (r *Repo) HasLeadServices(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (bool, error) {
	exists, err := r.queries.ServiceTypeHasLeadServices(ctx, servicesdb.ServiceTypeHasLeadServicesParams{ServiceTypeID: toPgUUID(id), OrganizationID: toPgUUID(organizationID)})
	if err != nil {
		return false, fmt.Errorf("check service type lead services: %w", err)
	}

	return exists, nil
}

// Create creates a new service type.
func (r *Repo) Create(ctx context.Context, params CreateParams) (ServiceType, error) {
	row, err := r.queries.CreateServiceType(ctx, servicesdb.CreateServiceTypeParams{
		OrganizationID:       toPgUUID(params.OrganizationID),
		Name:                 params.Name,
		Slug:                 params.Slug,
		Description:          toPgText(params.Description),
		IntakeGuidelines:     toPgText(params.IntakeGuidelines),
		EstimationGuidelines: toPgText(params.EstimationGuidelines),
		Icon:                 toPgText(params.Icon),
		Color:                toPgText(params.Color),
	})
	if err != nil {
		return ServiceType{}, fmt.Errorf("create service type: %w", err)
	}

	return serviceTypeFromModel(servicesdb.RacServiceType{ID: row.ID, Name: row.Name, Slug: row.Slug, Description: row.Description, Icon: row.Icon, Color: row.Color, IsActive: row.IsActive, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID, IntakeGuidelines: row.IntakeGuidelines, EstimationGuidelines: row.EstimationGuidelines}), nil
}

// Update updates an existing service type.
func (r *Repo) Update(ctx context.Context, params UpdateParams) (ServiceType, error) {
	row, err := r.queries.UpdateServiceType(ctx, servicesdb.UpdateServiceTypeParams{
		Name:                 toPgText(params.Name),
		Slug:                 toPgText(params.Slug),
		Description:          toPgText(params.Description),
		IntakeGuidelines:     toPgText(params.IntakeGuidelines),
		EstimationGuidelines: toPgText(params.EstimationGuidelines),
		Icon:                 toPgText(params.Icon),
		Color:                toPgText(params.Color),
		ID:                   toPgUUID(params.ID),
		OrganizationID:       toPgUUID(params.OrganizationID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ServiceType{}, apperr.NotFound(serviceTypeNotFoundMessage)
		}
		return ServiceType{}, fmt.Errorf("update service type: %w", err)
	}

	return serviceTypeFromModel(servicesdb.RacServiceType{ID: row.ID, Name: row.Name, Slug: row.Slug, Description: row.Description, Icon: row.Icon, Color: row.Color, IsActive: row.IsActive, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID, IntakeGuidelines: row.IntakeGuidelines, EstimationGuidelines: row.EstimationGuidelines}), nil
}

// Delete removes a service type by ID (hard delete).
// Use SetActive(false) for soft delete.
func (r *Repo) Delete(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) error {
	result, err := r.queries.DeleteServiceType(ctx, servicesdb.DeleteServiceTypeParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID)})
	if err != nil {
		return fmt.Errorf("delete service type: %w", err)
	}

	if result == 0 {
		return apperr.NotFound(serviceTypeNotFoundMessage)
	}

	return nil
}

// SetActive sets the is_active flag for a service type.
func (r *Repo) SetActive(ctx context.Context, organizationID uuid.UUID, id uuid.UUID, isActive bool) error {
	result, err := r.queries.SetServiceTypeActive(ctx, servicesdb.SetServiceTypeActiveParams{ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID), IsActive: isActive})
	if err != nil {
		return fmt.Errorf("set service type active: %w", err)
	}

	if result == 0 {
		return apperr.NotFound(serviceTypeNotFoundMessage)
	}

	return nil
}
