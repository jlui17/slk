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
		{"report with own next step", true, "The WIP PR is up, and I need nothing from you now: #2225, the ephemeral runner stops its poll before it deletes its VM, Kaneo task colony-1748. CI is green.\n\n• *Behavior:* the idle guard does its stay checks, sends SIGTERM to the runner, waits up to 30 seconds, then looks again. A taken task means no delete, and the VM deletes itself after the job. A runner that is still up also means no delete. The ephemeral role gets `shutdown_timeout: 120m`; the shared role keeps `0s`, and its startup script is byte-identical.\n• *Proof:* 6 new self-delete tests fail on main and pass on the branch (196 runner tests pass, shellcheck clean). A dry run of the old script deletes the VM under a task; the new one doesn't.\n• *Not run:* no VM has run the new stop order. The PR body holds the recipe for one test VM, for your \"canary ok\" or your Claude Code.\n• *Two choices I made, say if you want them changed:* when the idle delete call fails (such as a 403), the VM no longer stays with a stopped runner; the after-job path removes it 90 seconds later. And the Kaneo task has you as assignee, because the PR title check needs one.\nNext I'll ask for the review in #gitea, and I'll post here again at the verdict. The merge stays your typed word.", VerdictWorking},
		{"next step held by another agent session", true, "The second read of #2228 (terraform comments) is done, and nothing is left to fix at `322f57fa` (verdict in #gitea). The author session will merge it, and Justin has nothing to do.", VerdictWorking},
		{"relay: the author session will run the gates and merge", true, "Nothing is left to fix on #2231 (Dockerfile and Cloud Build comments) after my second read (review on the PR). The author session will refresh 2 stale facts in the PR body, will run the pre-merge gates, and will merge. Justin has nothing to do.", VerdictWorking},
		{"merged, other PRs wait for reviewers", true, "#2227 (comments and texts that name the slack-twinner binary) is merged, 3 of 5. You have nothing to do. What I checked is in this PR comment.\n\nOne behavior change to know: the binary's warning about orphan replies now starts with `slack cut:` and not `slack twinner:`. No filter in the repo, Grafana or Cloud Logging of the Colony project reads the old text. A log metric made by hand in the Sandbox project is the one place that nobody could check.\n\nThe last 2 PRs wait for the reviewers' second reads.", VerdictWorking},
		{"relay: the author session still has fixes", true, "The review of #2229 is done: it looks good at `d4b82254`, and the author session has 3 should-fixes and 12 nits in my review on the PR (verdict in #gitea).\n\nJustin, the PR's question for you (where a time travel adapter pushes its images) has an answer in the code: the composer pulls each image from one fixed registry path. What's left for you, with no hurry, is whether the comment states that path as the rule.", VerdictWorking},
		{"last PR merged, nothing left", true, "#2231 (Dockerfile and Cloud Build comments) is merged, 5 of 5. All the cleanup PRs are merged now, and you have nothing to do. What I checked is in this PR comment.", VerdictIdle},
		{"user thanks", false, "thanks!", VerdictIdle},
		{"user fyi", false, "fyi I merged the manifest PR, no action needed", VerdictIdle},
		{"user hold off", false, "hold off on this for now, we'll revisit next week", VerdictIdle},
		{"user go ahead", false, "go ahead with A", VerdictWorking},
		{"user merge it", false, "merge it", VerdictWorking},
		{"user question", false, "why did you drop the hydrator digest check?", VerdictWorking},
		{"user follow-up request", false, "can you also update the README while you're in there?", VerdictWorking},
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
			if got != row.want {
				t.Errorf("verdict = %v, want %v", got, row.want)
			}
		})
	}
}
