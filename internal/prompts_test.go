package internal

import (
	"regexp"
	"strings"
	"testing"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/stretchr/testify/assert"
)

func TestGetPromptLegacyWhenEmpty(t *testing.T) {
	cfg := &config.Config{
		DefaultModel: "gpt4",
		Models: map[string]config.ModelConfig{
			"gpt4":   {Provider: "openai", Model: "gpt-4", APIKey: "sk"},
			"claude": {Provider: "openrouter", Model: "claude", APIKey: "sk"},
		},
	}

	t.Run("basic legacy prompt", func(t *testing.T) {
		m := &Manager{Config: cfg, SessionOverrides: map[string]interface{}{}, LoadedKBs: map[string]string{}, Messages: []ChatMessage{}}
		m.SetModelsDefault("gpt4")
		prompt := m.GetPrompt()
		assert.Contains(t, prompt, "TmuxAI")
		assert.Contains(t, prompt, "»")
		assert.NotContains(t, prompt, "[gpt4]")
	})

	t.Run("status symbols", func(t *testing.T) {
		m := &Manager{Config: cfg, SessionOverrides: map[string]interface{}{}, LoadedKBs: map[string]string{}, Messages: []ChatMessage{}}
		m.SetModelsDefault("gpt4")

		m.Status = "running"
		assert.Contains(t, m.GetPrompt(), "▶")

		m.Status = "waiting"
		assert.Contains(t, m.GetPrompt(), "?")

		m.Status = "done"
		assert.Contains(t, m.GetPrompt(), "✓")
	})

	t.Run("watch mode overrides status", func(t *testing.T) {
		m := &Manager{Config: cfg, SessionOverrides: map[string]interface{}{}, LoadedKBs: map[string]string{}, Status: "running", WatchMode: true}
		m.SetModelsDefault("gpt4")
		prompt := m.GetPrompt()
		assert.Contains(t, prompt, "∞")
		assert.NotContains(t, prompt, "▶")
	})

	t.Run("changed model shown when different", func(t *testing.T) {
		m := &Manager{Config: cfg, SessionOverrides: map[string]interface{}{}, LoadedKBs: map[string]string{}, Messages: []ChatMessage{}}
		m.SetModelsDefault("claude") // Different from config default "gpt4"
		prompt := m.GetPrompt()
		assert.Contains(t, prompt, "[claude]")
	})
}

func TestGetPromptCustomTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		status   string
		contains []string
	}{
		{
			name:     "app placeholder",
			template: "{app}> ",
			contains: []string{"TmuxAI", ">"},
		},
		{
			name:     "state badge with status",
			template: "{app} {state_badge}$ ",
			status:   "running",
			contains: []string{"TmuxAI", "[▶]", "$"},
		},
		{
			name:     "model placeholder",
			template: "[{model}] {app}> ",
			contains: []string{"[gpt4]", "TmuxAI", ">"},
		},
		{
			name:     "context placeholder",
			template: "{app} {context}> ",
			contains: []string{"TmuxAI", "/", ">"},
		},
	}

	cfg := &config.Config{
		DefaultModel:   "gpt4",
		Models:         map[string]config.ModelConfig{"gpt4": {Provider: "openai", Model: "gpt-4", APIKey: "sk"}},
		MaxContextSize: 100000,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg.StatusLine = tt.template
			m := &Manager{Config: cfg, SessionOverrides: map[string]interface{}{}, LoadedKBs: map[string]string{}, Messages: []ChatMessage{{Content: "test"}}, Status: tt.status}
			m.SetModelsDefault("gpt4")
			prompt := m.GetPrompt()
			for _, s := range tt.contains {
				assert.Contains(t, prompt, s)
			}
		})
	}
}

func TestGetPromptSessionOverride(t *testing.T) {
	cfg := &config.Config{
		StatusLine:   "{app}> ",
		DefaultModel: "gpt4",
		Models:       map[string]config.ModelConfig{"gpt4": {Provider: "openai", Model: "gpt-4", APIKey: "sk"}},
	}

	t.Run("session override takes precedence", func(t *testing.T) {
		m := &Manager{Config: cfg, SessionOverrides: map[string]interface{}{"status_line": "[{model}] » "}, LoadedKBs: map[string]string{}}
		m.SetModelsDefault("gpt4")
		prompt := m.GetPrompt()
		assert.Contains(t, prompt, "[gpt4]")
		assert.Contains(t, prompt, "»")
		assert.NotContains(t, prompt, "TmuxAI>")
	})

	t.Run("empty session override falls back to legacy", func(t *testing.T) {
		m := &Manager{Config: cfg, SessionOverrides: map[string]interface{}{"status_line": ""}, LoadedKBs: map[string]string{}, Status: "running"}
		m.SetModelsDefault("gpt4")
		prompt := m.GetPrompt()
		assert.Contains(t, prompt, "TmuxAI")
		assert.Contains(t, prompt, "▶")
	})
}

func TestFormatPromptTokenCount(t *testing.T) {
	tests := []struct {
		tokens   int
		expected string
	}{
		{999, "999"},
		{1000, "1k"},
		{1500, "2k"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatPromptTokenCount(tt.tokens)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPromptUnknownPlaceholdersPreserved(t *testing.T) {
	cfg := &config.Config{
		StatusLine:   "{app} {unknown} » ",
		DefaultModel: "gpt4",
		Models:       map[string]config.ModelConfig{"gpt4": {Provider: "openai", Model: "gpt-4", APIKey: "sk"}},
	}
	m := &Manager{Config: cfg, SessionOverrides: map[string]interface{}{}, LoadedKBs: map[string]string{}}
	m.SetModelsDefault("gpt4")
	prompt := m.GetPrompt()
	assert.Contains(t, prompt, "TmuxAI")
	assert.Contains(t, prompt, "{unknown}")
}

func TestPromptEmptyStateHasNoAnsiCodes(t *testing.T) {
	cfg := &config.Config{
		StatusLine:     "{state}{state_badge}",
		DefaultModel:   "gpt4",
		Models:         map[string]config.ModelConfig{"gpt4": {Provider: "openai", Model: "gpt-4", APIKey: "sk"}},
		MaxContextSize: 100000,
	}
	m := &Manager{Config: cfg, SessionOverrides: map[string]interface{}{}, LoadedKBs: map[string]string{}}
	m.SetModelsDefault("gpt4")
	prompt := m.GetPrompt()
	assert.Empty(t, prompt)
	assert.False(t, regexp.MustCompile(`\x1b\[[0-9;]*m`).MatchString(prompt))
}

func TestPromptEmptyModelHasNoAnsiCodes(t *testing.T) {
	cfg := &config.Config{
		StatusLine:     "{model}",
		MaxContextSize: 100000,
	}
	m := &Manager{Config: cfg, SessionOverrides: map[string]interface{}{}, LoadedKBs: map[string]string{}}
	prompt := m.GetPrompt()
	assert.Empty(t, prompt)
	assert.False(t, regexp.MustCompile(`\x1b\[[0-9;]*m`).MatchString(prompt))
}

func TestPromptWithLargeContext(t *testing.T) {
	largeContent := strings.Repeat("Hello world this is a test message with many tokens. ", 500)
	cfg := &config.Config{
		StatusLine:     "{app} {context} » ",
		DefaultModel:   "gpt4",
		Models:         map[string]config.ModelConfig{"gpt4": {Provider: "openai", Model: "gpt-4", APIKey: "sk"}},
		MaxContextSize: 100000,
	}
	m := &Manager{Config: cfg, SessionOverrides: map[string]interface{}{}, LoadedKBs: map[string]string{}, Messages: []ChatMessage{{Content: largeContent}}}
	m.SetModelsDefault("gpt4")
	prompt := m.GetPrompt()
	assert.Contains(t, prompt, "TmuxAI")
	assert.Contains(t, prompt, "/") // Shows used/max format
	assert.Contains(t, prompt, "»")
}
