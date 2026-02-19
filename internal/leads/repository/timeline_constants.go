package repository

// ActorType constants identify the category of entity that produced a timeline event.
const (
	ActorTypeUser   = "User"   // Human user acting through the application UI
	ActorTypeAI     = "AI"     // AI agent (Gatekeeper, Dispatcher, Estimator, etc.)
	ActorTypeSystem = "System" // Internal system process (Orchestrator, reconciler, etc.)
	ActorTypeLead   = "Lead"   // The lead/customer acting via the public portal
)

// System actor name constants for AI and system actors.
// Human user actor names come from the user record (email address).
const (
	ActorNameGatekeeper      = "Gatekeeper"
	ActorNameOrchestrator    = "Orchestrator"
	ActorNameDispatcher      = "Dispatcher"
	ActorNameEstimator       = "Estimator"
	ActorNameStateReconciler = "StateReconciler"
	ActorNameKlant           = "Klant"              // Customer self-service via public portal
	ActorNamePhotoAnalysis   = "Foto-analyse Agent" // Photo analysis AI agent
)

// EventType constants identify the nature of a timeline event.
const (
	EventTypeNote                 = "note"
	EventTypeCallLog              = "call_log"
	EventTypeCallOutcome          = "call_outcome"
	EventTypeStageChange          = "stage_change"
	EventTypeAI                   = "ai"
	EventTypeAnalysis             = "analysis"
	EventTypeAlert                = "alert"
	EventTypeStateReconciled      = "service_state_reconciled"
	EventTypePreferencesUpdated   = "preferences_updated"
	EventTypeInfoAdded            = "info_added"
	EventTypeAppointmentRequested = "appointment_requested"
	EventTypeServiceTypeChange    = "service_type_change"
	EventTypeLeadUpdate                = "lead_update"
	EventTypePartnerSearch             = "partner_search"
	EventTypePhotoAnalysisCompleted    = "photo_analysis_completed"
	EventTypeVisitCompleted            = "visit_completed"
)

// EventTitle constants are the human-readable labels shown in the timeline UI.
const (
	EventTitleNoteAdded            = "Notitie toegevoegd"
	EventTitleCallLog              = "Gesprek geregistreerd"
	EventTitleCallOutcome          = "Belresultaat"
	EventTitleStageUpdated         = "Fase bijgewerkt"
	EventTitleAutoDisqualified     = "Auto-Disqualified"
	EventTitleDispatcherFailed     = "Partner matching mislukt"
	EventTitlePhotoAnalysisFailed  = "Foto-analyse mislukt"
	EventTitleManualIntervention   = "Handmatige interventie vereist"
	EventTitleQuoteAccepted        = "Offerte Geaccepteerd"
	EventTitleQuoteRejected        = "Offerte Afgewezen"
	EventTitleStateReconciled      = "Status automatisch gecorrigeerd"
	EventTitleGatekeeperAnalysis   = "Gatekeeper analyse voltooid"
	EventTitleGatekeeperFallback   = "Gatekeeper-triage (fallback)"
	EventTitleLeadScoreUpdated     = "Leadscore bijgewerkt"
	EventTitleServiceTypeUpdated   = "Diensttype bijgewerkt"
	EventTitleLeadDetailsUpdated   = "Leadgegevens bijgewerkt"
	EventTitlePartnerSearch        = "Partnerzoekactie"
	EventTitleEstimationSaved      = "Schatting opgeslagen"
	EventTitleEstimationMissing    = "Schatting ontbreekt"
	EventTitlePreferencesUpdated        = "Voorkeuren bijgewerkt"
	EventTitleCustomerInfo              = "Klant update"
	EventTitleAppointmentRequested      = "Inspectie aangevraagd"
	EventTitlePhotoAnalysisCompleted    = "Foto-analyse voltooid"
)
