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

// CollectionName returns the configured default collection for this client.
func (c *Client) CollectionName() string {
	return c.collection
}

// SearchRequest is the request body for a vector search.
type SearchRequest struct {
	CollectionName string    `json:"collection_name,omitempty"`
	Vector         []float32 `json:"vector"`
	Limit          int       `json:"limit"`
	WithPayload    bool      `json:"with_payload"`
	ScoreThreshold *float64  `json:"score_threshold,omitempty"` // Minimum similarity score (Qdrant filters server-side)
	Filter         *Filter   `json:"filter,omitempty"`
}

// BatchSearchRequest is the request body for multi-collection batch search.
type BatchSearchRequest struct {
	Searches []SearchRequest `json:"searches"`
}

// MatchValue represents a Qdrant match condition value.
type MatchValue struct {
	Value string `json:"value"`
}

// FieldCondition represents a Qdrant payload field match condition.
type FieldCondition struct {
	Key   string     `json:"key"`
	Match MatchValue `json:"match"`
}

// Filter represents Qdrant payload filtering clauses.
type Filter struct {
	Must []FieldCondition `json:"must,omitempty"`
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

// BatchSearchResponse is the response from a batch search query.
type BatchSearchResponse struct {
	Result [][]SearchResult `json:"result"`
	Status interface{}      `json:"status"`
	Time   float64          `json:"time"`
}

// SearchWithThreshold performs a vector similarity search with a minimum score threshold.
func (c *Client) SearchWithThreshold(ctx context.Context, vector []float32, limit int, scoreThreshold float64) ([]SearchResult, error) {
	return c.searchInternal(ctx, vector, limit, &scoreThreshold, nil)
}

// SearchWithFilter performs a vector similarity search with threshold and payload filter.
func (c *Client) SearchWithFilter(ctx context.Context, vector []float32, limit int, scoreThreshold float64, filter *Filter) ([]SearchResult, error) {
	return c.searchInternal(ctx, vector, limit, &scoreThreshold, filter)
}

// Search performs a vector similarity search in the configured collection.
func (c *Client) Search(ctx context.Context, vector []float32, limit int) ([]SearchResult, error) {
	return c.searchInternal(ctx, vector, limit, nil, nil)
}

// BatchSearch performs multiple vector searches in one request.
// If a request omits CollectionName, the client's configured collection is used.
func (c *Client) BatchSearch(ctx context.Context, requests []SearchRequest) ([][]SearchResult, error) {
	if len(requests) == 0 {
		return nil, nil
	}

	normalized := make([]SearchRequest, 0, len(requests))
	for _, req := range requests {
		if req.CollectionName == "" {
			req.CollectionName = c.collection
		}
		if req.Limit <= 0 {
			req.Limit = 5
		}
		req.WithPayload = true
		normalized = append(normalized, req)
	}

	bodyBytes, err := json.Marshal(BatchSearchRequest{Searches: normalized})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal batch search request: %w", err)
	}

	url := fmt.Sprintf("%s/points/search/batch", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create batch search request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("api-key", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("batch search request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("qdrant batch search returned %d: %s", resp.StatusCode, string(body))
	}

	var searchResp BatchSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode batch search response: %w", err)
	}

	return searchResp.Result, nil
}

// NewOrganizationFilter builds a payload filter for tenant-scoped catalog search.
func NewOrganizationFilter(organizationID string) *Filter {
	if organizationID == "" {
		return nil
	}

	return &Filter{
		Must: []FieldCondition{
			{
				Key:   "organization_id",
				Match: MatchValue{Value: organizationID},
			},
		},
	}
}

func (c *Client) searchInternal(ctx context.Context, vector []float32, limit int, scoreThreshold *float64, filter *Filter) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}

	reqBody := SearchRequest{
		Vector:         vector,
		Limit:          limit,
		WithPayload:    true,
		ScoreThreshold: scoreThreshold,
		Filter:         filter,
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
	defer func() { _ = resp.Body.Close() }()

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
