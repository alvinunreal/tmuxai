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
		_, _ = w.Write([]byte(`{"output":[{"type":"message","status":"completed","content":[{"type":"output_text","text":"ok with system instruction"}]}],"output_text":"ok with system instruction"}`))
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
	msg := []Message{
		{Role: "system", Content: "You are a helpful assistant"},
		{Role: "user", Content: "hi"},
	}
	resp, err := client.Response(context.Background(), msg, "gpt-5")
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
			Model:  "gpt-5",
		},
		AzureOpenAI: config.AzureOpenAIConfig{},
	}

	client := NewAiClient(cfg)

	// Test OpenAI API type (highest priority)
	apiType := client.determineAPIType("gpt-5")
	if apiType != "responses" {
		t.Errorf("expected 'responses', got %s", apiType)
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
