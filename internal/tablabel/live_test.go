package tablabel

import (
	"context"
	"os"
	"testing"
	"time"
	"unicode/utf8"
)

// TestLabelLive hits the real Anthropic API; set SLK_TABLABEL_LIVE=1 (and
// ANTHROPIC_API_KEY) to run it. tools/go.sh forwards both into the docker
// container it runs tests in on Santa hosts.
func TestLabelLive(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
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

func liveClient(t *testing.T) *Client {
	t.Helper()
	if os.Getenv("SLK_TABLABEL_LIVE") == "" {
		t.Skip("set SLK_TABLABEL_LIVE=1 to hit the real API")
	}
	return New("claude-haiku-4-5")
}

// TestWorkingLive pins the working judge's verdicts on the message shapes
// the deterministic signal can't read, against the real model.
func TestWorkingLive(t *testing.T) {
	c := liveClient(t)
	rows := []struct {
		name      string
		fromAgent bool
		message   string
		want      bool
	}{
		{"plan awaiting go", true, "Plan for gitea, verified against current main. Two PRs. Justin Lui I'll wait for your go before I open anything. Verification will be the manual tier: a dev VM for compose and the dump script, throwaway GCP infra for hydration. Decisions for you: 1. A capture mismatch will be a hard failure, not a WARN. My default is yes. 2. The postgres twin key will stay `14`. My default is yes. I could not verify whether the postgres archive exists in the bucket today, because listing is denied for me.", false},
		{"question with options", true, "Two ways to do this. Option A keeps the check in the capture job and fails the run on mismatch. Option B logs a WARN and keeps going. Which do you want? I lean A.", false},
		{"blocked on credentials", true, "I can't list the binaries bucket, listing is denied for this account. Can you grant storage.objects.list on colony-binaries, or paste the listing here? I'm stopped until then.", false},
		{"pr opened", true, "PR opened: https://git.colony.camp/colony/colony/pulls/1412. CI is green. Ready for review.", false},
		{"nit fixed, merge word yours", true, "The loki reader-filter nit is fixed and pushed as 0d69b1cc on #1398: the filter matches -loki.json again, exactly as before the PR. Review thread asked to re-check. Merge word is yours.", false},
		{"answer to a question", true, "Yes, with one precision: the grader's shell runs outside k8s, in the problem container that hosts the k3d cluster, as root. Same container, two vantage points.", false},
		{"let me check", true, "Let me go check the workflow config.", true},
		{"on it", true, "On it. Cloning main and re-verifying the six claims line by line now.", true},
		{"redoing after feedback", true, "Fair on both counts. Redoing it as a diagram page that starts from why the twin exists and defines each term, no infra knowledge assumed.", true},
		{"future ask while continuing", true, "One thing I'll need from you eventually: the bucket name for the postgres archive. Not blocking yet, I'm doing the capture-side change first and will ask again when I get to the twin.", true},
		{"watching ci", true, "Pushed the fix. Watching CI, will report when it finishes.", true},
		{"user thanks", false, "thanks!", false},
		{"user fyi", false, "fyi I merged the manifest PR, no action needed", false},
		{"user hold off", false, "hold off on this for now, we'll revisit next week", false},
		{"user go ahead", false, "go ahead with A", true},
		{"user merge it", false, "merge it", true},
		{"user question", false, "why did you drop the hydrator digest check?", true},
		{"user follow-up request", false, "can you also update the README while you're in there?", true},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			got, err := c.Working(ctx, row.message, row.fromAgent)
			if err != nil {
				t.Fatalf("Working: %v", err)
			}
			if got != row.want {
				t.Errorf("working = %v, want %v", got, row.want)
			}
		})
	}
}
