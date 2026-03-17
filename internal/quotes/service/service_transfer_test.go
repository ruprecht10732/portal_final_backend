package service

import (
	"testing"

	"github.com/google/uuid"

	leadrepo "portal_final_backend/internal/leads/repository"
)

func TestPlanQuoteTransferSourceCleanupDeletesLeadWhenOnlyService(t *testing.T) {
	leadID := uuid.New()
	serviceID := uuid.New()
	plan := planQuoteTransferSourceCleanup(leadID, serviceID, []leadrepo.LeadService{{ID: serviceID}})
	if !plan.DeleteLead {
		t.Fatal("expected single-service transfer to delete source lead")
	}
	if plan.ServiceID != nil {
		t.Fatal("expected no service-only cleanup when deleting lead")
	}
}

func TestPlanQuoteTransferSourceCleanupDeletesOnlyTransferredServiceForMultiServiceLead(t *testing.T) {
	leadID := uuid.New()
	serviceID := uuid.New()
	otherServiceID := uuid.New()
	plan := planQuoteTransferSourceCleanup(leadID, serviceID, []leadrepo.LeadService{{ID: serviceID}, {ID: otherServiceID}})
	if plan.DeleteLead {
		t.Fatal("expected multi-service transfer to preserve source lead")
	}
	if plan.ServiceID == nil || *plan.ServiceID != serviceID {
		t.Fatalf("expected cleanup to target transferred service %s, got %+v", serviceID, plan.ServiceID)
	}
}