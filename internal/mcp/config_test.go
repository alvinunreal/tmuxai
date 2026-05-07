package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigMissing(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/path/mcp.json")
	if err != nil {
		t.Errorf("Expected nil error for missing file, got: %v", err)
	}
	if cfg != nil {
		t.Error("Expected nil config for missing file")
	}
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(p, []byte("{bad json}"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(p)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
	if cfg != nil {
		t.Error("Expected nil config for invalid JSON")
	}
}

func TestLoadConfigValid(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mcp.json")
	input := `{
		"mcpServers": {
			"myserver": {
				"command": "/usr/bin/foo",
				"args": ["--flag"],
				"env": {"KEY": "val"},
				"timeout_seconds": 10
			},
			"remotesrv": {
				"url": "http://localhost:8080/mcp",
				"headers": {"Authorization": "Bearer tok"}
			}
		}
	}`
	if err := os.WriteFile(p, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Expected non-nil config")
	}
	if len(cfg.MCPServers) != 2 {
		t.Errorf("Expected 2 servers, got %d", len(cfg.MCPServers))
	}
	srv, ok := cfg.MCPServers["myserver"]
	if !ok {
		t.Fatal("Expected myserver")
	}
	if srv.Command != "/usr/bin/foo" {
		t.Errorf("Expected command /usr/bin/foo, got %s", srv.Command)
	}
	if len(srv.Args) != 1 || srv.Args[0] != "--flag" {
		t.Errorf("Expected args [--flag], got %v", srv.Args)
	}
	if srv.Env["KEY"] != "val" {
		t.Errorf("Expected env KEY=val, got %v", srv.Env)
	}
	if srv.TimeoutSeconds != 10 {
		t.Errorf("Expected timeout 10, got %d", srv.TimeoutSeconds)
	}
	remote, ok := cfg.MCPServers["remotesrv"]
	if !ok {
		t.Fatal("Expected remotesrv")
	}
	if remote.URL != "http://localhost:8080/mcp" {
		t.Errorf("Expected url, got %s", remote.URL)
	}
	if remote.Headers["Authorization"] != "Bearer tok" {
		t.Errorf("Expected header, got %v", remote.Headers)
	}
}

func TestLoadConfigEmptyServers(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(p, []byte(`{"mcpServers": {}}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Expected non-nil config")
	}
	if len(cfg.MCPServers) != 0 {
		t.Errorf("Expected 0 servers, got %d", len(cfg.MCPServers))
	}
}

func TestExpandEnvSet(t *testing.T) {
	t.Setenv("TEST_MCP_KEY", "secret123")
	env := map[string]string{"API_KEY": "${TEST_MCP_KEY}"}
	result := ExpandEnv(env)
	if result["API_KEY"] != "secret123" {
		t.Errorf("Expected 'secret123', got %q", result["API_KEY"])
	}
}

func TestExpandEnvUnset(t *testing.T) {
	// t.Setenv registers cleanup; Unsetenv makes it truly absent for ExpandEnv
	t.Setenv("TEST_MCP_MISSING_VAR", "")
	_ = os.Unsetenv("TEST_MCP_MISSING_VAR")
	env := map[string]string{"API_KEY": "${TEST_MCP_MISSING_VAR}"}
	result := ExpandEnv(env)
	if result["API_KEY"] != "" {
		t.Errorf("Expected empty string for unset var, got %q", result["API_KEY"])
	}
}

func TestExpandEnvMixed(t *testing.T) {
	t.Setenv("TEST_MCP_HOST", "example.com")
	env := map[string]string{"URL": "https://${TEST_MCP_HOST}/path", "PLAIN": "no-vars-here"}
	result := ExpandEnv(env)
	if result["URL"] != "https://example.com/path" {
		t.Errorf("Expected expanded URL, got %q", result["URL"])
	}
	if result["PLAIN"] != "no-vars-here" {
		t.Errorf("Expected plain value, got %q", result["PLAIN"])
	}
}

func TestValidateBothCommandAndURL(t *testing.T) {
	cfg := &MCPConfig{
		MCPServers: map[string]ServerConfig{
			"bad": {Command: "/bin/foo", URL: "http://localhost"},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("Expected error for both command and url")
	}
}

func TestValidateNeitherCommandNorURL(t *testing.T) {
	cfg := &MCPConfig{
		MCPServers: map[string]ServerConfig{
			"bad": {},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("Expected error for neither command nor url")
	}
}

func TestValidateDisabledServerNoCommandURL(t *testing.T) {
	cfg := &MCPConfig{
		MCPServers: map[string]ServerConfig{
			"disabled": {Disabled: true},
		},
	}
	err := Validate(cfg)
	if err != nil {
		t.Errorf("Expected no error for disabled server, got: %v", err)
	}
}

func TestValidateValidCommandOnly(t *testing.T) {
	cfg := &MCPConfig{
		MCPServers: map[string]ServerConfig{
			"srv": {Command: "/bin/foo"},
		},
	}
	err := Validate(cfg)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestValidateValidURLOnly(t *testing.T) {
	cfg := &MCPConfig{
		MCPServers: map[string]ServerConfig{
			"srv": {URL: "http://localhost:8080"},
		},
	}
	err := Validate(cfg)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestLoadConfigWithEnvExpansion(t *testing.T) {
	t.Setenv("TEST_MCP_SECRET", "mytoken")
	dir := t.TempDir()
	p := filepath.Join(dir, "mcp.json")
	input := `{
		"mcpServers": {
			"s1": {
				"command": "/bin/echo",
				"env": {"TOKEN": "${TEST_MCP_SECRET}"}
			}
		}
	}`
	if err := os.WriteFile(p, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if cfg.MCPServers["s1"].Env["TOKEN"] != "mytoken" {
		t.Errorf("Expected expanded env, got %q", cfg.MCPServers["s1"].Env["TOKEN"])
	}
}

func TestLoadConfigValidatesOnLoad(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mcp.json")
	input := `{
		"mcpServers": {
			"bad": {
				"command": "/bin/foo",
				"url": "http://localhost"
			}
		}
	}`
	if err := os.WriteFile(p, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(p)
	if err == nil {
		t.Error("Expected validation error on load")
	}
}

func TestValidateStreamableHTTPWithURL(t *testing.T) {
	cfg := &MCPConfig{
		MCPServers: map[string]ServerConfig{
			"good": {Type: "streamable-http", URL: "http://localhost:3050/mcp"},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("Expected no error for streamable-http with url, got: %v", err)
	}
}

func TestValidateStreamableHTTPWithoutURL(t *testing.T) {
	cfg := &MCPConfig{
		MCPServers: map[string]ServerConfig{
			"bad": {Type: "streamable-http"},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("Expected error for streamable-http without url")
	}
}

func TestValidateUnsupportedType(t *testing.T) {
	cfg := &MCPConfig{
		MCPServers: map[string]ServerConfig{
			"bad": {Type: "grpc", Command: "thing"},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("Expected error for unsupported type grpc")
	}
}

func TestValidateExplicitStdioWithoutCommand(t *testing.T) {
	cfg := &MCPConfig{
		MCPServers: map[string]ServerConfig{
			"bad": {Type: "stdio"},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("Expected error for stdio without command")
	}
}

func TestValidateExplicitSSEWithoutURL(t *testing.T) {
	cfg := &MCPConfig{
		MCPServers: map[string]ServerConfig{
			"bad": {Type: "sse"},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("Expected error for sse without url")
	}
}

func TestValidateBackwardCompatNoTypeField(t *testing.T) {
	cfg := &MCPConfig{
		MCPServers: map[string]ServerConfig{
			"stdio_srv": {Command: "npx"},
			"sse_srv":   {URL: "http://localhost:3000"},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("Existing configs should validate without type field: %v", err)
	}
}

func TestLoadConfigStreamableHTTP(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mcp.json")
	input := `{
		"mcpServers": {
			"1mcp": {
				"type": "streamable-http",
				"url": "http://192.168.3.100:3050/mcp",
				"headers": {"X-API-Key": "1mcp"}
			}
		}
	}`
	if err := os.WriteFile(p, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Expected non-nil config")
	}
	srv := cfg.MCPServers["1mcp"]
	if srv.Type != "streamable-http" {
		t.Errorf("Expected type streamable-http, got %q", srv.Type)
	}
	if srv.URL != "http://192.168.3.100:3050/mcp" {
		t.Errorf("Expected url, got %q", srv.URL)
	}
	if srv.Headers["X-API-Key"] != "1mcp" {
		t.Errorf("Expected header, got %v", srv.Headers)
	}
}

func TestMCPConfigJSONRoundTrip(t *testing.T) {
	original := MCPConfig{
		MCPServers: map[string]ServerConfig{
			"s": {Command: "cmd", Args: []string{"a"}, TimeoutSeconds: 5},
		},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var parsed MCPConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.MCPServers["s"].Command != "cmd" {
		t.Error("Round-trip failed")
	}
}
