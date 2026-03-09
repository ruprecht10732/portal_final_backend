package agent

import "testing"

func TestQuoteGeneratorProfileEnablesReasoning(t *testing.T) {
	profile := quotingAgentModeQuoteGenerator.profile()
	if !profile.reasoning {
		t.Fatalf("expected quote generator profile to enable reasoning")
	}
	if profile.instruction == "" {
		t.Fatalf("expected quote generator instruction to be set")
	}
	if profile.name != "QuoteGenerator" {
		t.Fatalf("unexpected profile name %q", profile.name)
	}
}

func TestEstimatorProfileStillEnablesReasoning(t *testing.T) {
	profile := quotingAgentModeEstimator.profile()
	if !profile.reasoning {
		t.Fatalf("expected estimator profile to keep reasoning enabled")
	}
}
