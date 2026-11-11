package internal

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/alvinunreal/tmuxai/logger"
)

const (
	// keenableDefaultURL is Keenable's public MCP endpoint. It needs no account
	// or API key; an optional X-API-Key raises the anonymous rate limits.
	keenableDefaultURL      = "https://api.keenable.ai/mcp-public"
	keenableProtocolVersion = "2025-03-26"
	keenableMaxResults      = 50
)

// KeenableProvider adapts Keenable's MCP search tool (search_web_pages) to
// WebSearchProvider over Streamable HTTP.
type KeenableProvider struct {
	apiKey   string
	endpoint string
	client   *http.Client
}

// NewKeenableProvider creates a KeenableProvider. apiKey is optional; an empty
// baseURL selects the public endpoint.
func NewKeenableProvider(apiKey, baseURL string, hc *http.Client, timeoutSeconds int) *KeenableProvider {
	endpoint := strings.TrimRight(baseURL, "/")
	if endpoint == "" {
		endpoint = keenableDefaultURL
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 10
	}
	if hc == nil {
		hc = &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}
	} else {
		shallowCopy := *hc
		if shallowCopy.Timeout == 0 {
			shallowCopy.Timeout = time.Duration(timeoutSeconds) * time.Second
		}
		hc = &shallowCopy
	}
	// Never follow redirects, so an optional API key is only sent to the
	// configured endpoint.
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &KeenableProvider{apiKey: apiKey, endpoint: endpoint, client: hc}
}

func (kp *KeenableProvider) Name() string {
	return "keenable"
}

type keenableRPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// rpc sends one JSON-RPC request and returns its result and any session ID.
func (kp *KeenableProvider) rpc(ctx context.Context, sessionID string, id int, method string, params any) (json.RawMessage, string, error) {
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return nil, "", fmt.Errorf("keenable: failed to encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kp.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("keenable: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", keenableProtocolVersion)
	if kp.apiKey != "" {
		req.Header.Set("X-API-Key", kp.apiKey)
	}
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := kp.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("keenable: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, "", fmt.Errorf("keenable: failed to read body: %w", err)
	}
	switch {
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return nil, "", fmt.Errorf("keenable: authentication failed. Check your KEENABLE_API_KEY, or remove it to use anonymous access")
	case resp.StatusCode == 429:
		return nil, "", fmt.Errorf("keenable: rate limited, try later")
	case resp.StatusCode != http.StatusOK:
		return nil, "", fmt.Errorf("keenable: unexpected status %d", resp.StatusCode)
	}

	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		raw = lastSSEData(raw)
	}
	var msg keenableRPCResponse
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, "", fmt.Errorf("keenable: failed to parse response: %w", err)
	}
	if msg.Error != nil {
		return nil, "", fmt.Errorf("keenable: MCP error %d: %s", msg.Error.Code, msg.Error.Message)
	}
	return msg.Result, resp.Header.Get("Mcp-Session-Id"), nil
}

// lastSSEData returns the data of the last SSE event in body.
func lastSSEData(body []byte) []byte {
	var last []byte
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		if data, ok := strings.CutPrefix(scanner.Text(), "data:"); ok {
			last = []byte(strings.TrimSpace(data))
		}
	}
	return last
}

func (kp *KeenableProvider) Search(ctx context.Context, query string, maxResults int) SearchResponse {
	logger.Debug("Keenable search: %s (maxResults=%d)", query, maxResults)
	if maxResults <= 0 {
		maxResults = 5
	}
	if maxResults > keenableMaxResults {
		maxResults = keenableMaxResults
	}

	_, sessionID, err := kp.rpc(ctx, "", 1, "initialize", map[string]any{
		"protocolVersion": keenableProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "tmuxai", "version": "1"},
	})
	if err != nil {
		return SearchResponse{Error: err}
	}
	result, _, err := kp.rpc(ctx, sessionID, 2, "tools/call", map[string]any{
		"name": "search_web_pages",
		"arguments": map[string]any{
			"query":         query,
			"max_results":   maxResults,
			"output_format": "json",
		},
	})
	if err != nil {
		return SearchResponse{Error: err}
	}

	var call struct {
		IsError bool `json:"isError"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(result, &call); err != nil {
		return SearchResponse{Error: fmt.Errorf("keenable: failed to parse tool result: %w", err)}
	}
	var text strings.Builder
	for _, c := range call.Content {
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}
	if call.IsError {
		return SearchResponse{Error: fmt.Errorf("keenable: search failed: %s", strings.TrimSpace(text.String()))}
	}

	var payload struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Snippet     string `json:"snippet"`
			Description string `json:"description"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text.String()), &payload); err != nil {
		return SearchResponse{Error: fmt.Errorf("keenable: failed to parse search results: %w", err)}
	}

	results := make([]SearchResult, 0, len(payload.Results))
	for _, r := range payload.Results {
		if r.URL == "" {
			continue
		}
		snippet := r.Snippet
		if snippet == "" {
			snippet = r.Description
		}
		results = append(results, SearchResult{
			Title:   cleanSnippet(r.Title),
			URL:     r.URL,
			Snippet: cleanSnippet(snippet),
		})
		if len(results) >= maxResults {
			break
		}
	}
	return SearchResponse{Results: results, Provider: "keenable"}
}
