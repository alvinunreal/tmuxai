package internal

import (
	"testing"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/stretchr/testify/assert"
)

// Test setting and retrieving active model
func TestSetActiveModel(t *testing.T) {
	cfg := &config.Config{
		Models: map[string]*config.ModelConfig{
			"fast": {
				Provider: "openrouter",
				Model:    "google/gemini-2.5-flash-preview",
				APIKey:   "test-key",
				BaseURL:  "https://openrouter.ai/api/v1",
			},
			"powerful": {
				Provider: "openai",
				Model:    "gpt-5-codex",
				APIKey:   "test-openai-key",
			},
		},
		DefaultModel: "fast",
	}

	manager := &Manager{
		Config:           cfg,
		AiClient:         NewAiClient(cfg),
		SessionOverrides: make(map[string]interface{}),
		ActiveModel:      cfg.DefaultModel,
	}

	// Test setting to a valid model
	err := manager.SetActiveModel("powerful")
	assert.NoError(t, err)
	assert.Equal(t, "powerful", manager.ActiveModel)

	// Verify session overrides were set
	assert.Equal(t, "test-openai-key", manager.SessionOverrides["openai.api_key"])
	assert.Equal(t, "gpt-5-codex", manager.SessionOverrides["openai.model"])

	// Test setting to an invalid model
	err = manager.SetActiveModel("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// Test listing models
func TestListModels(t *testing.T) {
	cfg := &config.Config{
		Models: map[string]*config.ModelConfig{
			"fast": {
				Provider: "openrouter",
				Model:    "google/gemini-2.5-flash-preview",
			},
			"powerful": {
				Provider: "openai",
				Model:    "gpt-5-codex",
			},
			"local": {
				Provider: "openrouter",
				Model:    "gemma3:1b",
			},
		},
	}

	manager := &Manager{
		Config:           cfg,
		SessionOverrides: make(map[string]interface{}),
	}

	models := manager.ListModels()
	assert.Len(t, models, 3)
	assert.Contains(t, models, "fast")
	assert.Contains(t, models, "powerful")
	assert.Contains(t, models, "local")
}

// Test GetActiveModelInfo with active model
func TestGetActiveModelInfo_WithActiveModel(t *testing.T) {
	cfg := &config.Config{
		Models: map[string]*config.ModelConfig{
			"fast": {
				Provider: "openrouter",
				Model:    "google/gemini-2.5-flash-preview",
				APIKey:   "test-key",
				BaseURL:  "https://openrouter.ai/api/v1",
			},
		},
		DefaultModel: "fast",
	}

	manager := &Manager{
		Config:           cfg,
		SessionOverrides: make(map[string]interface{}),
		ActiveModel:      "fast",
	}

	info := manager.GetActiveModelInfo()
	assert.Contains(t, info, "Active Model: fast")
	assert.Contains(t, info, "Provider: openrouter")
	assert.Contains(t, info, "Model: google/gemini-2.5-flash-preview")
	assert.Contains(t, info, "Base URL: https://openrouter.ai/api/v1")
}

// Test GetActiveModelInfo without active model (fallback to direct config)
func TestGetActiveModelInfo_WithoutActiveModel(t *testing.T) {
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			APIKey: "test-key",
			Model:  "google/gemini-2.5-flash-preview",
		},
		Models: map[string]*config.ModelConfig{},
	}

	manager := &Manager{
		Config:           cfg,
		SessionOverrides: make(map[string]interface{}),
		ActiveModel:      "",
	}

	info := manager.GetActiveModelInfo()
	assert.Contains(t, info, "Provider: OpenRouter")
	assert.Contains(t, info, "Model: google/gemini-2.5-flash-preview")
	assert.Contains(t, info, "Using direct configuration")
}

// Test UpdateAiClientForModel with OpenRouter provider
func TestUpdateAiClientForModel_OpenRouter(t *testing.T) {
	cfg := &config.Config{
		Models: map[string]*config.ModelConfig{
			"test": {
				Provider: "openrouter",
				Model:    "test-model",
				APIKey:   "test-key",
				BaseURL:  "https://test.example.com",
			},
		},
	}

	manager := &Manager{
		Config:           cfg,
		AiClient:         NewAiClient(cfg),
		SessionOverrides: make(map[string]interface{}),
		ActiveModel:      "test",
	}

	err := manager.UpdateAiClientForModel()
	assert.NoError(t, err)
	assert.Equal(t, "test-key", manager.SessionOverrides["openrouter.api_key"])
	assert.Equal(t, "test-model", manager.SessionOverrides["openrouter.model"])
	assert.Equal(t, "https://test.example.com", manager.SessionOverrides["openrouter.base_url"])
}

// Test UpdateAiClientForModel with Azure provider
func TestUpdateAiClientForModel_Azure(t *testing.T) {
	cfg := &config.Config{
		Models: map[string]*config.ModelConfig{
			"azure-test": {
				Provider:       "azure",
				APIKey:         "azure-key",
				APIBase:        "https://test.openai.azure.com",
				APIVersion:     "2024-02-01",
				DeploymentName: "gpt-4o",
			},
		},
	}

	manager := &Manager{
		Config:           cfg,
		AiClient:         NewAiClient(cfg),
		SessionOverrides: make(map[string]interface{}),
		ActiveModel:      "azure-test",
	}

	err := manager.UpdateAiClientForModel()
	assert.NoError(t, err)
	assert.Equal(t, "azure-key", manager.SessionOverrides["azure_openai.api_key"])
	assert.Equal(t, "https://test.openai.azure.com", manager.SessionOverrides["azure_openai.api_base"])
	assert.Equal(t, "2024-02-01", manager.SessionOverrides["azure_openai.api_version"])
	assert.Equal(t, "gpt-4o", manager.SessionOverrides["azure_openai.deployment_name"])
}

// Test UpdateAiClientForModel with invalid provider
func TestUpdateAiClientForModel_InvalidProvider(t *testing.T) {
	cfg := &config.Config{
		Models: map[string]*config.ModelConfig{
			"invalid": {
				Provider: "unknown-provider",
				Model:    "test-model",
				APIKey:   "test-key",
			},
		},
	}

	manager := &Manager{
		Config:           cfg,
		AiClient:         NewAiClient(cfg),
		SessionOverrides: make(map[string]interface{}),
		ActiveModel:      "invalid",
	}

	err := manager.UpdateAiClientForModel()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
}

// Test environment variable expansion in model configs
func TestModelConfig_EnvExpansion(t *testing.T) {
	// Set test environment variable
	t.Setenv("TEST_API_KEY", "expanded-key")

	cfg := &config.Config{
		Models: map[string]*config.ModelConfig{
			"test": {
				Provider: "openai",
				Model:    "gpt-5-codex",
				APIKey:   "${TEST_API_KEY}",
			},
		},
	}

	// Manually resolve env vars (this would normally happen in config.Load)
	config.ResolveEnvKeyInConfig(cfg)

	assert.Equal(t, "expanded-key", cfg.Models["test"].APIKey)
}

// Test backward compatibility - works without model profiles
func TestBackwardCompatibility_NoModelProfiles(t *testing.T) {
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			APIKey: "test-key",
			Model:  "google/gemini-2.5-flash-preview",
		},
		Models: map[string]*config.ModelConfig{},
	}

	manager := &Manager{
		Config:           cfg,
		AiClient:         NewAiClient(cfg),
		SessionOverrides: make(map[string]interface{}),
		ActiveModel:      "",
	}

	// Should work without any active model
	models := manager.ListModels()
	assert.Len(t, models, 0)

	// GetActiveModelInfo should show direct config
	info := manager.GetActiveModelInfo()
	assert.Contains(t, info, "Using direct configuration")
}
