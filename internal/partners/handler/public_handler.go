package handler

import (
	"net/http"

	"portal_final_backend/internal/partners/service"
	"portal_final_backend/internal/partners/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
)

// PublicHandler handles unauthenticated, token-based partner offer endpoints.
type PublicHandler struct {
	svc *service.Service
	val *validator.Validator
}

// NewPublicHandler creates a new public handler for partner offers.
func NewPublicHandler(svc *service.Service, val *validator.Validator) *PublicHandler {
	return &PublicHandler{svc: svc, val: val}
}

// RegisterRoutes mounts public partner offer routes (no auth middleware).
func (h *PublicHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/:token", h.GetOffer)
	rg.POST("/:token/accept", h.AcceptOffer)
	rg.POST("/:token/reject", h.RejectOffer)
}

// GetOffer returns the public-facing offer details for a vakman.
func (h *PublicHandler) GetOffer(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	resp, err := h.svc.GetPublicOffer(c.Request.Context(), token)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, resp)
}

// AcceptOffer processes a vakman's acceptance of an offer.
func (h *PublicHandler) AcceptOffer(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.AcceptOfferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	if err := h.svc.AcceptOffer(c.Request.Context(), token, req); httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"status": "accepted"})
}

// RejectOffer processes a vakman's rejection of an offer.
func (h *PublicHandler) RejectOffer(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.RejectOfferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body for rejection without reason
		req = transport.RejectOfferRequest{}
	}

	if err := h.svc.RejectOffer(c.Request.Context(), token, req); httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"status": "rejected"})
}
