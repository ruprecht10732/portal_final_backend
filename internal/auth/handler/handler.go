package handler

import (
	"net/http"
	"strings"
	"time"

	"portal_final_backend/internal/auth/service"
	"portal_final_backend/internal/auth/transport"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	msgInvalidRequest   = "invalid request"
	msgValidationFailed = "validation failed"

	headerAuthorization = "Authorization"
	bearerPrefix        = "Bearer "
)

// Handler manages HTTP requests for the authentication domain.
type Handler struct {
	svc *service.Service
	cfg config.CookieConfig
	val *validator.Validator
}

// New creates and returns a new auth Handler.
func New(svc *service.Service, cfg config.CookieConfig, val *validator.Validator) *Handler {
	return &Handler{
		svc: svc,
		cfg: cfg,
		val: val,
	}
}

// RegisterRoutes binds the authentication endpoints to the provided Gin router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/sign-up", h.SignUp)
	rg.POST("/sign-in", h.SignIn)
	rg.POST("/refresh", h.Refresh)
	rg.POST("/sign-out", h.SignOut)
	rg.POST("/forgot-password", h.ForgotPassword)
	rg.POST("/reset-password", h.ResetPassword)
	rg.POST("/verify-email", h.VerifyEmail)
	rg.GET("/invites/resolve", h.ResolveInvite)
}

// ListUsers retrieves a summary list of all users accessible to the requester.
func (h *Handler) ListUsers(c *gin.Context) {
	id := httpkit.MustGetIdentity(c)
	if id == nil {
		return
	}

	users, err := h.svc.ListUsersForRequester(c.Request.Context(), id.UserID())
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, users)
}

// GetMe retrieves the profile details of the currently authenticated user.
func (h *Handler) GetMe(c *gin.Context) {
	id := httpkit.MustGetIdentity(c)
	if id == nil {
		return
	}

	profile, err := h.svc.GetMe(c.Request.Context(), id.UserID())
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.ProfileResponse{
		ID:                  profile.ID.String(),
		Email:               profile.Email,
		EmailVerified:       profile.EmailVerified,
		FirstName:           profile.FirstName,
		LastName:            profile.LastName,
		Phone:               profile.Phone,
		PreferredLang:       profile.PreferredLang,
		Roles:               profile.Roles,
		HasOrganization:     profile.HasOrganization,
		OnboardingCompleted: profile.OnboardingCompleted,
		CreatedAt:           profile.CreatedAt,
		UpdatedAt:           profile.UpdatedAt,
	})
}

// UpdateMe modifies the profile details of the currently authenticated user.
func (h *Handler) UpdateMe(c *gin.Context) {
	id := httpkit.MustGetIdentity(c)
	if id == nil {
		return
	}

	req, ok := bindAndValidate[transport.UpdateProfileRequest](c, h.val)
	if !ok {
		return
	}

	profile, err := h.svc.UpdateMe(c.Request.Context(), id.UserID(), req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.ProfileResponse{
		ID:                  profile.ID.String(),
		Email:               profile.Email,
		EmailVerified:       profile.EmailVerified,
		FirstName:           profile.FirstName,
		LastName:            profile.LastName,
		Phone:               profile.Phone,
		PreferredLang:       profile.PreferredLang,
		Roles:               profile.Roles,
		HasOrganization:     profile.HasOrganization,
		OnboardingCompleted: profile.OnboardingCompleted,
		CreatedAt:           profile.CreatedAt,
		UpdatedAt:           profile.UpdatedAt,
	})
}

// CompleteOnboarding processes the user's initial onboarding information.
func (h *Handler) CompleteOnboarding(c *gin.Context) {
	id := httpkit.MustGetIdentity(c)
	if id == nil {
		return
	}

	req, ok := bindAndValidate[transport.CompleteOnboardingRequest](c, h.val)
	if !ok {
		return
	}

	if httpkit.HandleError(c, h.svc.CompleteOnboarding(c.Request.Context(), id.UserID(), req)) {
		return
	}

	httpkit.OK(c, gin.H{"message": "onboarding complete"})
}

// MarkOnboardingComplete flags the authenticated user's onboarding process as finished.
func (h *Handler) MarkOnboardingComplete(c *gin.Context) {
	id := httpkit.MustGetIdentity(c)
	if id == nil {
		return
	}

	if httpkit.HandleError(c, h.svc.MarkOnboardingComplete(c.Request.Context(), id.UserID())) {
		return
	}

	httpkit.OK(c, gin.H{"message": "onboarding marked complete"})
}

// ChangePassword updates the authenticated user's password.
func (h *Handler) ChangePassword(c *gin.Context) {
	id := httpkit.MustGetIdentity(c)
	if id == nil {
		return
	}

	req, ok := bindAndValidate[transport.ChangePasswordRequest](c, h.val)
	if !ok {
		return
	}

	if httpkit.HandleError(c, h.svc.ChangePassword(c.Request.Context(), id.UserID(), req.CurrentPassword, req.NewPassword)) {
		return
	}

	httpkit.OK(c, gin.H{"message": "password updated"})
}

// SignUp registers a new user in the system.
func (h *Handler) SignUp(c *gin.Context) {
	req, ok := bindAndValidate[transport.SignUpRequest](c, h.val)
	if !ok {
		return
	}

	if httpkit.HandleError(c, h.svc.SignUp(c.Request.Context(), req.Email, req.Password, req.OrganizationName, req.InviteToken)) {
		return
	}
	httpkit.JSON(c, http.StatusCreated, gin.H{"message": "account created"})
}

// SignIn authenticates a user and returns an access and refresh token.
func (h *Handler) SignIn(c *gin.Context) {
	req, ok := bindAndValidate[transport.SignInRequest](c, h.val)
	if !ok {
		return
	}

	accessToken, refreshToken, err := h.svc.SignIn(c.Request.Context(), req.Email, req.Password)
	if httpkit.HandleError(c, err) {
		return
	}

	h.setRefreshCookie(c, refreshToken)
	httpkit.OK(c, transport.AuthResponse{AccessToken: accessToken, RefreshToken: refreshToken})
}

// Refresh issues a new access token based on a valid refresh token (from body or cookie).
func (h *Handler) Refresh(c *gin.Context) {
	var refreshToken string
	var usedCookie bool

	// We only bind the JSON (don't strictly validate) because the token might be in the cookie instead
	var req transport.RefreshRequest
	if err := c.ShouldBindJSON(&req); err == nil {
		refreshToken = strings.TrimSpace(req.RefreshToken)
	}

	if refreshToken == "" {
		cookieValue, err := c.Cookie(h.cfg.GetRefreshCookieName())
		if err != nil || cookieValue == "" {
			httpkit.Error(c, http.StatusUnauthorized, "token invalid", nil)
			return
		}
		refreshToken = cookieValue
		usedCookie = true
	}

	accessToken, newRefreshToken, err := h.svc.Refresh(c.Request.Context(), refreshToken)
	if httpkit.HandleError(c, err) {
		if usedCookie {
			h.clearRefreshCookie(c)
		}
		return
	}

	if usedCookie {
		h.setRefreshCookie(c, newRefreshToken)
	}
	httpkit.OK(c, transport.AuthResponse{AccessToken: accessToken, RefreshToken: newRefreshToken})
}

// Verify checks the current session and returns basic valid user information.
func (h *Handler) Verify(c *gin.Context) {
	id := httpkit.GetIdentity(c)
	if !id.IsAuthenticated() {
		httpkit.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	profile, err := h.svc.GetMe(c.Request.Context(), id.UserID())
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.VerifyResponse{
		Valid:  true,
		UserID: profile.ID.String(),
		Email:  profile.Email,
	})
}

// SignOut revokes the user's refresh token and clears the auth cookie.
func (h *Handler) SignOut(c *gin.Context) {
	accessToken, _ := bearerTokenFromHeader(c.GetHeader(headerAuthorization))
	var refreshToken string

	var req transport.SignOutRequest
	if err := c.ShouldBindJSON(&req); err == nil {
		refreshToken = strings.TrimSpace(req.RefreshToken)
	}

	if refreshToken == "" {
		if cookieValue, err := c.Cookie(h.cfg.GetRefreshCookieName()); err == nil && cookieValue != "" {
			refreshToken = cookieValue
		}
	}

	if refreshToken != "" {
		if httpkit.HandleError(c, h.svc.SignOut(c.Request.Context(), refreshToken, accessToken)) {
			return
		}
	}

	h.clearRefreshCookie(c)
	httpkit.OK(c, gin.H{"message": "signed out"})
}

// ForgotPassword initiates the password reset workflow by dispatching an email.
func (h *Handler) ForgotPassword(c *gin.Context) {
	req, ok := bindAndValidate[transport.ForgotPasswordRequest](c, h.val)
	if !ok {
		return
	}

	if httpkit.HandleError(c, h.svc.ForgotPassword(c.Request.Context(), req.Email)) {
		return
	}
	httpkit.OK(c, gin.H{"message": "if the account exists, a reset link will be sent"})
}

// ResetPassword finalizes a password reset using the token provided via email.
func (h *Handler) ResetPassword(c *gin.Context) {
	req, ok := bindAndValidate[transport.ResetPasswordRequest](c, h.val)
	if !ok {
		return
	}

	if httpkit.HandleError(c, h.svc.ResetPassword(c.Request.Context(), req.Token, req.NewPassword)) {
		return
	}

	httpkit.OK(c, gin.H{"message": "password reset"})
}

// VerifyEmail confirms a user's email address using a verification token.
func (h *Handler) VerifyEmail(c *gin.Context) {
	req, ok := bindAndValidate[transport.VerifyEmailRequest](c, h.val)
	if !ok {
		return
	}

	if httpkit.HandleError(c, h.svc.VerifyEmail(c.Request.Context(), req.Token)) {
		return
	}

	httpkit.OK(c, gin.H{"message": "email verified"})
}

// ResolveInvite looks up an organization invite via token without accepting it yet.
func (h *Handler) ResolveInvite(c *gin.Context) {
	tokenValue := c.Query("token")
	if tokenValue == "" {
		httpkit.Error(c, http.StatusBadRequest, "token is required", nil)
		return
	}

	resp, err := h.svc.ResolveInvite(c.Request.Context(), tokenValue)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, resp)
}

// SetUserRoles allows an admin to update a specific user's assigned roles.
func (h *Handler) SetUserRoles(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}

	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	req, ok := bindAndValidate[transport.RoleUpdateRequest](c, h.val)
	if !ok {
		return
	}

	if httpkit.HandleError(c, h.svc.SetUserRoles(c.Request.Context(), identity.UserID(), identity.Roles(), userID, req.Roles)) {
		return
	}

	httpkit.OK(c, transport.RoleUpdateResponse{UserID: userID.String(), Roles: req.Roles})
}

// ---------------------------------------------------------------------------
// Internal Helpers
// ---------------------------------------------------------------------------

// bindAndValidate acts as a generic helper to decode and validate JSON requests.
func bindAndValidate[T any](c *gin.Context, val *validator.Validator) (T, bool) {
	var req T
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return req, false
	}
	if err := val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return req, false
	}
	return req, true
}

func bearerTokenFromHeader(authHeader string) (string, bool) {
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return "", false
	}

	token := strings.TrimSpace(strings.TrimPrefix(authHeader, bearerPrefix))
	if token == "" {
		return "", false
	}

	return token, true
}

func (h *Handler) setRefreshCookie(c *gin.Context, value string) {
	maxAge := int(h.cfg.GetRefreshTokenTTL() / time.Second)
	c.SetSameSite(h.cfg.GetRefreshCookieSameSite())
	c.SetCookie(
		h.cfg.GetRefreshCookieName(),
		value,
		maxAge,
		h.cfg.GetRefreshCookiePath(),
		h.cfg.GetRefreshCookieDomain(),
		h.cfg.GetRefreshCookieSecure(),
		true,
	)
}

func (h *Handler) clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(h.cfg.GetRefreshCookieSameSite())
	c.SetCookie(
		h.cfg.GetRefreshCookieName(),
		"",
		-1,
		h.cfg.GetRefreshCookiePath(),
		h.cfg.GetRefreshCookieDomain(),
		h.cfg.GetRefreshCookieSecure(),
		true,
	)
}
