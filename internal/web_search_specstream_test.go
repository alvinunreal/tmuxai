package internal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const specstreamSources = `{"data":{"query":"tmux capture-pane","sources":[` +
	`{"title":"tmux <b>manual</b>","url":"https://man7.org/linux/man-pages/man1/tmux.1.html","snippet":"page summary","highlights":["capture-pane [-aepPqCJN]","Capture the contents of a pane &amp; print it"]},` +
	`{"title":"no url","url":"","snippet":"skipped"},` +
	`{"title":"Wiki","url":"https://github.com/tmux/tmux/wiki","snippet":"fallback snippet","highlights":[]}]},"error":null}`

func newSpecStreamServer(t *testing.T, status int, body string) (*httptest.Server, *http.Request) {
	t.Helper()
	var got http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r.Clone(context.Background())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestSpecStreamSearch(t *testing.T) {
	srv, got := newSpecStreamServer(t, 200, specstreamSources)
	resp := NewSpecStreamProvider("secret", srv.URL, nil, 5).Search(context.Background(), "tmux capture-pane", 5)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if resp.Provider != "specstream" || len(resp.Results) != 2 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	first := resp.Results[0]
	if first.Title != "tmux manual" || first.Snippet != "capture-pane [-aepPqCJN] Capture the contents of a pane & print it" {
		t.Errorf("highlights not joined or cleaned: %+v", first)
	}
	if resp.Results[1].Snippet != "fallback snippet" {
		t.Errorf("snippet fallback not used: %+v", resp.Results[1])
	}
	q := got.URL.Query()
	if got.Method != http.MethodGet || q.Get("q") != "tmux capture-pane" || q.Get("num") != "5" {
		t.Errorf("unexpected request: %s %s", got.Method, got.URL.String())
	}
	if q.Get("speed") != "fast" || q.Get("passages") != "true" {
		t.Errorf("expected speed=fast and passages=true, got %s", got.URL.RawQuery)
	}
	if got.Header.Get("Authorization") != "Bearer secret" {
		t.Errorf("unexpected Authorization header: %q", got.Header.Get("Authorization"))
	}
}

func TestSpecStreamCapsResults(t *testing.T) {
	srv, got := newSpecStreamServer(t, 200, specstreamSources)
	resp := NewSpecStreamProvider("secret", srv.URL, nil, 5).Search(context.Background(), "q", 500)
	if got.URL.Query().Get("num") != "100" {
		t.Errorf("num should be capped at 100, got %q", got.URL.Query().Get("num"))
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	srv2, _ := newSpecStreamServer(t, 200, specstreamSources)
	one := NewSpecStreamProvider("secret", srv2.URL, nil, 5).Search(context.Background(), "q", 1)
	if len(one.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(one.Results))
	}
}

func TestSpecStreamKeyFromEnv(t *testing.T) {
	t.Setenv("SPECSTREAM_API_KEY", "from-env")
	srv, got := newSpecStreamServer(t, 200, specstreamSources)
	NewSpecStreamProvider("", srv.URL, nil, 5).Search(context.Background(), "q", 3)
	if got.Header.Get("Authorization") != "Bearer from-env" {
		t.Errorf("SPECSTREAM_API_KEY not used: %q", got.Header.Get("Authorization"))
	}
}

func TestSpecStreamNoKeyMakesNoRequest(t *testing.T) {
	t.Setenv("SPECSTREAM_API_KEY", "")
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	t.Cleanup(srv.Close)
	resp := NewSpecStreamProvider("", srv.URL, nil, 5).Search(context.Background(), "q", 3)
	if resp.Error == nil || !strings.Contains(resp.Error.Error(), "SPECSTREAM_API_KEY") {
		t.Errorf("expected a missing key error, got %v", resp.Error)
	}
	if called {
		t.Errorf("no request should be sent without a key")
	}
}

func TestSpecStreamErrors(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   string
	}{
		{401, `{"error":{"code":"invalid_token","message":"key not accepted"}}`, "authentication failed. Check your SPECSTREAM_API_KEY: key not accepted"},
		{402, `{"error":{"code":"insufficient_credit","message":"add credit"}}`, "no search credit left: add credit"},
		{429, `{"error":{"code":"rate_limited","message":"slow down"}}`, "rate limited"},
		{500, `not json`, "unexpected status 500"},
		{200, `not json`, "failed to parse response"},
	}
	for _, c := range cases {
		srv, _ := newSpecStreamServer(t, c.status, c.body)
		resp := NewSpecStreamProvider("secret", srv.URL, nil, 5).Search(context.Background(), "q", 3)
		if resp.Error == nil || !strings.Contains(resp.Error.Error(), c.want) {
			t.Errorf("status %d: expected error containing %q, got %v", c.status, c.want, resp.Error)
		}
	}
}

func TestSpecStreamDoesNotFollowRedirects(t *testing.T) {
	var leaked bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			leaked = true
		}
	}))
	t.Cleanup(target.Close)
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	t.Cleanup(redirect.Close)
	resp := NewSpecStreamProvider("secret", redirect.URL, nil, 5).Search(context.Background(), "q", 3)
	if resp.Error == nil {
		t.Errorf("expected an error for a redirect")
	}
	if leaked {
		t.Errorf("the API key was sent to the redirect target")
	}
}
