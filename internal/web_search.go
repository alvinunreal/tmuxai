package internal

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alvinunreal/tmuxai/logger"
)

var htmlTagRegex = regexp.MustCompile(`<[^>]+>`)

// cleanSnippet strips HTML tags and unescapes HTML entities from a string.
// WARN-1: Used for both Title and Snippet fields — applies html.UnescapeString consistently.
func cleanSnippet(s string) string {
	s = htmlTagRegex.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(s)
}

// SearchResult represents a single search hit.
type SearchResult struct {
	Title   string
	URL     string
	Snippet string
}

// SearchResponse is what a provider returns.
type SearchResponse struct {
	Results  []SearchResult
	Provider string // which provider answered (for logging/metrics)
	Error    error  // nil on success
}

// WebSearchProvider is the adapter interface. Implementations are stateless
// after construction — config is injected at init, not per-call.
type WebSearchProvider interface {
	// Name returns the provider identifier (e.g. "brave", "searxng").
	Name() string

	// Search executes a query and returns up to maxResults items.
	// Returns a non-nil SearchResponse with Error set on failure.
	Search(ctx context.Context, query string, maxResults int) SearchResponse
}

// WebFetchResponse contains the result of a web fetch operation.
type WebFetchResponse struct {
	Title    string
	URL      string
	Content  string // extracted readable text
	Provider string // always "builtin" or provider name
	Error    error
}

// SearchEngine is the facade that sits between providers and the rest
// of the app. It handles provider selection, failover, and budget tracking.
type SearchEngine struct {
	providers  []WebSearchProvider // ordered: primary first
	maxResults int                 // default max results per query
	maxChars   int                 // character budget for result injection
}

// NewSearchEngine creates a new SearchEngine.
func NewSearchEngine(providers []WebSearchProvider, maxResults, maxChars int) *SearchEngine {
	if maxResults <= 0 {
		maxResults = 5
	}
	if maxChars <= 0 {
		maxChars = 4000
	}
	engines := make([]string, 0, len(providers))
	for _, p := range providers {
		engines = append(engines, p.Name())
	}
	logger.Info("Created search engine with %d provider(s): %v", len(providers), engines)
	return &SearchEngine{
		providers:  providers,
		maxResults: maxResults,
		maxChars:   maxChars,
	}
}

// Search queries the primary provider first, falls back to secondary on error.
func (se *SearchEngine) Search(ctx context.Context, query string) SearchResponse {
	start := time.Now()
	logger.Debug("Starting web search: %s", query)

	for _, provider := range se.providers {
		resp := provider.Search(ctx, query, se.maxResults)
		if resp.Error != nil {
			continue
		}
		duration := time.Since(start)
		logger.Debug("Search results from %s: %d results in %v", resp.Provider, len(resp.Results), duration)
		// Apply character budget
		se.enforceCharBudget(&resp)
		return resp
	}
	logger.Error("All search providers failed for query: %s", query)
	return SearchResponse{
		Results: []SearchResult{},
		Error:   fmt.Errorf("all search providers unavailable"),
	}
}

// enforceCharBudget trims results from the bottom until total text fits within maxChars.
// CRIT-5: Consistent budget calculation using raw rune lengths (no magic overhead).
func (se *SearchEngine) enforceCharBudget(resp *SearchResponse) {
	totalChars := 0
	for _, r := range resp.Results {
		totalChars += utf8.RuneCountInString(r.Title) + utf8.RuneCountInString(r.URL) + utf8.RuneCountInString(r.Snippet)
	}
	if totalChars <= se.maxChars {
		return
	}
	for len(resp.Results) > 0 {
		last := resp.Results[len(resp.Results)-1]
		resp.Results = resp.Results[:len(resp.Results)-1]
		totalChars -= utf8.RuneCountInString(last.Title) + utf8.RuneCountInString(last.URL) + utf8.RuneCountInString(last.Snippet)
		if totalChars <= se.maxChars {
			break
		}
	}
}
