package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/alvinunreal/tmuxai/logger"
)

// SearXNGProvider adapts a self-hosted SearXNG instance to WebSearchProvider.
type SearXNGProvider struct {
	baseURL string
	client  *http.Client
}

// NewSearXNGProvider creates a SearXNGProvider. Validates the baseURL.
// WARN-4: Accepts timeoutSeconds from config.
func NewSearXNGProvider(baseURL string, hc *http.Client, timeoutSeconds int) (*SearXNGProvider, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("searxng: base_url must not be empty")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("searxng: invalid base_url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("searxng: base_url must use http or https scheme")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("searxng: base_url must contain a hostname")
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
	return &SearXNGProvider{
		baseURL: baseURL,
		client:  hc,
	}, nil
}

func (sp *SearXNGProvider) Name() string {
	return "searxng"
}

func (sp *SearXNGProvider) Search(ctx context.Context, query string, maxResults int) SearchResponse {
	logger.Debug("SearXNG search: %s (maxResults=%d)", query, maxResults)
	params := url.Values{}
	params.Set("q", query)
	params.Set("format", "json")
	// FIX 9: Pass maxResults to SearXNG
	if maxResults > 0 {
		params.Set("num_results", fmt.Sprintf("%d", maxResults))
	}

	reqURL := sp.baseURL + "/search?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		logger.Error("SearXNG: failed to create request for query %q: %v", query, err)
		return SearchResponse{Error: fmt.Errorf("searxng: failed to create request: %w", err)}
	}

	req.Header.Set("Accept", "application/json")

	resp, err := sp.client.Do(req)
	if err != nil {
		logger.Error("SearXNG: request failed for query %q: %v", query, err)
		return SearchResponse{Error: fmt.Errorf("searxng: request failed: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return SearchResponse{Error: fmt.Errorf("searxng: failed to read body: %w", err)}
	}

	if resp.StatusCode != http.StatusOK {
		return SearchResponse{Error: fmt.Errorf("searxng: unexpected status %d", resp.StatusCode)}
	}

	var result struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return SearchResponse{Error: fmt.Errorf("searxng: failed to parse response: %w", err)}
	}

	results := make([]SearchResult, 0, len(result.Results))
	for _, r := range result.Results {
		results = append(results, SearchResult{
			Title:   cleanSnippet(r.Title), // WARN-1: clean title too
			URL:     r.URL,
			Snippet: cleanSnippet(r.Content),
		})
	}

	return SearchResponse{
		Results:  results,
		Provider: "searxng",
	}
}
