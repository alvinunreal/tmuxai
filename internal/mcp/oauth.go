package mcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

type oauthCallback struct {
	mu            sync.Mutex
	expectedState string
	complete      bool
	resultCh      chan *auth.AuthorizationResult
	errorCh       chan error
}

func newOAuthCallback() *oauthCallback {
	return &oauthCallback{
		resultCh: make(chan *auth.AuthorizationResult, 1),
		errorCh:  make(chan error, 1),
	}
}

func (c *oauthCallback) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.expectedState == "" || r.URL.Query().Get("state") != c.expectedState {
		http.Error(w, "invalid OAuth state", http.StatusBadRequest)
		return
	}
	if c.complete {
		http.Error(w, "authorization already received", http.StatusConflict)
		return
	}
	if code := r.URL.Query().Get("code"); code != "" {
		select {
		case c.resultCh <- &auth.AuthorizationResult{Code: code, State: c.expectedState, Iss: r.URL.Query().Get("iss")}:
			c.complete = true
			fmt.Fprint(w, "TmuxAI authorization complete. You can close this tab.")
		default:
			http.Error(w, "authorization already received", http.StatusConflict)
		}
		return
	}
	if r.URL.Query().Get("error") != "" {
		select {
		case c.errorCh <- errors.New("authorization was denied by the server"):
			c.complete = true
			fmt.Fprint(w, "TmuxAI authorization was not completed.")
		default:
			http.Error(w, "authorization already received", http.StatusConflict)
		}
		return
	}
	http.Error(w, "missing authorization code", http.StatusBadRequest)
}

func (c *oauthCallback) fetch(ctx context.Context, args *auth.AuthorizationArgs, openBrowser func(string)) (*auth.AuthorizationResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	authorizationURL, err := url.Parse(args.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid authorization URL: %w", err)
	}
	state := authorizationURL.Query().Get("state")
	if state == "" {
		return nil, errors.New("authorization URL has no state")
	}
	c.mu.Lock()
	// A cancelled attempt may leave a callback buffered. It must never be
	// returned as the result of a subsequent authorization attempt.
	select {
	case <-c.resultCh:
	default:
	}
	select {
	case <-c.errorCh:
	default:
	}
	c.expectedState = state
	c.complete = false
	c.mu.Unlock()

	fmt.Fprintf(os.Stderr, "Open this URL to authorize TmuxAI: %s\n", args.URL)
	openBrowser(args.URL)
	select {
	case result := <-c.resultCh:
		return result, nil
	case err := <-c.errorCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// newOAuthHandler uses a loopback-only redirect and the MCP SDK's discovery,
// dynamic registration, PKCE, and token handling. Tokens remain in memory.
func newOAuthHandler() (*auth.AuthorizationCodeHandler, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, fmt.Errorf("listen for OAuth callback: %w", err)
	}
	redirectURL := "http://" + listener.Addr().String() + "/callback"
	callback := newOAuthCallback()

	mux := http.NewServeMux()
	mux.Handle("/callback", callback)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	closeCallback := func() { _ = server.Close() }

	handler, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		RedirectURL: redirectURL,
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{
			Metadata: &oauthex.ClientRegistrationMetadata{
				ClientName:   "TmuxAI",
				RedirectURIs: []string{redirectURL},
			},
		},
		AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			return callback.fetch(ctx, args, openAuthorizationURL)
		},
	})
	if err != nil {
		closeCallback()
		return nil, nil, fmt.Errorf("configure OAuth: %w", err)
	}
	return handler, closeCallback, nil
}

func openAuthorizationURL(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}
	go func() { _ = cmd.Run() }() // Reap the browser launcher; URL is also printed.
}
