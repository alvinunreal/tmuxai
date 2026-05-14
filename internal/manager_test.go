package internal

import (
	"testing"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/stretchr/testify/assert"
)

func TestFormatCompactNumber(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected string
	}{
		{"zero", 0, "0"},
		{"small number", 500, "500"},
		{"exact thousand", 1000, "1k"},
		{"thousands", 37000, "37k"},
		{"large thousands", 999999, "999k"},
		{"million", 1000000, "1.0m"},
		{"millions", 2500000, "2.5m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatCompactNumber(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetCustomPromptTemplate(t *testing.T) {
	tests := []struct {
		name           string
		configPrompt   string
		sessionPrompt  interface{}
		hasOverride    bool
		expectedResult string
	}{
		{
			name:           "no custom prompt configured",
			configPrompt:   "",
			hasOverride:    false,
			expectedResult: "",
		},
		{
			name:           "custom prompt from config",
			configPrompt:   "{app} » ",
			hasOverride:    false,
			expectedResult: "{app} » ",
		},
		{
			name:           "session override takes precedence",
			configPrompt:   "{app} » ",
			sessionPrompt:  "✨ ",
			hasOverride:    true,
			expectedResult: "✨ ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &Manager{
				Config: &config.Config{
					Prompts: config.PromptsConfig{
						Prompt: tt.configPrompt,
					},
				},
				SessionOverrides: make(map[string]interface{}),
				LoadedKBs:        make(map[string]string),
			}

			if tt.hasOverride {
				manager.SessionOverrides["prompts.prompt"] = tt.sessionPrompt
			}

			result := manager.getCustomPromptTemplate()
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestGetPromptDefaultBehavior(t *testing.T) {
	// Test default prompt without custom template
	manager := &Manager{
		Config: &config.Config{
			DefaultModel: "test-model",
			Models: map[string]config.ModelConfig{
				"test-model": {Provider: "openai", Model: "gpt-4"},
			},
		},
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
		Status:           "running",
	}

	prompt := manager.GetPrompt()
	// Should contain TmuxAI
	assert.Contains(t, prompt, "TmuxAI")
	// Should contain the arrow
	assert.Contains(t, prompt, "»")
}

func TestGetPromptWithCustomTemplate(t *testing.T) {
	manager := &Manager{
		Config: &config.Config{
			DefaultModel:   "test-model",
			MaxContextSize: 100000,
			Prompts: config.PromptsConfig{
				Prompt: "{app} ({context}/{max_context}) » ",
			},
		},
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
		Messages:         []ChatMessage{},
		Status:           "",
	}

	prompt := manager.GetPrompt()
	// Should contain TmuxAI (colored)
	assert.Contains(t, prompt, "TmuxAI")
	// Should contain context info (0/100k)
	assert.Contains(t, prompt, "0")
	assert.Contains(t, prompt, "100k")
	// Should contain arrow
	assert.Contains(t, prompt, "»")
}

func TestGetPromptWithAllPlaceholders(t *testing.T) {
	manager := &Manager{
		Config: &config.Config{
			DefaultModel:   "my-model",
			MaxContextSize: 200000,
			Prompts: config.PromptsConfig{
				Prompt: "{app} {model} {state} {context} {max_context} {context_percent}",
			},
		},
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
		Messages:         []ChatMessage{},
		Status:           "running",
		WatchMode:        false,
	}

	prompt := manager.GetPrompt()
	// All placeholders should be replaced (not present in output)
	assert.NotContains(t, prompt, "{app}")
	assert.NotContains(t, prompt, "{model}")
	assert.NotContains(t, prompt, "{state}")
	assert.NotContains(t, prompt, "{context}")
	assert.NotContains(t, prompt, "{max_context}")
	assert.NotContains(t, prompt, "{context_percent}")
	// Should contain the values
	assert.Contains(t, prompt, "TmuxAI")
	assert.Contains(t, prompt, "my-model")
	assert.Contains(t, prompt, "▶") // running state
	assert.Contains(t, prompt, "200k")
}

func TestGetPromptWithUnknownPlaceholders(t *testing.T) {
	// Unknown placeholders should be kept as-is (benign behavior)
	manager := &Manager{
		Config: &config.Config{
			Prompts: config.PromptsConfig{
				Prompt: "{app} {unknown} {foo}",
			},
		},
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
	}

	prompt := manager.GetPrompt()
	// Known placeholder replaced
	assert.Contains(t, prompt, "TmuxAI")
	// Unknown placeholders kept literal
	assert.Contains(t, prompt, "{unknown}")
	assert.Contains(t, prompt, "{foo}")
}

func TestGetPromptWithCompactTemplate(t *testing.T) {
	// Test a compact prompt like "✨ "
	manager := &Manager{
		Config: &config.Config{
			Prompts: config.PromptsConfig{
				Prompt: "✨ ",
			},
		},
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
	}

	prompt := manager.GetPrompt()
	// Custom templates are the full prompt string.
	assert.Equal(t, "✨ ", prompt)
	assert.Contains(t, prompt, "✨")
	assert.NotContains(t, prompt, "»")
}

func TestGetPromptWatchModeState(t *testing.T) {
	manager := &Manager{
		Config: &config.Config{
			Prompts: config.PromptsConfig{
				Prompt: "{app} {state}",
			},
		},
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
		Status:           "running",
		WatchMode:        true,
	}

	prompt := manager.GetPrompt()
	// Watch mode should show ∞ symbol regardless of status
	assert.Contains(t, prompt, "∞")
	assert.NotContains(t, prompt, "▶")
}

func TestGetPromptContextCalculation(t *testing.T) {
	manager := &Manager{
		Config: &config.Config{
			MaxContextSize: 100000,
			Prompts: config.PromptsConfig{
				Prompt: "{context}/{max_context} ({context_percent})",
			},
		},
		SessionOverrides: make(map[string]interface{}),
		LoadedKBs:        make(map[string]string),
		Messages: []ChatMessage{
			{Content: "Hello world this is a test message"},
			{Content: "Another message here"},
		},
	}

	prompt := manager.GetPrompt()
	// Should contain formatted context info
	assert.Contains(t, prompt, "100k")
	// Should contain percentage (should be very small %)
	assert.Contains(t, prompt, "%")
}
