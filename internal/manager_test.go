package internal

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alvinunreal/tmuxai/config"
)

func TestExecuteOnce(t *testing.T) {
	// Create a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"This is a test response from AI","id":"test-id","object":"response","created_at":1234567890}`))
	}))
	defer server.Close()

	// Create a test configuration
	cfg := &config.Config{
		OpenAI: config.OpenAIConfig{
			APIKey:  "test-key",
			Model:   "gpt-5",
			BaseURL: server.URL,
		},
		OpenRouter:  config.OpenRouterConfig{},
		AzureOpenAI: config.AzureOpenAIConfig{},
		Models:      map[string]config.ModelConfig{},
	}

	// Create a manager
	aiClient := NewAiClient(cfg)
	manager := &Manager{
		Config:           cfg,
		AiClient:         aiClient,
		OS:               "test-os",
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
	}
	aiClient.SetConfigManager(manager)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Execute once
	err := manager.ExecuteOnce("test message")
	if err != nil {
		t.Fatalf("ExecuteOnce failed: %v", err)
	}

	// Restore stdout and read captured output
	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify output contains the AI response
	if !strings.Contains(output, "This is a test response from AI") {
		t.Errorf("Expected output to contain AI response, got: %s", output)
	}
}

func TestExecuteOnceWithKnowledgeBase(t *testing.T) {
	// Create a mock HTTP server that verifies KB is in the request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the request body
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)

		// Verify that the KB content is in the request
		if !strings.Contains(bodyStr, "Knowledge Base: test-kb") {
			t.Errorf("Expected request to contain knowledge base, got: %s", bodyStr)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"Response with KB","id":"test-id","object":"response","created_at":1234567890}`))
	}))
	defer server.Close()

	// Create a test configuration
	cfg := &config.Config{
		OpenAI: config.OpenAIConfig{
			APIKey:  "test-key",
			Model:   "gpt-5",
			BaseURL: server.URL,
		},
		OpenRouter:  config.OpenRouterConfig{},
		AzureOpenAI: config.AzureOpenAIConfig{},
		Models:      map[string]config.ModelConfig{},
	}

	// Create a manager with knowledge base
	aiClient := NewAiClient(cfg)
	manager := &Manager{
		Config:           cfg,
		AiClient:         aiClient,
		OS:               "test-os",
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs: map[string]string{
			"test-kb": "This is test knowledge base content",
		},
	}
	aiClient.SetConfigManager(manager)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Execute once
	err := manager.ExecuteOnce("test message with KB")
	if err != nil {
		t.Fatalf("ExecuteOnce with KB failed: %v", err)
	}

	// Restore stdout and read captured output
	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify output contains the AI response
	if !strings.Contains(output, "Response with KB") {
		t.Errorf("Expected output to contain AI response, got: %s", output)
	}
}

func TestExecuteOnceNoConfiguration(t *testing.T) {
	// Create a configuration without any API keys
	cfg := &config.Config{
		OpenAI:      config.OpenAIConfig{},
		OpenRouter:  config.OpenRouterConfig{},
		AzureOpenAI: config.AzureOpenAIConfig{},
		Models:      map[string]config.ModelConfig{},
	}

	// Create a manager
	aiClient := NewAiClient(cfg)
	manager := &Manager{
		Config:           cfg,
		AiClient:         aiClient,
		OS:               "test-os",
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
	}
	aiClient.SetConfigManager(manager)

	// Execute once should fail
	err := manager.ExecuteOnce("test message")
	if err == nil {
		t.Fatal("Expected ExecuteOnce to fail with no configuration")
	}

	// Verify error message
	if !strings.Contains(err.Error(), "no AI configuration found") {
		t.Errorf("Expected error about missing configuration, got: %v", err)
	}
}

func TestExecuteOnceWithModelConfig(t *testing.T) {
	// Create a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Response from custom model"}}]}`))
	}))
	defer server.Close()

	// Create a test configuration with new model config system
	cfg := &config.Config{
		OpenAI:      config.OpenAIConfig{},
		OpenRouter:  config.OpenRouterConfig{},
		AzureOpenAI: config.AzureOpenAIConfig{},
		DefaultModel: "custom-model",
		Models: map[string]config.ModelConfig{
			"custom-model": {
				Provider: "openrouter",
				Model:    "test/model",
				APIKey:   "test-key",
				BaseURL:  server.URL,
			},
		},
	}

	// Create a manager
	aiClient := NewAiClient(cfg)
	manager := &Manager{
		Config:           cfg,
		AiClient:         aiClient,
		OS:               "test-os",
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
	}
	aiClient.SetConfigManager(manager)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Execute once
	err := manager.ExecuteOnce("test with custom model")
	if err != nil {
		t.Fatalf("ExecuteOnce with custom model failed: %v", err)
	}

	// Restore stdout and read captured output
	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify output contains the AI response
	if !strings.Contains(output, "Response from custom model") {
		t.Errorf("Expected output to contain AI response, got: %s", output)
	}
}

func TestExecuteOnceAPIError(t *testing.T) {
	// Create a mock HTTP server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"Internal server error"}`))
	}))
	defer server.Close()

	// Create a test configuration
	cfg := &config.Config{
		OpenAI: config.OpenAIConfig{
			APIKey:  "test-key",
			Model:   "gpt-5",
			BaseURL: server.URL,
		},
		OpenRouter:  config.OpenRouterConfig{},
		AzureOpenAI: config.AzureOpenAIConfig{},
		Models:      map[string]config.ModelConfig{},
	}

	// Create a manager
	aiClient := NewAiClient(cfg)
	manager := &Manager{
		Config:           cfg,
		AiClient:         aiClient,
		OS:               "test-os",
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
	}
	aiClient.SetConfigManager(manager)

	// Execute once should fail
	err := manager.ExecuteOnce("test message")
	if err == nil {
		t.Fatal("Expected ExecuteOnce to fail with API error")
	}

	// Verify error message
	if !strings.Contains(err.Error(), "failed to get AI response") {
		t.Errorf("Expected error about API failure, got: %v", err)
	}
}

func TestExecuteOnceWithContext(t *testing.T) {
	// Create a mock HTTP server
	requestReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true

		// Read the request body to verify context
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)

		// Verify system prompt includes OS information
		if !strings.Contains(bodyStr, "test-os") {
			t.Errorf("Expected request to contain OS info, got: %s", bodyStr)
		}

		// Verify user message is included
		if !strings.Contains(bodyStr, "test user message") {
			t.Errorf("Expected request to contain user message, got: %s", bodyStr)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"Context verified","id":"test-id","object":"response","created_at":1234567890}`))
	}))
	defer server.Close()

	// Create a test configuration
	cfg := &config.Config{
		OpenAI: config.OpenAIConfig{
			APIKey:  "test-key",
			Model:   "gpt-5",
			BaseURL: server.URL,
		},
		OpenRouter:  config.OpenRouterConfig{},
		AzureOpenAI: config.AzureOpenAIConfig{},
		Models:      map[string]config.ModelConfig{},
	}

	// Create a manager
	aiClient := NewAiClient(cfg)
	manager := &Manager{
		Config:           cfg,
		AiClient:         aiClient,
		OS:               "test-os",
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
	}
	aiClient.SetConfigManager(manager)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Execute once
	err := manager.ExecuteOnce("test user message")
	if err != nil {
		t.Fatalf("ExecuteOnce failed: %v", err)
	}

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout
	io.Copy(io.Discard, r)

	// Verify request was received
	if !requestReceived {
		t.Error("Expected HTTP request to be sent")
	}
}

func TestExecuteOnceCancellation(t *testing.T) {
	// Create a mock HTTP server that takes time to respond
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if context is cancelled
		ctx := r.Context()
		select {
		case <-ctx.Done():
			return
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"output_text":"Response","id":"test-id","object":"response","created_at":1234567890}`))
		}
	}))
	defer server.Close()

	// Create a test configuration
	cfg := &config.Config{
		OpenAI: config.OpenAIConfig{
			APIKey:  "test-key",
			Model:   "gpt-5",
			BaseURL: server.URL,
		},
		OpenRouter:  config.OpenRouterConfig{},
		AzureOpenAI: config.AzureOpenAIConfig{},
		Models:      map[string]config.ModelConfig{},
	}

	// Create a manager
	aiClient := NewAiClient(cfg)
	manager := &Manager{
		Config:           cfg,
		AiClient:         aiClient,
		OS:               "test-os",
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
	}
	aiClient.SetConfigManager(manager)

	// Note: ExecuteOnce creates its own context, so we can't test cancellation directly
	// This test verifies that ExecuteOnce completes successfully with a context
	err := manager.ExecuteOnce("test message")
	if err != nil {
		t.Fatalf("ExecuteOnce failed: %v", err)
	}
}

func TestExecuteOnceIntegration(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// This test would require a real API key and real API calls
	// For now, we'll skip it in normal test runs
	t.Skip("Integration test requires real API credentials")
}

func TestNewManagerWithExecuteOnce(t *testing.T) {
	// This test verifies that a Manager created with NewManager can use ExecuteOnce
	// Note: NewManager requires tmux interactions which are hard to mock
	// So we'll just test that our ExecuteOnce method signature is compatible
	// with the Manager struct

	cfg := &config.Config{
		OpenAI: config.OpenAIConfig{
			APIKey: "test-key",
			Model:  "gpt-5",
		},
		OpenRouter:  config.OpenRouterConfig{},
		AzureOpenAI: config.AzureOpenAIConfig{},
		Models:      map[string]config.ModelConfig{},
	}

	aiClient := NewAiClient(cfg)
	manager := &Manager{
		Config:           cfg,
		AiClient:         aiClient,
		OS:               "test-os",
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
	}
	aiClient.SetConfigManager(manager)

	// Verify that ExecuteOnce method exists and has correct signature
	var _ func(string) error = manager.ExecuteOnce
}
