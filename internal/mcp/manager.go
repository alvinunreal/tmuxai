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
	timeout := 15 * time.Second
	if sc.TimeoutSeconds > 0 {
		timeout = time.Duration(sc.TimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(m.processLife, timeout)
	defer cancel()

	client := mcpsdk.NewClient(
		&mcpsdk.Implementation{Name: "tmuxai", Version: "1.0.0"},
		nil,
	)

	var transport mcpsdk.Transport
	if sc.Command != "" {
		cmd := exec.Command(sc.Command, sc.Args...)
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
		return fmt.Errorf("connect: %w", err)
	}

	tools, err := m.listSessionTools(ctx, session)
	if err != nil {
		session.Close()
		return fmt.Errorf("list tools: %w", err)
	}

	m.mu.Lock()
	m.servers[name] = &ServerInfo{
		Name:      name,
		Config:    sc,
		Status:    StatusHealthy,
		Tools:     tools,
		Transport: transportType(&sc),
	}
	m.sessions[name] = session
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
	if session, ok := m.sessions[name]; ok {
		session.Close()
		delete(m.sessions, name)
	}
	delete(m.servers, name)
}

func (m *MCPManager) Reload(newCfg *MCPConfig) (added, removed, restarted, kept int, firstErr error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cacheDirty = true

	if newCfg == nil || len(newCfg.MCPServers) == 0 {
		for name, session := range m.sessions {
			session.Close()
			delete(m.sessions, name)
		}
		m.servers = make(map[string]*ServerInfo)
		m.config = newCfg
		removed = len(m.sessions)
		return
	}

	for name := range m.servers {
		if _, exists := newCfg.MCPServers[name]; !exists {
			m.shutdownServer(name)
			removed++
		}
	}

	for name, newSC := range newCfg.MCPServers {
		oldInfo, exists := m.servers[name]
		if !exists {
			m.mu.Unlock()
			err := m.initServer(name, newSC)
			m.mu.Lock()
			if err != nil {
				logger.Info("MCP reload: failed to init %q: %v", name, err)
				if firstErr == nil {
					firstErr = fmt.Errorf("server %q: %w", name, err)
				}
			} else {
				added++
			}
			continue
		}

		if configEqual(oldInfo.Config, newSC) {
			kept++
			continue
		}

		m.shutdownServer(name)
		m.mu.Unlock()
		err := m.initServer(name, newSC)
		m.mu.Lock()
		if err != nil {
			logger.Info("MCP reload: failed to restart %q: %v", name, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("server %q: %w", name, err)
			}
		} else {
			restarted++
		}
	}

	m.config = newCfg
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

func (m *MCPManager) ReconnectServer(serverName string) error {
	sc, ok := m.config.MCPServers[serverName]
	if !ok {
		return fmt.Errorf("unknown server: %s", serverName)
	}
	// Close existing session if stale
	m.mu.RLock()
	oldSession := m.sessions[serverName]
	m.mu.RUnlock()
	if oldSession != nil {
		oldSession.Close()
	}
	// Re-init the server
	return m.initServer(serverName, sc)
}
