package tablabel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/gammons/slk/internal/config"
)

// Credential is how a Client authenticates its requests.
type Credential struct {
	// Source names where the credential came from, for logs. It never
	// holds the secret.
	Source string
	// rejected is what a request the API answers 401 says about it.
	rejected string
	option   option.RequestOption
}

// ResolveCredential picks how slk's model calls authenticate. A configured
// key wins (herdr.anthropic_api_key, then ANTHROPIC_API_KEY), even one the
// API rejects: a rejected key fails every call, it never falls through.
// Without a key, the ant login that tokens serves is used, once a first
// token is in hand. The error says why there is neither.
func ResolveCredential(ctx context.Context, cfg config.Herdr, tokens *TokenSource) (Credential, error) {
	if key, source := cfg.ResolveAnthropicAPIKey(); key != "" {
		return Credential{
			Source:   source,
			rejected: "the configured key (" + source + ") was rejected; remove it to use the ant login",
			option:   option.WithAPIKey(key),
		}, nil
	}
	const noKey = "no API key (herdr.anthropic_api_key or ANTHROPIC_API_KEY)"
	if tokens == nil {
		return Credential{}, errors.New(noKey + " and no ant login bridge (SLK_ANTHROPIC_TOKEN_ADDR)")
	}
	if _, err := tokens.Token(ctx); err != nil {
		return Credential{}, fmt.Errorf("%s and no ant login: %w", noKey, err)
	}
	return Credential{
		Source:   "the host's ant login",
		rejected: "the ant login's access token was rejected; run `ant auth login` on the host",
		option:   option.WithMiddleware(bearerAuth(tokens)),
	}, nil
}

// oauthBeta is the anthropic-beta value the API requires beside an OAuth
// access token.
const oauthBeta = "oauth-2025-04-20"

// bearerAuth sends each request with an access token from tokens, the way
// the SDK's own profile auth does (Authorization: Bearer, plus the oauth
// beta). A 401 drops the token, so the next request fetches a fresh one.
func bearerAuth(tokens *TokenSource) option.Middleware {
	return func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		token, err := tokens.Token(req.Context())
		if err != nil {
			return nil, fmt.Errorf("ant login: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		switch beta := req.Header.Get("anthropic-beta"); {
		case beta == "":
			req.Header.Set("anthropic-beta", oauthBeta)
		case !strings.Contains(beta, oauthBeta):
			req.Header.Set("anthropic-beta", beta+","+oauthBeta)
		}
		resp, err := next(req)
		if err == nil && resp.StatusCode == http.StatusUnauthorized {
			tokens.drop(token)
		}
		return resp, err
	}
}

// AccessToken is an OAuth access token and the time it stops working.
type AccessToken struct {
	Value     string
	ExpiresAt time.Time
}

// refreshBefore is how long before its expiry a token is fetched again: a
// request outlives labelTimeout (cmd/slk) at most, and ant itself refreshes
// a token in its last two minutes, so a fetch inside this window gets the
// new one.
const refreshBefore = 90 * time.Second

// reuseFor is how long a fetched token is reused at most, so a new `ant
// auth login` (another org, say) or a logout takes effect within a minute
// rather than when the old token expires.
const reuseFor = time.Minute

// TokenSource hands out an access token, reusing one for reuseFor and
// until refreshBefore its expiry, then fetching again.
type TokenSource struct {
	fetch   func(context.Context) (AccessToken, error)
	mu      sync.Mutex
	cached  AccessToken
	fetched time.Time
}

// NewTokenSource returns a TokenSource that gets its tokens from fetch.
func NewTokenSource(fetch func(context.Context) (AccessToken, error)) *TokenSource {
	return &TokenSource{fetch: fetch}
}

// Token is a token good for at least refreshBefore. A failed fetch leaves
// nothing cached, so a logout stops the calls within reuseFor.
func (s *TokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached.Value != "" && time.Since(s.fetched) < reuseFor && time.Until(s.cached.ExpiresAt) > refreshBefore {
		return s.cached.Value, nil
	}
	s.cached = AccessToken{}
	tok, err := s.fetch(ctx)
	if err != nil {
		return "", err
	}
	if tok.Value == "" {
		return "", errors.New("empty access token")
	}
	s.cached, s.fetched = tok, time.Now()
	return tok.Value, nil
}

// drop forgets token, when it is still the cached one.
func (s *TokenSource) drop(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached.Value == token {
		s.cached = AccessToken{}
	}
}
