package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/spf13/viper"
)

// Config holds the application configuration
type Config struct {
	Debug                 bool              `mapstructure:"debug"`
	MaxCaptureLines       int               `mapstructure:"max_capture_lines"`
	MaxContextSize        int               `mapstructure:"max_context_size"`
	WaitInterval          int               `mapstructure:"wait_interval"`
	SendKeysConfirm       bool              `mapstructure:"send_keys_confirm"`
	PasteMultilineConfirm bool              `mapstructure:"paste_multiline_confirm"`
	ExecConfirm           bool              `mapstructure:"exec_confirm"`
	WhitelistPatterns     []string          `mapstructure:"whitelist_patterns"`
	BlacklistPatterns     []string          `mapstructure:"blacklist_patterns"`
	OpenRouter            OpenRouterConfig  `mapstructure:"openrouter"`
	AzureOpenAI           AzureOpenAIConfig `mapstructure:"azure_openai"`
	Prompts               PromptsConfig     `mapstructure:"prompts"`
}

// OpenRouterConfig holds OpenRouter API configuration
type OpenRouterConfig struct {
	APIKey  string `mapstructure:"api_key"`
	Model   string `mapstructure:"model"`
	BaseURL string `mapstructure:"base_url"`
}

// AzureOpenAIConfig holds Azure OpenAI API configuration
type AzureOpenAIConfig struct {
	APIKey         string `mapstructure:"api_key"`
	APIBase        string `mapstructure:"api_base"`
	APIVersion     string `mapstructure:"api_version"`
	DeploymentName string `mapstructure:"deployment_name"`
}

// PromptsConfig holds customizable prompt templates
type PromptsConfig struct {
	BaseSystem            string `mapstructure:"base_system"`
	ChatAssistant         string `mapstructure:"chat_assistant"`
	ChatAssistantPrepared string `mapstructure:"chat_assistant_prepared"`
	Watch                 string `mapstructure:"watch"`
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *Config {
	return &Config{
		Debug:                 false,
		MaxCaptureLines:       200,
		MaxContextSize:        20000,
		WaitInterval:          5,
		SendKeysConfirm:       true,
		PasteMultilineConfirm: true,
		ExecConfirm:           true,
		WhitelistPatterns:     []string{},
		BlacklistPatterns:     []string{},
		OpenRouter: OpenRouterConfig{
			BaseURL: "https://openrouter.ai/api/v1",
			Model:   "google/gemini-2.5-flash-preview",
		},
		AzureOpenAI: AzureOpenAIConfig{},
		Prompts: PromptsConfig{
			BaseSystem:    ``,
			ChatAssistant: ``,
		},
	}
}

// Load loads the configuration from file or environment variables
func Load() (*Config, error) {
	config := DefaultConfig()

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	viper.AddConfigPath(".")

	configDir, err := GetConfigDir()
	if err == nil {
		viper.AddConfigPath(configDir)
	} else {
		viper.AddConfigPath(filepath.Join(homeDir, ".config", "tmuxai"))
	}

	// Environment variables
	viper.SetEnvPrefix("TMUXAI")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Automatically bind all config keys to environment variables
	configType := reflect.TypeOf(*config)
	for _, key := range EnumerateConfigKeys(configType, "") {
		viper.BindEnv(key)
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	ResolveEnvKeyInConfig(config)

	return config, nil
}

// EnumerateConfigKeys returns all config keys (dot notation) for the given struct type.
func EnumerateConfigKeys(cfgType reflect.Type, prefix string) []string {
	var keys []string
	for i := 0; i < cfgType.NumField(); i++ {
		field := cfgType.Field(i)
		tag := field.Tag.Get("mapstructure")
		if tag == "" {
			tag = strings.ToLower(field.Name)
		}
		key := tag
		if prefix != "" {
			key = prefix + "." + tag
		}
		if field.Type.Kind() == reflect.Struct {
			keys = append(keys, EnumerateConfigKeys(field.Type, key)...)
		} else {
			keys = append(keys, key)
		}
	}
	return keys
}

// GetConfigDir returns the path to the tmuxai config directory (~/.config/tmuxai)
func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", "tmuxai")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}

	return configDir, nil
}

func GetConfigFilePath(filename string) string {
	configDir, _ := GetConfigDir()
	return filepath.Join(configDir, filename)
}

func TryInferType(key, value string) any {
	var typedValue any = value
	// Only basic type inference for bool/int/string
	for i := 0; i < reflect.TypeOf(Config{}).NumField(); i++ {
		field := reflect.TypeOf(Config{}).Field(i)
		tag := field.Tag.Get("mapstructure")
		if tag == "" {
			tag = strings.ToLower(field.Name)
		}
		// Support dot notation for nested fields
		fullKey := tag
		if key == fullKey {
			switch field.Type.Kind() {
			case reflect.Bool:
				if value == "true" {
					typedValue = true
				} else if value == "false" {
					typedValue = false
				}
			case reflect.Int, reflect.Int64, reflect.Int32:
				var intVal int
				_, err := fmt.Sscanf(value, "%d", &intVal)
				if err == nil {
					typedValue = intVal
				}
			}
		}
		// Nested struct support
		if field.Type.Kind() == reflect.Struct {
			nestedType := field.Type
			prefix := tag + "."
			if strings.HasPrefix(key, prefix) {
				nestedKey := key[len(prefix):]
				for j := 0; j < nestedType.NumField(); j++ {
					nf := nestedType.Field(j)
					ntag := nf.Tag.Get("mapstructure")
					if ntag == "" {
						ntag = strings.ToLower(nf.Name)
					}
					if ntag == nestedKey {
						switch nf.Type.Kind() {
						case reflect.Bool:
							if value == "true" {
								typedValue = true
							} else if value == "false" {
								typedValue = false
							}
						case reflect.Int, reflect.Int64, reflect.Int32:
							var intVal int
							_, err := fmt.Sscanf(value, "%d", &intVal)
							if err == nil {
								typedValue = intVal
							}
						}
					}
				}
			}
		}
	}
	return typedValue
}

// ResolveEnvKeyInConfig recursively expands environment variables in all string fields of the config struct.
func ResolveEnvKeyInConfig(cfg *Config) {
	val := reflect.ValueOf(cfg).Elem()
	resolveEnvKeyReferenceInValue(val)
}

func resolveEnvKeyReferenceInValue(val reflect.Value) {
	switch val.Kind() {
	case reflect.String:
		val.SetString(os.ExpandEnv(val.String()))
	case reflect.Struct:
		for i := 0; i < val.NumField(); i++ {
			resolveEnvKeyReferenceInValue(val.Field(i))
		}
	case reflect.Ptr:
		if !val.IsNil() {
			resolveEnvKeyReferenceInValue(val.Elem())
		}
	}
}
