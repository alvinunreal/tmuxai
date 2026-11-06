package internal

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/spf13/viper"
)

// parallelTestServer serves synthetic MCP responses, never retained live data.
func parallelTestServer(t *testing.T, body string, rpcError bool, status int, key string) (*httptest.Server, *sync.Map) {
	t.Helper()
	methods := &sync.Map{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != "tmuxai/"+Version {
			t.Errorf("User-Agent = %q", r.UserAgent())
		}
		if r.Header.Get("x-api-key") != key {
			t.Errorf("API key header mismatch")
		}
		if r.Method == http.MethodDelete {
			t.Logf("synthetic wire HTTP=DELETE User-Agent=%q; reply=500", r.UserAgent())
			methods.Store("DELETE", true)
			http.Error(w, "synthetic teardown failure", 500)
			return
		}
		var req struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		t.Logf("synthetic wire HTTP=POST RPC=%s User-Agent=%q", req.Method, r.UserAgent())
		methods.Store(req.Method, true)
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		if req.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "Application/JSON; charset=utf-8")
		response := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		switch req.Method {
		case "server/discover":
			response["error"] = map[string]any{"code": -32601, "message": "unsupported discovery"}
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "synthetic-session")
			response["result"] = map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "synthetic", "version": "1"}}
		case "tools/list":
			response["result"] = map[string]any{"tools": []any{map[string]any{"name": "web_search", "inputSchema": map[string]any{"type": "object"}}}}
		case "tools/call":
			if req.Params.Name != "web_search" || req.Params.Arguments["objective"] != "tmux documentation" {
				t.Errorf("unexpected tool arguments: %+v", req.Params)
			}
			if rpcError {
				response["error"] = map[string]any{"code": -32042, "message": "synthetic search error"}
			} else {
				response["result"] = json.RawMessage(body)
			}
		default:
			t.Errorf("unexpected method %s", req.Method)
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, methods
}

func TestParallelSearch(t *testing.T) {
	const payload = `{"results":[{"title":"<b>Tmux</b>","url":"https://example.org/tmux","excerpts":["alpha\u0085beta\u2028gamma\u2029delta"]},{"title":"extra","url":"https://example.org/extra","excerpts":[]}]}`
	for _, structured := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "structured"}[structured], func(t *testing.T) {
			content, _ := json.Marshal(payload)
			body := `{"content":[{"type":"text","text":` + string(content) + `}]}`
			if structured {
				body = `{"content":[],"structuredContent":` + payload + `}`
			}
			srv, methods := parallelTestServer(t, body, false, 0, "")
			p := NewParallelProvider("", srv.URL, nil, 2)
			r := p.Search(context.Background(), "tmux documentation", 1)
			if r.Error != nil || r.Provider != "parallel" || len(r.Results) != 1 {
				t.Fatalf("response: %+v", r)
			}
			if r.Results[0] != (SearchResult{Title: "Tmux", URL: "https://example.org/tmux", Snippet: "alpha\u0085beta\u2028gamma\u2029delta"}) {
				t.Fatalf("result: %+v", r.Results[0])
			}
			for _, m := range []string{"initialize", "notifications/initialized", "tools/list", "tools/call", "DELETE"} {
				if _, ok := methods.Load(m); !ok {
					t.Errorf("missing %s", m)
				}
			}
		})
	}
}

func TestParallelErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body, key, want string
		rpc                   bool
		status                int
	}{
		{name: "rpc", rpc: true, want: "synthetic search error"},
		{name: "tool", body: `{"isError":true,"content":[{"type":"text","text":"synthetic failure"}]}`, want: "web_search failed"},
		{name: "invalid payload", body: `{"content":[{"type":"text","text":"oops"}]}`, want: "invalid search response"},
		{name: "missing results", body: `{"content":[],"structuredContent":{}}`, want: "missing results"},
		{name: "explicit bad key", key: "bad-key", status: 401, want: "Unauthorized"},
		{name: "rate limit", status: 429, want: "Too Many Requests"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := parallelTestServer(t, tc.body, tc.rpc, tc.status, tc.key)
			r := NewParallelProvider(tc.key, srv.URL, nil, 2).Search(context.Background(), "tmux documentation", 5)
			if r.Error == nil || !strings.Contains(r.Error.Error(), tc.want) {
				t.Fatalf("error=%v, want %s", r.Error, tc.want)
			}
		})
	}
	t.Run("cancelled", func(t *testing.T) {
		srv, _ := parallelTestServer(t, `{}`, false, 0, "")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if r := NewParallelProvider("", srv.URL, nil, 2).Search(ctx, "tmux documentation", 5); r.Error == nil {
			t.Fatal("expected cancellation")
		}
	})
}

func TestParallelSelection(t *testing.T) {
	for _, tc := range []struct {
		name, primary string
		enabled       bool
		providers     map[string]config.WebSearchProviderCfg
		want          string
		count         int
	}{
		{name: "disabled", primary: "parallel", providers: map[string]config.WebSearchProviderCfg{"parallel": {}}},
		{name: "enabled no providers", enabled: true},
		{name: "explicit primary", enabled: true, primary: "parallel", providers: map[string]config.WebSearchProviderCfg{"parallel": {}, "brave": {APIKey: "b"}}, want: "parallel", count: 2},
		{name: "fallback", enabled: true, primary: "brave", providers: map[string]config.WebSearchProviderCfg{"parallel": {}, "brave": {}}, want: "brave", count: 2},
		{name: "existing keyless", enabled: true, primary: "searxng", providers: map[string]config.WebSearchProviderCfg{"searxng": {BaseURL: "http://127.0.0.1:8888"}}, want: "searxng", count: 1},
		{name: "explicit missing brave key", enabled: true, primary: "brave", providers: map[string]config.WebSearchProviderCfg{"brave": {}}, want: "brave", count: 1},
		{name: "explicit missing firecrawl key", enabled: true, primary: "firecrawl", providers: map[string]config.WebSearchProviderCfg{"firecrawl": {}}, want: "firecrawl", count: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.WebSearch.Enabled = tc.enabled
			cfg.WebSearch.DefaultProvider = tc.primary
			cfg.WebSearch.Providers = tc.providers
			m := &Manager{Config: cfg}
			m.initSearchEngine()
			if tc.count == 0 {
				if m.SearchEngine != nil {
					t.Fatal("unexpected engine")
				}
				return
			}
			if m.SearchEngine == nil || len(m.SearchEngine.providers) != tc.count || m.SearchEngine.providers[0].Name() != tc.want {
				t.Fatalf("engine=%+v", m.SearchEngine)
			}
		})
	}
}

func TestParallelCloseIdleConnections(t *testing.T) {
	closed := make(chan struct{}, 1)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateClosed {
			closed <- struct{}{}
		}
	}
	srv.Start()
	defer srv.Close()
	p := NewParallelProvider("", srv.URL, nil, 2)
	r, err := p.client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Body.Close(); err != nil {
		t.Fatal(err)
	}
	p.client.CloseIdleConnections()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("underlying idle connection still open")
	}
}

// Load the documented empty provider map through the normal config path.
func parallelManagerFromYAML(t *testing.T) *Manager {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("web_search:\n  enabled: true\n  default_provider: parallel\n  providers:\n    parallel: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{Config: cfg}
	m.initSearchEngine()
	if m.SearchEngine == nil || len(m.SearchEngine.providers) != 1 || m.SearchEngine.providers[0].Name() != "parallel" {
		t.Fatalf("documented YAML did not select Parallel: %+v", m.SearchEngine)
	}
	return m
}

func TestParallelConfigYAML(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = parallelOriginProbe(func(*http.Request) (*http.Response, error) {
		t.Error("config loading or provider selection attempted network I/O")
		return nil, errors.New("offline config test")
	})
	t.Cleanup(func() { http.DefaultTransport = original })
	m := parallelManagerFromYAML(t)
	p := m.SearchEngine.providers[0].(*ParallelProvider)
	if p.baseURL != parallelDefaultBaseURL || p.client.Transport.(*parallelTransport).apiKey != "" {
		t.Fatal("documented YAML did not select the anonymous hosted provider")
	}
	t.Log("config.Load -> initSearchEngine selects anonymous Parallel from documented parallel: {} YAML; no Search call or network")
}

// Optional real network gate through normal provider selection and SearchEngine.
func TestParallelLive(t *testing.T) {
	if os.Getenv("TMUXAI_PARALLEL_LIVE") != "1" {
		t.Skip("set TMUXAI_PARALLEL_LIVE=1 for anonymous live search")
	}
	m := parallelManagerFromYAML(t)
	p := m.SearchEngine.providers[0].(*ParallelProvider)
	p.client.Transport = &parallelWireTrace{base: p.client.Transport, t: t}
	r := m.SearchEngine.Search(context.Background(), "tmux split-window documentation")
	if r.Error != nil || r.Provider != "parallel" || len(r.Results) == 0 {
		t.Fatalf("live response: %+v", r)
	}
	for _, hit := range r.Results {
		t.Logf("source=%s passage=%s", hit.URL, hit.Snippet)
	}
}

// parallelWireTrace observes final headers written by net/http, after all SDK
// and provider middleware. It is only used by the opt-in anonymous live gate.
type parallelWireTrace struct {
	base http.RoundTripper
	t    *testing.T
}

func (tr *parallelWireTrace) RoundTrip(req *http.Request) (*http.Response, error) {
	rpc := req.Method
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err == nil {
			var msg struct {
				Method string `json:"method"`
			}
			_ = json.NewDecoder(body).Decode(&msg)
			_ = body.Close()
			rpc = msg.Method
		}
	}
	var ua string
	trace := &httptrace.ClientTrace{
		WroteHeaderField: func(key string, values []string) {
			if strings.EqualFold(key, "user-agent") {
				ua = strings.Join(values, ",")
			}
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			tr.t.Logf("wire HTTP=%s RPC=%s URL=%s User-Agent=%q write_error=%v", req.Method, rpc, req.URL, ua, info.Err)
			if ua != "tmuxai/"+Version {
				tr.t.Errorf("wire User-Agent=%q", ua)
			}
		},
	}
	return tr.base.RoundTrip(req.WithContext(httptrace.WithClientTrace(req.Context(), trace)))
}
func (tr *parallelWireTrace) CloseIdleConnections() {
	if closer, ok := tr.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func TestParallelEmptyAndBudget(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		count         int
	}{
		{"empty", `{"results":[]}`, 0},
		{"long excerpt", `{"results":[{"title":"Tmux","url":"https://example.org/tmux","excerpts":["` + strings.Repeat("界", 2000) + `"]}]}`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := parallelTestServer(t, `{"content":[],"structuredContent":`+tc.payload+`}`, false, 0, "")
			cfg := config.DefaultConfig()
			cfg.WebSearch.Enabled = true
			cfg.WebSearch.DefaultProvider = "parallel"
			cfg.WebSearch.Providers = map[string]config.WebSearchProviderCfg{"parallel": {BaseURL: srv.URL}}
			m := &Manager{Config: cfg}
			m.initSearchEngine()
			r := m.SearchEngine.Search(context.Background(), "tmux documentation")
			if r.Error != nil || len(r.Results) != tc.count {
				t.Fatalf("response: %+v", r)
			}
			if tc.count > 0 && len([]rune(r.Results[0].Snippet)) != 1000 {
				t.Fatal("snippet rune budget not applied")
			}
		})
	}
}

func TestParallelConfiguredFailover(t *testing.T) {
	for _, primary := range []string{"brave", "parallel"} {
		t.Run(primary, func(t *testing.T) {
			parallel, _ := parallelTestServer(t, `{"content":[],"structuredContent":{"results":[{"title":"Tmux","url":"https://example.org/tmux","excerpts":["synthetic passage"]}]}}`, primary == "parallel", 0, "")
			brave := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Subscription-Token") != "configured-key" {
					t.Error("Brave key was not preserved")
				}
				if primary == "brave" {
					w.WriteHeader(503)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"web":{"results":[{"title":"Tmux","url":"https://example.org/tmux","description":"synthetic Brave passage"}]}}`))
			}))
			defer brave.Close()
			cfg := config.DefaultConfig()
			cfg.WebSearch.Enabled = true
			cfg.WebSearch.DefaultProvider = primary
			cfg.WebSearch.Providers = map[string]config.WebSearchProviderCfg{"parallel": {BaseURL: parallel.URL}, "brave": {APIKey: "configured-key", BaseURL: brave.URL}}
			m := &Manager{Config: cfg}
			m.initSearchEngine()
			r := m.SearchEngine.Search(context.Background(), "tmux documentation")
			want := "parallel"
			if primary == "parallel" {
				want = "brave"
			}
			if r.Error != nil || r.Provider != want || len(r.Results) != 1 {
				t.Fatalf("response: %+v", r)
			}
		})
	}
}
