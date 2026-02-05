// Package qdrant provides a REST client for Qdrant vector database.
package qdrant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is an HTTP client for Qdrant vector database.
type Client struct {
	baseURL    string
	apiKey     string
	collection string
	httpClient *http.Client
}

// Config configures the Qdrant client.
type Config struct {
	BaseURL    string
	APIKey     string
	Collection string
	Timeout    time.Duration
}

// NewClient creates a new Qdrant client.
func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		baseURL:    cfg.BaseURL,
		apiKey:     cfg.APIKey,
		collection: cfg.Collection,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// SearchRequest is the request body for a vector search.
type SearchRequest struct {
	Vector      []float32 `json:"vector"`
	Limit       int       `json:"limit"`
	WithPayload bool      `json:"with_payload"`
}

// SearchResult is a single search result from Qdrant.
type SearchResult struct {
	ID      interface{}            `json:"id"`
	Score   float64                `json:"score"`
	Payload map[string]interface{} `json:"payload"`
}

// SearchResponse is the response from a search query.
type SearchResponse struct {
	Result []SearchResult `json:"result"`
	Status interface{}    `json:"status"`
	Time   float64        `json:"time"`
}

// Search performs a vector similarity search in the configured collection.
func (c *Client) Search(ctx context.Context, vector []float32, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}

	reqBody := SearchRequest{
		Vector:      vector,
		Limit:       limit,
		WithPayload: true,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search request: %w", err)
	}

	url := fmt.Sprintf("%s/collections/%s/points/search", c.baseURL, c.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create search request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("api-key", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("qdrant returned %d: %s", resp.StatusCode, string(body))
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	return searchResp.Result, nil
}
