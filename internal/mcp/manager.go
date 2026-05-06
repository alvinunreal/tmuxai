package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/alvinunreal/tmuxai/logger"
)

type MCPManager struct {
	mu            sync.RWMutex
	processLife   context.Context
	cancelLife    context.CancelFunc
	servers       map[string]*ServerInfo
	sessions      map[string]*mcpsdk.ClientSession
	cmds          map[string]*exec.Cmd // stdio server commands for process group cleanup
	inFlight      sync.Map             // serverName → *atomic.Int32 (in-flight call count)
	config        *MCPConfig
	toolDefsCache string
	cacheDirty    bool
}

func NewMCPManager(cfg *MCPConfig) *MCPManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &MCPManager{
		processLife: ctx,
		cancelLife:  cancel,
		servers:     make(map[string]*ServerInfo),
		sessions:    make(map[string]*mcpsdk.ClientSession),
		cmds:        make(map[string]*exec.Cmd),
		config:      cfg,
		cacheDirty:  true,
	}
}

func (m *MCPManager) Init() error {
	var firstErr error
	for name, sc := range m.config.MCPServers {
		if sc.Disabled {
			m.mu.Lock()
			m.servers[name] = &ServerInfo{
				Name:      name,
				Config:    sc,
				Status:    StatusUnhealthy,
				ErrMsg:    "disabled",
				Transport: transportType(&sc),
			}
			m.mu.Unlock()
			continue
		}
		if err := m.initServer(name, sc); err != nil {
			logger.Info("MCP server %q init failed: %v", name, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("server %q: %w", name, err)
			}
		}
	}
	m.cacheDirty = true
	return firstErr
}

func (m *MCPManager) initServer(name string, sc ServerConfig) error {
	// Handle disabled servers: register as unhealthy and return early
	if sc.Disabled {
		m.mu.Lock()
		m.servers[name] = &ServerInfo{
			Name:      name,
			Config:    sc,
			Status:    StatusUnhealthy,
			ErrMsg:    "disabled",
			Transport: transportType(&sc),
		}
		m.mu.Unlock()
		return nil
	}

	timeout := 15 * time.Second
	if sc.TimeoutSeconds > 0 {
		timeout = time.Duration(sc.TimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(m.processLife, timeout)
	defer cancel()

	// Pre-register server so it appears in /mcp list even on failure
	m.mu.Lock()
	m.servers[name] = &ServerInfo{
		Name:      name,
		Config:    sc,
		Status:    StatusPending,
		Transport: transportType(&sc),
	}
	m.mu.Unlock()

	client := mcpsdk.NewClient(
		&mcpsdk.Implementation{Name: "tmuxai", Version: "1.0.0"},
		nil,
	)

	var transport mcpsdk.Transport
	var cmd *exec.Cmd
	if sc.Command != "" {
		cmd = exec.Command(sc.Command, sc.Args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if len(sc.Env) > 0 {
			cmd.Env = append(cmd.Environ(), envSlice(sc.Env)...)
		}
		transport = &mcpsdk.CommandTransport{Command: cmd}
	} else {
		httpClient := http.DefaultClient
		if len(sc.Headers) > 0 {
			httpClient = &http.Client{
				Transport: &headerRoundTripper{
					base:    http.DefaultTransport,
					headers: sc.Headers,
				},
			}
		}
		transport = &mcpsdk.SSEClientTransport{
			Endpoint:   sc.URL,
			HTTPClient: httpClient,
		}
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		m.mu.Lock()
		m.servers[name].Status = StatusUnhealthy
		m.servers[name].ErrMsg = fmt.Sprintf("connect: %v", err)
		m.mu.Unlock()
		return fmt.Errorf("connect: %w", err)
	}

	tools, err := m.listSessionTools(ctx, session)
	if err != nil {
		_ = session.Close()
		m.mu.Lock()
		m.servers[name].Status = StatusUnhealthy
		m.servers[name].ErrMsg = fmt.Sprintf("list tools: %v", err)
		m.mu.Unlock()
		return fmt.Errorf("list tools: %w", err)
	}

	m.mu.Lock()
	si := m.servers[name]
	si.Status = StatusHealthy
	si.Tools = tools
	m.sessions[name] = session
	// Store cmd for process group cleanup on shutdown
	if cmd != nil {
		m.cmds[name] = cmd
	}
	m.cacheDirty = true
	m.mu.Unlock()

	logger.Info("MCP server %q connected: %d tools", name, len(tools))
	return nil
}

func (m *MCPManager) listSessionTools(ctx context.Context, session *mcpsdk.ClientSession) ([]ToolDef, error) {
	var tools []ToolDef
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return tools, err
		}
		td := ToolDef{
			Name:        tool.Name,
			Description: tool.Description,
		}
		if tool.InputSchema != nil {
			schemaBytes, _ := json.Marshal(tool.InputSchema)
			td.InputSchema = schemaBytes
		}
		tools = append(tools, td)
	}
	return tools, nil
}

func (m *MCPManager) Shutdown() {
	m.cancelLife()

	m.mu.Lock()
	defer m.mu.Unlock()

	for name, session := range m.sessions {
		if err := session.Close(); err != nil {
			logger.Info("MCP: error closing session %q: %v", name, err)
		}
		delete(m.sessions, name)
	}
	// Kill process groups for all stdio servers
	for name := range m.cmds {
		m.killProcessGroup(name)
	}
	for name := range m.servers {
		delete(m.servers, name)
	}
	m.toolDefsCache = ""
	m.cacheDirty = true
}

func (m *MCPManager) GetServerInfo() []ServerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]ServerInfo, 0, len(m.servers))
	for _, info := range m.servers {
		result = append(result, *info)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func (m *MCPManager) ListTools() []ToolDef {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var tools []ToolDef
	for _, name := range sortedServerNames(m.servers) {
		info := m.servers[name]
		if info.Status != StatusHealthy {
			continue
		}
		tools = append(tools, info.Tools...)
	}
	return tools
}

func (m *MCPManager) ToolDefinitionsBlock() string {
	m.mu.RLock()
	if !m.cacheDirty {
		cache := m.toolDefsCache
		m.mu.RUnlock()
		return cache
	}
	m.mu.RUnlock()

	var b strings.Builder
	m.mu.RLock()
	names := sortedServerNames(m.servers)
	serversCopy := make(map[string]*ServerInfo, len(m.servers))
	for k, v := range m.servers {
		serversCopy[k] = v
	}
	m.mu.RUnlock()

	for _, name := range names {
		info := serversCopy[name]
		if info.Status != StatusHealthy || len(info.Tools) == 0 {
			continue
		}
		fmt.Fprintf(&b, "--- MCP: %s ---\n", name)
		for _, t := range info.Tools {
			params := formatToolParams(t.InputSchema)
			desc := t.Description
			if desc == "" {
				desc = "(no description)"
			}
			fmt.Fprintf(&b, "  - %s(%s) — %s\n", t.Name, params, desc)
		}
	}

	result := b.String()

	m.mu.Lock()
	m.toolDefsCache = result
	m.cacheDirty = false
	m.mu.Unlock()

	return result
}

func (m *MCPManager) GetSession(serverName string) *mcpsdk.ClientSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[serverName]
}

func (m *MCPManager) InvalidateCache() {
	m.mu.Lock()
	m.cacheDirty = true
	m.mu.Unlock()
}

func transportType(sc *ServerConfig) string {
	if sc.Command != "" {
		return "stdio"
	}
	return "sse"
}

func formatToolParams(schema []byte) string {
	if len(schema) == 0 {
		return ""
	}
	var s struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &s); err != nil || len(s.Properties) == 0 {
		return ""
	}

	paramNames := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		paramNames = append(paramNames, k)
	}
	sort.Strings(paramNames)

	parts := make([]string, 0, len(paramNames))
	for _, k := range paramNames {
		t := s.Properties[k].Type
		if t == "" {
			t = "any"
		}
		parts = append(parts, k+": "+t)
	}
	return strings.Join(parts, ", ")
}

func sortedServerNames(servers map[string]*ServerInfo) []string {
	names := make([]string, 0, len(servers))
	for n := range servers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func envSlice(env map[string]string) []string {
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	return t.base.RoundTrip(req)
}

func (m *MCPManager) shutdownServer(name string) {
	// Wait for in-flight calls to drain before closing
	m.waitForDrain(name, 5*time.Second)
	if session, ok := m.sessions[name]; ok {
		_ = session.Close()
		delete(m.sessions, name)
	}
	// Kill process group for stdio servers
	m.killProcessGroup(name)
	delete(m.servers, name)
}

// killProcessGroup sends SIGKILL to the entire process group of a stdio server,
// cleaning up any grandchild processes that the SDK's Close() might miss.
func (m *MCPManager) killProcessGroup(name string) {
	cmd, ok := m.cmds[name]
	if !ok || cmd == nil || cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
	delete(m.cmds, name)
}

// TrackCallStart increments the in-flight call counter for a server.
func (m *MCPManager) TrackCallStart(serverName string) {
	v, _ := m.inFlight.LoadOrStore(serverName, &atomic.Int32{})
	v.(*atomic.Int32).Add(1)
}

// TrackCallEnd decrements the in-flight call counter for a server.
func (m *MCPManager) TrackCallEnd(serverName string) {
	if v, ok := m.inFlight.Load(serverName); ok {
		v.(*atomic.Int32).Add(-1)
	}
}

// waitForDrain blocks until all in-flight calls for a server complete, or timeout expires.
func (m *MCPManager) waitForDrain(serverName string, timeout time.Duration) {
	v, ok := m.inFlight.Load(serverName)
	if !ok {
		return
	}
	counter := v.(*atomic.Int32)
	deadline := time.After(timeout)
	for counter.Load() > 0 {
		select {
		case <-deadline:
			logger.Info("MCP: drain timeout for %q, proceeding with shutdown", serverName)
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Reload diffs the current config against newCfg, shutting down removed servers,
// restarting changed ones, and initializing new ones. Work is collected under lock,
// then server init runs without holding the lock to avoid blocking other operations.
func (m *MCPManager) Reload(newCfg *MCPConfig) (added, removed, restarted, kept int, firstErr error) {
	m.mu.Lock()
	m.cacheDirty = true

	// Handle nil/empty config: shutdown everything
	if newCfg == nil || len(newCfg.MCPServers) == 0 {
		count := len(m.servers)
		for name := range m.servers {
			m.shutdownServer(name)
		}
		m.config = newCfg
		m.mu.Unlock()
		removed = count
		return
	}

	// Phase 1: Identify work under lock
	type serverWork struct {
		name string
		sc   ServerConfig
	}
	var toRemove []string
	var toAdd []serverWork
	var toRestart []serverWork

	for name := range m.servers {
		if _, exists := newCfg.MCPServers[name]; !exists {
			toRemove = append(toRemove, name)
		}
	}

	for name, newSC := range newCfg.MCPServers {
		oldInfo, exists := m.servers[name]
		if !exists {
			toAdd = append(toAdd, serverWork{name, newSC})
		} else if !configEqual(oldInfo.Config, newSC) {
			toRestart = append(toRestart, serverWork{name, newSC})
		} else {
			kept++
		}
	}

	// Phase 2: Shutdown removals and restarts under lock
	for _, name := range toRemove {
		m.shutdownServer(name)
		removed++
	}
	for _, item := range toRestart {
		m.shutdownServer(item.name)
	}
	m.mu.Unlock()

	// Phase 3: Init new/restarted servers WITHOUT holding the lock
	// (initServer acquires its own lock internally to register results)
	for _, item := range toAdd {
		err := m.initServer(item.name, item.sc)
		if err != nil {
			logger.Info("MCP reload: failed to init %q: %v", item.name, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("server %q: %w", item.name, err)
			}
		} else {
			added++
		}
	}
	for _, item := range toRestart {
		err := m.initServer(item.name, item.sc)
		if err != nil {
			logger.Info("MCP reload: failed to restart %q: %v", item.name, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("server %q: %w", item.name, err)
			}
		} else {
			restarted++
		}
	}

	// Phase 4: Update config under lock
	m.mu.Lock()
	m.config = newCfg
	m.mu.Unlock()

	return
}

func configEqual(a, b ServerConfig) bool {
	if a.Command != b.Command || a.URL != b.URL || a.Disabled != b.Disabled || a.TimeoutSeconds != b.TimeoutSeconds {
		return false
	}
	if len(a.Args) != len(b.Args) {
		return false
	}
	for i := range a.Args {
		if a.Args[i] != b.Args[i] {
			return false
		}
	}
	if len(a.Env) != len(b.Env) {
		return false
	}
	for k, v := range a.Env {
		if b.Env[k] != v {
			return false
		}
	}
	if len(a.Headers) != len(b.Headers) {
		return false
	}
	for k, v := range a.Headers {
		if b.Headers[k] != v {
			return false
		}
	}
	return true
}

// ReconnectServer tears down and reinitializes a server's session.
func (m *MCPManager) ReconnectServer(serverName string) error {
	sc, ok := m.config.MCPServers[serverName]
	if !ok {
		return fmt.Errorf("unknown server: %s", serverName)
	}

	// Wait for in-flight calls before reconnecting
	m.waitForDrain(serverName, 5*time.Second)

	// Close existing session under write lock
	m.mu.Lock()
	if oldSession, ok := m.sessions[serverName]; ok {
		_ = oldSession.Close()
		delete(m.sessions, serverName)
	}
	m.killProcessGroup(serverName)
	m.mu.Unlock()

	// Re-init (acquires its own lock to register)
	return m.initServer(serverName, sc)
}
