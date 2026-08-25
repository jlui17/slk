# slk

## Fork layout: fork code lives in fork-only files

This repo is a fork of gammons/slk that tracks `upstream/main`. Fork-added
declarations (funcs, methods, types, tests) never land in upstream files:
put them in a sibling `<base>_fork.go` / `<base>_fork_test.go`, or a
descriptively named file for a whole feature. Upstream files carry only what
can't move — hook lines, struct/interface members, switch arms, in-place
behavior changes — and prefer a one-line hook into a fork-file helper over
an inline block. `tools/fork-footprint.sh` shows the current churn on
upstream-owned files; don't grow it where a fork-only file would do. Merge
runbook and the list of intentionally diverged files: `docs/fork.md`.

## Branch workflow

Squash-merge every feature branch into main (`git merge --squash <branch>`,
then one commit named for the feature): main carries one commit per feature,
never a merge commit from a local branch. Merges *from* `upstream/main` are
the exception: keep those as true merge commits.

## Running Go on Santa-managed hosts

This machine runs Santa, which SIGKILLs locally built binaries (every `go
build`/`go test`/`go run` artifact has a fresh ad-hoc signature no allowlist
rule can cover — the symptom is `signal: killed` / exit 137). Never invoke
`go` or a locally built binary directly:

- Any go command: `tools/go.sh <args>` (runs go in docker on Santa hosts,
  execs native go elsewhere).
- Running the slk TUI itself: `tools/run-docker.sh`.

Agent sessions are auto-isolated from the user's live slk: `run-docker.sh`
detects `CLAUDECODE` and uses its own state volume (`slk-agent-state`),
its own binary (`bin/slk-linux-agent`), and `slk-agent-*` container names
with a `slk.role=agent` label. Never build to `bin/slk-linux` and never
kill/stop/rm any docker container or volume that isn't one this session
created and isn't labeled `slk.role=agent` — the user's live session runs
from the same image and checkout.

Details: `docs/developing-on-santa-hosts.md`.

## Never run `go fmt` across the tree

`tools/go.sh` runs go 1.26 in docker, whose gofmt reflows doc comments that
older toolchains wrote — `go fmt ./...` rewrites ~35 files it has no other
reason to touch. This fork tracks `upstream/main`, so reformatting files we
don't own buys nothing and conflicts with every future upstream merge. If
`go fmt` touches a file you didn't edit, revert it.
