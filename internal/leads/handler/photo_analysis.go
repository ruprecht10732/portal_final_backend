package handler

import (
	"context"
	"io"
	"log"
	"net/http"

	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/leads/agent"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PhotoAnalysisHandler handles HTTP requests for photo analysis.
type PhotoAnalysisHandler struct {
	analyzer *agent.PhotoAnalyzer
	repo     repository.LeadsRepository
	storage  storage.StorageService
	bucket   string
	sse      *sse.Service
	val      *validator.Validator
}

// NewPhotoAnalysisHandler creates a new photo analysis handler.
func NewPhotoAnalysisHandler(analyzer *agent.PhotoAnalyzer, repo repository.LeadsRepository, storageSvc storage.StorageService, bucket string, sseSvc *sse.Service, val *validator.Validator) *PhotoAnalysisHandler {
	return &PhotoAnalysisHandler{
		analyzer: analyzer,
		repo:     repo,
		storage:  storageSvc,
		bucket:   bucket,
		sse:      sseSvc,
		val:      val,
	}
}

// RegisterRoutes registers photo analysis routes.
func (h *PhotoAnalysisHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/analyze-photos", h.AnalyzePhotos)
	rg.GET("/photo-analysis", h.GetPhotoAnalysis)
	rg.GET("/photo-analysis/history", h.ListPhotoAnalyses)
}

// PhotoAnalysisRequest represents the request to analyze photos.
type PhotoAnalysisRequest struct {
	Context string `json:"context"` // Optional context about the issue
}

const (
	errTenantContextRequired = "tenant context required"
	errInvalidServiceID      = "invalid service id"
)

// AnalyzePhotos triggers AI analysis of photos for a lead service.
// This analyzes all attachments that are images for the given service.
func (h *PhotoAnalysisHandler) AnalyzePhotos(c *gin.Context) {
	identity, tenantID, ok := h.getIdentityAndTenant(c)
	if !ok {
		return
	}

	leadID, serviceID, ok := h.parseLeadServiceIDs(c)
	if !ok {
		return
	}

	contextInfo := parsePhotoAnalysisContext(c)

	imageAttachments, ok := h.loadImageAttachments(c, serviceID, *tenantID)
	if !ok {
		return
	}

	userID := identity.UserID()
	go h.runPhotoAnalysis(context.Background(), leadID, serviceID, *tenantID, &userID, imageAttachments, contextInfo)

	httpkit.OK(c, gin.H{
		"status":     "processing",
		"message":    "Photo analysis started",
		"photoCount": len(imageAttachments),
	})
}

func (h *PhotoAnalysisHandler) getIdentityAndTenant(c *gin.Context) (httpkit.Identity, *uuid.UUID, bool) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return nil, nil, false
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusForbidden, errTenantContextRequired, nil)
		return nil, nil, false
	}
	return identity, tenantID, true
}

func (h *PhotoAnalysisHandler) parseLeadServiceIDs(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	leadID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid lead id", nil)
		return uuid.UUID{}, uuid.UUID{}, false
	}
	serviceID, err := uuid.Parse(c.Param("serviceId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, errInvalidServiceID, nil)
		return uuid.UUID{}, uuid.UUID{}, false
	}
	return leadID, serviceID, true
}

func parsePhotoAnalysisContext(c *gin.Context) string {
	var req PhotoAnalysisRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return ""
	}
	return req.Context
}

func (h *PhotoAnalysisHandler) loadImageAttachments(c *gin.Context, serviceID, tenantID uuid.UUID) ([]repository.Attachment, bool) {
	attachments, err := h.repo.ListAttachmentsByService(c.Request.Context(), serviceID, tenantID)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to fetch attachments", nil)
		return nil, false
	}

	imageAttachments := filterImageAttachments(attachments)
	if len(imageAttachments) == 0 {
		httpkit.Error(c, http.StatusBadRequest, "no image attachments found for this service", nil)
		return nil, false
	}

	return imageAttachments, true
}

func filterImageAttachments(attachments []repository.Attachment) []repository.Attachment {
	imageAttachments := make([]repository.Attachment, 0, len(attachments))
	for _, att := range attachments {
		if att.ContentType != nil && isImageContentType(*att.ContentType) {
			imageAttachments = append(imageAttachments, att)
		}
	}
	return imageAttachments
}

// runPhotoAnalysis performs the photo analysis in the background and sends SSE notification when done.
func (h *PhotoAnalysisHandler) runPhotoAnalysis(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, userID *uuid.UUID, attachments []repository.Attachment, contextInfo string) {
	serviceType, intakeRequirements := h.getServiceAnalysisContext(ctx, serviceID, tenantID)
	images := h.loadImages(ctx, attachments)
	if len(images) == 0 {
		h.publishPhotoAnalysisFailure(userID, leadID, serviceID, "Failed to load images for analysis", "no_valid_images")
		return
	}

	result, err := h.analyzer.AnalyzePhotos(ctx, agent.PhotoAnalysisRequest{
		LeadID:             leadID,
		ServiceID:          serviceID,
		TenantID:           tenantID,
		Images:             images,
		ContextInfo:        contextInfo,
		ServiceType:        serviceType,
		IntakeRequirements: intakeRequirements,
	})
	if err != nil {
		h.publishPhotoAnalysisFailure(userID, leadID, serviceID, "Photo analysis failed", err.Error())
		return
	}

	result.PhotoCount = len(images)
	h.persistPhotoAnalysis(ctx, leadID, serviceID, tenantID, result)
	h.writePhotoAnalysisTimeline(ctx, leadID, serviceID, tenantID, result)
	h.publishPhotoAnalysisSuccess(userID, leadID, serviceID, result)
}

// RunAutoAnalysis triggers photo analysis without user-specific SSE notifications.
func (h *PhotoAnalysisHandler) RunAutoAnalysis(leadID, serviceID, tenantID uuid.UUID) {
	attachments, err := h.repo.ListAttachmentsByService(context.Background(), serviceID, tenantID)
	if err != nil {
		log.Printf("photo analysis: failed to load attachments for service %s: %v", serviceID, err)
		return
	}

	imageAttachments := filterImageAttachments(attachments)
	if len(imageAttachments) == 0 {
		return
	}

	go h.runPhotoAnalysis(context.Background(), leadID, serviceID, tenantID, nil, imageAttachments, "")
}

func (h *PhotoAnalysisHandler) getServiceAnalysisContext(ctx context.Context, serviceID, tenantID uuid.UUID) (string, string) {
	serviceType := ""
	intakeRequirements := ""
	svc, svcErr := h.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if svcErr != nil {
		return serviceType, intakeRequirements
	}
	serviceType = svc.ServiceType

	serviceTypes, stErr := h.repo.ListActiveServiceTypes(ctx, tenantID)
	if stErr != nil {
		return serviceType, intakeRequirements
	}
	for _, st := range serviceTypes {
		if st.Name == serviceType && st.IntakeGuidelines != nil {
			intakeRequirements = *st.IntakeGuidelines
			break
		}
	}

	return serviceType, intakeRequirements
}

func (h *PhotoAnalysisHandler) loadImages(ctx context.Context, attachments []repository.Attachment) []agent.ImageData {
	images := make([]agent.ImageData, 0, len(attachments))
	for _, att := range attachments {
		data, err := h.storage.DownloadFile(ctx, h.bucket, att.FileKey)
		if err != nil {
			continue
		}
		imgData, readErr := readAllAndClose(data)
		if readErr != nil {
			continue
		}

		mimeType := "image/jpeg"
		if att.ContentType != nil {
			mimeType = *att.ContentType
		}

		images = append(images, agent.ImageData{
			MIMEType: mimeType,
			Data:     imgData,
			Filename: att.FileName,
		})
	}

	return images
}

func readAllAndClose(data io.ReadCloser) ([]byte, error) {
	defer func() {
		_ = data.Close()
	}()
	return io.ReadAll(data)
}

func (h *PhotoAnalysisHandler) publishPhotoAnalysisFailure(userID *uuid.UUID, leadID, serviceID uuid.UUID, message, errCode string) {
	if userID == nil {
		return
	}
	h.sse.Publish(*userID, sse.Event{
		Type:      sse.EventPhotoAnalysisComplete,
		LeadID:    leadID,
		ServiceID: serviceID,
		Message:   message,
		Data:      gin.H{"success": false, "error": errCode},
	})
}

func (h *PhotoAnalysisHandler) persistPhotoAnalysis(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, result *agent.PhotoAnalysis) {
	repoMeasurements := make([]repository.Measurement, 0, len(result.Measurements))
	for _, m := range result.Measurements {
		repoMeasurements = append(repoMeasurements, repository.Measurement{
			Description: m.Description,
			Value:       m.Value,
			Unit:        m.Unit,
			Type:        m.Type,
			Confidence:  m.Confidence,
			PhotoRef:    m.PhotoRef,
		})
	}

	_, dbErr := h.repo.CreatePhotoAnalysis(ctx, repository.CreatePhotoAnalysisParams{
		LeadID:                 leadID,
		ServiceID:              serviceID,
		OrganizationID:         tenantID,
		Summary:                result.Summary,
		Observations:           result.Observations,
		ScopeAssessment:        result.ScopeAssessment,
		CostIndicators:         result.CostIndicators,
		SafetyConcerns:         result.SafetyConcerns,
		AdditionalInfo:         result.AdditionalInfo,
		ConfidenceLevel:        result.ConfidenceLevel,
		PhotoCount:             result.PhotoCount,
		Measurements:           repoMeasurements,
		NeedsOnsiteMeasurement: result.NeedsOnsiteMeasurement,
		Discrepancies:          result.Discrepancies,
		ExtractedText:          result.ExtractedText,
		SuggestedSearchTerms:   result.SuggestedSearchTerms,
	})
	if dbErr != nil {
		log.Printf("warning: failed to persist photo analysis for lead %s service %s: %v", leadID, serviceID, dbErr)
	}
}

func (h *PhotoAnalysisHandler) writePhotoAnalysisTimeline(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, result *agent.PhotoAnalysis) {
	summary := result.Summary
	if len(result.Observations) > 0 && summary == "" {
		summary = result.Observations[0]
	}

	metadata := buildPhotoAnalysisMetadata(result)
	_, _ = h.repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &serviceID,
		OrganizationID: tenantID,
		ActorType:      "AI",
		ActorName:      "Foto-analyse Agent",
		EventType:      "photo_analysis_completed",
		Title:          "Foto-analyse voltooid",
		Summary:        &summary,
		Metadata:       metadata,
	})
}

func buildPhotoAnalysisMetadata(result *agent.PhotoAnalysis) map[string]any {
	metadata := map[string]any{
		"photoCount":      result.PhotoCount,
		"scopeAssessment": result.ScopeAssessment,
		"confidenceLevel": result.ConfidenceLevel,
		"observations":    result.Observations,
		"costIndicators":  result.CostIndicators,
	}
	if len(result.SafetyConcerns) > 0 {
		metadata["safetyConcerns"] = result.SafetyConcerns
	}
	if len(result.AdditionalInfo) > 0 {
		metadata["additionalInfo"] = result.AdditionalInfo
	}
	if len(result.Measurements) > 0 {
		metadata["measurements"] = result.Measurements
	}
	if len(result.NeedsOnsiteMeasurement) > 0 {
		metadata["needsOnsiteMeasurement"] = result.NeedsOnsiteMeasurement
	}
	if len(result.Discrepancies) > 0 {
		metadata["discrepancies"] = result.Discrepancies
	}
	if len(result.ExtractedText) > 0 {
		metadata["extractedText"] = result.ExtractedText
	}
	if len(result.SuggestedSearchTerms) > 0 {
		metadata["suggestedSearchTerms"] = result.SuggestedSearchTerms
	}

	return metadata
}

func (h *PhotoAnalysisHandler) publishPhotoAnalysisSuccess(userID *uuid.UUID, leadID, serviceID uuid.UUID, result *agent.PhotoAnalysis) {
	if userID == nil {
		return
	}
	h.sse.Publish(*userID, sse.Event{
		Type:      sse.EventPhotoAnalysisComplete,
		LeadID:    leadID,
		ServiceID: serviceID,
		Message:   "Foto-analyse voltooid",
		Data: gin.H{
			"success":  true,
			"analysis": result,
		},
	})
}

// GetPhotoAnalysis retrieves the latest photo analysis for a service.
func (h *PhotoAnalysisHandler) GetPhotoAnalysis(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusForbidden, errTenantContextRequired, nil)
		return
	}

	serviceID, err := uuid.Parse(c.Param("serviceId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, errInvalidServiceID, nil)
		return
	}

	analysis, err := h.repo.GetLatestPhotoAnalysis(c.Request.Context(), serviceID, *tenantID)
	if err == repository.ErrPhotoAnalysisNotFound {
		httpkit.OK(c, gin.H{"analysis": nil})
		return
	}
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to fetch photo analysis", nil)
		return
	}

	httpkit.OK(c, gin.H{"analysis": analysis})
}

// ListPhotoAnalyses retrieves all photo analyses for a service.
func (h *PhotoAnalysisHandler) ListPhotoAnalyses(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusForbidden, errTenantContextRequired, nil)
		return
	}

	serviceID, err := uuid.Parse(c.Param("serviceId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, errInvalidServiceID, nil)
		return
	}

	analyses, err := h.repo.ListPhotoAnalysesByService(c.Request.Context(), serviceID, *tenantID)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to fetch photo analyses", nil)
		return
	}

	httpkit.OK(c, gin.H{"items": analyses})
}

// isImageContentType checks if a content type is an image type supported by Kimi
func isImageContentType(contentType string) bool {
	switch contentType {
	case "image/jpeg", "image/jpg", "image/png", "image/webp", "image/gif":
		return true
	default:
		return false
	}
}
