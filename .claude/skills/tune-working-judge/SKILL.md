---
name: tune-working-judge
description: Use when an agent thread's status in the herdr agents panel was wrong (blocked/red when the agent only reported or kept working, working when the thread waited for Justin or the agent went quiet, idle when the agent had work), when Justin asks to check or improve the working judge, or before editing the judge prompts in internal/tablabel/working.go or the status rules in internal/ui/agentworking*.go.
---

# Tuning the working judge

The status slk shows for an agent thread comes from a few deterministic
rules and, for the shapes they can't decide, a one-letter verdict from a
small model that reads only the newest message. Every status is logged to
`agent_state_reports` in cache.db with the `source` that decided it. Tuning
is a loop: the log names a wrong status, the source names the layer to fix,
and a row in the live test holds the fix. A prompt edit is judged by
running it, never by reading it: each wording change moves some other
borderline message.

## 1. Collect the wrong statuses

Run `tools/agent-state-report.sh` (Justin's sessions; pass `slk-agent-state`
for agent sessions). Its lists are candidates, not errors: they join the
log to cached messages, so a thread nobody reopened shows `none yet` even
if the agent replied. A thread Justin names outranks the lists; find its
rows by channel and thread_ts.

Read each candidate's message before calling it wrong. The judge saw the
message's rich_text blocks with their lines (`workingJudgeSource`), which
the flat `messages.text` has lost; copy the db out the way the script does
to read it.

Done when every candidate is marked wrong or right, and each wrong one has
its `source`, `judge_reply` and `judge_prompt_hash` written down.

## 2. Let `source` pick the layer

| source | What decided | Where the fix goes |
|---|---|---|
| `judge` | the model; `judge_reply` is its letter and reason | the prompts, step 3 |
| `unacked_human`, `todo_post`, `judge_pending`, `assistant_status` | a rule in `derived()` / `effective()` | internal/ui/agentworking*.go, with a unit test; no live test needed |
| `working_expired` | silence outlasted `agentWorkingExpiry` | the period; the script's "expired, but the agent then posted again" list is the evidence that it is too short |
| `judge_error` | the request failed and the row read idle | the `error` text says why |

A `judge` row under an older `judge_prompt_hash` may already be fixed. Add
it as a row (step 3) and run it against the current prompt before editing
anything.

Some wrong statuses are right by the prompt's own definitions: blocked
means the agent has stopped and cannot continue until the user answers, so
a question with a stated default while the agent continues is working.
Changing a definition changes what every status means. That is Justin's
decision; bring him the rows and ask.

## 3. Fix a prompt with the live test

`TestWorkingLive` in internal/tablabel/live_test.go is the eval. It costs a
few cents of Haiku per run, which needs no permission.

```bash
SLK_TABLABEL_LIVE=1 tools/go.sh test ./internal/tablabel -run '^TestWorkingLive$' -count=1 -v > <scratch>/live.txt 2>&1
```

Rows run in parallel, so read replies from the saved file, matched to
their row names.

1. **Rows first.** Add the wrong message as a row, paraphrased (Slack text
   never goes in the repo) and with its line structure kept. Add two or
   three near neighbours on both sides of the boundary ("merge it", "lgtm,
   merge", and "lgtm" alone). Keep at least one neighbour out of the
   prompt's own examples, so a pass shows the rule generalised and was not
   fitted to a string.
2. **Baseline.** Run the full test before editing, so a later flip is
   known to be yours.
3. **Read the reason.** The failing reply names the category that won.
   Most failures are an overlap: two categories in the prompt both fit the
   message and nothing says which wins ("merge it" is a go-ahead and also
   approval of finished work). Close the overlap or state the priority.
   Another carve-out example fixes one phrasing and leaves the overlap.
4. **Iterate on a subset** (`-run '^TestWorkingLive$/(user_.*)$'`), then
   run everything: a row far from the edit can flip.

Done when every row passes in three consecutive full runs. Temperature 0
makes replies nearly repeatable, not fully: a borderline row has flipped
between two runs of one prompt.

When a row will not hold without contorting the prompt, leave the prompt,
keep the evidence, and tell Justin. A message the newest-message-only
input cannot decide needs more input, not more wording, and that is a
design change.

## 4. Ship and check back

The commit message says what now reads differently and the before and
after counts (n of m runs), like the earlier judge commits. After the
merge, Justin's panes need the new build (the `refresh-sessions` skill);
from then on rows carry the new `judge_prompt_hash`, which separates
before from after. The fix is confirmed when the report shows the pattern
gone under the new hash.
