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
  # The runner needs liveClients and New with an effort, both added in
  # 22e56fe1; an older judge fails to compile with it.
  if ! git merge-base --is-ancestor 22e56fe1 "$1"; then
    echo "$1 predates 22e56fe1, the earliest commit the eval runs on" >&2
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
