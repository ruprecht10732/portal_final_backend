package pdf

import (
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"time"
)

// OfferAcceptancePDFData holds all data needed to generate an accepted offer PDF.
type OfferAcceptancePDFData struct {
	// Offer metadata
	OfferRef   string // human-readable reference (short UUID suffix)
	AcceptedAt time.Time

	// Organization (the trade company presenting the offer)
	OrganizationName string
	OrgEmail         string
	OrgPhone         string
	OrgVatNumber     string
	OrgKvkNumber     string
	OrgAddressLine1  string
	OrgPostalCode    string
	OrgCity          string
	OrgLogo          []byte

	// Job context
	ServiceType string
	JobSummary  string
	LeadCity    string
	LeadName    string
	LeadPhone   string
	LeadEmail   string
	LeadAddress string

	// Line items
	Items      []OfferLineItemPDF
	TotalCents int64

	// Signer (vakman / partner who accepted)
	SignerName         string
	SignerBusinessName string
	SignerAddress      string

	// Signature drawn by the signer
	SignatureImage []byte // raw PNG bytes
}

// OfferLineItemPDF is the per-line view for the PDF.
type OfferLineItemPDF struct {
	Description    string
	Quantity       string
	UnitPriceCents int64
	LineTotalCents int64
}

// GenerateOfferAcceptancePDF produces a signed PDF confirming acceptance of a partner offer.
func GenerateOfferAcceptancePDF(data OfferAcceptancePDFData) ([]byte, error) {
	if gotenbergClient == nil {
		return nil, fmt.Errorf("gotenberg client not initialized — call pdf.Init first")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logoB64, logoMime := encodeLogoBase64(data.OrgLogo)

	var signatureB64 string
	if len(data.SignatureImage) > 0 {
		signatureB64 = "data:image/png;base64," + base64.StdEncoding.EncodeToString(data.SignatureImage)
	}

	vm := offerAcceptanceViewModel{
		LogoBase64:          logoB64,
		LogoMimeType:        logoMime,
		OrganizationName:    data.OrganizationName,
		OrgEmail:            data.OrgEmail,
		OrgPhone:            data.OrgPhone,
		OrgVatNumber:        data.OrgVatNumber,
		OrgKvkNumber:        data.OrgKvkNumber,
		OrgAddressLine1:     data.OrgAddressLine1,
		OrgPostalCode:       data.OrgPostalCode,
		OrgCity:             data.OrgCity,
		OfferRef:            data.OfferRef,
		AcceptedAtFormatted: data.AcceptedAt.Format(dateTimeFormatDMY),
		ServiceType:         data.ServiceType,
		JobSummary:          template.HTML(clampPDFText(data.JobSummary, maxPDFLongText)), //nolint:gosec
		LeadCity:            data.LeadCity,
		LeadName:            data.LeadName,
		LeadPhone:           data.LeadPhone,
		LeadEmail:           data.LeadEmail,
		LeadAddress:         data.LeadAddress,
		TotalFormatted:      formatCurrency(data.TotalCents),
		SignerName:          data.SignerName,
		SignerBusinessName:  data.SignerBusinessName,
		SignerAddress:       data.SignerAddress,
		HasSignature:        signatureB64 != "",
		SignatureBase64:     signatureB64,
		Items:               buildOfferItemVMs(data.Items),
	}

	htmlContent, err := renderTemplate("templates/offer_acceptance.html", vm)
	if err != nil {
		return nil, fmt.Errorf("render offer acceptance template: %w", err)
	}

	opts := DefaultContentOpts()
	pdfBytes, err := gotenbergClient.ConvertHTML(ctx, htmlContent, opts)
	if err != nil {
		return nil, fmt.Errorf("convert offer acceptance to PDF: %w", err)
	}

	return pdfBytes, nil
}

// ── View models ──────────────────────────────────────────────────────────────

type offerAcceptanceViewModel struct {
	LogoBase64          string
	LogoMimeType        string
	OrganizationName    string
	OrgEmail            string
	OrgPhone            string
	OrgVatNumber        string
	OrgKvkNumber        string
	OrgAddressLine1     string
	OrgPostalCode       string
	OrgCity             string
	OfferRef            string
	AcceptedAtFormatted string
	ServiceType         string
	JobSummary          template.HTML
	LeadCity            string
	LeadName            string
	LeadPhone           string
	LeadEmail           string
	LeadAddress         string
	Items               []offerItemViewModel
	TotalFormatted      string
	SignerName          string
	SignerBusinessName  string
	SignerAddress       string
	HasSignature        bool
	SignatureBase64     string
}

type offerItemViewModel struct {
	Description        template.HTML
	Quantity           string
	UnitFormatted      string
	LineTotalFormatted string
}

func buildOfferItemVMs(items []OfferLineItemPDF) []offerItemViewModel {
	vms := make([]offerItemViewModel, len(items))
	for i, it := range items {
		vms[i] = offerItemViewModel{
			Description:        template.HTML(it.Description), //nolint:gosec // content comes from trusted quote editors
			Quantity:           it.Quantity,
			UnitFormatted:      formatCurrency(it.UnitPriceCents),
			LineTotalFormatted: formatCurrency(it.LineTotalCents),
		}
	}
	return vms
}
