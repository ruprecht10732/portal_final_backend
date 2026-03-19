package pdf

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

const (
	isdeRVOURL          = "https://www.rvo.nl/subsidies-financiering/isde/woningeigenaren"
	isdeKlimaatrouteURL = "https://www.klimaatroute.nl/bewoners"
	isdeQRCodeSize      = 160
)

// ISDESummaryPDFData holds the content rendered into an ISDE subsidy summary PDF.
type ISDESummaryPDFData struct {
	QuoteNumber          string
	OrganizationName     string
	LeadName             string
	LeadAddress          string
	TotalAmountCents     int64
	IsDoubled            bool
	EligibleMeasureCount int
	InsulationBreakdown  []ISDESummaryLineItem
	GlassBreakdown       []ISDESummaryLineItem
	Installations        []ISDESummaryLineItem
	UnknownMeasureIDs    []string
	UnknownMeldcodes     []string
}

// ISDESummaryLineItem is one subsidy breakdown row in the generated PDF.
type ISDESummaryLineItem struct {
	Description string
	AreaM2      float64
	AmountCents int64
}

type isdeSummaryViewModel struct {
	OrganizationName         string
	QuoteNumber              string
	LeadName                 string
	LeadAddress              string
	GeneratedAtFormatted     string
	TotalFormatted           string
	DoublingLabel            string
	EligibleMeasureCount     int
	RvoURL                   string
	RvoQrCodeBase64          string
	KlimaatrouteURL          string
	KlimaatrouteQrCodeBase64 string
	InsulationBreakdown      []isdeSummaryLineItemViewModel
	GlassBreakdown           []isdeSummaryLineItemViewModel
	Installations            []isdeSummaryLineItemViewModel
	HasInsulation            bool
	HasGlass                 bool
	HasInstallations         bool
	HasUnknownMeasures       bool
	HasUnknownMeldcodes      bool
	UnknownMeasureIDs        string
	UnknownMeldcodes         string
}

type isdeSummaryLineItemViewModel struct {
	Description     string
	AreaFormatted   string
	HasArea         bool
	AmountFormatted string
}

// GenerateISDESummaryPDF renders an ISDE subsidy summary as a standalone PDF.
func GenerateISDESummaryPDF(data ISDESummaryPDFData) ([]byte, error) {
	if gotenbergClient == nil {
		return nil, fmt.Errorf("gotenberg client not initialized — call pdf.Init first")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vm, err := buildISDESummaryViewModel(data, time.Now())
	if err != nil {
		return nil, err
	}

	htmlContent, err := renderTemplate("templates/isde_subsidy_summary.html", vm)
	if err != nil {
		return nil, fmt.Errorf("render isde subsidy summary template: %w", err)
	}

	pdfBytes, err := gotenbergClient.ConvertHTML(ctx, htmlContent, DefaultContentOpts())
	if err != nil {
		return nil, fmt.Errorf("convert isde subsidy summary to PDF: %w", err)
	}

	return pdfBytes, nil
}

func buildISDESummaryViewModel(data ISDESummaryPDFData, now time.Time) (isdeSummaryViewModel, error) {
	rvoQrCodeBase64, err := generateQRCodeBase64(isdeRVOURL, isdeQRCodeSize)
	if err != nil {
		return isdeSummaryViewModel{}, fmt.Errorf("generate RVO QR code: %w", err)
	}

	klimaatrouteQrCodeBase64, err := generateQRCodeBase64(isdeKlimaatrouteURL, isdeQRCodeSize)
	if err != nil {
		return isdeSummaryViewModel{}, fmt.Errorf("generate Klimaatroute QR code: %w", err)
	}

	return isdeSummaryViewModel{
		OrganizationName:         strings.TrimSpace(data.OrganizationName),
		QuoteNumber:              strings.TrimSpace(data.QuoteNumber),
		LeadName:                 strings.TrimSpace(data.LeadName),
		LeadAddress:              strings.TrimSpace(data.LeadAddress),
		GeneratedAtFormatted:     now.Format(dateTimeFormatDMY),
		TotalFormatted:           formatCurrency(data.TotalAmountCents),
		DoublingLabel:            isdeDoublingLabel(data.IsDoubled),
		EligibleMeasureCount:     data.EligibleMeasureCount,
		RvoURL:                   isdeRVOURL,
		RvoQrCodeBase64:          rvoQrCodeBase64,
		KlimaatrouteURL:          isdeKlimaatrouteURL,
		KlimaatrouteQrCodeBase64: klimaatrouteQrCodeBase64,
		InsulationBreakdown:      buildISDESummaryLineItemVMs(data.InsulationBreakdown),
		GlassBreakdown:           buildISDESummaryLineItemVMs(data.GlassBreakdown),
		Installations:            buildISDESummaryLineItemVMs(data.Installations),
		HasInsulation:            len(data.InsulationBreakdown) > 0,
		HasGlass:                 len(data.GlassBreakdown) > 0,
		HasInstallations:         len(data.Installations) > 0,
		HasUnknownMeasures:       len(data.UnknownMeasureIDs) > 0,
		HasUnknownMeldcodes:      len(data.UnknownMeldcodes) > 0,
		UnknownMeasureIDs:        strings.Join(data.UnknownMeasureIDs, ", "),
		UnknownMeldcodes:         strings.Join(data.UnknownMeldcodes, ", "),
	}, nil
}

func generateQRCodeBase64(content string, size int) (string, error) {
	pngBytes, err := qrcode.Encode(strings.TrimSpace(content), qrcode.Medium, size)
	if err != nil {
		return "", fmt.Errorf("encode QR code: %w", err)
	}

	return base64.StdEncoding.EncodeToString(pngBytes), nil
}

func buildISDESummaryLineItemVMs(items []ISDESummaryLineItem) []isdeSummaryLineItemViewModel {
	result := make([]isdeSummaryLineItemViewModel, 0, len(items))
	for _, item := range items {
		result = append(result, isdeSummaryLineItemViewModel{
			Description:     strings.TrimSpace(item.Description),
			AreaFormatted:   formatISDEArea(item.AreaM2),
			HasArea:         item.AreaM2 > 0,
			AmountFormatted: formatCurrency(item.AmountCents),
		})
	}
	return result
}

func formatISDEArea(area float64) string {
	if area <= 0 {
		return ""
	}
	text := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", area), "0"), ".")
	return text + " m2"
}

func isdeDoublingLabel(isDoubled bool) string {
	if isDoubled {
		return "Verdubbeld tarief toegepast"
	}
	return "Basistarief toegepast"
}
