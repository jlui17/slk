#!/usr/bin/env bash
# Run this checkout's slk inside docker with the TUI attached to the current
# terminal — for Santa-lockdown hosts that kill locally built binaries.
#
# Config and cached tokens are seeded ONCE from the host into the
# slk-test-state volume (never the live cache.db: copying a WAL database
# mid-write can tear it, and slk rebuilds the cache from the API). Every pane
# that runs this script shares that volume, and the containers share the
# docker VM's kernel, so cross-process flocks are exercised for real.
# Reseed with: docker volume rm slk-test-state
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
image=slk-go:1.26

docker volume create slk-test-state >/dev/null
docker run --rm \
  -v slk-test-state:/state \
  -v "$HOME/.config/slk":/seed/config:ro \
  -v "$HOME/.local/share/slk/tokens":/seed/tokens:ro \
  "$image" sh -c '
    [ -e /state/.seeded ] && exit 0
    mkdir -p /state/xdg/config/slk /state/xdg/data/slk/tokens /state/xdg/cache
    cp /seed/config/config.toml /state/xdg/config/slk/
    cp /seed/tokens/*.json /state/xdg/data/slk/tokens/
    touch /state/.seeded'

bin="$repo/bin/slk-linux"
if [ ! -x "$bin" ] || [ -n "$(find "$repo/cmd" "$repo/internal" -name '*.go' -newer "$bin" -print -quit 2>/dev/null)" ]; then
  echo "building linux slk..." >&2
  docker run --rm -v "$repo":/src -w /src \
    -v slk-gomodcache:/go/pkg/mod -v slk-gobuildcache:/root/.cache/go-build \
    -e GOFLAGS=-buildvcs=false \
    "$image" go build -o bin/slk-linux ./cmd/slk
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
docker run --rm -it \
  -v "$repo":/src \
  -w /src \
  -v slk-test-state:/state \
  -v "$spool":/host-open \
  -e BROWSER=/src/tools/spool-open \
  ${tz:+-e TZ="$tz"} \
  ${SLK_DEBUG:+-e SLK_DEBUG="$SLK_DEBUG"} \
  ${bridge_port:+-e HERDR_ENV=1 -e HERDR_PANE_ID="$HERDR_PANE_ID" -e SLK_HERDR_ADDR="host.docker.internal:$bridge_port"} \
  ${bridge_port:+${HERDR_TAB_ID:+-e HERDR_TAB_ID="$HERDR_TAB_ID"}} \
  -e XDG_CONFIG_HOME=/state/xdg/config \
  -e XDG_DATA_HOME=/state/xdg/data \
  -e XDG_CACHE_HOME=/state/xdg/cache \
  -e TERM="${TERM:-xterm-256color}" \
  ${TERM_PROGRAM:+-e TERM_PROGRAM="$TERM_PROGRAM"} \
  ${COLORTERM:+-e COLORTERM="$COLORTERM"} \
  ${KITTY_WINDOW_ID:+-e KITTY_WINDOW_ID="$KITTY_WINDOW_ID"} \
  ${COLORTERM_CELL_WIDTH:+-e COLORTERM_CELL_WIDTH="$COLORTERM_CELL_WIDTH"} \
  ${COLORTERM_CELL_HEIGHT:+-e COLORTERM_CELL_HEIGHT="$COLORTERM_CELL_HEIGHT"} \
  "$image" /src/bin/slk-linux "$@"
