package handler

import (
	"context"
	"net/http"

	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/internal/quotes/service"
	"portal_final_backend/internal/quotes/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PublicHandler handles unauthenticated HTTP requests for public quote proposals.
type PublicHandler struct {
	svc *service.Service
	val *validator.Validator
	sse *sse.Service
}

// NewPublicHandler creates a new public quotes handler.
func NewPublicHandler(svc *service.Service, val *validator.Validator) *PublicHandler {
	return &PublicHandler{svc: svc, val: val}
}

// SetSSE injects the SSE service for public real-time events.
func (h *PublicHandler) SetSSE(s *sse.Service) {
	h.sse = s
}

// RegisterRoutes registers the public quote routes (no auth middleware).
func (h *PublicHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/:token", h.GetPublicQuote)
	rg.PATCH("/:token/items/:itemId/toggle", h.ToggleItem)
	rg.POST("/:token/items/:itemId/annotations", h.AnnotateItem)
	rg.POST("/:token/accept", h.Accept)
	rg.POST("/:token/reject", h.Reject)

	// Public SSE — customer page gets real-time updates
	if h.sse != nil {
		rg.GET("/:token/events", h.sse.PublicQuoteHandler(h.resolveQuoteID))
	}
}

// resolveQuoteID maps a public token to a quote UUID.
func (h *PublicHandler) resolveQuoteID(token string) (uuid.UUID, error) {
	ctx := context.Background()
	return h.svc.GetPublicQuoteID(ctx, token)
}

// GetPublicQuote handles GET /api/v1/public/quotes/:token
func (h *PublicHandler) GetPublicQuote(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		httpkit.Error(c, http.StatusBadRequest, "token is required", nil)
		return
	}

	result, err := h.svc.GetPublic(c.Request.Context(), token)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// ToggleItem handles PATCH /api/v1/public/quotes/:token/items/:itemId/toggle
func (h *PublicHandler) ToggleItem(c *gin.Context) {
	token := c.Param("token")
	itemID, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid item ID", nil)
		return
	}

	var req transport.ToggleItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	result, err := h.svc.ToggleLineItem(c.Request.Context(), token, itemID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// AnnotateItem handles POST /api/v1/public/quotes/:token/items/:itemId/annotations
func (h *PublicHandler) AnnotateItem(c *gin.Context) {
	token := c.Param("token")
	itemID, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid item ID", nil)
		return
	}

	var req transport.AnnotateItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	// For public (customer) annotations, use client IP as author ID
	authorID := c.ClientIP()
	result, err := h.svc.AnnotateItem(c.Request.Context(), token, itemID, "customer", authorID, req.Text)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, result)
}

// Accept handles POST /api/v1/public/quotes/:token/accept
func (h *PublicHandler) Accept(c *gin.Context) {
	token := c.Param("token")

	var req transport.AcceptQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	clientIP := c.ClientIP()
	result, err := h.svc.Accept(c.Request.Context(), token, req, clientIP)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// Reject handles POST /api/v1/public/quotes/:token/reject
func (h *PublicHandler) Reject(c *gin.Context) {
	token := c.Param("token")

	var req transport.RejectQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	result, err := h.svc.Reject(c.Request.Context(), token, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}
