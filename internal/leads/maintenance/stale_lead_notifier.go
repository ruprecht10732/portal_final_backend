package maintenance

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"portal_final_backend/internal/notification/inapp"
	"portal_final_backend/platform/logger"
)

// StaleLeadNotifier creates in-app notifications for the assigned agents of
// stale lead services, prompting proactive re-engagement.
//
// Notifications are deduplicated: a new notification is only created when no
// stale-lead notification was created for the same service within the last
// 48 hours.
type StaleLeadNotifier struct {
	pool  *pgxpool.Pool
	notif *inapp.Service
	log   *logger.Logger
}

// NewStaleLeadNotifier creates a new StaleLeadNotifier.
func NewStaleLeadNotifier(pool *pgxpool.Pool, notif *inapp.Service, log *logger.Logger) *StaleLeadNotifier {
	return &StaleLeadNotifier{pool: pool, notif: notif, log: log}
}

// orgMember holds the minimal user data needed to send a notification.
type orgMember struct {
	UserID uuid.UUID
}

const orgMembersQuery = `
SELECT u.id
FROM RAC_users u
JOIN RAC_organization_members m ON m.user_id = u.id
WHERE m.organization_id = $1
`

const cooldownCheckQuery = `
SELECT COUNT(*)
FROM RAC_in_app_notifications
WHERE resource_id = $1
  AND resource_type = 'stale_lead'
  AND created_at > NOW() - INTERVAL '48 hours'
`

// Notify creates in-app notifications for all members of the organisation for
// a stale lead service, unless an identical notification was already sent
// within the cooldown window.
func (n *StaleLeadNotifier) Notify(ctx context.Context, orgID, leadID, serviceID uuid.UUID, staleReason, consumerName, serviceType string) error {
	if n == nil || n.pool == nil {
		return nil
	}

	// Deduplication: skip if a stale_lead notification for this service was
	// already created within the cooldown window.
	var count int64
	pgServiceID := pgtype.UUID{Bytes: serviceID, Valid: true}
	if err := n.pool.QueryRow(ctx, cooldownCheckQuery, pgServiceID).Scan(&count); err != nil {
		return fmt.Errorf("stale lead notify cooldown check: %w", err)
	}
	if count > 0 {
		return nil
	}

	members, err := n.listOrgMembers(ctx, orgID)
	if err != nil {
		return fmt.Errorf("stale lead notify list members: %w", err)
	}
	if len(members) == 0 {
		return nil
	}

	title, content := buildNotificationText(staleReason, consumerName, serviceType)
	resourceID := serviceID

	for _, m := range members {
		if sendErr := n.notif.Send(ctx, inapp.SendParams{
			OrgID:        orgID,
			UserID:       m.UserID,
			Title:        title,
			Content:      content,
			ResourceID:   &resourceID,
			ResourceType: "stale_lead",
			Category:     "warning",
		}); sendErr != nil {
			if n.log != nil {
				n.log.Warn("stale lead notify: failed to send notification",
					"orgId", orgID,
					"userId", m.UserID,
					"serviceId", serviceID,
					"error", sendErr,
				)
			}
		}
	}

	return nil
}

func (n *StaleLeadNotifier) listOrgMembers(ctx context.Context, orgID uuid.UUID) ([]orgMember, error) {
	rows, err := n.pool.Query(ctx, orgMembersQuery, pgtype.UUID{Bytes: orgID, Valid: true})
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []orgMember
	for rows.Next() {
		var m orgMember
		var pgID pgtype.UUID
		if err := rows.Scan(&pgID); err != nil {
			return nil, err
		}
		m.UserID = uuid.UUID(pgID.Bytes)
		members = append(members, m)
	}
	return members, rows.Err()
}

func buildNotificationText(staleReason, consumerName, serviceType string) (title, content string) {
	displayName := consumerName
	if displayName == "" {
		displayName = "Lead"
	}

	switch StaleReason(staleReason) {
	case StaleReasonNoActivity:
		title = fmt.Sprintf("%s – geen activiteit", displayName)
		content = fmt.Sprintf("%s (%s) heeft al meer dan 7 dagen geen activiteit gehad. Neem contact op.", displayName, serviceType)
	case StaleReasonStuckNurturing:
		title = fmt.Sprintf("%s – vastgelopen in Nurturing", displayName)
		content = fmt.Sprintf("%s (%s) staat al meer dan 7 dagen op 'Attempted Contact'. Probeer een andere benadering.", displayName, serviceType)
	case StaleReasonNoQuoteSent:
		title = fmt.Sprintf("%s – nog geen offerte verstuurd", displayName)
		content = fmt.Sprintf("Voor %s (%s) is al meer dan 14 dagen geen offerte verstuurd. Maak een offerte aan.", displayName, serviceType)
	case StaleReasonStaleDraft:
		title = fmt.Sprintf("%s – offerte in concept al 30 dagen", displayName)
		content = fmt.Sprintf("Er staat al meer dan 30 dagen een concept-offerte klaar voor %s (%s). Stuur hem op of verwijder.", displayName, serviceType)
	case StaleReasonNeedsRescheduling:
		title = fmt.Sprintf("%s – afspraak moet worden verzet", displayName)
		content = fmt.Sprintf("%s (%s) staat al meer dan 2 dagen op 'Needs Rescheduling'. Plan een nieuwe afspraak.", displayName, serviceType)
	default:
		title = fmt.Sprintf("%s – lead vereist aandacht", displayName)
		content = fmt.Sprintf("%s (%s) heeft aandacht nodig.", displayName, serviceType)
	}

	return title, content
}
