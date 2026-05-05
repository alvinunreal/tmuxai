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
	Debug                 bool                   `mapstructure:"debug"`
	Yolo                  bool                   `mapstructure:"yolo"`
	MaxCaptureLines       int                    `mapstructure:"max_capture_lines"`
	MaxContextSize        int                    `mapstructure:"max_context_size"`
	WaitInterval          int                    `mapstructure:"wait_interval"`
	SendKeysConfirm       bool                   `mapstructure:"send_keys_confirm"`
	PasteMultilineConfirm bool                   `mapstructure:"paste_multiline_confirm"`
	ExecConfirm           bool                   `mapstructure:"exec_confirm"`
	WhitelistPatterns     []string               `mapstructure:"whitelist_patterns"`
	BlacklistPatterns     []string               `mapstructure:"blacklist_patterns"`
	Tmux                  TmuxConfig             `mapstructure:"tmux"`
	OpenRouter            OpenRouterConfig       `mapstructure:"openrouter"`
	OpenAI                OpenAIConfig           `mapstructure:"openai"`
	AzureOpenAI           AzureOpenAIConfig      `mapstructure:"azure_openai"`
	DefaultModel          string                 `mapstructure:"default_model"`
	Models                map[string]ModelConfig `mapstructure:"models"`
	Prompts               PromptsConfig          `mapstructure:"prompts"`
	KnowledgeBase         KnowledgeBaseConfig    `mapstructure:"knowledge_base"`
	WebSearch             WebSearchConfig        `mapstructure:"web_search"`
	WebFetch              WebFetchConfig         `mapstructure:"web_fetch"`
}

// OpenRouterConfig holds OpenRouter API configuration
type OpenRouterConfig struct {
	APIKey  string `mapstructure:"api_key"`
	Model   string `mapstructure:"model"`
	BaseURL string `mapstructure:"base_url"`
}

// OpenAIConfig holds OpenAI API configuration
type OpenAIConfig struct {
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

// ModelConfig holds a single model configuration
type ModelConfig struct {
	Provider string `mapstructure:"provider"`
	Model    string `mapstructure:"model"`
	APIKey   string `mapstructure:"api_key"`
	BaseURL  string `mapstructure:"base_url"`

	// Azure-specific fields
	APIBase        string `mapstructure:"api_base"`
	APIVersion     string `mapstructure:"api_version"`
	DeploymentName string `mapstructure:"deployment_name"`

	// AWS Bedrock-specific fields
	// Region is the AWS region (e.g. "us-east-1"). If empty, falls back to
	// AWS_REGION / AWS_DEFAULT_REGION from the environment.
	// AWSProfile optionally selects a named profile from ~/.aws/credentials.
	// Credentials are otherwise resolved via the default AWS credential chain
	// (environment variables, shared config, IAM roles, SSO, etc.).
	Region     string `mapstructure:"region"`
	AWSProfile string `mapstructure:"aws_profile"`

	// Inference parameters (used by Bedrock today; other providers may adopt
	// them later). Zero values mean "unset"; the provider layer supplies a
	// safe default where one is required.
	MaxTokens   int32   `mapstructure:"max_tokens"`
	Temperature float32 `mapstructure:"temperature"`
}

// PromptsConfig holds customizable prompt templates
type PromptsConfig struct {
	BaseSystem            string `mapstructure:"base_system"`
	ChatAssistant         string `mapstructure:"chat_assistant"`
	ChatAssistantPrepared string `mapstructure:"chat_assistant_prepared"`
	Watch                 string `mapstructure:"watch"`
}

// SkillsConfig holds skill system configuration
type SkillsConfig struct {
	Enabled            bool    `mapstructure:"enabled"`
	AutoScan           bool    `mapstructure:"auto_scan"`
	AutoMatch          bool    `mapstructure:"auto_match"`
	AutoMatchThreshold float64 `mapstructure:"auto_match_threshold"`
	MaxL1Chars         int     `mapstructure:"max_l1_chars"`
	MaxLoadedChars     int     `mapstructure:"max_loaded_chars"`
	MaxSkillChars      int     `mapstructure:"max_skill_chars"`
	TruncateDescAt     int     `mapstructure:"truncate_desc_at"`
}

// KnowledgeBaseConfig holds knowledge base configuration
type KnowledgeBaseConfig struct {
	AutoLoad []string     `mapstructure:"auto_load"`
	Path     string       `mapstructure:"path"`
	Skills   SkillsConfig `mapstructure:"skills"`
}

// WebSearchConfig holds web search configuration.
type WebSearchConfig struct {
	Enabled         bool                            `mapstructure:"enabled"`
	DefaultProvider string                          `mapstructure:"default_provider"`
	MaxResults      int                             `mapstructure:"max_results"`
	MaxResultChars  int                             `mapstructure:"max_result_chars"`
	FetchMaxChars   int                             `mapstructure:"fetch_max_chars"`
	TimeoutSeconds  int                             `mapstructure:"timeout_seconds"`
	Providers       map[string]WebSearchProviderCfg `mapstructure:"providers"`
}

// WebSearchProviderCfg holds per-provider configuration.
type WebSearchProviderCfg struct {
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
}

// WebFetchConfig holds web fetch configuration.
type WebFetchConfig struct {
	Enabled          bool `mapstructure:"enabled"`
	MaxChars         int  `mapstructure:"max_chars"`
	TimeoutSeconds   int  `mapstructure:"timeout_seconds"`
	AllowedRedirects bool `mapstructure:"allowed_redirects"`
}

// TmuxConfig holds tmux-specific behavior settings.
// ExecSplitArgs are raw args passed to `tmux split-window` before target/format flags.
type TmuxConfig struct {
	ExecSplitArgs []string `mapstructure:"exec_split_args"`
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *Config {
	return &Config{
		Debug:                 false,
		Yolo:                  false,
		MaxCaptureLines:       200,
		MaxContextSize:        100000,
		WaitInterval:          5,
		SendKeysConfirm:       true,
		PasteMultilineConfirm: true,
		ExecConfirm:           true,
		WhitelistPatterns:     []string{},
		BlacklistPatterns:     []string{},
		Tmux: TmuxConfig{
			ExecSplitArgs: []string{"-d", "-h"},
		},
		OpenRouter: OpenRouterConfig{
			BaseURL: "https://openrouter.ai/api/v1",
			Model:   "google/gemini-2.5-flash-preview",
		},
		OpenAI: OpenAIConfig{
			BaseURL: "https://api.openai.com/v1",
		},
		AzureOpenAI:  AzureOpenAIConfig{},
		DefaultModel: "",
		Models:       make(map[string]ModelConfig),
		Prompts: PromptsConfig{
			BaseSystem:    ``,
			ChatAssistant: ``,
		},
		KnowledgeBase: KnowledgeBaseConfig{
			AutoLoad: []string{},
			Path:     "",
			Skills: SkillsConfig{
				Enabled:            false,
				AutoScan:           true,
				AutoMatch:          false,
				AutoMatchThreshold: 0.1,
				MaxL1Chars:         8000,
				MaxLoadedChars:     32000,
				MaxSkillChars:      20000,
				TruncateDescAt:     200,
			},
		},
		WebSearch: WebSearchConfig{
			Enabled:         false,
			DefaultProvider: "brave",
			MaxResults:      5,
			MaxResultChars:  6000,
			FetchMaxChars:   15000,
			TimeoutSeconds:  10,
			Providers:       make(map[string]WebSearchProviderCfg),
		},
		WebFetch: WebFetchConfig{
			Enabled:          false,
			MaxChars:         25000,
			TimeoutSeconds:   8,
			AllowedRedirects: true,
		},
	}
}

// Load loads the configuration from file or environment variables.
// An optional configFilePath can be passed to load from a specific file.
func Load(configFilePath ...string) (*Config, error) {
	config := DefaultConfig()

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	// If a custom config file path is provided, use it directly
	if len(configFilePath) > 0 && configFilePath[0] != "" {
		viper.SetConfigFile(configFilePath[0])
	} else if envPath := os.Getenv("TMUXAI_CONFIG"); envPath != "" {
		// Support TMUXAI_CONFIG env var as well
		viper.SetConfigFile(envPath)
	} else {
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
	}

	// Environment variables
	viper.SetEnvPrefix("TMUXAI")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Automatically bind all config keys to environment variables
	configType := reflect.TypeOf(*config)
	for _, key := range EnumerateConfigKeys(configType, "") {
		_ = viper.BindEnv(key)
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

// GetKBDir returns the path to the knowledge base directory
func GetKBDir() string {
	// Try to load config to check for custom path
	cfg, err := Load()
	if err == nil && cfg.KnowledgeBase.Path != "" {
		// Use custom path if specified
		return cfg.KnowledgeBase.Path
	}

	// Default to ~/.config/tmuxai/kb/
	configDir, _ := GetConfigDir()
	kbDir := filepath.Join(configDir, "kb")

	// Create KB directory if it doesn't exist
	_ = os.MkdirAll(kbDir, 0o755)

	return kbDir
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
				switch value {
				case "true":
					typedValue = true
				case "false":
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
							switch value {
							case "true":
								typedValue = true
							case "false":
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
	if isReflectPtr(val.Kind()) {
		if !val.IsNil() {
			resolveEnvKeyReferenceInValue(val.Elem())
		}
		return
	}

	switch val.Kind() {
	case reflect.String:
		val.SetString(os.ExpandEnv(val.String()))
	case reflect.Struct:
		for i := 0; i < val.NumField(); i++ {
			resolveEnvKeyReferenceInValue(val.Field(i))
		}
	case reflect.Map:
		if val.IsNil() {
			return
		}
		for _, key := range val.MapKeys() {
			mapVal := val.MapIndex(key)
			if isReflectPtr(mapVal.Kind()) && mapVal.IsNil() {
				continue
			}
			resolved := resolveEnvValueDeepCopy(mapVal)
			val.SetMapIndex(key, resolved)
		}
	}
}

// resolveEnvValueDeepCopy returns a new value with env vars expanded.
// For non-addressable map values, we need to create a new copy.
func resolveEnvValueDeepCopy(val reflect.Value) reflect.Value {
	if isReflectPtr(val.Kind()) {
		if val.IsNil() {
			return val
		}
		elem := resolveEnvValueDeepCopy(val.Elem())
		ptr := reflect.New(elem.Type())
		ptr.Elem().Set(elem)
		return ptr
	}

	switch val.Kind() {
	case reflect.String:
		return reflect.ValueOf(os.ExpandEnv(val.String()))
	case reflect.Struct:
		cp := reflect.New(val.Type()).Elem()
		cp.Set(val)
		resolveEnvKeyReferenceInValue(cp)
		return cp
	default:
		return val
	}
}

func isReflectPtr(kind reflect.Kind) bool {
	return kind.String() == "ptr"
}
