package agent

import (
	"context"
	"testing"

	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"

	"github.com/google/uuid"
)

const (
	testCustomerEmail   = "customer@example.com"
	expectedNilErrorFmt = "expected nil error, got %v"
)

type emailReplyLookupStoreStub struct {
	leadByID             map[uuid.UUID]repository.Lead
	serviceByID          map[uuid.UUID]repository.LeadService
	currentServiceByLead map[uuid.UUID]repository.LeadService
	leadByEmail          map[string]repository.LeadSummary
	leadLookupOrder      []string
	serviceLookupOrder   []string
}

func (s *emailReplyLookupStoreStub) GetByID(_ context.Context, id uuid.UUID, _ uuid.UUID) (repository.Lead, error) {
	s.leadLookupOrder = append(s.leadLookupOrder, id.String())
	if lead, ok := s.leadByID[id]; ok {
		return lead, nil
	}
	return repository.Lead{}, repository.ErrNotFound
}

func (s *emailReplyLookupStoreStub) GetByPhoneOrEmail(_ context.Context, _ string, email string, _ uuid.UUID) (*repository.LeadSummary, []repository.LeadService, error) {
	if summary, ok := s.leadByEmail[email]; ok {
		return &summary, nil, nil
	}
	return nil, nil, nil
}

func (s *emailReplyLookupStoreStub) GetLeadServiceByID(_ context.Context, id uuid.UUID, _ uuid.UUID) (repository.LeadService, error) {
	s.serviceLookupOrder = append(s.serviceLookupOrder, id.String())
	if service, ok := s.serviceByID[id]; ok {
		return service, nil
	}
	return repository.LeadService{}, repository.ErrServiceNotFound
}

func (s *emailReplyLookupStoreStub) GetCurrentLeadService(_ context.Context, leadID uuid.UUID, _ uuid.UUID) (repository.LeadService, error) {
	s.serviceLookupOrder = append(s.serviceLookupOrder, "current:"+leadID.String())
	if service, ok := s.currentServiceByLead[leadID]; ok {
		return service, nil
	}
	return repository.LeadService{}, repository.ErrServiceNotFound
}

func TestResolveEmailReplyLeadAndServiceHydratesLeadFromServiceFirst(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	leadID := uuid.New()
	serviceID := uuid.New()
	store := &emailReplyLookupStoreStub{
		leadByID: map[uuid.UUID]repository.Lead{
			leadID: {ID: leadID, ConsumerEmail: emailStringPtr(testCustomerEmail)},
		},
		serviceByID: map[uuid.UUID]repository.LeadService{
			serviceID: {ID: serviceID, LeadID: leadID, ServiceType: "Warmtepomp"},
		},
	}

	lead, service, err := resolveEmailReplyLeadAndService(context.Background(), store, ports.EmailReplyInput{
		OrganizationID: orgID,
		LeadServiceID:  &serviceID,
	})
	if err != nil {
		t.Fatalf(expectedNilErrorFmt, err)
	}
	if lead == nil || lead.ID != leadID {
		t.Fatalf("expected hydrated lead %s, got %#v", leadID, lead)
	}
	if service == nil || service.ID != serviceID {
		t.Fatalf("expected service %s, got %#v", serviceID, service)
	}
	if len(store.serviceLookupOrder) == 0 || store.serviceLookupOrder[0] != serviceID.String() {
		t.Fatalf("expected service lookup by explicit service id first, got %v", store.serviceLookupOrder)
	}
}

func TestResolveEmailReplyLeadAndServiceFallsBackToLeadThenCurrentService(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	leadID := uuid.New()
	serviceID := uuid.New()
	store := &emailReplyLookupStoreStub{
		leadByEmail: map[string]repository.LeadSummary{
			testCustomerEmail: {ID: leadID},
		},
		leadByID: map[uuid.UUID]repository.Lead{
			leadID: {ID: leadID},
		},
		currentServiceByLead: map[uuid.UUID]repository.LeadService{
			leadID: {ID: serviceID, LeadID: leadID, ServiceType: "Isolatie"},
		},
	}

	lead, service, err := resolveEmailReplyLeadAndService(context.Background(), store, ports.EmailReplyInput{
		OrganizationID: orgID,
		CustomerEmail:  testCustomerEmail,
	})
	if err != nil {
		t.Fatalf(expectedNilErrorFmt, err)
	}
	if lead == nil || lead.ID != leadID {
		t.Fatalf("expected lead %s, got %#v", leadID, lead)
	}
	if service == nil || service.ID != serviceID {
		t.Fatalf("expected current service %s, got %#v", serviceID, service)
	}
	if len(store.serviceLookupOrder) != 1 || store.serviceLookupOrder[0] != "current:"+leadID.String() {
		t.Fatalf("expected current-service lookup after lead resolution, got %v", store.serviceLookupOrder)
	}
}

func TestResolveEmailReplyLeadAndServiceIgnoresNotFoundServiceAndStillUsesLead(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	leadID := uuid.New()
	missingServiceID := uuid.New()
	currentServiceID := uuid.New()
	store := &emailReplyLookupStoreStub{
		leadByID: map[uuid.UUID]repository.Lead{
			leadID: {ID: leadID},
		},
		currentServiceByLead: map[uuid.UUID]repository.LeadService{
			leadID: {ID: currentServiceID, LeadID: leadID},
		},
	}

	lead, service, err := resolveEmailReplyLeadAndService(context.Background(), store, ports.EmailReplyInput{
		OrganizationID: orgID,
		LeadID:         &leadID,
		LeadServiceID:  &missingServiceID,
	})
	if err != nil {
		t.Fatalf(expectedNilErrorFmt, err)
	}
	if lead == nil || lead.ID != leadID {
		t.Fatalf("expected lead %s, got %#v", leadID, lead)
	}
	if service == nil || service.ID != currentServiceID {
		t.Fatalf("expected current service %s after missing explicit service, got %#v", currentServiceID, service)
	}
}

func emailStringPtr(value string) *string { return &value }
