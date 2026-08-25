# Developing on a Santa host

On machines managed by [Santa](https://santa.dev) (a macOS binary authorizer)
in Lockdown mode, `go test` and `go run` die with `signal: killed`, and each
death pops a notification. Every `go` build produces a fresh ad-hoc-signed
binary with a new hash, so no allowlist rule can cover them; Santa kills the
binary the moment it runs. Compiling is unaffected — only running the fresh
binary is blocked.

Two wrapper scripts run those binaries inside docker instead, where Santa
can't see them. Both detect Santa by the presence of `santactl` and behave as
plain native runs on unmanaged machines, so they're safe to use everywhere.

## `tools/go.sh` — any go command

```sh
tools/go.sh test ./... -race
tools/go.sh build ./...
```

Runs the identical `go` command in a container with the checkout mounted and
module/build caches in shared named volumes, so repeat runs are warm. On
first use it builds the `slk-go` image: the stock `golang` image plus
`libx11-dev`, because `golang.design/x/clipboard` compiles C that needs X11
headers, and `-race` needs cgo. `GOOS`/`GOARCH`/`CGO_ENABLED` are forwarded,
so cross-builds behave the same as native.

## `tools/run-docker.sh` — the slk TUI itself

```sh
tools/run-docker.sh
```

Builds a linux binary of this checkout (cached, rebuilt when sources change)
and runs it with the TUI attached to your terminal. Your config and cached
workspace tokens are seeded **once** from the host into a `slk-test-state`
docker volume — never the live `cache.db`, which slk rebuilds from the API.
slk's keyring re-mint fails inside linux and falls back to the cached tokens,
so auth just works.

Run it in two panes and both instances share the volume on the docker VM's
kernel, so cross-process file locking behaves exactly as it does natively.
Reset the sandbox with `docker volume rm slk-test-state`.

### Headless use

Without a host tty (agent shells, CI) the script drops to `docker run -t`:
the app still gets a pty, stdin stays detached, so `--version`,
`--list-workspaces`, `--dump-sections` and bounded TUI runs work
non-interactively. `SLK_LOG_DIR=<host dir>` makes that dir the container
cwd so `slk-debug.log` outlives the `--rm` container, and
`SLK_TIMEOUT=<secs>` wraps slk in `timeout -s INT` — SIGINT quits tea
cleanly, which is what makes slk write its shutdown API request tally.

`tools/smoke.sh [secs]` composes all of that into the standing end-to-end
check: boot every workspace against real Slack in the agent sandbox for a
bounded window, then assert clean shutdown (exactly one tally), zero
connect failures, and zero reconnect catch-up passes, printing the
per-endpoint request tally for eyeballing. It keeps the debug log on
failure. Near-read-only: boot sends one `conversations.mark` for the
restored channel, nothing else.

### Agent isolation

Agent sessions (Claude Code shells export `CLAUDECODE`; `SLK_ROLE=agent|user`
overrides) get a parallel sandbox that can't disturb an interactive one:
their own state volume (`slk-agent-state`, reset with
`docker volume rm slk-agent-state`, seeded from the user sandbox's config
and tokens when `slk-test-state` exists so agent runs see the state the
user's working session has), their own binary
(`bin/slk-linux-agent`), and containers named `slk-agent-*`. The separate
binary matters: a rebuild writes the file a running container executes over
the bind mount, which can crash it, so agent builds never touch
`bin/slk-linux`. Both scripts also label their containers `slk.role=agent`
or `slk.role=user` — agent-side cleanup kills only containers it started (by
recorded name), and must never touch anything not labeled `slk.role=agent`.

Link opening (`o`) works through a bridge: the container has no browser, so
slk honors `$BROWSER`, which the script points at `tools/spool-open` — it
drops the URL into a `/tmp` spool directory that a host-side watcher in the
script opens with `open`.

**Caveat:** the docker pty reports no pixel dimensions, so the TIOCGWINSZ
path for cell metrics always fails. slk recovers by querying the terminal
directly (XTWINOPS `CSI 16t`), which rides through the docker pty; only a
terminal that ignores that query too falls back to 8x16 and renders images
at low resolution. `COLORTERM_CELL_WIDTH` / `COLORTERM_CELL_HEIGHT` remain
as a manual override (the script forwards them).

## Native alternative

Where your Santa deployment accepts local rules, allowlist a built binary by
hash:

```sh
make build
sudo santactl rule --allow --identifier "$(shasum -a 256 bin/slk | cut -d' ' -f1)"
```

Every rebuild changes the hash and needs a new rule, and deployments synced
to a management server may reject or later remove local rules — the docker
path has neither problem.
