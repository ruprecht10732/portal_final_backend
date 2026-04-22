// Package eval provides trajectory-based evaluation for agentic workflows.
// It compares actual agent execution traces against expected golden trajectories
// to detect regressions before deployment.
package eval

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"portal_final_backend/internal/leads/repository"
)

// Trajectory is the ordered sequence of tool calls expected for a given scenario.
type Trajectory struct {
	ScenarioID   string
	Description  string
	LeadID       uuid.UUID
	ServiceType  string
	ExpectedTools []string // ordered list of tool names
}

// ActualTrace is a recorded agent run from the observability tables.
type ActualTrace struct {
	AgentName     string
	ToolCalls     []string // ordered list of tool names actually invoked
	Outcome       string
}

// TrajectoryMetrics holds the evaluation results for a single scenario.
type TrajectoryMetrics struct {
	ScenarioID        string
	ExactMatch        bool
	Precision         float64 // correct / total invoked
	Recall            float64 // correct / total expected
	F1                float64
	HallucinatedTools []string
	MissingTools      []string
}

// Evaluator runs trajectory-based evaluations against golden datasets.
type Evaluator struct {
	repo repository.AgentRunStore
}

// NewEvaluator creates an evaluator backed by the agent run store.
func NewEvaluator(repo repository.AgentRunStore) *Evaluator {
	return &Evaluator{repo: repo}
}

// EvaluateScenario compares an expected trajectory against an actual agent run.
func (e *Evaluator) EvaluateScenario(expected Trajectory, actual ActualTrace) TrajectoryMetrics {
	m := TrajectoryMetrics{ScenarioID: expected.ScenarioID}

	// Exact Match
	m.ExactMatch = slicesEqual(expected.ExpectedTools, actual.ToolCalls)

	// Build sets for precision/recall
	expectedSet := make(map[string]struct{}, len(expected.ExpectedTools))
	for _, t := range expected.ExpectedTools {
		expectedSet[t] = struct{}{}
	}
	actualSet := make(map[string]struct{}, len(actual.ToolCalls))
	for _, t := range actual.ToolCalls {
		actualSet[t] = struct{}{}
	}

	// Precision = correct invoked / total invoked
	correct := 0
	for _, t := range actual.ToolCalls {
		if _, ok := expectedSet[t]; ok {
			correct++
		} else {
			m.HallucinatedTools = append(m.HallucinatedTools, t)
		}
	}
	if len(actual.ToolCalls) > 0 {
		m.Precision = float64(correct) / float64(len(actual.ToolCalls))
	} else {
		m.Precision = 0
	}

	// Recall = correct invoked / total expected
	for _, t := range expected.ExpectedTools {
		if _, ok := actualSet[t]; !ok {
			m.MissingTools = append(m.MissingTools, t)
		}
	}
	if len(expected.ExpectedTools) > 0 {
		m.Recall = float64(correct) / float64(len(expected.ExpectedTools))
	} else {
		m.Recall = 0
	}

	// F1
	if m.Precision+m.Recall > 0 {
		m.F1 = 2 * (m.Precision * m.Recall) / (m.Precision + m.Recall)
	}

	return m
}

// EvaluateDataset runs evaluation over a full dataset and returns aggregate metrics.
func (e *Evaluator) EvaluateDataset(ctx context.Context, scenarios []Trajectory, lookbackDays int) (map[string]TrajectoryMetrics, error) {
	results := make(map[string]TrajectoryMetrics, len(scenarios))
	for _, scenario := range scenarios {
		// Fetch the most recent actual run for this lead/service
		runs, err := e.repo.ListAgentRunsByService(ctx, scenario.LeadID, uuid.Nil, 1)
		if err != nil {
			return nil, fmt.Errorf("eval: list runs for scenario %s: %w", scenario.ScenarioID, err)
		}
		if len(runs) == 0 {
			results[scenario.ScenarioID] = TrajectoryMetrics{
				ScenarioID: scenario.ScenarioID,
				Precision:  0,
				Recall:     0,
			}
			continue
		}
		// Note: tool call sequence is not directly available from ListAgentRunsByService.
		// In production, this would join agent_tool_calls to reconstruct the ordered trace.
		// For now, we stub with an empty trace.
		actual := ActualTrace{
			AgentName: runs[0].AgentName,
			Outcome:   runs[0].Outcome,
		}
		results[scenario.ScenarioID] = e.EvaluateScenario(scenario, actual)
	}
	return results, nil
}

// AggregateMetrics summarizes a dataset evaluation.
type AggregateMetrics struct {
	ExactMatchRate float64
	AvgPrecision   float64
	AvgRecall      float64
	AvgF1          float64
	TotalScenarios int
}

// Summarize computes aggregate statistics over per-scenario metrics.
func Summarize(metrics map[string]TrajectoryMetrics) AggregateMetrics {
	var am AggregateMetrics
	if len(metrics) == 0 {
		return am
	}
	var emCount int
	var pSum, rSum, f1Sum float64
	for _, m := range metrics {
		am.TotalScenarios++
		if m.ExactMatch {
			emCount++
		}
		pSum += m.Precision
		rSum += m.Recall
		f1Sum += m.F1
	}
	am.ExactMatchRate = float64(emCount) / float64(am.TotalScenarios)
	am.AvgPrecision = pSum / float64(am.TotalScenarios)
	am.AvgRecall = rSum / float64(am.TotalScenarios)
	am.AvgF1 = f1Sum / float64(am.TotalScenarios)
	return am
}

// slicesEqual compares two string slices for equality (ordered).
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i], b[i]) {
			return false
		}
	}
	return true
}

// GoldenDataset returns the built-in golden trajectory dataset for the portal.
// In production, this should be loaded from a file or database.
func GoldenDataset() []Trajectory {
	return []Trajectory{
		{
			ScenarioID:    "gatekeeper-complete-intake",
			Description:   "Complete intake → SaveAnalysis → UpdatePipelineStage",
			ExpectedTools: []string{"SaveAnalysis", "UpdatePipelineStage"},
		},
		{
			ScenarioID:    "estimator-standard-quote",
			Description:   "Standard estimation flow with scope + quote",
			ExpectedTools: []string{"CommitScopeArtifact", "SearchProductMaterials", "CalculateEstimate", "DraftQuote"},
		},
		{
			ScenarioID:    "dispatcher-match-offer",
			Description:   "Dispatcher finds partners and creates offers",
			ExpectedTools: []string{"FindMatchingPartners", "CreatePartnerOffer"},
		},
	}
}
