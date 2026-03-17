package webhook

import (
	"context"
	"strings"

	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
)

const whatsAppAccountJIDSyncFailedLog = "whatsapp webhook account jid sync failed"

type whatsAppAccountJIDSyncer interface {
	RefreshOrganizationAccountJID(ctx context.Context, organizationID uuid.UUID, observedDeviceID string) error
	RefreshAgentAccountJID(ctx context.Context, observedDeviceID string) error
}

type whatsAppDeviceBinding struct {
	DeviceID   string
	AccountJID string
}

type whatsAppAccountJIDStore interface {
	GetOrganizationWhatsAppBinding(ctx context.Context, organizationID uuid.UUID) (whatsAppDeviceBinding, error)
	UpdateOrganizationWhatsAppAccountJID(ctx context.Context, organizationID uuid.UUID, accountJID string) error
	GetAgentWhatsAppBinding(ctx context.Context) (whatsAppDeviceBinding, error)
	UpdateAgentWhatsAppAccountJID(ctx context.Context, deviceID string, accountJID string) error
}

type whatsAppDeviceInfoReader interface {
	GetDeviceInfo(ctx context.Context, deviceID string) (*whatsapp.DeviceInfoResponse, error)
}

type deviceAccountJIDSyncer struct {
	store   whatsAppAccountJIDStore
	devices whatsAppDeviceInfoReader
	log     *logger.Logger
}

func newWhatsAppAccountJIDSyncer(store whatsAppAccountJIDStore, devices whatsAppDeviceInfoReader, log *logger.Logger) whatsAppAccountJIDSyncer {
	if store == nil || devices == nil {
		return nil
	}
	return &deviceAccountJIDSyncer{store: store, devices: devices, log: log}
}

func (s *deviceAccountJIDSyncer) RefreshOrganizationAccountJID(ctx context.Context, organizationID uuid.UUID, observedDeviceID string) error {
	binding, err := s.store.GetOrganizationWhatsAppBinding(ctx, organizationID)
	if err != nil {
		s.logWarn(ctx, whatsAppAccountJIDSyncFailedLog, "scope", "organization", "organization_id", organizationID.String(), "observed_device_id", strings.TrimSpace(observedDeviceID), "error", err)
		return err
	}
	resolved, source, err := s.resolveAccountJID(ctx, binding, observedDeviceID)
	if err != nil || resolved == "" || sameWhatsAppAccountJID(binding.AccountJID, resolved) {
		if err != nil {
			s.logWarn(ctx, whatsAppAccountJIDSyncFailedLog, "scope", "organization", "organization_id", organizationID.String(), "device_id", strings.TrimSpace(binding.DeviceID), "observed_device_id", strings.TrimSpace(observedDeviceID), "error", err)
		}
		return err
	}
	if err := s.store.UpdateOrganizationWhatsAppAccountJID(ctx, organizationID, resolved); err != nil {
		s.logWarn(ctx, whatsAppAccountJIDSyncFailedLog, "scope", "organization", "organization_id", organizationID.String(), "device_id", strings.TrimSpace(binding.DeviceID), "observed_device_id", strings.TrimSpace(observedDeviceID), "resolved_account_jid", resolved, "error", err)
		return err
	}
	s.logInfo(ctx, "whatsapp webhook account jid synchronized", "scope", "organization", "organization_id", organizationID.String(), "device_id", strings.TrimSpace(binding.DeviceID), "observed_device_id", strings.TrimSpace(observedDeviceID), "previous_account_jid", strings.TrimSpace(binding.AccountJID), "resolved_account_jid", resolved, "resolution_source", source)
	return nil
}

func (s *deviceAccountJIDSyncer) RefreshAgentAccountJID(ctx context.Context, observedDeviceID string) error {
	binding, err := s.store.GetAgentWhatsAppBinding(ctx)
	if err != nil {
		s.logWarn(ctx, whatsAppAccountJIDSyncFailedLog, "scope", "agent", "observed_device_id", strings.TrimSpace(observedDeviceID), "error", err)
		return err
	}
	resolved, source, err := s.resolveAccountJID(ctx, binding, observedDeviceID)
	if err != nil || resolved == "" || sameWhatsAppAccountJID(binding.AccountJID, resolved) {
		if err != nil {
			s.logWarn(ctx, whatsAppAccountJIDSyncFailedLog, "scope", "agent", "device_id", strings.TrimSpace(binding.DeviceID), "observed_device_id", strings.TrimSpace(observedDeviceID), "error", err)
		}
		return err
	}
	if err := s.store.UpdateAgentWhatsAppAccountJID(ctx, binding.DeviceID, resolved); err != nil {
		s.logWarn(ctx, whatsAppAccountJIDSyncFailedLog, "scope", "agent", "device_id", strings.TrimSpace(binding.DeviceID), "observed_device_id", strings.TrimSpace(observedDeviceID), "resolved_account_jid", resolved, "error", err)
		return err
	}
	s.logInfo(ctx, "whatsapp webhook account jid synchronized", "scope", "agent", "device_id", strings.TrimSpace(binding.DeviceID), "observed_device_id", strings.TrimSpace(observedDeviceID), "previous_account_jid", strings.TrimSpace(binding.AccountJID), "resolved_account_jid", resolved, "resolution_source", source)
	return nil
}

func (s *deviceAccountJIDSyncer) resolveAccountJID(ctx context.Context, binding whatsAppDeviceBinding, observedDeviceID string) (string, string, error) {
	if current := normalizeWhatsAppAccountJID(binding.AccountJID); current != "" && sameWhatsAppAccountJID(current, observedDeviceID) {
		return current, "stored_account_jid", nil
	}
	if observed := normalizeWhatsAppAccountJID(observedDeviceID); observed != "" {
		return observed, "observed_device_id", nil
	}
	if strings.TrimSpace(binding.DeviceID) == "" {
		return "", "", nil
	}
	info, err := s.devices.GetDeviceInfo(ctx, binding.DeviceID)
	if err != nil || info == nil {
		return "", "", err
	}
	return normalizeWhatsAppAccountJID(info.JID), "provider_device_info", nil
}

func (s *deviceAccountJIDSyncer) logInfo(ctx context.Context, message string, args ...any) {
	if s == nil || s.log == nil {
		return
	}
	s.log.WithContext(ctx).Info(message, args...)
}

func (s *deviceAccountJIDSyncer) logWarn(ctx context.Context, message string, args ...any) {
	if s == nil || s.log == nil {
		return
	}
	s.log.WithContext(ctx).Warn(message, args...)
}

func sameWhatsAppAccountJID(left string, right string) bool {
	normalizedLeft := normalizeWhatsAppAccountJID(left)
	normalizedRight := normalizeWhatsAppAccountJID(right)
	return normalizedLeft != "" && normalizedLeft == normalizedRight
}

func normalizeWhatsAppAccountJID(raw string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "agent_") || strings.HasPrefix(trimmed, "org_") {
		return ""
	}
	if at := strings.Index(trimmed, "@"); at >= 0 {
		base := strings.TrimPrefix(strings.TrimSpace(trimmed[:at]), "+")
		if isNumericPhoneIdentifier(base) {
			return base + "@s.whatsapp.net"
		}
		return ""
	}
	base := strings.TrimPrefix(trimmed, "+")
	if !isNumericPhoneIdentifier(base) {
		return ""
	}
	return base + "@s.whatsapp.net"
}

func isNumericPhoneIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
