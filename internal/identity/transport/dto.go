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
	Name string `json:"name" validate:"required,max=120"`
}

type OrganizationResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
