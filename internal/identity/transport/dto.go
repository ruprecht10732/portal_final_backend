package transport

import "time"

type CreateInviteRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type CreateInviteResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type UpdateOrganizationRequest struct {
	Name         *string `json:"name" validate:"omitempty,max=120"`
	Email        *string `json:"email" validate:"omitempty,email"`
	Phone        *string `json:"phone" validate:"omitempty,max=50"`
	VatNumber    *string `json:"vatNumber" validate:"omitempty,max=20"`
	KvkNumber    *string `json:"kvkNumber" validate:"omitempty,max=20"`
	AddressLine1 *string `json:"addressLine1" validate:"omitempty,max=200"`
	AddressLine2 *string `json:"addressLine2" validate:"omitempty,max=200"`
	PostalCode   *string `json:"postalCode" validate:"omitempty,max=20"`
	City         *string `json:"city" validate:"omitempty,max=120"`
	Country      *string `json:"country" validate:"omitempty,max=120"`
}

type OrganizationResponse struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Email        *string `json:"email,omitempty"`
	Phone        *string `json:"phone,omitempty"`
	VatNumber    *string `json:"vatNumber,omitempty"`
	KvkNumber    *string `json:"kvkNumber,omitempty"`
	AddressLine1 *string `json:"addressLine1,omitempty"`
	AddressLine2 *string `json:"addressLine2,omitempty"`
	PostalCode   *string `json:"postalCode,omitempty"`
	City         *string `json:"city,omitempty"`
	Country      *string `json:"country,omitempty"`
}
