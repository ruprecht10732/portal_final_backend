// Package embeddings provides a client for external embedding API services.
package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is an HTTP client for embedding API services.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// Config configures the embedding client.
type Config struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

// NewClient creates a new embedding API client.
func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// embeddingRequest is the request body for the embedding API.
type embeddingRequest struct {
	Text string `json:"text"`
}

// Embed generates an embedding vector for the given text.
// Returns a 1024-dimensional vector for BGE-M3.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := embeddingRequest{Text: text}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding API returned %d: %s", resp.StatusCode, string(body))
	}

	// Accept both {"vector": [...]} and raw array responses for compatibility.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedding response: %w", err)
	}

	var wrapped struct {
		Vector []float32 `json:"vector"`
	}
	if err := json.Unmarshal(body, &wrapped); err == nil && len(wrapped.Vector) > 0 {
		return wrapped.Vector, nil
	}

	var vector []float32
	if err := json.Unmarshal(body, &vector); err == nil {
		return vector, nil
	}

	return nil, fmt.Errorf("failed to decode embedding response")
}
