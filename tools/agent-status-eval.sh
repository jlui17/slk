#!/usr/bin/env bash
# Runs the agent status eval (internal/tablabel/agent_status_eval_test.go)
# against the real model.
#
#   tools/agent-status-eval.sh          # this checkout's judge
#   tools/agent-status-eval.sh <ref>    # the judge at <ref>, with this
#                                       # checkout's dataset and runner
#
# SLK_AGENT_STATUS_EVAL_RUNS, _MODEL and _EFFORT pass through (see the test).
set -euo pipefail

repo=$(git rev-parse --show-toplevel)
eval_files=(internal/tablabel/agent_status_eval_test.go internal/tablabel/testdata/agent_status_eval.json)
dir=$repo

if [[ $# -gt 0 ]]; then
  # The runner calls Judge with a tablabel.Author and the user's Claude
  # Code's most recent post, both added with Author; an older judge fails
  # to compile with it. To measure an older judge, run its own runner and
  # dataset at that ref with no argument.
  if ! git grep -q '^type Author string' "$1" -- internal/tablabel/working.go; then
    echo "$1 predates tablabel.Author, the earliest judge this runner and dataset run on" >&2
    exit 1
  fi
  main=$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")
  dir=$main/.claude/worktrees/agent-status-eval-ref-$$-$(date +%s)
  git worktree add --detach --quiet "$dir" "$1"
  trap 'git -C "$repo" worktree remove --force "$dir"' EXIT
  mkdir -p "$dir/internal/tablabel/testdata"
  for f in "${eval_files[@]}"; do cp "$repo/$f" "$dir/$f"; done
fi

cd "$dir/internal/tablabel"
SLK_TABLABEL_LIVE=1 "$repo/tools/go.sh" test . -run '^TestAgentStatusEval$' -count=1 -v -timeout 30m
