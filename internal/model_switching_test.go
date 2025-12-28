package internal

import (
	"testing"

	"github.com/BoringDystopiaDevelopment/tmuxai/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelSwitchingCommands(t *testing.T) {
	cfg := &config.Config{
		DefaultModel: "gpt4",
		Models: map[string]config.ModelConfig{
			"gpt4": {
				Provider: "openai",
				Model:    "gpt-4",
				APIKey:   "sk-test-key-1",
			},
			"claude": {
				Provider: "openrouter",
				Model:    "claude-3.5-sonnet",
				APIKey:   "sk-or-test-key-2",
			},
			"gemini": {
				Provider: "openrouter",
				Model:    "gemini-flash",
				APIKey:   "sk-or-test-key-3",
			},
		},
	}

	manager := &Manager{
		Config:           cfg,
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
	}

	t.Run("GetModelsDefault returns configured default", func(t *testing.T) {
		assert.Equal(t, "gpt4", manager.GetModelsDefault())
	})

	t.Run("SetModelsDefault changes active model", func(t *testing.T) {
		manager.SetModelsDefault("claude")
		assert.Equal(t, "claude", manager.GetModelsDefault())
	})

	t.Run("GetAvailableModels returns all models", func(t *testing.T) {
		models := manager.GetAvailableModels()
		assert.Len(t, models, 3)
		assert.Contains(t, models, "gpt4")
		assert.Contains(t, models, "claude")
		assert.Contains(t, models, "gemini")
	})

	t.Run("GetModelConfig returns correct config", func(t *testing.T) {
		config, exists := manager.GetModelConfig("gpt4")
		require.True(t, exists)
		assert.Equal(t, "openai", config.Provider)
		assert.Equal(t, "gpt-4", config.Model)
		assert.Equal(t, "sk-test-key-1", config.APIKey)
	})

	t.Run("GetModelConfig returns false for invalid model", func(t *testing.T) {
		_, exists := manager.GetModelConfig("nonexistent")
		assert.False(t, exists)
	})
}

func TestModelSwitchingWithNoDefault(t *testing.T) {
	cfg := &config.Config{
		DefaultModel: "", // No default set
		Models: map[string]config.ModelConfig{
			"zeta": {
				Provider: "openrouter",
				Model:    "zeta-model",
				APIKey:   "sk-test-key-1",
			},
			"alpha": {
				Provider: "openai",
				Model:    "alpha-model",
				APIKey:   "sk-test-key-2",
			},
			"beta": {
				Provider: "openrouter",
				Model:    "beta-model",
				APIKey:   "sk-test-key-3",
			},
		},
	}

	manager := &Manager{
		Config:           cfg,
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
	}

	t.Run("GetModelsDefault returns first alphabetically", func(t *testing.T) {
		defaultModel := manager.GetModelsDefault()
		assert.Equal(t, "alpha", defaultModel) // First alphabetically
	})

	t.Run("GetAvailableModels returns sorted models", func(t *testing.T) {
		models := manager.GetAvailableModels()
		assert.Equal(t, []string{"alpha", "beta", "zeta"}, models)
	})
}

func TestModelSwitchingWithSingleModel(t *testing.T) {
	cfg := &config.Config{
		DefaultModel: "",
		Models: map[string]config.ModelConfig{
			"onlymodel": {
				Provider: "openai",
				Model:    "gpt-4",
				APIKey:   "sk-test-key",
			},
		},
	}

	manager := &Manager{
		Config:           cfg,
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
	}

	t.Run("single model is automatically selected", func(t *testing.T) {
		assert.Equal(t, "onlymodel", manager.GetModelsDefault())
		assert.Equal(t, []string{"onlymodel"}, manager.GetAvailableModels())
	})
}

func TestModelSwitchingWithNoModels(t *testing.T) {
	cfg := &config.Config{
		DefaultModel: "",
		Models:       map[string]config.ModelConfig{},
		OpenRouter: config.OpenRouterConfig{
			APIKey: "sk-legacy-key",
			Model:  "legacy-model",
		},
	}

	manager := &Manager{
		Config:           cfg,
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
	}

	t.Run("no models configured falls back to legacy", func(t *testing.T) {
		assert.Equal(t, "", manager.GetModelsDefault())
		assert.Equal(t, 0, len(manager.GetAvailableModels()))

		// Test that legacy config is still accessible
		assert.Equal(t, "sk-legacy-key", manager.Config.OpenRouter.APIKey)
		assert.Equal(t, "legacy-model", manager.Config.OpenRouter.Model)
	})
}

func TestModelSorting(t *testing.T) {
	cfg := &config.Config{
		DefaultModel: "",
		Models: map[string]config.ModelConfig{
			"zebra": {
				Provider: "openrouter",
				Model:    "z-model",
				APIKey:   "sk-test-key-1",
			},
			"alpha": {
				Provider: "openai",
				Model:    "a-model",
				APIKey:   "sk-test-key-2",
			},
			"beta": {
				Provider: "openrouter",
				Model:    "b-model",
				APIKey:   "sk-test-key-3",
			},
		},
	}

	manager := &Manager{
		Config:           cfg,
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
	}

	t.Run("models are returned alphabetically", func(t *testing.T) {
		models := manager.GetAvailableModels()
		assert.Equal(t, []string{"alpha", "beta", "zebra"}, models)

		// Should auto-select first alphabetically
		assert.Equal(t, "alpha", manager.GetModelsDefault())
	})
}