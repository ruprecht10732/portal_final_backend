package adapters

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/email"
	identityrepo "portal_final_backend/internal/identity/repository"
	"portal_final_backend/internal/identity/smtpcrypto"
	partnersrepo "portal_final_backend/internal/partners/repository"
	"portal_final_backend/internal/pdf"

	"github.com/google/uuid"
)

// PartnerOfferPDFReader is the narrow repo interface used to read offer data.
type PartnerOfferPDFReader interface {
	GetOfferByIDWithContext(ctx context.Context, offerID uuid.UUID, organizationID uuid.UUID) (partnersrepo.PartnerOfferWithContext, error)
	SetOfferPDFFileKey(ctx context.Context, offerID uuid.UUID, fileKey string) error
	GetByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (partnersrepo.Partner, error)
	GetLeadServiceImageAttachments(ctx context.Context, leadServiceID uuid.UUID, organizationID uuid.UUID) ([]partnersrepo.PhotoAttachment, error)
	GetActivePartnerOfferTerms(ctx context.Context, organizationID uuid.UUID) (partnersrepo.PartnerOfferTerms, error)
}

// PartnerOfferPDFBucketConfig is the narrow config interface for bucket names.
type PartnerOfferPDFBucketConfig interface {
	GetMinioBucketQuotePDFs() string
	GetMinioBucketOrganizationLogos() string
	GetMinioBucketLeadServiceAttachments() string
	GetSMTPEncryptionKey() string
}

// OfferPDFOrgReader is the narrow interface for fetching organization data.
type OfferPDFOrgReader interface {
	GetOrganization(ctx context.Context, organizationID uuid.UUID) (identityrepo.Organization, error)
	GetOrganizationSettings(ctx context.Context, organizationID uuid.UUID) (identityrepo.OrganizationSettings, error)
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
	fileKey, _, err := p.generateAndStoreOfferPDF(ctx, offerID, tenantID, true)
	return fileKey, err
}

func (p *PartnerOfferPDFProcessor) RegeneratePDF(ctx context.Context, offerID, tenantID uuid.UUID) (string, []byte, error) {
	return p.generateAndStoreOfferPDF(ctx, offerID, tenantID, false)
}

func (p *PartnerOfferPDFProcessor) generateAndStoreOfferPDF(ctx context.Context, offerID, tenantID uuid.UUID, sendEmail bool) (string, []byte, error) {
	// 1. Fetch offer with context
	offer, err := p.repo.GetOfferByIDWithContext(ctx, offerID, tenantID)
	if err != nil {
		return "", nil, fmt.Errorf("fetch offer for PDF: %w", err)
	}

	// 2. Fetch organisation profile
	org, orgErr := p.orgReader.GetOrganization(ctx, tenantID)

	// 3. Download org logo
	logoBytes := p.downloadOrgLogo(ctx, org, orgErr, tenantID)
	photos := p.downloadOfferPhotos(ctx, offer)
	termsContent := p.loadTermsContent(ctx, tenantID)

	// 4. Decode signature
	var sigBytes []byte
	if offer.SignatureData != nil && *offer.SignatureData != "" {
		sigBytes = decodeOfferSignatureDataURL(*offer.SignatureData)
	}

	// 5. Build PDF data
	data := buildOfferAcceptancePDFData(offer, org, orgErr, logoBytes, sigBytes, photos, termsContent)

	// 6. Generate PDF
	pdfBytes, err := pdf.GenerateOfferAcceptancePDF(data)
	if err != nil {
		return "", nil, fmt.Errorf("generate offer acceptance PDF: %w", err)
	}

	// 7. Upload to MinIO
	bucket := p.cfg.GetMinioBucketQuotePDFs()
	folder := tenantID.String()
	fileName := fmt.Sprintf("offer-%s.pdf", offer.ID.String()[:8])
	reader := bytes.NewReader(pdfBytes)

	fileKey, err := p.storage.UploadFile(ctx, bucket, folder, fileName, "application/pdf", reader, int64(len(pdfBytes)))
	if err != nil {
		return "", nil, fmt.Errorf("upload offer PDF to storage: %w", err)
	}

	// 8. Persist file key
	if err := p.repo.SetOfferPDFFileKey(ctx, offerID, fileKey); err != nil {
		return "", nil, fmt.Errorf("persist offer PDF file key: %w", err)
	}

	if sendEmail {
		p.sendOfferPDFEmail(ctx, offer, tenantID, pdfBytes, fileName)
	}

	return fileKey, pdfBytes, nil
}

func buildOfferAcceptancePDFData(
	offer partnersrepo.PartnerOfferWithContext,
	org identityrepo.Organization,
	orgErr error,
	logoBytes []byte,
	sigBytes []byte,
	photos []pdf.OfferPhotoPDF,
	termsContent string,
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
		Photos:         photos,
		TermsContent:   termsContent,
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
	sender := p.resolveSender(ctx, tenantID)
	if err := sender.SendPartnerOfferAcceptedConfirmationEmail(ctx, partner.ContactEmail, partner.ContactName, attachment); err != nil {
		slog.Warn("failed to send partner offer confirmation email with pdf", "offerId", offer.ID, "partnerId", partner.ID, "error", err)
	}
}

func (p *PartnerOfferPDFProcessor) resolveSender(ctx context.Context, organizationID uuid.UUID) email.Sender {
	if p == nil || p.sender == nil {
		return email.NoopSender{}
	}
	if p.orgReader == nil {
		return p.sender
	}

	settings, err := p.orgReader.GetOrganizationSettings(ctx, organizationID)
	if err != nil {
		slog.Warn("could not load organization smtp settings for partner offer pdf email", "organizationId", organizationID, "error", err)
		return p.sender
	}
	if settings.SMTPHost == nil || strings.TrimSpace(*settings.SMTPHost) == "" {
		return p.sender
	}

	password := ""
	if settings.SMTPPassword != nil && strings.TrimSpace(*settings.SMTPPassword) != "" {
		keyHex := strings.TrimSpace(p.cfg.GetSMTPEncryptionKey())
		if keyHex == "" {
			slog.Warn("smtp password configured but SMTP_ENCRYPTION_KEY is missing for partner offer pdf email", "organizationId", organizationID)
			return p.sender
		}
		key, err := hex.DecodeString(keyHex)
		if err != nil {
			slog.Warn("invalid SMTP_ENCRYPTION_KEY for partner offer pdf email", "organizationId", organizationID, "error", err)
			return p.sender
		}
		decrypted, err := smtpcrypto.Decrypt(*settings.SMTPPassword, key)
		if err != nil {
			slog.Warn("failed to decrypt smtp password for partner offer pdf email", "organizationId", organizationID, "error", err)
			return p.sender
		}
		password = decrypted
	}

	port := 587
	if settings.SMTPPort != nil {
		port = *settings.SMTPPort
	}

	return email.NewSMTPSender(
		strings.TrimSpace(*settings.SMTPHost),
		port,
		strings.TrimSpace(derefStr(settings.SMTPUsername)),
		password,
		strings.TrimSpace(derefStr(settings.SMTPFromEmail)),
		strings.TrimSpace(derefStr(settings.SMTPFromName)),
	)
}

func (p *PartnerOfferPDFProcessor) loadTermsContent(ctx context.Context, organizationID uuid.UUID) string {
	terms, err := p.repo.GetActivePartnerOfferTerms(ctx, organizationID)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(terms.Content)
}

func (p *PartnerOfferPDFProcessor) downloadOfferPhotos(ctx context.Context, offer partnersrepo.PartnerOfferWithContext) []pdf.OfferPhotoPDF {
	if p == nil || p.storage == nil {
		return nil
	}
	bucket := strings.TrimSpace(p.cfg.GetMinioBucketLeadServiceAttachments())
	if bucket == "" {
		return nil
	}

	attachments, err := p.repo.GetLeadServiceImageAttachments(ctx, offer.LeadServiceID, offer.OrganizationID)
	if err != nil {
		slog.Warn("failed to load offer photo attachments for pdf", "offerId", offer.ID, "error", err)
		return nil
	}

	photos := make([]pdf.OfferPhotoPDF, 0, len(attachments))
	for _, attachment := range attachments {
		reader, err := p.storage.DownloadFile(ctx, bucket, attachment.FileKey)
		if err != nil {
			slog.Warn("failed to download offer photo for pdf", "offerId", offer.ID, "attachmentId", attachment.ID, "error", err)
			continue
		}
		data, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil || len(data) == 0 {
			slog.Warn("failed to read offer photo for pdf", "offerId", offer.ID, "attachmentId", attachment.ID, "error", readErr)
			continue
		}
		photos = append(photos, pdf.OfferPhotoPDF{
			FileName:    attachment.FileName,
			ContentType: attachment.ContentType,
			Bytes:       data,
		})
	}

	return photos
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
