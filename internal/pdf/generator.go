// Package pdf provides quote PDF generation using maroto/v2.
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
	"github.com/johnfercher/maroto/v2/pkg/consts/extension"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

// QuotePDFData holds all data needed to generate a quote PDF.
type QuotePDFData struct {
	QuoteNumber      string
	OrganizationName string
	CustomerName     string
	Status           string
	PricingMode      string
	ValidUntil       *time.Time
	Notes            *string
	SignatureName    *string
	SignatureImage   []byte // raw PNG bytes of the drawn signature
	AcceptedAt       *time.Time
	Items            []transport.PublicQuoteItemResponse
	SubtotalCents    int64
	DiscountAmount   int64
	TaxTotalCents    int64
	TotalCents       int64
	VatBreakdown     []transport.VatBreakdown
}

// GenerateQuotePDF creates a PDF document for the given quote data, returning the bytes.
func GenerateQuotePDF(data QuotePDFData) ([]byte, error) {
	cfg := config.NewBuilder().
		WithPageNumber(props.PageNumber{
			Pattern: "Pagina {current} van {total}",
			Place:   props.Bottom,
			Size:    8,
			Color:   &props.Color{Red: 150, Green: 150, Blue: 150},
		}).
		WithLeftMargin(15).
		WithTopMargin(15).
		WithRightMargin(15).
		Build()

	m := maroto.New(cfg)

	// Header
	m.AddRows(headerRow(data))
	m.AddRows(row.New(6)) // spacer

	// Meta info
	m.AddRows(metaRows(data)...)
	m.AddRows(row.New(6))

	// Line items table header
	m.AddRows(lineItemsHeader())
	for _, item := range data.Items {
		m.AddRows(lineItemRow(item))
	}

	// Separator
	m.AddRows(row.New(4))
	m.AddRows(separatorRow())

	// Totals
	m.AddRows(row.New(4))
	m.AddRows(totalsRows(data)...)

	// VAT breakdown
	if len(data.VatBreakdown) > 0 {
		m.AddRows(row.New(6))
		m.AddRows(vatBreakdownRows(data.VatBreakdown)...)
	}

	// Notes
	if data.Notes != nil && *data.Notes != "" {
		m.AddRows(row.New(8))
		m.AddRows(notesRows(*data.Notes)...)
	}

	// Signature block (if accepted)
	if data.SignatureName != nil && data.AcceptedAt != nil {
		m.AddRows(row.New(10))
		m.AddRows(signatureRows(data)...)
	}

	doc, err := m.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate PDF: %w", err)
	}

	return doc.GetBytes(), nil
}

func headerRow(data QuotePDFData) core.Row {
	return row.New(14).Add(
		col.New(8).Add(
			text.New(fmt.Sprintf("OFFERTE %s", data.QuoteNumber), props.Text{
				Size:  16,
				Style: fontstyle.Bold,
				Color: &props.Color{Red: 17, Green: 24, Blue: 39},
			}),
		),
		col.New(4).Add(
			text.New(data.Status, props.Text{
				Size:  12,
				Style: fontstyle.Bold,
				Align: align.Right,
				Color: statusColor(data.Status),
			}),
		),
	)
}

func metaRows(data QuotePDFData) []core.Row {
	rows := []core.Row{
		row.New(6).Add(
			col.New(6).Add(text.New("Organisatie: "+data.OrganizationName, props.Text{Size: 9, Color: grayColor()})),
			col.New(6).Add(text.New("Klant: "+data.CustomerName, props.Text{Size: 9, Color: grayColor(), Align: align.Right})),
		),
	}

	if data.ValidUntil != nil {
		rows = append(rows, row.New(6).Add(
			col.New(12).Add(text.New(
				fmt.Sprintf("Geldig tot: %s", data.ValidUntil.Format("02-01-2006")),
				props.Text{Size: 9, Color: grayColor()},
			)),
		))
	}

	return rows
}

func lineItemsHeader() core.Row {
	headerStyle := props.Text{Size: 8, Style: fontstyle.Bold, Color: grayColor()}
	return row.New(7).Add(
		col.New(5).Add(text.New("Omschrijving", headerStyle)),
		col.New(1).Add(text.New("Aantal", headerStyle)),
		col.New(2).Add(text.New("Prijs", props.Text{Size: 8, Style: fontstyle.Bold, Color: grayColor(), Align: align.Right})),
		col.New(1).Add(text.New("BTW", props.Text{Size: 8, Style: fontstyle.Bold, Color: grayColor(), Align: align.Right})),
		col.New(3).Add(text.New("Totaal", props.Text{Size: 8, Style: fontstyle.Bold, Color: grayColor(), Align: align.Right})),
	)
}

func lineItemRow(item transport.PublicQuoteItemResponse) core.Row {
	desc := item.Description
	if item.IsOptional {
		if item.IsSelected {
			desc = "[v] " + desc + " (optie)"
		} else {
			desc = "[x] " + desc + " (optie - niet geselecteerd)"
		}
	}

	textColor := &props.Color{Red: 17, Green: 24, Blue: 39}
	if item.IsOptional && !item.IsSelected {
		textColor = &props.Color{Red: 160, Green: 160, Blue: 160}
	}

	normalStyle := props.Text{Size: 8, Color: textColor}
	rightStyle := props.Text{Size: 8, Color: textColor, Align: align.Right}

	vatPct := fmt.Sprintf("%.0f%%", float64(item.TaxRateBps)/100.0)

	return row.New(6).Add(
		col.New(5).Add(text.New(desc, normalStyle)),
		col.New(1).Add(text.New(item.Quantity, normalStyle)),
		col.New(2).Add(text.New(formatCents(item.UnitPriceCents), rightStyle)),
		col.New(1).Add(text.New(vatPct, rightStyle)),
		col.New(3).Add(text.New(formatCents(item.LineTotalCents), rightStyle)),
	)
}

func separatorRow() core.Row {
	return row.New(1).Add(
		col.New(12).Add(text.New("────────────────────────────────────────────────────────────────────────", props.Text{
			Size:  4,
			Color: grayColor(),
		})),
	)
}

func totalsRows(data QuotePDFData) []core.Row {
	boldRight := props.Text{Size: 9, Style: fontstyle.Bold, Align: align.Right}
	normalRight := props.Text{Size: 9, Align: align.Right}
	labelStyle := props.Text{Size: 9, Align: align.Right, Color: grayColor()}

	rows := []core.Row{
		row.New(6).Add(
			col.New(9).Add(text.New("Subtotaal", labelStyle)),
			col.New(3).Add(text.New(formatCents(data.SubtotalCents), normalRight)),
		),
	}

	if data.DiscountAmount > 0 {
		rows = append(rows, row.New(6).Add(
			col.New(9).Add(text.New("Korting", labelStyle)),
			col.New(3).Add(text.New("-"+formatCents(data.DiscountAmount), normalRight)),
		))
	}

	rows = append(rows, row.New(6).Add(
		col.New(9).Add(text.New("BTW", labelStyle)),
		col.New(3).Add(text.New(formatCents(data.TaxTotalCents), normalRight)),
	))

	rows = append(rows, row.New(8).Add(
		col.New(9).Add(text.New("TOTAAL", props.Text{Size: 11, Style: fontstyle.Bold, Align: align.Right})),
		col.New(3).Add(text.New(formatCents(data.TotalCents), boldRight)),
	))

	return rows
}

func vatBreakdownRows(breakdown []transport.VatBreakdown) []core.Row {
	rows := []core.Row{
		row.New(6).Add(col.New(12).Add(text.New("BTW Specificatie", props.Text{Size: 8, Style: fontstyle.Bold, Color: grayColor()}))),
	}
	for _, vat := range breakdown {
		pct := fmt.Sprintf("%.0f%%", float64(vat.RateBps)/100.0)
		rows = append(rows, row.New(5).Add(
			col.New(9).Add(text.New(pct, props.Text{Size: 8, Color: grayColor(), Align: align.Right})),
			col.New(3).Add(text.New(formatCents(vat.AmountCents), props.Text{Size: 8, Color: grayColor(), Align: align.Right})),
		))
	}
	return rows
}

func notesRows(notes string) []core.Row {
	return []core.Row{
		row.New(6).Add(
			col.New(12).Add(text.New("Opmerkingen", props.Text{Size: 8, Style: fontstyle.Bold, Color: grayColor()})),
		),
		row.New(10).Add(
			col.New(12).Add(text.New(notes, props.Text{Size: 8, Color: grayColor()})),
		),
	}
}

func signatureRows(data QuotePDFData) []core.Row {
	rows := []core.Row{
		separatorRow(),
		row.New(8).Add(
			col.New(6).Add(text.New(fmt.Sprintf("Geaccepteerd door: %s", *data.SignatureName), props.Text{Size: 9, Style: fontstyle.Bold})),
			col.New(6).Add(text.New(fmt.Sprintf("Datum: %s", data.AcceptedAt.Format("02-01-2006 15:04")), props.Text{Size: 9, Align: align.Right, Color: grayColor()})),
		),
	}

	// Render the drawn signature image if available
	if len(data.SignatureImage) > 0 {
		rows = append(rows,
			row.New(4), // spacer
			row.New(25).Add(
				col.New(2).Add(text.New("Handtekening:", props.Text{Size: 8, Color: grayColor(), Top: 8})),
				col.New(5).Add(
					image.NewFromBytes(data.SignatureImage, extension.Png, props.Rect{
						Center:  false,
						Percent: 90,
					}),
				),
				col.New(5), // empty right space
			),
		)
	}

	return rows
}

func statusColor(status string) *props.Color {
	switch status {
	case "Accepted":
		return &props.Color{Red: 22, Green: 163, Blue: 74}
	case "Rejected":
		return &props.Color{Red: 220, Green: 38, Blue: 38}
	case "Sent":
		return &props.Color{Red: 37, Green: 99, Blue: 235}
	default:
		return &props.Color{Red: 107, Green: 114, Blue: 128}
	}
}

func grayColor() *props.Color {
	return &props.Color{Red: 107, Green: 114, Blue: 128}
}

func formatCents(cents int64) string {
	return fmt.Sprintf("€%.2f", float64(cents)/100.0)
}
