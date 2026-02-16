package handler

import (
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

func servePDFBytes(c *gin.Context, quoteNumber string, pdfBytes []byte) {
	setPDFHeaders(c, quoteNumber)
	c.Data(http.StatusOK, contentTypePDF, pdfBytes)
}

func streamPDFFromReader(c *gin.Context, quoteNumber string, reader io.ReadCloser) {
	defer func() { _ = reader.Close() }()

	setPDFHeaders(c, quoteNumber)
	c.Status(http.StatusOK)

	if _, err := io.Copy(c.Writer, reader); err != nil {
		_ = c.Error(err)
	}
}

func setPDFHeaders(c *gin.Context, quoteNumber string) {
	fileName := fmt.Sprintf("Offerte-%s.pdf", quoteNumber)
	c.Header("Content-Type", contentTypePDF)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
}
