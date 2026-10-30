package internal

import (
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
	firecrawlDefaultBaseURL = "https://api.firecrawl.dev"
	// firecrawlIntegration tells Firecrawl which of its API requests come from TmuxAI.
	// It is only sent to the hosted API, never to a custom base_url.
	firecrawlIntegration = "_tmuxai"
)

// FirecrawlProvider adapts the Firecrawl Search API (v2) to WebSearchProvider.
type FirecrawlProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewFirecrawlProvider creates a FirecrawlProvider. An empty baseURL selects the
// hosted API; set baseURL to point at a self-hosted Firecrawl instance.
func NewFirecrawlProvider(apiKey, baseURL string, hc *http.Client, timeoutSeconds int) *FirecrawlProvider {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = firecrawlDefaultBaseURL
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 10
	}
	if hc == nil {
		hc = &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}
	} else if hc.Timeout == 0 {
		shallowCopy := *hc
		shallowCopy.Timeout = time.Duration(timeoutSeconds) * time.Second
		hc = &shallowCopy
	}
	return &FirecrawlProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  hc,
	}
}

func (fp *FirecrawlProvider) Name() string {
	return "firecrawl"
}

func (fp *FirecrawlProvider) Search(ctx context.Context, query string, maxResults int) SearchResponse {
	logger.Debug("Firecrawl search: %s (maxResults=%d)", query, maxResults)
	if maxResults <= 0 {
		maxResults = 5
	}

	payload := map[string]any{
		"query": query,
		"limit": maxResults,
	}
	if fp.baseURL == firecrawlDefaultBaseURL {
		payload["integration"] = firecrawlIntegration
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return SearchResponse{Error: fmt.Errorf("firecrawl: failed to encode request: %w", err)}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fp.baseURL+"/v2/search", bytes.NewReader(reqBody))
	if err != nil {
		logger.Error("Firecrawl: failed to create request for query %q: %v", query, err)
		return SearchResponse{Error: fmt.Errorf("firecrawl: failed to create request: %w", err)}
	}

	if fp.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+fp.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := fp.client.Do(req)
	if err != nil {
		logger.Error("Firecrawl: request failed for query %q: %v", query, err)
		return SearchResponse{Error: fmt.Errorf("firecrawl: request failed: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return SearchResponse{Error: fmt.Errorf("firecrawl: failed to read body: %w", err)}
	}

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return SearchResponse{Error: fmt.Errorf("firecrawl: authentication failed. Check your FIRECRAWL_API_KEY")}
	}
	if resp.StatusCode == 402 {
		return SearchResponse{Error: fmt.Errorf("firecrawl: payment required, check your Firecrawl plan or credits")}
	}
	if resp.StatusCode == 429 {
		return SearchResponse{Error: fmt.Errorf("firecrawl: rate limited, try later")}
	}
	if resp.StatusCode != http.StatusOK {
		return SearchResponse{Error: fmt.Errorf("firecrawl: unexpected status %d", resp.StatusCode)}
	}

	var result struct {
		Data struct {
			Web []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"web"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return SearchResponse{Error: fmt.Errorf("firecrawl: failed to parse response: %w", err)}
	}

	results := make([]SearchResult, 0, len(result.Data.Web))
	for _, r := range result.Data.Web {
		results = append(results, SearchResult{
			Title:   cleanSnippet(r.Title),
			URL:     r.URL,
			Snippet: cleanSnippet(r.Description),
		})
	}

	return SearchResponse{
		Results:  results,
		Provider: "firecrawl",
	}
}
