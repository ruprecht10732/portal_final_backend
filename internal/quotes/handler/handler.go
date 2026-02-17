package handler

import (
	"fmt"
	"net/http"
	"strings"

	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/quotes/service"
	"portal_final_backend/internal/quotes/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	msgInvalidRequest      = "invalid request"
	msgValidationFailed    = "validation failed"
	msgPDFGenerationFailed = "PDF generation failed"
)

// Handler handles HTTP requests for quotes
type Handler struct {
	svc              *service.Service
	val              *validator.Validator
	storageSvc       storage.StorageService
	pdfBucket        string
	attachmentBucket string
	catalogBucket    string
	pdfGen           PDFOnDemandGenerator
}

// New creates a new quotes handler
func New(svc *service.Service, val *validator.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

// SetStorageForPDF injects the storage service and bucket for PDF downloads.
func (h *Handler) SetStorageForPDF(svc storage.StorageService, bucket string) {
	h.storageSvc = svc
	h.pdfBucket = bucket
}

// SetAttachmentBucket injects the bucket name for manual quote attachments.
func (h *Handler) SetAttachmentBucket(bucket string) {
	h.attachmentBucket = bucket
}

// SetCatalogBucket injects the bucket name for catalog asset downloads.
func (h *Handler) SetCatalogBucket(bucket string) {
	h.catalogBucket = bucket
}

// SetPDFGenerator injects the on-demand PDF generator for lazy PDF creation.
func (h *Handler) SetPDFGenerator(gen PDFOnDemandGenerator) {
	h.pdfGen = gen
}

// RegisterRoutes registers the quote routes
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.List)
	rg.GET("/integrations/moneybird/authorize-url", h.GetMoneybirdAuthorizeURL)
	rg.GET("/integrations/:provider/status", h.GetProviderIntegrationStatus)
	rg.GET("/pending-approval", h.ListPendingApprovals)
	rg.POST("", h.Create)
	rg.POST("/calculate", h.PreviewCalculation)
	rg.POST("/generate", h.Generate)
	rg.GET("/generate-jobs/:id", h.GetGenerateJob)
	rg.POST("/export/:provider/bulk", h.BulkExportToProvider)
	rg.DELETE("/integrations/:provider", h.DisconnectProvider)
	rg.GET("/:id", h.GetByID)
	rg.GET("/:id/export/:provider/status", h.GetQuoteExportStatus)
	rg.POST("/:id/export/:provider", h.ExportToProvider)
	rg.PUT("/:id", h.Update)
	rg.PATCH("/:id/status", h.UpdateStatus)
	rg.POST("/:id/send", h.Send)
	rg.GET("/:id/preview-link", h.GetPreviewLink)
	rg.POST("/:id/items/:itemId/annotations", h.AgentAnnotate)
	rg.GET("/:id/activities", h.ListActivities)
	rg.GET("/:id/pdf", h.DownloadPDF)
	rg.POST("/:id/attachments/presign", h.PresignAttachmentUpload)
	rg.GET("/:id/attachments/:attachmentId/download", h.GetAttachmentDownloadURL)
	rg.DELETE("/:id", h.Delete)
}

func (h *Handler) RegisterPublicRoutes(rg *gin.RouterGroup) {
	rg.GET("/integrations/moneybird/callback", h.HandleMoneybirdOAuthCallback)
}

func (h *Handler) GetMoneybirdAuthorizeURL(c *gin.Context) {
	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	result, err := h.svc.GetMoneybirdAuthorizeURL(c.Request.Context(), tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) HandleMoneybirdOAuthCallback(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	state := strings.TrimSpace(c.Query("state"))
	if code == "" || state == "" {
		c.Redirect(http.StatusFound, h.svc.MoneybirdIntegrationRedirectURL("error"))
		return
	}

	if _, _, err := h.svc.HandleMoneybirdOAuthCallback(c.Request.Context(), code, state); err != nil {
		c.Redirect(http.StatusFound, h.svc.MoneybirdIntegrationRedirectURL("error"))
		return
	}

	c.Redirect(http.StatusFound, h.svc.MoneybirdIntegrationRedirectURL("connected"))
}

func (h *Handler) DisconnectProvider(c *gin.Context) {
	provider := c.Param("provider")

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	if err := h.svc.DisconnectProvider(c.Request.Context(), tenantID, provider); httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"status": "disconnected"})
}

func (h *Handler) GetProviderIntegrationStatus(c *gin.Context) {
	provider := c.Param("provider")

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	result, err := h.svc.GetProviderIntegrationStatus(c.Request.Context(), tenantID, provider)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) GetQuoteExportStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	provider := c.Param("provider")

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	result, err := h.svc.GetQuoteExportStatus(c.Request.Context(), id, tenantID, provider)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) ExportToProvider(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	provider := c.Param("provider")

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	result, err := h.svc.ExportQuoteToProvider(c.Request.Context(), id, tenantID, provider)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) BulkExportToProvider(c *gin.Context) {
	provider := c.Param("provider")

	var req transport.BulkQuoteExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	result, err := h.svc.BulkExportQuotesToProvider(c.Request.Context(), req.QuoteIDs, tenantID, provider)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// ListPendingApprovals handles GET /api/v1/quotes/pending-approval
func (h *Handler) ListPendingApprovals(c *gin.Context) {
	var req transport.ListPendingApprovalsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, err.Error())
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	result, err := h.svc.ListPendingApprovals(c.Request.Context(), tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// List handles GET /api/v1/quotes
func (h *Handler) List(c *gin.Context) {
	var req transport.ListQuotesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, err.Error())
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	result, err := h.svc.List(c.Request.Context(), tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// Create handles POST /api/v1/quotes
func (h *Handler) Create(c *gin.Context) {
	var req transport.CreateQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	identity := httpkit.MustGetIdentity(c)
	result, err := h.svc.Create(c.Request.Context(), tenantID, identity.UserID(), req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, result)
}

// Generate handles POST /api/v1/quotes/generate
// Generates a draft quote from a user prompt using the AI agent pipeline.
func (h *Handler) Generate(c *gin.Context) {
	var req transport.GenerateQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	// LeadServiceID is required for generation
	if req.LeadServiceID == nil {
		httpkit.Error(c, http.StatusBadRequest, "leadServiceId is required for quote generation", nil)
		return
	}

	identity := httpkit.MustGetIdentity(c)
	jobID, err := h.svc.StartGenerateQuoteJob(c.Request.Context(), tenantID, identity.UserID(), req.LeadID, *req.LeadServiceID, req.Prompt, req.QuoteID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusAccepted, transport.GenerateQuoteAcceptedResponse{
		JobID:  jobID,
		Status: "pending",
	})
}

// GetGenerateJob handles GET /api/v1/quotes/generate-jobs/:id
// Returns current progress/status for an async quote generation job.
func (h *Handler) GetGenerateJob(c *gin.Context) {
	jobID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	identity := httpkit.MustGetIdentity(c)
	job, err := h.svc.GetGenerateQuoteJob(c.Request.Context(), tenantID, identity.UserID(), jobID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, transport.GenerateQuoteJobResponse{
		JobID:           job.JobID,
		Status:          string(job.Status),
		Step:            job.Step,
		ProgressPercent: job.ProgressPercent,
		Error:           job.Error,
		QuoteID:         job.QuoteID,
		QuoteNumber:     job.QuoteNumber,
		ItemCount:       job.ItemCount,
		LeadID:          job.LeadID,
		LeadServiceID:   job.LeadServiceID,
		StartedAt:       job.StartedAt,
		UpdatedAt:       job.UpdatedAt,
		FinishedAt:      job.FinishedAt,
	})
}

// GetByID handles GET /api/v1/quotes/:id
func (h *Handler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	result, err := h.svc.GetByID(c.Request.Context(), id, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// Update handles PUT /api/v1/quotes/:id
func (h *Handler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.UpdateQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	result, err := h.svc.Update(c.Request.Context(), id, tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// UpdateStatus handles PATCH /api/v1/quotes/:id/status
func (h *Handler) UpdateStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.UpdateQuoteStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	identity := httpkit.MustGetIdentity(c)
	result, err := h.svc.UpdateStatus(c.Request.Context(), id, tenantID, identity.UserID(), req.Status)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// Delete handles DELETE /api/v1/quotes/:id
func (h *Handler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	identity := httpkit.MustGetIdentity(c)

	if err := h.svc.Delete(c.Request.Context(), id, tenantID, identity.UserID()); httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"message": "quote deleted"})
}

// PreviewCalculation handles POST /api/v1/quotes/calculate
// Returns calculated totals without persisting anything.
func (h *Handler) PreviewCalculation(c *gin.Context) {
	var req transport.QuoteCalculationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	result := service.CalculateQuote(req)
	httpkit.OK(c, result)
}

// Send handles POST /api/v1/quotes/:id/send
// Generates a public token and transitions the quote to "Sent" status.
func (h *Handler) Send(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	identity := httpkit.MustGetIdentity(c)
	result, err := h.svc.Send(c.Request.Context(), id, tenantID, identity.UserID())
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// GetPreviewLink handles GET /api/v1/quotes/:id/preview-link
// Returns a read-only preview token for internal agent preview.
func (h *Handler) GetPreviewLink(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	result, err := h.svc.GetPreviewLink(c.Request.Context(), id, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// AgentAnnotate handles POST /api/v1/quotes/:id/items/:itemId/annotations
// Allows an authenticated agent to add an annotation to a quote item.
func (h *Handler) AgentAnnotate(c *gin.Context) {
	quoteID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

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

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	identity := httpkit.MustGetIdentity(c)
	result, err := h.svc.AgentAnnotateItem(c.Request.Context(), quoteID, itemID, tenantID, identity.UserID(), req.Text)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, result)
}

// ListActivities handles GET /api/v1/quotes/:id/activities
func (h *Handler) ListActivities(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	activities, err := h.svc.ListActivities(c.Request.Context(), id, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, activities)
}

// DownloadPDF handles GET /api/v1/quotes/:id/pdf
// Streams the generated PDF directly from object storage.
func (h *Handler) DownloadPDF(c *gin.Context) {
	if h.storageSvc == nil {
		httpkit.Error(c, http.StatusServiceUnavailable, "PDF downloads are not configured", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	result, err := h.svc.GetByID(c.Request.Context(), id, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	if result.PDFFileKey == nil || *result.PDFFileKey == "" {
		// Lazy generation: if no PDF is stored yet, generate on the fly
		if h.pdfGen != nil {
			_, pdfBytes, genErr := h.pdfGen.RegeneratePDF(c.Request.Context(), id, tenantID)
			if genErr != nil {
				httpkit.Error(c, http.StatusInternalServerError, msgPDFGenerationFailed, genErr.Error())
				return
			}
			servePDFBytes(c, result.QuoteNumber, pdfBytes)
			return
		}
		httpkit.Error(c, http.StatusNotFound, "no PDF available for this quote", nil)
		return
	}

	reader, err := h.storageSvc.DownloadFile(c.Request.Context(), h.pdfBucket, *result.PDFFileKey)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to retrieve PDF", err.Error())
		return
	}
	streamPDFFromReader(c, result.QuoteNumber, reader)
}

// PresignAttachmentUpload handles POST /api/v1/quotes/:id/attachments/presign
// Generates a presigned URL for uploading a manual PDF attachment.
func (h *Handler) PresignAttachmentUpload(c *gin.Context) {
	if h.storageSvc == nil || h.attachmentBucket == "" {
		httpkit.Error(c, http.StatusServiceUnavailable, "attachment uploads are not configured", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	// Verify the quote exists and belongs to this tenant
	if _, err := h.svc.GetByID(c.Request.Context(), id, tenantID); err != nil {
		if httpkit.HandleError(c, err) {
			return
		}
		return
	}

	var req transport.PresignAttachmentUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	// Validate via storage service
	if err := h.storageSvc.ValidateContentType(req.ContentType); err != nil {
		httpkit.Error(c, http.StatusBadRequest, "file type not allowed", nil)
		return
	}
	if err := h.storageSvc.ValidateFileSize(req.SizeBytes); err != nil {
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	// Folder path: {org_id}/quotes/{quote_id}
	folder := fmt.Sprintf("%s/quotes/%s", tenantID.String(), id.String())

	presigned, err := h.storageSvc.GenerateUploadURL(c.Request.Context(), h.attachmentBucket, folder, req.FileName, req.ContentType, req.SizeBytes)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to generate upload URL", nil)
		return
	}

	httpkit.OK(c, transport.PresignedUploadResponse{
		UploadURL: presigned.URL,
		FileKey:   presigned.FileKey,
		ExpiresAt: presigned.ExpiresAt.Unix(),
	})
}

// GetAttachmentDownloadURL handles GET /api/v1/quotes/:id/attachments/:attachmentId/download
// Returns a presigned download URL for the attachment, resolving the bucket by source.
func (h *Handler) GetAttachmentDownloadURL(c *gin.Context) {
	if h.storageSvc == nil {
		httpkit.Error(c, http.StatusServiceUnavailable, "storage is not configured", nil)
		return
	}

	quoteID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	attachmentID, err := uuid.Parse(c.Param("attachmentId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	tenantID, ok := mustGetTenantID(c)
	if !ok {
		return
	}

	att, err := h.svc.GetAttachmentByID(c.Request.Context(), attachmentID, quoteID, tenantID)
	if err != nil {
		if httpkit.HandleError(c, err) {
			return
		}
		return
	}

	bucket := h.attachmentBucket
	if att.Source == "catalog" {
		bucket = h.catalogBucket
	}

	if bucket == "" {
		httpkit.Error(c, http.StatusServiceUnavailable, "storage bucket not configured for source: "+att.Source, nil)
		return
	}

	presigned, err := h.storageSvc.GenerateDownloadURL(c.Request.Context(), bucket, att.FileKey)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to generate download URL", nil)
		return
	}

	httpkit.OK(c, transport.PresignedDownloadResponse{
		DownloadURL: presigned.URL,
		ExpiresAt:   presigned.ExpiresAt.Unix(),
	})
}

// mustGetTenantID extracts the tenant ID from identity.
func mustGetTenantID(c *gin.Context) (uuid.UUID, bool) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return uuid.UUID{}, false
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, "tenant ID is required", nil)
		return uuid.UUID{}, false
	}
	return *tenantID, true
}
