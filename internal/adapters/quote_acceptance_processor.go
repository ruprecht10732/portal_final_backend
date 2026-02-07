package adapters

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
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
	GetAttachmentsByQuoteID(ctx context.Context, quoteID uuid.UUID, orgID uuid.UUID) ([]repository.QuoteAttachment, error)
	GetURLsByQuoteID(ctx context.Context, quoteID uuid.UUID, orgID uuid.UUID) ([]repository.QuoteURL, error)
	SetPDFFileKey(ctx context.Context, quoteID uuid.UUID, fileKey string) error
}

// QuotePDFBucketConfig is the narrow config interface for the PDF bucket name.
type QuotePDFBucketConfig interface {
	GetMinioBucketQuotePDFs() string
	GetMinioBucketOrganizationLogos() string
	GetMinioBucketCatalogAssets() string
	GetMinioBucketQuoteAttachments() string
}

// QuoteOrgReader is the narrow interface for fetching organization profile data.
type QuoteOrgReader interface {
	GetOrganization(ctx context.Context, organizationID uuid.UUID) (identityrepo.Organization, error)
}

// QuoteAcceptanceProcessor implements notification.QuoteAcceptanceProcessor.
// It generates the quote PDF, uploads it to MinIO, and persists the file key.
type QuoteAcceptanceProcessor struct {
	repo           QuoteDataReader
	orgReader      QuoteOrgReader
	contactReader  service.QuoteContactReader
	storage        storage.StorageService
	cfg            QuotePDFBucketConfig
	settingsReader OrgSettingsReaderRepo
}

// NewQuoteAcceptanceProcessor creates a new processor adapter.
func NewQuoteAcceptanceProcessor(repo QuoteDataReader, orgReader QuoteOrgReader, contactReader service.QuoteContactReader, storageSvc storage.StorageService, cfg QuotePDFBucketConfig, settingsReader OrgSettingsReaderRepo) *QuoteAcceptanceProcessor {
	return &QuoteAcceptanceProcessor{
		repo:           repo,
		orgReader:      orgReader,
		contactReader:  contactReader,
		storage:        storageSvc,
		cfg:            cfg,
		settingsReader: settingsReader,
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
	calc := service.CalculateQuote(buildCalcRequest(items, quote))

	// 4. Build PDF data
	bc := pdfBuildContext{
		org:            org,
		orgErr:         orgErr,
		organizationID: organizationID,
		orgName:        orgName,
		customerName:   customerName,
		signatureName:  signatureName,
	}
	pdfData := p.buildPDFData(ctx, quote, items, calc, bc)

	// 5. Generate PDF bytes
	pdfBytes, err := pdf.GenerateQuotePDF(pdfData)
	if err != nil {
		return "", nil, fmt.Errorf("generate PDF: %w", err)
	}

	// 6. Upload and persist
	return p.uploadAndPersist(ctx, pdfBytes, quoteID, organizationID, quote.QuoteNumber)
}

// buildCalcRequest converts repository items + quote into a calculation request.
func buildCalcRequest(items []repository.QuoteItem, quote *repository.Quote) transport.QuoteCalculationRequest {
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
	return transport.QuoteCalculationRequest{
		Items:         itemReqs,
		PricingMode:   quote.PricingMode,
		DiscountType:  quote.DiscountType,
		DiscountValue: quote.DiscountValue,
	}
}

// pdfBuildContext groups ancillary data needed to build the PDF model.
type pdfBuildContext struct {
	org            identityrepo.Organization
	orgErr         error
	organizationID uuid.UUID
	orgName        string
	customerName   string
	signatureName  string
}

// buildPDFData assembles the full QuotePDFData struct from all gathered data.
func (p *QuoteAcceptanceProcessor) buildPDFData(
	ctx context.Context,
	quote *repository.Quote,
	items []repository.QuoteItem,
	calc transport.QuoteCalculationResponse,
	bc pdfBuildContext,
) pdf.QuotePDFData {
	pdfItems := buildPDFItems(items, quote.PricingMode)

	var signatureImageBytes []byte
	if quote.SignatureData != nil && *quote.SignatureData != "" {
		signatureImageBytes = decodeSignatureDataURL(*quote.SignatureData)
	}

	logoBytes := p.downloadOrgLogo(ctx, bc.org, bc.orgErr, bc.organizationID)

	data := pdf.QuotePDFData{
		QuoteNumber:      quote.QuoteNumber,
		OrganizationName: bc.orgName,
		CustomerName:     bc.customerName,
		Status:           quote.Status,
		PricingMode:      quote.PricingMode,
		ValidUntil:       quote.ValidUntil,
		CreatedAt:        quote.CreatedAt,
		Notes:            quote.Notes,
		SignatureName:    &bc.signatureName,
		SignatureImage:   signatureImageBytes,
		AcceptedAt:       quote.AcceptedAt,
		Items:            pdfItems,
		SubtotalCents:    calc.SubtotalCents,
		DiscountAmount:   calc.DiscountAmountCents,
		TaxTotalCents:    calc.VatTotalCents,
		TotalCents:       calc.TotalCents,
		VatBreakdown:     calc.VatBreakdown,
		OrgLogo:          logoBytes,
		PaymentDays:      7,
		QuoteValidDays:   14,
	}

	if p.settingsReader != nil {
		settings, settingsErr := p.settingsReader.GetOrganizationSettings(ctx, bc.organizationID)
		if settingsErr == nil {
			data.PaymentDays = settings.QuotePaymentDays
			data.QuoteValidDays = settings.QuoteValidDays
		}
	}

	if bc.orgErr == nil {
		data.OrgEmail = derefStr(bc.org.Email)
		data.OrgPhone = derefStr(bc.org.Phone)
		data.OrgVatNumber = derefStr(bc.org.VatNumber)
		data.OrgKvkNumber = derefStr(bc.org.KvkNumber)
		data.OrgAddressLine1 = derefStr(bc.org.AddressLine1)
		data.OrgAddressLine2 = derefStr(bc.org.AddressLine2)
		data.OrgPostalCode = derefStr(bc.org.PostalCode)
		data.OrgCity = derefStr(bc.org.City)
		data.OrgCountry = derefStr(bc.org.Country)
	}

	// Load document attachments and download enabled PDFs from MinIO
	data.AttachmentPDFs = p.downloadEnabledAttachments(ctx, quote.ID, quote.OrganizationID)
	data.URLs = p.loadURLEntries(ctx, quote.ID, quote.OrganizationID)

	return data
}

// downloadOrgLogo fetches the organization logo from storage, returning nil on any failure.
func (p *QuoteAcceptanceProcessor) downloadOrgLogo(
	ctx context.Context,
	org identityrepo.Organization,
	orgErr error,
	organizationID uuid.UUID,
) []byte {
	if orgErr != nil {
		slog.Warn("could not fetch organization for logo", "error", orgErr)
		return nil
	}
	if org.LogoFileKey == nil || *org.LogoFileKey == "" {
		slog.Info("organization has no logo file key", "orgID", organizationID)
		return nil
	}

	logoBucket := p.cfg.GetMinioBucketOrganizationLogos()
	slog.Info("downloading org logo", "bucket", logoBucket, "key", *org.LogoFileKey)

	logoReader, dlErr := p.storage.DownloadFile(ctx, logoBucket, *org.LogoFileKey)
	if dlErr != nil {
		slog.Warn("logo download failed", "bucket", logoBucket, "key", *org.LogoFileKey, "error", dlErr)
		return nil
	}
	defer func() { _ = logoReader.Close() }()

	data, readErr := io.ReadAll(logoReader)
	if readErr != nil {
		slog.Warn("failed to read logo bytes", "key", *org.LogoFileKey, "error", readErr)
		return nil
	}
	if len(data) == 0 {
		slog.Warn("logo file is empty", "key", *org.LogoFileKey)
		return nil
	}

	slog.Info("logo loaded", "key", *org.LogoFileKey, "bytes", len(data))
	return data
}

// downloadEnabledAttachments fetches all enabled attachment PDFs from MinIO.
// Catalog-sourced attachments are read from the catalog-assets bucket;
// manually uploaded attachments are read from the quote-attachments bucket.
func (p *QuoteAcceptanceProcessor) downloadEnabledAttachments(ctx context.Context, quoteID, orgID uuid.UUID) []pdf.AttachmentPDFEntry {
	attachments, err := p.repo.GetAttachmentsByQuoteID(ctx, quoteID, orgID)
	if err != nil {
		slog.Warn("failed to load quote attachments", "quoteID", quoteID, "error", err)
		return nil
	}

	catalogBucket := p.cfg.GetMinioBucketCatalogAssets()
	manualBucket := p.cfg.GetMinioBucketQuoteAttachments()

	var result []pdf.AttachmentPDFEntry
	for _, att := range attachments {
		if !att.Enabled || att.FileKey == "" {
			continue
		}

		bucket := catalogBucket
		if att.Source == "manual" {
			bucket = manualBucket
		}

		reader, dlErr := p.storage.DownloadFile(ctx, bucket, att.FileKey)
		if dlErr != nil {
			slog.Warn("failed to download attachment PDF", "fileKey", att.FileKey, "error", dlErr)
			continue
		}
		data, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil || len(data) == 0 {
			slog.Warn("empty or unreadable attachment PDF", "fileKey", att.FileKey)
			continue
		}

		result = append(result, pdf.AttachmentPDFEntry{
			Filename: att.Filename,
			PDFBytes: data,
		})
	}
	return result
}

// loadURLEntries converts stored quote URLs into PDF URL entries.
func (p *QuoteAcceptanceProcessor) loadURLEntries(ctx context.Context, quoteID, orgID uuid.UUID) []pdf.QuoteURLEntry {
	urls, err := p.repo.GetURLsByQuoteID(ctx, quoteID, orgID)
	if err != nil {
		slog.Warn("failed to load quote URLs", "quoteID", quoteID, "error", err)
		return nil
	}

	result := make([]pdf.QuoteURLEntry, len(urls))
	for i, u := range urls {
		result[i] = pdf.QuoteURLEntry{Label: u.Label, Href: u.Href}
	}
	return result
}

// uploadAndPersist uploads the PDF to MinIO and persists the file key on the quote record.
func (p *QuoteAcceptanceProcessor) uploadAndPersist(
	ctx context.Context,
	pdfBytes []byte,
	quoteID, organizationID uuid.UUID,
	quoteNumber string,
) (string, []byte, error) {
	bucket := p.cfg.GetMinioBucketQuotePDFs()
	folder := organizationID.String()
	fileName := fmt.Sprintf("%s.pdf", quoteNumber)
	reader := bytes.NewReader(pdfBytes)

	fileKey, err := p.storage.UploadFile(ctx, bucket, folder, fileName, "application/pdf", reader, int64(len(pdfBytes)))
	if err != nil {
		return "", nil, fmt.Errorf("upload PDF to storage: %w", err)
	}

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

// RegeneratePDF generates and stores the quote PDF on demand, looking up all
// required metadata (org name, customer name, signature name) from the database.
// This is used for lazy/on-demand PDF generation in download endpoints.
func (p *QuoteAcceptanceProcessor) RegeneratePDF(
	ctx context.Context,
	quoteID, organizationID uuid.UUID,
) (string, []byte, error) {
	// Fetch quote to get LeadID & SignatureName
	quote, err := p.repo.GetByID(ctx, quoteID, organizationID)
	if err != nil {
		return "", nil, fmt.Errorf("fetch quote for PDF regeneration: %w", err)
	}

	signatureName := ""
	if quote.SignatureName != nil {
		signatureName = *quote.SignatureName
	}

	// Resolve org name and customer name from the lead contact data
	orgName := ""
	customerName := signatureName // fallback to signature name
	if p.contactReader != nil {
		contactData, contactErr := p.contactReader.GetQuoteContactData(ctx, quote.LeadID, organizationID)
		if contactErr == nil {
			orgName = contactData.OrganizationName
			if contactData.ConsumerName != "" {
				customerName = contactData.ConsumerName
			}
		}
	}

	// Fallback: get org name from org reader if contact reader didn't provide it
	if orgName == "" {
		org, orgErr := p.orgReader.GetOrganization(ctx, organizationID)
		if orgErr == nil {
			orgName = org.Name
		}
	}

	return p.GenerateAndStorePDF(ctx, quoteID, organizationID, orgName, customerName, signatureName)
}

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
