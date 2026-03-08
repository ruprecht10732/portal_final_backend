package agent

import "testing"

func TestNewMoonshotModelConfigDisablesThinkingByDefault(t *testing.T) {
	cfg := newMoonshotModelConfig("api-key", "kimi-k2.5")
	if !cfg.DisableThinking {
		t.Fatal("expected default Moonshot config to disable thinking")
	}
}

func TestNewMoonshotReasoningModelConfigKeepsThinkingEnabled(t *testing.T) {
	cfg := newMoonshotReasoningModelConfig("api-key", "kimi-k2.5")
	if cfg.DisableThinking {
		t.Fatal("expected reasoning Moonshot config to keep thinking enabled")
	}
}
