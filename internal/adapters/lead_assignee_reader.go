package adapters

import (
	"context"

	leadsrepo "portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/notification"

	"github.com/google/uuid"
)

// leadAssigneeReaderStore captures the read dependency needed to resolve assignees.
type leadAssigneeReaderStore interface {
	GetByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (leadsrepo.Lead, error)
}

// LeadAssigneeReader adapts leads repository access for notification recipient routing.
type LeadAssigneeReader struct {
	store leadAssigneeReaderStore
}

// NewLeadAssigneeReader creates a lead assignee reader adapter.
func NewLeadAssigneeReader(store leadAssigneeReaderStore) *LeadAssigneeReader {
	return &LeadAssigneeReader{store: store}
}

// GetAssignedAgentID returns the currently assigned agent for the given lead.
func (a *LeadAssigneeReader) GetAssignedAgentID(ctx context.Context, leadID uuid.UUID, orgID uuid.UUID) (*uuid.UUID, error) {
	lead, err := a.store.GetByID(ctx, leadID, orgID)
	if err != nil {
		return nil, err
	}
	return lead.AssignedAgentID, nil
}

// Compile-time check.
var _ notification.LeadAssigneeReader = (*LeadAssigneeReader)(nil)
