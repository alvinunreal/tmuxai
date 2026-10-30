package internal

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/alvinunreal/tmuxai/config"
)

const firecrawlSearchFixture = `{
	"success": true,
	"data": {
		"web": [
			{"url": "https://example.com/a", "title": "First <b>hit</b>", "description": "Snippet &amp; more"},
			{"url": "https://example.com/b", "title": "Second hit", "description": "Another snippet"}
		]
	}
}`

type firecrawlCapture struct {
	method string
	path   string
	header http.Header
	body   map[string]any
}

func firecrawlTestServer(t *testing.T, status int, response string, capture *firecrawlCapture) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		capture.method = r.Method
		capture.path = r.URL.Path
		capture.header = r.Header.Clone()
		_ = json.Unmarshal(raw, &capture.body)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// rewriteTransport sends every request to target, recording the original URL.
type rewriteTransport struct {
	target   *url.URL
	original *url.URL
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.original = req.URL
	clone := req.Clone(req.Context())
	clone.URL.Scheme = rt.target.Scheme
	clone.URL.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(clone)
}

func Test_FirecrawlProvider_Search_RequestAndMapping(t *testing.T) {
	var got firecrawlCapture
	srv := firecrawlTestServer(t, http.StatusOK, firecrawlSearchFixture, &got)

	p := NewFirecrawlProvider("fc-test-key", srv.URL, nil, 5)
	resp := p.Search(context.Background(), "tmux tips", 3)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if resp.Provider != "firecrawl" || p.Name() != "firecrawl" {
		t.Errorf("provider name: got %q / %q, want firecrawl", resp.Provider, p.Name())
	}
	if got.method != http.MethodPost || got.path != "/v2/search" {
		t.Errorf("request: got %s %s, want POST /v2/search", got.method, got.path)
	}
	if auth := got.header.Get("Authorization"); auth != "Bearer fc-test-key" {
		t.Errorf("Authorization: got %q", auth)
	}
	if ct := got.header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q", ct)
	}
	if got.body["query"] != "tmux tips" {
		t.Errorf("query: got %v", got.body["query"])
	}
	if got.body["limit"] != float64(3) {
		t.Errorf("limit: got %v, want 3", got.body["limit"])
	}
	if len(resp.Results) != 2 {
		t.Fatalf("results: got %d, want 2", len(resp.Results))
	}
	first := resp.Results[0]
	if first.URL != "https://example.com/a" || first.Title != "First hit" || first.Snippet != "Snippet & more" {
		t.Errorf("first result not mapped and cleaned: %+v", first)
	}
}

func Test_FirecrawlProvider_Search_HostedRequestCarriesIntegrationTag(t *testing.T) {
	for _, baseURL := range []string{"", "https://api.firecrawl.dev", "https://api.firecrawl.dev/"} {
		t.Run("base_url="+baseURL, func(t *testing.T) {
			var got firecrawlCapture
			srv := firecrawlTestServer(t, http.StatusOK, firecrawlSearchFixture, &got)
			target, _ := url.Parse(srv.URL)
			rt := &rewriteTransport{target: target}

			p := NewFirecrawlProvider("fc-test-key", baseURL, &http.Client{Transport: rt}, 5)
			resp := p.Search(context.Background(), "tmux tips", 5)

			if resp.Error != nil {
				t.Fatalf("unexpected error: %v", resp.Error)
			}
			if rt.original == nil || rt.original.String() != "https://api.firecrawl.dev/v2/search" {
				t.Errorf("request URL: got %v, want https://api.firecrawl.dev/v2/search", rt.original)
			}
			if got.body["integration"] != "_tmuxai" {
				t.Errorf("integration: got %v, want _tmuxai", got.body["integration"])
			}
		})
	}
}

func Test_FirecrawlProvider_Search_CustomBaseURLOmitsIntegrationTag(t *testing.T) {
	var got firecrawlCapture
	srv := firecrawlTestServer(t, http.StatusOK, firecrawlSearchFixture, &got)

	p := NewFirecrawlProvider("", srv.URL+"/", nil, 5)
	resp := p.Search(context.Background(), "tmux tips", 5)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if _, ok := got.body["integration"]; ok {
		t.Errorf("self-hosted request must not carry integration, got %v", got.body["integration"])
	}
	if got.path != "/v2/search" {
		t.Errorf("path: got %q, want /v2/search (trailing slash trimmed)", got.path)
	}
	if auth := got.header.Get("Authorization"); auth != "" {
		t.Errorf("no key configured, Authorization should be absent, got %q", auth)
	}
}

func Test_FirecrawlProvider_Search_DefaultLimit(t *testing.T) {
	var got firecrawlCapture
	srv := firecrawlTestServer(t, http.StatusOK, firecrawlSearchFixture, &got)

	NewFirecrawlProvider("k", srv.URL, nil, 5).Search(context.Background(), "q", 0)

	if got.body["limit"] != float64(5) {
		t.Errorf("limit: got %v, want 5", got.body["limit"])
	}
}

func Test_FirecrawlProvider_Search_Errors(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		response string
		wantErr  string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"success":false}`, "authentication failed"},
		{"forbidden", http.StatusForbidden, `{"success":false}`, "authentication failed"},
		{"payment required", http.StatusPaymentRequired, `{"success":false}`, "payment required"},
		{"rate limited", http.StatusTooManyRequests, `{"success":false}`, "rate limited"},
		{"server error", http.StatusInternalServerError, `oops`, "unexpected status 500"},
		{"invalid json", http.StatusOK, `not json`, "failed to parse response"},
		{"flat data list is not the v2 shape", http.StatusOK, `{"success":true,"data":[{"url":"https://example.com"}]}`, "failed to parse response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got firecrawlCapture
			srv := firecrawlTestServer(t, tt.status, tt.response, &got)

			resp := NewFirecrawlProvider("k", srv.URL, nil, 5).Search(context.Background(), "q", 5)

			if resp.Error == nil || !strings.Contains(resp.Error.Error(), tt.wantErr) {
				t.Errorf("error: got %v, want substring %q", resp.Error, tt.wantErr)
			}
		})
	}
}

func Test_FirecrawlProvider_Search_NoWebResults(t *testing.T) {
	var got firecrawlCapture
	srv := firecrawlTestServer(t, http.StatusOK, `{"success":true,"data":{}}`, &got)

	resp := NewFirecrawlProvider("k", srv.URL, nil, 5).Search(context.Background(), "q", 5)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if resp.Results == nil || len(resp.Results) != 0 {
		t.Errorf("results: got %#v, want empty non-nil slice", resp.Results)
	}
}

func Test_initSearchEngine_FirecrawlProvider(t *testing.T) {
	newManager := func(defaultProvider string, providers map[string]config.WebSearchProviderCfg) *Manager {
		cfg := config.DefaultConfig()
		cfg.WebSearch.Enabled = true
		cfg.WebSearch.DefaultProvider = defaultProvider
		cfg.WebSearch.Providers = providers
		return &Manager{Config: cfg}
	}

	t.Run("firecrawl as default provider", func(t *testing.T) {
		m := newManager("firecrawl", map[string]config.WebSearchProviderCfg{
			"brave":     {APIKey: "b"},
			"firecrawl": {APIKey: "f"},
		})
		m.initSearchEngine()
		if m.SearchEngine == nil || len(m.SearchEngine.providers) != 2 {
			t.Fatalf("expected 2 providers, got %+v", m.SearchEngine)
		}
		if name := m.SearchEngine.providers[0].Name(); name != "firecrawl" {
			t.Errorf("primary provider: got %q, want firecrawl", name)
		}
	})

	t.Run("brave stays the default when firecrawl is only configured", func(t *testing.T) {
		m := newManager("", map[string]config.WebSearchProviderCfg{
			"firecrawl": {APIKey: "f"},
			"brave":     {APIKey: "b"},
		})
		m.initSearchEngine()
		if m.SearchEngine == nil || len(m.SearchEngine.providers) != 2 {
			t.Fatalf("expected 2 providers, got %+v", m.SearchEngine)
		}
		if name := m.SearchEngine.providers[0].Name(); name != "brave" {
			t.Errorf("primary provider: got %q, want brave", name)
		}
		if name := m.SearchEngine.providers[1].Name(); name != "firecrawl" {
			t.Errorf("fallback provider: got %q, want firecrawl", name)
		}
	})
}
