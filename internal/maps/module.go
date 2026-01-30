package maps

import (
	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/platform/logger"
)

// Module wires the maps address lookup HTTP routes.
type Module struct {
	handler *Handler
}

func NewModule(log *logger.Logger) *Module {
	svc := NewService(log)
	h := NewHandler(svc)
	return &Module{handler: h}
}

func (m *Module) Name() string {
	return "maps"
}

func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	group := ctx.Protected.Group("/maps")
	group.GET("/address-lookup", m.handler.LookupAddress)
}

var _ apphttp.Module = (*Module)(nil)
