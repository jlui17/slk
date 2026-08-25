#!/usr/bin/env bash
# Run this checkout's slk inside docker with the TUI attached to the current
# terminal — for Santa-lockdown hosts that kill locally built binaries.
#
# Config and cached tokens are seeded ONCE from the host into the role's
# state volume (never the live cache.db: copying a WAL database mid-write can
# tear it, and slk rebuilds the cache from the API). Every pane that runs this
# script in the same role shares that volume, and the containers share the
# docker VM's kernel, so cross-process flocks are exercised for real.
# Reseed with: docker volume rm slk-test-state (or slk-agent-state)
#
# Agent sessions (CLAUDECODE set, or SLK_ROLE=agent) run fully isolated from
# interactive use: their own state volume, their own binary, and containers
# named/labeled by role. The separate binary is load-bearing, not tidiness:
# a rebuild writes the file a live container is executing over the bind
# mount, which can crash it — so an agent build must never touch the file a
# user session runs. Cleanup must only ever match label slk.role=agent.
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
image=slk-go:1.26

role=${SLK_ROLE:-${CLAUDECODE:+agent}}
role=${role:-user}
case "$role" in
  agent) state_vol=slk-agent-state; bin_name=slk-linux-agent ;;
  user)  state_vol=slk-test-state;  bin_name=slk-linux ;;
  *) echo "SLK_ROLE must be 'agent' or 'user', got '$role'" >&2; exit 1 ;;
esac

# The agent sandbox seeds from the user sandbox when one exists (config and
# tokens only, never cache.db) so agent runs exercise the exact state the
# user's working session has; a fresh machine falls back to the host files.
# (${arr[@]+"${arr[@]}"} below: macOS ships bash 3.2, where expanding an
# empty array under set -u dies with "unbound variable".)
seed_vol=()
if [ "$role" = agent ] && docker volume inspect slk-test-state >/dev/null 2>&1; then
  seed_vol=(-v slk-test-state:/seed-vol:ro)
fi

docker volume create "$state_vol" >/dev/null
docker run --rm \
  -v "$state_vol":/state \
  ${seed_vol[@]+"${seed_vol[@]}"} \
  -v "$HOME/.config/slk":/seed/config:ro \
  -v "$HOME/.local/share/slk/tokens":/seed/tokens:ro \
  "$image" sh -c '
    set -e
    [ -e /state/.seeded ] && exit 0
    mkdir -p /state/xdg/config/slk /state/xdg/data/slk/tokens /state/xdg/cache
    if [ -f /seed-vol/xdg/config/slk/config.toml ]; then
      cp /seed-vol/xdg/config/slk/config.toml /state/xdg/config/slk/
      cp /seed-vol/xdg/data/slk/tokens/*.json /state/xdg/data/slk/tokens/
    else
      cp /seed/config/config.toml /state/xdg/config/slk/
      cp /seed/tokens/*.json /state/xdg/data/slk/tokens/
    fi
    touch /state/.seeded'

bin="$repo/bin/$bin_name"
if [ ! -x "$bin" ] || [ -n "$(find "$repo/cmd" "$repo/internal" -name '*.go' -newer "$bin" -print -quit 2>/dev/null)" ]; then
  echo "building linux slk ($bin_name)..." >&2
  docker run --rm -v "$repo":/src -w /src \
    -v slk-gomodcache:/go/pkg/mod -v slk-gobuildcache:/root/.cache/go-build \
    -e GOFLAGS=-buildvcs=false \
    "$image" go build -ldflags="-s -w" -trimpath -o "bin/$bin_name" ./cmd/slk
fi

# The container has no host timezone, so timestamps render as UTC unless the
# host zone rides in; the image's Debian tzdata resolves the name.
tz="${TZ:-$(readlink /etc/localtime | sed 's|.*/zoneinfo/||')}"


# The container has no browser, so the `o` keybinding's launch can't happen
# inside it. slk honors $BROWSER; point it at tools/spool-open, which drops
# the URL into a spool directory this watcher opens on the host. The spool
# must live under /tmp — Docker Desktop shares it with the VM by default.
spool=$(mktemp -d /tmp/slk-open.XXXXXX)
watcher_pid=
bridge_pid=
cleanup() {
  [ -n "$watcher_pid" ] && kill "$watcher_pid" 2>/dev/null
  [ -n "$bridge_pid" ] && kill "$bridge_pid" 2>/dev/null
  rm -rf "$spool"
}
trap cleanup EXIT
# A PID-targeted signal must not skip cleanup and orphan the watcher or the
# bridge; `exit` re-routes through the EXIT trap. Bash delivers these traps
# only once the foreground docker run returns.
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
(
  while :; do
    for f in "$spool"/url-*; do
      [ -e "$f" ] || continue
      url=$(cat "$f")
      rm -f "$f"
      # `|| true`: a bad URL must not kill the watcher (set -e is
      # inherited by the subshell).
      open "$url" || true
    done
    sleep 0.3
  done
) &
watcher_pid=$!

# Docker Desktop does not forward bind-mounted unix sockets across its VM
# boundary, so the herdr socket rides in as host.docker.internal:<port> via a
# host-side TCP bridge. The bridge watches this shell's PID so it can't
# outlive the run even if the EXIT trap never fires. Best-effort like the
# reporter itself: a bridge that fails to start only costs the sidebar
# integration, never the launch.
bridge_port=
if [ "${HERDR_ENV:-}" = 1 ] && [ -n "${HERDR_SOCKET_PATH:-}" ] && [ -n "${HERDR_PANE_ID:-}" ]; then
  port_file=$(mktemp)
  python3 "$repo/tools/herdr-bridge.py" "$HERDR_SOCKET_PATH" $$ >"$port_file" &
  bridge_pid=$!
  for _ in $(seq 1 50); do
    bridge_port=$(head -n 1 "$port_file")
    [ -n "$bridge_port" ] && break
    sleep 0.1
  done
  rm -f "$port_file"
  if [ -z "$bridge_port" ]; then
    echo "warning: herdr bridge did not report a port; starting without the agent-sidebar integration" >&2
    kill "$bridge_pid" 2>/dev/null || true
    bridge_pid=
  fi
fi

# Terminal identity rides into the container so graphics-protocol detection
# sees the real terminal; the kitty probe's reply comes back over the -it pty.
# -w /src puts slk-debug.log (written to cwd under SLK_DEBUG) in the
# host checkout instead of dying with the --rm container. No exec: the EXIT
# trap must run to stop the spool watcher and the bridge.
#
# Headless hooks (tools/smoke.sh composes all three):
#   no host tty  -> -t only: the app still gets a pty, stdin stays detached
#   SLK_LOG_DIR  -> host dir mounted as the container cwd, so slk-debug.log
#                   survives the --rm container somewhere other than /src
#   SLK_TIMEOUT  -> wraps slk in `timeout -s INT <secs>` for a bounded run
#                   that still shuts down cleanly (INT -> tea quit -> tally)
tty_args=(-it)
[ -t 0 ] || tty_args=(-t)
workdir=/src
log_mount=()
if [ -n "${SLK_LOG_DIR:-}" ]; then
  workdir=/log
  log_mount=(-v "$SLK_LOG_DIR":/log)
fi
cmd=(/src/bin/"$bin_name")
[ -n "${SLK_TIMEOUT:-}" ] && cmd=(timeout -s INT "$SLK_TIMEOUT" "${cmd[@]}")
# GOMEMLIMIT: image-decode bursts on a warm cache measured a 974MB RSS
# peak per instance from GC lazily returning pages; the soft ceiling
# trades brief GC pressure during those bursts for a bounded footprint
# when many instances run at once. Wrong if a legitimately live heap
# approaches it (sustained GC thrash) — raise it then.
docker run --rm "${tty_args[@]}" \
  --name "slk-${role}-$$" \
  --label "slk.role=${role}" \
  -v "$repo":/src \
  -w "$workdir" \
  ${log_mount[@]+"${log_mount[@]}"} \
  -v "$state_vol":/state \
  -v "$spool":/host-open \
  -e BROWSER=/src/tools/spool-open \
  ${tz:+-e TZ="$tz"} \
  ${SLK_DEBUG:+-e SLK_DEBUG="$SLK_DEBUG"} \
  ${ANTHROPIC_API_KEY:+-e ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY"} \
  ${bridge_port:+-e HERDR_ENV=1 -e HERDR_PANE_ID="$HERDR_PANE_ID" -e SLK_HERDR_ADDR="host.docker.internal:$bridge_port"} \
  ${bridge_port:+${HERDR_TAB_ID:+-e HERDR_TAB_ID="$HERDR_TAB_ID"}} \
  ${bridge_port:+${HERDR_WORKSPACE_ID:+-e HERDR_WORKSPACE_ID="$HERDR_WORKSPACE_ID"}} \
  -e GOMEMLIMIT=400MiB \
  -e XDG_CONFIG_HOME=/state/xdg/config \
  -e XDG_DATA_HOME=/state/xdg/data \
  -e XDG_CACHE_HOME=/state/xdg/cache \
  -e TERM="${TERM:-xterm-256color}" \
  ${TERM_PROGRAM:+-e TERM_PROGRAM="$TERM_PROGRAM"} \
  ${COLORTERM:+-e COLORTERM="$COLORTERM"} \
  ${KITTY_WINDOW_ID:+-e KITTY_WINDOW_ID="$KITTY_WINDOW_ID"} \
  ${COLORTERM_CELL_WIDTH:+-e COLORTERM_CELL_WIDTH="$COLORTERM_CELL_WIDTH"} \
  ${COLORTERM_CELL_HEIGHT:+-e COLORTERM_CELL_HEIGHT="$COLORTERM_CELL_HEIGHT"} \
  "$image" "${cmd[@]}" "$@"
