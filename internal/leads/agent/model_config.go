package agent

import (
	"log/slog"

	"google.golang.org/adk/model"

	"portal_final_backend/platform/ai/fallback"
	"portal_final_backend/platform/ai/openaicompat"
	"portal_final_backend/platform/config"
)

// BuildLLM creates a model.LLM from a primary config and optional fallback config.
// When fallbackCfg is non-nil, the returned model transparently falls back to
// the secondary provider on errors with circuit-breaker protection.
func BuildLLM(primary openaicompat.Config, fallbackCfg *openaicompat.Config, logger *slog.Logger) model.LLM {
	primaryModel := openaicompat.NewModel(primary)
	if fallbackCfg == nil {
		return primaryModel
	}
	secondaryModel := openaicompat.NewModel(*fallbackCfg)
	return fallback.NewModel(fallback.Config{
		Primary:   primaryModel,
		Secondary: secondaryModel,
		Logger:    logger,
	})
}

// newModelConfig builds an openaicompat.Config from a resolved provider
// configuration. When reasoning is true the provider-specific reasoning
// model is selected; otherwise the default model. Thinking-mode payload
// behaviour differs per provider (Kimi toggles it, DeepSeek ignores it).
func newModelConfig(providerCfg config.LLMProviderConfig, reasoning bool) openaicompat.Config {
	modelName := providerCfg.Model
	disableThinking := true

	if reasoning {
		modelName = providerCfg.ReasoningModel
		// On Kimi thinking mode is a payload toggle on the same model.
		// On DeepSeek reasoning is a separate model, no toggle needed.
		if providerCfg.Provider == config.LLMProviderKimi {
			disableThinking = false
		}
	}

	return openaicompat.Config{
		APIKey:          providerCfg.APIKey,
		BaseURL:         providerCfg.BaseURL,
		Model:           modelName,
		Provider:        providerCfg.Provider,
		DisableThinking: disableThinking,
		SupportsVision:  config.ProviderSupportsVision(providerCfg.Provider),
	}
}

// NewProviderModelConfig is like newModelConfig but allows an explicit
// model-name override (from per-agent env vars set via ResolveLLMModel).
// When the override is non-empty it replaces both default and reasoning models.
func NewProviderModelConfig(providerCfg config.LLMProviderConfig, reasoning bool, modelOverride string) openaicompat.Config {
	cfg := newModelConfig(providerCfg, reasoning)
	if modelOverride != "" {
		cfg.Model = modelOverride
	}
	return cfg
}

// Legacy helpers — kept for backward compatibility during migration.
func newMoonshotModelConfig(apiKey string, modelName string) openaicompat.Config {
	return openaicompat.Config{
		APIKey:          apiKey,
		Model:           modelName,
		Provider:        config.LLMProviderKimi,
		DisableThinking: true,
	}
}

func newMoonshotReasoningModelConfig(apiKey string, modelName string) openaicompat.Config {
	return openaicompat.Config{
		APIKey:          apiKey,
		Model:           modelName,
		Provider:        config.LLMProviderKimi,
		DisableThinking: false,
	}
}
