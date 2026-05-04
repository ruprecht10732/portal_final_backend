package handler

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
)

func servePDFBytes(c *gin.Context, quoteNumber string, pdfBytes []byte) {
	if err := validatePDFBytes(pdfBytes); err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to serve PDF", err.Error())
		return
	}
	slog.Info("serving PDF bytes", "quoteNumber", quoteNumber, "bytes", len(pdfBytes))
	setPDFHeaders(c, quoteNumber)
	c.Data(http.StatusOK, contentTypePDF, pdfBytes)
}

func streamPDFFromReader(c *gin.Context, quoteNumber string, reader io.ReadCloser) {
	defer func() { _ = reader.Close() }()

	pdfBytes, err := io.ReadAll(reader)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to read PDF", err.Error())
		return
	}

	slog.Info("streaming PDF from storage", "quoteNumber", quoteNumber, "bytes", len(pdfBytes))
	servePDFBytes(c, quoteNumber, pdfBytes)
}

func validatePDFBytes(pdfBytes []byte) error {
	if len(pdfBytes) == 0 {
		return fmt.Errorf("PDF is empty")
	}
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
		return fmt.Errorf("PDF has invalid structure (missing %%PDF- header, starts with: %q)", string(pdfBytes[:min(len(pdfBytes), 50)]))
	}
	// A valid PDF must end with %%EOF (possibly preceded by whitespace/newlines).
	// Truncated PDFs (e.g. from network errors or size limits) often miss this.
	trimmed := bytes.TrimRight(pdfBytes, " \t\r\n\x00")
	if !bytes.HasSuffix(trimmed, []byte("%%EOF")) {
		return fmt.Errorf("PDF appears truncated (missing %%EOF trailer, size=%d)", len(pdfBytes))
	}
	return nil
}

func setPDFHeaders(c *gin.Context, quoteNumber string) {
	fileName := fmt.Sprintf("Offerte-%s.pdf", quoteNumber)
	c.Header("Content-Type", contentTypePDF)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
}
