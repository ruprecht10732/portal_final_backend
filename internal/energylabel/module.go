// Package energylabel provides the energy label bounded context module.
// This file defines the module that encapsulates all energy label setup.
package energylabel

import (
	"portal_final_backend/internal/energylabel/client"
	"portal_final_backend/internal/energylabel/service"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"
)

// Module is the energy label bounded context module.
type Module struct {
	service *service.Service
	enabled bool
}

// NewModule creates and initializes the energy label module.
// Returns nil if energy label API is not configured (graceful degradation).
func NewModule(cfg config.EnergyLabelConfig, log *logger.Logger) *Module {
	if !cfg.IsEnergyLabelEnabled() {
		log.Info("energy label module disabled: EP_ONLINE_API_KEY not configured")
		return &Module{enabled: false}
	}

	apiClient := client.New(cfg.GetEPOnlineAPIKey(), log)
	svc := service.New(apiClient, log)

	log.Info("energy label module initialized")

	return &Module{
		service: svc,
		enabled: true,
	}
}

// Service returns the energy label service for external use.
// Returns nil if the module is disabled.
func (m *Module) Service() *service.Service {
	if m == nil || !m.enabled {
		return nil
	}
	return m.service
}

// IsEnabled returns true if the energy label module is configured and enabled.
func (m *Module) IsEnabled() bool {
	return m != nil && m.enabled
}
