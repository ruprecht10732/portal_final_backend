// Package leadenrichment provides the composition root for lead enrichment.
package leadenrichment

import (
	"portal_final_backend/internal/leadenrichment/client"
	"portal_final_backend/internal/leadenrichment/service"
	"portal_final_backend/platform/logger"
)

// Module wires the lead enrichment service.
type Module struct {
	service *service.Service
}

// NewModule creates a new lead enrichment module.
func NewModule(log *logger.Logger) *Module {
	cli := client.New(log)
	svc := service.New(cli, log)
	return &Module{service: svc}
}

// Service returns the enrichment service.
func (m *Module) Service() *service.Service {
	return m.service
}
