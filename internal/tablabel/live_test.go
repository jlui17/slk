package tablabel

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
		// A thread's root is judged alone at every boot, before the replies
		// load. Saying that later work waits for the user's word asks nothing
		// now.
		{"kickoff root: a session will start and align first", true, "*Sim env issue 36: the model's processes run on into grading.* A background process that the model starts can rewrite its report after the episode ends, and in a QA probe this raised a 0.0 answer to 1.0. It's a P0 in the sim env triage, assigned to @.Justin. A Claude session will start in this thread and align on the approach before it writes code.", VerdictWorking},
		{"kickoff root: a session will debrief, no fix work before his word", true, "*A Claude session will start here to debrief Justin on a CI runner bug, and it will do no fix work before his word.* An ephemeral Gitea CI runner can delete its own VM just after it takes a job, and the job then fails later with no error text. The evidence is in this PR comment. Justin asked for this thread in the model pod hardening thread.", VerdictWorking},
		{"user thanks", false, "thanks!", VerdictIdle},
		{"user fyi", false, "fyi I merged the manifest PR, no action needed", VerdictIdle},
		{"user hold off", false, "hold off on this for now, we'll revisit next week", VerdictIdle},
		{"user go ahead", false, "go ahead with A", VerdictWorking},
		{"user merge it", false, "merge it", VerdictWorking},
		{"user question", false, "why did you drop the hydrator digest check?", VerdictWorking},
		{"user follow-up request", false, "can you also update the README while you're in there?", VerdictWorking},
	}
	judge := func(name string, earlier []string, message string, fromAgent bool, want Verdict) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			got, err := c.Judge(ctx, message, earlier, fromAgent)
			if err != nil {
				t.Fatalf("Judge: %v", err)
			}
			if got != want {
				t.Errorf("verdict = %v, want %v", got, want)
			}
		})
	}
	// Production nearly always has messages before the newest one, so every
	// agent-side row runs a second time after the most common opening. A
	// newest message that reports a result completes that "On it", so the
	// verdict must not move.
	askAndAck := []string{"user: can you take a look at this and fix it?", "agent: On it. I'll report back here when it's done."}
	for _, row := range rows {
		judge(row.name, nil, row.message, row.fromAgent, row.want)
		if row.fromAgent {
			judge("after ask and ack: "+row.name, askAndAck, row.message, true, row.want)
		}
	}

	// Rows whose verdict needs the messages before the newest one. The
	// first is a real thread: two PRs in flight, and a newest message that
	// reports one of them merged while the message before it says the agent
	// is still fixing the other.
	const (
		reviewOf2281  = "agent: For the author session of WIP PR #2281 (the 3 \"Right after the kill\" lines): looks good to me, and nothing is left at head `1bd1702a` (review 4037). CI is 11 of 11 success. Each new line agrees with `SetupGrader`, and no other line in the repo has the claim. Justin's typed words of 18:54Z and 18:55Z here cover this PR, so the merge is your step: `WIP:` off, then your gates (Gitea shows `mergeable: false` only because of the prefix). Justin has nothing to do now. The detail is in the #gitea review thread."
		reviewOf2282  = "agent: For the author session of PR #2282 (the rename and doc fix row in `who-permits-a-merge.md`): round 1 is \"changes needed\" at `dbac0354`, with 3 must-fixes, 5 should-fixes and 1 nit, in review 4038. Justin has nothing to do now. The most important one: \"a PR that a Claude session wrote\" also covers a teammate's PR. A cold reader merged a PR that Eitan's local Claude Code wrote under `eitank`, and Justin's answer 2 was \"not other authors' PRs\". The text also drops \"I tell the person after the merge\", which was in the proposal that he answered with \"sure\". The review has 2 questions that you must put to Justin: if \"a teammate agreed\" means all 6 teammates or only him, and if a large rewrite \"for clarity\" is a doc fix. After your push, tell me in the #gitea review thread. The merge needs a teammate's typed merge word that names #2282."
		fixing2282    = "agent: The PR for the standing permission is up and in review: WIP PR #2282. It adds the rename and doc fix row to `who-permits-a-merge.md`, written with the managing-context skill. The reviewer tested the text on 22 fresh sessions and found 3 places where it gives more than you typed (one session merged a PR from Eitan's local Claude Code). I'm fixing those now. The reviewer has 2 questions for you. I write the narrow reading into the PR now, so you only need to answer if you want it wider: 1. *Who must agree the new wording before I merge with no ask: only you, or any of the 6 teammates in their own thread?* Narrow default: only you. The permission is yours, and your words were \"as long as we're aligned\". 2. *Is a large rewrite of a doc \"for clarity\" a doc fix?* Narrow default: no. A doc fix corrects or clarifies sentences that exist. A new doc, a new section or a rewrite still gets a merge ask. Next: after the reviewer says that nothing is left, I ask you for \"merge 2282\" with the exact final text, because this PR changes merge policy. One more doc fix, PR #2281 (3 lines that said \"right after the kill\"), is reviewed and merges now on your standing word."
		merged2281    = "PR #2281, the doc fix of the 3 \"right after the kill\" lines, is merged as `bc05fc5f` on your standing word for doc fixes. Before the merge the reviewer said that nothing is left, CI was 11 of 11 green, and the diff was still only 2 doc lines and 1 code comment. I need nothing from you for it."
		mergedBoth    = "PR #2282 is merged too, on your typed \"merge 2282\". Both doc PRs are merged now, and you have nothing to do."
		blockingQuery = "Which reading do you want for question 1? I'm stopped until you answer."
	)
	threadRows := []struct {
		name    string
		earlier []string
		message string
		want    Verdict
	}{
		{"one PR merged, the earlier message is still fixing the other", []string{reviewOf2281, reviewOf2282, fixing2282}, merged2281, VerdictWorking},
		{"the newest message completes the earlier step", []string{"agent: On it. Cloning main and re-verifying the six claims now."}, "PR opened: #1412. CI is green. Ready for review.", VerdictIdle},
		{"the newest message closes every earlier step", []string{reviewOf2281, reviewOf2282, fixing2282}, mergedBoth, VerdictIdle},
		{"the newest message asks a blocking question", []string{fixing2282}, blockingQuery, VerdictBlocked},
	}
	for _, row := range threadRows {
		judge(row.name, row.earlier, row.message, true, row.want)
	}

	// A second real thread, with two pieces of work: a runner fix whose
	// agent holds a step for this evening (proofNext), and a CI fix whose
	// merge waits on the user's typed word (mergeAsk). The ask decides.
	const (
		statusAsk   = "user: status update"
		whatIssue   = "user: wats that issue ^"
		prQuestion  = "agent: That post is from the PR nanny, and it is a separate fault from the runner self-delete. Nothing is broken now. Each CI job downloads all Go modules from proxy.golang.org, because the module cache never hits on the ephemeral runners. On Sep 28 and again today that download broke mid-stream, so the build of main went red before any compile or test. A re-run fixed it each time (3 red runs in 2 days). The fix is one env line in the 3 Go workflows. With the pipe, Go falls back to a direct download on any proxy error; the default comma form does not: ```GOPROXY=https://proxy.golang.org|direct``` *Justin: do you want a PR for this change?* On a yes, I will start a work session for it. With no answer, nothing changes, and the nanny only records repeats."
		proofTodos  = "agent: On your \"status update\": this thread is for the bug where an ephemeral CI runner took a job after its VM's own delete call. The fix, #2225, merged yesterday and has been on the pool's template since 3:29 AM PDT. The proof on real pool VMs is still open, and I'm reading it now, one hour after the work window opened. I need nothing from you. The Go download fault above is the PR nanny's topic, and its answer covers it. ✱ Read the first idle deletes on VMs from the new template: the runner must exit before the delete call. ✱ Count the jobs that died with their VM since 3:29 AM PDT. ○ Post the result here. _todos as of 17:33 UTC_"
		driveIt     = "user: sure yes just drive that to completion code-nanny"
		mergeScope  = "Will do. A work session started in this thread; it will open the PR, and the PR nanny tracks it to the merge. The PR link and the result will come here. Done means: the 3 Go workflows carry the pipe-form `GOPROXY`, CI is green, and a CI job log prints the new value. *Justin: does \"drive to completion\" include the merge, when the review has nothing left and CI is green?* Our merge rule needs your typed word for that. With no answer, the PR will wait at ready-to-merge."
		proofNext   = "Result for the self-delete fix (#2225) on real pool VMs: it works so far, with 0 dead jobs since the new template went live at 3:29 AM PDT. I need nothing from you. • *Numbers:* 57 VMs from the new template, 46 ephemeral jobs, 49 self-deletes. No job died with its VM, and no runner took a job after a delete call. • *Idle deletes:* 5, all clean. Each VM was gone about 52 seconds after its call, and none had taken a job. • *After-job deletes:* 43 of 43 came 90 to 94 seconds after the job's end, as before. • *Work hours:* the 8 new VMs print \"at warm minimum, staying\" each minute, and the pool holds at 10. • *Not proven yet:* the sample is small. The old script killed about 1 job in 200 self-deletes, so 0 in 49 can't show the rate. Also, the idle guard's notes exist only on a VM's serial console, which goes away with the VM, so no log shows \"runner stopped, then delete call\" on a pool VM. Your Claude Code's test VM run did show that order. Next: this evening, when idle deletes start again, I'll read one VM's serial console live to see the order on a pool VM. On 10/1 I'll run the two-day audit log join for the rate. I'll post both here."
		prUp        = "The WIP PR for the Go module download failures is up, and CI is green: #2278, Go CI jobs fall back to a direct module download when proxy.golang.org fails, Kaneo task colony-1796. I need nothing new from you; the nanny's merge question above is still open. • *Behavior:* 4 Go workflows set `GOPROXY: \"https://proxy.golang.org|direct\"` at workflow level. On any proxy error, Go downloads the module from its source repo; go.sum checks do not change. • *One more file than the ask:* `gitea-ci-injection-e2e.yml` also runs `go test` after `setup-go`, and it is a required check. Say if you want it out. • *Proof:* all 5 Go job logs print the new value, such as Sim City / test: `GOPROXY='https://proxy.golang.org|direct'`. A probe with a dead proxy fails with the comma form and passes with the pipe form. • *Not covered:* the `docker build` steps download modules inside the image build, where the workflow env does not reach. The review ask is in this #gitea thread. I will post the verdict here."
		mergeAsk    = "@Justin Lui #2278, Go CI jobs fall back to a direct module download when proxy.golang.org fails, is ready for the merge, and it waits only on you. *Justin: type \"merge\" and I will remove WIP from the title and make one squash merge of #2278.* With no answer, the PR stays open as WIP and nothing merges. • *Review:* a different session said \"nothing left\" at the head commit d268935c after 2 rounds. Round 1 had text-only findings: one doc paragraph now owns the reason for the pipe form, and the PR body names the 4 failed jobs correctly. • *CI:* 13 of 13 contexts are green at that head (read again at 19:10 UTC). • *Verification that I ran:* the head's Sim City / test log prints `GOPROXY='https://proxy.golang.org|direct'`, as do the 4 other Go jobs. A dead-proxy probe fails with the comma form and passes with the pipe form. The reviewer also cut a zip download mid-stream on a local proxy, the real error class of the failed runs: the pipe form passed. • *Not run:* a direct download on a CI runner. If it fails there, the job fails as it does today. • *One limit:* the permission checker blocked my edit of the PR title just now. If it also blocks it after your word, I will hand you the click: remove `WIP:` and press merge."
		agentPrefix = "agent: "
	)
	askRows := []struct {
		name    string
		earlier []string
		message string
		want    Verdict
	}{
		{"merge ask alone", nil, mergeAsk, VerdictBlocked},
		{"merge ask, while an earlier message holds a step for this evening", []string{proofTodos, driveIt, agentPrefix + mergeScope, agentPrefix + proofNext, agentPrefix + prUp}, mergeAsk, VerdictBlocked},
		{"ack that also asks whether the go covers the merge", []string{statusAsk, whatIssue, prQuestion, proofTodos, driveIt}, mergeScope, VerdictBlocked},
		{"PR up, asks nothing new and will post the review verdict", []string{prQuestion, proofTodos, driveIt, agentPrefix + mergeScope, agentPrefix + proofNext}, prUp, VerdictWorking},
		{"result with its own next steps", []string{whatIssue, prQuestion, proofTodos, driveIt, agentPrefix + mergeScope}, proofNext, VerdictWorking},
	}
	for _, row := range askRows {
		judge(row.name, row.earlier, row.message, true, row.want)
	}

	// The first thread again, later: the merge ask (mergeAsk2282) is still
	// open when the newest message answers the user's question back, so the
	// thread still needs the user. An answer to the ask closes it.
	const (
		reviewDone2282  = "agent: For the author session of PR #2282 (the rename and doc fix row in `who-permits-a-merge.md`): looks good to me, and nothing is left at head `56f2c27c` (review 4054). CI is 10 of 10 success, the merge commit has no change but the version line, and the policy text is byte-equal to the text of round 3. The next steps are yours: `WIP:` off, your gates, and Justin's typed merge word that names #2282. No standing row covers this PR, and my review is no permission. The ask must show him the exact final text, his 2 open questions with the narrow readings, the boundaries from the Notes of review 4051, and the word count (820 -> 1098). Compare the plugin version with `main` again right before the merge call. If the head moves in a line that the PR changes, tell me in the #gitea review thread."
		mergeAsk2282    = "agent: @Justin Lui PR #2282, the standing permission for rename and doc fix PRs, is ready to merge. *Type \"merge 2282\" and I'll merge it.* It changes merge policy, so your word must name it. The new row in `who-permits-a-merge.md`, written with the managing-context skill: > A rename or doc fix PR from the `claude` account that the session of the PR's work thread wrote for a teammate's ask, when Justin's own typed text in that thread agreed a new wording, or a direction for it, that the thread states | Justin's standing permission (2026-09-29): no word per PR, within the limits below. The limits, in plain words: • A doc fix corrects or clarifies sentences that exist. A new doc, a new section or a rewrite still gets an ask. A rename needs a proof that the reviewer can run again. • Not covered: a change of behavior (also text that a program prints), `.tf`, a rule in agent context that is new, changed or made clearer, and any text or code on merge policy, safety, permissions, hooks or CI gates. A corrected fact in a skill (a path, a command name) is covered. • \"Any wording is fine\" or an early \"go ahead\" is not your agreement. Another session must review the PR. Only the author session merges, and it tells the work thread at once. Two readings are narrow on purpose; say so if you want them wider: 1. Only your typed agreement on the wording counts, not the other 5 teammates'. 2. A skill's `description:` line gets an ask, and a rewrite gets an ask. How it was tested: the reviewer and I gave the text to fresh sessions with 30 cases. In the first rounds a session merged a PR from Eitan's local Claude Code, a 300-line rewrite, and a waiver. With the final text, 30 of 30 cases end on the meant side, and the reviewer says that nothing is left (review 4054). CI is green. The cost: the always-loaded file grows from 820 to 1,098 words. After the merge: a Claude organization owner uploads the plugin again, and I post the settings card for the allow rule, as you said."
		clarifyQuestion = "user: to clarify, this allows u to merge prs that are renames/doc fixes on ur own if we agreed on wording/language"
		clarifyAnswer   = "Yes, that is what it does. After PR #2282, a Claude session merges its own rename or doc fix PR with no ask, when you agreed the wording or its direction in that PR's thread. Three conditions stay, and they are the same as for every merge: another Claude session reviewed the PR and says that nothing is left, CI is green, and the session tells you in the thread right after the merge. One detail that my ask left out: if a new session takes over a thread, it counts as that thread's session and can merge there too. It doesn't merge anything else alone. A PR that changes behavior, `.tf`, or a rule still gets an ask. The PR still waits for your typed \"merge 2282\"."
	)
	openAskRows := []struct {
		name    string
		earlier []string
		message string
		want    Verdict
	}{
		{"answer to a question back that restates the ask, alone", nil, clarifyAnswer, VerdictBlocked},
		{"answer to a question back that does not restate the ask, the merge ask before it still open", []string{reviewDone2282, mergeAsk2282, clarifyQuestion}, strings.TrimSuffix(clarifyAnswer, " The PR still waits for your typed \"merge 2282\"."), VerdictBlocked},
		{"answer to a question back, the merge ask before it still open", []string{reviewDone2282, mergeAsk2282, clarifyQuestion}, clarifyAnswer, VerdictBlocked},
		{"the merge ask answered with the word, and merged", []string{reviewDone2282, mergeAsk2282, "user: merge 2282"}, "PR #2282 is merged as `abc1234` on your typed word. CI was green at the head, and I need nothing from you.", VerdictIdle},
		{"the merge ask answered with a no", []string{mergeAsk2282, "user: no, hold off on this one for now"}, "Understood. #2282 stays open as WIP, and I do nothing more on it until you say so.", VerdictIdle},
	}
	for _, row := range openAskRows {
		judge(row.name, row.earlier, row.message, true, row.want)
	}
}
