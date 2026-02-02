package storage

import (
	"fmt"
	"strings"
)

// AllowedContentTypes defines the allowed MIME types for uploads.
var AllowedContentTypes = map[string]bool{
	// Images
	"image/jpeg":    true,
	"image/png":     true,
	"image/gif":     true,
	"image/webp":    true,
	"image/svg+xml": true,

	// Documents
	"application/pdf":                                                               true,
	"application/msword":                                                            true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":       true,
	"application/vnd.ms-excel":                                                      true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":             true,
	"application/vnd.ms-powerpoint":                                                 true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation":     true,
	"text/plain":                                                                    true,
	"text/csv":                                                                      true,

	// Video
	"video/mp4":       true,
	"video/webm":      true,
	"video/quicktime": true,
	"video/x-msvideo": true,
	"video/mpeg":      true,

	// Audio
	"audio/mpeg":  true,
	"audio/wav":   true,
	"audio/ogg":   true,
	"audio/webm":  true,
	"audio/x-wav": true,
}

// ValidateContentType checks if the content type is allowed.
func (s *MinIOService) ValidateContentType(contentType string) error {
	// Normalize content type (remove parameters like charset)
	normalized := strings.Split(contentType, ";")[0]
	normalized = strings.TrimSpace(strings.ToLower(normalized))

	if !AllowedContentTypes[normalized] {
		return fmt.Errorf("content type %q is not allowed", contentType)
	}
	return nil
}

// ValidateFileSize checks if the file size is within limits.
func (s *MinIOService) ValidateFileSize(sizeBytes int64) error {
	if sizeBytes <= 0 {
		return fmt.Errorf("file size must be greater than 0")
	}
	if sizeBytes > s.maxFileSize {
		return fmt.Errorf("file size %d bytes exceeds maximum allowed size of %d bytes", sizeBytes, s.maxFileSize)
	}
	return nil
}

// GetAllowedContentTypes returns a list of allowed content types.
// Useful for frontend validation.
func GetAllowedContentTypes() []string {
	types := make([]string, 0, len(AllowedContentTypes))
	for ct := range AllowedContentTypes {
		types = append(types, ct)
	}
	return types
}

// IsImageContentType checks if the content type is an image.
func IsImageContentType(contentType string) bool {
	return strings.HasPrefix(strings.ToLower(contentType), "image/")
}

// IsVideoContentType checks if the content type is a video.
func IsVideoContentType(contentType string) bool {
	return strings.HasPrefix(strings.ToLower(contentType), "video/")
}

// IsDocumentContentType checks if the content type is a document.
func IsDocumentContentType(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.HasPrefix(ct, "application/pdf") ||
		strings.HasPrefix(ct, "application/msword") ||
		strings.Contains(ct, "officedocument") ||
		strings.HasPrefix(ct, "text/")
}
