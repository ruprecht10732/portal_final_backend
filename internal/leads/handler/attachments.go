package handler

import (
	"fmt"
	"net/http"

	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AttachmentsHandler handles HTTP requests for lead service attachments.
type AttachmentsHandler struct {
	repo    repository.AttachmentStore
	storage storage.StorageService
	bucket  string
	val     *validator.Validator
}

// NewAttachmentsHandler creates a new attachments handler.
func NewAttachmentsHandler(repo repository.AttachmentStore, storageSvc storage.StorageService, bucket string, val *validator.Validator) *AttachmentsHandler {
	return &AttachmentsHandler{repo: repo, storage: storageSvc, bucket: bucket, val: val}
}

// RegisterRoutes adds attachment routes to a service-specific router group.
// Expected route: /leads/:id/services/:serviceId/attachments
func (h *AttachmentsHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/presign", h.GetPresignedUploadURL)
	rg.POST("", h.CreateAttachment)
	rg.GET("", h.ListAttachments)
	rg.GET("/:attachmentId", h.GetAttachment)
	rg.GET("/:attachmentId/download", h.GetDownloadURL)
	rg.DELETE("/:attachmentId", h.DeleteAttachment)
}

// GetPresignedUploadURL generates a presigned URL for uploading a file to MinIO.
func (h *AttachmentsHandler) GetPresignedUploadURL(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	leadID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	serviceID, err := uuid.Parse(c.Param("serviceId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.PresignedUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	// Validate content type
	if err := h.storage.ValidateContentType(req.ContentType); err != nil {
		httpkit.Error(c, http.StatusBadRequest, "file type not allowed", nil)
		return
	}

	// Validate file size
	if err := h.storage.ValidateFileSize(req.SizeBytes); err != nil {
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	// Build folder path: {org_id}/{lead_id}/{service_id}
	folder := fmt.Sprintf("%s/%s/%s", tenantID.String(), leadID.String(), serviceID.String())

	// Generate presigned URL
	presigned, err := h.storage.GenerateUploadURL(c.Request.Context(), h.bucket, folder, req.FileName, req.ContentType, req.SizeBytes)
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

// CreateAttachment records a file attachment after successful upload to MinIO.
func (h *AttachmentsHandler) CreateAttachment(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	serviceID, err := uuid.Parse(c.Param("serviceId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.CreateAttachmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	att, err := h.repo.CreateAttachment(c.Request.Context(), repository.CreateAttachmentParams{
		LeadServiceID:  serviceID,
		OrganizationID: tenantID,
		FileKey:        req.FileKey,
		FileName:       req.FileName,
		ContentType:    req.ContentType,
		SizeBytes:      req.SizeBytes,
		UploadedBy:     identity.UserID(),
	})
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to create attachment record", nil)
		return
	}

	httpkit.JSON(c, http.StatusCreated, toAttachmentResponse(att, nil))
}

// ListAttachments returns all attachments for a lead service.
func (h *AttachmentsHandler) ListAttachments(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	serviceID, err := uuid.Parse(c.Param("serviceId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	attachments, err := h.repo.ListAttachmentsByService(c.Request.Context(), serviceID, tenantID)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to list attachments", nil)
		return
	}

	items := make([]transport.AttachmentResponse, len(attachments))
	for i, att := range attachments {
		items[i] = toAttachmentResponse(att, nil)
	}

	httpkit.OK(c, transport.AttachmentListResponse{Items: items})
}

// GetAttachment returns a single attachment by ID.
func (h *AttachmentsHandler) GetAttachment(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	attachmentID, err := uuid.Parse(c.Param("attachmentId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	att, err := h.repo.GetAttachmentByID(c.Request.Context(), attachmentID, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, toAttachmentResponse(att, nil))
}

// GetDownloadURL generates a presigned URL for downloading a file.
func (h *AttachmentsHandler) GetDownloadURL(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	attachmentID, err := uuid.Parse(c.Param("attachmentId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	att, err := h.repo.GetAttachmentByID(c.Request.Context(), attachmentID, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	presigned, err := h.storage.GenerateDownloadURL(c.Request.Context(), h.bucket, att.FileKey)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to generate download URL", nil)
		return
	}

	httpkit.OK(c, transport.PresignedDownloadResponse{
		DownloadURL: presigned.URL,
		ExpiresAt:   presigned.ExpiresAt.Unix(),
	})
}

// DeleteAttachment removes an attachment record and the file from MinIO.
func (h *AttachmentsHandler) DeleteAttachment(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	attachmentID, err := uuid.Parse(c.Param("attachmentId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	// Get attachment to find file key for deletion
	att, err := h.repo.GetAttachmentByID(c.Request.Context(), attachmentID, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	// Delete from MinIO
	if err := h.storage.DeleteObject(c.Request.Context(), h.bucket, att.FileKey); err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to delete file from storage", nil)
		return
	}

	// Delete record from database
	if err := h.repo.DeleteAttachment(c.Request.Context(), attachmentID, tenantID); err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to delete attachment record", nil)
		return
	}

	httpkit.OK(c, gin.H{"message": "attachment deleted"})
}

// toAttachmentResponse converts a repository attachment to a transport response.
func toAttachmentResponse(att repository.Attachment, downloadURL *string) transport.AttachmentResponse {
	var contentType string
	if att.ContentType != nil {
		contentType = *att.ContentType
	}
	var sizeBytes int64
	if att.SizeBytes != nil {
		sizeBytes = *att.SizeBytes
	}

	return transport.AttachmentResponse{
		ID:          att.ID,
		FileKey:     att.FileKey,
		FileName:    att.FileName,
		ContentType: contentType,
		SizeBytes:   sizeBytes,
		UploadedBy:  att.UploadedBy,
		CreatedAt:   att.CreatedAt,
		DownloadURL: downloadURL,
	}
}
