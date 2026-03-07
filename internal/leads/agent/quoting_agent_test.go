package agent

import (
	"testing"

	"portal_final_backend/platform/ai/embeddings"
	"portal_final_backend/platform/qdrant"

	"google.golang.org/adk/tool"
)

func TestNewEstimatorAgentUsesEstimatorProfile(t *testing.T) {
	agent, err := NewEstimatorAgent(QuotingAgentConfig{})
	if err != nil {
		t.Fatalf("NewEstimatorAgent returned error: %v", err)
	}
	if agent == nil {
		t.Fatal("expected estimator agent instance")
	}
	if agent.mode != quotingAgentModeEstimator {
		t.Fatalf("expected estimator mode, got %q", agent.mode)
	}
	if agent.appName != "estimator" {
		t.Fatalf("expected estimator app name, got %q", agent.appName)
	}
}

func TestNewQuoteGeneratorAgentUsesQuoteGeneratorProfile(t *testing.T) {
	agent, err := NewQuoteGeneratorAgent(QuotingAgentConfig{})
	if err != nil {
		t.Fatalf("NewQuoteGeneratorAgent returned error: %v", err)
	}
	if agent == nil {
		t.Fatal("expected quote generator agent instance")
	}
	if agent.mode != quotingAgentModeQuoteGenerator {
		t.Fatalf("expected quote-generator mode, got %q", agent.mode)
	}
	if agent.appName != "quote-generator" {
		t.Fatalf("expected quote-generator app name, got %q", agent.appName)
	}
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
