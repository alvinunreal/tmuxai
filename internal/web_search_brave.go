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

// BraveProvider adapts the Brave Search API to WebSearchProvider.
type BraveProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewBraveProvider creates a BraveProvider.
// WARN-4: Accepts timeoutSeconds from config to wire it into the HTTP client.
func NewBraveProvider(apiKey, baseURL string, hc *http.Client, timeoutSeconds int) *BraveProvider {
	if baseURL == "" {
		baseURL = "https://api.search.brave.com/res/v1/web/search"
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
	return &BraveProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  hc,
	}
}

func (bp *BraveProvider) Name() string {
	return "brave"
}

func (bp *BraveProvider) Search(ctx context.Context, query string, maxResults int) SearchResponse {
	logger.Debug("Brave search: %s (maxResults=%d)", query, maxResults)
	params := url.Values{}
	params.Set("q", query)
	params.Set("count", fmt.Sprintf("%d", maxResults))
	params.Set("search_lang", "en")

	reqURL := bp.baseURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		logger.Error("Brave: failed to create request for query %q: %v", query, err)
		return SearchResponse{Error: fmt.Errorf("brave: failed to create request: %w", err)}
	}

	req.Header.Set("X-Subscription-Token", bp.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := bp.client.Do(req)
	if err != nil {
		logger.Error("Brave: request failed for query %q: %v", query, err)
		return SearchResponse{Error: fmt.Errorf("brave: request failed: %w", err)}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return SearchResponse{Error: fmt.Errorf("brave: failed to read body: %w", err)}
	}

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return SearchResponse{Error: fmt.Errorf("brave: authentication failed. Check your BRAVE_API_KEY")}
	}
	if resp.StatusCode == 429 {
		return SearchResponse{Error: fmt.Errorf("brave: rate limited, try later")}
	}
	if resp.StatusCode != http.StatusOK {
		return SearchResponse{Error: fmt.Errorf("brave: unexpected status %d", resp.StatusCode)}
	}

	var result struct {
		Web struct {
			Results []struct {
				Title   string `json:"title"`
				URL     string `json:"url"`
				Snippet string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return SearchResponse{Error: fmt.Errorf("brave: failed to parse response: %w", err)}
	}

	results := make([]SearchResult, 0, len(result.Web.Results))
	for _, r := range result.Web.Results {
		results = append(results, SearchResult{
			Title:   cleanSnippet(r.Title), // WARN-1: clean title too
			URL:     r.URL,
			Snippet: cleanSnippet(r.Snippet),
		})
	}

	return SearchResponse{
		Results:  results,
		Provider: "brave",
	}
}
