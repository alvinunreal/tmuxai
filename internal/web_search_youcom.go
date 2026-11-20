package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/alvinunreal/tmuxai/logger"
)

const (
	// youcomKeylessURL needs no account or API key (about 100 searches a day).
	youcomKeylessURL = "https://api.you.com/v1/agents/search"
	// youcomKeyedURL is used when an API key is configured.
	youcomKeyedURL = "https://ydc-index.io/v1/search"
	// youcomKeylessMaxResults is the most results the keyless endpoint returns.
	youcomKeylessMaxResults = 50
	youcomUserAgent         = "youdotcom-integration/alvinunreal-tmuxai"
)

// YouComProvider adapts the You.com Search API to WebSearchProvider. Without an
// API key it uses the keyless endpoint; with one (api_key, or YDC_API_KEY in the
// environment) it uses the keyed endpoint.
type YouComProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewYouComProvider creates a YouComProvider. apiKey is optional; when empty,
// YDC_API_KEY is read from the environment. A non-empty baseURL overrides the
// endpoint (used for tests and proxies).
func NewYouComProvider(apiKey, baseURL string, hc *http.Client, timeoutSeconds int) *YouComProvider {
	if apiKey == "" {
		apiKey = os.Getenv("YDC_API_KEY")
	}
	baseURL = strings.TrimRight(baseURL, "/")
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
	return &YouComProvider{apiKey: apiKey, baseURL: baseURL, client: hc}
}

func (yp *YouComProvider) Name() string {
	return "youcom"
}

func (yp *YouComProvider) Search(ctx context.Context, query string, maxResults int) SearchResponse {
	logger.Debug("You.com search: %s (maxResults=%d)", query, maxResults)
	if maxResults <= 0 {
		maxResults = 5
	}

	var (
		req *http.Request
		err error
	)
	if yp.apiKey != "" {
		endpoint := yp.baseURL
		if endpoint == "" {
			endpoint = youcomKeyedURL
		}
		payload, merr := json.Marshal(map[string]any{"query": query, "count": maxResults})
		if merr != nil {
			return SearchResponse{Error: fmt.Errorf("youcom: failed to encode request: %w", merr)}
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-API-Key", yp.apiKey)
		}
	} else {
		endpoint := yp.baseURL
		if endpoint == "" {
			endpoint = youcomKeylessURL
		}
		if maxResults > youcomKeylessMaxResults {
			maxResults = youcomKeylessMaxResults
		}
		params := url.Values{}
		params.Set("query", query)
		params.Set("count", fmt.Sprintf("%d", maxResults))
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	}
	if err != nil {
		logger.Error("You.com: failed to create request for query %q: %v", query, err)
		return SearchResponse{Error: fmt.Errorf("youcom: failed to create request: %w", err)}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", youcomUserAgent)

	resp, err := yp.client.Do(req)
	if err != nil {
		logger.Error("You.com: request failed for query %q: %v", query, err)
		return SearchResponse{Error: fmt.Errorf("youcom: request failed: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return SearchResponse{Error: fmt.Errorf("youcom: failed to read body: %w", err)}
	}

	switch {
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return SearchResponse{Error: fmt.Errorf("youcom: authentication failed. Check your YDC_API_KEY, or remove it to use the keyless tier")}
	case resp.StatusCode == 429:
		return SearchResponse{Error: fmt.Errorf("youcom: rate limited, try later")}
	case resp.StatusCode != http.StatusOK:
		return SearchResponse{Error: fmt.Errorf("youcom: unexpected status %d", resp.StatusCode)}
	}

	var result struct {
		Results struct {
			Web []struct {
				Title       string   `json:"title"`
				URL         string   `json:"url"`
				Description string   `json:"description"`
				Snippets    []string `json:"snippets"`
			} `json:"web"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return SearchResponse{Error: fmt.Errorf("youcom: failed to parse response: %w", err)}
	}

	results := make([]SearchResult, 0, len(result.Results.Web))
	for _, r := range result.Results.Web {
		if r.URL == "" {
			continue
		}
		snippet := strings.Join(r.Snippets, " ")
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
	return SearchResponse{Results: results, Provider: "youcom"}
}
