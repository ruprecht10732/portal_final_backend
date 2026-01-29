package handler

import (
	"errors"
	"net/http"
	"time"

	"portal_final_backend/internal/auth/service"
	"portal_final_backend/internal/auth/transport"
	"portal_final_backend/internal/auth/validator"
	"portal_final_backend/internal/config"
	"portal_final_backend/internal/http/middleware"
	"portal_final_backend/internal/http/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	svc *service.Service
	cfg *config.Config
}

const (
	msgInvalidRequest   = "invalid request"
	msgValidationFailed = "validation failed"
)

func New(svc *service.Service, cfg *config.Config) *Handler {
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

func (h *Handler) GetMe(c *gin.Context) {
	userID, ok := c.Get(middleware.ContextUserIDKey)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	profile, err := h.svc.GetMe(c.Request.Context(), userID.(uuid.UUID))
	if err != nil {
		response.Error(c, http.StatusNotFound, err.Error(), nil)
		return
	}

	response.OK(c, transport.ProfileResponse{
		ID:            profile.ID.String(),
		Email:         profile.Email,
		EmailVerified: profile.EmailVerified,
		CreatedAt:     profile.CreatedAt,
		UpdatedAt:     profile.UpdatedAt,
	})
}

func (h *Handler) UpdateMe(c *gin.Context) {
	userID, ok := c.Get(middleware.ContextUserIDKey)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	var req transport.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		response.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	profile, err := h.svc.UpdateMe(c.Request.Context(), userID.(uuid.UUID), req.Email)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrEmailTaken):
			response.Error(c, http.StatusConflict, err.Error(), nil)
		case errors.Is(err, service.ErrInvalidCredentials):
			response.Error(c, http.StatusBadRequest, err.Error(), nil)
		default:
			response.Error(c, http.StatusBadRequest, err.Error(), nil)
		}
		return
	}

	response.OK(c, transport.ProfileResponse{
		ID:            profile.ID.String(),
		Email:         profile.Email,
		EmailVerified: profile.EmailVerified,
		CreatedAt:     profile.CreatedAt,
		UpdatedAt:     profile.UpdatedAt,
	})
}

func (h *Handler) ChangePassword(c *gin.Context) {
	userID, ok := c.Get(middleware.ContextUserIDKey)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	var req transport.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		response.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	if err := h.svc.ChangePassword(c.Request.Context(), userID.(uuid.UUID), req.CurrentPassword, req.NewPassword); err != nil {
		if errors.Is(err, service.ErrInvalidCurrentPassword) {
			response.Error(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	response.OK(c, gin.H{"message": "password updated"})
}

func (h *Handler) SignUp(c *gin.Context) {
	var req transport.SignUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		response.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	if err := h.svc.SignUp(c.Request.Context(), req.Email, req.Password); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	response.JSON(c, http.StatusCreated, gin.H{"message": "account created"})
}

func (h *Handler) SignIn(c *gin.Context) {
	var req transport.SignInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		response.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	accessToken, refreshToken, err := h.svc.SignIn(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, err.Error(), nil)
		return
	}

	h.setRefreshCookie(c, refreshToken)
	response.OK(c, transport.AuthResponse{AccessToken: accessToken})
}

func (h *Handler) Refresh(c *gin.Context) {
	refreshToken, err := c.Cookie(h.cfg.RefreshCookieName)
	if err != nil || refreshToken == "" {
		response.Error(c, http.StatusUnauthorized, service.ErrTokenInvalid.Error(), nil)
		return
	}

	accessToken, newRefreshToken, err := h.svc.Refresh(c.Request.Context(), refreshToken)
	if err != nil {
		h.clearRefreshCookie(c)
		response.Error(c, http.StatusUnauthorized, err.Error(), nil)
		return
	}

	h.setRefreshCookie(c, newRefreshToken)
	response.OK(c, transport.AuthResponse{AccessToken: accessToken})
}

func (h *Handler) SignOut(c *gin.Context) {
	if refreshToken, err := c.Cookie(h.cfg.RefreshCookieName); err == nil && refreshToken != "" {
		if err := h.svc.SignOut(c.Request.Context(), refreshToken); err != nil {
			response.Error(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
	}

	h.clearRefreshCookie(c)

	response.OK(c, gin.H{"message": "signed out"})
}

func (h *Handler) ForgotPassword(c *gin.Context) {
	var req transport.ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		response.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	if err := h.svc.ForgotPassword(c.Request.Context(), req.Email); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	response.OK(c, gin.H{"message": "if the account exists, a reset link will be sent"})
}

func (h *Handler) ResetPassword(c *gin.Context) {
	var req transport.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		response.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	if err := h.svc.ResetPassword(c.Request.Context(), req.Token, req.NewPassword); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	response.OK(c, gin.H{"message": "password reset"})
}

func (h *Handler) VerifyEmail(c *gin.Context) {
	var req transport.VerifyEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		response.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	if err := h.svc.VerifyEmail(c.Request.Context(), req.Token); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	response.OK(c, gin.H{"message": "email verified"})
}

func (h *Handler) SetUserRoles(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.RoleUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		response.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	if err := h.svc.SetUserRoles(c.Request.Context(), userID, req.Roles); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	response.OK(c, transport.RoleUpdateResponse{UserID: userID.String(), Roles: req.Roles})
}

func (h *Handler) setRefreshCookie(c *gin.Context, value string) {
	maxAge := int(h.cfg.RefreshTokenTTL / time.Second)
	c.SetSameSite(h.cfg.RefreshCookieSameSite)
	c.SetCookie(
		h.cfg.RefreshCookieName,
		value,
		maxAge,
		h.cfg.RefreshCookiePath,
		h.cfg.RefreshCookieDomain,
		h.cfg.RefreshCookieSecure,
		true,
	)
}

func (h *Handler) clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(h.cfg.RefreshCookieSameSite)
	c.SetCookie(
		h.cfg.RefreshCookieName,
		"",
		-1,
		h.cfg.RefreshCookiePath,
		h.cfg.RefreshCookieDomain,
		h.cfg.RefreshCookieSecure,
		true,
	)
}
