package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	// PresignedURLTTL is the default expiration time for presigned URLs (15 minutes).
	PresignedURLTTL = 15 * time.Minute
)

// MinIOService implements StorageService using MinIO.
type MinIOService struct {
	client      *minio.Client
	maxFileSize int64
}

// NewMinIOService creates a new MinIO storage service.
func NewMinIOService(cfg Config) (*MinIOService, error) {
	if !cfg.IsMinIOEnabled() {
		return nil, fmt.Errorf("MinIO is not configured")
	}

	client, err := minio.New(cfg.GetMinIOEndpoint(), &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.GetMinIOAccessKey(), cfg.GetMinIOSecretKey(), ""),
		Secure: cfg.GetMinIOUseSSL(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	return &MinIOService{
		client:      client,
		maxFileSize: cfg.GetMinIOMaxFileSize(),
	}, nil
}

// EnsureBucketExists creates the bucket if it doesn't exist.
func (s *MinIOService) EnsureBucketExists(ctx context.Context, bucket string) error {
	exists, err := s.client.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}

	if !exists {
		err = s.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
		if err != nil {
			return fmt.Errorf("failed to create bucket %s: %w", bucket, err)
		}
	}

	return nil
}

// GenerateUploadURL creates a presigned URL for uploading a file.
func (s *MinIOService) GenerateUploadURL(ctx context.Context, bucket, folder, fileName, contentType string, sizeBytes int64) (*PresignedURL, error) {
	// Validate inputs
	if err := s.ValidateContentType(contentType); err != nil {
		return nil, err
	}
	if err := s.ValidateFileSize(sizeBytes); err != nil {
		return nil, err
	}

	// Generate unique file key with UUID to prevent overwrites
	ext := path.Ext(fileName)
	baseName := strings.TrimSuffix(fileName, ext)
	uniqueFileName := fmt.Sprintf("%s_%s%s", baseName, uuid.New().String()[:8], ext)
	fileKey := filepath.ToSlash(filepath.Join(folder, uniqueFileName))

	// Generate presigned PUT URL
	expiresAt := time.Now().Add(PresignedURLTTL)
	presignedURL, err := s.client.PresignedPutObject(ctx, bucket, fileKey, PresignedURLTTL)
	if err != nil {
		return nil, fmt.Errorf("failed to generate presigned upload URL: %w", err)
	}

	return &PresignedURL{
		URL:       presignedURL.String(),
		FileKey:   fileKey,
		ExpiresAt: expiresAt,
	}, nil
}

// GenerateDownloadURL creates a presigned URL for downloading a file.
func (s *MinIOService) GenerateDownloadURL(ctx context.Context, bucket, fileKey string) (*PresignedURL, error) {
	expiresAt := time.Now().Add(PresignedURLTTL)

	// Set request parameters for download
	reqParams := make(url.Values)

	presignedURL, err := s.client.PresignedGetObject(ctx, bucket, fileKey, PresignedURLTTL, reqParams)
	if err != nil {
		return nil, fmt.Errorf("failed to generate presigned download URL: %w", err)
	}

	return &PresignedURL{
		URL:       presignedURL.String(),
		FileKey:   fileKey,
		ExpiresAt: expiresAt,
	}, nil
}

// DownloadFile downloads a file directly from storage.
// The caller is responsible for closing the returned io.ReadCloser.
func (s *MinIOService) DownloadFile(ctx context.Context, bucket, fileKey string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, bucket, fileKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object %s: %w", fileKey, err)
	}
	return obj, nil
}

// DeleteObject removes an object from storage.
func (s *MinIOService) DeleteObject(ctx context.Context, bucket, fileKey string) error {
	err := s.client.RemoveObject(ctx, bucket, fileKey, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object %s: %w", fileKey, err)
	}
	return nil
}

// UploadFile uploads a file directly to storage from an io.Reader and returns the file key.
func (s *MinIOService) UploadFile(ctx context.Context, bucket, folder, fileName, contentType string, reader io.Reader, size int64) (string, error) {
	ext := path.Ext(fileName)
	baseName := strings.TrimSuffix(fileName, ext)
	uniqueFileName := fmt.Sprintf("%s_%s%s", baseName, uuid.New().String()[:8], ext)
	fileKey := filepath.ToSlash(filepath.Join(folder, uniqueFileName))

	_, err := s.client.PutObject(ctx, bucket, fileKey, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload file %s: %w", fileKey, err)
	}
	return fileKey, nil
}

// GetMaxFileSize returns the configured maximum file size in bytes.
func (s *MinIOService) GetMaxFileSize() int64 {
	return s.maxFileSize
}
