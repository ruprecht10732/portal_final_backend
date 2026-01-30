package repository

import (
	"context"

	"github.com/google/uuid"
)

// ServiceType represents a service category that can be assigned to leads.
type ServiceType struct {
	ID           uuid.UUID `db:"id"`
	Name         string    `db:"name"`
	Slug         string    `db:"slug"`
	Description  *string   `db:"description"`
	Icon         *string   `db:"icon"`
	Color        *string   `db:"color"`
	IsActive     bool      `db:"is_active"`
	DisplayOrder int       `db:"display_order"`
	CreatedAt    string    `db:"created_at"`
	UpdatedAt    string    `db:"updated_at"`
}

// CreateParams contains parameters for creating a service type.
type CreateParams struct {
	Name         string
	Slug         string
	Description  *string
	Icon         *string
	Color        *string
	DisplayOrder int
}

// UpdateParams contains parameters for updating a service type.
type UpdateParams struct {
	ID           uuid.UUID
	Name         *string
	Slug         *string
	Description  *string
	Icon         *string
	Color        *string
	DisplayOrder *int
}

// ReorderItem represents a single item in a reorder request.
type ReorderItem struct {
	ID           uuid.UUID
	DisplayOrder int
}

// ServiceTypeReader provides read operations for service types.
type ServiceTypeReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (ServiceType, error)
	GetBySlug(ctx context.Context, slug string) (ServiceType, error)
	List(ctx context.Context) ([]ServiceType, error)
	ListActive(ctx context.Context) ([]ServiceType, error)
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
}

// ServiceTypeWriter provides write operations for service types.
type ServiceTypeWriter interface {
	Create(ctx context.Context, params CreateParams) (ServiceType, error)
	Update(ctx context.Context, params UpdateParams) (ServiceType, error)
	Delete(ctx context.Context, id uuid.UUID) error
	SetActive(ctx context.Context, id uuid.UUID, isActive bool) error
	Reorder(ctx context.Context, items []ReorderItem) error
}

// Repository combines all service type repository operations.
type Repository interface {
	ServiceTypeReader
	ServiceTypeWriter
}
