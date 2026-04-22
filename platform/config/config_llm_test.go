package config

import (
	"strings"
	"testing"
)

const (
	globalModelName    = "kimi-global"
	testDeepSeekChat   = "deepseek-chat"
	testDeepSeekReason = "deepseek-reasoner"
	testKeyDeepSeek    = "sk-deepseek"
	testKeyMoonshot    = "sk-moonshot"
)

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

func TestResolveLLMModelReturnsEmptyWhenNoOverride(t *testing.T) {
	cfg := &Config{}

	if got := cfg.ResolveLLMModel(LLMModelAgentDispatcher); got != "" {
		t.Fatalf("expected empty model override, got %q", got)
	}
	if got := cfg.ResolveLLMModel(LLMModelAgentOfferSummaryGenerator); got != DefaultOfferSummaryLLMModel {
		t.Fatalf("expected offer summary fallback model %q, got %q", DefaultOfferSummaryLLMModel, got)
	}
}

func TestResolveAgentModelPreventsCrossProviderMismatch(t *testing.T) {
	cfg := &Config{
		LLMProvider:    testDeepSeekReason,
		DeepSeekAPIKey: testKeyDeepSeek,
		MoonshotAPIKey: testKeyMoonshot,
	}

	// No per-agent override: should use DeepSeek provider with empty override.
	provCfg, modelOvr := cfg.ResolveAgentModel(LLMModelAgentGatekeeper)
	if provCfg.Provider != LLMProviderDeepSeek {
		t.Fatalf("expected deepseek provider, got %q", provCfg.Provider)
	}
	if modelOvr != "" {
		t.Fatalf("expected empty model override, got %q", modelOvr)
	}
	if provCfg.Model != testDeepSeekChat {
		t.Fatalf("expected non-reasoning model %q, got %q", testDeepSeekChat, provCfg.Model)
	}
	if provCfg.ReasoningModel != testDeepSeekReason {
		t.Fatalf("expected reasoning model %q, got %q", testDeepSeekReason, provCfg.ReasoningModel)
	}

	// Offer summary has hardcoded moonshot-v1-8k default: should re-resolve to Kimi.
	provCfg, modelOvr = cfg.ResolveAgentModel(LLMModelAgentOfferSummaryGenerator)
	if provCfg.Provider != LLMProviderKimi {
		t.Fatalf("expected kimi provider for offer summary, got %q", provCfg.Provider)
	}
	if modelOvr != "" {
		t.Fatalf("expected empty model override after re-resolve, got %q", modelOvr)
	}
	if provCfg.Model != DefaultOfferSummaryLLMModel {
		t.Fatalf("expected model %q, got %q", DefaultOfferSummaryLLMModel, provCfg.Model)
	}
}

func TestResolveAgentModelKeepsSameProviderOverride(t *testing.T) {
	cfg := &Config{
		LLMProvider:     testDeepSeekReason,
		DeepSeekAPIKey:  testKeyDeepSeek,
		LLMModelDefault: testDeepSeekChat,
	}

	provCfg, modelOvr := cfg.ResolveAgentModel(LLMModelAgentEstimator)
	if provCfg.Provider != LLMProviderDeepSeek {
		t.Fatalf("expected deepseek provider, got %q", provCfg.Provider)
	}
	if modelOvr != testDeepSeekChat {
		t.Fatalf("expected model override %q, got %q", testDeepSeekChat, modelOvr)
	}
}

func TestResolveProviderConfigKeepsChatModelWhenProviderIsReasonerName(t *testing.T) {
	cfg := &Config{LLMProvider: testDeepSeekReason, DeepSeekAPIKey: testKeyDeepSeek}

	provCfg := cfg.ResolveProviderConfig(testDeepSeekReason)
	if provCfg.Provider != LLMProviderDeepSeek {
		t.Fatalf("expected deepseek provider, got %q", provCfg.Provider)
	}
	if provCfg.Model != testDeepSeekChat {
		t.Fatalf("expected default chat model %q, got %q", testDeepSeekChat, provCfg.Model)
	}
	if provCfg.ReasoningModel != testDeepSeekReason {
		t.Fatalf("expected reasoning model %q, got %q", testDeepSeekReason, provCfg.ReasoningModel)
	}
}

func TestResolveProviderConfigLocksBothModelsForSingleModeProviderOverride(t *testing.T) {
	cfg := &Config{MoonshotAPIKey: testKeyMoonshot}

	provCfg := cfg.ResolveProviderConfig(defaultKimiModel)
	if provCfg.Provider != LLMProviderKimi {
		t.Fatalf("expected kimi provider, got %q", provCfg.Provider)
	}
	if provCfg.Model != defaultKimiModel || provCfg.ReasoningModel != defaultKimiModel {
		t.Fatalf("expected both kimi models to be locked to %q, got model=%q reasoning=%q", defaultKimiModel, provCfg.Model, provCfg.ReasoningModel)
	}
}

func TestLLMSelectorWarningsFlagModelNamesInProviderSlots(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		LLMProvider: testDeepSeekReason,
	}

	warnings := cfg.LLMSelectorWarnings()
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %#v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], `LLM_PROVIDER is set to model name "deepseek-reasoner"`) {
		t.Fatalf("unexpected primary warning: %q", warnings[0])
	}
	if !strings.Contains(warnings[0], `prefer provider id "deepseek"`) {
		t.Fatalf("expected deepseek guidance, got %q", warnings[0])
	}
}

func TestLLMSelectorWarningsIgnoreProviderIDs(t *testing.T) {
	t.Parallel()

	cfg := &Config{LLMProvider: LLMProviderDeepSeek}
	if warnings := cfg.LLMSelectorWarnings(); len(warnings) != 0 {
		t.Fatalf("expected no warnings for provider ids, got %#v", warnings)
	}
}

func TestLLMSelectorWarningsFlagUnknownSelectors(t *testing.T) {
	t.Parallel()

	cfg := &Config{LLMProvider: "unknown-provider"}
	warnings := cfg.LLMSelectorWarnings()
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %#v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], `LLM_PROVIDER has unknown selector "unknown-provider"`) {
		t.Fatalf("unexpected warning: %q", warnings[0])
	}
	if !strings.Contains(warnings[0], `falls back to provider "kimi"`) {
		t.Fatalf("expected fallback explanation, got %q", warnings[0])
	}
}
