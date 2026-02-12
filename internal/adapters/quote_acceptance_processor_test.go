package adapters

import "testing"

const (
	manualBucketName  = "manual-bucket"
	catalogBucketName = "catalog-bucket"
	unexpectedOrderMsg = "unexpected buckets order: %#v"
)

func TestIsPDFAttachmentByFilename(t *testing.T) {
	if !isPDFAttachment("brochure.pdf", []byte("not-a-real-pdf")) {
		t.Fatal("expected .pdf filename to be accepted")
	}
}

func TestIsPDFAttachmentByMagicHeader(t *testing.T) {
	pdfBytes := []byte("%PDF-1.7\n1 0 obj\n")
	if !isPDFAttachment("attachment.bin", pdfBytes) {
		t.Fatal("expected PDF magic header to be accepted")
	}
}

func TestIsPDFAttachmentRejectsNonPDF(t *testing.T) {
	pngHeader := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if isPDFAttachment("logo.png", pngHeader) {
		t.Fatal("expected non-PDF attachment to be rejected")
	}
}

func TestResolveAttachmentBucketsManualFirst(t *testing.T) {
	buckets := resolveAttachmentBuckets("manual", manualBucketName, catalogBucketName)
	if len(buckets) != 2 || buckets[0] != manualBucketName || buckets[1] != catalogBucketName {
		t.Fatalf(unexpectedOrderMsg, buckets)
	}
}

func TestResolveAttachmentBucketsCatalogFirst(t *testing.T) {
	buckets := resolveAttachmentBuckets("catalog", manualBucketName, catalogBucketName)
	if len(buckets) != 2 || buckets[0] != catalogBucketName || buckets[1] != manualBucketName {
		t.Fatalf(unexpectedOrderMsg, buckets)
	}
}

func TestResolveAttachmentBucketsUnknownFallsBack(t *testing.T) {
	buckets := resolveAttachmentBuckets("", manualBucketName, catalogBucketName)
	if len(buckets) != 2 || buckets[0] != manualBucketName || buckets[1] != catalogBucketName {
		t.Fatalf(unexpectedOrderMsg, buckets)
	}
}
