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

func TestResponsesEndpointForGPT5Codex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the correct endpoint is being used
		if r.URL.Path != "/responses" {
			t.Errorf("unexpected path: %s, expected /responses", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing or incorrect Authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"response from gpt-5-codex"}}]}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
			Model:   "gpt-5-codex",
		},
	}

	client := NewAiClient(cfg)
	msg := []Message{{Role: "user", Content: "test"}}
	resp, err := client.ChatCompletion(context.Background(), msg, "gpt-5-codex")
	if err != nil {
		t.Fatalf("ChatCompletion error: %v", err)
	}
	if resp != "response from gpt-5-codex" {
		t.Errorf("unexpected response: %s", resp)
	}
}

func TestChatCompletionsEndpointForRegularModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the correct endpoint is being used
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s, expected /chat/completions", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing or incorrect Authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"response from regular model"}}]}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
			Model:   "gpt-4",
		},
	}

	client := NewAiClient(cfg)
	msg := []Message{{Role: "user", Content: "test"}}
	resp, err := client.ChatCompletion(context.Background(), msg, "gpt-4")
	if err != nil {
		t.Fatalf("ChatCompletion error: %v", err)
	}
	if resp != "response from regular model" {
		t.Errorf("unexpected response: %s", resp)
	}
}

func TestRequiresResponsesAPI(t *testing.T) {
	tests := []struct {
		model    string
		expected bool
	}{
		{"gpt-5-codex", true},
		{"GPT-5-CODEX", true},
		{"o1", true},
		{"o1-mini", true},
		{"o1-preview", true},
		{"gpt-4", false},
		{"gpt-3.5-turbo", false},
		{"claude-3", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := requiresResponsesAPI(tt.model)
			if result != tt.expected {
				t.Errorf("requiresResponsesAPI(%s) = %v, expected %v", tt.model, result, tt.expected)
			}
		})
	}
}
