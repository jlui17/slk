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
cleanup() {
  [ -n "$watcher_pid" ] && kill "$watcher_pid" 2>/dev/null
  rm -rf "$spool"
}
trap cleanup EXIT
# A PID-targeted signal must not skip cleanup and orphan the watcher;
# `exit` re-routes through the EXIT trap. Bash delivers these traps
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

# Terminal identity rides into the container so graphics-protocol detection
# sees the real terminal; the kitty probe's reply comes back over the -it pty.
# -w /src puts slk-debug.log (written to cwd under SLK_DEBUG) in the
# host checkout instead of dying with the --rm container. No exec: the EXIT
# trap must run to stop the spool watcher.
docker run --rm -it \
  -v "$repo":/src \
  -w /src \
  -v slk-test-state:/state \
  -v "$spool":/host-open \
  -e BROWSER=/src/tools/spool-open \
  ${tz:+-e TZ="$tz"} \
  ${SLK_DEBUG:+-e SLK_DEBUG="$SLK_DEBUG"} \
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
