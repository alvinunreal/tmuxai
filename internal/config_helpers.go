package internal

import (
	"fmt"
	"reflect"
	"strings"
)

// AllowedConfigKeys defines the list of configuration keys that users are allowed to modify
var AllowedConfigKeys = []string{
	"max_capture_lines",
	"max_context_size",
	"wait_interval",
	"send_keys_confirm",
	"paste_multiline_confirm",
	"exec_confirm",
	"openrouter.model",
	"openai.api_key",
	"openai.model",
	"openai.base_url",
	"azure_openai.api_key",
	"azure_openai.deployment_name",
	"azure_openai.api_base",
	"azure_openai.api_version",
	"current_model",
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
	if override, exists := m.SessionOverrides["send_keys_confirm"]; exists {
		if val, ok := override.(bool); ok {
			return val
		}
	}
	return m.Config.SendKeysConfirm
}

func (m *Manager) GetPasteMultilineConfirm() bool {
	if override, exists := m.SessionOverrides["paste_multiline_confirm"]; exists {
		if val, ok := override.(bool); ok {
			return val
		}
	}
	return m.Config.PasteMultilineConfirm
}

func (m *Manager) GetExecConfirm() bool {
	if override, exists := m.SessionOverrides["exec_confirm"]; exists {
		if val, ok := override.(bool); ok {
			return val
		}
	}
	return m.Config.ExecConfirm
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

// GetCurrentModelName returns the name of the currently active model
func (m *Manager) GetCurrentModelName() string {
	// Check for session override first
	if override, exists := m.SessionOverrides["current_model"]; exists {
		if val, ok := override.(string); ok {
			return val
		}
	}

	// Check for default_model in config
	if m.Config.DefaultModel != "" {
		return m.Config.DefaultModel
	}

	return ""
}

// SetCurrentModel sets the current model and syncs with AiClient
func (m *Manager) SetCurrentModel(modelName string) error {
	// Validate that the model exists
	if modelName != "" {
		if _, ok := m.Config.GetModel(modelName); !ok {
			return fmt.Errorf("model '%s' not found in configuration", modelName)
		}
	}

	// Set in session overrides
	m.SessionOverrides["current_model"] = modelName

	// Sync with AiClient
	m.AiClient.SetCurrentModel(modelName)

	return nil
}

// GetModel returns the appropriate model based on configuration priority
// Priority: Named Model Profile > OpenAI > Azure > OpenRouter
func (m *Manager) GetModel() string {
	// Check if a named model profile is active
	currentModelName := m.GetCurrentModelName()
	if currentModelName != "" {
		if modelConfig, ok := m.Config.GetModel(currentModelName); ok {
			return modelConfig.Model
		}
	}

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
