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
	outcrawlDefaultBaseURL = "https://api.outcrawl.ai"
	// outcrawlMaxResults is the most hits the Outcrawl search API returns per call.
	outcrawlMaxResults = 20
)

// OutcrawlProvider adapts the Outcrawl Search API (v1) to WebSearchProvider.
type OutcrawlProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewOutcrawlProvider creates an OutcrawlProvider. An empty baseURL selects the
// hosted API.
func NewOutcrawlProvider(apiKey, baseURL string, hc *http.Client, timeoutSeconds int) *OutcrawlProvider {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = outcrawlDefaultBaseURL
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
	return &OutcrawlProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  hc,
	}
}

func (op *OutcrawlProvider) Name() string {
	return "outcrawl"
}

func (op *OutcrawlProvider) Search(ctx context.Context, query string, maxResults int) SearchResponse {
	logger.Debug("Outcrawl search: %s (maxResults=%d)", query, maxResults)
	if maxResults <= 0 {
		maxResults = 5
	}
	if maxResults > outcrawlMaxResults {
		maxResults = outcrawlMaxResults
	}

	reqBody, err := json.Marshal(map[string]any{
		"query": query,
		"limit": maxResults,
	})
	if err != nil {
		return SearchResponse{Error: fmt.Errorf("outcrawl: failed to encode request: %w", err)}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, op.baseURL+"/v1/search", bytes.NewReader(reqBody))
	if err != nil {
		logger.Error("Outcrawl: failed to create request for query %q: %v", query, err)
		return SearchResponse{Error: fmt.Errorf("outcrawl: failed to create request: %w", err)}
	}

	req.Header.Set("Authorization", "Bearer "+op.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := op.client.Do(req)
	if err != nil {
		logger.Error("Outcrawl: request failed for query %q: %v", query, err)
		return SearchResponse{Error: fmt.Errorf("outcrawl: request failed: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return SearchResponse{Error: fmt.Errorf("outcrawl: failed to read body: %w", err)}
	}

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return SearchResponse{Error: fmt.Errorf("outcrawl: authentication failed. Check your OUTCRAWL_API_KEY")}
	}
	if resp.StatusCode == 402 {
		return SearchResponse{Error: fmt.Errorf("outcrawl: out of credits, check your Outcrawl plan or credits")}
	}
	if resp.StatusCode == 429 {
		return SearchResponse{Error: fmt.Errorf("outcrawl: rate limited, try later")}
	}
	if resp.StatusCode != http.StatusOK {
		return SearchResponse{Error: fmt.Errorf("outcrawl: unexpected status %d", resp.StatusCode)}
	}

	var result struct {
		Hits []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Snippet string `json:"snippet"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return SearchResponse{Error: fmt.Errorf("outcrawl: failed to parse response: %w", err)}
	}

	results := make([]SearchResult, 0, len(result.Hits))
	for _, r := range result.Hits {
		results = append(results, SearchResult{
			Title:   cleanSnippet(r.Title),
			URL:     r.URL,
			Snippet: cleanSnippet(r.Snippet),
		})
	}

	return SearchResponse{
		Results:  results,
		Provider: "outcrawl",
	}
}
