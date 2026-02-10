package transport

import "time"

type SignUpRequest struct {
	Email            string  `json:"email" validate:"required,email"`
	Password         string  `json:"password" validate:"required,strongpassword"`
	OrganizationName *string `json:"organizationName" validate:"omitempty,max=120"`
	InviteToken      *string `json:"inviteToken" validate:"omitempty"`
}

type SignInRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token" validate:"required"`
	NewPassword string `json:"newPassword" validate:"required,strongpassword"`
}

type VerifyEmailRequest struct {
	Token string `json:"token" validate:"required"`
}

type ResolveInviteResponse struct {
	Email            string `json:"email"`
	OrganizationName string `json:"organizationName"`
}

type RoleUpdateRequest struct {
	Roles []string `json:"roles" validate:"required,min=1,dive,required"`
}

type RoleUpdateResponse struct {
	UserID string   `json:"userId"`
	Roles  []string `json:"roles"`
}

type AuthResponse struct {
	AccessToken string `json:"accessToken"`
}

type ProfileResponse struct {
	ID              string    `json:"id"`
	Email           string    `json:"email"`
	EmailVerified   bool      `json:"emailVerified"`
	FirstName       *string   `json:"firstName"`
	LastName        *string   `json:"lastName"`
	PreferredLang   string    `json:"preferredLanguage"`
	Roles           []string  `json:"roles"`
	HasOrganization bool      `json:"hasOrganization"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type UpdateProfileRequest struct {
	Email             *string `json:"email" validate:"omitempty,email"`
	FirstName         *string `json:"firstName" validate:"omitempty,max=100"`
	LastName          *string `json:"lastName" validate:"omitempty,max=100"`
	PreferredLanguage *string `json:"preferredLanguage" validate:"omitempty,oneof=en nl"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" validate:"required"`
	NewPassword     string `json:"newPassword" validate:"required,strongpassword"`
}

type CompleteOnboardingRequest struct {
	FirstName          string  `json:"firstName" validate:"required,max=100"`
	LastName           string  `json:"lastName" validate:"required,max=100"`
	OrganizationName   *string `json:"organizationName" validate:"omitempty,max=120"`
	OrganizationEmail  *string `json:"organizationEmail" validate:"omitempty,email"`
	OrganizationPhone  *string `json:"organizationPhone" validate:"omitempty,max=50"`
	VatNumber          *string `json:"vatNumber" validate:"omitempty,max=20"`
	KvkNumber          *string `json:"kvkNumber" validate:"omitempty,max=20"`
	AddressLine1       *string `json:"addressLine1" validate:"omitempty,max=200"`
	AddressLine2       *string `json:"addressLine2" validate:"omitempty,max=200"`
	PostalCode         *string `json:"postalCode" validate:"omitempty,max=20"`
	City               *string `json:"city" validate:"omitempty,max=120"`
	Country            *string `json:"country" validate:"omitempty,max=120"`
}

type UserSummary struct {
	ID        string   `json:"id"`
	Email     string   `json:"email"`
	FirstName *string  `json:"firstName"`
	LastName  *string  `json:"lastName"`
	Roles     []string `json:"roles"`
}
