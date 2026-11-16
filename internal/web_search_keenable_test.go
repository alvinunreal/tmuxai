package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type keenableCall struct {
	method  string
	params  map[string]any
	headers http.Header
}

func newKeenableServer(t *testing.T, toolResult string, sse bool, isError bool) (*httptest.Server, *[]keenableCall) {
	t.Helper()
	calls := []keenableCall{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		calls = append(calls, keenableCall{req.Method, req.Params, r.Header.Clone()})
		var result any = map[string]any{"protocolVersion": keenableProtocolVersion}
		if req.Method == "initialize" {
			w.Header().Set("Mcp-Session-Id", "session-1")
		} else {
			result = map[string]any{
				"isError": isError,
				"content": []map[string]string{{"type": "text", "text": toolResult}},
			}
		}
		msg, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		if sse {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(msg)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

const keenableResults = `{"results":[` +
	`{"title":"tmux <b>docs</b>","url":"https://github.com/tmux/tmux/wiki","snippet":"Getting &amp; started"},` +
	`{"title":"no url","url":"","snippet":"skipped"},` +
	`{"title":"Second","url":"https://example.org/","description":"fallback description"}]}`

func TestKeenableSearchParsesResults(t *testing.T) {
	for _, sse := range []bool{false, true} {
		t.Run(fmt.Sprintf("sse=%v", sse), func(t *testing.T) {
			srv, calls := newKeenableServer(t, keenableResults, sse, false)
			resp := NewKeenableProvider("", srv.URL, nil, 5).Search(context.Background(), "tmux docs", 5)
			if resp.Error != nil {
				t.Fatalf("unexpected error: %v", resp.Error)
			}
			if resp.Provider != "keenable" || len(resp.Results) != 2 {
				t.Fatalf("unexpected response: %+v", resp)
			}
			if resp.Results[0].Title != "tmux docs" || resp.Results[0].Snippet != "Getting & started" {
				t.Errorf("snippet not cleaned: %+v", resp.Results[0])
			}
			if resp.Results[1].Snippet != "fallback description" {
				t.Errorf("description fallback not used: %+v", resp.Results[1])
			}
			if len(*calls) != 2 || (*calls)[0].method != "initialize" || (*calls)[1].method != "tools/call" {
				t.Fatalf("unexpected calls: %+v", *calls)
			}
			call := (*calls)[1]
			if call.headers.Get("Mcp-Session-Id") != "session-1" {
				t.Errorf("session id not forwarded")
			}
			if call.headers.Get("X-API-Key") != "" {
				t.Errorf("API key sent without configuration")
			}
			args := call.params["arguments"].(map[string]any)
			if call.params["name"] != "search_web_pages" || args["query"] != "tmux docs" ||
				args["max_results"] != float64(5) || args["output_format"] != "json" {
				t.Errorf("unexpected tool call: %+v", call.params)
			}
		})
	}
}

func TestKeenableSearchOptionalAPIKeyAndLimit(t *testing.T) {
	srv, calls := newKeenableServer(t, keenableResults, false, false)
	resp := NewKeenableProvider("k-123", srv.URL, nil, 5).Search(context.Background(), "q", 500)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	for _, c := range *calls {
		if c.headers.Get("X-API-Key") != "k-123" {
			t.Errorf("%s: missing X-API-Key", c.method)
		}
	}
	args := (*calls)[1].params["arguments"].(map[string]any)
	if args["max_results"] != float64(keenableMaxResults) {
		t.Errorf("max_results not capped: %v", args["max_results"])
	}
}

func TestKeenableSearchErrors(t *testing.T) {
	srv, _ := newKeenableServer(t, "quota exceeded", false, true)
	if resp := NewKeenableProvider("", srv.URL, nil, 5).Search(context.Background(), "q", 3); resp.Error == nil ||
		!strings.Contains(resp.Error.Error(), "quota exceeded") {
		t.Errorf("expected tool error, got %+v", resp)
	}

	for status, want := range map[int]string{429: "rate limited", 403: "authentication", 500: "unexpected status"} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
		resp := NewKeenableProvider("", s.URL, nil, 5).Search(context.Background(), "q", 3)
		s.Close()
		if resp.Error == nil || !strings.Contains(resp.Error.Error(), want) {
			t.Errorf("status %d: expected %q, got %v", status, want, resp.Error)
		}
	}

	rpcErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"boom"}}`))
	}))
	defer rpcErr.Close()
	if resp := NewKeenableProvider("", rpcErr.URL, nil, 5).Search(context.Background(), "q", 3); resp.Error == nil ||
		!strings.Contains(resp.Error.Error(), "boom") {
		t.Errorf("expected RPC error, got %+v", resp)
	}
}

func TestKeenableDoesNotFollowRedirects(t *testing.T) {
	hit := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	resp := NewKeenableProvider("secret", redirect.URL, nil, 5).Search(context.Background(), "q", 3)
	if resp.Error == nil || hit {
		t.Fatalf("redirect followed (hit=%v, err=%v)", hit, resp.Error)
	}
}

func TestKeenableDefaults(t *testing.T) {
	p := NewKeenableProvider("", "", nil, 0)
	if p.endpoint != keenableDefaultURL || p.Name() != "keenable" {
		t.Fatalf("unexpected defaults: %+v", p)
	}
}

func TestLastSSEDataJoinsMultilineEvent(t *testing.T) {
	body := []byte("event: message\ndata: {\"a\":1}\n\nevent: message\ndata: {\"jsonrpc\":\"2.0\",\ndata: \"id\":2}\n\n")
	got := string(lastSSEData(body))
	want := "{\"jsonrpc\":\"2.0\",\n\"id\":2}"
	if got != want {
		t.Fatalf("lastSSEData = %q, want %q", got, want)
	}
}
