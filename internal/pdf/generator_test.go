package pdf

import (
	"testing"
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
		{Filename: "first.pdf", PDFBytes: []byte("same-pdf")},
		{Filename: "duplicate.pdf", PDFBytes: []byte("same-pdf")},
		{Filename: "second.pdf", PDFBytes: []byte("different-pdf")},
	}

	actual := dedupeAttachmentPDFs(attachments)
	if len(actual) != 2 {
		t.Fatalf("dedupeAttachmentPDFs() length = %d, want 2", len(actual))
	}

	if actual[0].Filename != "first.pdf" {
		t.Fatalf("first attachment = %q, want %q", actual[0].Filename, "first.pdf")
	}

	if actual[1].Filename != "second.pdf" {
		t.Fatalf("second attachment = %q, want %q", actual[1].Filename, "second.pdf")
	}

	if string(actual[0].PDFBytes) != "same-pdf" {
		t.Fatalf("first attachment bytes changed unexpectedly")
	}

	if string(actual[1].PDFBytes) != "different-pdf" {
		t.Fatalf("second attachment bytes changed unexpectedly")
	}
}
