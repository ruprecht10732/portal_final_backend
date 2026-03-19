package pdf

import (
	"html"
	"strings"
	"testing"
	"time"
)

const (
	firstAttachmentFilename  = "first.pdf"
	secondAttachmentFilename = "second.pdf"
	sharedPDFBytes           = "same-pdf"
	differentPDFBytes        = "different-pdf"
)

func TestNormalizePDFQuantity(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "keeps unit text", input: "10 m2", expected: "10 m2"},
		{name: "trims whitespace", input: " 10 stuks ", expected: "10 stuks"},
		{name: "removes trailing multiplication sign", input: "1 stuk×", expected: "1 stuk"},
		{name: "removes standalone trailing x token", input: "1 x", expected: "1"},
		{name: "keeps attached x quantity", input: "10x", expected: "10x"},
		{name: "keeps plain x when alone", input: "x", expected: "x"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := normalizePDFQuantity(test.input)
			if actual != test.expected {
				t.Fatalf("normalizePDFQuantity(%q) = %q, want %q", test.input, actual, test.expected)
			}
		})
	}
}

func TestDedupeAttachmentPDFsPreservesFirstOccurrenceAndOrder(t *testing.T) {
	attachments := []AttachmentPDFEntry{
		{Filename: firstAttachmentFilename, PDFBytes: []byte(sharedPDFBytes)},
		{Filename: "duplicate.pdf", PDFBytes: []byte(sharedPDFBytes)},
		{Filename: secondAttachmentFilename, PDFBytes: []byte(differentPDFBytes)},
	}

	actual := dedupeAttachmentPDFs(attachments)
	if len(actual) != 2 {
		t.Fatalf("dedupeAttachmentPDFs() length = %d, want 2", len(actual))
	}

	if actual[0].Filename != firstAttachmentFilename {
		t.Fatalf("first attachment = %q, want %q", actual[0].Filename, firstAttachmentFilename)
	}

	if actual[1].Filename != secondAttachmentFilename {
		t.Fatalf("second attachment = %q, want %q", actual[1].Filename, secondAttachmentFilename)
	}

	if string(actual[0].PDFBytes) != sharedPDFBytes {
		t.Fatalf("first attachment bytes changed unexpectedly")
	}

	if string(actual[1].PDFBytes) != differentPDFBytes {
		t.Fatalf("second attachment bytes changed unexpectedly")
	}
}

func TestQuotePDFTemplatesIncludeCustomerContactDetails(t *testing.T) {
	data := QuotePDFData{
		QuoteNumber:          "OFF-2026-0042",
		Status:               "Sent",
		CreatedAt:            time.Date(2026, time.March, 18, 10, 30, 0, 0, time.UTC),
		OrganizationName:     "Salestainable",
		CustomerName:         "Robin Janssen",
		CustomerEmail:        "robin@example.com",
		CustomerPhone:        "+31612345678",
		CustomerAddressLine1: "Voorbeeldstraat 12",
		CustomerPostalCode:   "1234AB",
		CustomerCity:         "Amsterdam",
		OrgEmail:             "info@example.com",
		OrgPhone:             "+31101234567",
	}

	coverHTML, err := renderTemplate("templates/cover.html", buildCoverVM(data, "", ""))
	if err != nil {
		t.Fatalf("render cover template: %v", err)
	}
	quoteHTML, err := renderTemplate("templates/quote.html", buildQuoteVM(data, "", ""))
	if err != nil {
		t.Fatalf("render quote template: %v", err)
	}

	for _, renderedHTML := range []string{string(coverHTML), string(quoteHTML)} {
		decoded := html.UnescapeString(renderedHTML)
		for _, expected := range []string{"Voorbeeldstraat 12", "1234AB Amsterdam", "+31612345678", "robin@example.com"} {
			if !strings.Contains(decoded, expected) {
				t.Fatalf("template output missing %q: %s", expected, renderedHTML)
			}
		}
	}
}

func TestISDESummaryTemplateIncludesEmbeddedQRCodes(t *testing.T) {
	vm, err := buildISDESummaryViewModel(ISDESummaryPDFData{
		QuoteNumber:          "OFF-2026-0042",
		OrganizationName:     "Salestainable",
		LeadName:             "Robin Janssen",
		LeadAddress:          "Voorbeeldstraat 12, Amsterdam",
		TotalAmountCents:     275000,
		IsDoubled:            true,
		EligibleMeasureCount: 3,
		InsulationBreakdown: []ISDESummaryLineItem{
			{Description: "Dakisolatie", AreaM2: 42.5, AmountCents: 125000},
		},
	}, time.Date(2026, time.March, 19, 15, 4, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build ISDE summary view model: %v", err)
	}

	renderedHTML, err := renderTemplate("templates/isde_subsidy_summary.html", vm)
	if err != nil {
		t.Fatalf("render ISDE summary template: %v", err)
	}

	decoded := html.UnescapeString(string(renderedHTML))
	for _, expected := range []string{
		"data:image/png;base64,",
		isdeRVOURL,
		isdeKlimaatrouteURL,
		"Subsidie aanvragen & extra informatie",
		"Salestainable",
	} {
		if !strings.Contains(decoded, expected) {
			t.Fatalf("ISDE summary template output missing %q", expected)
		}
	}
}
