package agent

import "portal_final_backend/platform/ai/moonshot"

func newMoonshotModelConfig(apiKey string, modelName string) moonshot.Config {
	return moonshot.Config{
		APIKey:          apiKey,
		Model:           modelName,
		DisableThinking: true,
	}
}

func newMoonshotReasoningModelConfig(apiKey string, modelName string) moonshot.Config {
	return moonshot.Config{
		APIKey:          apiKey,
		Model:           modelName,
		DisableThinking: false,
	}
}
