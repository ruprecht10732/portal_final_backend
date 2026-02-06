// Package pdf – Gotenberg HTTP client for HTML→PDF conversion and merging.
package pdf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"time"
)

// GotenbergClient converts HTML to PDF via a Gotenberg instance.
type GotenbergClient struct {
	baseURL  string
	username string
	password string
	http     *http.Client
}

// NewGotenbergClient creates a client pointing at the given Gotenberg URL.
// If username and password are non-empty, every request will include HTTP Basic Auth.
func NewGotenbergClient(baseURL, username, password string) *GotenbergClient {
	return &GotenbergClient{
		baseURL:  baseURL,
		username: username,
		password: password,
		http: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// ConvertOpts configures the HTML→PDF conversion request.
type ConvertOpts struct {
	MarginTop    string
	MarginBottom string
	MarginLeft   string
	MarginRight  string
	FooterHTML   []byte
	// WaitDelay adds a delay before capture (e.g. "2s") for font loading.
	WaitDelay string
}

// DefaultContentOpts returns options for the main quote body pages.
func DefaultContentOpts() ConvertOpts {
	return ConvertOpts{
		MarginTop:    "0.5",
		MarginBottom: "0.7",
		MarginLeft:   "0.5",
		MarginRight:  "0.5",
		WaitDelay:    "1s",
	}
}

// CoverPageOpts returns options for a full-bleed cover page (no margins, no footer).
func CoverPageOpts() ConvertOpts {
	return ConvertOpts{
		MarginTop:    "0",
		MarginBottom: "0",
		MarginLeft:   "0",
		MarginRight:  "0",
		WaitDelay:    "2s",
	}
}

// ConvertHTML sends index.html to Gotenberg and returns the resulting PDF bytes.
func (g *GotenbergClient) ConvertHTML(ctx context.Context, indexHTML []byte, opts ConvertOpts) ([]byte, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	fields := map[string]string{
		"paperWidth":        "8.27",
		"paperHeight":       "11.7",
		"marginTop":         opts.MarginTop,
		"marginBottom":      opts.MarginBottom,
		"marginLeft":        opts.MarginLeft,
		"marginRight":       opts.MarginRight,
		"printBackground":   "true",
		"preferCssPageSize": "false",
	}
	if opts.WaitDelay != "" {
		fields["waitDelay"] = opts.WaitDelay
		fields["skipNetworkIdleEvent"] = "true"
	}
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			return nil, fmt.Errorf("write field %s: %w", k, err)
		}
	}

	if err := addHTMLPart(writer, "index.html", indexHTML); err != nil {
		return nil, err
	}

	if len(opts.FooterHTML) > 0 {
		if err := addHTMLPart(writer, "footer.html", opts.FooterHTML); err != nil {
			return nil, err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	return g.doPost(ctx, "/forms/chromium/convert/html", body, writer.FormDataContentType())
}

// MergePDFs merges multiple PDFs into one using Gotenberg's merge endpoint.
// The PDFs are merged alphanumerically by filename, so prefix with "01_", "02_", etc.
func (g *GotenbergClient) MergePDFs(ctx context.Context, pdfs map[string][]byte) ([]byte, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	for filename, data := range pdfs {
		if err := addFilePart(writer, filename, "application/pdf", data); err != nil {
			return nil, err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	return g.doPost(ctx, "/forms/pdfengines/merge", body, writer.FormDataContentType())
}

// doPost sends a POST request and reads the response body.
func (g *GotenbergClient) doPost(ctx context.Context, path string, body *bytes.Buffer, contentType string) ([]byte, error) {
	url := g.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	if g.username != "" && g.password != "" {
		req.SetBasicAuth(g.username, g.password)
	}

	resp, err := g.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gotenberg %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gotenberg %s returned %d: %s", path, resp.StatusCode, string(errBody))
	}

	result, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response from %s: %w", path, err)
	}
	return result, nil
}

// addHTMLPart adds an HTML file to the multipart form.
func addHTMLPart(w *multipart.Writer, filename string, content []byte) error {
	return addFilePart(w, filename, "text/html", content)
}

// addFilePart adds a file to the multipart form.
func addFilePart(w *multipart.Writer, filename, mimeType string, content []byte) error {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files"; filename="%s"`, filename))
	h.Set("Content-Type", mimeType)

	part, err := w.CreatePart(h)
	if err != nil {
		return fmt.Errorf("create part %s: %w", filename, err)
	}
	if _, err := part.Write(content); err != nil {
		return fmt.Errorf("write part %s: %w", filename, err)
	}
	return nil
}
