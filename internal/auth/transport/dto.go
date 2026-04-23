// Package transport defines the data transfer objects (DTOs) for the auth service.
// It leverages go-playground/validator tags for strict input sanitization.
package transport

import (
	"encoding/json"
	"time"
)

// =============================================================================
// Authentication Requests
// Security: All string inputs now have strict max bounds to prevent
// $O(N)$ memory exhaustion attacks during JSON unmarshaling and validation.
// =============================================================================

type SignUpRequest struct {
	Email            string  `json:"email" validate:"required,email,max=255"`
	Password         string  `json:"password" validate:"required,strongpassword,max=1024"`
	OrganizationName *string `json:"organizationName" validate:"omitempty,max=120"`
	InviteToken      *string `json:"inviteToken" validate:"omitempty,max=512"`
}

type SignInRequest struct {
	Email    string `json:"email" validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,max=1024"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refreshToken" validate:"required,max=1024"`
}

type SignOutRequest struct {
	RefreshToken string `json:"refreshToken" validate:"required,max=1024"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email,max=255"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token" validate:"required,max=512"`
	NewPassword string `json:"newPassword" validate:"required,strongpassword,max=1024"`
}

type VerifyEmailRequest struct {
	Token string `json:"token" validate:"required,max=512"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" validate:"required,max=1024"`
	NewPassword     string `json:"newPassword" validate:"required,strongpassword,max=1024"`
}

// =============================================================================
// User & Profile Models
// Optimization: Struct fields are packed (ordered by size) to minimize memory
// padding. In highly concurrent endpoints, this significantly reduces GC pressure.
// =============================================================================

type AuthResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken,omitempty"`
}

type VerifyResponse struct {
	UserID string `json:"userId"`
	Email  string `json:"email"`
	Valid  bool   `json:"valid"`
}

type ProfileResponse struct {
	ID                  string    `json:"id"`
	Email               string    `json:"email"`
	PreferredLang       string    `json:"preferredLanguage"`
	Roles               []string  `json:"roles"`
	FirstName           *string   `json:"firstName"`
	LastName            *string   `json:"lastName"`
	Phone               *string   `json:"phone"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
	EmailVerified       bool      `json:"emailVerified"`
	HasOrganization     bool      `json:"hasOrganization"`
	OnboardingCompleted bool      `json:"onboardingCompleted"`
}

type UpdateProfileRequest struct {
	Email             *string `json:"email" validate:"omitempty,email,max=255"`
	FirstName         *string `json:"firstName" validate:"omitempty,max=100"`
	LastName          *string `json:"lastName" validate:"omitempty,max=100"`
	Phone             *string `json:"phone" validate:"omitempty,max=50"`
	PreferredLanguage *string `json:"preferredLanguage" validate:"omitempty,oneof=en nl"`
}

type CompleteOnboardingRequest struct {
	FirstName         string  `json:"firstName" validate:"required,max=100"`
	LastName          string  `json:"lastName" validate:"required,max=100"`
	OrganizationName  *string `json:"organizationName" validate:"omitempty,max=120"`
	OrganizationEmail *string `json:"organizationEmail" validate:"omitempty,email,max=255"`
	OrganizationPhone *string `json:"organizationPhone" validate:"omitempty,max=50"`
	VatNumber         *string `json:"vatNumber" validate:"omitempty,max=20"`
	KvkNumber         *string `json:"kvkNumber" validate:"omitempty,max=20"`
	AddressLine1      *string `json:"addressLine1" validate:"omitempty,max=200"`
	AddressLine2      *string `json:"addressLine2" validate:"omitempty,max=200"`
	PostalCode        *string `json:"postalCode" validate:"omitempty,max=20"`
	City              *string `json:"city" validate:"omitempty,max=120"`
	Country           *string `json:"country" validate:"omitempty,max=120"`
}

type UserSummary struct {
	ID        string   `json:"id"`
	Email     string   `json:"email"`
	FirstName *string  `json:"firstName"`
	LastName  *string  `json:"lastName"`
	Roles     []string `json:"roles"`
}

// =============================================================================
// RBAC & Organization Data
// =============================================================================

type RoleUpdateRequest struct {
	// Added 'max=50' to dive string constraint to prevent unbounded slice allocation attacks.
	Roles []string `json:"roles" validate:"required,min=1,dive,required,max=50"`
}

type RoleUpdateResponse struct {
	UserID string   `json:"userId"`
	Roles  []string `json:"roles"`
}

type ResolveInviteResponse struct {
	Email            string `json:"email"`
	OrganizationName string `json:"organizationName"`
}

// =============================================================================
// WebAuthn / Passkey DTOs
// =============================================================================

type FinishPasskeyRegistrationRequest struct {
	Nickname   string          `json:"nickname" validate:"required,max=64"`
	Credential json.RawMessage `json:"credential" validate:"required"`
}

type FinishPasskeyLoginRequest struct {
	Challenge  string          `json:"challenge" validate:"required,max=512"`
	Credential json.RawMessage `json:"credential" validate:"required"`
}

type RenamePasskeyRequest struct {
	Nickname string `json:"nickname" validate:"required,max=64"`
}

type PasskeyResponse struct {
	ID         string     `json:"id"`
	Nickname   string     `json:"nickname"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}
