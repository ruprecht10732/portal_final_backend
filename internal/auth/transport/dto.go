package transport

import "time"

type SignUpRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,strongpassword"`
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
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"emailVerified"`
	Roles         []string  `json:"roles"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type UpdateProfileRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" validate:"required"`
	NewPassword     string `json:"newPassword" validate:"required,strongpassword"`
}

type UserSummary struct {
	ID    string   `json:"id"`
	Email string   `json:"email"`
	Roles []string `json:"roles"`
}
