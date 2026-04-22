package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client consumes an external MCP server.
type Client struct {
	baseURL string
	client  *http.Client
}

// NewClient creates an MCP client pointing at the given server base URL.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// NewClientWithHTTP creates an MCP client with a custom HTTP client.
func NewClientWithHTTP(baseURL string, httpClient *http.Client) *Client {
	return &Client{baseURL: baseURL, client: httpClient}
}

// Initialize performs the MCP handshake.
func (c *Client) Initialize(ctx context.Context) error {
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: map[string]any{"protocolVersion": "2024-11-05"}}
	_, err := c.call(ctx, req)
	return err
}

// ListTools fetches the tool manifest from the remote MCP server.
func (c *Client) ListTools(ctx context.Context) ([]MCPTool, error) {
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "tools/list"}
	resp, err := c.call(ctx, req)
	if err != nil {
		return nil, err
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mcp client: unexpected result type")
	}
	rawTools, ok := result["tools"].([]any)
	if !ok {
		return nil, fmt.Errorf("mcp client: missing tools field")
	}

	tools := make([]MCPTool, 0, len(rawTools))
	for _, rt := range rawTools {
		m, ok := rt.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		desc, _ := m["description"].(string)
		schemaRaw, _ := json.Marshal(m["inputSchema"])
		tools = append(tools, MCPTool{
			Name:        name,
			Description: desc,
			InputSchema: schemaRaw,
		})
	}
	return tools, nil
}

// CallTool invokes a remote MCP tool with the given arguments.
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) (string, bool, error) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      name,
			"arguments": arguments,
		},
	}
	resp, err := c.call(ctx, req)
	if err != nil {
		return "", false, err
	}
	if resp.Error != nil {
		return "", false, fmt.Errorf("mcp client: tool error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		return "", false, fmt.Errorf("mcp client: unexpected result type")
	}
	isError, _ := result["isError"].(bool)
	content, _ := result["content"].([]any)
	var text string
	if len(content) > 0 {
		first, _ := content[0].(map[string]any)
		text, _ = first["text"].(string)
	}
	return text, isError, nil
}

func (c *Client) call(ctx context.Context, req JSONRPCRequest) (*JSONRPCResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mcp client: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var rpcResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("mcp client: decode failed: %w", err)
	}
	return &rpcResp, nil
}
