package agent

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/repository"
)

const (
	CouncilDecisionAllow               = "allow"
	CouncilDecisionDowngradeToNurture  = "downgrade_to_nurturing"
	CouncilDecisionRequireManualReview = "require_manual_review"
)

// CouncilAction identifies where the council is asked for a decision.
type CouncilAction string

const (
	CouncilActionStageUpdate CouncilAction = "stage_update"
	CouncilActionDraftQuote  CouncilAction = "draft_quote"
)

// CouncilEvaluationInput is the runtime contract for council checks.
type CouncilEvaluationInput struct {
	Action      CouncilAction
	LeadID      uuid.UUID
	ServiceID   uuid.UUID
	TenantID    uuid.UUID
	Mode        string
	TargetStage string
	ItemCount   int
}

// CouncilEvaluation describes the council outcome and rationale.
type CouncilEvaluation struct {
	Decision         string
	ReasonCode       string
	Summary          string
	EstimatorSignals []string
	RiskSignals      []string
	ReadinessSignals []string
}

// MultiAgentCouncil defines the runtime council decision contract.
type MultiAgentCouncil interface {
	Evaluate(ctx context.Context, input CouncilEvaluationInput) (CouncilEvaluation, error)
}

// DefaultMultiAgentCouncil is a conservative deterministic council implementation.
type DefaultMultiAgentCouncil struct {
	repo repository.LeadsRepository
}

func NewDefaultMultiAgentCouncil(repo repository.LeadsRepository) *DefaultMultiAgentCouncil {
	return &DefaultMultiAgentCouncil{repo: repo}
}

func (c *DefaultMultiAgentCouncil) Evaluate(ctx context.Context, input CouncilEvaluationInput) (CouncilEvaluation, error) {
	if c == nil || c.repo == nil {
		return CouncilEvaluation{}, fmt.Errorf("council repository is not configured")
	}

	analysis, err := c.repo.GetLatestAIAnalysis(ctx, input.ServiceID, input.TenantID)
	if err != nil {
		return CouncilEvaluation{
			Decision:   CouncilDecisionRequireManualReview,
			ReasonCode: "council_missing_analysis",
			Summary:    "Council kon de laatste intake-analyse niet laden.",
			RiskSignals: []string{
				"missing_latest_ai_analysis",
			},
		}, nil
	}

	out := CouncilEvaluation{
		Decision:   CouncilDecisionAllow,
		ReasonCode: "council_allow",
		Summary:    "Council akkoord.",
	}

	mode := normalizeCouncilConsensusMode(input.Mode)
	out = addBaseCouncilSignals(out, mode, analysis)

	if input.Action == CouncilActionDraftQuote {
		return evaluateDraftQuoteCouncil(out, mode, analysis, input.ItemCount), nil
	}

	if input.Action == CouncilActionStageUpdate && input.TargetStage == domain.PipelineStageEstimation {
		out = evaluateStageUpdateCouncil(out, mode, analysis)
	}

	out.ReadinessSignals = append(out.ReadinessSignals, "readiness_mode=advisory")
	result := normalizeCouncilEvaluation(out)
	if result.Decision != CouncilDecisionAllow {
		log.Printf("council_veto: decision=%s reason_code=%s lead=%s service=%s risk_signals=%v estimator_signals=%v",
			result.Decision, result.ReasonCode, input.LeadID, input.ServiceID, result.RiskSignals, result.EstimatorSignals)
	}
	return result, nil
}

func addBaseCouncilSignals(out CouncilEvaluation, mode string, analysis repository.AIAnalysis) CouncilEvaluation {
	out.ReadinessSignals = append(out.ReadinessSignals, "consensus_mode="+mode)
	if analysis.CompositeConfidence != nil {
		out.EstimatorSignals = append(out.EstimatorSignals, fmt.Sprintf("analysis_confidence=%.2f", *analysis.CompositeConfidence))
	}
	if len(analysis.MissingInformation) > 0 {
		out.RiskSignals = append(out.RiskSignals, fmt.Sprintf("missing_information_count=%d", len(analysis.MissingInformation)))
	}
	return out
}

func evaluateDraftQuoteCouncil(out CouncilEvaluation, mode string, analysis repository.AIAnalysis, itemCount int) CouncilEvaluation {
	if itemCount <= 0 {
		out.Decision = CouncilDecisionRequireManualReview
		out.ReasonCode = "council_empty_quote_items"
		out.Summary = "Council blokkeert conceptofferte zonder regels."
		out.RiskSignals = append(out.RiskSignals, "empty_quote_items")
		return out
	}

	intakeReady, lowConfidence := deriveIntakeReadiness(analysis)
	switch mode {
	case "majority":
		return applyDraftQuoteMajority(out, intakeReady, lowConfidence)
	case "estimator_final":
		return applyDraftQuoteEstimatorFinal(out, intakeReady, lowConfidence)
	default:
		return applyDraftQuoteWeighted(out, intakeReady, lowConfidence)
	}
}

func applyDraftQuoteMajority(out CouncilEvaluation, intakeReady, lowConfidence bool) CouncilEvaluation {
	allowVotes := 0
	if intakeReady && !lowConfidence {
		allowVotes++
	} else {
		out.EstimatorSignals = append(out.EstimatorSignals, "estimator_vote=manual_review")
	}
	if intakeReady {
		allowVotes++
	} else {
		out.RiskSignals = append(out.RiskSignals, "risk_vote=manual_review")
	}
	if !lowConfidence {
		allowVotes++
	} else {
		out.ReadinessSignals = append(out.ReadinessSignals, "readiness_vote=manual_review")
	}
	if allowVotes < 2 {
		out.Decision = CouncilDecisionRequireManualReview
		out.ReasonCode = "council_majority_block_quote"
		out.Summary = "Council blokkeert conceptofferte: onvoldoende consensus."
	}
	return out
}

func applyDraftQuoteEstimatorFinal(out CouncilEvaluation, intakeReady, lowConfidence bool) CouncilEvaluation {
	if !intakeReady || lowConfidence {
		out.Decision = CouncilDecisionRequireManualReview
		out.ReasonCode = "council_estimator_final_block_quote"
		out.Summary = "Council blokkeert conceptofferte op basis van estimator-final beleid."
		out.EstimatorSignals = append(out.EstimatorSignals, "estimator_final=block")
	}
	return out
}

func applyDraftQuoteWeighted(out CouncilEvaluation, intakeReady, lowConfidence bool) CouncilEvaluation {
	if !intakeReady {
		out.Decision = CouncilDecisionRequireManualReview
		out.ReasonCode = "council_intake_not_ready_for_quote"
		out.Summary = "Council blokkeert conceptofferte: intake is nog onvolledig."
		out.RiskSignals = append(out.RiskSignals, "intake_not_ready_for_quote")
		return out
	}
	if lowConfidence {
		out.Decision = CouncilDecisionRequireManualReview
		out.ReasonCode = "council_low_confidence_quote"
		out.Summary = "Council blokkeert conceptofferte: analysekans is te laag."
		out.RiskSignals = append(out.RiskSignals, "low_confidence")
	}
	return out
}

func evaluateStageUpdateCouncil(out CouncilEvaluation, mode string, analysis repository.AIAnalysis) CouncilEvaluation {
	intakeReady, lowConfidence := deriveIntakeReadiness(analysis)
	switch mode {
	case "majority":
		return applyStageUpdateMajority(out, intakeReady, lowConfidence)
	case "estimator_final":
		return applyStageUpdateEstimatorFinal(out, intakeReady, lowConfidence)
	default:
		return applyStageUpdateWeighted(out, intakeReady, lowConfidence)
	}
}

func applyStageUpdateMajority(out CouncilEvaluation, intakeReady, lowConfidence bool) CouncilEvaluation {
	allowVotes := 0
	if intakeReady && !lowConfidence {
		allowVotes++
	} else {
		out.EstimatorSignals = append(out.EstimatorSignals, "estimator_vote=downgrade")
	}
	if intakeReady {
		allowVotes++
	} else {
		out.RiskSignals = append(out.RiskSignals, "risk_vote=downgrade")
	}
	if !lowConfidence {
		allowVotes++
	} else {
		out.ReadinessSignals = append(out.ReadinessSignals, "readiness_vote=downgrade")
	}
	if allowVotes < 2 {
		out.Decision = CouncilDecisionDowngradeToNurture
		out.ReasonCode = "council_majority_downgrade"
		out.Summary = "Council verlaagt naar Nurturing: onvoldoende consensus."
	}
	return out
}

func applyStageUpdateEstimatorFinal(out CouncilEvaluation, intakeReady, lowConfidence bool) CouncilEvaluation {
	if !intakeReady || lowConfidence {
		out.Decision = CouncilDecisionDowngradeToNurture
		out.ReasonCode = "council_estimator_final_downgrade"
		out.Summary = "Council verlaagt naar Nurturing op basis van estimator-final beleid."
		out.EstimatorSignals = append(out.EstimatorSignals, "estimator_final=downgrade")
	}
	return out
}

func applyStageUpdateWeighted(out CouncilEvaluation, intakeReady, lowConfidence bool) CouncilEvaluation {
	if !intakeReady {
		out.Decision = CouncilDecisionDowngradeToNurture
		out.ReasonCode = "council_intake_incomplete"
		out.Summary = "Council verlaagt naar Nurturing: intake is nog onvolledig."
		out.RiskSignals = append(out.RiskSignals, "intake_incomplete")
		return out
	}
	if lowConfidence {
		out.Decision = CouncilDecisionDowngradeToNurture
		out.ReasonCode = "council_low_confidence"
		out.Summary = "Council verlaagt naar Nurturing: analysekans is te laag."
		out.RiskSignals = append(out.RiskSignals, "low_confidence")
	}
	return out
}

func deriveIntakeReadiness(analysis repository.AIAnalysis) (bool, bool) {
	intakeReady := domain.ValidateAnalysisStageTransition(analysis.RecommendedAction, analysis.MissingInformation, domain.PipelineStageEstimation) == ""
	lowConfidence := analysis.CompositeConfidence != nil && *analysis.CompositeConfidence < 0.45
	return intakeReady, lowConfidence
}


