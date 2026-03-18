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
	"portal_final_backend/internal/email"
	identityrepo "portal_final_backend/internal/identity/repository"
	partnersrepo "portal_final_backend/internal/partners/repository"
	"portal_final_backend/internal/pdf"

	"github.com/google/uuid"
)

// PartnerOfferPDFReader is the narrow repo interface used to read offer data.
type PartnerOfferPDFReader interface {
	GetOfferByIDWithContext(ctx context.Context, offerID uuid.UUID, organizationID uuid.UUID) (partnersrepo.PartnerOfferWithContext, error)
	SetOfferPDFFileKey(ctx context.Context, offerID uuid.UUID, fileKey string) error
	GetByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (partnersrepo.Partner, error)
}

// PartnerOfferPDFBucketConfig is the narrow config interface for bucket names.
type PartnerOfferPDFBucketConfig interface {
	GetMinioBucketQuotePDFs() string
	GetMinioBucketOrganizationLogos() string
}

// OfferPDFOrgReader is the narrow interface for fetching organization data.
type OfferPDFOrgReader interface {
	GetOrganization(ctx context.Context, organizationID uuid.UUID) (identityrepo.Organization, error)
}

// PartnerOfferPDFProcessor implements scheduler.OfferPDFProcessor.
// It generates a signed offer acceptance PDF and uploads it to MinIO.
type PartnerOfferPDFProcessor struct {
	repo      PartnerOfferPDFReader
	orgReader OfferPDFOrgReader
	storage   storage.StorageService
	cfg       PartnerOfferPDFBucketConfig
	sender    email.Sender
}

// NewPartnerOfferPDFProcessor creates a new processor.
func NewPartnerOfferPDFProcessor(
	repo PartnerOfferPDFReader,
	orgReader OfferPDFOrgReader,
	storageSvc storage.StorageService,
	cfg PartnerOfferPDFBucketConfig,
	sender email.Sender,
) *PartnerOfferPDFProcessor {
	return &PartnerOfferPDFProcessor{
		repo:      repo,
		orgReader: orgReader,
		storage:   storageSvc,
		cfg:       cfg,
		sender:    sender,
	}
}

// GenerateAndStoreOfferPDF fetches the accepted offer, generates a PDF, uploads it
// to MinIO, and persists the file key on the offer record.
func (p *PartnerOfferPDFProcessor) GenerateAndStoreOfferPDF(ctx context.Context, offerID, tenantID uuid.UUID) (string, error) {
	// 1. Fetch offer with context
	offer, err := p.repo.GetOfferByIDWithContext(ctx, offerID, tenantID)
	if err != nil {
		return "", fmt.Errorf("fetch offer for PDF: %w", err)
	}

	// 2. Fetch organisation profile
	org, orgErr := p.orgReader.GetOrganization(ctx, tenantID)

	// 3. Download org logo
	logoBytes := p.downloadOrgLogo(ctx, org, orgErr, tenantID)

	// 4. Decode signature
	var sigBytes []byte
	if offer.SignatureData != nil && *offer.SignatureData != "" {
		sigBytes = decodeOfferSignatureDataURL(*offer.SignatureData)
	}

	// 5. Build PDF data
	data := buildOfferAcceptancePDFData(offer, org, orgErr, logoBytes, sigBytes)

	// 6. Generate PDF
	pdfBytes, err := pdf.GenerateOfferAcceptancePDF(data)
	if err != nil {
		return "", fmt.Errorf("generate offer acceptance PDF: %w", err)
	}

	// 7. Upload to MinIO
	bucket := p.cfg.GetMinioBucketQuotePDFs()
	folder := tenantID.String()
	fileName := fmt.Sprintf("offer-%s.pdf", offer.ID.String()[:8])
	reader := bytes.NewReader(pdfBytes)

	fileKey, err := p.storage.UploadFile(ctx, bucket, folder, fileName, "application/pdf", reader, int64(len(pdfBytes)))
	if err != nil {
		return "", fmt.Errorf("upload offer PDF to storage: %w", err)
	}

	// 8. Persist file key
	if err := p.repo.SetOfferPDFFileKey(ctx, offerID, fileKey); err != nil {
		return "", fmt.Errorf("persist offer PDF file key: %w", err)
	}

	p.sendOfferPDFEmail(ctx, offer, tenantID, pdfBytes, fileName)

	return fileKey, nil
}

func buildOfferAcceptancePDFData(
	offer partnersrepo.PartnerOfferWithContext,
	org identityrepo.Organization,
	orgErr error,
	logoBytes []byte,
	sigBytes []byte,
) pdf.OfferAcceptancePDFData {
	data := pdf.OfferAcceptancePDFData{
		OfferRef:       offer.ID.String()[:8],
		OrgLogo:        logoBytes,
		ServiceType:    offer.ServiceType,
		JobSummary:     derefStr(offer.BuilderSummary),
		LeadCity:       offer.LeadCity,
		LeadName:       strings.TrimSpace(strings.TrimSpace(offer.LeadFirstName + " " + offer.LeadLastName)),
		LeadPhone:      strings.TrimSpace(offer.LeadPhone),
		LeadEmail:      strings.TrimSpace(offer.LeadEmail),
		LeadAddress:    formatOfferLeadAddress(offer),
		TotalCents:     offer.VakmanPriceCents,
		SignatureImage: sigBytes,
	}

	if offer.AcceptedAt != nil {
		data.AcceptedAt = *offer.AcceptedAt
	}
	if offer.SignerName != nil {
		data.SignerName = *offer.SignerName
	}
	if offer.SignerBusinessName != nil {
		data.SignerBusinessName = *offer.SignerBusinessName
	}
	if offer.SignerAddress != nil {
		data.SignerAddress = *offer.SignerAddress
	}
	if orgErr == nil {
		data.OrganizationName = org.Name
		data.OrgEmail = derefStr(org.Email)
		data.OrgPhone = derefStr(org.Phone)
		data.OrgVatNumber = derefStr(org.VatNumber)
		data.OrgKvkNumber = derefStr(org.KvkNumber)
		data.OrgAddressLine1 = derefStr(org.AddressLine1)
		data.OrgPostalCode = derefStr(org.PostalCode)
		data.OrgCity = derefStr(org.City)
	}

	data.Items = make([]pdf.OfferLineItemPDF, len(offer.OfferLineItems))
	for i, it := range offer.OfferLineItems {
		data.Items[i] = pdf.OfferLineItemPDF{
			Description:    it.Description,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
			LineTotalCents: it.LineTotalCents,
		}
	}

	return data
}

func formatOfferLeadAddress(offer partnersrepo.PartnerOfferWithContext) string {
	streetLine := strings.TrimSpace(strings.Join([]string{offer.LeadStreet, offer.LeadHouseNumber}, " "))
	cityLine := strings.TrimSpace(strings.Join([]string{offer.LeadZipCode, offer.LeadCity}, " "))
	return strings.TrimSpace(strings.Join([]string{streetLine, cityLine}, ", "))
}

func (p *PartnerOfferPDFProcessor) sendOfferPDFEmail(
	ctx context.Context,
	offer partnersrepo.PartnerOfferWithContext,
	tenantID uuid.UUID,
	pdfBytes []byte,
	fileName string,
) {
	if p == nil || p.sender == nil {
		return
	}

	partner, err := p.repo.GetByID(ctx, offer.PartnerID, tenantID)
	if err != nil {
		slog.Warn("could not fetch partner for offer pdf email", "offerId", offer.ID, "error", err)
		return
	}
	if strings.TrimSpace(partner.ContactEmail) == "" {
		return
	}

	attachment := email.Attachment{
		Content:  pdfBytes,
		FileName: fileName,
		MIMEType: "application/pdf",
	}
	if err := p.sender.SendPartnerOfferAcceptedConfirmationEmail(ctx, partner.ContactEmail, partner.ContactName, attachment); err != nil {
		slog.Warn("failed to send partner offer confirmation email with pdf", "offerId", offer.ID, "partnerId", partner.ID, "error", err)
	}
}

// downloadOrgLogo fetches the logo from MinIO, returning nil on any failure.
func (p *PartnerOfferPDFProcessor) downloadOrgLogo(
	ctx context.Context,
	org identityrepo.Organization,
	orgErr error,
	organizationID uuid.UUID,
) []byte {
	if orgErr != nil {
		slog.Warn("could not fetch organization for offer PDF logo", "error", orgErr)
		return nil
	}
	if org.LogoFileKey == nil || *org.LogoFileKey == "" {
		slog.Info("organization has no logo", "orgID", organizationID)
		return nil
	}

	bucket := p.cfg.GetMinioBucketOrganizationLogos()
	logoReader, dlErr := p.storage.DownloadFile(ctx, bucket, *org.LogoFileKey)
	if dlErr != nil {
		slog.Warn("offer PDF logo download failed", "key", *org.LogoFileKey, "error", dlErr)
		return nil
	}
	defer func() { _ = logoReader.Close() }()

	logoBytes, readErr := io.ReadAll(logoReader)
	if readErr != nil || len(logoBytes) == 0 {
		slog.Warn("offer PDF logo read failed", "key", *org.LogoFileKey, "error", readErr)
		return nil
	}

	return logoBytes
}

// decodeOfferSignatureDataURL strips the "data:image/png;base64," prefix and decodes.
// Returns nil if the input is not a valid data URL.
func decodeOfferSignatureDataURL(dataURL string) []byte {
	const prefix = "base64,"
	idx := strings.Index(dataURL, prefix)
	if idx < 0 {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(dataURL[idx+len(prefix):])
	if err != nil {
		slog.Warn("failed to decode signature data URL", "error", err)
		return nil
	}
	return raw
}
