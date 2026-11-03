package tablabel

import (
	"context"
	"os"
	"time"
)

// LiveTokens is the TokenSource the env-gated live tests and evals
// (SLK_TABLABEL_LIVE) pass ResolveCredential: the host's ant login access
// token that tools/go.sh hands the run as SLK_TABLABEL_LIVE_TOKEN, or nil
// when it handed none. Nothing can fetch a fresh one from inside the
// container, so the token counts as good for the whole run; a run that
// outlives it fails with a 401.
func LiveTokens() *TokenSource {
	token := os.Getenv("SLK_TABLABEL_LIVE_TOKEN")
	if token == "" {
		return nil
	}
	return NewTokenSource(func(context.Context) (AccessToken, error) {
		return AccessToken{Value: token, ExpiresAt: time.Now().Add(24 * time.Hour)}, nil
	})
}
