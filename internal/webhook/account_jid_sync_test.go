package webhook

import (
	"context"
	"errors"
	"testing"

	"portal_final_backend/internal/whatsapp"

	"github.com/google/uuid"
)

const syncTestAccountJID = "31686261598@s.whatsapp.net"

type accountJIDSyncStoreStub struct {
	orgBinding         whatsAppDeviceBinding
	orgBindingErr      error
	agentBinding       whatsAppDeviceBinding
	agentBindingErr    error
	updatedOrgID       uuid.UUID
	updatedOrgJID      string
	updatedAgentDevice string
	updatedAgentJID    string
}

func (s *accountJIDSyncStoreStub) GetOrganizationWhatsAppBinding(context.Context, uuid.UUID) (whatsAppDeviceBinding, error) {
	if s.orgBindingErr != nil {
		return whatsAppDeviceBinding{}, s.orgBindingErr
	}
	return s.orgBinding, nil
}

func (s *accountJIDSyncStoreStub) UpdateOrganizationWhatsAppAccountJID(_ context.Context, organizationID uuid.UUID, accountJID string) error {
	s.updatedOrgID = organizationID
	s.updatedOrgJID = accountJID
	return nil
}

func (s *accountJIDSyncStoreStub) GetAgentWhatsAppBinding(context.Context) (whatsAppDeviceBinding, error) {
	if s.agentBindingErr != nil {
		return whatsAppDeviceBinding{}, s.agentBindingErr
	}
	return s.agentBinding, nil
}

func (s *accountJIDSyncStoreStub) UpdateAgentWhatsAppAccountJID(_ context.Context, deviceID string, accountJID string) error {
	s.updatedAgentDevice = deviceID
	s.updatedAgentJID = accountJID
	return nil
}

type accountJIDSyncDeviceStub struct {
	info       *whatsapp.DeviceInfoResponse
	err        error
	lastDevice string
}

func (s *accountJIDSyncDeviceStub) GetDeviceInfo(_ context.Context, deviceID string) (*whatsapp.DeviceInfoResponse, error) {
	s.lastDevice = deviceID
	return s.info, s.err
}

func TestDeviceAccountJIDSyncerRefreshesOrganizationFromDeviceInfo(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	store := &accountJIDSyncStoreStub{orgBinding: whatsAppDeviceBinding{DeviceID: "org_123"}}
	devices := &accountJIDSyncDeviceStub{info: &whatsapp.DeviceInfoResponse{JID: syncTestAccountJID}}
	syncer := newWhatsAppAccountJIDSyncer(store, devices, nil)

	if err := syncer.RefreshOrganizationAccountJID(context.Background(), orgID, "org_123"); err != nil {
		t.Fatalf("expected org jid refresh to succeed, got %v", err)
	}
	if devices.lastDevice != "org_123" {
		t.Fatalf("expected device info lookup for org_123, got %q", devices.lastDevice)
	}
	if store.updatedOrgID != orgID || store.updatedOrgJID != syncTestAccountJID {
		t.Fatalf("expected org jid update, got id=%q jid=%q", store.updatedOrgID, store.updatedOrgJID)
	}
}

func TestDeviceAccountJIDSyncerSkipsLookupWhenObservedAgentJIDAlreadyMatches(t *testing.T) {
	t.Parallel()

	store := &accountJIDSyncStoreStub{agentBinding: whatsAppDeviceBinding{DeviceID: "agent_123", AccountJID: syncTestAccountJID}}
	devices := &accountJIDSyncDeviceStub{}
	syncer := newWhatsAppAccountJIDSyncer(store, devices, nil)

	if err := syncer.RefreshAgentAccountJID(context.Background(), "+31686261598"); err != nil {
		t.Fatalf("expected agent jid refresh to succeed, got %v", err)
	}
	if devices.lastDevice != "" {
		t.Fatalf("expected no device lookup when jid already matches, got %q", devices.lastDevice)
	}
	if store.updatedAgentDevice != "" || store.updatedAgentJID != "" {
		t.Fatalf("expected no agent update when jid already matches, got device=%q jid=%q", store.updatedAgentDevice, store.updatedAgentJID)
	}
}

func TestDeviceAccountJIDSyncerPropagatesBindingErrors(t *testing.T) {
	t.Parallel()

	syncer := newWhatsAppAccountJIDSyncer(&accountJIDSyncStoreStub{orgBindingErr: errors.New("boom")}, &accountJIDSyncDeviceStub{}, nil)
	if err := syncer.RefreshOrganizationAccountJID(context.Background(), uuid.New(), "org_123"); err == nil {
		t.Fatal("expected binding error to be returned")
	}
}
