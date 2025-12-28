package internal

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/alvinunreal/tmuxai/config"
)

// AllowedConfigKeys defines the list of configuration keys that users are allowed to modify
var AllowedConfigKeys = []string{
	"max_capture_lines",
	"max_context_size",
	"wait_interval",
	"send_keys_confirm",
	"paste_multiline_confirm",
	"exec_confirm",
	"yolo",
	"openrouter.model",
	"openai.api_key",
	"openai.model",
	"openai.base_url",
	"azure_openai.api_key",
	"azure_openai.deployment_name",
	"azure_openai.api_base",
	"azure_openai.api_version",
	"default_model",
}

// GetMaxCaptureLines returns the max capture lines value with session override if present
func (m *Manager) GetMaxCaptureLines() int {
	if override, exists := m.SessionOverrides["max_capture_lines"]; exists {
		if val, ok := override.(int); ok {
			return val
		}
	}
	return m.Config.MaxCaptureLines
}

// GetMaxContextSize returns the max context size value with session override if present
func (m *Manager) GetMaxContextSize() int {
	if override, exists := m.SessionOverrides["max_context_size"]; exists {
		if val, ok := override.(int); ok {
			return val
		}
	}
	return m.Config.MaxContextSize
}

// GetWaitInterval returns the wait interval value with session override if present
func (m *Manager) GetWaitInterval() int {
	if override, exists := m.SessionOverrides["wait_interval"]; exists {
		if val, ok := override.(int); ok {
			return val
		}
	}
	return m.Config.WaitInterval
}

func (m *Manager) GetSendKeysConfirm() bool {
	if m.GetYolo() {
		return false
	}
	if override, exists := m.SessionOverrides["send_keys_confirm"]; exists {
		if val, ok := override.(bool); ok {
			return val
		}
	}
	return m.Config.SendKeysConfirm
}

func (m *Manager) GetPasteMultilineConfirm() bool {
	if m.GetYolo() {
		return false
	}
	if override, exists := m.SessionOverrides["paste_multiline_confirm"]; exists {
		if val, ok := override.(bool); ok {
			return val
		}
	}
	return m.Config.PasteMultilineConfirm
}

func (m *Manager) GetExecConfirm() bool {
	if m.GetYolo() {
		return false
	}
	if override, exists := m.SessionOverrides["exec_confirm"]; exists {
		if val, ok := override.(bool); ok {
			return val
		}
	}
	return m.Config.ExecConfirm
}

func (m *Manager) GetYolo() bool {
	if override, exists := m.SessionOverrides["yolo"]; exists {
		if val, ok := override.(bool); ok {
			return val
		}
	}
	return m.Config.Yolo
}

func (m *Manager) GetOpenRouterModel() string {
	if override, exists := m.SessionOverrides["openrouter.model"]; exists {
		if val, ok := override.(string); ok {
			return val
		}
	}
	return m.Config.OpenRouter.Model
}

// GetOpenAIModel returns the OpenAI model value with session override if present
func (m *Manager) GetOpenAIModel() string {
	if override, exists := m.SessionOverrides["openai.model"]; exists {
		if val, ok := override.(string); ok {
			return val
		}
	}
	return m.Config.OpenAI.Model
}

// GetOpenAIAPIKey returns the OpenAI API key value with session override if present
func (m *Manager) GetOpenAIAPIKey() string {
	if override, exists := m.SessionOverrides["openai.api_key"]; exists {
		if val, ok := override.(string); ok {
			return val
		}
	}
	return m.Config.OpenAI.APIKey
}

// GetOpenAIBaseURL returns the OpenAI base URL value with session override if present
func (m *Manager) GetOpenAIBaseURL() string {
	if override, exists := m.SessionOverrides["openai.base_url"]; exists {
		if val, ok := override.(string); ok {
			return val
		}
	}
	return m.Config.OpenAI.BaseURL
}

// GetAzureOpenAIAPIKey returns the Azure OpenAI API key value with session override if present
func (m *Manager) GetAzureOpenAIAPIKey() string {
	if override, exists := m.SessionOverrides["azure_openai.api_key"]; exists {
		if val, ok := override.(string); ok {
			return val
		}
	}
	return m.Config.AzureOpenAI.APIKey
}

// GetAzureOpenAIDeploymentName returns the Azure OpenAI deployment name value with session override if present
func (m *Manager) GetAzureOpenAIDeploymentName() string {
	if override, exists := m.SessionOverrides["azure_openai.deployment_name"]; exists {
		if val, ok := override.(string); ok {
			return val
		}
	}
	return m.Config.AzureOpenAI.DeploymentName
}

// GetModelsDefault returns the default model configuration name with session override if present
func (m *Manager) GetModelsDefault() string {
	// Check for session override first
	if override, exists := m.SessionOverrides["default_model"]; exists {
		if val, ok := override.(string); ok {
			return val
		}
	}

	// If default model is configured, use it
	if m.Config.DefaultModel != "" {
		return m.Config.DefaultModel
	}

	// If no default model is set, use the first available model
	availableModels := m.GetAvailableModels()
	if len(availableModels) > 0 {
		return availableModels[0]
	}

	// Return empty string if no models are configured
	return ""
}

// SetModelsDefault sets the default model configuration for the current session
func (m *Manager) SetModelsDefault(modelName string) {
	m.SessionOverrides["default_model"] = modelName
}

// GetAvailableModels returns a list of available model configuration names
func (m *Manager) GetAvailableModels() []string {
	var models []string
	for name := range m.Config.Models {
		models = append(models, name)
	}
	// Sort the models alphabetically for consistent ordering
	for i := 0; i < len(models); i++ {
		for j := i + 1; j < len(models); j++ {
			if models[i] > models[j] {
				models[i], models[j] = models[j], models[i]
			}
		}
	}
	return models
}

// GetModelConfig returns the model configuration for the given name
func (m *Manager) GetModelConfig(name string) (config.ModelConfig, bool) {
	config, exists := m.Config.Models[name]
	return config, exists
}

// GetCurrentModelConfig returns the currently active model configuration
func (m *Manager) GetCurrentModelConfig() (config.ModelConfig, bool) {
	// First try to get the model from the new models system
	defaultModel := m.GetModelsDefault()
	if defaultModel != "" {
		if modelConfig, exists := m.GetModelConfig(defaultModel); exists {
			return modelConfig, true
		}
	}

	// Fall back to legacy configuration by converting to a ModelConfig
	return m.getLegacyModelConfig(), true
}

// hasValidAIConfiguration checks if there's a valid AI configuration available
func (m *Manager) hasValidAIConfiguration() bool {
	// Check new model configurations first
	availableModels := m.GetAvailableModels()
	if len(availableModels) > 0 {
		// Check if any model has an API key
		for _, modelName := range availableModels {
			if modelConfig, exists := m.GetModelConfig(modelName); exists {
				if modelConfig.APIKey != "" {
					return true
				}
			}
		}
		// Also check if current model has API key
		if currentModelConfig, exists := m.GetCurrentModelConfig(); exists {
			if currentModelConfig.APIKey != "" {
				return true
			}
		}
	}

	// Fall back to legacy configuration
	return m.Config.OpenRouter.APIKey != "" || m.Config.OpenAI.APIKey != "" || m.Config.AzureOpenAI.APIKey != ""
}

// getLegacyModelConfig converts the legacy provider configuration to a ModelConfig
func (m *Manager) getLegacyModelConfig() config.ModelConfig {
	// Priority: OpenAI > Azure > OpenRouter
	if m.GetOpenAIAPIKey() != "" {
		return config.ModelConfig{
			Provider: "openai",
			Model:    m.GetOpenAIModel(),
			APIKey:   m.GetOpenAIAPIKey(),
			BaseURL:  m.GetOpenAIBaseURL(),
		}
	}

	if m.GetAzureOpenAIAPIKey() != "" {
		return config.ModelConfig{
			Provider:       "azure",
			Model:          m.GetAzureOpenAIDeploymentName(),
			APIKey:         m.GetAzureOpenAIAPIKey(),
			APIBase:        m.Config.AzureOpenAI.APIBase,
			APIVersion:     m.Config.AzureOpenAI.APIVersion,
			DeploymentName: m.GetAzureOpenAIDeploymentName(),
		}
	}

	return config.ModelConfig{
		Provider: "openrouter",
		Model:    m.GetOpenRouterModel(),
		APIKey:   m.Config.OpenRouter.APIKey,
		BaseURL:  m.Config.OpenRouter.BaseURL,
	}
}

// GetModel returns the appropriate model based on configuration priority
// New logic: Use models.default if available, otherwise use first available model, then fall back to legacy priority system
func (m *Manager) GetModel() string {
	// Get the current model name (using auto-selection logic)
	currentModelName := m.GetModelsDefault()
	if currentModelName == "" {
		// No models configured, fall back to legacy configuration
		// If OpenAI is configured, use OpenAI model
		if m.GetOpenAIAPIKey() != "" {
			model := m.GetOpenAIModel()
			if model != "" {
				return model
			}
			// Default model for OpenAI if not specified
			return "gpt-5-codex"
		}

		// If Azure is configured, use Azure deployment name
		if m.GetAzureOpenAIAPIKey() != "" {
			deployment := m.GetAzureOpenAIDeploymentName()
			if deployment != "" {
				return deployment
			}
			// Default deployment for Azure if not specified
			return "gpt-4o"
		}

		// Default to OpenRouter
		return m.GetOpenRouterModel()
	}

	// Get the actual model name from the configuration
	if modelConfig, exists := m.GetModelConfig(currentModelName); exists {
		return modelConfig.Model
	}

	// Fallback: try to get model from legacy config if current model name doesn't exist
	return m.GetOpenRouterModel()
}

// FormatConfig returns a nicely formatted string of all config values with session overrides applied
func (m *Manager) FormatConfig() string {
	var result strings.Builder
	formatConfigValue(&result, "", reflect.ValueOf(m.Config).Elem(), m.SessionOverrides, 1)
	return result.String()
}

// formatConfigValue recursively formats config values using reflection
func formatConfigValue(sb *strings.Builder, prefix string, val reflect.Value, overrides map[string]interface{}, indent int) {
	typ := val.Type()

	indentStr := ""
	if indent > 1 {
		indentStr = strings.Repeat("  ", indent)
	}

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Get the field name from mapstructure tag or use field name
		tag := fieldType.Tag.Get("mapstructure")
		if tag == "" {
			tag = strings.ToLower(fieldType.Name)
		}

		// Build the key path for checking overrides
		key := tag
		if prefix != "" {
			key = prefix + "." + tag
		}

		// Handle nested structs
		if field.Kind() == reflect.Struct {
			_, _ = fmt.Fprintf(sb, "%s%s:\n", indentStr, tag)
			formatConfigValue(sb, key, field, overrides, indent+1)
			continue
		}

		// Format the field value
		var valueStr string
		switch field.Kind() {
		case reflect.String:
			// Mask API keys for security
			if strings.Contains(strings.ToLower(fieldType.Name), "apikey") {
				valueStr = maskAPIKey(field.String())
			} else {
				valueStr = field.String()
			}
		case reflect.Bool:
			valueStr = fmt.Sprintf("%t", field.Bool())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			valueStr = fmt.Sprintf("%d", field.Int())
		case reflect.Slice, reflect.Array:
			valueStr = fmt.Sprintf("%v", field.Interface())
		default:
			valueStr = fmt.Sprintf("%v", field.Interface())
		}

		// Check if there's a session override for this key
		if override, exists := overrides[key]; exists {
			_, _ = fmt.Fprintf(sb, "%s%s: %v", indentStr, tag, override)
		} else {
			_, _ = fmt.Fprintf(sb, "%s%s: %s", indentStr, tag, valueStr)
		}

		sb.WriteString("\n")
	}
}

// maskAPIKey hides most of the API key for security
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}
