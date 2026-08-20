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

# Docker Desktop does not forward bind-mounted unix sockets across its VM
# boundary, so the herdr socket rides in as host.docker.internal:<port> via a
# host-side TCP bridge. The bridge watches this shell's PID so it can't
# outlive the run even if the EXIT trap never fires.
bridge_pid=
bridge_port=
if [ "${HERDR_ENV:-}" = 1 ] && [ -n "${HERDR_SOCKET_PATH:-}" ] && [ -n "${HERDR_PANE_ID:-}" ]; then
  port_file=$(mktemp)
  python3 "$repo/tools/herdr-bridge.py" "$HERDR_SOCKET_PATH" $$ >"$port_file" &
  bridge_pid=$!
  trap 'kill "$bridge_pid" 2>/dev/null || true' EXIT
  for _ in $(seq 1 50); do
    bridge_port=$(head -n 1 "$port_file")
    [ -n "$bridge_port" ] && break
    sleep 0.1
  done
  rm -f "$port_file"
  if [ -z "$bridge_port" ]; then
    echo "herdr bridge did not report a port" >&2
    exit 1
  fi
fi

# Terminal identity rides into the container so graphics-protocol detection
# sees the real terminal; the kitty probe's reply comes back over the -it pty.
# -w /src puts slk-debug.log (written to cwd under SLK_DEBUG) in the
# host checkout instead of dying with the --rm container.
run_opts=(--rm -it
  -v "$repo":/src
  -w /src
  -v slk-test-state:/state
  ${tz:+-e TZ="$tz"}
  ${SLK_DEBUG:+-e SLK_DEBUG="$SLK_DEBUG"}
  -e XDG_CONFIG_HOME=/state/xdg/config
  -e XDG_DATA_HOME=/state/xdg/data
  -e XDG_CACHE_HOME=/state/xdg/cache
  -e TERM="${TERM:-xterm-256color}"
  ${TERM_PROGRAM:+-e TERM_PROGRAM="$TERM_PROGRAM"}
  ${COLORTERM:+-e COLORTERM="$COLORTERM"}
  ${KITTY_WINDOW_ID:+-e KITTY_WINDOW_ID="$KITTY_WINDOW_ID"}
  ${COLORTERM_CELL_WIDTH:+-e COLORTERM_CELL_WIDTH="$COLORTERM_CELL_WIDTH"}
  ${COLORTERM_CELL_HEIGHT:+-e COLORTERM_CELL_HEIGHT="$COLORTERM_CELL_HEIGHT"}
  ${bridge_port:+-e HERDR_ENV=1 -e HERDR_PANE_ID="$HERDR_PANE_ID" -e SLK_HERDR_ADDR="host.docker.internal:$bridge_port"}
)

if [ -z "$bridge_pid" ]; then
  exec docker run "${run_opts[@]}" "$image" /src/bin/slk-linux "$@"
fi

# No exec here: the bridge must be reaped after docker run returns, and the
# EXIT trap needs this shell alive to fire.
code=0
docker run "${run_opts[@]}" "$image" /src/bin/slk-linux "$@" || code=$?
exit "$code"
