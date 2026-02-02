package transport

import "github.com/google/uuid"

// VAT Rates

type CreateVatRateRequest struct {
	Name    string `json:"name" validate:"required,min=1,max=100"`
	RateBps int    `json:"rateBps" validate:"required,min=0,max=10000"`
}

type UpdateVatRateRequest struct {
	Name    *string `json:"name,omitempty" validate:"omitempty,min=1,max=100"`
	RateBps *int    `json:"rateBps,omitempty" validate:"omitempty,min=0,max=10000"`
}

type ListVatRatesRequest struct {
	Search    string `form:"search" validate:"max=100"`
	Page      int    `form:"page" validate:"omitempty,min=1"`
	PageSize  int    `form:"pageSize" validate:"omitempty,min=1,max=100"`
	SortBy    string `form:"sortBy" validate:"omitempty,oneof=name rateBps createdAt updatedAt"`
	SortOrder string `form:"sortOrder" validate:"omitempty,oneof=asc desc"`
}

type VatRateResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	RateBps   int       `json:"rateBps"`
	CreatedAt string    `json:"createdAt"`
	UpdatedAt string    `json:"updatedAt"`
}

type VatRateListResponse struct {
	Items      []VatRateResponse `json:"items"`
	Total      int               `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"pageSize"`
	TotalPages int               `json:"totalPages"`
}

// Products

type CreateProductRequest struct {
	Title       string    `json:"title" validate:"required,min=1,max=200"`
	Reference   string    `json:"reference" validate:"required,min=1,max=100"`
	Description *string   `json:"description,omitempty" validate:"omitempty,max=1000"`
	PriceCents  int       `json:"priceCents" validate:"required,min=0"`
	VatRateID   uuid.UUID `json:"vatRateId" validate:"required"`
	Type        string    `json:"type" validate:"required,oneof=digital_service service product material"`
	PeriodCount *int      `json:"periodCount,omitempty" validate:"omitempty,min=1"`
	PeriodUnit  *string   `json:"periodUnit,omitempty" validate:"omitempty,oneof=day week month quarter year"`
}

type UpdateProductRequest struct {
	Title       *string    `json:"title,omitempty" validate:"omitempty,min=1,max=200"`
	Reference   *string    `json:"reference,omitempty" validate:"omitempty,min=1,max=100"`
	Description *string    `json:"description,omitempty" validate:"omitempty,max=1000"`
	PriceCents  *int       `json:"priceCents,omitempty" validate:"omitempty,min=0"`
	VatRateID   *uuid.UUID `json:"vatRateId,omitempty" validate:"omitempty"`
	Type        *string    `json:"type,omitempty" validate:"omitempty,oneof=digital_service service product material"`
	PeriodCount *int       `json:"periodCount,omitempty" validate:"omitempty,min=1"`
	PeriodUnit  *string    `json:"periodUnit,omitempty" validate:"omitempty,oneof=day week month quarter year"`
}

type ListProductsRequest struct {
	Search    string `form:"search" validate:"max=100"`
	Type      string `form:"type" validate:"omitempty,oneof=digital_service service product material"`
	VatRateID string `form:"vatRateId" validate:"omitempty"`
	Page      int    `form:"page" validate:"omitempty,min=1"`
	PageSize  int    `form:"pageSize" validate:"omitempty,min=1,max=100"`
	SortBy    string `form:"sortBy" validate:"omitempty,oneof=title reference priceCents type createdAt updatedAt"`
	SortOrder string `form:"sortOrder" validate:"omitempty,oneof=asc desc"`
}

type ProductResponse struct {
	ID          uuid.UUID `json:"id"`
	VatRateID   uuid.UUID `json:"vatRateId"`
	Title       string    `json:"title"`
	Reference   string    `json:"reference"`
	Description *string   `json:"description,omitempty"`
	PriceCents  int       `json:"priceCents"`
	Type        string    `json:"type"`
	PeriodCount *int      `json:"periodCount,omitempty"`
	PeriodUnit  *string   `json:"periodUnit,omitempty"`
	CreatedAt   string    `json:"createdAt"`
	UpdatedAt   string    `json:"updatedAt"`
}

type ProductListResponse struct {
	Items      []ProductResponse `json:"items"`
	Total      int               `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"pageSize"`
	TotalPages int               `json:"totalPages"`
}

// Materials

type ProductMaterialsRequest struct {
	MaterialIDs []uuid.UUID `json:"materialIds" validate:"required,min=1,dive,required"`
}
