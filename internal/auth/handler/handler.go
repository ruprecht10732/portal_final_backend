package handler

import (
	"log"
	"net/http"

	"portal_final_backend/internal/auth/service"
	"portal_final_backend/internal/auth/transport"
	"portal_final_backend/internal/auth/validator"
	"portal_final_backend/internal/http/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc *service.Service
}

const (
	msgInvalidRequest   = "invalid request"
	msgValidationFailed = "validation failed"
)

func New(svc *service.Service) *Handler {
	return &Handler{svc: svc}
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

	verifyToken, err := h.svc.SignUp(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	log.Printf("verification token for %s: %s", req.Email, verifyToken)
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

	response.OK(c, transport.AuthResponse{AccessToken: accessToken, RefreshToken: refreshToken})
}

func (h *Handler) Refresh(c *gin.Context) {
	var req transport.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		response.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	accessToken, refreshToken, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, err.Error(), nil)
		return
	}

	response.OK(c, transport.AuthResponse{AccessToken: accessToken, RefreshToken: refreshToken})
}

func (h *Handler) SignOut(c *gin.Context) {
	var req transport.SignOutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		response.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	if err := h.svc.SignOut(c.Request.Context(), req.RefreshToken); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

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

	resetToken, err := h.svc.ForgotPassword(c.Request.Context(), req.Email)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	if resetToken != "" {
		log.Printf("password reset token for %s: %s", req.Email, resetToken)
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
