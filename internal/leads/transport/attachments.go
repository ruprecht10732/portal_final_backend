package transport

import (
	"time"

	"github.com/google/uuid"
)

// PresignedUploadRequest is the request for generating a presigned upload URL.
type PresignedUploadRequest struct {
	FileName    string `json:"fileName" validate:"required,min=1,max=255"`
	ContentType string `json:"contentType" validate:"required,min=1,max=100"`
	SizeBytes   int64  `json:"sizeBytes" validate:"required,min=1"`
}

// PresignedUploadResponse is the response containing the presigned URL for uploading.
type PresignedUploadResponse struct {
	UploadURL string `json:"uploadUrl"`
	FileKey   string `json:"fileKey"`
	ExpiresAt int64  `json:"expiresAt"` // Unix timestamp
}

// CreateAttachmentRequest creates an attachment record after successful upload.
type CreateAttachmentRequest struct {
	FileKey     string `json:"fileKey" validate:"required,min=1,max=500"`
	FileName    string `json:"fileName" validate:"required,min=1,max=255"`
	ContentType string `json:"contentType" validate:"required,min=1,max=100"`
	SizeBytes   int64  `json:"sizeBytes" validate:"required,min=1"`
}

// AttachmentResponse is the response DTO for an attachment.
type AttachmentResponse struct {
	ID          uuid.UUID  `json:"id"`
	FileKey     string     `json:"fileKey"`
	FileName    string     `json:"fileName"`
	ContentType string     `json:"contentType"`
	SizeBytes   int64      `json:"sizeBytes"`
	UploadedBy  *uuid.UUID `json:"uploadedBy,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	DownloadURL *string    `json:"downloadUrl,omitempty"` // Presigned download URL when requested
}

// AttachmentListResponse is the list of attachments for a service.
type AttachmentListResponse struct {
	Items []AttachmentResponse `json:"items"`
}

// PresignedDownloadResponse is the response containing the presigned URL for downloading.
type PresignedDownloadResponse struct {
	DownloadURL string `json:"downloadUrl"`
	ExpiresAt   int64  `json:"expiresAt"` // Unix timestamp
}
