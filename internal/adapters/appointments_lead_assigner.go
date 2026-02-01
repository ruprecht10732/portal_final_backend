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

func (a *AppointmentsLeadAssigner) GetAssignedAgentID(ctx context.Context, leadID uuid.UUID) (*uuid.UUID, error) {
	lead, err := a.mgmt.GetByID(ctx, leadID)
	if err != nil {
		return nil, err
	}
	return lead.AssignedAgentID, nil
}

func (a *AppointmentsLeadAssigner) AssignLead(ctx context.Context, leadID uuid.UUID, agentID uuid.UUID) error {
	return a.mgmt.AssignIfUnassigned(ctx, leadID, agentID)
}
