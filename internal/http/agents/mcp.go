package agents

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"portal_final_backend/platform/mcp"
	"portal_final_backend/platform/mcp/toolbox"
)

// MCPHandler mounts the Model Context Protocol server on an HTTP route.
type MCPHandler struct {
	server *mcp.Server
}

func NewMCPHandler(pool *pgxpool.Pool) *MCPHandler {
	var tbHandler mcp.ToolHandler

	server := mcp.NewServer(func(ctx context.Context, name string, args map[string]any) (any, error) {
		if tbHandler != nil {
			return tbHandler(ctx, name, args)
		}
		return fmt.Sprintf("tool %s executed with args %+v (placeholder)", name, args), nil
	})

	// Load declarative MCP toolbox tools from YAML
	tb := toolbox.NewLoader(pool, server)
	handler, err := tb.LoadAndRegister("agents/calculator/tools.yaml")
	if err != nil {
		slog.Error("failed to load MCP toolbox", "error", err)
	} else {
		tbHandler = handler
	}
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
