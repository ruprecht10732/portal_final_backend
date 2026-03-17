package handler

import (
	"io"
	"net/http"

	"portal_final_backend/internal/partners/service"
	"portal_final_backend/internal/partners/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	rg.GET("/:token/photos/:attachmentId", h.GetOfferPhoto)
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

func (h *PublicHandler) GetOfferPhoto(c *gin.Context) {
	token := c.Param("token")
	attachmentID, err := uuid.Parse(c.Param("attachmentId"))
	if token == "" || err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	attachment, reader, err := h.svc.GetOfferPhotoByToken(c.Request.Context(), token, attachmentID)
	if httpkit.HandleError(c, err) {
		return
	}
	defer closeQuietly(reader)

	streamAttachment(c, attachment.ContentType, attachment.FileName, reader)
}

func closeQuietly(reader io.ReadCloser) {
	if reader != nil {
		_ = reader.Close()
	}
}

func streamAttachment(c *gin.Context, contentType string, fileName string, reader io.Reader) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "public, max-age=3600")
	c.Header("Content-Disposition", "inline; filename=\""+fileName+"\"")
	_, _ = io.Copy(c.Writer, reader)
}
