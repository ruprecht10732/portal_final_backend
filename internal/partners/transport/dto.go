package transport

import (
	"time"

	"github.com/google/uuid"
)

type CreatePartnerRequest struct {
	BusinessName   string      `json:"businessName" validate:"required,min=1,max=200"`
	KVKNumber      string      `json:"kvkNumber" validate:"required,max=20"`
	VATNumber      string      `json:"vatNumber" validate:"required,max=20"`
	AddressLine1   string      `json:"addressLine1" validate:"required,max=200"`
	AddressLine2   string      `json:"addressLine2,omitempty" validate:"omitempty,max=200"`
	HouseNumber    string      `json:"houseNumber" validate:"required,max=20"`
	PostalCode     string      `json:"postalCode" validate:"required,max=20"`
	City           string      `json:"city" validate:"required,max=120"`
	Country        string      `json:"country" validate:"required,max=120"`
	Latitude       *float64    `json:"latitude,omitempty" validate:"omitempty,gte=-90,lte=90"`
	Longitude      *float64    `json:"longitude,omitempty" validate:"omitempty,gte=-180,lte=180"`
	ContactName    string      `json:"contactName" validate:"required,max=120"`
	ContactEmail   string      `json:"contactEmail" validate:"required,email"`
	ContactPhone   string      `json:"contactPhone" validate:"required,max=50"`
	ServiceTypeIDs []uuid.UUID `json:"serviceTypeIds,omitempty" validate:"omitempty,dive,required"`
}

type UpdatePartnerRequest struct {
	BusinessName   *string      `json:"businessName,omitempty" validate:"omitempty,min=1,max=200"`
	KVKNumber      *string      `json:"kvkNumber,omitempty" validate:"omitempty,max=20"`
	VATNumber      *string      `json:"vatNumber,omitempty" validate:"omitempty,max=20"`
	AddressLine1   *string      `json:"addressLine1,omitempty" validate:"omitempty,max=200"`
	AddressLine2   *string      `json:"addressLine2,omitempty" validate:"omitempty,max=200"`
	HouseNumber    *string      `json:"houseNumber,omitempty" validate:"omitempty,max=20"`
	PostalCode     *string      `json:"postalCode,omitempty" validate:"omitempty,max=20"`
	City           *string      `json:"city,omitempty" validate:"omitempty,max=120"`
	Country        *string      `json:"country,omitempty" validate:"omitempty,max=120"`
	Latitude       *float64     `json:"latitude,omitempty" validate:"omitempty,gte=-90,lte=90"`
	Longitude      *float64     `json:"longitude,omitempty" validate:"omitempty,gte=-180,lte=180"`
	ContactName    *string      `json:"contactName,omitempty" validate:"omitempty,max=120"`
	ContactEmail   *string      `json:"contactEmail,omitempty" validate:"omitempty,email"`
	ContactPhone   *string      `json:"contactPhone,omitempty" validate:"omitempty,max=50"`
	ServiceTypeIDs *[]uuid.UUID `json:"serviceTypeIds,omitempty" validate:"omitempty,dive,required"`
}

type PartnerResponse struct {
	ID              uuid.UUID   `json:"id"`
	BusinessName    string      `json:"businessName"`
	KVKNumber       string      `json:"kvkNumber"`
	VATNumber       string      `json:"vatNumber"`
	AddressLine1    string      `json:"addressLine1"`
	AddressLine2    *string     `json:"addressLine2,omitempty"`
	HouseNumber     *string     `json:"houseNumber,omitempty"`
	PostalCode      string      `json:"postalCode"`
	City            string      `json:"city"`
	Country         string      `json:"country"`
	Latitude        *float64    `json:"latitude,omitempty"`
	Longitude       *float64    `json:"longitude,omitempty"`
	ContactName     string      `json:"contactName"`
	ContactEmail    string      `json:"contactEmail"`
	ContactPhone    string      `json:"contactPhone"`
	LogoFileKey     *string     `json:"logoFileKey,omitempty"`
	LogoFileName    *string     `json:"logoFileName,omitempty"`
	LogoContentType *string     `json:"logoContentType,omitempty"`
	LogoSizeBytes   *int64      `json:"logoSizeBytes,omitempty"`
	ServiceTypeIDs  []uuid.UUID `json:"serviceTypeIds,omitempty"`
	CreatedAt       time.Time   `json:"createdAt"`
	UpdatedAt       time.Time   `json:"updatedAt"`
}

type ListPartnersRequest struct {
	Search    string `form:"search" validate:"omitempty,max=100"`
	SortBy    string `form:"sortBy" validate:"omitempty,oneof=businessName contactName createdAt updatedAt"`
	SortOrder string `form:"sortOrder" validate:"omitempty,oneof=asc desc"`
	Page      int    `form:"page" validate:"omitempty,min=1"`
	PageSize  int    `form:"pageSize" validate:"omitempty,min=1,max=100"`
}

type ListPartnersResponse struct {
	Items      []PartnerResponse `json:"items"`
	Total      int               `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"pageSize"`
	TotalPages int               `json:"totalPages"`
}

type PartnerLeadResponse struct {
	ID        uuid.UUID `json:"id"`
	FirstName string    `json:"firstName"`
	LastName  string    `json:"lastName"`
	Phone     string    `json:"phone"`
	Address   string    `json:"address"`
}

type LinkLeadRequest struct {
	LeadID uuid.UUID `json:"leadId" validate:"required"`
}

type CreatePartnerInviteRequest struct {
	Email         string     `json:"email" validate:"required,email"`
	LeadID        *uuid.UUID `json:"leadId,omitempty"`
	LeadServiceID *uuid.UUID `json:"leadServiceId,omitempty"`
}

type CreatePartnerInviteResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type PartnerInviteResponse struct {
	ID            uuid.UUID  `json:"id"`
	Email         string     `json:"email"`
	LeadID        *uuid.UUID `json:"leadId,omitempty"`
	LeadServiceID *uuid.UUID `json:"leadServiceId,omitempty"`
	ExpiresAt     time.Time  `json:"expiresAt"`
	CreatedAt     time.Time  `json:"createdAt"`
	UsedAt        *time.Time `json:"usedAt,omitempty"`
}

type ListPartnerInvitesResponse struct {
	Invites []PartnerInviteResponse `json:"invites"`
}
