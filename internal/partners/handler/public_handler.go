package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

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

const headerContentType = "Content-Type"
const headerCacheControl = "Cache-Control"

// NewPublicHandler creates a new public handler for partner offers.
func NewPublicHandler(svc *service.Service, val *validator.Validator) *PublicHandler {
	return &PublicHandler{svc: svc, val: val}
}

// RegisterRoutes mounts public partner offer routes (no auth middleware).
func (h *PublicHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/:token/photos/:attachmentId", h.GetOfferPhoto)
	rg.GET("/:token", h.GetOffer)
	rg.GET("/:token/terms", h.GetTerms)
	rg.GET("/:token/pdf-ready", h.GetOfferPDFReady)
	rg.GET("/:token/pdf", h.GetOfferPDF)
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

func (h *PublicHandler) GetTerms(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	resp, err := h.svc.GetPublicOfferTerms(c.Request.Context(), token)
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

func (h *PublicHandler) GetOfferPDFReady(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		httpkit.Error(c, http.StatusInternalServerError, "streaming not supported", nil)
		return
	}

	c.Header(headerContentType, "text/event-stream")
	c.Header(headerCacheControl, "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	writeSSE(c, "waiting", map[string]any{"ready": false})
	flusher.Flush()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	timeout := time.NewTimer(90 * time.Second)
	defer timeout.Stop()

	for {
		ready, err := h.svc.IsOfferPDFReady(c.Request.Context(), token)
		if err != nil {
			writeSSE(c, "error", map[string]any{"message": err.Error()})
			flusher.Flush()
			return
		}
		if ready {
			writeSSE(c, "ready", map[string]any{"ready": true})
			flusher.Flush()
			return
		}

		select {
		case <-c.Request.Context().Done():
			return
		case <-timeout.C:
			writeSSE(c, "timeout", map[string]any{"ready": false})
			flusher.Flush()
			return
		case <-ticker.C:
		}
	}
}

func (h *PublicHandler) GetOfferPDF(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	fileName, reader, err := h.svc.GetOfferPDFByToken(c.Request.Context(), token)
	if httpkit.HandleError(c, err) {
		return
	}
	defer closeQuietly(reader)

	streamDownload(c, "application/pdf", fileName, reader)
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
	c.Header(headerContentType, contentType)
	c.Header(headerCacheControl, "public, max-age=3600")
	c.Header("Content-Disposition", "inline; filename=\""+fileName+"\"")
	_, _ = io.Copy(c.Writer, reader)
}

func streamDownload(c *gin.Context, contentType string, fileName string, reader io.Reader) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header(headerContentType, contentType)
	c.Header(headerCacheControl, "no-store")
	c.Header("Content-Disposition", "attachment; filename=\""+fileName+"\"")
	_, _ = io.Copy(c.Writer, reader)
}

func writeSSE(c *gin.Context, event string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(`{"ready":false}`)
	}
	_, _ = fmt.Fprintf(c.Writer, "event: %s\n", event)
	_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", data)
}
