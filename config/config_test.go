package config

import (
	"os"
	"testing"
)

func TestResolveEnvKeyInConfig_MapValues(t *testing.T) {
	_ = os.Setenv("TEST_API_KEY", "secret123")
	_ = os.Setenv("TEST_MODEL", "test-model")
	defer func() {
		_ = os.Unsetenv("TEST_API_KEY")
		_ = os.Unsetenv("TEST_MODEL")
	}()

	cfg := DefaultConfig()
	cfg.Models = map[string]ModelConfig{
		"mymodel": {
			APIKey:  "${TEST_API_KEY}",
			Model:   "${TEST_MODEL}",
			BaseURL: "https://example.com",
		},
	}

	ResolveEnvKeyInConfig(cfg)

	m := cfg.Models["mymodel"]
	if m.APIKey != "secret123" {
		t.Errorf("expected APIKey 'secret123', got %q", m.APIKey)
	}
	if m.Model != "test-model" {
		t.Errorf("expected Model 'test-model', got %q", m.Model)
	}
	if m.BaseURL != "https://example.com" {
		t.Errorf("expected BaseURL unchanged, got %q", m.BaseURL)
	}
}

func TestResolveEnvKeyInConfig_NestedStruct(t *testing.T) {
	_ = os.Setenv("TEST_OPENROUTER_KEY", "or-key-456")
	defer func() { _ = os.Unsetenv("TEST_OPENROUTER_KEY") }()

	cfg := DefaultConfig()
	cfg.OpenRouter.APIKey = "${TEST_OPENROUTER_KEY}"

	ResolveEnvKeyInConfig(cfg)

	if cfg.OpenRouter.APIKey != "or-key-456" {
		t.Errorf("expected OpenRouter.APIKey 'or-key-456', got %q", cfg.OpenRouter.APIKey)
	}
}

func TestResolveEnvKeyInConfig_UndefinedEnvVar(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Models = map[string]ModelConfig{
		"mymodel": {
			APIKey: "${THIS_VAR_DEFINITELY_DOES_NOT_EXIST}",
		},
	}

	ResolveEnvKeyInConfig(cfg)

	if cfg.Models["mymodel"].APIKey != "" {
		t.Errorf("expected empty string for undefined env var, got %q", cfg.Models["mymodel"].APIKey)
	}
}
