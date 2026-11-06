package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const parallelDefaultBaseURL = "https://mcp.parallel.ai/mcp"

// ParallelProvider adapts the anonymous Parallel MCP web_search tool.
// It uses the server's free-tier Fast defaults and never requests other tools.
type ParallelProvider struct {
	baseURL string
	client  *http.Client
	timeout time.Duration
}

// NewParallelProvider creates an opt-in provider. baseURL is the complete MCP
// endpoint; an empty value selects the hosted service. No API key is required.
func NewParallelProvider(apiKey, baseURL string, hc *http.Client, timeoutSeconds int) *ParallelProvider {
	if baseURL == "" {
		baseURL = parallelDefaultBaseURL
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 10
	}
	client := http.Client{}
	if hc != nil {
		client = *hc
	}
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
		if base, ok := transport.(*http.Transport); ok {
			transport = base.Clone()
		}
	}
	origin, _ := url.Parse(baseURL)
	client.Transport = &parallelTransport{base: transport, apiKey: apiKey, origin: origin}
	return &ParallelProvider{baseURL: baseURL, client: &client, timeout: time.Duration(timeoutSeconds) * time.Second}
}

func (p *ParallelProvider) Name() string { return "parallel" }

// parallelTransport stamps every SDK request, including notifications and DELETE.
// Clone the request to avoid mutating SDK-owned headers or an injected client.
type parallelTransport struct {
	base   http.RoundTripper
	apiKey string
	origin *url.URL
}

func (t *parallelTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Check every request before adding credentials or performing network I/O.
	// Redirects and SDK-generated requests share this boundary; CheckRedirect
	// alone would not cover requests made directly by the SDK.
	if t.apiKey != "" && (!parallelSameOrigin(t.origin, req.URL) ||
		(req.Host != "" && !parallelSameOrigin(t.origin, &url.URL{Scheme: req.URL.Scheme, Host: req.Host}))) {
		return nil, fmt.Errorf("parallel: refusing authenticated request outside configured origin")
	}
	req = req.Clone(req.Context())
	req.Header.Set("User-Agent", "tmuxai/"+Version)
	if t.apiKey != "" {
		req.Header.Set("x-api-key", t.apiKey)
	}
	return t.base.RoundTrip(req)
}

// Compare scheme, hostname and effective port. An HTTPS downgrade is always a
// different origin, including when the host and port are otherwise identical.
func parallelSameOrigin(a, b *url.URL) bool {
	if a == nil || b == nil || a.Hostname() == "" || b.Hostname() == "" {
		return false
	}
	port := func(u *url.URL) string {
		if u.Port() != "" {
			return u.Port()
		}
		if strings.EqualFold(u.Scheme, "https") {
			return "443"
		}
		return "80"
	}
	return (strings.EqualFold(a.Scheme, "http") || strings.EqualFold(a.Scheme, "https")) &&
		strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}

func (t *parallelTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func (p *ParallelProvider) Search(ctx context.Context, query string, maxResults int) SearchResponse {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	defer p.client.CloseIdleConnections()
	fail := func(err error) SearchResponse {
		return SearchResponse{Provider: p.Name(), Error: fmt.Errorf("parallel: %w", err)}
	}
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "tmuxai", Version: Version}, nil)
	session, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{
		Endpoint: p.baseURL, HTTPClient: p.client, DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		return fail(err)
	}
	// Remote cleanup is best effort and must not replace a result or primary error.
	defer func() { _ = session.Close() }()
	found := false
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return fail(err)
		}
		if tool.Name == "web_search" {
			found = true
			break
		}
	}
	if !found {
		return fail(fmt.Errorf("web_search tool unavailable"))
	}
	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "web_search", Arguments: map[string]any{
		"objective": query, "search_queries": []string{query},
	}})
	if err != nil {
		return fail(err)
	}
	if result.IsError {
		var messages []string
		for _, content := range result.Content {
			if text, ok := content.(*mcpsdk.TextContent); ok {
				messages = append(messages, text.Text)
			}
		}
		return fail(fmt.Errorf("web_search failed: %s", strings.Join(messages, "; ")))
	}
	var data []byte
	if result.StructuredContent != nil {
		data, err = json.Marshal(result.StructuredContent)
	} else {
		for _, content := range result.Content {
			if text, ok := content.(*mcpsdk.TextContent); ok {
				data = []byte(text.Text)
				break
			}
		}
	}
	if err != nil {
		return fail(err)
	}
	var payload struct {
		Results []struct {
			Title    string   `json:"title"`
			URL      string   `json:"url"`
			Excerpts []string `json:"excerpts"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fail(fmt.Errorf("invalid search response: %w", err))
	}
	if payload.Results == nil {
		return fail(fmt.Errorf("search response missing results"))
	}
	if maxResults <= 0 {
		maxResults = 5
	}
	results := make([]SearchResult, 0, min(maxResults, len(payload.Results)))
	for _, r := range payload.Results {
		if len(results) >= maxResults {
			break
		}
		// MCP excerpts can span entire pages. Keep snippets useful within the normal
		// SearchEngine character budget instead of dropping every large result.
		snippet := []rune(cleanSnippet(strings.Join(r.Excerpts, "\n")))
		if len(snippet) > 1000 {
			snippet = append(snippet[:999], '…')
		}
		results = append(results, SearchResult{Title: cleanSnippet(r.Title), URL: r.URL, Snippet: string(snippet)})
	}
	return SearchResponse{Provider: p.Name(), Results: results}
}
