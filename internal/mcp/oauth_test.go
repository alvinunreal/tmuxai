package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

func TestOAuthCallbackStateAndReplay(t *testing.T) {
	callback := newOAuthCallback()
	callback.expectedState = "expected"
	for _, tc := range []struct {
		name   string
		method string
		url    string
		status int
	}{
		{"wrong state", http.MethodGet, "/callback?state=wrong&code=secret", http.StatusBadRequest},
		{"missing state", http.MethodGet, "/callback?code=secret", http.StatusBadRequest},
		{"wrong method", http.MethodPost, "/callback?state=expected&code=secret", http.StatusMethodNotAllowed},
		{"missing code", http.MethodGet, "/callback?state=expected", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			callback.ServeHTTP(response, httptest.NewRequest(tc.method, tc.url, nil))
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d", response.Code, tc.status)
			}
		})
	}

	response := httptest.NewRecorder()
	callback.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/callback?state=expected&code=secret&iss=https://issuer.example", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("valid callback status = %d", response.Code)
	}
	result := <-callback.resultCh
	if result.Code != "secret" || result.State != "expected" || result.Iss != "https://issuer.example" {
		t.Fatalf("unexpected authorization result: %+v", result)
	}
	response = httptest.NewRecorder()
	callback.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/callback?state=expected&code=replay", nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("replayed callback status = %d, want 409", response.Code)
	}
}

func TestOAuthCallbackDenial(t *testing.T) {
	callback := newOAuthCallback()
	callback.expectedState = "expected"
	response := httptest.NewRecorder()
	callback.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/callback?state=expected&error=access_denied", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("denial status = %d", response.Code)
	}
	if err := <-callback.errorCh; err == nil {
		t.Fatal("expected authorization error")
	}
}

func TestOAuthCallbackCancellation(t *testing.T) {
	callback := newOAuthCallback()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opened := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := callback.fetch(ctx, &auth.AuthorizationArgs{URL: "https://issuer.example/authorize?state=expected"}, func(string) { close(opened) })
		done <- err
	}()
	select {
	case <-opened:
	case <-time.After(time.Second):
		t.Fatal("authorization fetch did not open")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("fetch error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("authorization fetch did not stop after cancellation")
	}
}

func TestOAuthCallbackNewAttemptDropsStaleCode(t *testing.T) {
	callback := newOAuthCallback()
	callback.expectedState = "old"
	response := httptest.NewRecorder()
	callback.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/callback?state=old&code=stale", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("old callback status = %d", response.Code)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := callback.fetch(ctx, &auth.AuthorizationArgs{
		URL: "https://issuer.example/authorize?state=new",
	}, func(string) {
		response = httptest.NewRecorder()
		callback.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/callback?state=old&code=stale-again", nil))
		if response.Code != http.StatusBadRequest {
			t.Errorf("old state status = %d, want 400", response.Code)
		}
		response = httptest.NewRecorder()
		callback.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/callback?state=new&code=fresh", nil))
		if response.Code != http.StatusOK {
			t.Errorf("new state status = %d, want 200", response.Code)
		}
	})
	if err != nil || result == nil || result.Code != "fresh" {
		t.Fatalf("new attempt result = %+v, error = %v", result, err)
	}
}
