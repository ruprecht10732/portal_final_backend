package transport

import "github.com/google/uuid"

// CreateServiceTypeRequest contains data for creating a new service type.
type CreateServiceTypeRequest struct {
	Name             string  `json:"name" validate:"required,min=1,max=100"`
	Description      *string `json:"description,omitempty" validate:"omitempty,max=500"`
	IntakeGuidelines *string `json:"intakeGuidelines,omitempty" validate:"omitempty,max=2000"`
	Icon             *string `json:"icon,omitempty" validate:"omitempty,max=50"`
	Color            *string `json:"color,omitempty" validate:"omitempty,max=20"`
	DisplayOrder     *int    `json:"displayOrder,omitempty" validate:"omitempty,min=0"`
}

// UpdateServiceTypeRequest contains data for updating an existing service type.
type UpdateServiceTypeRequest struct {
	Name             *string `json:"name,omitempty" validate:"omitempty,min=1,max=100"`
	Description      *string `json:"description,omitempty" validate:"omitempty,max=500"`
	IntakeGuidelines *string `json:"intakeGuidelines,omitempty" validate:"omitempty,max=2000"`
	Icon             *string `json:"icon,omitempty" validate:"omitempty,max=50"`
	Color            *string `json:"color,omitempty" validate:"omitempty,max=20"`
	DisplayOrder     *int    `json:"displayOrder,omitempty" validate:"omitempty,min=0"`
}

// ReorderRequest contains the new order for service types.
type ReorderRequest struct {
	Items []ReorderItem `json:"items" validate:"required,min=1,dive"`
}

// ReorderItem represents a single item in a reorder request.
type ReorderItem struct {
	ID           uuid.UUID `json:"id" validate:"required"`
	DisplayOrder int       `json:"displayOrder" validate:"min=0"`
}

// ListServiceTypesRequest defines query params for admin listing.
type ListServiceTypesRequest struct {
	Search    string `form:"search" validate:"max=100"`
	IsActive  *bool  `form:"isActive" validate:"omitempty"`
	Page      int    `form:"page" validate:"min=1"`
	PageSize  int    `form:"pageSize" validate:"min=1,max=100"`
	SortBy    string `form:"sortBy" validate:"omitempty,oneof=name slug displayOrder isActive createdAt updatedAt"`
	SortOrder string `form:"sortOrder" validate:"omitempty,oneof=asc desc"`
}

// ServiceTypeResponse represents a service type in API responses.
type ServiceTypeResponse struct {
	ID               uuid.UUID `json:"id"`
	Name             string    `json:"name"`
	Slug             string    `json:"slug"`
	Description      *string   `json:"description,omitempty"`
	IntakeGuidelines *string   `json:"intakeGuidelines,omitempty"`
	Icon             *string   `json:"icon,omitempty"`
	Color            *string   `json:"color,omitempty"`
	IsActive         bool      `json:"isActive"`
	DisplayOrder     int       `json:"displayOrder"`
	CreatedAt        string    `json:"createdAt"`
	UpdatedAt        string    `json:"updatedAt"`
}

// ServiceTypeListResponse wraps a list of service types.
type ServiceTypeListResponse struct {
	Items      []ServiceTypeResponse `json:"items"`
	Total      int                   `json:"total"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"pageSize"`
	TotalPages int                   `json:"totalPages"`
}

// DeleteServiceTypeResponse indicates whether a service type was deleted or deactivated.
type DeleteServiceTypeResponse struct {
	Status string `json:"status"`
}
