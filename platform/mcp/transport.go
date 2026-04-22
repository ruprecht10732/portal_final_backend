package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	contentTypeHeader      = "Content-Type"
	acceptHeader           = "Accept"
	contentTypeJSON        = "application/json"
	contentTypeEventStream = "text/event-stream"
)

// StreamableHTTPTransport implements the next-generation MCP transport over a
// single HTTP endpoint, replacing the legacy SSE dual-endpoint model.
// It multiplexes bidirectional communication (requests + streaming notifications)
// over one persistent connection.
type StreamableHTTPTransport struct {
	server     *Server
	streamPath string
}

// NewStreamableHTTPTransport wraps an MCP Server with Streamable HTTP support.
func NewStreamableHTTPTransport(server *Server, streamPath string) *StreamableHTTPTransport {
	return &StreamableHTTPTransport{server: server, streamPath: streamPath}
}

// ServeHTTP implements the unified endpoint.
// POST with Accept: application/json → unary JSON-RPC response.
// POST with Accept: text/event-stream → SSE stream of notifications.
func (t *StreamableHTTPTransport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	accept := r.Header.Get(acceptHeader)
	if strings.Contains(accept, contentTypeEventStream) {
		t.handleStream(w, r)
		return
	}

	// Unary JSON-RPC
	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONRPCError(w, nil, -32700, "Parse error")
		return
	}
	resp := t.server.HandleJSONRPC(r.Context(), req)
	w.Header().Set(contentTypeHeader, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (t *StreamableHTTPTransport) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set(contentTypeHeader, contentTypeEventStream)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Simple heartbeat to keep connection alive
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	done := r.Context().Done()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			_, _ = fmt.Fprintf(w, ":heartbeat\n\n")
			flusher.Flush()
		}
	}
}

// StreamableHTTPClient is a client for Streamable HTTP MCP servers.
type StreamableHTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewStreamableHTTPClient creates a client for a Streamable HTTP MCP endpoint.
func NewStreamableHTTPClient(baseURL string) *StreamableHTTPClient {
	return &StreamableHTTPClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

// Call performs a unary JSON-RPC request.
func (c *StreamableHTTPClient) Call(ctx context.Context, req JSONRPCRequest) (*JSONRPCResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set(contentTypeHeader, contentTypeJSON)
	httpReq.Header.Set(acceptHeader, contentTypeJSON)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mcp streamable client: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var rpcResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("mcp streamable client: decode failed: %w", err)
	}
	return &rpcResp, nil
}

// Stream opens an SSE connection for server-initiated notifications.
func (c *StreamableHTTPClient) Stream(ctx context.Context, req JSONRPCRequest) (chan string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set(contentTypeHeader, contentTypeJSON)
	httpReq.Header.Set(acceptHeader, contentTypeEventStream)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mcp streamable client: stream failed: %w", err)
	}

	ch := make(chan string, 16)
	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					ch <- fmt.Sprintf("error: %v", err)
				}
				return
			}
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data:") {
				ch <- strings.TrimPrefix(line, "data:")
			}
		}
	}()
	return ch, nil
}
