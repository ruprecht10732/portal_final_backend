// Package pdf provides quote PDF generation using maroto/v2.
// The generated document meets Dutch legal requirements for commercial quotes
// (offerte) and includes organization branding, KVK/BTW numbers, addresses,
// line-item details, and a signature block.
package pdf

import (
	"fmt"
	"time"

	"portal_final_backend/internal/quotes/transport"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/image"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/border"
	"github.com/johnfercher/maroto/v2/pkg/consts/extension"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

// ── Colour palette ──────────────────────────────────────────────────────

var (
	colorPrimary    = &props.Color{Red: 17, Green: 24, Blue: 39}    // near-black
	colorSecondary  = &props.Color{Red: 107, Green: 114, Blue: 128} // gray-500
	colorAccent     = &props.Color{Red: 37, Green: 99, Blue: 235}   // blue-600
	colorTableHead  = &props.Color{Red: 241, Green: 245, Blue: 249} // slate-100
	colorTableAlt   = &props.Color{Red: 249, Green: 250, Blue: 251} // gray-50
	colorGreenLight = &props.Color{Red: 220, Green: 252, Blue: 231} // green-100
	colorGreen      = &props.Color{Red: 22, Green: 163, Blue: 74}   // green-600
	colorRed        = &props.Color{Red: 220, Green: 38, Blue: 38}   // red-600
	colorBorder     = &props.Color{Red: 226, Green: 232, Blue: 240} // slate-200
)

// ── Data struct ─────────────────────────────────────────────────────────

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
	OrgLogo          []byte         // raw image bytes (PNG or JPEG)
	OrgLogoExt       extension.Type // maroto extension type

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

// GenerateQuotePDF creates a professional PDF document for the given quote data.
func GenerateQuotePDF(data QuotePDFData) ([]byte, error) {
	cfg := config.NewBuilder().
		WithLeftMargin(15).
		WithTopMargin(12).
		WithRightMargin(15).
		Build()

	m := maroto.New(cfg)

	// ── Registered footer (repeats on every page) ───────────────────────
	if err := m.RegisterFooter(buildFooter(data)); err != nil {
		return nil, fmt.Errorf("register footer: %w", err)
	}

	// ── Page content ────────────────────────────────────────────────────

	// 1. Header: logo + OFFERTE title
	m.AddRows(buildHeader(data)...)

	// 2. Separator line
	m.AddRows(row.New(1).WithStyle(&props.Cell{
		BorderType:  border.Bottom,
		BorderColor: colorBorder,
	}))
	m.AddRows(row.New(6)) // spacer

	// 3. From / To + Quote meta
	m.AddRows(buildAddressBlock(data)...)
	m.AddRows(row.New(6)) // spacer

	// 4. Status banner (accepted/rejected)
	if data.Status == "Accepted" || data.Status == "Rejected" {
		m.AddRows(buildStatusBanner(data))
		m.AddRows(row.New(4))
	}

	// 5. Line items table
	m.AddRows(buildItemsTable(data)...)
	m.AddRows(row.New(4))

	// 6. Totals block
	m.AddRows(buildTotalsBlock(data)...)

	// 7. Notes
	if data.Notes != nil && *data.Notes != "" {
		m.AddRows(row.New(6))
		m.AddRows(buildNotesBlock(*data.Notes)...)
	}

	// 8. Legal terms
	m.AddRows(row.New(8))
	m.AddRows(buildLegalTerms()...)

	// 9. Signature (if accepted)
	if data.SignatureName != nil && data.AcceptedAt != nil {
		m.AddRows(row.New(8))
		m.AddRows(buildSignatureBlock(data)...)
	}

	doc, err := m.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate PDF: %w", err)
	}

	return doc.GetBytes(), nil
}

// ── Header ──────────────────────────────────────────────────────────────

func buildHeader(data QuotePDFData) []core.Row {
	var rows []core.Row

	logoCol := col.New(4)
	if len(data.OrgLogo) > 0 {
		logoCol.Add(
			image.NewFromBytes(data.OrgLogo, data.OrgLogoExt, props.Rect{
				Percent: 85,
				Center:  false,
			}),
		)
	} else {
		logoCol.Add(
			text.New(data.OrganizationName, props.Text{
				Size:  14,
				Style: fontstyle.Bold,
				Color: colorPrimary,
				Top:   4,
			}),
		)
	}

	titleCol := col.New(8).Add(
		text.New("OFFERTE", props.Text{
			Size:  24,
			Style: fontstyle.Bold,
			Align: align.Right,
			Color: colorAccent,
		}),
		text.New(data.QuoteNumber, props.Text{
			Size:  11,
			Align: align.Right,
			Color: colorSecondary,
			Top:   12,
		}),
	)

	rows = append(rows, row.New(20).Add(logoCol, titleCol))

	return rows
}

// ── Address block ───────────────────────────────────────────────────────

func buildAddressBlock(data QuotePDFData) []core.Row {
	var rows []core.Row

	// Row 1: labels
	rows = append(rows, row.New(5).Add(
		col.New(6).Add(text.New("VAN", props.Text{Size: 7, Style: fontstyle.Bold, Color: colorAccent})),
		col.New(3).Add(text.New("AAN", props.Text{Size: 7, Style: fontstyle.Bold, Color: colorAccent})),
		col.New(3).Add(text.New("OFFERTE DETAILS", props.Text{Size: 7, Style: fontstyle.Bold, Color: colorAccent, Align: align.Right})),
	))

	// Row 2: org name + customer name + date
	dateStr := data.CreatedAt.Format("02-01-2006")
	rows = append(rows, row.New(5).Add(
		col.New(6).Add(text.New(data.OrganizationName, props.Text{Size: 9, Style: fontstyle.Bold, Color: colorPrimary})),
		col.New(3).Add(text.New(data.CustomerName, props.Text{Size: 9, Style: fontstyle.Bold, Color: colorPrimary})),
		col.New(3).Add(text.New("Datum: "+dateStr, props.Text{Size: 8, Color: colorSecondary, Align: align.Right})),
	))

	// Row 3: address line 1 + empty + validity
	validityStr := ""
	if data.ValidUntil != nil {
		validityStr = "Geldig tot: " + data.ValidUntil.Format("02-01-2006")
	}
	addr1 := data.OrgAddressLine1
	if data.OrgAddressLine2 != "" {
		addr1 += ", " + data.OrgAddressLine2
	}

	rows = append(rows, row.New(5).Add(
		col.New(6).Add(text.New(addr1, props.Text{Size: 8, Color: colorSecondary})),
		col.New(3),
		col.New(3).Add(text.New(validityStr, props.Text{Size: 8, Color: colorSecondary, Align: align.Right})),
	))

	// Row 4: postcode + city + status
	postalCity := ""
	if data.OrgPostalCode != "" || data.OrgCity != "" {
		postalCity = data.OrgPostalCode
		if postalCity != "" && data.OrgCity != "" {
			postalCity += " "
		}
		postalCity += data.OrgCity
	}
	statusLabel := translateStatus(data.Status)
	rows = append(rows, row.New(5).Add(
		col.New(6).Add(text.New(postalCity, props.Text{Size: 8, Color: colorSecondary})),
		col.New(3),
		col.New(3).Add(text.New("Status: "+statusLabel, props.Text{Size: 8, Style: fontstyle.Bold, Color: statusColor(data.Status), Align: align.Right})),
	))

	// Row 5: contact info
	contactParts := []string{}
	if data.OrgEmail != "" {
		contactParts = append(contactParts, data.OrgEmail)
	}
	if data.OrgPhone != "" {
		contactParts = append(contactParts, data.OrgPhone)
	}
	contactStr := joinParts(contactParts, "  |  ")
	rows = append(rows, row.New(5).Add(
		col.New(12).Add(text.New(contactStr, props.Text{Size: 8, Color: colorSecondary})),
	))

	// Row 6: KVK / BTW
	legalParts := []string{}
	if data.OrgKvkNumber != "" {
		legalParts = append(legalParts, "KVK: "+data.OrgKvkNumber)
	}
	if data.OrgVatNumber != "" {
		legalParts = append(legalParts, "BTW: "+data.OrgVatNumber)
	}
	if len(legalParts) > 0 {
		rows = append(rows, row.New(5).Add(
			col.New(12).Add(text.New(joinParts(legalParts, "  |  "), props.Text{Size: 8, Color: colorSecondary})),
		))
	}

	return rows
}

// ── Status banner ───────────────────────────────────────────────────────

func buildStatusBanner(data QuotePDFData) core.Row {
	if data.Status == "Accepted" {
		label := "Offerte geaccepteerd"
		if data.AcceptedAt != nil {
			label += " op " + data.AcceptedAt.Format("02-01-2006 15:04")
		}
		return row.New(8).Add(
			col.New(12).Add(text.New(label, props.Text{
				Size:  9,
				Style: fontstyle.Bold,
				Color: colorGreen,
				Top:   2,
			})),
		).WithStyle(&props.Cell{BackgroundColor: colorGreenLight})
	}

	// Rejected
	return row.New(8).Add(
		col.New(12).Add(text.New("Offerte afgewezen", props.Text{
			Size:  9,
			Style: fontstyle.Bold,
			Color: colorRed,
			Top:   2,
		})),
	).WithStyle(&props.Cell{BackgroundColor: &props.Color{Red: 254, Green: 226, Blue: 226}})
}

// ── Line items table ────────────────────────────────────────────────────

func buildItemsTable(data QuotePDFData) []core.Row {
	var rows []core.Row

	// Section title
	rows = append(rows, row.New(7).Add(
		col.New(12).Add(text.New("ONDERDELEN", props.Text{
			Size:  8,
			Style: fontstyle.Bold,
			Color: colorAccent,
		})),
	))

	// Table header
	headerStyle := props.Text{Size: 7.5, Style: fontstyle.Bold, Color: colorPrimary, Top: 1.5}
	headerStyleRight := props.Text{Size: 7.5, Style: fontstyle.Bold, Color: colorPrimary, Align: align.Right, Top: 1.5}

	rows = append(rows, row.New(7).Add(
		col.New(5).Add(text.New("Omschrijving", headerStyle)),
		col.New(1).Add(text.New("Aantal", headerStyle)),
		col.New(2).Add(text.New("Stuksprijs", headerStyleRight)),
		col.New(1).Add(text.New("BTW", headerStyleRight)),
		col.New(3).Add(text.New("Bedrag", headerStyleRight)),
	).WithStyle(&props.Cell{
		BackgroundColor: colorTableHead,
		BorderType:      border.Bottom,
		BorderColor:     colorBorder,
	}))

	// Item rows
	for i, item := range data.Items {
		rows = append(rows, buildItemRow(item, i))
	}

	return rows
}

func buildItemRow(item transport.PublicQuoteItemResponse, idx int) core.Row {
	desc := item.Description
	if item.IsOptional {
		if item.IsSelected {
			desc += "  (optioneel)"
		} else {
			desc += "  (optioneel - niet geselecteerd)"
		}
	}

	textColor := colorPrimary
	if item.IsOptional && !item.IsSelected {
		textColor = &props.Color{Red: 160, Green: 160, Blue: 160}
	}

	normalStyle := props.Text{Size: 8, Color: textColor, Top: 1}
	rightStyle := props.Text{Size: 8, Color: textColor, Align: align.Right, Top: 1}

	vatPct := fmt.Sprintf("%.0f%%", float64(item.TaxRateBps)/100.0)

	r := row.New(7).Add(
		col.New(5).Add(text.New(desc, normalStyle)),
		col.New(1).Add(text.New(item.Quantity, normalStyle)),
		col.New(2).Add(text.New(formatCurrency(item.UnitPriceCents), rightStyle)),
		col.New(1).Add(text.New(vatPct, rightStyle)),
		col.New(3).Add(text.New(formatCurrency(item.LineTotalCents), rightStyle)),
	)

	// Alternating row background
	if idx%2 == 0 {
		r.WithStyle(&props.Cell{BackgroundColor: colorTableAlt})
	}

	return r
}

// ── Totals block ────────────────────────────────────────────────────────

func buildTotalsBlock(data QuotePDFData) []core.Row {
	var rows []core.Row

	// Thin separator
	rows = append(rows, row.New(1).WithStyle(&props.Cell{
		BorderType:  border.Bottom,
		BorderColor: colorBorder,
	}))
	rows = append(rows, row.New(3)) // spacer

	labelStyle := props.Text{Size: 9, Color: colorSecondary, Align: align.Right}
	valueStyle := props.Text{Size: 9, Color: colorPrimary, Align: align.Right}

	// Subtotal
	rows = append(rows, row.New(6).Add(
		col.New(9).Add(text.New("Subtotaal", labelStyle)),
		col.New(3).Add(text.New(formatCurrency(data.SubtotalCents), valueStyle)),
	))

	// Discount
	if data.DiscountAmount > 0 {
		rows = append(rows, row.New(6).Add(
			col.New(9).Add(text.New("Korting", labelStyle)),
			col.New(3).Add(text.New("-"+formatCurrency(data.DiscountAmount), props.Text{
				Size:  9,
				Color: colorGreen,
				Align: align.Right,
			})),
		))
	}

	// VAT breakdown per rate
	for _, vat := range data.VatBreakdown {
		pct := fmt.Sprintf("BTW %.0f%%", float64(vat.RateBps)/100.0)
		rows = append(rows, row.New(6).Add(
			col.New(9).Add(text.New(pct, labelStyle)),
			col.New(3).Add(text.New(formatCurrency(vat.AmountCents), valueStyle)),
		))
	}

	// Total amount — highlighted
	rows = append(rows, row.New(2)) // spacer
	rows = append(rows, row.New(10).Add(
		col.New(9).Add(text.New("TOTAAL", props.Text{
			Size:  12,
			Style: fontstyle.Bold,
			Color: colorPrimary,
			Align: align.Right,
			Top:   2,
		})),
		col.New(3).Add(text.New(formatCurrency(data.TotalCents), props.Text{
			Size:  12,
			Style: fontstyle.Bold,
			Color: colorPrimary,
			Align: align.Right,
			Top:   2,
		})),
	).WithStyle(&props.Cell{
		BackgroundColor: colorTableHead,
		BorderType:      border.Top | border.Bottom,
		BorderColor:     colorBorder,
	}))

	return rows
}

// ── Notes ───────────────────────────────────────────────────────────────

func buildNotesBlock(notes string) []core.Row {
	return []core.Row{
		row.New(5).Add(
			col.New(12).Add(text.New("OPMERKINGEN", props.Text{
				Size:  8,
				Style: fontstyle.Bold,
				Color: colorAccent,
			})),
		),
		row.New(12).Add(
			col.New(12).Add(text.New(notes, props.Text{
				Size:  8,
				Color: colorSecondary,
				Top:   1,
			})),
		),
	}
}

// ── Legal terms ─────────────────────────────────────────────────────────

func buildLegalTerms() []core.Row {
	return []core.Row{
		row.New(1).WithStyle(&props.Cell{
			BorderType:  border.Bottom,
			BorderColor: colorBorder,
		}),
		row.New(3),
		row.New(5).Add(
			col.New(12).Add(text.New("VOORWAARDEN", props.Text{
				Size:  7,
				Style: fontstyle.Bold,
				Color: colorAccent,
			})),
		),
		row.New(4).Add(
			col.New(12).Add(text.New(
				"1.  Deze offerte is vrijblijvend en onder voorbehoud van tussentijdse wijzigingen.",
				props.Text{Size: 7, Color: colorSecondary},
			)),
		),
		row.New(4).Add(
			col.New(12).Add(text.New(
				"2.  Betaling dient te geschieden binnen 14 dagen na factuurdatum, tenzij anders overeengekomen.",
				props.Text{Size: 7, Color: colorSecondary},
			)),
		),
		row.New(4).Add(
			col.New(12).Add(text.New(
				"3.  Op al onze offertes, overeenkomsten en leveringen zijn onze algemene voorwaarden van toepassing.",
				props.Text{Size: 7, Color: colorSecondary},
			)),
		),
		row.New(4).Add(
			col.New(12).Add(text.New(
				"4.  Alle genoemde bedragen zijn in euro's. BTW is gespecificeerd conform Nederlandse wetgeving.",
				props.Text{Size: 7, Color: colorSecondary},
			)),
		),
	}
}

// ── Signature block ─────────────────────────────────────────────────────

func buildSignatureBlock(data QuotePDFData) []core.Row {
	var rows []core.Row

	rows = append(rows, row.New(1).WithStyle(&props.Cell{
		BorderType:  border.Bottom,
		BorderColor: colorBorder,
	}))
	rows = append(rows, row.New(3))

	rows = append(rows, row.New(5).Add(
		col.New(12).Add(text.New("AKKOORDVERKLARING", props.Text{
			Size:  8,
			Style: fontstyle.Bold,
			Color: colorAccent,
		})),
	))

	rows = append(rows, row.New(6).Add(
		col.New(6).Add(text.New(fmt.Sprintf("Geaccepteerd door: %s", *data.SignatureName), props.Text{
			Size:  9,
			Style: fontstyle.Bold,
			Color: colorPrimary,
		})),
		col.New(6).Add(text.New(fmt.Sprintf("Datum: %s", data.AcceptedAt.Format("02-01-2006 15:04")), props.Text{
			Size:  9,
			Color: colorSecondary,
			Align: align.Right,
		})),
	))

	// Render the drawn signature image if available
	if len(data.SignatureImage) > 0 {
		rows = append(rows,
			row.New(3), // spacer
			row.New(25).Add(
				col.New(2).Add(text.New("Handtekening:", props.Text{
					Size:  8,
					Color: colorSecondary,
					Top:   8,
				})),
				col.New(5).Add(
					image.NewFromBytes(data.SignatureImage, extension.Png, props.Rect{
						Center:  false,
						Percent: 90,
					}),
				),
				col.New(5),
			),
		)
	}

	return rows
}

// ── Footer (registered — repeats on every page) ─────────────────────────

func buildFooter(data QuotePDFData) core.Row {
	// Build a compact footer line: Org Name | KVK: ... | BTW: ... | tel: ... | email: ...
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
	footerText := joinParts(parts, "  ·  ")

	return row.New(10).Add(
		col.New(12).Add(
			text.New(footerText, props.Text{
				Size:  6.5,
				Color: colorSecondary,
				Align: align.Center,
				Top:   4,
			}),
		),
	).WithStyle(&props.Cell{
		BorderType:  border.Top,
		BorderColor: colorBorder,
	})
}

// ── Helpers ─────────────────────────────────────────────────────────────

func statusColor(status string) *props.Color {
	switch status {
	case "Accepted":
		return colorGreen
	case "Rejected":
		return colorRed
	case "Sent":
		return colorAccent
	default:
		return colorSecondary
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

func joinParts(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if p == "" {
			continue
		}
		if result != "" && i > 0 {
			result += sep
		}
		result += p
	}
	return result
}
