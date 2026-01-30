package transport

import "github.com/google/uuid"

// CreateServiceTypeRequest contains data for creating a new service type.
type CreateServiceTypeRequest struct {
	Name         string  `json:"name" validate:"required,min=1,max=100"`
	Description  *string `json:"description,omitempty" validate:"omitempty,max=500"`
	Icon         *string `json:"icon,omitempty" validate:"omitempty,max=50"`
	Color        *string `json:"color,omitempty" validate:"omitempty,max=20"`
	DisplayOrder *int    `json:"displayOrder,omitempty" validate:"omitempty,min=0"`
}

// UpdateServiceTypeRequest contains data for updating an existing service type.
type UpdateServiceTypeRequest struct {
	Name         *string `json:"name,omitempty" validate:"omitempty,min=1,max=100"`
	Description  *string `json:"description,omitempty" validate:"omitempty,max=500"`
	Icon         *string `json:"icon,omitempty" validate:"omitempty,max=50"`
	Color        *string `json:"color,omitempty" validate:"omitempty,max=20"`
	DisplayOrder *int    `json:"displayOrder,omitempty" validate:"omitempty,min=0"`
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

// ServiceTypeResponse represents a service type in API responses.
type ServiceTypeResponse struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Description  *string   `json:"description,omitempty"`
	Icon         *string   `json:"icon,omitempty"`
	Color        *string   `json:"color,omitempty"`
	IsActive     bool      `json:"isActive"`
	DisplayOrder int       `json:"displayOrder"`
	CreatedAt    string    `json:"createdAt"`
	UpdatedAt    string    `json:"updatedAt"`
}

// ServiceTypeListResponse wraps a list of service types.
type ServiceTypeListResponse struct {
	Items []ServiceTypeResponse `json:"items"`
	Total int                   `json:"total"`
}
