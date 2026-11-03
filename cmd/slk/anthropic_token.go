package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gammons/slk/internal/tablabel"
)

// antLoginTokens is the host's ant login, served by the host bridge
// (tools/host-bridge.py) named by SLK_ANTHROPIC_TOKEN_ADDR ("host:port"),
// or nil when slk runs without one.
func antLoginTokens() *tablabel.TokenSource {
	addr := os.Getenv("SLK_ANTHROPIC_TOKEN_ADDR")
	if addr == "" {
		return nil
	}
	return tablabel.NewTokenSource(func(ctx context.Context) (tablabel.AccessToken, error) {
		return fetchBridgeToken(ctx, "http://"+addr+"/token")
	})
}

// fetchBridgeToken asks the host bridge at url for the ant login's access
// token. The bridge answers 200 with {"access_token", "expires_at" (unix
// seconds)}, or 503 with a plain-text reason (ant missing, not logged in).
func fetchBridgeToken(ctx context.Context, url string) (tablabel.AccessToken, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return tablabel.AccessToken{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return tablabel.AccessToken{}, fmt.Errorf("host bridge: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		reason, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return tablabel.AccessToken{}, fmt.Errorf("host bridge: %s: %s", resp.Status, strings.TrimSpace(string(reason)))
	}
	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresAt   int64  `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return tablabel.AccessToken{}, fmt.Errorf("host bridge: %w", err)
	}
	return tablabel.AccessToken{Value: body.AccessToken, ExpiresAt: time.Unix(body.ExpiresAt, 0)}, nil
}
