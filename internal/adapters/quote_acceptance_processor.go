package adapters

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"portal_final_backend/internal/adapters/storage"
	identityrepo "portal_final_backend/internal/identity/repository"
	"portal_final_backend/internal/notification"
	"portal_final_backend/internal/pdf"
	"portal_final_backend/internal/quotes/repository"
	"portal_final_backend/internal/quotes/service"
	"portal_final_backend/internal/quotes/transport"

	"github.com/google/uuid"
)

// QuoteDataReader is the narrow interface the acceptance processor uses
// to read quote data and persist the PDF file key.
type QuoteDataReader interface {
	GetByID(ctx context.Context, id uuid.UUID, orgID uuid.UUID) (*repository.Quote, error)
	GetItemsByQuoteID(ctx context.Context, quoteID uuid.UUID, orgID uuid.UUID) ([]repository.QuoteItem, error)
	SetPDFFileKey(ctx context.Context, quoteID uuid.UUID, fileKey string) error
}

// QuotePDFBucketConfig is the narrow config interface for the PDF bucket name.
type QuotePDFBucketConfig interface {
	GetMinioBucketQuotePDFs() string
	GetMinioBucketOrganizationLogos() string
}

// QuoteOrgReader is the narrow interface for fetching organization profile data.
type QuoteOrgReader interface {
	GetOrganization(ctx context.Context, organizationID uuid.UUID) (identityrepo.Organization, error)
}

// QuoteAcceptanceProcessor implements notification.QuoteAcceptanceProcessor.
// It generates the quote PDF, uploads it to MinIO, and persists the file key.
type QuoteAcceptanceProcessor struct {
	repo       QuoteDataReader
	orgReader  QuoteOrgReader
	storage    storage.StorageService
	cfg        QuotePDFBucketConfig
}

// NewQuoteAcceptanceProcessor creates a new processor adapter.
func NewQuoteAcceptanceProcessor(repo QuoteDataReader, orgReader QuoteOrgReader, storageSvc storage.StorageService, cfg QuotePDFBucketConfig) *QuoteAcceptanceProcessor {
	return &QuoteAcceptanceProcessor{
		repo:      repo,
		orgReader: orgReader,
		storage:   storageSvc,
		cfg:       cfg,
	}
}

// GenerateAndStorePDF builds the quote PDF, uploads it to storage,
// and persists the file key on the quote record.
func (p *QuoteAcceptanceProcessor) GenerateAndStorePDF(
	ctx context.Context,
	quoteID, organizationID uuid.UUID,
	orgName, customerName, signatureName string,
) (string, []byte, error) {
	// 1. Fetch quote and items
	quote, err := p.repo.GetByID(ctx, quoteID, organizationID)
	if err != nil {
		return "", nil, fmt.Errorf("fetch quote for PDF: %w", err)
	}

	items, err := p.repo.GetItemsByQuoteID(ctx, quoteID, organizationID)
	if err != nil {
		return "", nil, fmt.Errorf("fetch quote items for PDF: %w", err)
	}

	// 2. Fetch full organization profile
	org, orgErr := p.orgReader.GetOrganization(ctx, organizationID)

	// 3. Calculate totals + VAT breakdown using the service calculator
	itemReqs := make([]transport.QuoteItemRequest, len(items))
	for i, it := range items {
		itemReqs[i] = transport.QuoteItemRequest{
			Description:    it.Description,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
			TaxRateBps:     it.TaxRateBps,
			IsOptional:     it.IsOptional,
			IsSelected:     it.IsSelected,
		}
	}
	calc := service.CalculateQuote(transport.QuoteCalculationRequest{
		Items:         itemReqs,
		PricingMode:   quote.PricingMode,
		DiscountType:  quote.DiscountType,
		DiscountValue: quote.DiscountValue,
	})

	// 4. Build public-style item responses for the PDF
	pdfItems := buildPDFItems(items, quote.PricingMode)

	// 5. Decode drawn signature image (base64 data URL → raw PNG bytes)
	var signatureImageBytes []byte
	if quote.SignatureData != nil && *quote.SignatureData != "" {
		signatureImageBytes = decodeSignatureDataURL(*quote.SignatureData)
	}

	// 6. Download organization logo from MinIO (if available)
	var logoBytes []byte
	if orgErr == nil && org.LogoFileKey != nil && *org.LogoFileKey != "" {
		logoBucket := p.cfg.GetMinioBucketOrganizationLogos()
		logoReader, dlErr := p.storage.DownloadFile(ctx, logoBucket, *org.LogoFileKey)
		if dlErr == nil {
			defer logoReader.Close()
			logoBytes, _ = io.ReadAll(logoReader)
		}
	}

	// 7. Build QuotePDFData with full org profile
	pdfData := pdf.QuotePDFData{
		QuoteNumber:      quote.QuoteNumber,
		OrganizationName: orgName,
		CustomerName:     customerName,
		Status:           quote.Status,
		PricingMode:      quote.PricingMode,
		ValidUntil:       quote.ValidUntil,
		CreatedAt:        quote.CreatedAt,
		Notes:            quote.Notes,
		SignatureName:    &signatureName,
		SignatureImage:   signatureImageBytes,
		AcceptedAt:       quote.AcceptedAt,
		Items:            pdfItems,
		SubtotalCents:    calc.SubtotalCents,
		DiscountAmount:   calc.DiscountAmountCents,
		TaxTotalCents:    calc.VatTotalCents,
		TotalCents:       calc.TotalCents,
		VatBreakdown:     calc.VatBreakdown,
		OrgLogo:          logoBytes,
	}

	// Populate org details if available
	if orgErr == nil {
		pdfData.OrgEmail = derefStr(org.Email)
		pdfData.OrgPhone = derefStr(org.Phone)
		pdfData.OrgVatNumber = derefStr(org.VatNumber)
		pdfData.OrgKvkNumber = derefStr(org.KvkNumber)
		pdfData.OrgAddressLine1 = derefStr(org.AddressLine1)
		pdfData.OrgAddressLine2 = derefStr(org.AddressLine2)
		pdfData.OrgPostalCode = derefStr(org.PostalCode)
		pdfData.OrgCity = derefStr(org.City)
		pdfData.OrgCountry = derefStr(org.Country)
	}

	// 8. Generate PDF bytes
	pdfBytes, err := pdf.GenerateQuotePDF(pdfData)
	if err != nil {
		return "", nil, fmt.Errorf("generate PDF: %w", err)
	}

	// 9. Upload to MinIO
	bucket := p.cfg.GetMinioBucketQuotePDFs()
	folder := organizationID.String()
	fileName := fmt.Sprintf("%s.pdf", quote.QuoteNumber)
	reader := bytes.NewReader(pdfBytes)

	fileKey, err := p.storage.UploadFile(ctx, bucket, folder, fileName, "application/pdf", reader, int64(len(pdfBytes)))
	if err != nil {
		return "", nil, fmt.Errorf("upload PDF to storage: %w", err)
	}

	// 10. Persist file key on the quote record
	if err := p.repo.SetPDFFileKey(ctx, quoteID, fileKey); err != nil {
		return "", nil, fmt.Errorf("persist PDF file key: %w", err)
	}

	return fileKey, pdfBytes, nil
}

// derefStr safely dereferences a *string, returning "" if nil.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}



// buildPDFItems converts repository QuoteItems into transport PublicQuoteItemResponse
// suitable for the PDF generator, including per-line tax calculations.
func buildPDFItems(items []repository.QuoteItem, pricingMode string) []transport.PublicQuoteItemResponse {
	if pricingMode == "" {
		pricingMode = "exclusive"
	}

	result := make([]transport.PublicQuoteItemResponse, len(items))
	for i, it := range items {
		qty := parseQtyNumeric(it.Quantity)
		unitPrice := float64(it.UnitPriceCents)
		taxRateBps := it.TaxRateBps

		netUnitPrice := unitPrice
		if pricingMode == "inclusive" && taxRateBps > 0 {
			netUnitPrice = unitPrice / (1.0 + float64(taxRateBps)/10000.0)
		}

		lineSubtotal := qty * netUnitPrice
		lineVat := lineSubtotal * (float64(taxRateBps) / 10000.0)

		result[i] = transport.PublicQuoteItemResponse{
			ID:                  it.ID,
			Description:         it.Description,
			Quantity:            it.Quantity,
			UnitPriceCents:      it.UnitPriceCents,
			TaxRateBps:          it.TaxRateBps,
			IsOptional:          it.IsOptional,
			IsSelected:          it.IsSelected,
			SortOrder:           it.SortOrder,
			TotalBeforeTaxCents: roundC(lineSubtotal),
			TotalTaxCents:       roundC(lineVat),
			LineTotalCents:      roundC(lineSubtotal + lineVat),
		}
	}
	return result
}

// parseQtyNumeric parses a quantity string to a float64, defaulting to 1.
func parseQtyNumeric(q string) float64 {
	var f float64
	if _, err := fmt.Sscanf(q, "%f", &f); err != nil || f <= 0 {
		return 1
	}
	return f
}

// roundC rounds a float64 to the nearest cent.
func roundC(f float64) int64 {
	if f < 0 {
		return int64(f - 0.5)
	}
	return int64(f + 0.5)
}

// Compile-time check.
var _ notification.QuoteAcceptanceProcessor = (*QuoteAcceptanceProcessor)(nil)

// decodeSignatureDataURL strips the "data:image/png;base64," prefix from
// a data URL and decodes the remaining base64 payload into raw PNG bytes.
// Returns nil if decoding fails (non-fatal — the PDF just won't have the image).
func decodeSignatureDataURL(dataURL string) []byte {
	// Strip the data URL prefix if present, e.g. "data:image/png;base64,iVBOR..."
	b64 := dataURL
	if idx := strings.Index(dataURL, ","); idx >= 0 {
		b64 = dataURL[idx+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil
	}
	return raw
}
