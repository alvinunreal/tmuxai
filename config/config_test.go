package config

import (
	"testing"
)

func TestResolveEnvKeyInConfig_MapValues(t *testing.T) {
	t.Setenv("TEST_API_KEY", "secret123")
	t.Setenv("TEST_MODEL", "test-model")

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
	t.Setenv("TEST_OPENROUTER_KEY", "or-key-456")

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

func TestResolveEnvKeyInConfig_WebSearchProviders(t *testing.T) {
	t.Setenv("TEST_FIRECRAWL_API_KEY", "fc-key-789")

	cfg := DefaultConfig()
	cfg.WebSearch.Providers = map[string]WebSearchProviderCfg{
		"firecrawl": {APIKey: "${TEST_FIRECRAWL_API_KEY}"},
	}

	ResolveEnvKeyInConfig(cfg)

	if got := cfg.WebSearch.Providers["firecrawl"].APIKey; got != "fc-key-789" {
		t.Errorf("expected firecrawl APIKey 'fc-key-789', got %q", got)
	}
	if cfg.WebSearch.DefaultProvider != "brave" {
		t.Errorf("expected default provider to stay 'brave', got %q", cfg.WebSearch.DefaultProvider)
	}
}
