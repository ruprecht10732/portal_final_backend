package agents

import (
	apphttp "portal_final_backend/internal/http"
)

// Module is the HTTP module for agent discovery and A2A protocol support.
type Module struct{}

// NewModule creates a new agents module.
func NewModule() *Module {
	return &Module{}
}

// Name returns the module name.
func (m *Module) Name() string {
	return "agents"
}

// RegisterRoutes mounts agent discovery endpoints.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	RegisterRoutes(ctx.V1.Group("/agents"))
	mcpHandler := NewMCPHandler()
	mcpHandler.RegisterRoutes(ctx.V1)
}
