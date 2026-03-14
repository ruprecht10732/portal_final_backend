package adapters

import (
	"context"
	"testing"

	"portal_final_backend/internal/events"
	identityservice "portal_final_backend/internal/identity/service"
	leadrepo "portal_final_backend/internal/leads/repository"

	"github.com/google/uuid"
)

type fakeInboxLeadRepo struct {
	leadrepo.LeadsRepository
	service    leadrepo.LeadService
	attachment leadrepo.Attachment
	createArgs leadrepo.CreateAttachmentParams
}

func (f *fakeInboxLeadRepo) GetLeadServiceByID(_ context.Context, _ uuid.UUID, _ uuid.UUID) (leadrepo.LeadService, error) {
	return f.service, nil
}

func (f *fakeInboxLeadRepo) CreateAttachment(_ context.Context, params leadrepo.CreateAttachmentParams) (leadrepo.Attachment, error) {
	f.createArgs = params
	if f.attachment.ID == uuid.Nil {
		f.attachment.ID = uuid.New()
	}
	return f.attachment, nil
}

type fakeEventBus struct {
	events.Bus
	published events.Event
}

func (f *fakeEventBus) Publish(_ context.Context, event events.Event) {
	f.published = event
}

func (f *fakeEventBus) PublishSync(context.Context, events.Event) error { return nil }
func (f *fakeEventBus) Subscribe(string, events.Handler) {
	// No-op: subscription behavior is irrelevant for this focused unit test.
}
func (f *fakeEventBus) Shutdown(context.Context) error                 { return nil }

func TestInboxLeadActionsAdapterCreateAttachmentPublishesAttachmentUploaded(t *testing.T) {
	t.Parallel()

	leadID := uuid.New()
	serviceID := uuid.New()
	orgID := uuid.New()
	authorID := uuid.New()
	repo := &fakeInboxLeadRepo{
		service:    leadrepo.LeadService{ID: serviceID, LeadID: leadID},
		attachment: leadrepo.Attachment{ID: uuid.New()},
	}
	bus := &fakeEventBus{}
	adapter := NewInboxLeadActionsAdapter(nil, repo, bus)

	result, err := adapter.CreateAttachment(context.Background(), identityservice.CreateLeadAttachmentParams{
		LeadID:         leadID,
		ServiceID:      serviceID,
		OrganizationID: orgID,
		AuthorID:       authorID,
		FileKey:        "org/lead/file.jpg",
		FileName:       "file.jpg",
		ContentType:    "image/jpeg",
		SizeBytes:      2048,
	})
	if err != nil {
		t.Fatalf("CreateAttachment error: %v", err)
	}
	if result.AttachmentID == uuid.Nil {
		t.Fatal("expected attachment id to be returned")
	}
	published, ok := bus.published.(events.AttachmentUploaded)
	if !ok {
		t.Fatalf("expected AttachmentUploaded event, got %T", bus.published)
	}
	if published.LeadID != leadID {
		t.Fatalf("expected lead id %s, got %s", leadID, published.LeadID)
	}
	if published.LeadServiceID != serviceID {
		t.Fatalf("expected service id %s, got %s", serviceID, published.LeadServiceID)
	}
	if published.TenantID != orgID {
		t.Fatalf("expected tenant id %s, got %s", orgID, published.TenantID)
	}
	if published.FileKey != "org/lead/file.jpg" {
		t.Fatalf("expected file key org/lead/file.jpg, got %q", published.FileKey)
	}
	if published.ContentType != "image/jpeg" {
		t.Fatalf("expected content type image/jpeg, got %q", published.ContentType)
	}
	if published.SizeBytes != 2048 {
		t.Fatalf("expected sizeBytes 2048, got %d", published.SizeBytes)
	}
}
