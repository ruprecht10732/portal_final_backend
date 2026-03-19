package pdf

import (
	"context"
	"fmt"
	"strings"
	"time"
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
	OrganizationName     string
	QuoteNumber          string
	LeadName             string
	LeadAddress          string
	GeneratedAtFormatted string
	TotalFormatted       string
	DoublingLabel        string
	EligibleMeasureCount int
	InsulationBreakdown  []isdeSummaryLineItemViewModel
	GlassBreakdown       []isdeSummaryLineItemViewModel
	Installations        []isdeSummaryLineItemViewModel
	HasInsulation        bool
	HasGlass             bool
	HasInstallations     bool
	HasUnknownMeasures   bool
	HasUnknownMeldcodes  bool
	UnknownMeasureIDs    string
	UnknownMeldcodes     string
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

	vm := isdeSummaryViewModel{
		OrganizationName:     strings.TrimSpace(data.OrganizationName),
		QuoteNumber:          strings.TrimSpace(data.QuoteNumber),
		LeadName:             strings.TrimSpace(data.LeadName),
		LeadAddress:          strings.TrimSpace(data.LeadAddress),
		GeneratedAtFormatted: time.Now().Format(dateTimeFormatDMY),
		TotalFormatted:       formatCurrency(data.TotalAmountCents),
		DoublingLabel:        isdeDoublingLabel(data.IsDoubled),
		EligibleMeasureCount: data.EligibleMeasureCount,
		InsulationBreakdown:  buildISDESummaryLineItemVMs(data.InsulationBreakdown),
		GlassBreakdown:       buildISDESummaryLineItemVMs(data.GlassBreakdown),
		Installations:        buildISDESummaryLineItemVMs(data.Installations),
		HasInsulation:        len(data.InsulationBreakdown) > 0,
		HasGlass:             len(data.GlassBreakdown) > 0,
		HasInstallations:     len(data.Installations) > 0,
		HasUnknownMeasures:   len(data.UnknownMeasureIDs) > 0,
		HasUnknownMeldcodes:  len(data.UnknownMeldcodes) > 0,
		UnknownMeasureIDs:    strings.Join(data.UnknownMeasureIDs, ", "),
		UnknownMeldcodes:     strings.Join(data.UnknownMeldcodes, ", "),
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
