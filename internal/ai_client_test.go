package internal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alvinunreal/tmuxai/config"
)

func TestAzureOpenAIEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openai/deployments/test-dep/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("api-version") != "2025-04-01-preview" {
			t.Errorf("missing api-version query")
		}
		if r.Header.Get("api-key") != "test-key" {
			t.Errorf("missing api-key header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{},
		OpenAI:     config.OpenAIConfig{},
		AzureOpenAI: config.AzureOpenAIConfig{
			APIKey:         "test-key",
			APIBase:        server.URL,
			APIVersion:     "2025-04-01-preview",
			DeploymentName: "test-dep",
		},
	}

	client := NewAiClient(cfg)
	msg := []Message{{Role: "user", Content: "hi"}}
	resp, err := client.ChatCompletion(context.Background(), msg, "model")
	if err != nil {
		t.Fatalf("ChatCompletion error: %v", err)
	}
	if resp != "ok" {
		t.Errorf("unexpected response: %s", resp)
	}
}

func TestOpenAIResponsesEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing Authorization header: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"ok from responses api","id":"test-id","object":"response","created_at":1234567890}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{},
		OpenAI: config.OpenAIConfig{
			APIKey:  "test-key",
			Model:   "gpt-5",
			BaseURL: server.URL,
		},
		AzureOpenAI: config.AzureOpenAIConfig{},
	}

	client := NewAiClient(cfg)
	msg := []Message{{Role: "user", Content: "hi"}}
	resp, err := client.Response(context.Background(), msg, "gpt-5")
	if err != nil {
		t.Fatalf("Response error: %v", err)
	}
	if resp != "ok from responses api" {
		t.Errorf("unexpected response: %s", resp)
	}
}

func TestOpenAIResponsesEndpointWithSystemMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing Authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":[{"type":"message","status":"completed","content":[{"type":"text","text":"ok with system instruction"}]}],"output_text":"ok with system instruction"}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{},
		OpenAI: config.OpenAIConfig{
			APIKey:  "test-key",
			Model:   "gpt-5-codex",
			BaseURL: server.URL,
		},
		AzureOpenAI: config.AzureOpenAIConfig{},
	}

	client := NewAiClient(cfg)
	msg := []Message{
		{Role: "system", Content: "You are a helpful assistant"},
		{Role: "user", Content: "hi"},
	}
	resp, err := client.Response(context.Background(), msg, "gpt-5-codex")
	if err != nil {
		t.Fatalf("Response error: %v", err)
	}
	if resp != "ok with system instruction" {
		t.Errorf("unexpected response: %s", resp)
	}
}

func TestDetermineAPIType(t *testing.T) {
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			APIKey: "openrouter-key",
			Model:  "openrouter-model",
		},
		OpenAI: config.OpenAIConfig{
			APIKey: "openai-key",
			Model:  "gpt-5-codex",
		},
		AzureOpenAI: config.AzureOpenAIConfig{},
	}

	client := NewAiClient(cfg)

	// Test OpenAI API type (highest priority) - should work with any model when OpenAI key is present
	apiType := client.determineAPIType("gpt-5-codex")
	if apiType != "responses" {
		t.Errorf("expected 'responses', got %s", apiType)
	}

	// Test that OpenAI is selected regardless of model when API key is present
	apiType = client.determineAPIType("any-model")
	if apiType != "responses" {
		t.Errorf("expected 'responses' for any model when OpenAI key is present, got %s", apiType)
	}

	// Test Azure API type
	cfg.OpenAI.APIKey = ""
	cfg.AzureOpenAI.APIKey = "azure-key"
	client = NewAiClient(cfg)
	apiType = client.determineAPIType("any-model")
	if apiType != "azure" {
		t.Errorf("expected 'azure', got %s", apiType)
	}

	// Test OpenRouter API type (default)
	cfg.AzureOpenAI.APIKey = ""
	client = NewAiClient(cfg)
	apiType = client.determineAPIType("openrouter-model")
	if apiType != "openrouter" {
		t.Errorf("expected 'openrouter', got %s", apiType)
	}
}

func TestSessionOverrides(t *testing.T) {
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			APIKey: "original-openrouter-key",
			Model:  "original-openrouter-model",
		},
		OpenAI: config.OpenAIConfig{
			APIKey: "original-openai-key",
			Model:  "original-openai-model",
		},
		AzureOpenAI: config.AzureOpenAIConfig{
			APIKey:         "original-azure-key",
			DeploymentName: "original-deployment",
		},
	}

	manager := &Manager{
		Config:           cfg,
		SessionOverrides: make(map[string]interface{}),
	}

	// Test that original values are returned without overrides
	if manager.GetOpenAIAPIKey() != "original-openai-key" {
		t.Errorf("expected original OpenAI API key, got %s", manager.GetOpenAIAPIKey())
	}
	if manager.GetOpenAIModel() != "original-openai-model" {
		t.Errorf("expected original OpenAI model, got %s", manager.GetOpenAIModel())
	}

	// Test session overrides for OpenAI
	manager.SessionOverrides["openai.api_key"] = "override-openai-key"
	manager.SessionOverrides["openai.model"] = "override-openai-model"
	manager.SessionOverrides["openai.base_url"] = "https://override.example.com"

	if manager.GetOpenAIAPIKey() != "override-openai-key" {
		t.Errorf("expected overridden OpenAI API key, got %s", manager.GetOpenAIAPIKey())
	}
	if manager.GetOpenAIModel() != "override-openai-model" {
		t.Errorf("expected overridden OpenAI model, got %s", manager.GetOpenAIModel())
	}
	if manager.GetOpenAIBaseURL() != "https://override.example.com" {
		t.Errorf("expected overridden OpenAI base URL, got %s", manager.GetOpenAIBaseURL())
	}

	// Test session overrides for Azure
	manager.SessionOverrides["azure_openai.api_key"] = "override-azure-key"
	manager.SessionOverrides["azure_openai.deployment_name"] = "override-deployment"

	if manager.GetAzureOpenAIAPIKey() != "override-azure-key" {
		t.Errorf("expected overridden Azure API key, got %s", manager.GetAzureOpenAIAPIKey())
	}
	if manager.GetAzureOpenAIDeploymentName() != "override-deployment" {
		t.Errorf("expected overridden Azure deployment, got %s", manager.GetAzureOpenAIDeploymentName())
	}

	// Test that GetModel() respects session overrides
	// With OpenAI override
	if manager.GetModel() != "override-openai-model" {
		t.Errorf("expected overridden OpenAI model from GetModel(), got %s", manager.GetModel())
	}

	// Test clearing OpenAI config entirely to fall back to Azure
	originalOpenAIKey := manager.Config.OpenAI.APIKey
	manager.Config.OpenAI.APIKey = "" // Clear original OpenAI API key
	delete(manager.SessionOverrides, "openai.api_key")
	if manager.GetModel() != "override-deployment" {
		t.Errorf("expected overridden Azure deployment from GetModel(), got %s", manager.GetModel())
	}

	// Clear Azure config entirely to fall back to OpenRouter
	originalAzureKey := manager.Config.AzureOpenAI.APIKey
	manager.Config.AzureOpenAI.APIKey = "" // Clear original Azure API key
	delete(manager.SessionOverrides, "azure_openai.api_key")
	if manager.GetModel() != "original-openrouter-model" {
		t.Errorf("expected original OpenRouter model from GetModel(), got %s", manager.GetModel())
	}

	// Restore original config for other tests
	manager.Config.OpenAI.APIKey = originalOpenAIKey
	manager.Config.AzureOpenAI.APIKey = originalAzureKey
}

func TestModelProfiles(t *testing.T) {
	// Test model profile configuration
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			APIKey: "default-openrouter-key",
			Model:  "default-openrouter-model",
		},
		OpenAI: config.OpenAIConfig{},
		AzureOpenAI: config.AzureOpenAIConfig{},
		Models: map[string]config.ModelConfig{
			"fast": {
				Provider: "openrouter",
				Model:    "google/gemini-2.5-flash-preview",
				APIKey:   "fast-model-key",
				BaseURL:  "https://openrouter.ai/api/v1",
			},
			"powerful": {
				Provider: "openai",
				Model:    "gpt-5-codex",
				APIKey:   "powerful-model-key",
				BaseURL:  "https://api.openai.com/v1",
			},
			"azure-model": {
				Provider:       "azure",
				Model:          "gpt-4o",
				APIKey:         "azure-model-key",
				APIBase:        "https://test.openai.azure.com",
				APIVersion:     "2025-04-01-preview",
				DeploymentName: "test-deployment",
			},
		},
		DefaultModel: "fast",
	}

	// Test GetModel and ListModels
	models := cfg.ListModels()
	if len(models) != 3 {
		t.Errorf("expected 3 models, got %d", len(models))
	}

	fastModel, ok := cfg.GetModel("fast")
	if !ok {
		t.Error("expected to find 'fast' model")
	}
	if fastModel.Provider != "openrouter" {
		t.Errorf("expected provider 'openrouter', got %s", fastModel.Provider)
	}

	// Test AI client with model profile
	client := NewAiClient(cfg)
	client.SetCurrentModel("fast")

	apiKey, baseURL, apiType, _ := client.getEffectiveConfig()
	if apiKey != "fast-model-key" {
		t.Errorf("expected fast-model-key, got %s", apiKey)
	}
	if baseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("expected openrouter base URL, got %s", baseURL)
	}
	if apiType != "openrouter" {
		t.Errorf("expected openrouter API type, got %s", apiType)
	}

	// Test switching to OpenAI model profile
	client.SetCurrentModel("powerful")
	apiKey, baseURL, apiType, _ = client.getEffectiveConfig()
	if apiKey != "powerful-model-key" {
		t.Errorf("expected powerful-model-key, got %s", apiKey)
	}
	if apiType != "responses" {
		t.Errorf("expected responses API type for OpenAI, got %s", apiType)
	}

	// Test switching to Azure model profile
	client.SetCurrentModel("azure-model")
	apiKey, _, apiType, deployment := client.getEffectiveConfig()
	if apiKey != "azure-model-key" {
		t.Errorf("expected azure-model-key, got %s", apiKey)
	}
	if apiType != "azure" {
		t.Errorf("expected azure API type, got %s", apiType)
	}
	if deployment != "test-deployment" {
		t.Errorf("expected test-deployment, got %s", deployment)
	}
}

func TestModelProfileEndpointIntegration(t *testing.T) {
	// Test that model profiles correctly configure API endpoints
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the correct API key header
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-profile-key" {
			t.Errorf("unexpected auth header: %s", authHeader)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"profile response"}}]}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{},
		OpenAI:     config.OpenAIConfig{},
		AzureOpenAI: config.AzureOpenAIConfig{},
		Models: map[string]config.ModelConfig{
			"test-profile": {
				Provider: "openrouter",
				Model:    "test-model",
				APIKey:   "test-profile-key",
				BaseURL:  server.URL,
			},
		},
	}

	client := NewAiClient(cfg)
	client.SetCurrentModel("test-profile")

	msg := []Message{{Role: "user", Content: "test"}}
	resp, err := client.ChatCompletion(context.Background(), msg, "test-model")
	if err != nil {
		t.Fatalf("ChatCompletion error: %v", err)
	}
	if resp != "profile response" {
		t.Errorf("unexpected response: %s", resp)
	}
}

func TestManagerModelSwitching(t *testing.T) {
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			APIKey: "default-key",
			Model:  "default-model",
		},
		Models: map[string]config.ModelConfig{
			"fast": {
				Provider: "openrouter",
				Model:    "fast-model",
				APIKey:   "fast-key",
			},
			"powerful": {
				Provider: "openai",
				Model:    "powerful-model",
				APIKey:   "powerful-key",
			},
		},
		DefaultModel: "fast",
	}

	manager := &Manager{
		Config:           cfg,
		AiClient:         NewAiClient(cfg),
		SessionOverrides: make(map[string]interface{}),
	}

	// Test default model from config
	if manager.GetCurrentModelName() != "fast" {
		t.Errorf("expected 'fast' as default model, got %s", manager.GetCurrentModelName())
	}

	// Test switching models
	err := manager.SetCurrentModel("powerful")
	if err != nil {
		t.Errorf("SetCurrentModel error: %v", err)
	}
	if manager.GetCurrentModelName() != "powerful" {
		t.Errorf("expected 'powerful', got %s", manager.GetCurrentModelName())
	}

	// Test that AiClient is synced
	if manager.AiClient.GetCurrentModel() != "powerful" {
		t.Errorf("AiClient not synced, expected 'powerful', got %s", manager.AiClient.GetCurrentModel())
	}

	// Test switching to non-existent model
	err = manager.SetCurrentModel("non-existent")
	if err == nil {
		t.Error("expected error when switching to non-existent model")
	}

	// Test GetModel returns correct model from profile
	if manager.GetModel() != "powerful-model" {
		t.Errorf("expected 'powerful-model', got %s", manager.GetModel())
	}
}

func TestBackwardCompatibility(t *testing.T) {
	// Test that configs without model profiles still work
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			APIKey: "openrouter-key",
			Model:  "openrouter-model",
		},
		OpenAI: config.OpenAIConfig{
			APIKey: "openai-key",
			Model:  "openai-model",
		},
		Models: map[string]config.ModelConfig{}, // Empty models
	}

	client := NewAiClient(cfg)

	// Should fall back to priority-based selection (OpenAI > Azure > OpenRouter)
	apiKey, _, apiType, _ := client.getEffectiveConfig()
	if apiType != "responses" {
		t.Errorf("expected responses API type (OpenAI), got %s", apiType)
	}
	if apiKey != "openai-key" {
		t.Errorf("expected openai-key, got %s", apiKey)
	}

	// Test with no OpenAI config
	cfg.OpenAI.APIKey = ""
	client = NewAiClient(cfg)
	apiKey, _, apiType, _ = client.getEffectiveConfig()
	if apiType != "openrouter" {
		t.Errorf("expected openrouter API type, got %s", apiType)
	}
	if apiKey != "openrouter-key" {
		t.Errorf("expected openrouter-key, got %s", apiKey)
	}
}

func TestModelProfilePriorityOverDefault(t *testing.T) {
	// Test that model profiles take priority over default config
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			APIKey: "default-openrouter-key",
			Model:  "default-openrouter-model",
		},
		OpenAI: config.OpenAIConfig{
			APIKey: "default-openai-key",
			Model:  "default-openai-model",
		},
		Models: map[string]config.ModelConfig{
			"custom": {
				Provider: "openrouter",
				Model:    "custom-model",
				APIKey:   "custom-key",
				BaseURL:  "https://custom.example.com/v1",
			},
		},
	}

	manager := &Manager{
		Config:           cfg,
		AiClient:         NewAiClient(cfg),
		SessionOverrides: make(map[string]interface{}),
	}

	// Without model profile, should use default OpenAI (highest priority)
	if manager.GetModel() != "default-openai-model" {
		t.Errorf("expected default-openai-model, got %s", manager.GetModel())
	}

	// With model profile, should use profile
	err := manager.SetCurrentModel("custom")
	if err != nil {
		t.Fatalf("SetCurrentModel error: %v", err)
	}
	if manager.GetModel() != "custom-model" {
		t.Errorf("expected custom-model from profile, got %s", manager.GetModel())
	}

	// Verify AiClient uses profile config
	apiKey, baseURL, _, _ := manager.AiClient.getEffectiveConfig()
	if apiKey != "custom-key" {
		t.Errorf("expected custom-key, got %s", apiKey)
	}
	if baseURL != "https://custom.example.com/v1" {
		t.Errorf("expected custom base URL, got %s", baseURL)
	}
}
