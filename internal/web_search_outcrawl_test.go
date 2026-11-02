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

const outcrawlSearchFixture = `{
	"hits": [
		{"url": "https://example.com/a", "title": "First <b>hit</b>", "snippet": "Snippet &amp; more", "rank": 1},
		{"url": "https://example.com/b", "title": "Second hit", "snippet": "Another snippet", "rank": 2}
	],
	"usage": {"credits": "1"}
}`

type outcrawlCapture struct {
	method string
	path   string
	header http.Header
	body   map[string]any
}

func outcrawlTestServer(t *testing.T, status int, response string, capture *outcrawlCapture) *httptest.Server {
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

func Test_OutcrawlProvider_Search_RequestAndMapping(t *testing.T) {
	var got outcrawlCapture
	srv := outcrawlTestServer(t, http.StatusOK, outcrawlSearchFixture, &got)

	p := NewOutcrawlProvider("oc-test-key", srv.URL+"/", nil, 5)
	resp := p.Search(context.Background(), "tmux tips", 3)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if resp.Provider != "outcrawl" || p.Name() != "outcrawl" {
		t.Errorf("provider name: got %q / %q, want outcrawl", resp.Provider, p.Name())
	}
	if got.method != http.MethodPost || got.path != "/v1/search" {
		t.Errorf("request: got %s %s, want POST /v1/search", got.method, got.path)
	}
	if auth := got.header.Get("Authorization"); auth != "Bearer oc-test-key" {
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

func Test_OutcrawlProvider_Search_DefaultBaseURL(t *testing.T) {
	var got outcrawlCapture
	srv := outcrawlTestServer(t, http.StatusOK, outcrawlSearchFixture, &got)
	target, _ := url.Parse(srv.URL)
	rt := &rewriteTransport{target: target}

	resp := NewOutcrawlProvider("k", "", &http.Client{Transport: rt}, 5).Search(context.Background(), "q", 5)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if rt.original == nil || rt.original.String() != "https://api.outcrawl.ai/v1/search" {
		t.Errorf("request URL: got %v, want https://api.outcrawl.ai/v1/search", rt.original)
	}
}

func Test_OutcrawlProvider_Search_LimitBounds(t *testing.T) {
	tests := []struct {
		name       string
		maxResults int
		wantLimit  float64
	}{
		{"default", 0, 5},
		{"capped at the API maximum", 50, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got outcrawlCapture
			srv := outcrawlTestServer(t, http.StatusOK, outcrawlSearchFixture, &got)

			NewOutcrawlProvider("k", srv.URL, nil, 5).Search(context.Background(), "q", tt.maxResults)

			if got.body["limit"] != tt.wantLimit {
				t.Errorf("limit: got %v, want %v", got.body["limit"], tt.wantLimit)
			}
		})
	}
}

func Test_OutcrawlProvider_Search_Errors(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		response string
		wantErr  string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"code":"unauthorized"}`, "authentication failed"},
		{"forbidden", http.StatusForbidden, `{"code":"unauthorized"}`, "authentication failed"},
		{"out of credits", http.StatusPaymentRequired, `{"code":"quota_exceeded"}`, "out of credits"},
		{"rate limited", http.StatusTooManyRequests, `{"code":"quota_exceeded"}`, "rate limited"},
		{"server error", http.StatusInternalServerError, `oops`, "unexpected status 500"},
		{"invalid json", http.StatusOK, `not json`, "failed to parse response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got outcrawlCapture
			srv := outcrawlTestServer(t, tt.status, tt.response, &got)

			resp := NewOutcrawlProvider("k", srv.URL, nil, 5).Search(context.Background(), "q", 5)

			if resp.Error == nil || !strings.Contains(resp.Error.Error(), tt.wantErr) {
				t.Errorf("error: got %v, want substring %q", resp.Error, tt.wantErr)
			}
		})
	}
}

func Test_OutcrawlProvider_Search_NoHits(t *testing.T) {
	var got outcrawlCapture
	srv := outcrawlTestServer(t, http.StatusOK, `{"hits":[],"usage":{"credits":"0"}}`, &got)

	resp := NewOutcrawlProvider("k", srv.URL, nil, 5).Search(context.Background(), "q", 5)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if resp.Results == nil || len(resp.Results) != 0 {
		t.Errorf("results: got %#v, want empty non-nil slice", resp.Results)
	}
}

func Test_initSearchEngine_OutcrawlProvider(t *testing.T) {
	newManager := func(defaultProvider string, providers map[string]config.WebSearchProviderCfg) *Manager {
		cfg := config.DefaultConfig()
		cfg.WebSearch.Enabled = true
		cfg.WebSearch.DefaultProvider = defaultProvider
		cfg.WebSearch.Providers = providers
		return &Manager{Config: cfg}
	}

	t.Run("outcrawl as default provider", func(t *testing.T) {
		m := newManager("outcrawl", map[string]config.WebSearchProviderCfg{
			"brave":    {APIKey: "b"},
			"outcrawl": {APIKey: "o"},
		})
		m.initSearchEngine()
		if m.SearchEngine == nil || len(m.SearchEngine.providers) != 2 {
			t.Fatalf("expected 2 providers, got %+v", m.SearchEngine)
		}
		if name := m.SearchEngine.providers[0].Name(); name != "outcrawl" {
			t.Errorf("primary provider: got %q, want outcrawl", name)
		}
	})

	t.Run("brave stays the default when outcrawl is only configured", func(t *testing.T) {
		m := newManager("", map[string]config.WebSearchProviderCfg{
			"outcrawl": {APIKey: "o"},
			"brave":    {APIKey: "b"},
		})
		m.initSearchEngine()
		if m.SearchEngine == nil || len(m.SearchEngine.providers) != 2 {
			t.Fatalf("expected 2 providers, got %+v", m.SearchEngine)
		}
		if name := m.SearchEngine.providers[0].Name(); name != "brave" {
			t.Errorf("primary provider: got %q, want brave", name)
		}
		if name := m.SearchEngine.providers[1].Name(); name != "outcrawl" {
			t.Errorf("fallback provider: got %q, want outcrawl", name)
		}
	})
}
