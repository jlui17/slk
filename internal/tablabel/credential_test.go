package tablabel

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gammons/slk/internal/config"
)

const okReply = `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5-5","content":[{"type":"text","text":"label"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`

// authAPI is an httptest Messages API that records each request's auth
// headers and answers with the statuses given, then 200s.
type authAPI struct {
	mu       sync.Mutex
	statuses []int
	got      []http.Header
}

func (f *authAPI) requests() []http.Header {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.got
}

func (f *authAPI) start(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.got = append(f.got, r.Header.Clone())
		status := http.StatusOK
		if len(f.statuses) > 0 {
			status, f.statuses = f.statuses[0], f.statuses[1:]
		}
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusOK {
			w.Write([]byte(okReply))
		} else {
			w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
}

// countingTokens is a TokenSource whose every fetch returns token, good
// for ttl, and counts itself.
func countingTokens(token string, ttl time.Duration, fetches *atomic.Int32) *TokenSource {
	return NewTokenSource(func(context.Context) (AccessToken, error) {
		fetches.Add(1)
		return AccessToken{Value: token, ExpiresAt: time.Now().Add(ttl)}, nil
	})
}

func relabel(t *testing.T, cred Credential) error {
	t.Helper()
	_, _, err := New("claude-sonnet-5-5", "low", cred).Relabel(context.Background(), "transcript", nil)
	return err
}

// A configured key wins over the ant login, and puts only itself on the
// wire whatever the environment holds: the SDK's default client reads
// ANTHROPIC_API_KEY on its own.
func TestResolveCredentialKeyWins(t *testing.T) {
	for _, envKey := range []string{"", "from-env"} {
		t.Run("env="+envKey, func(t *testing.T) {
			api := &authAPI{}
			api.start(t)
			t.Setenv("ANTHROPIC_API_KEY", envKey)
			var fetches atomic.Int32
			cred, err := ResolveCredential(context.Background(), config.Herdr{AnthropicAPIKey: "from-config"},
				countingTokens("tok", time.Hour, &fetches))
			if err != nil {
				t.Fatalf("ResolveCredential: %v", err)
			}
			if cred.Source != "herdr.anthropic_api_key" {
				t.Errorf("Source = %q, want herdr.anthropic_api_key", cred.Source)
			}
			if err := relabel(t, cred); err != nil {
				t.Fatalf("Relabel: %v", err)
			}
			h := api.requests()[0]
			if h.Get("X-Api-Key") != "from-config" || h.Get("Authorization") != "" {
				t.Errorf("X-Api-Key = %q, Authorization = %q; want the config key alone", h.Get("X-Api-Key"), h.Get("Authorization"))
			}
			if n := fetches.Load(); n != 0 {
				t.Errorf("fetched %d ant tokens with a key configured, want 0", n)
			}
		})
	}
}

func TestResolveCredentialEnvKey(t *testing.T) {
	api := &authAPI{}
	api.start(t)
	t.Setenv("ANTHROPIC_API_KEY", "from-env")
	cred, err := ResolveCredential(context.Background(), config.Herdr{}, nil)
	if err != nil {
		t.Fatalf("ResolveCredential: %v", err)
	}
	if cred.Source != "ANTHROPIC_API_KEY" {
		t.Errorf("Source = %q, want ANTHROPIC_API_KEY", cred.Source)
	}
	if err := relabel(t, cred); err != nil {
		t.Fatalf("Relabel: %v", err)
	}
	if got := api.requests()[0].Get("X-Api-Key"); got != "from-env" {
		t.Errorf("X-Api-Key = %q, want from-env", got)
	}
}

// With no key, the ant login's token goes out as a bearer token with the
// oauth beta, and no x-api-key.
func TestResolveCredentialAntLogin(t *testing.T) {
	api := &authAPI{}
	api.start(t)
	t.Setenv("ANTHROPIC_API_KEY", "")
	var fetches atomic.Int32
	cred, err := ResolveCredential(context.Background(), config.Herdr{}, countingTokens("tok-1", time.Hour, &fetches))
	if err != nil {
		t.Fatalf("ResolveCredential: %v", err)
	}
	if cred.Source != "the host's ant login" {
		t.Errorf("Source = %q, want the host's ant login", cred.Source)
	}
	for range 2 {
		if err := relabel(t, cred); err != nil {
			t.Fatalf("Relabel: %v", err)
		}
	}
	for i, h := range api.requests() {
		if h.Get("Authorization") != "Bearer tok-1" || h.Get("X-Api-Key") != "" {
			t.Errorf("request %d: Authorization = %q, X-Api-Key = %q; want the bearer token alone", i, h.Get("Authorization"), h.Get("X-Api-Key"))
		}
		if !strings.Contains(h.Get("Anthropic-Beta"), oauthBeta) {
			t.Errorf("request %d: anthropic-beta = %q, want %s", i, h.Get("Anthropic-Beta"), oauthBeta)
		}
	}
	// The resolve's probe fetched the token; both requests reused it.
	if n := fetches.Load(); n != 1 {
		t.Errorf("fetched %d tokens, want 1", n)
	}
}

func TestResolveCredentialNone(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	if _, err := ResolveCredential(context.Background(), config.Herdr{}, nil); err == nil || !strings.Contains(err.Error(), "SLK_ANTHROPIC_TOKEN_ADDR") {
		t.Errorf("no key, no bridge: err = %v, want one naming the missing bridge", err)
	}
	notLoggedIn := NewTokenSource(func(context.Context) (AccessToken, error) {
		return AccessToken{}, errors.New("not logged in")
	})
	if _, err := ResolveCredential(context.Background(), config.Herdr{}, notLoggedIn); err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("no key, ant not logged in: err = %v, want one carrying the bridge's reason", err)
	}
}

// A token is reused until refreshBefore its expiry, then fetched again.
func TestTokenSourceRefreshesNearExpiry(t *testing.T) {
	var fetches atomic.Int32
	fresh := countingTokens("tok", time.Hour, &fetches)
	for range 3 {
		if _, err := fresh.Token(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if n := fetches.Load(); n != 1 {
		t.Errorf("an hour-long token: %d fetches over 3 calls, want 1", n)
	}

	fetches.Store(0)
	expiring := countingTokens("tok", refreshBefore-time.Second, &fetches)
	for range 3 {
		if _, err := expiring.Token(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if n := fetches.Load(); n != 3 {
		t.Errorf("a token inside refreshBefore: %d fetches over 3 calls, want 3", n)
	}
}

// A token is reused for reuseFor at most, so a re-login (another org) or a
// logout reaches the calls within a minute; a failed fetch serves nothing.
func TestTokenSourceSeesARelogin(t *testing.T) {
	login := "first-org"
	tokens := NewTokenSource(func(context.Context) (AccessToken, error) {
		if login == "" {
			return AccessToken{}, errors.New("not logged in")
		}
		return AccessToken{Value: login, ExpiresAt: time.Now().Add(8 * time.Hour)}, nil
	})
	age := func() { tokens.fetched = tokens.fetched.Add(-reuseFor) }

	if got, _ := tokens.Token(context.Background()); got != "first-org" {
		t.Fatalf("token = %q, want first-org", got)
	}
	login = "second-org"
	if got, _ := tokens.Token(context.Background()); got != "first-org" {
		t.Errorf("inside reuseFor: token = %q, want the cached first-org", got)
	}
	age()
	if got, _ := tokens.Token(context.Background()); got != "second-org" {
		t.Errorf("after reuseFor: token = %q, want second-org", got)
	}
	login = ""
	age()
	if got, err := tokens.Token(context.Background()); err == nil {
		t.Errorf("logged out: token = %q, want an error", got)
	}
	if got, err := tokens.Token(context.Background()); err == nil {
		t.Errorf("logged out, asked again: token = %q, want an error, not the old token", got)
	}
}

// A 401 names the credential and what to do, and drops an ant token so the
// next request fetches a fresh one.
func TestUnauthorized(t *testing.T) {
	t.Run("key", func(t *testing.T) {
		api := &authAPI{statuses: []int{http.StatusUnauthorized}}
		api.start(t)
		t.Setenv("ANTHROPIC_API_KEY", "")
		cred, _ := ResolveCredential(context.Background(), config.Herdr{AnthropicAPIKey: "revoked"}, nil)
		err := relabel(t, cred)
		want := "the configured key (herdr.anthropic_api_key) was rejected; remove it to use the ant login"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to say %q", err, want)
		}
	})
	t.Run("ant login", func(t *testing.T) {
		api := &authAPI{statuses: []int{http.StatusUnauthorized}}
		api.start(t)
		t.Setenv("ANTHROPIC_API_KEY", "")
		var fetches atomic.Int32
		cred, _ := ResolveCredential(context.Background(), config.Herdr{}, countingTokens("tok", time.Hour, &fetches))
		if err := relabel(t, cred); err == nil || !strings.Contains(err.Error(), "run `ant auth login` on the host") {
			t.Errorf("err = %v, want it to say to log in again", err)
		}
		if err := relabel(t, cred); err != nil {
			t.Fatalf("Relabel after the 401: %v", err)
		}
		if n := fetches.Load(); n != 2 {
			t.Errorf("fetched %d tokens, want 2: the probe's, and a fresh one after the 401", n)
		}
	})
}
