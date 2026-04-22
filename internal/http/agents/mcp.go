package agents

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"portal_final_backend/platform/mcp"
)

// MCPHandler mounts the Model Context Protocol server on an HTTP route.
type MCPHandler struct {
	server *mcp.Server
}

// NewMCPHandler creates an MCP handler with a basic set of registered tools.
// In production, this should be wired with the full domain tool registry.
func NewMCPHandler() *MCPHandler {
	server := mcp.NewServer(func(ctx context.Context, name string, args map[string]any) (any, error) {
		// This is intentionally a placeholder handler because actual wiring would require
		// dependency injection that would create circular dependencies at this level.
		return fmt.Sprintf("tool %s executed with args %+v", name, args), nil
	})

	// Register placeholder tools matching the domain tool catalog so external
	// agents can discover the API surface.
	_ = server.RegisterTool("GetLeadDetails", "Returns contact and address details for a lead.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"lead_id": map[string]any{"type": "string", "description": "UUID of the lead"},
		},
		"required": []string{"lead_id"},
	})
	_ = server.RegisterTool("SearchLeads", "Searches leads by customer name, phone, or quote number.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "Search query"},
		},
		"required": []string{"query"},
	})
	_ = server.RegisterTool("UpdatePipelineStage", "Updates the pipeline stage for a lead service.", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"lead_id":    map[string]any{"type": "string"},
			"service_id": map[string]any{"type": "string"},
			"stage":      map[string]any{"type": "string"},
		},
		"required": []string{"lead_id", "service_id", "stage"},
	})

	return &MCPHandler{server: server}
}

// RegisterRoutes mounts the MCP JSON-RPC endpoint.
func (h *MCPHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/mcp", gin.WrapH(h.server.HTTPHandler()))
}
