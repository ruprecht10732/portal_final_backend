// Package management — feed enrichment logic for the Smart Activity Feed.
// Adds sentiment analysis, aggregation formatting, suggested actions, and metadata pass-through.
package management

import (
	"encoding/json"
	"fmt"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/transport"
)

// enrichFeedItem enriches a mapped ActivityFeedItem with sentiment, grouped titles,
// suggested actions, and parsed metadata from the raw DB entry.
func enrichFeedItem(item *transport.ActivityFeedItem, entry *repository.ActivityFeedEntry) {
	item.GroupCount = entry.GroupCount
	item.ActorName = entry.ActorName

	// --- Sentiment ---
	item.Sentiment = mapSentiment(entry.EventType)

	// --- Aggregation: pluralise title when grouped ---
	if entry.GroupCount > 1 {
		if plural := pluralTitle(entry.EventType, entry.GroupCount); plural != "" {
			item.Title = plural
		}
	}

	// --- Suggested Action (CTA) ---
	action, link := suggestedAction(entry.EventType, entry.EntityID.String())
	item.SuggestedAction = action
	item.ActionLink = link

	// --- Metadata pass-through ---
	if len(entry.RawMetadata) > 0 {
		var meta map[string]any
		if err := json.Unmarshal(entry.RawMetadata, &meta); err == nil && len(meta) > 0 {
			item.Metadata = meta
		}
	}
}

// mapSentiment returns the sentiment label for a given event type.
func mapSentiment(eventType string) string {
	switch eventType {
	// Positive
	case "quote_accepted", "partner_offer_accepted":
		return "positive"
	// Negative
	case "quote_rejected", "partner_offer_rejected", "lead_lost":
		return "negative"
	// Urgent
	case "manual_intervention", "gatekeeper_rejected":
		return "urgent"
	// Info
	case "appointment_scheduled", "appointment_created", "appointment_updated",
		"appointment_upcoming", "lead_created", "lead_assigned":
		return "info"
	// Neutral — default bucket for notes, calls, generic events
	default:
		return "neutral"
	}
}

// pluralTitle returns a Dutch plural title for grouped events.
// Returns empty string if no specific plural mapping exists.
func pluralTitle(eventType string, count int) string {
	switch eventType {
	case "note_added":
		return fmt.Sprintf("%d notities toegevoegd", count)
	case "call_logged":
		return fmt.Sprintf("%d gesprekken gelogd", count)
	case "attachment_uploaded":
		return fmt.Sprintf("%d foto's geupload", count)
	case "lead_viewed":
		return fmt.Sprintf("%d leads bekeken", count)
	case "lead_updated":
		return fmt.Sprintf("%d leads bijgewerkt", count)
	case "lead_created":
		return fmt.Sprintf("%d nieuwe leads aangemaakt", count)
	case "quote_sent":
		return fmt.Sprintf("%d offertes verzonden", count)
	case "quote_viewed":
		return fmt.Sprintf("%d offertes bekeken", count)
	case "quote_accepted":
		return fmt.Sprintf("%d offertes geaccepteerd", count)
	case "quote_rejected":
		return fmt.Sprintf("%d offertes afgewezen", count)
	case "appointment_created":
		return fmt.Sprintf("%d afspraken aangemaakt", count)
	case "appointment_updated":
		return fmt.Sprintf("%d afspraken bijgewerkt", count)
	case "analysis_complete":
		return fmt.Sprintf("%d analyses voltooid", count)
	case "photo_analysis_complete", "photo_analysis_completed":
		return fmt.Sprintf("%d foto-analyses voltooid", count)
	case "status_change":
		return fmt.Sprintf("%d statuswijzigingen", count)
	default:
		return ""
	}
}

// suggestedAction returns a CTA label and deep link for event types that benefit from
// an immediate follow-up action.
func suggestedAction(eventType string, entityID string) (label string, link string) {
	switch eventType {
	case "quote_viewed":
		return "Bel klant", fmt.Sprintf("leads/%s/call", entityID)
	case "manual_intervention":
		return "Triage bekijken", fmt.Sprintf("leads/%s/triage", entityID)
	case "gatekeeper_rejected":
		return "Triage bekijken", fmt.Sprintf("leads/%s/triage", entityID)
	case "partner_offer_rejected":
		return "Nieuwe partner zoeken", fmt.Sprintf("leads/%s/dispatch", entityID)
	case "appointment_created":
		return "Bekijk agenda", fmt.Sprintf("appointments/%s", entityID)
	case "quote_accepted":
		return "Bekijk offerte", fmt.Sprintf("offertes/%s", entityID)
	case "lead_created":
		return "Bekijk lead", fmt.Sprintf("leads/%s", entityID)
	default:
		return "", ""
	}
}
