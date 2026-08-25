package tablabel

import (
	"context"
	"os"
	"testing"
	"time"
	"unicode/utf8"
)

// TestLabelLive hits the real Anthropic API; set SLK_TABLABEL_LIVE=1 (and
// ANTHROPIC_API_KEY) to run it. On Santa hosts the docker container tools/
// go.sh runs tests in needs both variables passed through.
func TestLabelLive(t *testing.T) {
	if os.Getenv("SLK_TABLABEL_LIVE") == "" {
		t.Skip("set SLK_TABLABEL_LIVE=1 to hit the real API")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := New("claude-haiku-4-5")
	label, err := c.Label(ctx,
		"colony-562 the flow viewer renders stale runs after a reconnect, can you fix it")
	if err != nil {
		t.Fatalf("Label: %v", err)
	}
	t.Logf("live label: %q", label)
	if label == "" || utf8.RuneCountInString(label) > 60 {
		t.Errorf("label %q outside expected shape", label)
	}
}
