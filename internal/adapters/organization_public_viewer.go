package adapters

import (
	"context"
	"strings"

	identitysvc "portal_final_backend/internal/identity/service"
	"portal_final_backend/internal/leads/ports"

	"github.com/google/uuid"
)

// OrganizationPublicAdapter exposes organization contact info for the public lead portal.
type OrganizationPublicAdapter struct {
	svc *identitysvc.Service
}

func NewOrganizationPublicAdapter(svc *identitysvc.Service) *OrganizationPublicAdapter {
	return &OrganizationPublicAdapter{svc: svc}
}

func (a *OrganizationPublicAdapter) GetPublicPhone(ctx context.Context, organizationID uuid.UUID) (string, error) {
	org, err := a.svc.GetOrganization(ctx, organizationID)
	if err != nil {
		return "", err
	}
	if org.Phone == nil {
		return "", nil
	}
	return normalizeWhatsAppNL(*org.Phone), nil
}

func normalizeWhatsAppNL(phone string) string {
	trimmed := strings.TrimSpace(phone)
	if trimmed == "" {
		return ""
	}

	digits := make([]rune, 0, len(trimmed))
	for _, ch := range trimmed {
		if ch >= '0' && ch <= '9' {
			digits = append(digits, ch)
		}
	}

	if len(digits) == 0 {
		return ""
	}

	normalized := string(digits)
	if strings.HasPrefix(normalized, "00") {
		normalized = normalized[2:]
	}
	if strings.HasPrefix(normalized, "0") {
		normalized = "31" + normalized[1:]
	}

	return normalized
}

var _ ports.OrganizationPublicViewer = (*OrganizationPublicAdapter)(nil)
