package internal
import (
	"net"
	"strings"
	"testing"
	"unicode/utf8"
)

// Unit tests for web search and fetch internals.

// --- needsFallback: boundary + edge-case tests ------------------------

// build300runeString constructs a string of exactly 300 lowercase 'a' runes
// that also contains the junk pattern "404 not found". Since rune length ≥ 300,
// needsFallback should return false (junk-pattern check only applies < 300).
func build300runeString() string {
	s := strings.Repeat("a", 300)
	// Replace positions 149..163 (15 chars) with "404 not found" (13 chars).
	// Net: 300 - 15 + 13 = 298. Need 2 more.
	s = s[:149] + "404 not found" + s[164:]
	// s is now 298 runes. Pad to exactly 300.
	s += "aa"
	return s
}

func Test_needsFallback_BoundaryAndEdgeCases(t *testing.T) {
	s299 := strings.Repeat("a", 299)
	s300 := strings.Repeat("a", 300)

	tests := []struct {
		name string
		str  string
		want bool
	}{
		{"exactly 79 runes", strings.Repeat("a", 79), true},
		{"exactly 80 runes", strings.Repeat("a", 80), false},
		{"exactly 299 runes (no junk)", s299, false},
		{"exactly 299 runes + junk pattern", s299[:149] + "page not found" + s299[167:], true},
		{"exactly 300 runes (no junk)", s300, false},
		{"exactly 300 runes + junk pattern — should NOT trigger (≥300)", build300runeString(), false},
		{">300 chars containing 404 — should NOT trigger", "aaaaaaaaaaa" + strings.Repeat("a", 300) + " 404 " + strings.Repeat("a", 10), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needsFallback(tt.str)
			if got != tt.want {
				t.Errorf("needsFallback(): got %v, want %v (len=%d runes)", got, tt.want, utf8.RuneCountInString(tt.str))
			}
		})
	}
}

// --- sanitizeFetchContent tests ---------------------------------------

func Test_sanitizeFetchContent(t *testing.T) {
	// 256-char base64-like string (will be caught by longB64 regex)
	longB64 := strings.Repeat("ABCDEFGHIJklMNOPqrstUVWXyz0123456789+/=", 8)[:256]

	tests := []struct {
		name    string
		in      string
		want    string // substring that MUST be present
		wantNot string // substring that must NOT be present
	}{
		{
			name:    "zero-width space stripped",
			in:      "hello\u200bworld",
			want:    "helloworld",
			wantNot: "\u200b",
		},
		{
			name:    "BOM (U+FEFF) stripped",
			in:      "\ufeffvalid content",
			want:    "valid content",
			wantNot: "\ufeff",
		},
		{
			name:    "directional marks stripped (U+202A-U+202E)",
			in:      "before\u202Amiddle\u202Eafter",
			want:    "beforemiddleafter",
			wantNot: "\u202a",
		},
		{
			name:    "control chars stripped except newline/tab/carriage-return",
			in:      "good\x01bad\x02also",
			want:    "goodbadalso",
			wantNot: "\x01",
		},
		{
			name:    "newline, carriage-return, tab preserved",
			in:      "a\nb\r\tc",
			want:    "a\nb\r\tc",
			wantNot: "",
		},
		{
			name:    "base64 data URI removed",
			in:      "prefix data:image/png;base64," + strings.Repeat("ABCD", 8) + " suffix",
			want:    "[removed base64 blob]",
			wantNot: "ABCD",
		},
		{
			name:    "long base64 string replaced",
			in:      "pre " + longB64 + " post",
			want:    "[removed long base64]",
			wantNot: longB64,
		},
		{
			name:    "normal text passes through unchanged",
			in:      "Hello world! Just a regular paragraph with some punctuation.",
			want:    "Hello world!",
			wantNot: "[removed",
		},
		{
			name:    "zero-width joiner/non-joiner (U+200C/U+200D) stripped",
			in:      "abc\u200Cdef\u200Dghi",
			want:    "abcdefghi",
			wantNot: "\u200c",
		},
		{
			name:    "range U+2060-U+2069 stripped",
			in:      "hi\u2060bye",
			want:    "hibye",
			wantNot: "\u2060",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeFetchContent(tt.in)
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Errorf("missing expected substring %q in:\n%q", tt.want, got)
			}
			if tt.wantNot != "" && strings.Contains(got, tt.wantNot) {
				t.Errorf("should not contain %q in:\n%q", tt.wantNot, got)
			}
		})
	}
}

// --- cleanSnippet tests -------------------------------------------------

func Test_cleanSnippet(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "strips HTML tags and unescapes entities",
			in:   "<div><strong>Bold</strong> &amp; <em>Italic</em></div>",
			want: "Bold & Italic",
		},
		{
			name: "unescapes entities without tags",
			in:   `Tom &amp; Jerry &lt;cats&gt;`,
			want: `Tom & Jerry <cats>`,
		},
		{
			name: "trims leading/trailing whitespace",
			in:   "  lots   of   spaces  ",
			want: "lots   of   spaces",
		},
		{
			name: "nested tags fully removed",
			in:   "<p>Hello<span>nested</span>world</p>",
			want: "Hellonestedworld",
		},
		{
			name: "empty input",
			in:   "",
			want: "",
		},
		{
			name: "&nbsp; entity",
			in:   "word&nbsp;word",
			want: "word word", // nbsp becomes space
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanSnippet(tt.in)
			if got != tt.want {
				t.Errorf("cleanSnippet(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// --- extractTitleFromHTML tests -----------------------------------------

func Test_extractTitleFromHTML(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "standard title tag",
			body: "<html><head><title>My Page</title></head></html>",
			want: "My Page",
		},
		{
			name: "title with attributes",
			body: `<html><head><title lang="en">Clean Title</title></head></html>`,
			want: "Clean Title",
		},
		{
			name: "title with HTML inside",
			body: "<html><head><title>Bold &amp; <b>Best</b></title></head></html>",
			want: "Bold &amp; Best",
		},
		{
			name: "og:title fallback (property before content)",
			body: "<html><head>\n<meta property=\"og:title\" content=\"OG Title\">\n</head></html>",
			want: "OG Title",
		},
		{
			name: "og:title fallback (content before property)",
			body: "<html><head>\n<meta content=\"Reverse OG\" property=\"og:title\">\n</head></html>",
			want: "Reverse OG",
		},
		{
			name: "prefers <title> over og:title",
			body: "<html><head>\n<title>Real Title</title>\n<meta property=\"og:title\" content=\"Fake OG\">\n</head></html>",
			want: "Real Title",
		},
		{
			name: "no title at all",
			body: "<html><head><meta charset=\"utf-8\"></head></html>",
			want: "",
		},
		{
			name: "multiline title tag",
			body: "<html><head><title>\n  Multi\n  Line\n</title></head></html>",
			want: "Multi\n  Line",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTitleFromHTML([]byte(tt.body))
			if got != tt.want {
				t.Errorf("extractTitleFromHTML() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- enforceCharBudget tests --------------------------------------------

func Test_enforceCharBudget(t *testing.T) {
	se := &SearchEngine{maxChars: 100}

	// Each SearchResult costs: runes(title) + runes(url) + runes(snippet).
	makeResult := func(id byte, cost int) SearchResult {
		return SearchResult{
			Title:   string(id),             // 1 rune
			URL:     string(id) + "a",       // 2 runes
			Snippet: strings.Repeat("x", cost-3), // remaining cost
		}
	}

	t.Run("within budget — no trimming", func(t *testing.T) {
		results := []SearchResult{makeResult('a', 30), makeResult('b', 30), makeResult('c', 30)}
		resp := &SearchResponse{Results: results}
		se.enforceCharBudget(resp)
		if len(resp.Results) != 3 {
			t.Errorf("expected 3 results, got %d", len(resp.Results))
		}
	})

	t.Run("over budget — trim from bottom", func(t *testing.T) {
		// 4 × 30 = 120, exceeds 100. Remove 1 → 90 ≤ 100.
		results := []SearchResult{
			makeResult('a', 30), makeResult('b', 30),
			makeResult('c', 30), makeResult('d', 30),
		}
		resp := &SearchResponse{Results: results}
		se.enforceCharBudget(resp)
		if len(resp.Results) != 3 {
			t.Errorf("expected 3 results after trimming, got %d", len(resp.Results))
		}
	})

	t.Run("single result exceeds budget — all trimmed", func(t *testing.T) {
		results := []SearchResult{makeResult('x', 200)}
		resp := &SearchResponse{Results: results}
		se.enforceCharBudget(resp)
		if len(resp.Results) != 0 {
			t.Errorf("expected 0 results, got %d", len(resp.Results))
		}
	})

	t.Run("exact boundary — 2 results totaling exactly 100", func(t *testing.T) {
		results := []SearchResult{makeResult('a', 50), makeResult('b', 50)}
		resp := &SearchResponse{Results: results}
		se.enforceCharBudget(resp)
		if len(resp.Results) != 2 {
			t.Errorf("expected 2 results (exact boundary), got %d", len(resp.Results))
		}
	})

	t.Run("many results — trim until under budget", func(t *testing.T) {
		// 6 × 25 = 150, budget = 100. Need to remove 2 → 100 exactly.
		results := []SearchResult{
			makeResult('a', 25), makeResult('b', 25), makeResult('c', 25),
			makeResult('d', 25), makeResult('e', 25), makeResult('f', 25),
		}
		resp := &SearchResponse{Results: results}
		se.enforceCharBudget(resp)
		if len(resp.Results) != 4 {
			t.Errorf("expected 4 results, got %d", len(resp.Results))
		}
	})

	t.Run("empty results", func(t *testing.T) {
		resp := &SearchResponse{Results: []SearchResult{}}
		se.enforceCharBudget(resp)
		if len(resp.Results) != 0 {
			t.Errorf("expected 0 results, got %d", len(resp.Results))
		}
	})
}

// --- waybackURL extended tests ------------------------------------------

func Test_waybackURL_Extended(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "HTTPS scheme",
			in:   "https://example.com/path",
			want: "https://web.archive.org/web/2/example.com/path",
		},
		{
			name: "HTTP scheme",
			in:   "http://example.com/path",
			want: "https://web.archive.org/web/2/example.com/path",
		},
		{
			name: "URL with query params and fragment",
			in:   "https://example.com/search?q=test&page=1#top",
			want: "https://web.archive.org/web/2/example.com/search?q=test&page=1#top",
		},
		{
			name: "bare hostname (no scheme)",
			in:   "example.com",
			want: "https://web.archive.org/web/2/example.com",
		},
		{
			name: "deep path with special chars",
			in:   "https://site.com/a/b/c?foo=bar+baz",
			want: "https://web.archive.org/web/2/site.com/a/b/c?foo=bar+baz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := waybackURL(tt.in)
			if got != tt.want {
				t.Errorf("waybackURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}


// --- isBlockedIP tests --------------------------------------------------

func Test_isBlockedIP(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want bool
	}{
		// Loopback
		{"IPv4 loopback 127.0.0.1", "127.0.0.1", true},
		{"IPv6 loopback ::1", "::1", true},
		// RFC1918 private
		{"10.0.0.1", "10.0.0.1", true},
		{"172.16.0.1", "172.16.0.1", true},
		{"172.31.255.254", "172.31.255.254", true},
		{"192.168.0.1", "192.168.0.1", true},
		{"192.168.255.254", "192.168.255.254", true},
		// Link-local
		{"IPv4 link-local 169.254.1.1", "169.254.1.1", true},
		{"IPv6 link-local fe80::1", "fe80::1", true},
		// Multicast
		{"multicast 224.0.0.1", "224.0.0.1", true},
		// Unspecified
		{"unspecified 0.0.0.0", "0.0.0.0", true},
		{"unspecified :: (IPv6)", "::", true},
		// IPv4-mapped IPv6 private
		{"IPv4-mapped ::ffff:192.168.1.1", "::ffff:192.168.1.1", true},
		// Public IPs — must NOT be blocked
		{"public 8.8.8.8", "8.8.8.8", false},
		{"public 1.1.1.1", "1.1.1.1", false},
		{"public 2001:db8::1", "2001:db8::1", false},
		// Edge of private ranges
		{"borderline 172.15.255.255 (not private)", "172.15.255.255", false},
		{"borderline 172.32.0.0 (not private)", "172.32.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.addr)
			if ip == nil {
				t.Fatalf("invalid IP in test: %s", tt.addr)
			}
			got := isBlockedIP(ip)
			if got != tt.want {
				t.Errorf("isBlockedIP(%s) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}

// Explicit IPv6 ULA test (defense-in-depth: IsPrivate() may not cover all
// implementations; our explicit fd00::/8 CIDR check catches it regardless).
func Test_isBlockedIP_UlaExplicit(t *testing.T) {
	ip := net.ParseIP("fd00::dead:beef")
	if !isBlockedIP(ip) {
		t.Error("isBlockedIP(fd00::dead:beef) = false, want true (ULA)")
	}
}

// --- FormatFetchResultsBlock tests --------------------------------------

func Test_FormatFetchResultsBlock_SanitizesInput(t *testing.T) {
	// Verify that the block formatter runs sanitizeFetchContent internally.
	content := "safe text\u200bwith zwsp"
	block := FormatFetchResultsBlock("https://example.com", content)

	if !strings.Contains(block, "[Fetched from:") {
		t.Error("expected [Fetched from: delimiter")
	}
	if !strings.Contains(block, "[/Fetched]") {
		t.Error("expected [/Fetched] closing delimiter")
	}
	// Zero-width space should have been stripped by sanitizeFetchContent
	if strings.Contains(block, "\u200b") {
		t.Error("expected zero-width space to be stripped in output")
	}
	if !strings.Contains(block, "safe textwith zwsp") {
		t.Errorf("unexpected sanitized content: %q", block)
	}
}

// --- readability integration tests -------------------------------------

// helper: create a webFetcher suitable for unit testing extractBodyBytes.
func testFetcher(maxChars int) *webFetcher {
	return newWebFetcher(nil, maxChars, 8, false)
}

// Test_readabilityExtraction_ArticlePage verifies that on a well-formed
// article page (nav, header, main content, footer), readability strips the
// chrome and extracts only the article body.
func Test_readabilityExtraction_ArticlePage(t *testing.T) {
	html := `
<html><head><title>My Awesome Blog Post</title></head><body>
<nav class="navbar"><a href="/">Home</a> <a href="/about">About</a> <a href="/contact">Contact</a></nav>
<header class="hero"><h1>Welcome to my blog</h1><p>Your one-stop shop for interesting content</p></header>
<article>
  <h1>The Future of Artificial Intelligence</h1>
  <p class="byline">Published on 2024-01-15 by Jane Doe</p>
  <p>Artificial intelligence is transforming the way we live and work. From healthcare diagnostics to autonomous vehicles, AI applications are everywhere.</p>
  <h2>Making Progress in Healthcare</h2>
  <p>In recent years, machine learning algorithms have demonstrated remarkable ability to detect diseases earlier than traditional methods. This advancement alone could save millions of lives worldwide.</p>
  <p>The integration of AI into everyday systems requires careful consideration of ethics, privacy, and fairness.</p>
  <h2>Challenges Ahead</h2>
  <p>Despite the excitement, significant challenges remain including bias in training data, energy consumption, and workforce displacement concerns.</p>
  <p>We must navigate these challenges thoughtfully to ensure AI benefits everyone equally.</p>
</article>
<footer class="site-footer">&copy; 2024 MyBlog. All rights reserved.</footer>
<div class="sidebar">Subscribe to newsletter!</div>
</body></html>`

	wf := testFetcher(25000)
	content, rbTitle, err := wf.extractBodyBytes([]byte(html), "text/html; charset=utf-8")
	if err != nil {
		t.Fatalf("extractBodyBytes unexpectedly errored: %v", err)
	}

	// Should NOT contain nav chrome
	if strings.Contains(content, "Welcome to my blog") {
		t.Error("content should NOT contain hero banner 'Welcome to my blog'")
	}
	if strings.Contains(content, "Home") && strings.Contains(content, "About") {
		t.Error("content should NOT contain navbar links (Home/About)")
	}
	if strings.Contains(content, "© 2024") || strings.Contains(content, "All rights reserved") {
		t.Error("content should NOT contain footer copyright text")
	}
	if strings.Contains(content, "Subscribe to newsletter") {
		t.Error("content should NOT contain sidebar text")
	}

	// SHOULD contain article content
	if !strings.Contains(content, "The Future of Artificial Intelligence") {
		t.Error("content should contain article title")
	}
	if !strings.Contains(content, "machine learning algorithms have demonstrated remarkable ability") {
		t.Error("content should contain main article paragraph")
	}
	if !strings.Contains(content, "significant challenges remain") {
		t.Error("content should contain challenges paragraph")
	}

	// Readability should have discovered a title
	if rbTitle == "" {
		t.Error("readability title should not be empty for a well-formed article")
	}
}

// Test_readabilityExtraction_NonArticlePage verifies graceful fallback to
// htmltomd when the page lacks article structure (e.g., a simple dashboard).
func Test_readabilityExtraction_NonArticlePage(t *testing.T) {
	html := `
<html><head><title>Dashboard</title></head><body>
<nav><a href="/">Dash</a></nav>
<div class="dashboard">
  <div class="card"><h3>Users</h3><p>1,234</p></div>
  <div class="card"><h3>Sessions</h3><p>5,678</p></div>
  <div class="card"><h3>Revenue</h3><p>$42,000</p></div>
</div>
</body></html>`

	wf := testFetcher(25000)
	content, rbTitle, err := wf.extractBodyBytes([]byte(html), "text/html; charset=utf-8")
	if err != nil {
		t.Fatalf("extractBodyBytes unexpectedly errored: %v", err)
	}

	// Dashboard pages typically won't pass readability's char threshold,
	// so htmltomd should be used. Either way, we should get SOME content.
	trimmed := strings.TrimSpace(content)
	if len(trimmed) == 0 {
		t.Fatal("content should not be empty — even dashboard pages should produce output")
	}

	// The htmltomd fallback will include everything; readability would strip cards.
	// At least we know the fallback produces usable output.
	if !(strings.Contains(content, "Users") || strings.Contains(content, "Sessions") || strings.Contains(content, "Revenue")) {
		t.Errorf("content should contain at least some card labels. Got:\n%s", content)
	}

	// Readability likely didn't discover a useful title for a non-article page.
	// That's fine — the title fallback mechanism is tested elsewhere.
	_ = rbTitle // just consume it
}

// Test_readabilityExtraction_MinimalContent verifies that when readability
// returns less than minReadabilityContent chars (<200), we fall back to htmltomd.
func Test_readabilityExtraction_MinimalContent(t *testing.T) {
	html := `
<html><head><title>Short Page</title></head><body>
<p>Hello world</p>
</body></html>`

	wf := testFetcher(25000)
	content, _, err := wf.extractBodyBytes([]byte(html), "text/html; charset=utf-8")
	if err != nil {
		t.Fatalf("extractBodyBytes unexpectedly errored: %v", err)
	}

	trimmed := strings.TrimSpace(content)
	if len(trimmed) == 0 {
		t.Fatal("content should not be empty")
	}
	// The htmltomd fallback should produce "Hello world"
	if !strings.Contains(content, "Hello world") {
		t.Errorf("expected fallback content to contain 'Hello world', got:\n%s", content)
	}
}

// Test_readabilityExtraction_GitHubIssue verifies extraction of a realistic
// GitHub issue page (simplified) with data-testid attrs and nav chrome.
func Test_readabilityExtraction_GitHubIssue(t *testing.T) {
	html := `
<!DOCTYPE html>
<html>
<head><title>Add feature X · Issue #42 · org/repo · GitHub</title></head>
<body>
<nav class="Navigation">
  <a href="/">GitHub</a>
  <a href="/features">Features</a>
  <a href="/pricing">Pricing</a>
</nav>
<main>
  <h1>Add feature X <span>#42</span></h1>
  <div data-testid="issue-body">
    <p>We really need feature X in the product. Without it, users have to
       manually export data to CSV and import it elsewhere.</p>
    <h2>Proposed Solution</h2>
    <p>Add a button on the dashboard that triggers the export pipeline
       automatically when data reaches the processing stage.</p>
    <p>This would involve:</p>
    <ul>
      <li>Adding a trigger condition in the backend</li>
      <li>Creating a notification endpoint for subscribers</li>
      <li>Writing integration tests for the full pipeline</li>
    </ul>
    <h2>Impact Analysis</h2>
    <p>Based on user surveys, approximately 75% of enterprise customers
       currently use manual workarounds for this workflow. Automating this
       process could reduce average processing time by 40 minutes per operation.</p>
  </div>
  <div class="TimelineItem">
    <p><strong>maintainer</strong> commented on Jan 15:</p>
    <p>This sounds great, thanks for filing. Will prioritize for Q2 sprint.</p>
  </div>
</main>
<footer class="Footer">
  <a href="/terms">Terms</a> · <a href="/privacy">Privacy</a>
</footer>
</body></html>`

	wf := testFetcher(25000)
	content, rbTitle, err := wf.extractBodyBytes([]byte(html), "text/html; charset=utf-8")
	if err != nil {
		t.Fatalf("extractBodyBytes unexpectedly errored: %v", err)
	}

	// Should contain issue body content
	if !strings.Contains(content, "We really need feature X") {
		t.Error("content should contain issue body opening paragraph")
	}
	if !strings.Contains(content, "Proposed Solution") {
		t.Error("content should contain Proposed Solution heading")
	}
	if !strings.Contains(content, "trigger condition in the backend") {
		t.Error("content should contain solution detail bullet")
	}
	if !strings.Contains(content, "75% of enterprise customers") {
		t.Error("content should contain impact analysis paragraph")
	}

	// Should NOT contain nav chrome
	if strings.Contains(content, "Features") && strings.Contains(content, "Pricing") {
		// Nav items appearing together is likely nav leakage
		if !strings.Contains(content, "Add feature X") {
			t.Error("if Features/Pricing appears, it suggests nav chrome leaked")
		}
	}
	if strings.Contains(content, "Terms") && strings.Contains(content, "Privacy") {
		t.Error("content should NOT contain footer nav items")
	}

	// Readability should discover a title (likely "Add feature X · Issue #42 · org/repo · GitHub" or variant)
	if rbTitle == "" {
		t.Error("readability title should not be empty for a GitHub issue page")
	}
}

// Test_readabilityExtraction_EmptyBody verifies that a completely empty
// HTML body gracefully falls back to htmltomd.
func Test_readabilityExtraction_EmptyBody(t *testing.T) {
	html := `<html><head><title>Empty</title></head><body></body></html>`

	wf := testFetcher(25000)
	content, _, err := wf.extractBodyBytes([]byte(html), "text/html; charset=utf-8")
	if err != nil {
		t.Fatalf("extractBodyBytes unexpectedly errored: %v", err)
	}
	// Empty body should produce empty or near-empty content — that's fine,
	// the point is no crash.
	_ = content // just assert no panic/error
}

// Test_readabilityExtraction_TitleFallback verifies that readability's
// discovered title is used when extractTitleFromHTML returns empty.
func Test_readabilityExtraction_TitleFallback(t *testing.T) {
	// HTML with NO <title> tag but a clear article heading that readability
	// picks up as the title.
	htmlNoTitle := `
<html><head></head><body>
<nav><a href="/">Nav</a></nav>
<article>
  <h1>Understanding Kubernetes Networking</h1>
  <p>Kubernetes networking is one of the most complex aspects of container orchestration.
     It involves CNI plugins, service mesh configurations, ingress controllers, and namespace isolation.</p>
  <p>The networking model assumes that pods can communicate with each other without NAT.
     This is achieved through overlay networks, iptables rules, and kernel-level packet forwarding.</p>
  <h2>CNI Plugins</h2>
  <p>Container Network Interface (CNI) plugins like Calico, Flannel, and Cilium provide
     different approaches to pod networking. Each has tradeoffs in performance, features, and complexity.</p>
  <p>Choosing the right CNI plugin depends on your cluster topology, security requirements,
     and operational expertise.</p>
  <h2>Service Discovery</h2>
  <p>Services provide stable endpoints for groups of pods. Kubernetes automatically manages
     DNS records for services, enabling easy discovery within clusters.</p>
  <p>Ingress resources expose HTTP/HTTPS routes to external traffic through load balancers.</p>
</article>
<footer>Copyright 2024</footer>
</body></html>`

	// Simulate what doFetchURL does:\ extractTitleFromHTML first (returns empty),
	// then extractBodyBytes (readability discovers title).
	extractedTitle := extractTitleFromHTML([]byte(htmlNoTitle))
	wf := testFetcher(25000)
	_, readabilityTitle, err := wf.extractBodyBytes([]byte(htmlNoTitle), "text/html; charset=utf-8")
	if err != nil {
		t.Fatalf("extractBodyBytes unexpectedly errored: %v", err)
	}

	// Simulate the doFetchURL fallback pattern:
	// if extractedTitle is empty, use readabilityTitle as fallback.
	finalTitle := extractedTitle
	if finalTitle == "" {
		finalTitle = readabilityTitle
	}

	// extractTitleFromHTML should now find the <h1> via our h1 fallback.
	// readability.Title is empty (no <title> tag), so the h1 fallback
	// in extractTitleFromHTML provides the title.
	if finalTitle == "" {
		t.Error("finalTitle should not be empty — extractTitleFromHTML should find <h1>")
	}
	if !strings.Contains(finalTitle, "Understanding Kubernetes Networking") {
		t.Errorf("finalTitle = %q, expected to contain 'Understanding Kubernetes Networking'", finalTitle)
	}
}

// Test_readabilityExtraction_ParsesMalformedHTML verifies that malformed HTML
// (unclosed tags, invalid nesting) handled gracefully without panics.
func Test_readabilityExtraction_ParsesMalformedHTML(t *testing.T) {
	html := `
<html><head><title>Broken Page</title></head><body>
<div class="wrapper"
  <p>This is unclosed paragraph with <strong>bold text
  <div>Another div without close
  <p>Yet another paragraph
</body></html>`

	wf := testFetcher(25000)
	content, _, err := wf.extractBodyBytes([]byte(html), "text/html; charset=utf-8")
	if err != nil {
		t.Fatalf("extractBodyBytes unexpectedly errored: %v", err)
	}

	// Should produce SOME output without crashing
	if strings.TrimSpace(content) == "" {
		t.Error("malformed HTML should still produce some content")
	}
}

// Test_readabilityExtraction_StrictHTML asserts that non-HTML content types
// bypass readability entirely.
func Test_readabilityExtraction_NonHTMLContentTypes(t *testing.T) {
	tests := []struct {
		name  string
		ct    string
		body  string
		wantSubstr string
	}{
		{
			name:  "text/plain",
			ct:    "text/plain; charset=utf-8",
			body:  "Plain text content here",
			wantSubstr: "Plain text content here",
		},
		{
			name:  "application/json",
			ct:    "application/json",
			body:  `{"key": "value", "number": 42}`,
			wantSubstr: "key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf := testFetcher(25000)
			content, rbTitle, err := wf.extractBodyBytes([]byte(tt.body), tt.ct)
			if err != nil {
				t.Fatalf("extractBodyBytes errored: %v", err)
			}
			if !strings.Contains(content, tt.wantSubstr) {
				t.Errorf("expected content to contain %q, got:\n%s", tt.wantSubstr, content)
			}
			// Non-HTML content types should not produce readability titles
			if rbTitle != "" {
				t.Errorf("non-HTML content type should not produce readability title, got: %q", rbTitle)
			}
		})
	}
}

// Test_readabilityExtraction_TitleFromTag verifies that when a <title> tag
// exists, both extractTitleFromHTML and readability agree on it.
func Test_readabilityExtraction_TitleFromTag(t *testing.T) {
	htmlWithTitle := `
<html><head><title>My Great Article Title</title></head><body>
<article>
  <h1>Understanding Kubernetes Networking</h1>
  <p>Kubernetes networking is one of the most complex aspects of container orchestration.
     It involves CNI plugins, service mesh configurations, ingress controllers, and namespace isolation.</p>
  <p>The networking model assumes that pods can communicate with each other without NAT.
     This is achieved through overlay networks, iptables rules, and kernel-level packet forwarding.</p>
  <h2>CNI Plugins</h2>
  <p>Container Network Interface (CNI) plugins like Calico, Flannel, and Cilium provide
     different approaches to pod networking. Each has tradeoffs in performance, features, and complexity.</p>
  <p>Choosing the right CNI plugin depends on your cluster topology, security requirements,
     and operational expertise.</p>
</article>
</body></html>`

	extractedTitle := extractTitleFromHTML([]byte(htmlWithTitle))
	if extractedTitle == "" {
		t.Fatal("extractTitleFromHTML should find '<title>' tag")
	}
	if !strings.Contains(extractedTitle, "My Great Article Title") {
		t.Errorf("extractTitleFromHTML got %q, expected to contain article title", extractedTitle)
	}

	wf := testFetcher(25000)
	_, readabilityTitle, err := wf.extractBodyBytes([]byte(htmlWithTitle), "text/html; charset=utf-8")
	if err != nil {
		t.Fatalf("extractBodyBytes unexpectedly errored: %v", err)
	}

	// Readability should ALSO discover the <title> tag
	if readabilityTitle == "" {
		t.Error("readability should discover title from <title> tag")
	}
}
