package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alvinunreal/tmuxai/logger"
)

const (
	specstreamSearchURL = "https://api.specstream.ai/v1/web-search"
	// specstreamMaxResults is the most results one call returns.
	specstreamMaxResults = 100
	specstreamUserAgent  = "tmuxai"
)

// SpecStreamProvider adapts the SpecStream Search API to WebSearchProvider.
// Each call runs the fast tier with passages, so every result carries the
// lines of its page that answer the query instead of only the page summary.
type SpecStreamProvider struct {
	apiKey   string
	endpoint string
	client   *http.Client
}

// NewSpecStreamProvider creates a SpecStreamProvider. apiKey falls back to
// SPECSTREAM_API_KEY from the environment. A non-empty baseURL replaces the
// search endpoint (used for tests and proxies).
func NewSpecStreamProvider(apiKey, baseURL string, hc *http.Client, timeoutSeconds int) *SpecStreamProvider {
	if apiKey == "" {
		apiKey = os.Getenv("SPECSTREAM_API_KEY")
	}
	endpoint := strings.TrimRight(baseURL, "/")
	if endpoint == "" {
		endpoint = specstreamSearchURL
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
	// Never follow redirects, so the API key is only sent to the configured endpoint.
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &SpecStreamProvider{apiKey: apiKey, endpoint: endpoint, client: hc}
}

func (sp *SpecStreamProvider) Name() string {
	return "specstream"
}

func (sp *SpecStreamProvider) Search(ctx context.Context, query string, maxResults int) SearchResponse {
	logger.Debug("SpecStream search: %s (maxResults=%d)", query, maxResults)
	if sp.apiKey == "" {
		return SearchResponse{Error: fmt.Errorf("specstream: no API key. Set api_key under web_search.providers.specstream or SPECSTREAM_API_KEY")}
	}
	if maxResults <= 0 {
		maxResults = 5
	}
	if maxResults > specstreamMaxResults {
		maxResults = specstreamMaxResults
	}

	params := url.Values{}
	params.Set("q", query)
	params.Set("num", strconv.Itoa(maxResults))
	params.Set("speed", "fast")
	params.Set("passages", "true")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sp.endpoint+"?"+params.Encode(), nil)
	if err != nil {
		logger.Error("SpecStream: failed to create request for query %q: %v", query, err)
		return SearchResponse{Error: fmt.Errorf("specstream: failed to create request: %w", err)}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+sp.apiKey)
	req.Header.Set("User-Agent", specstreamUserAgent)

	resp, err := sp.client.Do(req)
	if err != nil {
		logger.Error("SpecStream: request failed for query %q: %v", query, err)
		return SearchResponse{Error: fmt.Errorf("specstream: request failed: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return SearchResponse{Error: fmt.Errorf("specstream: failed to read body: %w", err)}
	}

	var result struct {
		Data struct {
			Sources []struct {
				Title      string   `json:"title"`
				URL        string   `json:"url"`
				Snippet    string   `json:"snippet"`
				Highlights []string `json:"highlights"`
			} `json:"sources"`
		} `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	parseErr := json.Unmarshal(body, &result)

	if resp.StatusCode != http.StatusOK {
		detail := ""
		if parseErr == nil && result.Error != nil && result.Error.Message != "" {
			detail = ": " + result.Error.Message
		}
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return SearchResponse{Error: fmt.Errorf("specstream: authentication failed. Check your SPECSTREAM_API_KEY%s", detail)}
		case http.StatusPaymentRequired:
			return SearchResponse{Error: fmt.Errorf("specstream: the account has no search credit left%s", detail)}
		case http.StatusTooManyRequests:
			return SearchResponse{Error: fmt.Errorf("specstream: rate limited, try later")}
		default:
			return SearchResponse{Error: fmt.Errorf("specstream: unexpected status %d%s", resp.StatusCode, detail)}
		}
	}
	if parseErr != nil {
		return SearchResponse{Error: fmt.Errorf("specstream: failed to parse response: %w", parseErr)}
	}

	results := make([]SearchResult, 0, len(result.Data.Sources))
	for _, s := range result.Data.Sources {
		if s.URL == "" {
			continue
		}
		snippet := strings.Join(s.Highlights, " ")
		if strings.TrimSpace(snippet) == "" {
			snippet = s.Snippet
		}
		results = append(results, SearchResult{
			Title:   cleanSnippet(s.Title),
			URL:     s.URL,
			Snippet: cleanSnippet(snippet),
		})
		if len(results) >= maxResults {
			break
		}
	}
	return SearchResponse{Results: results, Provider: "specstream"}
}
