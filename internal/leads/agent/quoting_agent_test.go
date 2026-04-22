package agent

import (
	"reflect"
	"testing"

	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/platform/ai/embeddings"
	"portal_final_backend/platform/ai/openaicompat"
	"portal_final_backend/platform/qdrant"

	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
)

func TestNewEstimatorAgentUsesEstimatorProfile(t *testing.T) {
	agent, err := NewEstimatorAgent(QuotingAgentConfig{ModelConfig: openaicompat.Config{Model: "kimi-test-estimator"}}, session.InMemoryService())
	if err != nil {
		t.Fatalf("NewEstimatorAgent returned error: %v", err)
	}
	if agent != nil {
		if agent.mode != quotingAgentModeEstimator {
			t.Fatalf("expected estimator mode, got %q", agent.mode)
		}
		if agent.appName != "estimator" {
			t.Fatalf("expected estimator app name, got %q", agent.appName)
		}
		if agent.modelConfig.Model != "kimi-test-estimator" {
			t.Fatalf("expected estimator model config to use configured model, got %q", agent.modelConfig.Model)
		}
		if agent.modelConfig.DisableThinking {
			t.Fatal("expected estimator to keep Moonshot thinking enabled")
		}
		return
	}
	t.Fatal("expected estimator agent instance")
}

func TestNewQuoteGeneratorAgentUsesQuoteGeneratorProfile(t *testing.T) {
	agent, err := NewQuoteGeneratorAgent(QuotingAgentConfig{ModelConfig: openaicompat.Config{Model: "kimi-test-quote"}}, session.InMemoryService())
	if err != nil {
		t.Fatalf("NewQuoteGeneratorAgent returned error: %v", err)
	}
	if agent != nil {
		if agent.mode != quotingAgentModeQuoteGenerator {
			t.Fatalf("expected quote-generator mode, got %q", agent.mode)
		}
		if agent.appName != "quote-generator" {
			t.Fatalf("expected quote-generator app name, got %q", agent.appName)
		}
		if agent.modelConfig.Model != "kimi-test-quote" {
			t.Fatalf("expected quote-generator model config to use configured model, got %q", agent.modelConfig.Model)
		}
		if agent.modelConfig.DisableThinking {
			t.Fatal("expected quote-generator to keep Moonshot thinking enabled")
		}
		return
	}
	t.Fatal("expected quote generator agent instance")
}

func TestBuildQuotingToolsEstimatorIncludesAutonomousTools(t *testing.T) {
	deps := &ToolDependencies{
		EmbeddingClient: embeddings.NewClient(embeddings.Config{BaseURL: "http://embeddings.test"}),
		CatalogQdrantClient: qdrant.NewClient(qdrant.Config{
			BaseURL:    "http://qdrant.test",
			Collection: "catalog-products",
		}),
	}

	tools, err := buildQuotingTools(deps, quotingAgentModeEstimator)
	if err != nil {
		t.Fatalf("buildQuotingTools(estimator) returned error: %v", err)
	}

	names := toolNames(tools)
	assertHasTool(t, names, "Calculator")
	assertHasTool(t, names, "DraftQuote")
	assertHasTool(t, names, "SearchProductMaterials")
	assertHasTool(t, names, "CalculateEstimate")
	assertHasTool(t, names, "SaveEstimation")
	assertHasTool(t, names, "UpdatePipelineStage")
	assertHasTool(t, names, "ListCatalogGaps")
	if len(names) != 7 {
		t.Fatalf("expected 7 estimator tools, got %d: %v", len(names), names)
	}
}

func TestBuildQuotingToolsQuoteGeneratorExcludesAutonomousTools(t *testing.T) {
	deps := &ToolDependencies{
		EmbeddingClient: embeddings.NewClient(embeddings.Config{BaseURL: "http://embeddings.test"}),
		CatalogQdrantClient: qdrant.NewClient(qdrant.Config{
			BaseURL:    "http://qdrant.test",
			Collection: "catalog-products",
		}),
	}

	tools, err := buildQuotingTools(deps, quotingAgentModeQuoteGenerator)
	if err != nil {
		t.Fatalf("buildQuotingTools(quote-generator) returned error: %v", err)
	}

	names := toolNames(tools)
	assertHasTool(t, names, "Calculator")
	assertHasTool(t, names, "DraftQuote")
	assertHasTool(t, names, "SearchProductMaterials")
	assertNoTool(t, names, "CalculateEstimate")
	assertNoTool(t, names, "SaveEstimation")
	assertNoTool(t, names, "UpdatePipelineStage")
	assertNoTool(t, names, "ListCatalogGaps")
	if len(names) != 3 {
		t.Fatalf("expected 3 quote-generator tools, got %d: %v", len(names), names)
	}
}

func toolNames(tools []tool.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	return names
}

func assertHasTool(t *testing.T, names []string, want string) {
	t.Helper()
	for _, name := range names {
		if name == want {
			return
		}
	}
	t.Fatalf("expected tool %q in %v", want, names)
}

func assertNoTool(t *testing.T, names []string, forbidden string) {
	t.Helper()
	for _, name := range names {
		if name == forbidden {
			t.Fatalf("did not expect tool %q in %v", forbidden, names)
		}
	}
}

func TestRunQuoteCriticRepairLoopRepairsThenApproves(t *testing.T) {
	criticAttempts := make([]int, 0, 2)
	repairAttempts := make([]int, 0, 1)
	repeatedCalls := 0
	exhaustedCalls := 0
	humanCalls := 0

	runQuoteCriticRepairLoop(2,
		func(attempt int) (*quoteCriticLoopReview, bool) {
			criticAttempts = append(criticAttempts, attempt)
			if attempt == 1 {
				return &quoteCriticLoopReview{
					result: &ports.QuoteAIReviewResult{Decision: ports.QuoteAIReviewDecisionNeedsRepair, Summary: "fix quantity"},
					critique: &SubmitQuoteCritiqueInput{
						Approved: false,
						Findings: []QuoteCritiqueFinding{{Code: "qty_missing", Message: "hoeveelheid ontbreekt", Severity: "high"}},
						Signals:  []string{"missing_quantity"},
					},
				}, true
			}
			return &quoteCriticLoopReview{
				result:   &ports.QuoteAIReviewResult{Decision: ports.QuoteAIReviewDecisionApproved, Summary: "ok"},
				critique: &SubmitQuoteCritiqueInput{Approved: true},
			}, true
		},
		func(attempt int) bool {
			repairAttempts = append(repairAttempts, attempt)
			return true
		},
		func(summary string) { humanCalls++ },
		func(summary string) { exhaustedCalls++ },
		func(summary string) { repeatedCalls++ },
	)

	if !reflect.DeepEqual(criticAttempts, []int{1, 2}) {
		t.Fatalf("expected critic attempts [1 2], got %v", criticAttempts)
	}
	if !reflect.DeepEqual(repairAttempts, []int{1}) {
		t.Fatalf("expected one repair attempt [1], got %v", repairAttempts)
	}
	if repeatedCalls != 0 {
		t.Fatalf("expected no repeated-findings escalation, got %d", repeatedCalls)
	}
	if exhaustedCalls != 0 {
		t.Fatalf("expected no exhausted escalation, got %d", exhaustedCalls)
	}
	if humanCalls != 0 {
		t.Fatalf("expected no direct requires-human alert, got %d", humanCalls)
	}
}

func TestRunQuoteCriticRepairLoopEscalatesRepeatedFindings(t *testing.T) {
	repairAttempts := make([]int, 0, 1)
	var repeatedSummary string

	sharedCritique := SubmitQuoteCritiqueInput{
		Approved: false,
		Findings: []QuoteCritiqueFinding{{Code: "dependency_missing", Message: "kit ontbreekt", Severity: "medium"}},
		Signals:  []string{"missing_dependency"},
	}

	runQuoteCriticRepairLoop(2,
		func(attempt int) (*quoteCriticLoopReview, bool) {
			if attempt == 1 {
				return &quoteCriticLoopReview{
					result:   &ports.QuoteAIReviewResult{Decision: ports.QuoteAIReviewDecisionNeedsRepair, Summary: "first review"},
					critique: &sharedCritique,
				}, true
			}
			copyCritique := cloneSubmitQuoteCritiqueInput(sharedCritique)
			return &quoteCriticLoopReview{
				result:   &ports.QuoteAIReviewResult{Decision: ports.QuoteAIReviewDecisionNeedsRepair, Summary: "second review"},
				critique: &copyCritique,
			}, true
		},
		func(attempt int) bool {
			repairAttempts = append(repairAttempts, attempt)
			return true
		},
		func(summary string) { t.Fatalf("did not expect direct requires-human callback: %s", summary) },
		func(summary string) { t.Fatalf("did not expect exhausted callback: %s", summary) },
		func(summary string) { repeatedSummary = summary },
	)

	if !reflect.DeepEqual(repairAttempts, []int{1}) {
		t.Fatalf("expected exactly one repair attempt before escalation, got %v", repairAttempts)
	}
	if repeatedSummary != "second review" {
		t.Fatalf("expected repeated-findings escalation to use latest summary, got %q", repeatedSummary)
	}
}

func TestRunQuoteCriticRepairLoopExhaustsAfterMaxRetries(t *testing.T) {
	criticAttempts := make([]int, 0, 3)
	repairAttempts := make([]int, 0, 2)
	var exhaustedSummary string

	runQuoteCriticRepairLoop(2,
		func(attempt int) (*quoteCriticLoopReview, bool) {
			criticAttempts = append(criticAttempts, attempt)
			return &quoteCriticLoopReview{
				result: &ports.QuoteAIReviewResult{Decision: ports.QuoteAIReviewDecisionNeedsRepair, Summary: "review still failing"},
				critique: &SubmitQuoteCritiqueInput{
					Approved: false,
					Findings: []QuoteCritiqueFinding{{Code: "issue", Message: "issue" + string(rune('0'+attempt)), Severity: "medium"}},
					Signals:  []string{"signal" + string(rune('0'+attempt))},
				},
			}, true
		},
		func(attempt int) bool {
			repairAttempts = append(repairAttempts, attempt)
			return true
		},
		func(summary string) { t.Fatalf("did not expect direct requires-human callback: %s", summary) },
		func(summary string) { exhaustedSummary = summary },
		func(summary string) { t.Fatalf("did not expect repeated-findings callback: %s", summary) },
	)

	if !reflect.DeepEqual(criticAttempts, []int{1, 2, 3}) {
		t.Fatalf("expected critic attempts [1 2 3], got %v", criticAttempts)
	}
	if !reflect.DeepEqual(repairAttempts, []int{1, 2}) {
		t.Fatalf("expected repair attempts [1 2], got %v", repairAttempts)
	}
	if exhaustedSummary != "review still failing" {
		t.Fatalf("expected exhaustion to use latest review summary, got %q", exhaustedSummary)
	}
}

func TestQuoteCritiquesEquivalentIgnoresOrderAndCase(t *testing.T) {
	firstItem := 1
	secondItem := 1
	left := SubmitQuoteCritiqueInput{
		Approved: false,
		Findings: []QuoteCritiqueFinding{
			{Code: "DEPENDENCY_MISSING", Message: "Kit ontbreekt", Severity: "HIGH", ItemIndex: &firstItem},
			{Code: "LABOR_LOW", Message: "Te weinig arbeid", Severity: "medium"},
		},
		Signals: []string{"Missing_Dependency", "labor_implausible"},
	}
	right := SubmitQuoteCritiqueInput{
		Approved: false,
		Findings: []QuoteCritiqueFinding{
			{Code: "labor_low", Message: "te weinig arbeid", Severity: "MEDIUM"},
			{Code: "dependency_missing", Message: "kit ontbreekt", Severity: "high", ItemIndex: &secondItem},
		},
		Signals: []string{"labor_implausible", "missing_dependency"},
	}

	if !quoteCritiquesEquivalent(left, right) {
		t.Fatal("expected critiques with equivalent findings/signals to match")
	}
}
