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

func TestGitHubCopilotEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/copilot/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing or incorrect Authorization header: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Copilot-Integration-Id") != "tmuxai" {
			t.Errorf("missing Copilot-Integration-Id header")
		}
		if r.Header.Get("Copilot-Integration-Version") != "1.0.0" {
			t.Errorf("missing Copilot-Integration-Version header")
		}
		if r.Header.Get("User-Agent") != "TmuxAI/1.0.0" {
			t.Errorf("missing User-Agent header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"github copilot response"}}]}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{},
		AzureOpenAI: config.AzureOpenAIConfig{},
		GitHubCopilot: config.GitHubCopilotConfig{
			Token:   "test-token",
			Model:   "gpt-4o",
			BaseURL: server.URL,
		},
	}

	client := NewAiClient(cfg)
	msg := []Message{{Role: "user", Content: "test message"}}
	resp, err := client.ChatCompletion(context.Background(), msg, "gpt-4o")
	if err != nil {
		t.Fatalf("ChatCompletion error: %v", err)
	}
	if resp != "github copilot response" {
		t.Errorf("unexpected response: %s", resp)
	}
}

func TestProviderPrecedence(t *testing.T) {
	// Test GitHub Copilot has priority over other providers
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			APIKey: "openrouter-key",
			Model:  "openrouter-model",
		},
		AzureOpenAI: config.AzureOpenAIConfig{
			APIKey:         "azure-key",
			DeploymentName: "azure-deployment",
		},
		GitHubCopilot: config.GitHubCopilotConfig{
			Token: "github-token",
			Model: "gpt-4o",
		},
	}
	
	// Mock Manager with no session overrides
	manager := &Manager{
		Config:           cfg,
		SessionOverrides: make(map[string]interface{}),
	}
	
	model := manager.GetAIModel()
	if model != "gpt-4o" {
		t.Errorf("Expected GitHub Copilot model 'gpt-4o', got: %s", model)
	}
	
	// Test Azure OpenAI precedence when GitHub Copilot is not configured
	cfg.GitHubCopilot.Token = ""
	model = manager.GetAIModel()
	if model != "azure-deployment" {
		t.Errorf("Expected Azure deployment 'azure-deployment', got: %s", model)
	}
	
	// Test OpenRouter fallback when neither GitHub Copilot nor Azure is configured
	cfg.AzureOpenAI.APIKey = ""
	model = manager.GetAIModel()
	if model != "openrouter-model" {
		t.Errorf("Expected OpenRouter model 'openrouter-model', got: %s", model)
	}
	
	// Test session override for GitHub Copilot model
	cfg.GitHubCopilot.Token = "github-token"
	manager.SessionOverrides["github_copilot.model"] = "gpt-4-turbo"
	model = manager.GetAIModel()
	if model != "gpt-4-turbo" {
		t.Errorf("Expected session override 'gpt-4-turbo', got: %s", model)
	}
}
