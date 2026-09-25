package tablabel

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gammons/slk/internal/config"
	toml "github.com/pelletier/go-toml/v2"
)

// TestRelabelLive hits the real Anthropic API with a root-only transcript,
// what a thread opened before anyone replied sends; set SLK_TABLABEL_LIVE=1
// to run it. The key is the one slk itself uses (see liveClient); tools/go.sh
// carries the gate, the config file and the env key into the docker
// container it runs tests in on Santa hosts.
func TestRelabelLive(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id, label, err := c.Relabel(ctx,
		"justin: colony-562 the flow viewer renders stale runs after a reconnect, can you fix it", nil)
	if err != nil {
		t.Fatalf("Relabel: %v", err)
	}
	t.Logf("live id: %q label: %q", id, label)
	if id != "colony-562" {
		t.Errorf("id = %q, want the root's task id", id)
	}
	if label == "" || utf8.RuneCountInString(label) > 60 {
		t.Errorf("label %q outside expected shape", label)
	}
}

func liveClient(t *testing.T) *Client {
	t.Helper()
	if os.Getenv("SLK_TABLABEL_LIVE") == "" {
		t.Skip("set SLK_TABLABEL_LIVE=1 to hit the real API")
	}
	apiKey := liveAPIKey(t)
	if apiKey == "" {
		t.Fatal("no API key: set herdr.anthropic_api_key in slk's config.toml, or ANTHROPIC_API_KEY")
	}
	return New("claude-haiku-4-5", apiKey)
}

// liveAPIKey finds the key where slk does, reading only what it needs:
// config.toml is decoded without config.Load's workspace validation, since
// a live test is not slk startup. The path restates xdgConfig (cmd/slk),
// which can't be imported. A missing file is an empty config.
func liveAPIKey(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	var cfg config.Config
	data, err := os.ReadFile(filepath.Join(dir, "slk", "config.toml"))
	if err == nil {
		err = toml.Unmarshal(data, &cfg)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read slk config: %v", err)
	}
	return cfg.Herdr.ResolveAnthropicAPIKey()
}

// TestWorkingLive pins the working judge's verdicts on the message shapes
// the deterministic signal can't read, against the real model.
func TestWorkingLive(t *testing.T) {
	c := liveClient(t)
	rows := []struct {
		name      string
		fromAgent bool
		message   string
		want      Verdict
	}{
		{"plan awaiting go", true, "Plan for gitea, verified against current main. Two PRs. Justin Lui I'll wait for your go before I open anything. Verification will be the manual tier: a dev VM for compose and the dump script, throwaway GCP infra for hydration. Decisions for you: 1. A capture mismatch will be a hard failure, not a WARN. My default is yes. 2. The postgres twin key will stay `14`. My default is yes. I could not verify whether the postgres archive exists in the bucket today, because listing is denied for me.", VerdictBlocked},
		{"question with options", true, "Two ways to do this. Option A keeps the check in the capture job and fails the run on mismatch. Option B logs a WARN and keeps going. Which do you want? I lean A.", VerdictBlocked},
		{"blocked on credentials", true, "I can't list the binaries bucket, listing is denied for this account. Can you grant storage.objects.list on colony-binaries, or paste the listing here? I'm stopped until then.", VerdictBlocked},
		{"pr opened", true, "PR opened: https://git.colony.camp/colony/colony/pulls/1412. CI is green. Ready for review.", VerdictIdle},
		{"nit fixed, merge word yours", true, "The loki reader-filter nit is fixed and pushed as 0d69b1cc on #1398: the filter matches -loki.json again, exactly as before the PR. Review thread asked to re-check. Merge word is yours.", VerdictIdle},
		{"answer to a question", true, "Yes, with one precision: the grader's shell runs outside k8s, in the problem container that hosts the k3d cluster, as root. Same container, two vantage points.", VerdictIdle},
		{"done report with optional offer", true, "Issue 47 (the model can see the authproxy listener) is closed on the sheet. I changed 2 cells of its row: Done? is ticked, and Status has your text. My read of the 5 transcripts of Taiga job 218d9a4d passed first.\n\nThe old Status had 5 links and the new text has none. I kept the old text, so say if you want the links back.", VerdictIdle},
		{"let me check", true, "Let me go check the workflow config.", VerdictWorking},
		{"on it", true, "On it. Cloning main and re-verifying the six claims line by line now.", VerdictWorking},
		{"redoing after feedback", true, "Fair on both counts. Redoing it as a diagram page that starts from why the twin exists and defines each term, no infra knowledge assumed.", VerdictWorking},
		{"future ask while continuing", true, "One thing I'll need from you eventually: the bucket name for the postgres archive. Not blocking yet, I'm doing the capture-side change first and will ask again when I get to the twin.", VerdictWorking},
		{"watching ci", true, "Pushed the fix. Watching CI, will report when it finishes.", VerdictWorking},
		{"ack with review pending", true, "Will do. \"Same\" will mean one real hydration run at main and at the PR head on the same slot, with identical pushed digests and row contents, posted here before each merge. Review for #1461 is requested here.", VerdictWorking},
		{"all-done checklist", true, "Fixing the broken link on the landing page\n✓ Rendered the page and checked it by eye\n✓ Fixed and republished", VerdictIdle},
		{"all-done checklist, flattened with stamp", true, "Merging #1502 (replay order fix) ✓ Approval was at the current head. ✓ Merge gates passed: 5 of 5 required checks green. ✓ What-ran note posted below. ✓ Merged: squash 38c1f0a2 on main, source branch deleted. _todos as of 18:26 UTC_", VerdictIdle},
		{"all-done checklist that continues", true, "✓ Cloned main.\n✓ Reproduced the failure.\n✓ Fix pushed.\nAll three are done. Moving on to the dump script now.", VerdictWorking},
		{"all-done checklist that asks", true, "✓ Option A built on a branch.\n✓ Option B built on a branch.\nBoth pass CI. Which one should I open as the PR, A or B?", VerdictBlocked},
		{"done report ending in an optional question", true, "The flaky test is fixed and pushed as 3fa91c2 on #1520: the retry loop now waits for the count to settle. CI is green.\n\nWant me to also backport it to the release branch?", VerdictIdle},
		{"done report ending in a rhetorical question", true, "Found it. The cache key left out the workspace id, so two workspaces shared one entry. Fixed in #1533 and merged after your approval. Who would have guessed a one-word key could cost a day?", VerdictIdle},
		{"question with a default while continuing", true, "One open choice: should a capture mismatch be a hard failure or a WARN? I'm going with hard failure unless you say otherwise. Continuing with the dump script now.", VerdictWorking},
		{"stop and ask for an ok", true, "The branch is ready and CI is green, but the PR is still marked WIP. Type ok and I will remove the WIP prefix and request review.", VerdictBlocked},
		{"will read the job when its id is posted", true, "The Taiga run is kicked off. I will read the job's transcripts when its id is posted here, and report what I find.", VerdictWorking},
		{"user thanks", false, "thanks!", VerdictIdle},
		{"user fyi", false, "fyi I merged the manifest PR, no action needed", VerdictIdle},
		{"user hold off", false, "hold off on this for now, we'll revisit next week", VerdictIdle},
		{"user go ahead", false, "go ahead with A", VerdictWorking},
		{"user merge it", false, "merge it", VerdictWorking},
		{"user question", false, "why did you drop the hydrator digest check?", VerdictWorking},
		{"user follow-up request", false, "can you also update the README while you're in there?", VerdictWorking},
		{"user sounds good", false, "sounds good", VerdictIdle},
		{"user note to another person", false, "@Priya fyi this is the thread I mentioned, the fix should land tomorrow", VerdictIdle},
		{"user plain request", false, "rebase this onto main and rerun the hydration check", VerdictWorking},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			got, err := c.Judge(ctx, row.message, row.fromAgent)
			if err != nil {
				t.Fatalf("Judge: %v", err)
			}
			t.Logf("reply: %q", got.Reply)
			if got.Verdict != row.want {
				t.Errorf("verdict = %v, want %v", got.Verdict, row.want)
			}
		})
	}
}
