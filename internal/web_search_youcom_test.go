package internal

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const youcomResults = `{"results":{"web":[` +
	`{"title":"tmux <b>docs</b>","url":"https://github.com/tmux/tmux/wiki","description":"short","snippets":["Getting &amp; started","second"]},` +
	`{"title":"no url","url":"","description":"skipped"},` +
	`{"title":"Second","url":"https://example.org/","description":"fallback description","snippets":[]}]}}`

func newYouComServer(t *testing.T, status int, body string) (*httptest.Server, *http.Request, *[]byte) {
	t.Helper()
	var got http.Request
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r.Clone(context.Background())
		raw, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &got, &raw
}

func TestYouComKeylessSearch(t *testing.T) {
	t.Setenv("YDC_API_KEY", "")
	srv, got, _ := newYouComServer(t, 200, youcomResults)
	resp := NewYouComProvider("", srv.URL, nil, 5).Search(context.Background(), "tmux docs", 5)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if resp.Provider != "youcom" || len(resp.Results) != 2 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Results[0].Title != "tmux docs" || resp.Results[0].Snippet != "Getting & started second" {
		t.Errorf("snippet not cleaned or joined: %+v", resp.Results[0])
	}
	if resp.Results[1].Snippet != "fallback description" {
		t.Errorf("description fallback not used: %+v", resp.Results[1])
	}
	if got.Method != http.MethodGet || got.URL.Query().Get("query") != "tmux docs" || got.URL.Query().Get("count") != "5" {
		t.Errorf("unexpected keyless request: %s %s", got.Method, got.URL.String())
	}
	if got.Header.Get("X-API-Key") != "" {
		t.Errorf("keyless request must not send X-API-Key")
	}
	if got.Header.Get("User-Agent") != "youdotcom-integration/alvinunreal-tmuxai" {
		t.Errorf("unexpected User-Agent: %q", got.Header.Get("User-Agent"))
	}
}

func TestYouComKeylessCapsCount(t *testing.T) {
	t.Setenv("YDC_API_KEY", "")
	srv, got, _ := newYouComServer(t, 200, youcomResults)
	NewYouComProvider("", srv.URL, nil, 5).Search(context.Background(), "q", 80)
	if got.URL.Query().Get("count") != "50" {
		t.Errorf("keyless count should be capped at 50, got %q", got.URL.Query().Get("count"))
	}
}

func TestYouComKeyedSearch(t *testing.T) {
	t.Setenv("YDC_API_KEY", "")
	srv, got, raw := newYouComServer(t, 200, youcomResults)
	resp := NewYouComProvider("secret", srv.URL, nil, 5).Search(context.Background(), "tmux docs", 80)
	if resp.Error != nil || len(resp.Results) != 2 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got.Method != http.MethodPost || got.Header.Get("X-API-Key") != "secret" {
		t.Errorf("unexpected keyed request: %s key=%q", got.Method, got.Header.Get("X-API-Key"))
	}
	var body map[string]any
	if err := json.Unmarshal(*raw, &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if body["query"] != "tmux docs" || body["count"] != float64(80) {
		t.Errorf("unexpected body: %v", body)
	}
	if got.Header.Get("User-Agent") != "youdotcom-integration/alvinunreal-tmuxai" {
		t.Errorf("unexpected User-Agent: %q", got.Header.Get("User-Agent"))
	}
}

func TestYouComKeyFromEnvironment(t *testing.T) {
	t.Setenv("YDC_API_KEY", "from-env")
	srv, got, _ := newYouComServer(t, 200, youcomResults)
	NewYouComProvider("", srv.URL, nil, 5).Search(context.Background(), "q", 5)
	if got.Method != http.MethodPost || got.Header.Get("X-API-Key") != "from-env" {
		t.Errorf("expected keyed request from env, got %s key=%q", got.Method, got.Header.Get("X-API-Key"))
	}
}

func TestYouComErrors(t *testing.T) {
	t.Setenv("YDC_API_KEY", "")
	cases := map[int]string{401: "authentication failed", 403: "authentication failed", 429: "rate limited", 500: "unexpected status 500"}
	for status, want := range cases {
		srv, _, _ := newYouComServer(t, status, `{}`)
		resp := NewYouComProvider("", srv.URL, nil, 5).Search(context.Background(), "q", 5)
		if resp.Error == nil || !strings.Contains(resp.Error.Error(), want) {
			t.Errorf("status %d: expected error containing %q, got %v", status, want, resp.Error)
		}
	}
	srv, _, _ := newYouComServer(t, 200, `not json`)
	if resp := NewYouComProvider("", srv.URL, nil, 5).Search(context.Background(), "q", 5); resp.Error == nil {
		t.Errorf("expected parse error")
	}
}

func TestYouComDoesNotFollowRedirects(t *testing.T) {
	t.Setenv("YDC_API_KEY", "")
	leaked := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("X-API-Key") != ""
		_, _ = w.Write([]byte(youcomResults))
	}))
	defer other.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer redir.Close()
	resp := NewYouComProvider("secret", redir.URL, nil, 5).Search(context.Background(), "q", 5)
	if resp.Error == nil || leaked {
		t.Errorf("redirect must not be followed (err=%v leaked=%v)", resp.Error, leaked)
	}
}
