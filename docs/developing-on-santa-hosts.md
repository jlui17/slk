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

Runs the identical `go` command by `docker exec` in the role's shared slk
backend (see below), where every checkout and worktree is mounted and the
module/build caches live in shared named volumes, so repeat runs are warm.
Ctrl+C stops the command inside the container too; a live model run (next
section) gets a container of its own, since it needs a config mount. On
first use it builds the `slk-go` image: the stock `golang` image plus
`libx11-dev`, because `golang.design/x/clipboard` compiles C that needs X11
headers, and `-race` needs cgo. `GOOS`/`GOARCH`/`CGO_ENABLED` are forwarded,
so cross-builds behave the same as native.

### Live model tests

`internal/tablabel`'s live tests call the real Anthropic API with the model,
effort and key from slk's `config.toml`, and skip unless
`SLK_TABLABEL_LIVE=1`. The agent status eval judges every case in
`internal/tablabel/testdata/agent_status_eval.json` and prints a pass table:

```sh
tools/agent-status-eval.sh          # this checkout's judge
tools/agent-status-eval.sh main     # main's judge, with this checkout's cases
SLK_AGENT_STATUS_EVAL_RUNS=3 SLK_AGENT_STATUS_EVAL_EFFORT=low tools/agent-status-eval.sh
```

The host `config.toml` is not what live sessions use: `run-docker.sh` seeds
the state volume's config once and syncs only the key after that, so a
session can judge at another effort than the tests read. Set
`SLK_AGENT_STATUS_EVAL_EFFORT` (and `_MODEL`) to test what sessions run.

## `tools/run-docker.sh` — the slk TUI itself

```sh
tools/run-docker.sh
```

Builds a linux binary of this checkout (cached, rebuilt when sources change)
and runs it with the TUI attached to your terminal. Your config and cached
workspace tokens are seeded **once** from the host into a `slk-test-state`
docker volume — never the live `cache.db`, which slk rebuilds from the API.
Only `anthropic_api_key` is copied in again on every launch, so a key rotated
in the host config reaches a session when it restarts.
slk's keyring re-mint fails inside linux and falls back to the cached tokens,
so auth just works.

Run it in two panes and both instances share the volume, so cross-process
file locking behaves exactly as it does natively.

**Reset a sandbox only while its backend runs nothing.** The idle backend
holds the role's volume, so it goes first, and with it every session in
that role: `slk-agent-backend` also serves other agents' sessions. Check
it, then remove both; the next launch recreates them and reseeds:

```sh
docker exec slk-user-backend sh -c 'grep -lz SLK_EXEC_ID= /proc/[0-9]*/environ'   # prints nothing when idle
docker rm -f slk-user-backend && docker volume rm slk-test-state
```

For the agent sandbox, the same with `slk-agent-backend` and
`slk-agent-state`.

### The shared backend

Every launch of a role runs by `docker exec` in one long-lived container,
`slk-<role>-backend`. The first launch creates it: the main checkout
mounted at its host path (so every worktree under it runs there too), the
role's state volume, the go caches, and `~/.cache/slk-sandbox/<backend>/`
for per-launch files. It
sleeps between launches and comes back after a docker restart. Launches
that start together race to create it; the losers wait for the winner.

Each launch carries `SLK_EXEC_ID=<launcher pid>-<random>` in its
environment, and every process it starts inherits it. That id is how a
launch is found and stopped:

- **An exec'd process outlives its `docker exec` client** when a pane
  closes or its launcher dies, so the launch's `tools/host-bridge.py`
  waits on the launcher and then runs `tools/stop-exec.sh <id>` inside the
  backend: SIGINT to the launch's root process (slk's shutdown path),
  SIGKILL to anything left after 5s.
- `tools/refresh-sessions.sh` reads the id from each pane's `docker exec`
  argv and stops its sessions the same way.

Check it by hand:

```sh
docker exec slk-user-backend ps -e -o pid,etime,rss,args   # one slk per pane
docker exec slk-user-backend sh -c 'grep -lz SLK_EXEC_ID= /proc/[0-9]*/environ'   # one per launch process
```

A label (`slk.config`) and the image ID record how the backend was made.
**When `run-docker.sh` or the image changes, a launch recreates the backend
only while nothing runs in it**; otherwise it warns and uses it as it is.
A refresh stops every session first, so its first relaunch is when the
change takes effect.

### Headless use

Without a host tty (agent shells, CI) the script drops to `docker exec -t`:
the app still gets a pty, stdin stays detached, so `--version`,
`--list-workspaces`, `--dump-sections` and bounded TUI runs work
non-interactively. `SLK_LOG_DIR=<host dir>` collects `slk-debug.log` there
instead of in the checkout (slk writes it in its launch directory, and the
script moves it on exit), and
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
their own state volume (`slk-agent-state`, reset as above, seeded from the
user sandbox's config
and tokens when `slk-test-state` exists so agent runs see the state the
user's working session has), their own binary
(`bin/slk-linux-agent`), and their own backend, `slk-agent-backend`. The
separate binary matters: a rebuild writes the file a running session
executes over the bind mount, which can crash it, so agent builds never
touch `bin/slk-linux`. Both scripts also label their containers
`slk.role=agent` or `slk.role=user` — agent-side cleanup stops only what it
started (a launch by its exec id, a container by recorded name), and must
never touch anything not labeled `slk.role=agent`. `slk-agent-backend` is
shared by every agent session, so leave it running.

Link opening (`o`) works through a bridge: the container has no browser, so
slk honors `$BROWSER`, which the script points at `tools/spool-open` — it
drops the URL into the launch's directory, which `tools/host-bridge.py`
watches and opens on the host with `open`.

Ctrl+V image paste works through a second bridge: the container can't see
the macOS clipboard, so the script starts `tools/host-bridge.py`, whose
clipboard bridge is a host-side HTTP server that dumps the clipboard through
`osascript` (Apple-signed, so Santa allows it), and passes its address in as
`SLK_CLIPBOARD_ADDR`. The same process relays the herdr socket over TCP
(`SLK_HERDR_ADDR`), so a launch pays Python's startup once. slk then reads the clipboard from that address
instead of the native library. Each paste costs one `osascript` launch
(~0.25s), which runs off the UI goroutine while the status bar shows
`Pasting from clipboard…`. Check it by hand: `python3 tools/host-bridge.py $$ clipboard` prints
`clipboard=<port>`; `curl http://127.0.0.1:<port>/image > x.png` returns the
clipboard image (204 when there is none).

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
