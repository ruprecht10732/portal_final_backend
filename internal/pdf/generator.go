// Package pdf provides quote PDF generation using Gotenberg (Chromium HTML→PDF).
// The generated document meets Dutch legal requirements for commercial quotes
// (offerte) and includes organization branding, KVK/BTW numbers, addresses,
// line-item details, and a signature block.
package pdf

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
	"time"

	"portal_final_backend/internal/quotes/transport"

	gotenberg "github.com/starwalkn/gotenberg-go-client/v8"
	"github.com/starwalkn/gotenberg-go-client/v8/document"
)

//go:embed templates/*.html
var templateFS embed.FS

// gotenbergClient is the package-level Gotenberg API client.
var gotenbergClient *gotenberg.Client

// Init initialises the Gotenberg client. Must be called once at startup.
func Init(gotenbergURL string) error {
	c, err := gotenberg.NewClient(gotenbergURL, &http.Client{Timeout: 60 * time.Second})
	if err != nil {
		return fmt.Errorf("create gotenberg client: %w", err)
	}
	gotenbergClient = c
	return nil
}

// ─── Public Data Types ───────────────────────────────────────────────────────

// QuotePDFData contains all information needed to render a quote PDF.
type QuotePDFData struct {
	QuoteNumber      string
	OrganizationName string
	CustomerName     string
	Status           string
	PricingMode      string
	ValidUntil       *time.Time
	CreatedAt        time.Time
	Notes            *string
	SignatureName    *string
	SignatureImage   []byte // raw PNG bytes of the drawn signature
	AcceptedAt       *time.Time

	// Line items (already calculated by the service layer)
	Items []transport.PublicQuoteItemResponse

	// Totals
	SubtotalCents  int64
	DiscountAmount int64
	TaxTotalCents  int64
	TotalCents     int64
	VatBreakdown   []transport.VatBreakdown

	// Organization branding
	OrgLogo     []byte // raw logo image bytes
	OrgLogoMime string // MIME type for the logo, e.g. "image/png"

	// Organization details
	OrgEmail        string
	OrgPhone        string
	OrgVatNumber    string
	OrgKvkNumber    string
	OrgAddressLine1 string
	OrgAddressLine2 string
	OrgPostalCode   string
	OrgCity         string
	OrgCountry      string
}

// ─── Template View Models ────────────────────────────────────────────────────

type templateData struct {
	LogoBase64          string
	LogoMimeType        string
	OrganizationName    string
	QuoteNumber         string
	CustomerName        string
	Status              string
	StatusClass         string
	StatusLabel         string
	CreatedAtFormatted  string
	ValidUntilFormatted string
	AcceptedAtFormatted string
	HasSignature        bool
	SignatureName       string
	SignatureBase64     string
	Notes               string
	HasDiscount         bool
	SubtotalFormatted   string
	DiscountFormatted   string
	TotalFormatted      string
	Items               []templateItem
	VatBreakdown        []templateVatLine

	// Organization address details
	OrgEmail        string
	OrgPhone        string
	OrgVatNumber    string
	OrgKvkNumber    string
	OrgAddressLine1 string
	OrgAddressLine2 string
	OrgPostalCode   string
	OrgCity         string
}

type templateItem struct {
	Description        string
	Quantity           string
	UnitPriceFormatted string
	VatPctFormatted    string
	LineTotalFormatted string
	IsOptional         bool
	IsSelected         bool
}

type templateVatLine struct {
	PctFormatted    string
	AmountFormatted string
}

type footerData struct {
	FooterText string
}

// ─── PDF Generation ──────────────────────────────────────────────────────────

// GenerateQuotePDF renders the quote as a professional PDF via Gotenberg.
func GenerateQuotePDF(data QuotePDFData) ([]byte, error) {
	if gotenbergClient == nil {
		return nil, fmt.Errorf("gotenberg client not initialized — call pdf.Init first")
	}

	// 1. Build template view models
	td := buildTemplateData(data)
	fd := footerData{FooterText: buildFooterText(data)}

	// 2. Render HTML templates
	mainHTML, err := renderTemplate("templates/quote.html", td)
	if err != nil {
		return nil, fmt.Errorf("render quote template: %w", err)
	}

	footerHTML, err := renderTemplate("templates/footer.html", fd)
	if err != nil {
		return nil, fmt.Errorf("render footer template: %w", err)
	}

	// 3. Create Gotenberg documents from rendered HTML bytes
	indexDoc, err := document.FromBytes("index.html", mainHTML)
	if err != nil {
		return nil, fmt.Errorf("create index document: %w", err)
	}

	footerDoc, err := document.FromBytes("footer.html", footerHTML)
	if err != nil {
		return nil, fmt.Errorf("create footer document: %w", err)
	}

	// 4. Build the HTML-to-PDF conversion request
	req := gotenberg.NewHTMLRequest(indexDoc)
	req.PaperSize(gotenberg.A4)
	req.Margins(gotenberg.PageMargins{
		Top:    0.6,
		Bottom: 0.8,
		Left:   0.6,
		Right:  0.6,
		Unit:   gotenberg.IN,
	})
	req.PrintBackground()
	req.Footer(footerDoc)

	// 5. Send to Gotenberg and read response
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := gotenbergClient.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gotenberg send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gotenberg returned status %d: %s", resp.StatusCode, string(body))
	}

	pdfBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read gotenberg response: %w", err)
	}

	return pdfBytes, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// renderTemplate parses an embedded template and executes it with the given data.
func renderTemplate(name string, data any) ([]byte, error) {
	tmplBytes, err := templateFS.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read embedded template %s: %w", name, err)
	}

	tmpl, err := template.New(name).Parse(string(tmplBytes))
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template %s: %w", name, err)
	}

	return buf.Bytes(), nil
}

// buildTemplateData maps QuotePDFData to the flat template view model.
func buildTemplateData(d QuotePDFData) templateData {
	td := templateData{
		OrganizationName:   d.OrganizationName,
		QuoteNumber:        d.QuoteNumber,
		CustomerName:       d.CustomerName,
		Status:             d.Status,
		StatusClass:        statusClass(d.Status),
		StatusLabel:        translateStatus(d.Status),
		CreatedAtFormatted: d.CreatedAt.Format("02-01-2006"),
		SubtotalFormatted:  formatCurrency(d.SubtotalCents),
		TotalFormatted:     formatCurrency(d.TotalCents),
		HasDiscount:        d.DiscountAmount > 0,
		DiscountFormatted:  formatCurrency(d.DiscountAmount),

		OrgEmail:        d.OrgEmail,
		OrgPhone:        d.OrgPhone,
		OrgVatNumber:    d.OrgVatNumber,
		OrgKvkNumber:    d.OrgKvkNumber,
		OrgAddressLine1: d.OrgAddressLine1,
		OrgAddressLine2: d.OrgAddressLine2,
		OrgPostalCode:   d.OrgPostalCode,
		OrgCity:         d.OrgCity,
	}

	// Logo
	if len(d.OrgLogo) > 0 && d.OrgLogoMime != "" {
		td.LogoBase64 = base64.StdEncoding.EncodeToString(d.OrgLogo)
		td.LogoMimeType = d.OrgLogoMime
	}

	// Dates
	if d.ValidUntil != nil {
		td.ValidUntilFormatted = d.ValidUntil.Format("02-01-2006")
	}
	if d.AcceptedAt != nil {
		td.AcceptedAtFormatted = d.AcceptedAt.Format("02-01-2006")
	}

	// Notes
	if d.Notes != nil && *d.Notes != "" {
		td.Notes = *d.Notes
	}

	// Signature
	if d.SignatureName != nil && *d.SignatureName != "" {
		td.HasSignature = true
		td.SignatureName = *d.SignatureName
	}
	if len(d.SignatureImage) > 0 {
		td.HasSignature = true
		td.SignatureBase64 = base64.StdEncoding.EncodeToString(d.SignatureImage)
	}

	// Items
	td.Items = make([]templateItem, len(d.Items))
	for i, it := range d.Items {
		td.Items[i] = templateItem{
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
	td.VatBreakdown = make([]templateVatLine, len(d.VatBreakdown))
	for i, vb := range d.VatBreakdown {
		td.VatBreakdown[i] = templateVatLine{
			PctFormatted:    fmt.Sprintf("%.0f%%", float64(vb.RateBps)/100.0),
			AmountFormatted: formatCurrency(vb.AmountCents),
		}
	}

	return td
}

// buildFooterText returns the single-line footer text with org info.
func buildFooterText(d QuotePDFData) string {
	parts := []string{d.OrganizationName}
	if d.OrgAddressLine1 != "" {
		parts = append(parts, d.OrgAddressLine1)
	}
	loc := joinParts(", ", d.OrgPostalCode, d.OrgCity)
	if loc != "" {
		parts = append(parts, loc)
	}
	if d.OrgKvkNumber != "" {
		parts = append(parts, "KVK: "+d.OrgKvkNumber)
	}
	if d.OrgVatNumber != "" {
		parts = append(parts, "BTW: "+d.OrgVatNumber)
	}
	return strings.Join(parts, "  |  ")
}

// statusClass returns the CSS class suffix for the quote status.
func statusClass(status string) string {
	switch strings.ToLower(status) {
	case "accepted":
		return "status-accepted"
	case "rejected":
		return "status-rejected"
	case "sent":
		return "status-sent"
	default:
		return "status-default"
	}
}

// translateStatus returns the Dutch label for a quote status.
func translateStatus(status string) string {
	switch strings.ToLower(status) {
	case "draft":
		return "Concept"
	case "sent":
		return "Verzonden"
	case "accepted":
		return "Geaccepteerd"
	case "rejected":
		return "Afgewezen"
	case "expired":
		return "Verlopen"
	default:
		return status
	}
}

// formatCurrency formats cents as a Euro string: "€ 1.234,56".
func formatCurrency(cents int64) string {
	negative := cents < 0
	if negative {
		cents = -cents
	}

	euros := cents / 100
	remainder := cents % 100

	// Format with thousands separator (dot in NL)
	euroStr := formatThousands(euros)

	result := fmt.Sprintf("€ %s,%02d", euroStr, remainder)
	if negative {
		result = "-" + result
	}
	return result
}

// formatThousands formats an integer with dots as thousands separators (Dutch convention).
func formatThousands(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}

	s := fmt.Sprintf("%d", n)
	var buf strings.Builder
	offset := len(s) % 3
	if offset > 0 {
		buf.WriteString(s[:offset])
	}
	for i := offset; i < len(s); i += 3 {
		if buf.Len() > 0 {
			buf.WriteByte('.')
		}
		buf.WriteString(s[i : i+3])
	}
	return buf.String()
}

// joinParts joins non-empty strings with a separator.
func joinParts(sep string, parts ...string) string {
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, sep)
}
