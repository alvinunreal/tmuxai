package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/alvinunreal/tmuxai/logger"
)

var envPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

func DefaultConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".config", "tmuxai", "mcp.json")
}

func LoadConfig(path string) (*MCPConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read MCP config: %w", err)
	}

	var cfg MCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse MCP config: %w", err)
	}

	if cfg.MCPServers == nil {
		cfg.MCPServers = make(map[string]ServerConfig)
	}

	for name, sc := range cfg.MCPServers {
		if sc.Env != nil {
			sc.Env = ExpandEnv(sc.Env)
		}
		if sc.Headers != nil {
			sc.Headers = ExpandEnv(sc.Headers)
		}
		cfg.MCPServers[name] = sc
	}

	if err := Validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// ExpandEnv replaces ${VAR} patterns in env map values with their OS environment values.
func ExpandEnv(env map[string]string) map[string]string {
	expanded := make(map[string]string, len(env))
	for k, v := range env {
		expanded[k] = envPattern.ReplaceAllStringFunc(v, func(match string) string {
			subs := envPattern.FindStringSubmatch(match)
			if len(subs) < 2 {
				return match
			}
			varName := subs[1]
			val, ok := os.LookupEnv(varName)
			if !ok {
				logger.Info("MCP config: environment variable %s not set, using empty string", varName)
				return ""
			}
			return val
		})
	}
	return expanded
}

func Validate(cfg *MCPConfig) error {
	for name, sc := range cfg.MCPServers {
		if sc.Disabled {
			continue
		}
		hasCommand := sc.Command != ""
		hasURL := sc.URL != ""
		if hasCommand && hasURL {
			return fmt.Errorf("MCP server %q: cannot specify both command and url", name)
		}
		if !hasCommand && !hasURL {
			return fmt.Errorf("MCP server %q: must specify either command or url", name)
		}
	}
	return nil
}
