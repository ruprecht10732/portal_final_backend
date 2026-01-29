package handler

import (
	"net/http"
	"time"

	"portal_final_backend/internal/auth/service"
	"portal_final_backend/internal/auth/transport"
	"portal_final_backend/internal/auth/validator"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	svc *service.Service
	cfg config.CookieConfig
}

const (
	msgInvalidRequest   = "invalid request"
	msgValidationFailed = "validation failed"
)

func New(svc *service.Service, cfg config.CookieConfig) *Handler {
	return &Handler{svc: svc, cfg: cfg}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/sign-up", h.SignUp)
	rg.POST("/sign-in", h.SignIn)
	rg.POST("/refresh", h.Refresh)
	rg.POST("/sign-out", h.SignOut)
	rg.POST("/forgot-password", h.ForgotPassword)
	rg.POST("/reset-password", h.ResetPassword)
	rg.POST("/verify-email", h.VerifyEmail)
}

func (h *Handler) ListUsers(c *gin.Context) {
	users, err := h.svc.ListUsers(c.Request.Context())
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, users)
}

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
		ID:            profile.ID.String(),
		Email:         profile.Email,
		EmailVerified: profile.EmailVerified,
		Roles:         profile.Roles,
		CreatedAt:     profile.CreatedAt,
		UpdatedAt:     profile.UpdatedAt,
	})
}

func (h *Handler) UpdateMe(c *gin.Context) {
	id := httpkit.MustGetIdentity(c)
	if id == nil {
		return
	}

	var req transport.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	profile, err := h.svc.UpdateMe(c.Request.Context(), id.UserID(), req.Email)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.ProfileResponse{
		ID:            profile.ID.String(),
		Email:         profile.Email,
		EmailVerified: profile.EmailVerified,
		Roles:         profile.Roles,
		CreatedAt:     profile.CreatedAt,
		UpdatedAt:     profile.UpdatedAt,
	})
}

func (h *Handler) ChangePassword(c *gin.Context) {
	id := httpkit.MustGetIdentity(c)
	if id == nil {
		return
	}

	var req transport.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	if httpkit.HandleError(c, h.svc.ChangePassword(c.Request.Context(), id.UserID(), req.CurrentPassword, req.NewPassword)) {
		return
	}

	httpkit.OK(c, gin.H{"message": "password updated"})
}

func (h *Handler) SignUp(c *gin.Context) {
	var req transport.SignUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	if httpkit.HandleError(c, h.svc.SignUp(c.Request.Context(), req.Email, req.Password)) {
		return
	}
	httpkit.JSON(c, http.StatusCreated, gin.H{"message": "account created"})
}

func (h *Handler) SignIn(c *gin.Context) {
	var req transport.SignInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	accessToken, refreshToken, err := h.svc.SignIn(c.Request.Context(), req.Email, req.Password)
	if httpkit.HandleError(c, err) {
		return
	}

	h.setRefreshCookie(c, refreshToken)
	httpkit.OK(c, transport.AuthResponse{AccessToken: accessToken})
}

func (h *Handler) Refresh(c *gin.Context) {
	refreshToken, err := c.Cookie(h.cfg.GetRefreshCookieName())
	if err != nil || refreshToken == "" {
		httpkit.Error(c, http.StatusUnauthorized, "token invalid", nil)
		return
	}

	accessToken, newRefreshToken, err := h.svc.Refresh(c.Request.Context(), refreshToken)
	if httpkit.HandleError(c, err) {
		h.clearRefreshCookie(c)
		return
	}

	h.setRefreshCookie(c, newRefreshToken)
	httpkit.OK(c, transport.AuthResponse{AccessToken: accessToken})
}

func (h *Handler) SignOut(c *gin.Context) {
	if refreshToken, err := c.Cookie(h.cfg.GetRefreshCookieName()); err == nil && refreshToken != "" {
		if httpkit.HandleError(c, h.svc.SignOut(c.Request.Context(), refreshToken)) {
			return
		}
	}

	h.clearRefreshCookie(c)

	httpkit.OK(c, gin.H{"message": "signed out"})
}

func (h *Handler) ForgotPassword(c *gin.Context) {
	var req transport.ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	if httpkit.HandleError(c, h.svc.ForgotPassword(c.Request.Context(), req.Email)) {
		return
	}
	httpkit.OK(c, gin.H{"message": "if the account exists, a reset link will be sent"})
}

func (h *Handler) ResetPassword(c *gin.Context) {
	var req transport.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	if httpkit.HandleError(c, h.svc.ResetPassword(c.Request.Context(), req.Token, req.NewPassword)) {
		return
	}

	httpkit.OK(c, gin.H{"message": "password reset"})
}

func (h *Handler) VerifyEmail(c *gin.Context) {
	var req transport.VerifyEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	if httpkit.HandleError(c, h.svc.VerifyEmail(c.Request.Context(), req.Token)) {
		return
	}

	httpkit.OK(c, gin.H{"message": "email verified"})
}

func (h *Handler) SetUserRoles(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.RoleUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	if httpkit.HandleError(c, h.svc.SetUserRoles(c.Request.Context(), userID, req.Roles)) {
		return
	}

	httpkit.OK(c, transport.RoleUpdateResponse{UserID: userID.String(), Roles: req.Roles})
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
