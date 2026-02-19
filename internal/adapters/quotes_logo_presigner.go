package adapters

import (
	"context"

	"portal_final_backend/internal/adapters/storage"
	quotesvc "portal_final_backend/internal/quotes/service"
)

// QuotesLogoPresigner generates presigned download URLs for organization logos.
type QuotesLogoPresigner struct {
	storage storage.StorageService
	bucket  string
}

// NewQuotesLogoPresigner creates a new logo presigner adapter.
func NewQuotesLogoPresigner(storageSvc storage.StorageService, bucket string) *QuotesLogoPresigner {
	return &QuotesLogoPresigner{storage: storageSvc, bucket: bucket}
}

// GenerateLogoURL generates a presigned download URL for the given logo file key.
func (p *QuotesLogoPresigner) GenerateLogoURL(ctx context.Context, fileKey string) (string, error) {
	presigned, err := p.storage.GenerateDownloadURL(ctx, p.bucket, fileKey)
	if err != nil {
		return "", err
	}
	return presigned.URL, nil
}

// Compile-time check that QuotesLogoPresigner implements quotes/service.LogoPresigner.
var _ quotesvc.LogoPresigner = (*QuotesLogoPresigner)(nil)
