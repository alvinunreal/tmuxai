package internal

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	htmltomd "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/mackee/go-readability"

	"github.com/alvinunreal/tmuxai/logger"
)

const (
	maxResponseBytes      = 524288 // 512KB
	defaultMaxChars       = 25000
	userAgent             = "TmuxAI/1.0"
	maxRedirects          = 5
	minReadabilityContent = 200 // Minimum content length for readability to be considered valid
)

// WARN-1: Precompile regexes used in hot paths at package level.
var (
	titleRe          = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	oh1Re            = regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>`)
	ogRe             = regexp.MustCompile(`(?is)<meta[^>]+property\s*=\s*"og:title"[^>]+content\s*=\s*"([^"]*)"`)
	ogRe2            = regexp.MustCompile(`(?is)<meta[^>]+content\s*=\s*"([^"]*)"[^>]+property\s*=\s*"og:title"`)
	fetchStripHTMLRx = regexp.MustCompile(`<[^>]*>`)
)

// webFetcher handles URL content extraction.
type webFetcher struct {
	httpClient       *http.Client
	maxChars         int
	timeoutSeconds   int
	allowedRedirects bool
}

// newWebFetcher creates a webFetcher instance.
func newWebFetcher(hc *http.Client, maxChars, timeoutSeconds int, allowedRedirects bool) *webFetcher {
	if maxChars <= 0 {
		maxChars = defaultMaxChars
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 8
	}
	if hc == nil {
		hc = &http.Client{
			Transport: secureHTTPTransport(timeoutSeconds),
			Timeout:   time.Duration(timeoutSeconds) * time.Second,
		}
	} else {
		// FIX: Shallow copy to avoid mutating the caller's client
		hc = &http.Client{
			Transport:     secureRoundTripper(hc.Transport, timeoutSeconds),
			CheckRedirect: hc.CheckRedirect,
			Jar:           hc.Jar,
			Timeout:       hc.Timeout,
		}
	}
	hc.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &webFetcher{
		httpClient:       hc,
		maxChars:         maxChars,
		timeoutSeconds:   timeoutSeconds,
		allowedRedirects: allowedRedirects,
	}
}

// WebFetch extracts readable content from a URL.
// FIX 5: Added context.Context as first parameter.
func WebFetch(ctx context.Context, rawURL string, maxChars int, timeoutSeconds int, allowedRedirects bool) WebFetchResponse {
	start := time.Now()
	logger.Debug("Fetching URL: %s (max_chars=%d, timeout=%ds, redirects=%v)", rawURL, maxChars, timeoutSeconds, allowedRedirects)

	fetcher := newWebFetcher(nil, maxChars, timeoutSeconds, allowedRedirects)
	result := fetcher.fetch(ctx, rawURL)

	duration := time.Since(start)
	if result.Error != nil {
		logger.Error("Fetch failed for %s: %v (%v)", rawURL, result.Error, duration)
	} else {
		logger.Debug("Fetch succeeded for %s: %d chars from %s in %v", rawURL, utf8.RuneCountInString(result.Content), result.Provider, duration)
	}
	return result
}

func (wf *webFetcher) fetch(ctx context.Context, rawURL string) WebFetchResponse {
	u, err := wf.normalizeURL(rawURL)
	if err != nil {
		return WebFetchResponse{Error: err}
	}

	// SSRF guard on initial URL
	if err := wf.checkSSRF(u); err != nil {
		return WebFetchResponse{Error: err}
	}

	return wf.fetchURL(ctx, u, maxRedirects)
}

func (wf *webFetcher) fetchURL(ctx context.Context, u *url.URL, redirectsLeft int) WebFetchResponse {
	return wf.doFetchURL(ctx, u, redirectsLeft, make(map[string]bool))
}

// doFetchURL handles the actual fetching with redirect deduplication (WARN-5)
// and title extraction from raw bytes (WARN-2).
func (wf *webFetcher) doFetchURL(ctx context.Context, u *url.URL, redirectsLeft int, visited map[string]bool) WebFetchResponse {
	// WARN-5: Deduplicate visited URLs to catch circular redirect loops
	if visited[u.String()] {
		return WebFetchResponse{Error: fmt.Errorf("redirect blocked: circular loop detected (%s)", u.String())}
	}
	visited[u.String()] = true

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return WebFetchResponse{Error: fmt.Errorf("failed to create request: %w", err)}
	}

	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain,application/json,*/*")
	req.Header.Set("User-Agent", userAgent)

	resp, err := wf.httpClient.Do(req)
	if err != nil {
		return WebFetchResponse{Error: fmt.Errorf("request failed: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle redirects
	if wf.allowedRedirects && resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		if location != "" {
			// FIX 3: Enforce redirect hop limit
			if redirectsLeft <= 0 {
				return WebFetchResponse{Error: fmt.Errorf("too many redirects (max %d)", maxRedirects)}
			}

			redirectURL, err := url.Parse(location)
			if err == nil {
				redirectURL = u.ResolveReference(redirectURL)

				// FIX 2: Re-validate scheme on redirect target
				scheme := strings.ToLower(redirectURL.Scheme)
				if scheme != "http" && scheme != "https" {
					return WebFetchResponse{Error: fmt.Errorf("redirect blocked: unsupported scheme (%s)", redirectURL.Scheme)}
				}

				// CRIT-2: Re-resolve DNS after redirect to defend against DNS rebinding.
				// The domain may resolve differently after the redirect header is served.
				if err := wf.checkSSRF(redirectURL); err != nil {
					return WebFetchResponse{Error: err}
				}
				return wf.doFetchURL(ctx, redirectURL, redirectsLeft-1, visited)
			}
		}
	}

	// WARN-2: Read body into bytes for title extraction BEFORE conversion
	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return WebFetchResponse{Error: fmt.Errorf("failed to read body: %w", err)}
	}

	// WARN-2: Extract title from raw HTML before htmltomd consumes it
	title := extractTitleFromHTML(rawBody)

	contentType := sanitizeContentType(resp.Header.Get("Content-Type"))
	content, readabilityTitle, err := wf.extractBodyBytes(rawBody, contentType)
	if err != nil {
		return WebFetchResponse{Error: err}
	}

	// Use readability's title if our extractTitleFromHTML returned empty
	if title == "" && readabilityTitle != "" {
		title = readabilityTitle
	}

	return WebFetchResponse{
		Title:    title,
		URL:      u.String(),
		Content:  content,
		Provider: "builtin",
	}
}

func (wf *webFetcher) normalizeURL(rawURL string) (*url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)

	// Check rejected schemes first
	lower := strings.ToLower(rawURL)
	if strings.HasPrefix(lower, "file://") || strings.HasPrefix(lower, "ftp://") || strings.HasPrefix(lower, "data:") {
		return nil, fmt.Errorf("unsupported URL scheme: %s", rawURL)
	}

	// Auto-prepend https:// for bare hostnames
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme: %s", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid URL: missing hostname")
	}

	return u, nil
}

// FIX 1: SSRF guard — block ALL private IPs with ZERO exceptions.
func (wf *webFetcher) checkSSRF(u *url.URL) error {
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("blocked: private network access denied")
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("DNS resolution failed: %w", err)
	}

	for _, ip := range ips {
		// FIX 1: Unwrap IPv4-mapped IPv6 (::ffff:x.x.x.x) → IPv4
		if mapped := ip.To4(); mapped != nil {
			ip = mapped
		}
		if isBlockedIP(ip) {
			return fmt.Errorf("blocked: private network access denied")
		}
	}
	return nil
}

func secureRoundTripper(rt http.RoundTripper, timeoutSeconds int) http.RoundTripper {
	if tr, ok := rt.(*http.Transport); ok {
		cloned := tr.Clone()
		cloned.Proxy = nil
		cloned.DialContext = ssrfSafeDialContext(timeoutSeconds)
		//nolint:staticcheck // Clear deprecated TLS dial hook so it cannot bypass DialContext.
		cloned.DialTLS = nil
		cloned.DialTLSContext = nil
		return cloned
	}
	if rt == nil {
		return secureHTTPTransport(timeoutSeconds)
	}
	// A custom RoundTripper cannot be hardened safely; fail closed by replacing it.
	return secureHTTPTransport(timeoutSeconds)
}

func secureHTTPTransport(timeoutSeconds int) *http.Transport {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	tr.DialContext = ssrfSafeDialContext(timeoutSeconds)
	//nolint:staticcheck // Clear deprecated TLS dial hook so it cannot bypass DialContext.
	tr.DialTLS = nil
	tr.DialTLSContext = nil
	return tr
}

func ssrfSafeDialContext(timeoutSeconds int) func(context.Context, string, string) (net.Conn, error) {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeoutSeconds <= 0 {
		timeout = 8 * time.Second
	}
	resolver := net.DefaultResolver
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid dial address: %w", err)
		}

		ips, err := resolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("DNS resolution failed: %w", err)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("DNS resolution failed: no addresses")
		}

		for _, ipAddr := range ips {
			ip := ipAddr.IP
			if mapped := ip.To4(); mapped != nil {
				ip = mapped
			}
			if isBlockedIP(ip) {
				return nil, fmt.Errorf("blocked: private network access denied")
			}
		}

		var lastErr error
		for _, ipAddr := range ips {
			ip := ipAddr.IP
			if mapped := ip.To4(); mapped != nil {
				ip = mapped
			}
			dialer := &net.Dialer{Timeout: timeout}
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
}

// isBlockedIP returns true for all private, loopback, unspecified,
// link-local, and multicast addresses. No exceptions.
// CRIT-2: Explicit IPv6 ULA (fd00::/8) and link-local (fe80::/10) CIDRs added.
func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}
	if ip.IsPrivate() {
		return true
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip.IsUnspecified() {
		return true
	}
	if ip.IsMulticast() {
		return true
	}
	// CRIT-2: Explicit IPv6 ULA (fd00::/8) block
	ulaNet := net.IPNet{IP: net.ParseIP("fd00::"), Mask: net.CIDRMask(8, 128)}
	if ulaNet.Contains(ip) {
		return true
	}
	// CRIT-2: Explicit IPv6 link-local (fe80::/10) block
	fe80Net := net.IPNet{IP: net.ParseIP("fe80::"), Mask: net.CIDRMask(10, 128)}
	return fe80Net.Contains(ip)
}

// sanitizeContentType extracts the media type from Content-Type.
func sanitizeContentType(ct string) string {
	ct = strings.TrimSpace(ct)
	idx := strings.Index(ct, ";")
	if idx != -1 {
		ct = ct[:idx]
	}
	return strings.TrimSpace(ct)
}

// extractTitleFromHTML parses <title> from raw HTML bytes, falling back to
// og:title, then <h1> if neither exists.
func extractTitleFromHTML(body []byte) string {
	s := string(body)

	// Try <title>...</title>
	if m := titleRe.FindStringSubmatch(s); m != nil {
		return stripHTMLTags(m[1])
	}

	// Fallback: <meta property="og:title" content="...">
	if m := ogRe.FindStringSubmatch(s); m != nil {
		return stripHTMLTags(m[1])
	}
	// Alternate attribute order: content before property
	if m := ogRe2.FindStringSubmatch(s); m != nil {
		return stripHTMLTags(m[1])
	}

	// Third fallback: <h1>...</h1> (useful when no <title> tag exists)
	if m := oh1Re.FindStringSubmatch(s); m != nil {
		return stripHTMLTags(m[1])
	}

	return ""
}

// stripHTMLTags removes all HTML tags from a string.
func stripHTMLTags(s string) string {
	return strings.TrimSpace(fetchStripHTMLRx.ReplaceAllString(s, ""))
}

// extractBodyBytes converts raw bytes to readable content based on content type.
// It returns the content, any readability-discovered title (if our extractTitleFromHTML
// returned empty), and an error. For text/html it tries go-readability first;
// if readability succeeds with meaningful content, it uses readability.ToMarkdown();
// otherwise it falls back to htmltomd.
func (wf *webFetcher) extractBodyBytes(body []byte, contentType string) (content string, readabilityTitle string, err error) {
	ct := strings.ToLower(contentType)

	if strings.Contains(ct, "text/html") {
		htmlStr := string(body)

		// Try go-readability first (primary path for HTML)
		article, errRead := readability.Extract(htmlStr, readability.DefaultOptions())
		if errRead == nil && article.Root != nil {
			md := readability.ToMarkdown(article.Root)
			if len(md) >= minReadabilityContent {
				// Readability succeeded with meaningful content — use it.
				truncated, wasTruncated := truncateString(md, wf.maxChars)
				if wasTruncated {
					truncated += "\n\n[...]truncated"
				}
				return truncated, article.Title, nil
			}
			// Readability returned too little content (<200 chars); fall through to htmltomd.
			logger.Debug("Readability returned minimal content (%d chars), falling back to htmltomd", len(md))
		}
		// Readability errored or returned nil root — fall through to htmltomd.
		if errRead != nil {
			logger.Debug("Readability extraction failed (%v), falling back to htmltomd", errRead)
		}

		// Fallback: htmltomd
		md, errConvert := htmltomd.ConvertString(htmlStr)
		if errConvert != nil {
			return "", "", fmt.Errorf("html-to-markdown conversion failed: %w", errConvert)
		}
		truncated, wasTruncated := truncateString(md, wf.maxChars)
		if wasTruncated {
			truncated += "\n\n[...]truncated"
		}
		return truncated, "", nil
	}

	if strings.Contains(ct, "text/plain") {
		truncated, wasTruncated := truncateString(string(body), wf.maxChars)
		if wasTruncated {
			truncated += "\n\n[...]truncated"
		}
		return truncated, "", nil
	}

	if strings.Contains(ct, "application/json") {
		truncated, wasTruncated := truncateString(string(body), wf.maxChars)
		if wasTruncated {
			truncated += "\n\n[...]truncated"
		}
		return truncated, "", nil
	}

	if strings.Contains(ct, "application/pdf") {
		return "", "", fmt.Errorf("cannot fetch: unsupported content type (PDF)")
	}

	// application/octet-stream is often a misconfigured server serving HTML/text.
	// Attempt HTML-to-markdown conversion; if the body looks like text, it'll work.
	if strings.Contains(ct, "application/octet-stream") {
		md, err := htmltomd.ConvertString(string(body))
		if err == nil {
			truncated, wasTruncated := truncateString(md, wf.maxChars)
			if wasTruncated {
				truncated += "\n\n[...]truncated"
			}
			return truncated, "", nil
		}
	}

	return "", "", fmt.Errorf("cannot fetch: unsupported content type (%s)", contentType)
}

func truncateString(s string, maxChars int) (string, bool) {
	marker := "\n\n[...]truncated"
	markerLen := utf8.RuneCountInString(marker)

	rs := []rune(s)
	if len(rs) <= maxChars {
		return s, false
	}

	effectiveMax := maxChars - markerLen
	if effectiveMax <= 0 {
		effectiveMax = 1
	}
	return string(rs[:effectiveMax]), true
}

// FetchResult captures a single fetch attempt's output.
type FetchResult struct {
	Content string
	Source  string // "direct" or "wayback"
	URL     string // the URL actually fetched (may differ from input)
}

// needsFallback returns true if content is too short/useless to be meaningful.
// Thresholds:
//   - 0 chars: definitely empty (JS-rendered page returned nothing)
//   - <80 chars: likely a redirect stub or empty shell
//   - <300 chars AND contains known junk patterns: soft-404
func needsFallback(content string) bool {
	trimmed := strings.TrimSpace(content)
	runeLen := utf8.RuneCountInString(trimmed)

	// Zero-length: empty JS-rendered page
	if runeLen == 0 {
		return true
	}

	// Very short: redirect stub, empty shell
	if runeLen < 80 {
		return true
	}

	// Moderately short + junk patterns: soft-404
	if runeLen < 300 {
		lower := strings.ToLower(trimmed)
		junkPatterns := []string{
			"page not found",
			"404",
			"could not be found",
			"this page has moved",
			"url not found",
			"access denied",
			"login required",
			"please sign in",
			"sign in to continue",
			"this site requires javascript",
			"enable javascript",
			"javascript is disabled",
		}
		for _, pat := range junkPatterns {
			if strings.Contains(lower, pat) {
				return true
			}
		}
	}

	return false
}

// waybackURL returns the Wayback Machine archived URL for the given page.
func waybackURL(rawURL string) string {
	stripped := rawURL
	stripped = strings.TrimPrefix(stripped, "https://")
	stripped = strings.TrimPrefix(stripped, "http://")
	return fmt.Sprintf("https://web.archive.org/web/2/%s", stripped)
}

// FetchWithFallbacks tries direct fetch, then Wayback Machine.
// Returns the first result with useful content, or the last attempt if all fail.
func FetchWithFallbacks(ctx context.Context, rawURL string, maxChars, timeoutSeconds int, allowedRedirects bool) FetchResult {
	if maxChars <= 0 {
		maxChars = defaultMaxChars
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 8
	}

	logger.Debug("Fetching with fallbacks for: %s (max_chars=%d, timeout=%ds)", rawURL, maxChars, timeoutSeconds)

	// WithoutCancel strips any existing deadline from ctx so each attempt
	// gets its own full timeoutSeconds budget, avoiding starvation when
	// earlier attempts consume the parent's deadline.
	baseCtx := context.WithoutCancel(ctx)

	// --- Attempt 1: Direct fetch ---
	ctx1, cancel1 := context.WithTimeout(baseCtx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel1()
	result := WebFetch(ctx1, rawURL, maxChars, timeoutSeconds, allowedRedirects)
	if result.Error == nil && !needsFallback(result.Content) {
		return FetchResult{
			Content: result.Content,
			Source:  "direct",
			URL:     rawURL,
		}
	}
	// Direct fetch failed or returned minimal content — enter fallback mode.

	// Check for cancellation before proceeding to fallbacks.
	if ctx.Err() != nil {
		return FetchResult{Content: "", Source: "", URL: rawURL}
	}

	// --- Attempt 2: Wayback Machine ---
	wbURL := waybackURL(rawURL)
	logger.Debug("Wayback fallback URL: %s", wbURL)
	ctx2, cancel2 := context.WithTimeout(baseCtx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel2()
	wbResult := WebFetch(ctx2, wbURL, maxChars, timeoutSeconds, false)
	if wbResult.Error == nil && !needsFallback(wbResult.Content) {
		infoMsg := "  -> Direct fetch failed, using fallback: wayback"
		logger.Info(infoMsg)
		fmt.Fprintln(os.Stderr, infoMsg)
		return FetchResult{
			Content: wbResult.Content,
			Source:  "wayback",
			URL:     wbURL,
		}
	}

	// All fallbacks exhausted
	logger.Info("All fallbacks exhausted for %s", rawURL)
	fmt.Fprintln(os.Stderr, "  -> Direct fetch failed, all fallbacks exhausted")
	best := result.Content
	if utf8.RuneCountInString(wbResult.Content) > utf8.RuneCountInString(best) {
		best = wbResult.Content
	}
	return FetchResult{
		Content: best,
		Source:  "",
		URL:     rawURL,
	}
}
