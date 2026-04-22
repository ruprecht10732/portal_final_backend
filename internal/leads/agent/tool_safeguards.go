package agent

import (
	"portal_final_backend/platform/adk/confirmation"
	"portal_final_backend/platform/adk/plugins"
)

// toolSafeguards holds the global HITL and retry policies for agent tools.
// These are set at application startup and applied to all high-stakes tools.
var toolSafeguards struct {
	confirmationProvider confirmation.Provider
	retryPolicy          plugins.RetryPolicy
	enableConfirmation   bool
	enableRetry          bool
}

// SetToolConfirmationProvider configures the global HITL provider.
func SetToolConfirmationProvider(p confirmation.Provider) {
	toolSafeguards.confirmationProvider = p
	toolSafeguards.enableConfirmation = p != nil
}

// SetToolRetryPolicy configures the global retry policy.
func SetToolRetryPolicy(p plugins.RetryPolicy) {
	toolSafeguards.retryPolicy = p
	toolSafeguards.enableRetry = true
}

// GetToolConfirmationProvider returns the configured confirmation provider.
func GetToolConfirmationProvider() confirmation.Provider {
	return toolSafeguards.confirmationProvider
}

// IsToolConfirmationEnabled returns true if HITL is globally enabled.
func IsToolConfirmationEnabled() bool {
	return toolSafeguards.enableConfirmation
}

// IsToolRetryEnabled returns true if retry/reflect is globally enabled.
func IsToolRetryEnabled() bool {
	return toolSafeguards.enableRetry
}

// GetToolRetryPolicy returns the configured retry policy.
func GetToolRetryPolicy() plugins.RetryPolicy {
	return toolSafeguards.retryPolicy
}
