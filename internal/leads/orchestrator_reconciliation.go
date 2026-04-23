package leads

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/repository"
)

func (o *Orchestrator) OnQuoteCreated(ctx context.Context, evt events.QuoteCreated) {
	if evt.LeadServiceID == nil {
		return
	}
	o.reconcileServiceState(ctx, evt.LeadID, *evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), true)
}
func (o *Orchestrator) OnQuoteDeleted(ctx context.Context, evt events.QuoteDeleted) {
	if evt.LeadServiceID == nil {
		return
	}
	o.reconcileServiceState(ctx, evt.LeadID, *evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), false)
}
func (o *Orchestrator) OnAppointmentCreated(ctx context.Context, evt events.AppointmentCreated) {
	if evt.LeadID == nil || evt.LeadServiceID == nil {
		return
	}
	o.reconcileServiceState(ctx, *evt.LeadID, *evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), true)
}
func (o *Orchestrator) OnAppointmentStatusChanged(ctx context.Context, evt events.AppointmentStatusChanged) {
	if evt.LeadID == nil || evt.LeadServiceID == nil {
		return
	}

	allowResurrection := evt.NewStatus == "scheduled" || evt.NewStatus == "requested"
	o.reconcileServiceState(ctx, *evt.LeadID, *evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), allowResurrection)
}
func (o *Orchestrator) OnAppointmentDeleted(ctx context.Context, evt events.AppointmentDeleted) {
	if evt.LeadID == nil || evt.LeadServiceID == nil {
		return
	}
	o.reconcileServiceState(ctx, *evt.LeadID, *evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), false)
}
func (o *Orchestrator) OnQuoteStatusChanged(ctx context.Context, evt events.QuoteStatusChanged) {
	if evt.LeadServiceID == nil {
		return
	}
	allowResurrection := evt.NewStatus == "Sent" || evt.NewStatus == "Accepted"
	o.reconcileServiceState(ctx, evt.LeadID, *evt.LeadServiceID, evt.OrganizationID, evt.EventName(), evt.OccurredAt(), allowResurrection)
}
func (o *Orchestrator) OnLeadServiceStatusChanged(ctx context.Context, evt events.LeadServiceStatusChanged) {
	o.reconcileServiceState(ctx, evt.LeadID, evt.LeadServiceID, evt.TenantID, evt.EventName(), evt.OccurredAt(), false)
}
func (o *Orchestrator) reconcileServiceState(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, triggerEvent string, triggerAt time.Time, allowResurrection bool) {
	if !o.reconciliationEnabled {
		return
	}
	if !o.markReconciliationRunning(ctx, serviceID) {
		o.log.Info("orchestrator: reconciliation already running, skipping", "serviceId", serviceID, "trigger", triggerEvent)
		return
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		o.markReconciliationComplete(releaseCtx, serviceID)
	}()

	svc, err := o.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		o.log.Error("orchestrator: failed to load service for reconciliation", "error", err)
		return
	}

	aggs, err := o.repo.GetServiceStateAggregates(ctx, serviceID, tenantID)
	if err != nil {
		o.log.Error("orchestrator: failed to load aggregates", "error", err)
		return
	}

	desired, ok := deriveDesiredServiceState(svc, aggs, allowResurrection, triggerAt)
	if !ok {
		return
	}
	desired = o.enforceReconciliationInvariants(ctx, serviceID, tenantID, desired)

	o.applyReconciledState(ctx, applyReconciledStateParams{
		LeadID:       leadID,
		ServiceID:    serviceID,
		TenantID:     tenantID,
		TriggerEvent: triggerEvent,
		TriggerAt:    triggerAt,
		Current:      svc,
		Desired:      desired,
		Aggregates:   aggs,
	})
}

func (o *Orchestrator) enforceReconciliationInvariants(ctx context.Context, serviceID, tenantID uuid.UUID, desired desiredServiceState) desiredServiceState {
	if desired.Stage == domain.PipelineStageFulfillment {
		return o.enforceFulfillmentInvariant(ctx, serviceID, tenantID, desired)
	}

	if desired.Stage != domain.PipelineStageEstimation {
		return desired
	}
	return o.enforceEstimationInvariants(ctx, serviceID, tenantID, desired)
}

func (o *Orchestrator) enforceFulfillmentInvariant(ctx context.Context, serviceID, tenantID uuid.UUID, desired desiredServiceState) desiredServiceState {
	hasNonDraftQuote, err := o.repo.HasNonDraftQuote(ctx, serviceID, tenantID)
	if err == nil && !hasNonDraftQuote {
		desired.Stage = domain.PipelineStageProposal
		desired.Status = domain.LeadStatusPending
		desired.ReasonCode = "artifact_guard_missing_quote_for_fulfillment"
		if strings.TrimSpace(desired.Reason) == "" {
			desired.Reason = "Geen definitieve offerte beschikbaar; service blijft in Proposal."
		}
	}
	return desired
}

func (o *Orchestrator) enforceEstimationInvariants(ctx context.Context, serviceID, tenantID uuid.UUID, desired desiredServiceState) desiredServiceState {
	analysis, err := o.repo.GetLatestAIAnalysis(ctx, serviceID, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			desired.Stage = domain.PipelineStageNurturing
			desired.Status = domain.LeadStatusAttemptedContact
			desired.ReasonCode = "artifact_guard_missing_analysis"
			if strings.TrimSpace(desired.Reason) == "" {
				desired.Reason = "Geen recente intake-analyse beschikbaar; service blijft in Nurturing."
			}
			return desired
		}
		o.log.Error("orchestrator: failed to load latest AI analysis for reconciliation", "serviceId", serviceID, "tenantId", tenantID, "error", err)
		desired.Stage = domain.PipelineStageNurturing
		desired.Status = domain.LeadStatusAttemptedContact
		desired.ReasonCode = "artifact_guard_analysis_load_failed"
		if strings.TrimSpace(desired.Reason) == "" {
			desired.Reason = "Intake-analyse kon niet worden geladen; service blijft in Nurturing."
		}
		return desired
	}
	if reason := domain.ValidateAnalysisStageTransition(analysis.RecommendedAction, analysis.MissingInformation, desired.Stage); reason != "" {
		desired.Stage = domain.PipelineStageNurturing
		desired.Status = domain.LeadStatusAttemptedContact
		desired.ReasonCode = "analysis_invariant_blocked_estimation"
		if strings.TrimSpace(desired.Reason) == "" {
			desired.Reason = "Intake is nog onvolledig; service blijft in Nurturing."
		}
		return desired
	}

	settings, settingsErr := o.loadOrgAISettings(ctx, tenantID)
	if settingsErr == nil && settings.AIConfidenceGateEnabled && analysis.CompositeConfidence != nil && *analysis.CompositeConfidence < minimumEstimationConfidence {
		desired.Stage = domain.PipelineStageNurturing
		desired.Status = domain.LeadStatusAttemptedContact
		desired.ReasonCode = "confidence_blocked_estimation"
		if strings.TrimSpace(desired.Reason) == "" {
			desired.Reason = "Onvoldoende zekerheid in intake-analyse; service blijft in Nurturing."
		}
	}
	return desired
}
type applyReconciledStateParams struct {
	LeadID       uuid.UUID
	ServiceID    uuid.UUID
	TenantID     uuid.UUID
	TriggerEvent string
	TriggerAt    time.Time
	Current      repository.LeadService
	Desired      desiredServiceState
	Aggregates   repository.ServiceStateAggregates
}
func (o *Orchestrator) applyReconciledState(ctx context.Context, p applyReconciledStateParams) {
	oldStage := p.Current.PipelineStage
	oldStatus := p.Current.Status
	if reason := domain.ValidateStateCombination(p.Desired.Status, p.Desired.Stage); reason != "" {
		o.log.Warn("orchestrator: skipping invalid reconciled state",
			"serviceId", p.ServiceID,
			"desiredStage", p.Desired.Stage,
			"desiredStatus", p.Desired.Status,
			"reason", reason,
		)
		return
	}

	stageChanged, statusChanged, err := o.updateServiceState(ctx, p.ServiceID, p.TenantID, oldStatus, oldStage, p.Desired.Status, p.Desired.Stage)
	if err != nil {
		o.log.Error("orchestrator: failed to apply reconciled state",
			"error", err,
			"serviceId", p.ServiceID,
			"desiredStage", p.Desired.Stage,
			"desiredStatus", p.Desired.Status,
		)
		return
	}

	// Visit reports represent a milestone that must be exportable (Visit_Completed)
	// without introducing a legacy or terminal status value.
	if p.Desired.ReasonCode == "visit_report_present" {
		_ = o.repo.InsertLeadServiceEvent(ctx, repository.InsertServiceEventParams{
			OrganizationID: p.TenantID,
			LeadID:         p.LeadID,
			LeadServiceID:  p.ServiceID,
			EventType:      repository.EventTypeVisitCompleted,
			Status:         p.Desired.Status,
			PipelineStage:  p.Desired.Stage,
			OccurredAt:     p.TriggerAt,
		})
	}

	if !stageChanged && !statusChanged {
		return
	}
	if stageChanged {
		o.eventBus.Publish(ctx, events.PipelineStageChanged{
			BaseEvent:     events.NewBaseEvent(),
			LeadID:        p.LeadID,
			LeadServiceID: p.ServiceID,
			TenantID:      p.TenantID,
			OldStage:      oldStage,
			NewStage:      p.Desired.Stage,
		})
	}

	reason := p.Desired.Reason
	if reason == "" {
		reason = defaultReconcileReason(p.TriggerEvent, oldStage, p.Desired.Stage)
	}

	o.maybeWriteReconcileTimeline(ctx, maybeWriteTimelineParams{
		LeadID:       p.LeadID,
		ServiceID:    p.ServiceID,
		TenantID:     p.TenantID,
		TriggerEvent: p.TriggerEvent,
		OldStage:     oldStage,
		NewStage:     p.Desired.Stage,
		OldStatus:    oldStatus,
		NewStatus:    p.Desired.Status,
		ReasonCode:   p.Desired.ReasonCode,
		Reason:       reason,
		Resurrecting: p.Desired.Resurrecting,
		Aggregates:   p.Aggregates,
	})

	o.log.Info("orchestrator: reconciled service state",
		"leadId", p.LeadID,
		"serviceId", p.ServiceID,
		"trigger", p.TriggerEvent,
		"reason", reason,
		"oldStage", oldStage,
		"newStage", p.Desired.Stage,
		"oldStatus", oldStatus,
		"newStatus", p.Desired.Status,
	)
}

func (o *Orchestrator) updateServiceState(ctx context.Context, serviceID, tenantID uuid.UUID, oldStatus, oldStage, newStatus, newStage string) (bool, bool, error) {
	targetStatus := newStatus
	if targetStatus == "" {
		targetStatus = oldStatus
	}
	targetStage := newStage
	if targetStage == "" {
		targetStage = oldStage
	}

	if reason := domain.ValidateStateCombination(targetStatus, targetStage); reason != "" {
		return false, false, fmt.Errorf("invalid state combination: %s", reason)
	}

	statusChanged := targetStatus != "" && targetStatus != oldStatus
	stageChanged := targetStage != "" && targetStage != oldStage
	if !statusChanged && !stageChanged {
		return false, false, nil
	}

	if statusChanged && stageChanged {
		if _, err := o.repo.UpdateServiceStatusAndPipelineStage(ctx, serviceID, tenantID, targetStatus, targetStage); err != nil {
			return false, false, err
		}
		return true, true, nil
	}
	if stageChanged {
		if _, err := o.repo.UpdatePipelineStage(ctx, serviceID, tenantID, targetStage); err != nil {
			return false, false, err
		}
		return true, false, nil
	}
	if _, err := o.repo.UpdateServiceStatus(ctx, serviceID, tenantID, targetStatus); err != nil {
		return false, false, err
	}
	return false, true, nil
}
type desiredServiceState struct {
	Stage        string
	Status       string
	ReasonCode   string
	Reason       string
	Resurrecting bool
}

func deriveDesiredServiceState(current repository.LeadService, aggs repository.ServiceStateAggregates, allowResurrection bool, triggerAt time.Time) (desiredServiceState, bool) {
	resurrecting := shouldResurrect(current, aggs, allowResurrection, triggerAt)
	if domain.IsTerminal(current.Status, current.PipelineStage) && !resurrecting {
		return desiredServiceState{}, false
	}

	desired := desiredServiceState{Resurrecting: resurrecting}
	if resurrecting {
		desired.ReasonCode = "terminal_resurrection"
		desired.Reason = "Lead automatisch heropend door nieuwe activiteit"
	}

	if stage, status, code, ok := deriveFromOffers(aggs); ok {
		desired.Stage, desired.Status, desired.ReasonCode = stage, status, coalesceReasonCode(desired.ReasonCode, code)
		return desired, true
	}
	if stage, status, code, reason, ok := deriveFromQuotes(aggs); ok {
		desired.Stage, desired.Status = stage, status
		desired.ReasonCode = coalesceReasonCode(desired.ReasonCode, code)
		if desired.Reason == "" {
			desired.Reason = reason
		}
		return desired, true
	}
	if stage, status, code, ok := deriveFromVisitReports(aggs); ok {
		desired.Stage, desired.Status, desired.ReasonCode = stage, status, coalesceReasonCode(desired.ReasonCode, code)
		return desired, true
	}
	if stage, status, code, ok := deriveFromAppointments(aggs); ok {
		// Appointments are status-driven and should not force a pipeline stage.
		// Keep the current stage unless a specific stage is explicitly derived.
		if stage == domain.StageUnchanged {
			stage = current.PipelineStage
		}
		desired.Stage, desired.Status, desired.ReasonCode = stage, status, coalesceReasonCode(desired.ReasonCode, code)
		return desired, true
	}
	if stage, status, code, ok := deriveFromAI(aggs); ok {
		desired.Stage, desired.Status, desired.ReasonCode = stage, status, coalesceReasonCode(desired.ReasonCode, code)
		return desired, true
	}

	desired.Stage = domain.PipelineStageTriage
	desired.Status = domain.LeadStatusNew
	desired.ReasonCode = coalesceReasonCode(desired.ReasonCode, "default")
	return desired, true
}

func shouldResurrect(current repository.LeadService, aggs repository.ServiceStateAggregates, allowResurrection bool, triggerAt time.Time) bool {
	if !domain.IsTerminal(current.Status, current.PipelineStage) {
		return false
	}
	if !allowResurrection {
		return false
	}

	// Time-safety: only resurrect if this triggering event happened AFTER the service became terminal.
	// This prevents replays/duplicates of old events from reopening long-closed services.
	terminalAt := aggs.TerminalAt
	if terminalAt == nil {
		// Fallback: the service row's updated_at is updated on pipeline/status changes.
		// Not perfect, but safer than allowing resurrection without a time barrier.
		fallback := current.UpdatedAt
		terminalAt = &fallback
	}
	if terminalAt != nil && !triggerAt.After(*terminalAt) {
		return false
	}

	// Additional safety: only resurrect if there is evidence of fresh child activity AFTER terminalAt.
	// This prevents reopening terminal services when a late/duplicated event arrives but the DB state
	// does not actually contain any post-terminal work.
	if terminalAt != nil {
		if latestChild := latestChildActivityAt(aggs); latestChild != nil {
			if !latestChild.After(*terminalAt) {
				return false
			}
		}
	}

	return aggs.ScheduledAppointments > 0 || aggs.AcceptedOffers > 0 || aggs.PendingOffers > 0 || aggs.AcceptedQuotes > 0 || aggs.SentQuotes > 0 || aggs.DraftQuotes > 0
}

func latestChildActivityAt(aggs repository.ServiceStateAggregates) *time.Time {
	var latest *time.Time
	for _, candidate := range []*time.Time{aggs.LatestQuoteAt, aggs.LatestAppointmentAt, aggs.LatestOfferAt} {
		if candidate == nil {
			continue
		}
		if latest == nil || candidate.After(*latest) {
			latest = candidate
		}
	}
	return latest
}

func deriveFromOffers(aggs repository.ServiceStateAggregates) (stage, status, reasonCode string, ok bool) {
	if aggs.AcceptedOffers > 0 {
		return domain.PipelineStageFulfillment, domain.LeadStatusInProgress, "offer_accepted", true
	}
	if aggs.PendingOffers > 0 {
		return domain.PipelineStageFulfillment, domain.LeadStatusPending, "offer_pending", true
	}
	return "", "", "", false
}

func deriveFromQuotes(aggs repository.ServiceStateAggregates) (stage, status, reasonCode, reason string, ok bool) {
	if aggs.AcceptedQuotes > 0 {
		return domain.PipelineStageFulfillment, domain.LeadStatusInProgress, "quote_accepted", "", true
	}
	if aggs.SentQuotes > 0 {
		return domain.PipelineStageProposal, domain.LeadStatusPending, "quote_sent", "", true
	}
	if aggs.DraftQuotes > 0 {
		return domain.PipelineStageEstimation, domain.LeadStatusInProgress, "quote_draft", "", true
	}
	if aggs.RejectedQuotes > 0 {
		return domain.PipelineStageLost, domain.LeadStatusDisqualified, "quotes_rejected_only", "", true
	}
	return "", "", "", "", false
}

func deriveFromVisitReports(aggs repository.ServiceStateAggregates) (stage, status, reasonCode string, ok bool) {
	if !aggs.HasVisitReport {
		return "", "", "", false
	}
	return domain.PipelineStageEstimation, domain.LeadStatusNew, "visit_report_present", true
}

func deriveFromAppointments(aggs repository.ServiceStateAggregates) (stage, status, reasonCode string, ok bool) {
	if aggs.ScheduledAppointments > 0 {
		return domain.StageUnchanged, domain.LeadStatusAppointmentScheduled, "appointment_scheduled", true
	}
	if aggs.CancelledAppointments > 0 {
		return domain.StageUnchanged, domain.LeadStatusNeedsRescheduling, "appointment_cancelled", true
	}
	return "", "", "", false
}

func deriveFromAI(aggs repository.ServiceStateAggregates) (stage, status, reasonCode string, ok bool) {
	if aggs.AiAction == nil {
		return "", "", "", false
	}
	switch *aggs.AiAction {
	case "ScheduleSurvey", "CallImmediately", "Review":
		return domain.PipelineStageEstimation, domain.LeadStatusNew, "ai_valid_intake", true
	case "RequestInfo":
		return domain.PipelineStageNurturing, domain.LeadStatusAttemptedContact, "ai_request_info", true
	case "Reject":
		return domain.PipelineStageLost, domain.LeadStatusDisqualified, "ai_reject", true
	default:
		return domain.PipelineStageTriage, domain.LeadStatusNew, "ai_default", true
	}
}

func defaultReconcileReason(triggerEvent, oldStage, newStage string) string {
	switch triggerEvent {
	case events.QuoteDeleted{}.EventName():
		return "Offerte verwijderd; status opnieuw bepaald"
	case events.PartnerOfferDeleted{}.EventName():
		return "Partneraanbod verwijderd; status opnieuw bepaald"
	case events.AppointmentStatusChanged{}.EventName():
		return "Afspraakstatus gewijzigd; status opnieuw bepaald"
	case events.AppointmentCreated{}.EventName():
		return "Nieuwe afspraak; status opnieuw bepaald"
	case events.AppointmentDeleted{}.EventName():
		return "Afspraak verwijderd; status opnieuw bepaald"
	case events.QuoteStatusChanged{}.EventName():
		return "Offertestatus gewijzigd; status opnieuw bepaald"
	case events.LeadServiceStatusChanged{}.EventName():
		return "Servicestatus gewijzigd; status opnieuw bepaald"
	default:
		return fmt.Sprintf("Auto-correctie: %s → %s", oldStage, newStage)
	}
}

func buildReconcileEvidence(aggs repository.ServiceStateAggregates) map[string]any {
	return map[string]any{
		"acceptedQuotes":        aggs.AcceptedQuotes,
		"sentQuotes":            aggs.SentQuotes,
		"draftQuotes":           aggs.DraftQuotes,
		"rejectedQuotes":        aggs.RejectedQuotes,
		"latestQuoteAt":         aggs.LatestQuoteAt,
		"acceptedOffers":        aggs.AcceptedOffers,
		"pendingOffers":         aggs.PendingOffers,
		"latestOfferAt":         aggs.LatestOfferAt,
		"scheduledAppointments": aggs.ScheduledAppointments,
		"completedAppointments": aggs.CompletedAppointments,
		"cancelledAppointments": aggs.CancelledAppointments,
		"latestAppointmentAt":   aggs.LatestAppointmentAt,
		"hasVisitReport":        aggs.HasVisitReport,
		"aiAction":              aggs.AiAction,
		"terminalAt":            aggs.TerminalAt,
	}
}

func coalesceReasonCode(existing, next string) string {
	if existing != "" {
		return existing
	}
	return next
}

func isPipelineRegression(oldStage, newStage string) bool {
	rank := map[string]int{
		domain.PipelineStageTriage:             10,
		domain.PipelineStageNurturing:          20,
		domain.PipelineStageManualIntervention: 25,
		domain.PipelineStageEstimation:         30,
		domain.PipelineStageProposal:           40,
		domain.PipelineStageFulfillment:        50,
		domain.PipelineStageCompleted:          90,
		domain.PipelineStageLost:               90,
	}

	oldRank, okOld := rank[oldStage]
	newRank, okNew := rank[newStage]
	if !okOld || !okNew {
		return false
	}
	return newRank < oldRank
}
