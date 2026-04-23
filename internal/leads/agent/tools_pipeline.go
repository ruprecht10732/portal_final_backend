package agent

import (
	"context"
	"fmt"
	"log"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/repository"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/adk/tool"
)

func validateProposalQuoteGuard(ctx context.Context, deps *ToolDependencies, stage string, serviceID, tenantID uuid.UUID) (UpdatePipelineStageOutput, error) {
	if stage != domain.PipelineStageProposal {
		return UpdatePipelineStageOutput{}, nil
	}

	hasNonDraftQuote, checkErr := deps.Repo.HasNonDraftQuote(ctx, serviceID, tenantID)
	if checkErr != nil {
		return UpdatePipelineStageOutput{Success: false, Message: "Failed to validate quote state"}, checkErr
	}
	if !hasNonDraftQuote {
		return UpdatePipelineStageOutput{Success: false, Message: "Cannot move to Proposal while quote is still draft"}, fmt.Errorf("quote state guard blocked Proposal for service %s", serviceID)
	}

	return UpdatePipelineStageOutput{}, nil
}

func validateActorSequence(stage string, serviceID uuid.UUID, deps *ToolDependencies, actorType, actorName string) (UpdatePipelineStageOutput, error) {
	if actorType == repository.ActorTypeAI && actorName == repository.ActorNameGatekeeper && !deps.WasSaveAnalysisCalled() {
		return UpdatePipelineStageOutput{Success: false, Message: "SaveAnalysis is required before stage update"}, fmt.Errorf("gatekeeper sequence violation: SaveAnalysis missing before UpdatePipelineStage for service %s", serviceID)
	}

	if actorType == repository.ActorTypeAI && actorName == repository.ActorNameEstimator {
		if !deps.WasSaveEstimationCalled() {
			return UpdatePipelineStageOutput{Success: false, Message: "SaveEstimation is required before stage update"}, fmt.Errorf("estimator sequence violation: SaveEstimation missing before UpdatePipelineStage for service %s", serviceID)
		}
		if stage == domain.PipelineStageEstimation && !deps.WasDraftQuoteCalled() {
			return UpdatePipelineStageOutput{Success: false, Message: "DraftQuote is required before moving to Estimation"}, fmt.Errorf("estimator sequence violation: DraftQuote missing before Estimation stage update for service %s", serviceID)
		}
	}

	if actorType == repository.ActorTypeAI && actorName == repository.ActorNameDispatcher && stage == domain.PipelineStageFulfillment && !deps.WasOfferCreated() {
		return UpdatePipelineStageOutput{Success: false, Message: "CreatePartnerOffer is required before moving to Fulfillment"}, fmt.Errorf("dispatcher sequence violation: CreatePartnerOffer missing before Fulfillment stage update for service %s", serviceID)
	}

	return UpdatePipelineStageOutput{}, nil
}

func validateEstimationInvariant(ctx context.Context, deps *ToolDependencies, stage string, serviceID, tenantID uuid.UUID) (UpdatePipelineStageOutput, error) {
	if stage != domain.PipelineStageEstimation {
		return UpdatePipelineStageOutput{}, nil
	}

	recommendedAction, missingInformation := latestAnalysisInvariantInputs(ctx, deps, serviceID, tenantID)
	if reason := domain.ValidateAnalysisStageTransition(recommendedAction, missingInformation, stage); reason != "" {
		log.Printf("stage_blocked=true stage=%s service=%s block_reason=%s recommended_action=%s missing_count=%d",
			stage, serviceID, reason, recommendedAction, len(missingInformation))
		return UpdatePipelineStageOutput{Success: false, Message: "Cannot move to Estimation while intake is incomplete"}, fmt.Errorf("analysis-stage invariant blocked Estimation for service %s: %s", serviceID, reason)
	}

	return UpdatePipelineStageOutput{}, nil
}

func evaluateCouncilForStageUpdate(ctx context.Context, deps *ToolDependencies, leadID, serviceID, tenantID uuid.UUID, targetStage string) (CouncilEvaluation, error) {
	settings := deps.GetOrganizationAISettingsOrDefault()
	if !settings.AICouncilMode || deps.CouncilService == nil {
		deps.SetCouncilMetadata(nil)
		return CouncilEvaluation{Decision: CouncilDecisionAllow, ReasonCode: "council_disabled", Summary: "Council uitgeschakeld."}, nil
	}

	evaluation, err := deps.CouncilService.Evaluate(ctx, CouncilEvaluationInput{
		Action:      CouncilActionStageUpdate,
		LeadID:      leadID,
		ServiceID:   serviceID,
		TenantID:    tenantID,
		Mode:        settings.AICouncilConsensusMode,
		TargetStage: targetStage,
	})
	if err != nil {
		return CouncilEvaluation{}, err
	}

	deps.SetCouncilMetadata(repository.CouncilAdviceMetadata{
		Decision:         evaluation.Decision,
		ReasonCode:       evaluation.ReasonCode,
		Summary:          evaluation.Summary,
		EstimatorSignals: evaluation.EstimatorSignals,
		RiskSignals:      evaluation.RiskSignals,
		ReadinessSignals: evaluation.ReadinessSignals,
	}.ToMap())

	return evaluation, nil
}

func handleUpdatePipelineStage(ctx tool.Context, deps *ToolDependencies, input UpdatePipelineStageInput) (UpdatePipelineStageOutput, error) {
	return applyPipelineStageUpdate(ctx, deps, input)
}

func applyPipelineStageUpdate(ctx context.Context, deps *ToolDependencies, input UpdatePipelineStageInput) (UpdatePipelineStageOutput, error) {
	state, loopResult, out, done, err := prepareStageUpdate(ctx, deps, &input)
	if done || err != nil {
		return out, err
	}

	councilEval, councilErr := evaluateCouncilForStageUpdate(ctx, deps, state.leadID, state.serviceID, state.tenantID, input.Stage)
	if councilErr != nil {
		return UpdatePipelineStageOutput{Success: false, Message: "Council evaluatie mislukt"}, councilErr
	}

	out, err = applyCouncilDecision(ctx, deps, councilEval, state, &input)
	if err != nil || out.Message != "" {
		return out, err
	}

	_, err = deps.Repo.UpdatePipelineStage(ctx, state.serviceID, state.tenantID, input.Stage)
	if err != nil {
		return UpdatePipelineStageOutput{Success: false, Message: "Failed to update pipeline stage"}, err
	}
	if shouldResetGatekeeperNurturingLoopState(state.service, input.Stage, loopResult.Trigger) {
		if resetErr := deps.Repo.ResetGatekeeperNurturingLoopState(ctx, state.serviceID, state.tenantID); resetErr != nil {
			log.Printf("gatekeeper nurturing loop reset failed for service=%s tenant=%s: %v", state.serviceID, state.tenantID, resetErr)
		}
	}
	if shouldResetAgentCycleState(state.service, input.Stage, loopResult.Trigger) {
		if resetErr := deps.Repo.ResetAgentCycleState(ctx, state.serviceID, state.tenantID); resetErr != nil {
			log.Printf("agent cycle state reset failed for service=%s tenant=%s: %v", state.serviceID, state.tenantID, resetErr)
		}
	}

	recordPipelineStageChange(ctx, deps, stageChangeParams{
		leadID:             state.leadID,
		serviceID:          state.serviceID,
		tenantID:           state.tenantID,
		oldStage:           state.oldStage,
		newStage:           input.Stage,
		reason:             input.Reason,
		trigger:            loopResult.Trigger,
		reasonCode:         loopResult.ReasonCode,
		loopAttemptCount:   loopResult.AttemptCount,
		blockerFingerprint: loopResult.BlockerFingerprint,
		missingInformation: loopResult.MissingInformation,
	})
	deps.MarkStageUpdateCalled(input.Stage)
	log.Printf("agent stage transition committed (run=%s actor=%s/%s lead=%s service=%s from=%s to=%s)",
		state.runID, state.actorType, state.actorName, state.leadID, state.serviceID, state.oldStage, input.Stage)

	// Defense-in-depth: signal session to end after a successful stage commit.
	// The prompt instructs the LLM to stop, but if it doesn't, this cancels the
	// context so the ADK runner terminates the event loop without burning budget.
	deps.mu.RLock()
	doneFn := deps.sessionDoneFunc
	deps.mu.RUnlock()
	if doneFn != nil {
		doneFn()
	}

	return UpdatePipelineStageOutput{Success: true, Message: "Pipeline stage updated"}, nil
}

type stageUpdateState struct {
	tenantID  uuid.UUID
	leadID    uuid.UUID
	serviceID uuid.UUID
	oldStage  string
	service   repository.LeadService
	actorType string
	actorName string
	runID     string
}

type gatekeeperNurturingLoopResult struct {
	Trigger            string
	ReasonCode         string
	AttemptCount       int
	BlockerFingerprint string
	MissingInformation []string
}

func prepareStageUpdate(ctx context.Context, deps *ToolDependencies, input *UpdatePipelineStageInput) (stageUpdateState, gatekeeperNurturingLoopResult, UpdatePipelineStageOutput, bool, error) {
	if !domain.IsKnownPipelineStage(input.Stage) {
		return stageUpdateState{}, gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{Success: false, Message: "Invalid pipeline stage"}, true, fmt.Errorf("invalid pipeline stage: %s", input.Stage)
	}

	tenantID, err := getTenantID(deps)
	if err != nil {
		return stageUpdateState{}, gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{Success: false, Message: missingTenantContextMessage}, true, err
	}
	leadID, serviceID, err := getLeadContext(deps)
	if err != nil {
		return stageUpdateState{}, gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{Success: false, Message: missingLeadContextMessage}, true, err
	}

	svc, err := deps.Repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		return stageUpdateState{}, gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{Success: false, Message: leadServiceNotFoundMessage}, true, err
	}
	actorType, actorName := deps.GetActor()
	state := stageUpdateState{
		tenantID:  tenantID,
		leadID:    leadID,
		serviceID: serviceID,
		oldStage:  svc.PipelineStage,
		service:   svc,
		actorType: actorType,
		actorName: actorName,
		runID:     deps.GetRunID(),
	}

	if domain.IsTerminal(svc.Status, svc.PipelineStage) {
		log.Printf("handleUpdatePipelineStage: REJECTED - service %s is in terminal state (status=%s, stage=%s)", state.serviceID, svc.Status, svc.PipelineStage)
		return state, gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{Success: false, Message: "Cannot update pipeline stage for a service in terminal state"}, true, fmt.Errorf("service %s is terminal", state.serviceID)
	}
	if out, err := validateProposalQuoteGuard(ctx, deps, input.Stage, state.serviceID, state.tenantID); err != nil {
		return state, gatekeeperNurturingLoopResult{}, out, true, err
	}
	if reason := domain.ValidateStateCombination(svc.Status, input.Stage); reason != "" {
		log.Printf("handleUpdatePipelineStage: invalid state combination: status=%s, newStage=%s - %s", svc.Status, input.Stage, reason)
		return state, gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{Success: false, Message: reason}, true, fmt.Errorf("invalid state combination: %s", reason)
	}
	if out, err := validateActorSequence(input.Stage, state.serviceID, deps, state.actorType, state.actorName); err != nil {
		return state, gatekeeperNurturingLoopResult{}, out, true, err
	}
	if out, err := validateEstimationInvariant(ctx, deps, input.Stage, state.serviceID, state.tenantID); err != nil {
		return state, gatekeeperNurturingLoopResult{}, out, true, err
	}

	loopResult, out, done, err := applyGatekeeperNurturingLoopPolicy(ctx, deps, state, input)
	if done || err != nil {
		return state, loopResult, out, done, err
	}

	cycleResult, out, done, err := applyAgentCyclePolicy(ctx, deps, state, input)
	if done || err != nil {
		return state, loopResult, out, done, err
	}
	if cycleResult.Trigger != "" {
		loopResult.Trigger = cycleResult.Trigger
		loopResult.ReasonCode = cycleResult.ReasonCode
	}

	if state.oldStage == input.Stage {
		deps.MarkStageUpdateCalled(input.Stage)
		log.Printf("agent stage transition skipped (run=%s actor=%s/%s lead=%s service=%s stage=%s): already at target stage",
			state.runID, state.actorType, state.actorName, state.leadID, state.serviceID, input.Stage)
		return state, loopResult, UpdatePipelineStageOutput{Success: false, Message: fmt.Sprintf("ERROR: Stage is already %s. Your task is complete. Do not call any more tools.", input.Stage)}, true, nil
	}

	return state, loopResult, UpdatePipelineStageOutput{}, false, nil
}

func applyGatekeeperNurturingLoopPolicy(ctx context.Context, deps *ToolDependencies, state stageUpdateState, input *UpdatePipelineStageInput) (gatekeeperNurturingLoopResult, UpdatePipelineStageOutput, bool, error) {
	if !shouldTrackGatekeeperNurturingLoop(state.actorName, input.Stage) {
		return gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{}, false, nil
	}

	// Prevent the loop counter from incrementing more than once per session.
	// Within a single gatekeeper run the LLM may call UpdatePipelineStage
	// repeatedly; only the first call should count toward the nurturing-loop
	// threshold. Cross-run detection still works because each run starts with
	// fresh ToolDependencies (stageUpdateCalled = false).
	if deps.WasStageUpdateCalled() {
		return gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{}, false, nil
	}

	fingerprint, missingInformation := resolveGatekeeperLoopFingerprint(ctx, deps, state.serviceID, state.tenantID, input.Reason)
	if fingerprint == "" {
		return gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{}, false, nil
	}

	attemptCount := 1
	if state.service.GatekeeperNurturingLoopFingerprint != nil && *state.service.GatekeeperNurturingLoopFingerprint == fingerprint {
		attemptCount = state.service.GatekeeperNurturingLoopCount + 1
	}
	if attemptCount < 1 {
		attemptCount = 1
	}
	if err := deps.Repo.SetGatekeeperNurturingLoopState(ctx, state.serviceID, state.tenantID, attemptCount, fingerprint); err != nil {
		return gatekeeperNurturingLoopResult{}, UpdatePipelineStageOutput{Success: false, Message: "Failed to update nurturing loop state"}, true, err
	}

	result := gatekeeperNurturingLoopResult{
		AttemptCount:       attemptCount,
		BlockerFingerprint: fingerprint,
		MissingInformation: missingInformation,
	}
	if attemptCount >= gatekeeperNurturingLoopThreshold {
		input.Stage = domain.PipelineStageManualIntervention
		input.Reason = gatekeeperLoopDetectedSummary
		result.Trigger = gatekeeperLoopDetectedTrigger
		result.ReasonCode = gatekeeperLoopReasonCode
	}

	return result, UpdatePipelineStageOutput{}, false, nil
}

func shouldTrackGatekeeperNurturingLoop(actorName, targetStage string) bool {
	return actorName == repository.ActorNameGatekeeper && targetStage == domain.PipelineStageNurturing
}

func shouldResetGatekeeperNurturingLoopState(service repository.LeadService, targetStage, trigger string) bool {
	if service.GatekeeperNurturingLoopCount == 0 && service.GatekeeperNurturingLoopFingerprint == nil {
		return false
	}
	if targetStage == domain.PipelineStageNurturing {
		return false
	}
	if targetStage == domain.PipelineStageManualIntervention && trigger == gatekeeperLoopDetectedTrigger {
		return false
	}
	return true
}

func resolveGatekeeperLoopFingerprint(ctx context.Context, deps *ToolDependencies, serviceID, tenantID uuid.UUID, fallbackReason string) (string, []string) {
	_, missingInformation := latestAnalysisInvariantInputs(ctx, deps, serviceID, tenantID)
	normalizedMissingInformation := normalizeGatekeeperLoopItems(missingInformation)
	if len(normalizedMissingInformation) > 0 {
		return strings.Join(normalizedMissingInformation, " | "), normalizedMissingInformation
	}

	fallbackParts := normalizeGatekeeperLoopItems([]string{fallbackReason})
	if len(fallbackParts) > 0 {
		return "reason:" + strings.Join(fallbackParts, " | "), nil
	}

	return "", nil
}

// --- Cross-agent cycle detection ---

type agentCycleResult struct {
	Trigger    string
	ReasonCode string
	Count      int
}

// applyAgentCyclePolicy detects cross-agent ping-pong between Gatekeeper and Estimator.
// When the Estimator sends a service back to Nurturing, we increment the cycle counter.
// When the Gatekeeper then advances to Estimation and the counter >= threshold, we force Manual_Intervention.
func applyAgentCyclePolicy(ctx context.Context, deps *ToolDependencies, state stageUpdateState, input *UpdatePipelineStageInput) (agentCycleResult, UpdatePipelineStageOutput, bool, error) {
	actorName := state.actorName

	// Track: Estimator sending to Nurturing (the "bounce-back" half of the cycle)
	if actorName == repository.ActorNameEstimator && input.Stage == domain.PipelineStageNurturing {
		if deps.WasStageUpdateCalled() {
			return agentCycleResult{}, UpdatePipelineStageOutput{}, false, nil
		}
		count := state.service.AgentCycleCount + 1
		transition := "Estimation→Nurturing"
		fingerprint := "estimator-bounce"
		if input.Reason != "" {
			parts := normalizeGatekeeperLoopItems([]string{input.Reason})
			if len(parts) > 0 {
				fingerprint = strings.Join(parts, " | ")
			}
		}
		if err := deps.Repo.SetAgentCycleState(ctx, state.serviceID, state.tenantID, count, fingerprint, transition); err != nil {
			log.Printf("agent cycle state update failed for service=%s: %v", state.serviceID, err)
		}
		return agentCycleResult{Count: count}, UpdatePipelineStageOutput{}, false, nil
	}

	// Enforce: Gatekeeper advancing to Estimation while cycle counter is high
	if actorName == repository.ActorNameGatekeeper && input.Stage == domain.PipelineStageEstimation && state.service.AgentCycleCount >= agentCycleThreshold {
		input.Stage = domain.PipelineStageManualIntervention
		input.Reason = agentCycleDetectedSummary
		return agentCycleResult{
			Trigger:    agentCycleDetectedTrigger,
			ReasonCode: agentCycleReasonCode,
			Count:      state.service.AgentCycleCount,
		}, UpdatePipelineStageOutput{}, false, nil
	}

	return agentCycleResult{}, UpdatePipelineStageOutput{}, false, nil
}

func shouldResetAgentCycleState(service repository.LeadService, targetStage, trigger string) bool {
	if service.AgentCycleCount == 0 && service.AgentCycleFingerprint == nil {
		return false
	}
	// Don't reset while in Nurturing or Estimation (cycle is still active)
	if targetStage == domain.PipelineStageNurturing || targetStage == domain.PipelineStageEstimation {
		return false
	}
	// Don't reset when we just triggered the cycle breaker
	if targetStage == domain.PipelineStageManualIntervention && trigger == agentCycleDetectedTrigger {
		return false
	}
	return true
}

func applyCouncilDecision(ctx context.Context, deps *ToolDependencies, eval CouncilEvaluation, state stageUpdateState, input *UpdatePipelineStageInput) (UpdatePipelineStageOutput, error) {
	if eval.Decision == CouncilDecisionRequireManualReview {
		summary := strings.TrimSpace(eval.Summary)
		if summary == "" {
			summary = "Council vraagt handmatige beoordeling voordat de stage kan wijzigen."
		}
		if deps.MarkAlertEmitted("council_stage_update", eval.ReasonCode, summary) {
			_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
				LeadID:         state.leadID,
				ServiceID:      &state.serviceID,
				OrganizationID: state.tenantID,
				ActorType:      repository.ActorTypeSystem,
				ActorName:      "Council",
				EventType:      repository.EventTypeAlert,
				Title:          repository.EventTitleManualIntervention,
				Summary:        &summary,
				Metadata: repository.CouncilAdviceMetadata{
					Decision:         eval.Decision,
					ReasonCode:       eval.ReasonCode,
					Summary:          eval.Summary,
					EstimatorSignals: eval.EstimatorSignals,
					RiskSignals:      eval.RiskSignals,
					ReadinessSignals: eval.ReadinessSignals,
				}.ToMap(),
			})
		}
		return UpdatePipelineStageOutput{Success: false, Message: summary}, fmt.Errorf("council blocked stage update: %s", eval.ReasonCode)
	}

	if eval.Decision == CouncilDecisionDowngradeToNurture && input.Stage == domain.PipelineStageEstimation {
		input.Stage = domain.PipelineStageNurturing
		if strings.TrimSpace(input.Reason) == "" {
			if strings.TrimSpace(eval.Summary) != "" {
				input.Reason = eval.Summary
			} else {
				input.Reason = "Council verlaagt stage: aanvullende intake nodig."
			}
		}
	}

	return UpdatePipelineStageOutput{}, nil
}

// stageChangeParams groups parameters for recording a pipeline stage change.
type stageChangeParams struct {
	leadID             uuid.UUID
	serviceID          uuid.UUID
	tenantID           uuid.UUID
	oldStage           string
	newStage           string
	reason             string
	trigger            string
	reasonCode         string
	loopAttemptCount   int
	blockerFingerprint string
	missingInformation []string
}

func recordPipelineStageChange(ctx context.Context, deps *ToolDependencies, p stageChangeParams) {
	actorType, actorName := deps.GetActor()
	reasonText := strings.TrimSpace(p.reason)
	var summary *string
	if reasonText != "" {
		summary = &reasonText
	}

	stageMetadata := repository.StageChangeMetadata{
		OldStage: p.oldStage,
		NewStage: p.newStage,
		RunID:    deps.GetRunID(),
	}.ToMap()
	if analysisMeta := deps.GetLastAnalysisMetadata(); analysisMeta != nil {
		stageMetadata["analysis"] = analysisMeta
	}
	if estimationMeta := deps.GetLastEstimationMetadata(); estimationMeta != nil {
		stageMetadata["estimation"] = estimationMeta
	}
	if councilMeta := deps.GetLastCouncilMetadata(); councilMeta != nil {
		stageMetadata["council"] = councilMeta
	}
	if draftResult := deps.GetLastDraftResult(); draftResult != nil {
		stageMetadata["draftQuote"] = map[string]any{
			"quoteId":     draftResult.QuoteID,
			"quoteNumber": draftResult.QuoteNumber,
			"itemCount":   draftResult.ItemCount,
		}
	}
	if p.trigger == gatekeeperLoopDetectedTrigger {
		stageMetadata["loopDetected"] = repository.LoopDetectedMetadata{
			Trigger:            p.trigger,
			ReasonCode:         p.reasonCode,
			AttemptCount:       p.loopAttemptCount,
			Threshold:          gatekeeperNurturingLoopThreshold,
			BlockerFingerprint: p.blockerFingerprint,
			MissingInformation: p.missingInformation,
		}.ToMap()
	}

	_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         p.leadID,
		ServiceID:      &p.serviceID,
		OrganizationID: p.tenantID,
		ActorType:      actorType,
		ActorName:      actorName,
		EventType:      repository.EventTypeStageChange,
		Title:          repository.EventTitleStageUpdated,
		Summary:        summary,
		Metadata:       stageMetadata,
	})

	if deps.EventBus != nil {
		deps.EventBus.Publish(ctx, events.PipelineStageChanged{
			BaseEvent:     events.NewBaseEvent(),
			LeadID:        p.leadID,
			LeadServiceID: p.serviceID,
			TenantID:      p.tenantID,
			OldStage:      p.oldStage,
			NewStage:      p.newStage,
			Reason:        reasonText,
			ReasonCode:    p.reasonCode,
			Trigger:       p.trigger,
			ActorType:     actorType,
			ActorName:     actorName,
			RunID:         deps.GetRunID(),
		})
	}

	logReason := reasonText
	if logReason == "" {
		logReason = "(no reason provided)"
	}
	log.Printf("gatekeeper UpdatePipelineStage: leadId=%s serviceId=%s from=%s to=%s reason=%s",
		p.leadID, p.serviceID, p.oldStage, p.newStage, logReason)
}
