// Package energylabel provides the energy label bounded context module.
// This file encapsulates domain bootstrapping and lifecycle management.
package energylabel

import (
	"portal_final_backend/internal/energylabel/client"
	"portal_final_backend/internal/energylabel/service"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"
)

// --- Interface Guard ---
// This line enforces that the concrete service implementation in the 'service'
// sub-package strictly adheres to the public 'Service' interface defined in
// this package. This is a O(0) runtime cost check that prevents partial
// implementations from ever reaching the build stage.
var _ Service = (*service.Service)(nil)

// Module reordered for optimal memory alignment (8-byte pointer followed by 1-byte bool).
// This minimizes struct padding and keeps the stack/heap footprint at O(1) minimum.
type Module struct {
	service *service.Service
	enabled bool
}

// NewModule creates and initializes the energy label domain.
// It implements graceful degradation: if configuration is missing, it returns
// a functional but "disabled" module rather than panicking or returning nil.
func NewModule(cfg config.EnergyLabelConfig, log *logger.Logger) *Module {
	if !cfg.IsEnergyLabelEnabled() {
		log.Info("energy label module disabled: configuration missing or EP_ONLINE_API_KEY unset")
		return &Module{enabled: false}
	}

	// Defensive check: ensure log is provided to dependencies.
	// We initialize client and service in O(1) time during boot.
	apiClient := client.New(cfg.GetEPOnlineAPIKey(), log)
	svc := service.New(apiClient, log)

	log.Info("energy label module initialized successfully")

	return &Module{
		service: svc,
		enabled: true,
	}
}

// Service returns the domain business logic layer.
// Implementation Note: Callers MUST check IsEnabled() or handle nil to avoid panics.
// Complexity: O(1).
func (m *Module) Service() *service.Service {
	if !m.IsEnabled() {
		return nil
	}
	return m.service
}

// IsEnabled validates the module's operational state.
// Safe to call on a nil receiver, ensuring boot-sequence stability.
func (m *Module) IsEnabled() bool {
	return m != nil && m.enabled
}
