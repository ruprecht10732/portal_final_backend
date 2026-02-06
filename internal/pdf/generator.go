// Package pdf generates quote PDFs using Gotenberg (HTML→PDF via Chromium).
// The cover page uses an industrial "construction proposal" design with the
// Barlow font; the detail page contains all quote data, line-items, totals,
// legal terms, and the signature block.
package pdf

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"portal_final_backend/internal/quotes/transport"
)

//go:embed templates/*.html
var templateFS embed.FS

// ── Package-level Gotenberg client ──────────────────────────────────────

var gotenbergClient *GotenbergClient

// Init initialises the Gotenberg client. Must be called before GenerateQuotePDF.
func Init(gotenbergURL string) {
	if gotenbergURL != "" {
		gotenbergClient = NewGotenbergClient(gotenbergURL)
	}
}

// ── Data structs ────────────────────────────────────────────────────────

// QuotePDFData holds all data needed to generate a quote PDF.
type QuotePDFData struct {
	// Quote
	QuoteNumber string
	Status      string
	PricingMode string
	ValidUntil  *time.Time
	CreatedAt   time.Time
	Notes       *string

	// Organization
	OrganizationName string
	OrgEmail         string
	OrgPhone         string
	OrgVatNumber     string
	OrgKvkNumber     string
	OrgAddressLine1  string
	OrgAddressLine2  string
	OrgPostalCode    string
	OrgCity          string
	OrgCountry       string
	OrgLogo          []byte // raw image bytes (PNG or JPEG)

	// Customer
	CustomerName string

	// Signature (populated when accepted)
	SignatureName  *string
	SignatureImage []byte // raw PNG bytes of the drawn signature
	AcceptedAt     *time.Time

	// Line items & totals
	Items          []transport.PublicQuoteItemResponse
	SubtotalCents  int64
	DiscountAmount int64
	TaxTotalCents  int64
	TotalCents     int64
	VatBreakdown   []transport.VatBreakdown
}

// ── Template view models ────────────────────────────────────────────────

type coverViewModel struct {
	LogoBase64          string
	LogoMimeType        string
	OrganizationName    string
	CustomerName        string
	QuoteNumber         string
	QuoteSequenceNumber string
	CreatedAtFormatted  string
	ValidUntilFormatted string
	OrgAddressLine1     string
	OrgPostalCode       string
	OrgCity             string
	OrgPhone            string
	OrgEmail            string
}

type quoteViewModel struct {
	LogoBase64          string
	LogoMimeType        string
	OrganizationName    string
	CustomerName        string
	QuoteNumber         string
	CreatedAtFormatted  string
	ValidUntilFormatted string
	Status              string
	StatusLabel         string
	StatusClass         string
	OrgAddressLine1     string
	OrgAddressLine2     string
	OrgPostalCode       string
	OrgCity             string
	OrgEmail            string
	OrgPhone            string
	OrgKvkNumber        string
	OrgVatNumber        string
	AcceptedAtFormatted string
	Items               []itemViewModel
	SubtotalFormatted   string
	HasDiscount         bool
	DiscountFormatted   string
	VatBreakdown        []vatLineViewModel
	TotalFormatted      string
	Notes               string
	HasSignature        bool
	SignatureName       string
	SignatureBase64     string
}

type itemViewModel struct {
	Description       string
	Quantity          string
	UnitPriceFormatted string
	VatPctFormatted   string
	LineTotalFormatted string
	IsOptional        bool
	IsSelected        bool
}

type vatLineViewModel struct {
	PctFormatted    string
	AmountFormatted string
}

type footerViewModel struct {
	FooterText string
}

// ── Public API ──────────────────────────────────────────────────────────

// GenerateQuotePDF creates a professional multi-page PDF document.
// Page 1 = cover page (industrial Barlow design).
// Page 2+ = quote details with line items, totals, legal terms, and signature.
func GenerateQuotePDF(data QuotePDFData) ([]byte, error) {
	if gotenbergClient == nil {
		return nil, fmt.Errorf("gotenberg client not initialized — call pdf.Init first")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// ── Build view models ───────────────────────────────────────────────
	logoB64, logoMime := encodeLogoBase64(data.OrgLogo)

	cover := buildCoverVM(data, logoB64, logoMime)
	quote := buildQuoteVM(data, logoB64, logoMime)
	footer := buildFooterVM(data)

	// ── Render HTML templates ───────────────────────────────────────────
	coverHTML, err := renderTemplate("templates/cover.html", cover)
	if err != nil {
		return nil, fmt.Errorf("render cover template: %w", err)
	}

	quoteHTML, err := renderTemplate("templates/quote.html", quote)
	if err != nil {
		return nil, fmt.Errorf("render quote template: %w", err)
	}

	footerHTML, err := renderTemplate("templates/footer.html", footer)
	if err != nil {
		return nil, fmt.Errorf("render footer template: %w", err)
	}

	// ── Convert cover HTML → PDF (full-bleed, no footer) ────────────────
	coverPDF, err := gotenbergClient.ConvertHTML(ctx, coverHTML, CoverPageOpts())
	if err != nil {
		return nil, fmt.Errorf("convert cover to PDF: %w", err)
	}

	// ── Convert quote HTML → PDF (with margins + footer) ────────────────
	contentOpts := DefaultContentOpts()
	contentOpts.FooterHTML = footerHTML
	contentOpts.WaitDelay = "1s"
	contentPDF, err := gotenbergClient.ConvertHTML(ctx, quoteHTML, contentOpts)
	if err != nil {
		return nil, fmt.Errorf("convert content to PDF: %w", err)
	}

	// ── Merge cover + content into one document ─────────────────────────
	merged, err := gotenbergClient.MergePDFs(ctx, map[string][]byte{
		"01_cover.pdf":   coverPDF,
		"02_content.pdf": contentPDF,
	})
	if err != nil {
		return nil, fmt.Errorf("merge PDFs: %w", err)
	}

	return merged, nil
}

// ── View model builders ─────────────────────────────────────────────────

func buildCoverVM(data QuotePDFData, logoB64, logoMime string) coverViewModel {
	vm := coverViewModel{
		LogoBase64:         logoB64,
		LogoMimeType:       logoMime,
		OrganizationName:   data.OrganizationName,
		CustomerName:       data.CustomerName,
		QuoteNumber:        data.QuoteNumber,
		CreatedAtFormatted: data.CreatedAt.Format("02-01-2006"),
		OrgAddressLine1:    data.OrgAddressLine1,
		OrgPostalCode:      data.OrgPostalCode,
		OrgCity:            data.OrgCity,
		OrgPhone:           data.OrgPhone,
		OrgEmail:           data.OrgEmail,
	}
	if data.ValidUntil != nil {
		vm.ValidUntilFormatted = data.ValidUntil.Format("02-01-2006")
	}
	vm.QuoteSequenceNumber = extractSequenceNumber(data.QuoteNumber)
	return vm
}

func buildQuoteVM(data QuotePDFData, logoB64, logoMime string) quoteViewModel {
	vm := quoteViewModel{
		LogoBase64:         logoB64,
		LogoMimeType:       logoMime,
		OrganizationName:   data.OrganizationName,
		CustomerName:       data.CustomerName,
		QuoteNumber:        data.QuoteNumber,
		CreatedAtFormatted: data.CreatedAt.Format("02-01-2006"),
		Status:             data.Status,
		StatusLabel:        translateStatus(data.Status),
		StatusClass:        statusCSSClass(data.Status),
		OrgAddressLine1:    data.OrgAddressLine1,
		OrgAddressLine2:    data.OrgAddressLine2,
		OrgPostalCode:      data.OrgPostalCode,
		OrgCity:            data.OrgCity,
		OrgEmail:           data.OrgEmail,
		OrgPhone:           data.OrgPhone,
		OrgKvkNumber:       data.OrgKvkNumber,
		OrgVatNumber:       data.OrgVatNumber,
		SubtotalFormatted:  formatCurrency(data.SubtotalCents),
		HasDiscount:        data.DiscountAmount > 0,
		DiscountFormatted:  formatCurrency(data.DiscountAmount),
		TotalFormatted:     formatCurrency(data.TotalCents),
	}
	if data.ValidUntil != nil {
		vm.ValidUntilFormatted = data.ValidUntil.Format("02-01-2006")
	}
	if data.AcceptedAt != nil {
		vm.AcceptedAtFormatted = data.AcceptedAt.Format("02-01-2006 15:04")
	}
	if data.Notes != nil && *data.Notes != "" {
		vm.Notes = *data.Notes
	}

	// Signature
	if data.SignatureName != nil && data.AcceptedAt != nil {
		vm.HasSignature = true
		vm.SignatureName = *data.SignatureName
		if len(data.SignatureImage) > 0 {
			vm.SignatureBase64 = base64.StdEncoding.EncodeToString(data.SignatureImage)
		}
	}

	// Items
	vm.Items = make([]itemViewModel, len(data.Items))
	for i, it := range data.Items {
		vm.Items[i] = itemViewModel{
			Description:        it.Description,
			Quantity:           it.Quantity,
			UnitPriceFormatted: formatCurrency(it.UnitPriceCents),
			VatPctFormatted:    fmt.Sprintf("%.0f%%", float64(it.TaxRateBps)/100.0),
			LineTotalFormatted: formatCurrency(it.LineTotalCents),
			IsOptional:         it.IsOptional,
			IsSelected:         it.IsSelected,
		}
	}

	// VAT breakdown
	vm.VatBreakdown = make([]vatLineViewModel, len(data.VatBreakdown))
	for i, vat := range data.VatBreakdown {
		vm.VatBreakdown[i] = vatLineViewModel{
			PctFormatted:    fmt.Sprintf("%.0f%%", float64(vat.RateBps)/100.0),
			AmountFormatted: formatCurrency(vat.AmountCents),
		}
	}

	return vm
}

func buildFooterVM(data QuotePDFData) footerViewModel {
	parts := []string{data.OrganizationName}
	if data.OrgKvkNumber != "" {
		parts = append(parts, "KVK: "+data.OrgKvkNumber)
	}
	if data.OrgVatNumber != "" {
		parts = append(parts, "BTW: "+data.OrgVatNumber)
	}
	if data.OrgPhone != "" {
		parts = append(parts, "Tel: "+data.OrgPhone)
	}
	if data.OrgEmail != "" {
		parts = append(parts, data.OrgEmail)
	}
	return footerViewModel{
		FooterText: strings.Join(parts, "  ·  "),
	}
}

// ── Template rendering ──────────────────────────────────────────────────

func renderTemplate(name string, data any) ([]byte, error) {
	raw, err := templateFS.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read embedded template %s: %w", name, err)
	}

	tmpl, err := template.New(name).Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

// ── Helpers ─────────────────────────────────────────────────────────────

func encodeLogoBase64(logo []byte) (string, string) {
	if len(logo) == 0 {
		return "", ""
	}
	mime := http.DetectContentType(logo)
	// Normalise to common image types
	switch {
	case strings.Contains(mime, "jpeg"):
		mime = "image/jpeg"
	case strings.Contains(mime, "png"):
		mime = "image/png"
	case strings.Contains(mime, "gif"):
		mime = "image/gif"
	case strings.Contains(mime, "svg"):
		mime = "image/svg+xml"
	default:
		mime = "image/png"
	}
	return base64.StdEncoding.EncodeToString(logo), mime
}

var seqNumberRe = regexp.MustCompile(`(\d+)$`)

// extractSequenceNumber pulls the trailing numeric part from a quote number.
// e.g. "OFF-2026-0042" → "42", "Q-001" → "01"
func extractSequenceNumber(qn string) string {
	m := seqNumberRe.FindString(qn)
	if m == "" {
		return "01"
	}
	// Strip leading zeroes for display, but keep at least two digits
	n, err := strconv.Atoi(m)
	if err != nil {
		return m
	}
	return fmt.Sprintf("%02d", n)
}

func statusCSSClass(status string) string {
	switch status {
	case "Accepted":
		return "status-accepted"
	case "Rejected":
		return "status-rejected"
	case "Sent":
		return "status-sent"
	default:
		return "status-default"
	}
}

func translateStatus(status string) string {
	switch status {
	case "Draft":
		return "Concept"
	case "Sent":
		return "Verzonden"
	case "Accepted":
		return "Geaccepteerd"
	case "Rejected":
		return "Afgewezen"
	case "Expired":
		return "Verlopen"
	default:
		return status
	}
}

func formatCurrency(cents int64) string {
	return fmt.Sprintf("€ %.2f", float64(cents)/100.0)
}
