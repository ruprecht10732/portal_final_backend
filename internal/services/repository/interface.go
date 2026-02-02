package repository

import (
	"context"

	"github.com/google/uuid"
)

// ServiceType represents a service category that can be assigned to leads.
type ServiceType struct {
	ID               uuid.UUID `db:"id"`
	OrganizationID   uuid.UUID `db:"organization_id"`
	Name             string    `db:"name"`
	Slug             string    `db:"slug"`
	Description      *string   `db:"description"`
	IntakeGuidelines *string   `db:"intake_guidelines"`
	Icon             *string   `db:"icon"`
	Color            *string   `db:"color"`
	IsActive         bool      `db:"is_active"`
	DisplayOrder     int       `db:"display_order"`
	CreatedAt        string    `db:"created_at"`
	UpdatedAt        string    `db:"updated_at"`
}

// CreateParams contains parameters for creating a service type.
type CreateParams struct {
	OrganizationID   uuid.UUID
	Name             string
	Slug             string
	Description      *string
	IntakeGuidelines *string
	Icon             *string
	Color            *string
	DisplayOrder     int
}

// UpdateParams contains parameters for updating a service type.
type UpdateParams struct {
	ID               uuid.UUID
	OrganizationID   uuid.UUID
	Name             *string
	Slug             *string
	Description      *string
	IntakeGuidelines *string
	Icon             *string
	Color            *string
	DisplayOrder     *int
}

// ReorderItem represents a single item in a reorder request.
type ReorderItem struct {
	ID           uuid.UUID
	DisplayOrder int
}

// ListParams defines filters for listing service types.
type ListParams struct {
	OrganizationID uuid.UUID
	Search         string
	IsActive       *bool
	Offset         int
	Limit          int
	SortBy         string
	SortOrder      string
}

// ServiceTypeReader provides read operations for service types.
type ServiceTypeReader interface {
	GetByID(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (ServiceType, error)
	GetBySlug(ctx context.Context, organizationID uuid.UUID, slug string) (ServiceType, error)
	List(ctx context.Context, organizationID uuid.UUID) ([]ServiceType, error)
	ListActive(ctx context.Context, organizationID uuid.UUID) ([]ServiceType, error)
	ListWithFilters(ctx context.Context, params ListParams) ([]ServiceType, int, error)
	Exists(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (bool, error)
	HasLeadServices(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) (bool, error)
}

// ServiceTypeWriter provides write operations for service types.
type ServiceTypeWriter interface {
	Create(ctx context.Context, params CreateParams) (ServiceType, error)
	Update(ctx context.Context, params UpdateParams) (ServiceType, error)
	Delete(ctx context.Context, organizationID uuid.UUID, id uuid.UUID) error
	SetActive(ctx context.Context, organizationID uuid.UUID, id uuid.UUID, isActive bool) error
	Reorder(ctx context.Context, organizationID uuid.UUID, items []ReorderItem) error
}

// Repository combines all service type repository operations.
type Repository interface {
	ServiceTypeReader
	ServiceTypeWriter
}
