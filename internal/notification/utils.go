package notification

import (
	"context"
	notificationdb "portal_final_backend/internal/notification/db"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func toPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func optionalTextValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func (m *Module) resolveOrganizationName(ctx context.Context, orgID uuid.UUID) string {
	if orgID == uuid.Nil {
		return ""
	}
	if cached, ok := m.orgNameCache.Load(orgID); ok {
		entry := cached.(cachedOrgName)
		if time.Now().Before(entry.expiresAt) {
			return entry.name
		}
		m.orgNameCache.Delete(orgID)
	}
	if m.queries == nil {
		return ""
	}
	name, err := m.queries.GetNotificationOrganizationName(ctx, toPgUUID(orgID))
	if err != nil {
		return ""
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	m.orgNameCache.Store(orgID, cachedOrgName{name: name, expiresAt: time.Now().Add(10 * time.Minute)})
	return name
}

// resolveLeadDetails fetches first/last name, address and service type for a lead.
func (m *Module) resolveLeadDetails(ctx context.Context, leadID uuid.UUID, orgID uuid.UUID) *leadDetails {
	if m.queries == nil || leadID == uuid.Nil {
		return nil
	}
	row, err := m.queries.GetNotificationLeadDetails(ctx, notificationdb.GetNotificationLeadDetailsParams{
		ID:             toPgUUID(leadID),
		OrganizationID: toPgUUID(orgID),
	})
	if err != nil {
		return nil
	}
	return &leadDetails{
		FirstName:   row.ConsumerFirstName,
		LastName:    row.ConsumerLastName,
		Phone:       row.ConsumerPhone,
		Email:       optionalTextValue(row.ConsumerEmail),
		Street:      row.AddressStreet,
		HouseNumber: row.AddressHouseNumber,
		ZipCode:     row.AddressZipCode,
		City:        row.AddressCity,
		ServiceType: row.ServiceType,
		PublicToken: optionalTextValue(row.PublicToken),
	}
}

func nilIfUUIDNil(value uuid.UUID) *uuid.UUID {
	if value == uuid.Nil {
		return nil
	}
	return &value
}

func (m *Module) buildURL(path string, tokenValue string) string {
	base := strings.TrimRight(m.cfg.GetAppBaseURL(), "/")
	return base + path + "?token=" + tokenValue
}

func (m *Module) buildPublicURL(path string, tokenValue string) string {
	base := strings.TrimRight(m.cfg.GetPublicBaseURL(), "/")
	return base + path + "/" + tokenValue
}

func nilIfEmptyString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
