#!/usr/bin/env bash
# SessionStart hook: front-loads the one-time slk-go image build and tells the
# agent up front when this checkout can't run tests. Warn-only. Warnings go to
# stdout because that is what Claude Code adds to the session's context; a
# SessionStart hook's stderr never reaches Claude.
set -u

cd "$(dirname "$0")/../.."

# tools/go.sh needs docker only on Santa hosts; elsewhere it execs native go.
if command -v santactl >/dev/null 2>&1 && ! docker info >/dev/null 2>&1; then
  echo "slk: docker unavailable — test tooling (tools/go.sh) needs it on Santa hosts"
  exit 0
fi
tools/go.sh vet ./... >/dev/null || echo "slk: tools/go.sh vet failed; test tooling is broken in this worktree"
