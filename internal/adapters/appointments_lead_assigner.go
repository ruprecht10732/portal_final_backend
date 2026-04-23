package adapters

import (
	"context"

	"portal_final_backend/internal/leads/management"

	"github.com/google/uuid"
)

// AppointmentsLeadAssigner adapts lead management for appointment lead ownership checks.
type AppointmentsLeadAssigner struct {
	mgmt *management.Service
}

func NewAppointmentsLeadAssigner(mgmt *management.Service) *AppointmentsLeadAssigner {
	return &AppointmentsLeadAssigner{mgmt: mgmt}
}

// GetAssignedAgentID retrieves the agent currently responsible for the lead.
func (a *AppointmentsLeadAssigner) GetAssignedAgentID(ctx context.Context, leadID, tenantID uuid.UUID) (*uuid.UUID, error) {
	// Standardized O(1) lookup via the management service.
	lead, err := a.mgmt.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return nil, err
	}

	// Returns the pointer directly from the lead object; safe as lead is non-nil if err is nil.
	return lead.AssignedAgentID, nil
}

// AssignLead attempts to set the agent for a lead only if it doesn't already have one.
func (a *AppointmentsLeadAssigner) AssignLead(ctx context.Context, leadID, agentID, tenantID uuid.UUID) error {
	return a.mgmt.AssignIfUnassigned(ctx, leadID, agentID, tenantID)
}
