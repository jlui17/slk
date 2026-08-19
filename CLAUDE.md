# slk

## Running Go on Santa-managed hosts

This machine runs Santa, which SIGKILLs locally built binaries (every `go
build`/`go test`/`go run` artifact has a fresh ad-hoc signature no allowlist
rule can cover — the symptom is `signal: killed` / exit 137). Never invoke
`go` or a locally built binary directly:

- Any go command: `tools/go.sh <args>` (runs go in docker on Santa hosts,
  execs native go elsewhere).
- Running the slk TUI itself: `tools/run-docker.sh`.

Details: `docs/developing-on-santa-hosts.md`.
