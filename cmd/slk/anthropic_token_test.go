package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchBridgeToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token": "tok", "expires_at": 1793695201}`))
	}))
	defer srv.Close()

	tok, err := fetchBridgeToken(context.Background(), srv.URL+"/token")
	if err != nil {
		t.Fatalf("fetchBridgeToken: %v", err)
	}
	if tok.Value != "tok" || !tok.ExpiresAt.Equal(time.Unix(1793695201, 0)) {
		t.Errorf("token = %q expiring %v, want tok expiring at 1793695201", tok.Value, tok.ExpiresAt)
	}
}

// A bridge that can't serve a token says why, and the error carries it.
func TestFetchBridgeTokenUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "ant auth print-credentials failed: not logged in", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := fetchBridgeToken(context.Background(), srv.URL+"/token")
	if err == nil || !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("err = %v, want the 503 and the bridge's reason", err)
	}
}

func TestAntLoginTokensNeedsTheBridge(t *testing.T) {
	t.Setenv("SLK_ANTHROPIC_TOKEN_ADDR", "")
	if antLoginTokens() != nil {
		t.Error("antLoginTokens without SLK_ANTHROPIC_TOKEN_ADDR is non-nil, want nil")
	}
}
