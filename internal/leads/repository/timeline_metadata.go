package repository

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// toMap serialises any struct to map[string]any via JSON round-trip.
// This ensures keys exactly match the JSON tags while keeping the
// CreateTimelineEventParams.Metadata field as its existing map[string]any type.
func toMap(v any) map[string]any {
	b, _ := json.Marshal(v)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

// NoteMetadata is the typed metadata for EventTypeNote events.
type NoteMetadata struct {
	NoteID   uuid.UUID `json:"noteId"`
	NoteType string    `json:"noteType"`
}

func (m NoteMetadata) ToMap() map[string]any { return toMap(m) }

// CallLogMetadata is the typed metadata for EventTypeCallLog events.
type CallLogMetadata struct {
	CallOutcome            *string    `json:"callOutcome,omitempty"`
	NoteCreated            bool       `json:"noteCreated"`
	StatusUpdated          *string    `json:"statusUpdated,omitempty"`
	PipelineStageUpdated   *string    `json:"pipelineStageUpdated,omitempty"`
	AppointmentBooked      *time.Time `json:"appointmentBooked,omitempty"`
	AppointmentRescheduled *time.Time `json:"appointmentRescheduled,omitempty"`
	AppointmentCancelled   bool       `json:"appointmentCancelled,omitempty"`
}

func (m CallLogMetadata) ToMap() map[string]any { return toMap(m) }

// CallOutcomeMetadata is the typed metadata for EventTypeCallOutcome events.
type CallOutcomeMetadata struct {
	Outcome string `json:"outcome"`
	Notes   string `json:"notes,omitempty"`
}

func (m CallOutcomeMetadata) ToMap() map[string]any { return toMap(m) }

// StageChangeMetadata is the typed metadata for EventTypeStageChange events.
// Callers may extend the returned map with additional context (e.g. analysis).
type StageChangeMetadata struct {
	OldStage string `json:"oldStage"`
	NewStage string `json:"newStage"`
}

func (m StageChangeMetadata) ToMap() map[string]any { return toMap(m) }

// AIAnalysisMetadata is the typed metadata for EventTypeAI (gatekeeper analysis) events.
type AIAnalysisMetadata struct {
	UrgencyLevel            string   `json:"urgencyLevel"`
	RecommendedAction       string   `json:"recommendedAction"`
	LeadQuality             string   `json:"leadQuality"`
	SuggestedContactMessage string   `json:"suggestedContactMessage,omitempty"`
	PreferredContactChannel string   `json:"preferredContactChannel,omitempty"`
	MissingInformation      []string `json:"missingInformation,omitempty"`
	Fallback                bool     `json:"fallback,omitempty"`
}

func (m AIAnalysisMetadata) ToMap() map[string]any { return toMap(m) }

// LeadScoreMetadata is the typed metadata for EventTypeAnalysis (score update) events.
type LeadScoreMetadata struct {
	LeadScore        int    `json:"leadScore"`
	LeadScorePreAI   int    `json:"leadScorePreAI"`
	LeadScoreVersion string `json:"leadScoreVersion"`
}

func (m LeadScoreMetadata) ToMap() map[string]any { return toMap(m) }

// AlertMetadata is the typed metadata for EventTypeAlert events.
type AlertMetadata struct {
	Trigger      string `json:"trigger"`
	ErrorCode    string `json:"errorCode,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

func (m AlertMetadata) ToMap() map[string]any { return toMap(m) }

// AutoDisqualifyMetadata is the typed metadata for auto-disqualification stage_change events.
type AutoDisqualifyMetadata struct {
	LeadQuality       string    `json:"leadQuality"`
	RecommendedAction string    `json:"recommendedAction"`
	AnalysisID        uuid.UUID `json:"analysisId"`
	Reason            string    `json:"reason"`
}

func (m AutoDisqualifyMetadata) ToMap() map[string]any { return toMap(m) }

// ManualInterventionMetadata is the typed metadata for manual-intervention alert events.
type ManualInterventionMetadata struct {
	PreviousStage string         `json:"previous_stage"`
	Trigger       string         `json:"trigger"`
	Drafts        map[string]any `json:"drafts"`
}

func (m ManualInterventionMetadata) ToMap() map[string]any { return toMap(m) }

// StateReconciledMetadata is the typed metadata for EventTypeStateReconciled events.
type StateReconciledMetadata struct {
	ReasonCode string         `json:"reasonCode"`
	Trigger    string         `json:"trigger"`
	OldStage   string         `json:"oldStage"`
	NewStage   string         `json:"newStage"`
	OldStatus  string         `json:"oldStatus"`
	NewStatus  string         `json:"newStatus"`
	Evidence   map[string]any `json:"evidence"`
}

func (m StateReconciledMetadata) ToMap() map[string]any { return toMap(m) }

// QuoteEventMetadata is the typed metadata for quote-related stage_change events.
type QuoteEventMetadata struct {
	QuoteID uuid.UUID `json:"quoteId"`
	Reason  string    `json:"reason,omitempty"`
}

func (m QuoteEventMetadata) ToMap() map[string]any { return toMap(m) }

// PreferencesMetadata is the typed metadata for EventTypePreferencesUpdated events.
type PreferencesMetadata struct {
	Budget       string `json:"budget,omitempty"`
	Timeframe    string `json:"timeframe,omitempty"`
	Availability string `json:"availability,omitempty"`
	ExtraNotes   string `json:"extraNotes,omitempty"`
}

func (m PreferencesMetadata) ToMap() map[string]any { return toMap(m) }

// CustomerInfoMetadata is the typed metadata for EventTypeInfoAdded events.
type CustomerInfoMetadata struct {
	Text string `json:"text"`
}

func (m CustomerInfoMetadata) ToMap() map[string]any { return toMap(m) }

// AppointmentRequestMetadata is the typed metadata for EventTypeAppointmentRequested events.
type AppointmentRequestMetadata struct {
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
}

func (m AppointmentRequestMetadata) ToMap() map[string]any { return toMap(m) }

// ServiceTypeChangeMetadata is the typed metadata for EventTypeServiceTypeChange events.
type ServiceTypeChangeMetadata struct {
	OldServiceType string `json:"oldServiceType,omitempty"`
	NewServiceType string `json:"newServiceType,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

func (m ServiceTypeChangeMetadata) ToMap() map[string]any { return toMap(m) }

// LeadUpdateMetadata is the typed metadata for EventTypeLeadUpdate events.
type LeadUpdateMetadata struct {
	UpdatedFields []string `json:"updatedFields"`
	Confidence    *float64 `json:"confidence,omitempty"`
}

func (m LeadUpdateMetadata) ToMap() map[string]any { return toMap(m) }

// EstimationMetadata is the typed metadata for estimation EventTypeAnalysis events.
type EstimationMetadata struct {
	Scope      string `json:"scope"`
	PriceRange string `json:"priceRange"`
	Notes      string `json:"notes,omitempty"`
}

func (m EstimationMetadata) ToMap() map[string]any { return toMap(m) }

// PartnerSearchMetadata is the typed metadata for EventTypePartnerSearch events.
type PartnerSearchMetadata struct {
	ServiceType string `json:"serviceType"`
	ZipCode     string `json:"zipCode"`
	RadiusKm    int    `json:"radiusKm"`
	MatchCount  int    `json:"matchCount"`
}

func (m PartnerSearchMetadata) ToMap() map[string]any { return toMap(m) }
