// Package embeddingapi provides a client for the product embedding API.
package embeddingapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is an HTTP client for the product embedding API.
type Client struct {
	baseURL    string
	apiKey     string
	collection string
	httpClient *http.Client
}

// Config configures the product embedding API client.
type Config struct {
	BaseURL    string
	APIKey     string
	Collection string
	Timeout    time.Duration
}

// NewClient creates a new product embedding API client.
func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:     cfg.APIKey,
		collection: strings.TrimSpace(cfg.Collection),
		httpClient: &http.Client{Timeout: timeout},
	}
}

// AddDocumentsRequest is the request body for adding documents.
type AddDocumentsRequest struct {
	Documents  []map[string]any `json:"documents"`
	TextFields []string         `json:"text_fields,omitempty"`
	IDField    string           `json:"id_field,omitempty"`
	Collection string           `json:"collection,omitempty"`
}

// AddDocumentsResponse is the response from the add documents endpoint.
type AddDocumentsResponse struct {
	Success        bool     `json:"success"`
	DocumentsAdded int      `json:"documents_added"`
	IDs            []string `json:"ids"`
	Message        string   `json:"message"`
}

// AddDocuments sends documents to the product embedding API.
func (c *Client) AddDocuments(ctx context.Context, req AddDocumentsRequest) (AddDocumentsResponse, error) {
	if len(req.Documents) == 0 {
		return AddDocumentsResponse{}, fmt.Errorf("documents are required")
	}
	if req.Collection == "" {
		req.Collection = c.collection
	}

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return AddDocumentsResponse{}, fmt.Errorf("failed to marshal add documents request: %w", err)
	}

	url := c.baseURL + "/api/documents"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return AddDocumentsResponse{}, fmt.Errorf("failed to create add documents request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(request)
	if err != nil {
		return AddDocumentsResponse{}, fmt.Errorf("add documents request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return AddDocumentsResponse{}, fmt.Errorf("embedding API returned %d: %s", resp.StatusCode, string(body))
	}

	var result AddDocumentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		body, _ := io.ReadAll(resp.Body)
		return AddDocumentsResponse{}, fmt.Errorf("failed to decode add documents response: %w (%s)", err, string(body))
	}

	return result, nil
}
