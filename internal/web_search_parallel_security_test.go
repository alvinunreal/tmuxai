package internal

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// All credentials and origins here are synthetic and confined to local servers.
func TestParallelAuthenticatedRedirectOrigins(t *testing.T) {
	for _, kind := range []string{"port", "host", "downgrade", "upgrade"} {
		t.Run(kind, func(t *testing.T) {
			var leaked, received atomic.Int32
			sink := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received.Add(1)
				if r.Header.Get("x-api-key") != "" {
					leaked.Add(1)
				}
				w.WriteHeader(http.StatusBadGateway)
			})
			var destination string
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/sink" {
					sink.ServeHTTP(w, r)
					return
				}
				if r.Header.Get("x-api-key") != "dummy-test-key" {
					t.Error("configured origin did not receive dummy key")
				}
				http.Redirect(w, r, destination, http.StatusTemporaryRedirect)
			})
			var source, target *httptest.Server
			if kind == "downgrade" {
				source = httptest.NewTLSServer(handler)
			} else {
				source = httptest.NewServer(handler)
			}
			defer source.Close()
			switch kind {
			case "host":
				destination = strings.Replace(source.URL, "127.0.0.1", "localhost", 1) + "/sink"
			case "upgrade":
				target = httptest.NewTLSServer(sink)
			default:
				target = httptest.NewServer(sink)
			}
			if target != nil {
				defer target.Close()
				destination = target.URL + "/sink"
			}
			hc := source.Client()
			if kind == "upgrade" {
				hc = target.Client()
			}
			p := NewParallelProvider("dummy-test-key", source.URL+"/mcp", hc, 2)
			result := p.Search(context.Background(), "tmux documentation", 1)
			t.Logf("local synthetic %s redirect: destination_requests=%d dummy_key_received=%t error=%v", kind, received.Load(), leaked.Load() > 0, result.Error)
			if leaked.Load() != 0 || received.Load() != 0 {
				t.Errorf("authenticated request escaped the configured origin")
			}
			if result.Error == nil {
				t.Error("expected cross-origin rejection")
			}
		})
	}
}

func TestParallelAuthenticatedSDKOrigin(t *testing.T) {
	var received atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(502)
	}))
	defer sink.Close()
	source := httptest.NewServer(http.NotFoundHandler())
	defer source.Close()
	p := NewParallelProvider("dummy-test-key", source.URL, nil, 2)
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "synthetic", Version: "1"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	session, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{
		Endpoint: sink.URL, HTTPClient: p.client, DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if session != nil {
		_ = session.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "outside configured origin") || received.Load() != 0 {
		t.Fatalf("direct cross-origin SDK request: error=%v received=%d", err, received.Load())
	}
	t.Log("direct cross-origin SDK request blocked before network I/O")
}

type parallelOriginProbe func(*http.Request) (*http.Response, error)

func (f parallelOriginProbe) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestParallelCredentialOriginBoundary(t *testing.T) {
	// Synthetic transport probes isolate each origin component without DNS or TLS.
	for _, tc := range []struct {
		name, endpoint, destination, host string
		allowed                           bool
	}{
		{name: "same origin path", endpoint: "https://localhost:8443/mcp", destination: "https://localhost:8443/other", allowed: true},
		{name: "scheme only", endpoint: "https://localhost:8443/mcp", destination: "http://localhost:8443/mcp"},
		{name: "host only", endpoint: "https://localhost:8443/mcp", destination: "https://127.0.0.1:8443/mcp"},
		{name: "port only", endpoint: "https://localhost:8443/mcp", destination: "https://localhost:8444/mcp"},
		{name: "default HTTPS port", endpoint: "https://LOCALHOST/mcp", destination: "https://localhost:443/other", allowed: true},
		{name: "default HTTP port", endpoint: "http://localhost:80/mcp", destination: "http://LOCALHOST/other", allowed: true},
		{name: "Host override", endpoint: "https://localhost:8443/mcp", destination: "https://localhost:8443/mcp", host: "foreign.localhost:8443"},
		{name: "invalid origin", endpoint: "://invalid", destination: "http://localhost:8443/mcp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sent := false
			hc := &http.Client{Transport: parallelOriginProbe(func(req *http.Request) (*http.Response, error) {
				sent = true
				if req.Header.Get("x-api-key") != "dummy-test-key" {
					t.Error("missing dummy key")
				}
				return &http.Response{StatusCode: 204, Header: make(http.Header), Body: http.NoBody}, nil
			})}
			p := NewParallelProvider("dummy-test-key", tc.endpoint, hc, 2)
			req, err := http.NewRequest(http.MethodPost, tc.destination, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.host != "" {
				req.Host = tc.host
			}
			response, err := p.client.Do(req)
			if response != nil {
				_ = response.Body.Close()
			}
			if sent != tc.allowed || (err == nil) != tc.allowed {
				t.Fatalf("sent=%t error=%v allowed=%t", sent, err, tc.allowed)
			}
		})
	}
}

func TestParallelClientRedirectRulesAndTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "dummy-test-key" {
			t.Error("missing dummy key at same origin")
		}
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/ok", http.StatusTemporaryRedirect)
		case "/ok":
			body, _ := io.ReadAll(r.Body)
			if r.Method != "POST" || string(body) != "synthetic body" {
				t.Error("same-origin redirect changed request")
			}
			w.WriteHeader(204)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/slow":
			<-r.Context().Done()
		}
	}))
	defer srv.Close()
	t.Run("same origin", func(t *testing.T) {
		p := NewParallelProvider("dummy-test-key", srv.URL, nil, 2)
		r, err := p.client.Post(srv.URL+"/start", "text/plain", strings.NewReader("synthetic body"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = r.Body.Close() }()
		if r.StatusCode != 204 {
			t.Fatalf("status=%d", r.StatusCode)
		}
	})
	t.Run("default redirect limit", func(t *testing.T) {
		p := NewParallelProvider("dummy-test-key", srv.URL, nil, 2)
		if p.client.CheckRedirect != nil {
			t.Fatal("overrode default redirect policy")
		}
		r, err := p.client.Get(srv.URL + "/loop")
		if r != nil {
			_ = r.Body.Close()
		}
		if err == nil || !strings.Contains(err.Error(), "stopped after 10 redirects") {
			t.Fatalf("error=%v", err)
		}
	})
	for _, lastResponse := range []bool{false, true} {
		name := "custom rejection"
		if lastResponse {
			name = "ErrUseLastResponse"
		}
		t.Run(name, func(t *testing.T) {
			rejected := errors.New("synthetic redirect rejection")
			called := false
			hc := &http.Client{Timeout: time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
				called = true
				if len(via) != 1 {
					t.Errorf("redirect history=%d", len(via))
				}
				if lastResponse {
					return http.ErrUseLastResponse
				}
				return rejected
			}}
			p := NewParallelProvider("dummy-test-key", srv.URL, hc, 2)
			if p.client.Timeout != hc.Timeout || hc.Transport != nil {
				t.Fatal("changed caller client")
			}
			r, err := p.client.Get(srv.URL + "/start")
			if r != nil {
				_ = r.Body.Close()
			}
			if !called {
				t.Fatal("caller redirect callback not invoked")
			}
			if lastResponse {
				if err != nil || r == nil || r.StatusCode != 307 {
					t.Fatalf("response=%v error=%v", r, err)
				}
			} else if !errors.Is(err, rejected) {
				t.Fatalf("error=%v", err)
			}
		})
	}
	t.Run("caller timeout", func(t *testing.T) {
		hc := &http.Client{Timeout: 50 * time.Millisecond}
		p := NewParallelProvider("dummy-test-key", srv.URL, hc, 2)
		r, err := p.client.Get(srv.URL + "/slow")
		if r != nil {
			_ = r.Body.Close()
		}
		if !errors.Is(err, context.DeadlineExceeded) || p.client.Timeout != hc.Timeout {
			t.Fatalf("timeout error=%v", err)
		}
	})
}

func TestParallelAnonymousRedirect(t *testing.T) {
	var received atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		if r.Header.Get("x-api-key") != "" || r.UserAgent() != "tmuxai/"+Version {
			t.Error("unexpected anonymous headers")
		}
		w.WriteHeader(204)
	}))
	defer sink.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, sink.URL, http.StatusFound) }))
	defer source.Close()
	p := NewParallelProvider("", source.URL, nil, 2)
	r, err := p.client.Get(source.URL)
	if r != nil {
		_ = r.Body.Close()
	}
	if err != nil || received.Load() != 1 {
		t.Fatalf("anonymous redirect: received=%d error=%v", received.Load(), err)
	}
}
