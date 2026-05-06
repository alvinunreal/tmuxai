package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTransportType(t *testing.T) {
	tests := []struct {
		name string
		sc   ServerConfig
		want string
	}{
		{"command", ServerConfig{Command: "/bin/foo"}, "stdio"},
		{"url", ServerConfig{URL: "http://localhost"}, "sse"},
		{"empty", ServerConfig{}, "sse"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := transportType(&tt.sc); got != tt.want {
				t.Errorf("transportType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatToolParams(t *testing.T) {
	tests := []struct {
		name   string
		schema []byte
		want   string
	}{
		{"empty", nil, ""},
		{"empty json", []byte("{}"), ""},
		{"invalid json", []byte("not-json"), ""},
		{"no properties", []byte(`{"properties":{}}`), ""},
		{"single param", []byte(`{"properties":{"path":{"type":"string"}}}`), "path: string"},
		{"multi params sorted", []byte(`{"properties":{"z":{"type":"number"},"a":{"type":"string"}}}`), "a: string, z: number"},
		{"missing type defaults any", []byte(`{"properties":{"foo":{}}}`), "foo: any"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatToolParams(tt.schema); got != tt.want {
				t.Errorf("formatToolParams() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSortedServerNames(t *testing.T) {
	servers := map[string]*ServerInfo{
		"zeta":  {},
		"alpha": {},
		"mid":   {},
	}
	names := sortedServerNames(servers)
	if len(names) != 3 {
		t.Fatalf("Expected 3, got %d", len(names))
	}
	if names[0] != "alpha" || names[1] != "mid" || names[2] != "zeta" {
		t.Errorf("Expected sorted, got %v", names)
	}
}

func TestSortedServerNamesEmpty(t *testing.T) {
	names := sortedServerNames(nil)
	if len(names) != 0 {
		t.Errorf("Expected empty, got %v", names)
	}
}

func TestEnvSlice(t *testing.T) {
	env := map[string]string{"B": "2", "A": "1"}
	result := envSlice(env)
	if len(result) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(result))
	}
	found := make(map[string]bool)
	for _, e := range result {
		found[e] = true
	}
	if !found["A=1"] || !found["B=2"] {
		t.Errorf("Expected A=1 and B=2, got %v", result)
	}
}

func TestConfigEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b ServerConfig
		want bool
	}{
		{
			"identical",
			ServerConfig{Command: "cmd", Args: []string{"a"}, TimeoutSeconds: 5, Disabled: false, Env: map[string]string{"K": "v"}, Headers: map[string]string{"H": "v"}},
			ServerConfig{Command: "cmd", Args: []string{"a"}, TimeoutSeconds: 5, Disabled: false, Env: map[string]string{"K": "v"}, Headers: map[string]string{"H": "v"}},
			true,
		},
		{
			"different command",
			ServerConfig{Command: "a"},
			ServerConfig{Command: "b"},
			false,
		},
		{
			"different url",
			ServerConfig{URL: "a"},
			ServerConfig{URL: "b"},
			false,
		},
		{
			"different disabled",
			ServerConfig{Disabled: false},
			ServerConfig{Disabled: true},
			false,
		},
		{
			"different timeout",
			ServerConfig{TimeoutSeconds: 1},
			ServerConfig{TimeoutSeconds: 2},
			false,
		},
		{
			"different args len",
			ServerConfig{Args: []string{"a"}},
			ServerConfig{Args: []string{"a", "b"}},
			false,
		},
		{
			"different args val",
			ServerConfig{Args: []string{"a"}},
			ServerConfig{Args: []string{"b"}},
			false,
		},
		{
			"different env len",
			ServerConfig{Env: map[string]string{"A": "1"}},
			ServerConfig{Env: map[string]string{"A": "1", "B": "2"}},
			false,
		},
		{
			"different env val",
			ServerConfig{Env: map[string]string{"A": "1"}},
			ServerConfig{Env: map[string]string{"A": "2"}},
			false,
		},
		{
			"different headers len",
			ServerConfig{Headers: map[string]string{"A": "1"}},
			ServerConfig{Headers: map[string]string{}},
			false,
		},
		{
			"different headers val",
			ServerConfig{Headers: map[string]string{"A": "1"}},
			ServerConfig{Headers: map[string]string{"A": "2"}},
			false,
		},
		{
			"nil slices and maps",
			ServerConfig{},
			ServerConfig{},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := configEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("configEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewMCPManager(t *testing.T) {
	cfg := &MCPConfig{MCPServers: map[string]ServerConfig{}}
	mgr := NewMCPManager(cfg)
	if mgr == nil {
		t.Fatal("Expected non-nil manager")
	}
	if mgr.config != cfg {
		t.Error("Config not set")
	}
	if !mgr.cacheDirty {
		t.Error("Expected cacheDirty=true on new manager")
	}
}

func TestMCPManagerGetServerInfo(t *testing.T) {
	cfg := &MCPConfig{MCPServers: map[string]ServerConfig{}}
	mgr := NewMCPManager(cfg)
	mgr.mu.Lock()
	mgr.servers["b"] = &ServerInfo{Name: "b", Status: StatusHealthy}
	mgr.servers["a"] = &ServerInfo{Name: "a", Status: StatusPending}
	mgr.mu.Unlock()
	info := mgr.GetServerInfo()
	if len(info) != 2 {
		t.Fatalf("Expected 2, got %d", len(info))
	}
	if info[0].Name != "a" || info[1].Name != "b" {
		t.Errorf("Expected sorted by name, got %v", info)
	}
}

func TestMCPManagerListTools(t *testing.T) {
	cfg := &MCPConfig{MCPServers: map[string]ServerConfig{}}
	mgr := NewMCPManager(cfg)
	mgr.mu.Lock()
	mgr.servers["srv"] = &ServerInfo{
		Name:   "srv",
		Status: StatusHealthy,
		Tools:  []ToolDef{{Name: "tool1"}, {Name: "tool2"}},
	}
	mgr.servers["down"] = &ServerInfo{
		Name:   "down",
		Status: StatusUnhealthy,
		Tools:  []ToolDef{{Name: "tool3"}},
	}
	mgr.mu.Unlock()
	tools := mgr.ListTools()
	if len(tools) != 2 {
		t.Fatalf("Expected 2 (healthy only), got %d", len(tools))
	}
	if tools[0].Name != "tool1" || tools[1].Name != "tool2" {
		t.Errorf("Expected tool1,tool2, got %v", tools)
	}
}

func TestMCPManagerToolDefinitionsBlock(t *testing.T) {
	cfg := &MCPConfig{MCPServers: map[string]ServerConfig{}}
	mgr := NewMCPManager(cfg)
	mgr.mu.Lock()
	mgr.servers["fs"] = &ServerInfo{
		Name:   "fs",
		Status: StatusHealthy,
		Tools: []ToolDef{
			{Name: "read_file", Description: "Read a file"},
			{Name: "write_file", Description: "Write a file", InputSchema: []byte(`{"properties":{"path":{"type":"string"},"content":{"type":"string"}}}`)},
		},
	}
	mgr.servers["empty"] = &ServerInfo{
		Name:   "empty",
		Status: StatusHealthy,
		Tools:  []ToolDef{},
	}
	mgr.mu.Unlock()
	block := mgr.ToolDefinitionsBlock()
	if block == "" {
		t.Fatal("Expected non-empty block")
	}
	if !contains(block, "--- MCP: fs ---") {
		t.Errorf("Expected server header, got %q", block)
	}
	if !contains(block, "read_file") {
		t.Errorf("Expected tool name, got %q", block)
	}
	if !contains(block, "write_file(content: string, path: string)") {
		t.Errorf("Expected formatted params, got %q", block)
	}
}

func TestMCPManagerToolDefinitionsBlockCaches(t *testing.T) {
	cfg := &MCPConfig{MCPServers: map[string]ServerConfig{}}
	mgr := NewMCPManager(cfg)
	mgr.mu.Lock()
	mgr.servers["s"] = &ServerInfo{
		Name:   "s",
		Status: StatusHealthy,
		Tools:  []ToolDef{{Name: "t", Description: "d"}},
	}
	mgr.mu.Unlock()
	first := mgr.ToolDefinitionsBlock()
	mgr.cacheDirty = false
	second := mgr.ToolDefinitionsBlock()
	if first != second {
		t.Error("Expected cached result to match")
	}
	if mgr.cacheDirty {
		t.Error("Expected cache to be clean after read")
	}
}

func TestMCPManagerGetSession(t *testing.T) {
	cfg := &MCPConfig{MCPServers: map[string]ServerConfig{}}
	mgr := NewMCPManager(cfg)
	s := mgr.GetSession("nonexistent")
	if s != nil {
		t.Error("Expected nil for nonexistent session")
	}
}

func TestMCPManagerInvalidateCache(t *testing.T) {
	cfg := &MCPConfig{MCPServers: map[string]ServerConfig{}}
	mgr := NewMCPManager(cfg)
	mgr.cacheDirty = false
	mgr.InvalidateCache()
	if !mgr.cacheDirty {
		t.Error("Expected cacheDirty after invalidate")
	}
}

func TestMCPManagerShutdown(t *testing.T) {
	cfg := &MCPConfig{MCPServers: map[string]ServerConfig{}}
	mgr := NewMCPManager(cfg)
	mgr.mu.Lock()
	mgr.servers["s"] = &ServerInfo{Name: "s"}
	mgr.mu.Unlock()
	mgr.Shutdown()
	if len(mgr.servers) != 0 {
		t.Error("Expected servers cleared")
	}
	if mgr.toolDefsCache != "" {
		t.Error("Expected cache cleared")
	}
	if !mgr.cacheDirty {
		t.Error("Expected cacheDirty after shutdown")
	}
}

func TestMCPManagerReconnectServerUnknown(t *testing.T) {
	cfg := &MCPConfig{MCPServers: map[string]ServerConfig{}}
	mgr := NewMCPManager(cfg)
	err := mgr.ReconnectServer("unknown")
	if err == nil {
		t.Error("Expected error for unknown server")
	}
}

func TestHeaderRoundTripper(t *testing.T) {
	var capturedHeaders http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	rt := &headerRoundTripper{
		base:    http.DefaultTransport,
		headers: map[string]string{"X-Custom": "test-value", "Authorization": "Bearer tok"},
	}
	client := &http.Client{Transport: rt}
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if capturedHeaders.Get("X-Custom") != "test-value" {
		t.Errorf("Expected X-Custom header, got %v", capturedHeaders.Get("X-Custom"))
	}
	if capturedHeaders.Get("Authorization") != "Bearer tok" {
		t.Errorf("Expected Authorization header, got %v", capturedHeaders.Get("Authorization"))
	}
}

func TestMCPManagerContextCancelledOnShutdown(t *testing.T) {
	cfg := &MCPConfig{MCPServers: map[string]ServerConfig{}}
	mgr := NewMCPManager(cfg)
	ctx := mgr.processLife
	mgr.Shutdown()
	select {
	case <-ctx.Done():
	default:
		t.Error("Expected processLife context to be cancelled after shutdown")
	}
}

// TestLazyReconnectUnknownServer verifies that reconnecting a nonexistent server fails gracefully.
func TestLazyReconnectUnknownServer(t *testing.T) {
	cfg := &MCPConfig{MCPServers: map[string]ServerConfig{}}
	mgr := NewMCPManager(cfg)
	ok := LazyReconnect(mgr, "nonexistent")
	if ok {
		t.Error("Expected LazyReconnect to fail for unknown server")
	}
}

// TestTrackCallStartEnd verifies in-flight call tracking and drain behavior.
func TestTrackCallStartEnd(t *testing.T) {
	cfg := &MCPConfig{MCPServers: map[string]ServerConfig{}}
	mgr := NewMCPManager(cfg)

	mgr.TrackCallStart("srv")
	mgr.TrackCallStart("srv")

	// waitForDrain should block briefly then succeed after TrackCallEnd
	done := make(chan bool, 1)
	go func() {
		mgr.TrackCallEnd("srv")
		mgr.TrackCallEnd("srv")
		done <- true
	}()
	<-done

	// After all calls end, drain should succeed immediately
	mgr.waitForDrain("srv", 1*time.Second)
}

// TestInitServerRegistersOnFailure verifies that a failed server still appears in GetServerInfo as unhealthy.
func TestInitServerRegistersOnFailure(t *testing.T) {
	cfg := &MCPConfig{MCPServers: map[string]ServerConfig{
		"badserver": {Command: "/nonexistent/binary/path"},
	}}
	mgr := NewMCPManager(cfg)
	err := mgr.initServer("badserver", cfg.MCPServers["badserver"])
	if err == nil {
		t.Fatal("Expected error from initServer with bad command")
	}

	// Server should still be registered as unhealthy
	servers := mgr.GetServerInfo()
	if len(servers) != 1 {
		t.Fatalf("Expected 1 server registered, got %d", len(servers))
	}
	if servers[0].Status != StatusUnhealthy {
		t.Errorf("Expected StatusUnhealthy, got %s", servers[0].Status)
	}
	if servers[0].ErrMsg == "" {
		t.Error("Expected ErrMsg to be set")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
