package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/internal/quotes/service"
	"portal_final_backend/internal/quotes/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PDFOnDemandGenerator generates and stores a quote PDF on the fly.
// Used for lazy PDF generation when the PDF wasn't created at acceptance time.
type PDFOnDemandGenerator interface {
	RegeneratePDF(ctx context.Context, quoteID, organizationID uuid.UUID) (fileKey string, pdfBytes []byte, err error)
}

// PublicHandler handles unauthenticated HTTP requests for public quote proposals.
type PublicHandler struct {
	svc        *service.Service
	val        *validator.Validator
	sse        *sse.Service
	storageSvc storage.StorageService
	pdfBucket  string
	pdfGen     PDFOnDemandGenerator
}

// NewPublicHandler creates a new public quotes handler.
func NewPublicHandler(svc *service.Service, val *validator.Validator) *PublicHandler {
	return &PublicHandler{svc: svc, val: val}
}

// SetSSE injects the SSE service for public real-time events.
func (h *PublicHandler) SetSSE(s *sse.Service) {
	h.sse = s
}

// SetStorageForPDF injects the storage service and bucket for PDF downloads.
func (h *PublicHandler) SetStorageForPDF(svc storage.StorageService, bucket string) {
	h.storageSvc = svc
	h.pdfBucket = bucket
}

// SetPDFGenerator injects the on-demand PDF generator for lazy PDF creation.
func (h *PublicHandler) SetPDFGenerator(gen PDFOnDemandGenerator) {
	h.pdfGen = gen
}

// RegisterRoutes registers the public quote routes (no auth middleware).
func (h *PublicHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/:token", h.GetPublicQuote)
	rg.PATCH("/:token/items/:itemId/toggle", h.ToggleItem)
	rg.POST("/:token/items/:itemId/annotations", h.AnnotateItem)
	rg.POST("/:token/accept", h.Accept)
	rg.POST("/:token/reject", h.Reject)
	rg.GET("/:token/pdf", h.DownloadPDF)

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

// DownloadPDF handles GET /api/v1/public/quotes/:token/pdf
// Allows customers to download the generated PDF using the public token.
func (h *PublicHandler) DownloadPDF(c *gin.Context) {
	if h.storageSvc == nil {
		httpkit.Error(c, http.StatusServiceUnavailable, "PDF downloads are not configured", nil)
		return
	}

	token := c.Param("token")
	if token == "" {
		httpkit.Error(c, http.StatusBadRequest, "token is required", nil)
		return
	}

	result, err := h.svc.GetPublic(c.Request.Context(), token)
	if httpkit.HandleError(c, err) {
		return
	}

	// Only accepted quotes have a PDF
	if result.Status != transport.QuoteStatusAccepted {
		httpkit.Error(c, http.StatusNotFound, "PDF is alleen beschikbaar voor geaccepteerde offertes", nil)
		return
	}

	// We need to get the internal quote to access the PDF file key (not exposed in public response)
	quoteID, err := h.svc.GetPublicQuoteID(c.Request.Context(), token)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to resolve quote", nil)
		return
	}

	pdfFileKey, err := h.svc.GetPDFFileKey(c.Request.Context(), quoteID)
	if err != nil || pdfFileKey == "" {
		// Lazy generation: if no PDF is stored yet but the quote is accepted, generate on the fly
		if h.pdfGen != nil {
			orgID, orgErr := h.svc.GetOrganizationID(c.Request.Context(), quoteID)
			if orgErr != nil {
				httpkit.Error(c, http.StatusInternalServerError, "failed to resolve organization", nil)
				return
			}

			fileKey, pdfBytes, genErr := h.pdfGen.RegeneratePDF(c.Request.Context(), quoteID, orgID)
			if genErr != nil {
				httpkit.Error(c, http.StatusInternalServerError, "PDF generatie mislukt", genErr.Error())
				return
			}

			// Serve the freshly generated PDF directly
			fileName := fmt.Sprintf("Offerte-%s.pdf", result.QuoteNumber)
			c.Header("Content-Type", "application/pdf")
			c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
			c.Data(http.StatusOK, "application/pdf", pdfBytes)
			_ = fileKey // stored by the processor
			return
		}
		httpkit.Error(c, http.StatusNotFound, "no PDF available for this quote", nil)
		return
	}

	reader, err := h.storageSvc.DownloadFile(c.Request.Context(), h.pdfBucket, pdfFileKey)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to retrieve PDF", err.Error())
		return
	}
	defer reader.Close()

	fileName := fmt.Sprintf("Offerte-%s.pdf", result.QuoteNumber)
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	c.Status(http.StatusOK)

	if _, err := io.Copy(c.Writer, reader); err != nil {
		_ = c.Error(err)
	}
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
