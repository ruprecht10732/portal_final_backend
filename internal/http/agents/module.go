package agents

import (
	apphttp "portal_final_backend/internal/http"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Module is the HTTP module for agent discovery and A2A protocol support.
type Module struct{
	pool *pgxpool.Pool
}

// NewModule creates a new agents module.
func NewModule(pool *pgxpool.Pool) *Module {
	return &Module{pool: pool}
}

// Name returns the module name.
func (m *Module) Name() string {
	return "agents"
}

// RegisterRoutes mounts agent discovery endpoints.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	RegisterRoutes(ctx.V1.Group("/agents"))
	mcpHandler := NewMCPHandler(m.pool)
	mcpHandler.RegisterRoutes(ctx.V1)
}
