// Package storage provides a domain-agnostic interface for S3-compatible object storage.
// This adapter can be reused across different domains (RAC_leads, RAC_appointments, etc.).
package storage

import (
	"context"
	"io"
	"time"
)

// PresignedURL contains the URL and metadata for a presigned upload/download operation.
type PresignedURL struct {
	URL       string    `json:"url"`
	FileKey   string    `json:"fileKey"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// StorageService defines the interface for object storage operations.
// This interface is designed to be domain-agnostic and can be used by any module.
type StorageService interface {
	// GenerateUploadURL creates a presigned URL for uploading a file.
	// The folder parameter defines the path prefix (e.g., "{org}/{lead}/{service}").
	// Returns the presigned URL, the full file key, and expiration time.
	GenerateUploadURL(ctx context.Context, bucket, folder, fileName, contentType string, sizeBytes int64) (*PresignedURL, error)

	// GenerateDownloadURL creates a presigned URL for downloading a file.
	GenerateDownloadURL(ctx context.Context, bucket, fileKey string) (*PresignedURL, error)

	// DownloadFile downloads a file directly from storage.
	// The caller is responsible for closing the returned io.ReadCloser.
	DownloadFile(ctx context.Context, bucket, fileKey string) (io.ReadCloser, error)

	// DeleteObject removes an object from storage.
	DeleteObject(ctx context.Context, bucket, fileKey string) error

	// UploadFile uploads a file directly to storage from an io.Reader.
	// Returns the full file key used for storage.
	UploadFile(ctx context.Context, bucket, folder, fileName, contentType string, reader io.Reader, size int64) (string, error)

	// EnsureBucketExists creates the bucket if it doesn't exist.
	EnsureBucketExists(ctx context.Context, bucket string) error

	// ValidateContentType checks if the content type is allowed.
	ValidateContentType(contentType string) error

	// ValidateFileSize checks if the file size is within limits.
	ValidateFileSize(sizeBytes int64) error

	// GetMaxFileSize returns the configured maximum file size in bytes.
	GetMaxFileSize() int64
}

// Config defines the configuration interface for storage.
type Config interface {
	GetMinIOEndpoint() string
	GetMinIOAccessKey() string
	GetMinIOSecretKey() string
	GetMinIOUseSSL() bool
	GetMinIOMaxFileSize() int64
	IsMinIOEnabled() bool
}
