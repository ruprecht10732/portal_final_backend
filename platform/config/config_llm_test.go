package config

import "testing"

const globalModelName = "kimi-global"

func TestResolveLLMModelUsesAgentOverrideFirst(t *testing.T) {
	cfg := &Config{
		LLMModelDefault:               "kimi-default",
		LLMModelGatekeeper:            "kimi-gatekeeper",
		LLMModelOfferSummaryGenerator: "summary-special",
	}

	if got := cfg.ResolveLLMModel(LLMModelAgentGatekeeper); got != "kimi-gatekeeper" {
		t.Fatalf("expected gatekeeper override, got %q", got)
	}
	if got := cfg.ResolveLLMModel(LLMModelAgentOfferSummaryGenerator); got != "summary-special" {
		t.Fatalf("expected offer summary override, got %q", got)
	}
}

func TestResolveLLMModelUsesGlobalDefaultBeforeLegacyFallback(t *testing.T) {
	cfg := &Config{LLMModelDefault: globalModelName}

	if got := cfg.ResolveLLMModel(LLMModelAgentEstimator); got != globalModelName {
		t.Fatalf("expected estimator to use global default, got %q", got)
	}
	if got := cfg.ResolveLLMModel(LLMModelAgentOfferSummaryGenerator); got != globalModelName {
		t.Fatalf("expected offer summary to use global default, got %q", got)
	}
}

func TestResolveLLMModelFallsBackToLegacyDefaults(t *testing.T) {
	cfg := &Config{}

	if got := cfg.ResolveLLMModel(LLMModelAgentDispatcher); got != DefaultLLMModel {
		t.Fatalf("expected dispatcher fallback model %q, got %q", DefaultLLMModel, got)
	}
	if got := cfg.ResolveLLMModel(LLMModelAgentOfferSummaryGenerator); got != DefaultOfferSummaryLLMModel {
		t.Fatalf("expected offer summary fallback model %q, got %q", DefaultOfferSummaryLLMModel, got)
	}
}
