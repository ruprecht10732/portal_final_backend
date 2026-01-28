package transport

type SignUpRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
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
	NewPassword string `json:"newPassword" validate:"required,min=8"`
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
