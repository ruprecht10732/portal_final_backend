// Package mcp provides Model Context Protocol server and client implementations.
// It enables programmatic discovery and invocation of backend tools by AI agents.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"google.golang.org/adk/tool"
)

// ToolHandler is the function signature for executing an MCP tool.
type ToolHandler func(ctx context.Context, name string, args map[string]any) (any, error)

// Server exposes a set of tools via the Model Context Protocol.
type Server struct {
	mu      sync.RWMutex
	tools   []MCPTool
	handler ToolHandler
}

// MCPTool describes a tool in MCP format.
type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// NewServer creates an MCP server with the given tool execution handler.
func NewServer(handler ToolHandler) *Server {
	return &Server{handler: handler, tools: make([]MCPTool, 0)}
}

// RegisterTool adds a tool to the server's manifest.
func (s *Server) RegisterTool(name, description string, inputSchema map[string]any) error {
	schema, err := json.Marshal(inputSchema)
	if err != nil {
		return fmt.Errorf("mcp: register tool %q: %w", name, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools = append(s.tools, MCPTool{
		Name:        name,
		Description: description,
		InputSchema: schema,
	})
	return nil
}

// RegisterADKTool registers an existing ADK tool by introspecting its metadata.
func (s *Server) RegisterADKTool(t tool.Tool) error {
	// ADK tool metadata exposes name/description via the Tool interface.
	// Schema is not directly available, so we use a permissive object schema
	// and let the handler validate arguments.
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
	return s.RegisterTool(t.Name(), t.Description(), schema)
}

// Manifest returns the current tool manifest (for Agent Card / discovery).
func (s *Server) Manifest() []MCPTool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MCPTool, len(s.tools))
	copy(out, s.tools)
	return out
}

// HandleJSONRPC processes an MCP JSON-RPC request and returns a response.
func (s *Server) HandleJSONRPC(ctx context.Context, req JSONRPCRequest) JSONRPCResponse {
	switch req.Method {
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	case "initialize":
		return s.handleInitialize(req)
	default:
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &JSONRPCError{Code: -32601, Message: "Method not found"},
		}
	}
}

func (s *Server) handleInitialize(req JSONRPCRequest) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "portal-mcp-server", "version": "1.0.0"}},
	}
}

func (s *Server) handleToolsList(req JSONRPCRequest) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]any{"tools": s.Manifest()},
	}
}

func (s *Server) handleToolsCall(ctx context.Context, req JSONRPCRequest) JSONRPCResponse {
	params, ok := req.Params.(map[string]any)
	if !ok {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &JSONRPCError{Code: -32602, Message: "Invalid params"},
		}
	}
	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)
	if name == "" {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &JSONRPCError{Code: -32602, Message: "Missing tool name"},
		}
	}

	result, err := s.handler(ctx, name, args)
	if err != nil {
		return JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("Error: %v", err)}},
				"isError": true,
			},
		}
	}

	content := fmt.Sprintf("%v", result)
	if b, ok := result.([]byte); ok {
		content = string(b)
	}
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"content": []map[string]any{{"type": "text", "text": content}},
			"isError": false,
		},
	}
}

// JSONRPCRequest is a simplified MCP JSON-RPC request.
type JSONRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

// JSONRPCResponse is a simplified MCP JSON-RPC response.
type JSONRPCResponse struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id,omitempty"`
	Result  any            `json:"result,omitempty"`
	Error   *JSONRPCError  `json:"error,omitempty"`
}

// JSONRPCError is a JSON-RPC error object.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// HTTPHandler returns an http.Handler that serves the MCP protocol over Streamable HTTP.
func (s *Server) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONRPCError(w, nil, -32700, "Parse error")
			return
		}
		resp := s.HandleJSONRPC(r.Context(), req)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	})
}

func writeJSONRPCError(w http.ResponseWriter, id any, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &JSONRPCError{Code: code, Message: message},
	})
}
