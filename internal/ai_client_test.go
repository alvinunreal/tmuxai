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

func TestAzureWithResponsesOnlyModel(t *testing.T) {
	// Test that Azure always uses /chat/completions even for models that would normally use /responses
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Azure should still use chat/completions endpoint, not /responses
		if r.URL.Path != "/openai/deployments/o1-deployment/chat/completions" {
			t.Errorf("Azure should use chat/completions endpoint, got path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("api-version") != "2025-04-01-preview" {
			t.Errorf("missing api-version query")
		}
		if r.Header.Get("api-key") != "test-key" {
			t.Errorf("missing api-key header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"azure response"}}]}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{},
		AzureOpenAI: config.AzureOpenAIConfig{
			APIKey:         "test-key",
			APIBase:        server.URL,
			APIVersion:     "2025-04-01-preview",
			DeploymentName: "o1-deployment",
		},
	}

	client := NewAiClient(cfg)
	msg := []Message{{Role: "user", Content: "test"}}
	// Even though "o1" would normally trigger /responses endpoint, Azure should use chat/completions
	resp, err := client.ChatCompletion(context.Background(), msg, "o1")
	if err != nil {
		t.Fatalf("ChatCompletion error: %v", err)
	}
	if resp != "azure response" {
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
		// Should match - exact matches
		{"gpt-5-codex", true},
		{"GPT-5-CODEX", true},
		{"o1", true},
		{"O1", true},
		{"o1-mini", true},
		{"o1-preview", true},

		// Should match - prefix matches for o1 family with delimiters
		{"o1-2024-12-17", true},
		{"o1_custom", true},

		// Should NOT match - false positives prevented
		{"gpt-4-o1-turbo", false},     // "o1" in middle of name
		{"custom-o1-fine-tune", false}, // "o1" in middle of name
		{"solo1", false},               // "o1" at end without delimiter

		// Should NOT match - regular models
		{"gpt-4", false},
		{"gpt-3.5-turbo", false},
		{"claude-3", false},

		// Edge cases
		{"", false}, // empty string
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

func TestErrorHandling(t *testing.T) {
	t.Run("empty response with no choices", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[]}`))
		}))
		defer server.Close()

		cfg := &config.Config{
			OpenRouter: config.OpenRouterConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			},
		}

		client := NewAiClient(cfg)
		msg := []Message{{Role: "user", Content: "test"}}
		_, err := client.ChatCompletion(context.Background(), msg, "gpt-4")
		if err == nil {
			t.Error("expected error for empty choices, got nil")
		}
		if err != nil && !contains(err.Error(), "no completion choices returned") {
			t.Errorf("expected 'no completion choices returned' error, got: %v", err)
		}
	})

	t.Run("malformed JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{invalid json`))
		}))
		defer server.Close()

		cfg := &config.Config{
			OpenRouter: config.OpenRouterConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			},
		}

		client := NewAiClient(cfg)
		msg := []Message{{Role: "user", Content: "test"}}
		_, err := client.ChatCompletion(context.Background(), msg, "gpt-4")
		if err == nil {
			t.Error("expected error for malformed JSON, got nil")
		}
		if err != nil && !contains(err.Error(), "failed to unmarshal") {
			t.Errorf("expected 'failed to unmarshal' error, got: %v", err)
		}
	})

	t.Run("API error status code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": "invalid request"}`))
		}))
		defer server.Close()

		cfg := &config.Config{
			OpenRouter: config.OpenRouterConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			},
		}

		client := NewAiClient(cfg)
		msg := []Message{{Role: "user", Content: "test"}}
		_, err := client.ChatCompletion(context.Background(), msg, "gpt-4")
		if err == nil {
			t.Error("expected error for API error status, got nil")
		}
		if err != nil && !contains(err.Error(), "API returned error") {
			t.Errorf("expected 'API returned error', got: %v", err)
		}
	})

	t.Run("empty response for responses API", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[]}`))
		}))
		defer server.Close()

		cfg := &config.Config{
			OpenRouter: config.OpenRouterConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			},
		}

		client := NewAiClient(cfg)
		msg := []Message{{Role: "user", Content: "test"}}
		_, err := client.ChatCompletion(context.Background(), msg, "o1")
		if err == nil {
			t.Error("expected error for empty choices, got nil")
		}
		if err != nil && !contains(err.Error(), "no completion choices returned") {
			t.Errorf("expected 'no completion choices returned' error, got: %v", err)
		}
	})

	t.Run("malformed JSON response from responses API", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify we're hitting the responses endpoint
			if r.URL.Path != "/responses" {
				t.Errorf("expected /responses endpoint, got: %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{invalid json for responses api`))
		}))
		defer server.Close()

		cfg := &config.Config{
			OpenRouter: config.OpenRouterConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			},
		}

		client := NewAiClient(cfg)
		msg := []Message{{Role: "user", Content: "test"}}
		// Use "o1" to trigger responses API endpoint
		_, err := client.ChatCompletion(context.Background(), msg, "o1")
		if err == nil {
			t.Error("expected error for malformed JSON from responses API, got nil")
		}
		if err != nil && !contains(err.Error(), "failed to unmarshal") {
			t.Errorf("expected 'failed to unmarshal' error, got: %v", err)
		}
	})
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
