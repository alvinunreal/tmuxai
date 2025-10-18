package internal

import (
	"testing"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelConfiguration(t *testing.T) {
	tests := []struct {
		name           string
		config         *config.Config
		expectedModel  string
		expectedValid  bool
		expectedCount  int
	}{
		{
			name: "multiple models with default",
			config: &config.Config{
				DefaultModel: "gpt4",
				Models: map[string]config.ModelConfig{
					"gpt4": {
						Provider: "openai",
						Model:    "gpt-4",
						APIKey:   "sk-test-key",
					},
					"claude": {
						Provider: "openrouter",
						Model:    "claude-3.5-sonnet",
						APIKey:   "sk-or-test-key",
					},
				},
			},
			expectedModel: "gpt4",
			expectedValid: true,
			expectedCount: 2,
		},
		{
			name: "multiple models without default (auto-select first)",
			config: &config.Config{
				DefaultModel: "",
				Models: map[string]config.ModelConfig{
					"claude": {
						Provider: "openrouter",
						Model:    "claude-3.5-sonnet",
						APIKey:   "sk-or-test-key",
					},
					"gpt4": {
						Provider: "openai",
						Model:    "gpt-4",
						APIKey:   "sk-test-key",
					},
				},
			},
			expectedModel: "claude", // Should auto-select first alphabetically
			expectedValid: true,
			expectedCount: 2,
		},
		{
			name: "single model without default",
			config: &config.Config{
				DefaultModel: "",
				Models: map[string]config.ModelConfig{
					"gpt4": {
						Provider: "openai",
						Model:    "gpt-4",
						APIKey:   "sk-test-key",
					},
				},
			},
			expectedModel: "gpt4",
			expectedValid: true,
			expectedCount: 1,
		},
		{
			name: "no models configured",
			config: &config.Config{
				DefaultModel: "",
				Models:       map[string]config.ModelConfig{},
			},
			expectedModel: "",
			expectedValid: false,
			expectedCount: 0,
		},
		{
			name: "models configured but no API keys",
			config: &config.Config{
				DefaultModel: "gpt4",
				Models: map[string]config.ModelConfig{
					"gpt4": {
						Provider: "openai",
						Model:    "gpt-4",
						APIKey:   "", // No API key
					},
					"claude": {
						Provider: "openrouter",
						Model:    "claude-3.5-sonnet",
						APIKey:   "", // No API key
					},
				},
			},
			expectedModel: "gpt4",
			expectedValid: false,
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &Manager{
				Config:           tt.config,
				SessionOverrides: make(map[string]interface{}),
				LoadedKBs:        make(map[string]string),
			}

			// Test GetAvailableModels
			availableModels := manager.GetAvailableModels()
			assert.Equal(t, tt.expectedCount, len(availableModels))

			// Test GetModelsDefault (auto-selection logic)
			defaultModel := manager.GetModelsDefault()
			assert.Equal(t, tt.expectedModel, defaultModel)

			// Test hasValidAIConfiguration
			hasValid := manager.hasValidAIConfiguration()
			assert.Equal(t, tt.expectedValid, hasValid)

			// Test GetCurrentModelConfig
			if tt.expectedCount > 0 {
				modelConfig, exists := manager.GetCurrentModelConfig()
				assert.True(t, exists)
				if tt.expectedModel != "" {
					// Get the actual model config for the expected model
					expectedConfig := tt.config.Models[tt.expectedModel]
					assert.Equal(t, expectedConfig.Provider, modelConfig.Provider)
					assert.Equal(t, expectedConfig.Model, modelConfig.Model)
					assert.Equal(t, expectedConfig.APIKey, modelConfig.APIKey)
				}
			}
		})
	}
}

func TestModelConfigurationWithSessionOverrides(t *testing.T) {
	cfg := &config.Config{
		DefaultModel: "gpt4",
		Models: map[string]config.ModelConfig{
			"gpt4": {
				Provider: "openai",
				Model:    "gpt-4",
				APIKey:   "sk-test-key",
			},
			"claude": {
				Provider: "openrouter",
				Model:    "claude-3.5-sonnet",
				APIKey:   "sk-or-test-key",
			},
		},
	}

	manager := &Manager{
		Config:           cfg,
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
	}

	// Test session override
	manager.SetModelsDefault("claude")
	assert.Equal(t, "claude", manager.GetModelsDefault())

	// Test getting the overridden model config
	modelConfig, exists := manager.GetModelConfig("claude")
	require.True(t, exists)
	assert.Equal(t, "openrouter", modelConfig.Provider)
	assert.Equal(t, "claude-3.5-sonnet", modelConfig.Model)
}

func TestGetModel(t *testing.T) {
	tests := []struct {
		name           string
		config         *config.Config
		expectedModel string
	}{
		{
			name: "new model system with default",
			config: &config.Config{
				DefaultModel: "gpt4",
				Models: map[string]config.ModelConfig{
					"gpt4": {
						Provider: "openai",
						Model:    "gpt-4",
						APIKey:   "sk-test-key",
					},
				},
			},
			expectedModel: "gpt-4",
		},
		{
			name: "new model system auto-select first",
			config: &config.Config{
				DefaultModel: "",
				Models: map[string]config.ModelConfig{
					"claude": {
						Provider: "openrouter",
						Model:    "claude-3.5-sonnet",
						APIKey:   "sk-or-test-key",
					},
					"gpt4": {
						Provider: "openai",
						Model:    "gpt-4",
						APIKey:   "sk-test-key",
					},
				},
			},
			expectedModel: "claude-3.5-sonnet", // First alphabetically
		},
		{
			name: "fallback to legacy openai",
			config: &config.Config{
				DefaultModel: "",
				Models:       map[string]config.ModelConfig{},
				OpenAI: config.OpenAIConfig{
					APIKey: "sk-test-key",
					Model:  "gpt-5-codex",
				},
			},
			expectedModel: "gpt-5-codex",
		},
		{
			name: "fallback to legacy openrouter",
			config: &config.Config{
				DefaultModel: "",
				Models:       map[string]config.ModelConfig{},
				OpenRouter: config.OpenRouterConfig{
					APIKey: "sk-or-test-key",
					Model:  "gemini-flash",
				},
			},
			expectedModel: "gemini-flash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &Manager{
				Config:           tt.config,
				SessionOverrides: make(map[string]interface{}),
				LoadedKBs:        make(map[string]string),
			}

			actualModel := manager.GetModel()
			assert.Equal(t, tt.expectedModel, actualModel)
		})
	}
}

func TestLegacyModelConfig(t *testing.T) {
	tests := []struct {
		name           string
		config         *config.Config
		expectedProvider string
		expectedModel   string
	}{
		{
			name: "openai priority",
			config: &config.Config{
				OpenAI: config.OpenAIConfig{
					APIKey: "sk-test-key",
					Model:  "gpt-4",
				},
				OpenRouter: config.OpenRouterConfig{
					APIKey: "sk-or-test-key",
					Model:  "gemini-flash",
				},
			},
			expectedProvider: "openai",
			expectedModel:   "gpt-4",
		},
		{
			name: "azure priority over openrouter",
			config: &config.Config{
				AzureOpenAI: config.AzureOpenAIConfig{
					APIKey:         "sk-test-key",
					DeploymentName: "gpt-4o",
				},
				OpenRouter: config.OpenRouterConfig{
					APIKey: "sk-or-test-key",
					Model:  "gemini-flash",
				},
			},
			expectedProvider: "azure",
			expectedModel:   "gpt-4o",
		},
		{
			name: "openrouter fallback",
			config: &config.Config{
				OpenRouter: config.OpenRouterConfig{
					APIKey: "sk-or-test-key",
					Model:  "gemini-flash",
				},
			},
			expectedProvider: "openrouter",
			expectedModel:   "gemini-flash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &Manager{
				Config:           tt.config,
				SessionOverrides: make(map[string]interface{}),
				LoadedKBs:        make(map[string]string),
			}

			// Empty models to force legacy fallback
			manager.Config.Models = make(map[string]config.ModelConfig)
			manager.Config.DefaultModel = ""

			modelConfig, exists := manager.GetCurrentModelConfig()
			require.True(t, exists)
			assert.Equal(t, tt.expectedProvider, modelConfig.Provider)
			assert.Equal(t, tt.expectedModel, modelConfig.Model)
		})
	}
}