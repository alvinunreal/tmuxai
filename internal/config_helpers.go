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
