#!/usr/bin/env bash
# Run this checkout's slk inside docker with the TUI attached to the current
# terminal — for Santa-lockdown hosts that kill locally built binaries.
#
# Config and cached tokens are seeded ONCE from the host into the role's
# state volume (never the live cache.db: copying a WAL database mid-write can
# tear it, and slk rebuilds the cache from the API). The one exception is the
# config's anthropic_api_key, which every launch copies in again
# (tools/sync-host-api-key.sh). Every pane that runs this
# script in the same role shares that volume, so cross-process flocks are
# exercised for real.
# Reseed by removing the role's backend and then its volume, only while the
# backend runs no launch: docs/developing-on-santa-hosts.md has the check.
#
# Every launch of a role runs by `docker exec` in the role's one
# long-lived container, slk-<role>-backend, and the launch's host bridge
# stops what it started there once the launcher is gone. Roles (user,
# agent) and the backend: tools/backend.sh.
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd -P)
. "$repo/tools/backend.sh"

# The seeder container costs ~0.5-1s per launch on Docker Desktop, so a host-side
# stamp keyed by the volume's CreatedAt skips it once seeded. Removing the
# volume changes CreatedAt, so a stamp that outlives its volume can never skip
# seeding a fresh one; the stamp is written only after the seeder succeeds, so
# a concurrent or failed seed just runs the (idempotent) seeder again.
stamp="$stamp_dir/seeded-$state_vol"
if ! vol_created=$(docker volume inspect -f '{{ .CreatedAt }}' "$state_vol" 2>/dev/null); then
  docker volume create "$state_vol" >/dev/null
  vol_created=$(docker volume inspect -f '{{ .CreatedAt }}' "$state_vol")
fi
if [ "$(cat "$stamp" 2>/dev/null)" != "$vol_created" ]; then
  # The agent sandbox seeds from the user sandbox when one exists (config and
  # tokens only, never cache.db) so agent runs exercise the exact state the
  # user's working session has; a fresh machine falls back to the host files.
  # (${arr[@]+"${arr[@]}"} below: macOS ships bash 3.2, where expanding an
  # empty array under set -u dies with "unbound variable".)
  seed_vol=()
  if [ "$role" = agent ] && docker volume inspect slk-test-state >/dev/null 2>&1; then
    seed_vol=(-v slk-test-state:/seed-vol:ro)
  fi

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
  mkdir -p "$stamp_dir"
  printf '%s\n' "$vol_created" >"$stamp"
fi

ensure_backend

bin="$repo/bin/$bin_name"
if [ ! -x "$bin" ] || [ -n "$(find "$repo/cmd" "$repo/internal" "$repo/go.mod" "$repo/go.sum" \( -name '*.go' -o -name go.mod -o -name go.sum \) -newer "$bin" -print -quit 2>/dev/null)" ]; then
  echo "slk: $bin_name missing or older than source, rebuilding in docker (10-60s)..." >&2
  docker exec -w "$repo" -e GOFLAGS=-buildvcs=false "$backend" \
    go build -ldflags="-s -w" -trimpath -o "bin/$bin_name" ./cmd/slk
fi

# The container has no host timezone, so timestamps render as UTC unless the
# host zone rides in; the image's Debian tzdata resolves the name.
tz="${TZ:-$(readlink /etc/localtime | sed 's|.*/zoneinfo/||')}"


# A run either draws the TUI or runs a CLI subcommand. Only the TUI opens
# links and pastes, so the browser spool watcher and the clipboard bridge
# are TUI-only; the herdr bridge also serves every `slk herdr` command,
# which drive herdr's socket API. Mirrors main.go's dispatch: no args or a
# permalink launches the TUI; `herdr`, `help`, `version`, and anything
# starting with `-` do not. A named subcommand missing here only starts
# slower, it does not break.
tui=1
case "${1:-}" in
  herdr|help|version|-*) tui= ;;
esac
herdr_bridge=$tui
[ "${1:-}" = herdr ] && herdr_bridge=1

# The ID marks every process this launch starts in the backend, so the
# bridge (and tools/refresh-sessions.sh) can stop exactly those. The
# launch's directory under $launches holds its link-open spool: the
# container has no browser, so slk's $BROWSER, tools/spool-open, drops each
# URL there for the host bridge to open.
exec_id=$$-$RANDOM$RANDOM
spool=$launches/$exec_id
mkdir "$spool"
signalled=
bridge_pid=
cleanup() {
  # Signalled (a pane closing, a kill), this shell has half a second before
  # herdr's SIGKILL, less than slk may take to shut down. The bridge, out
  # of the pane's session, outlives it and does the whole job once this
  # shell is gone: stop the launch in the backend, hand slk's debug log
  # (written in the spool under SLK_LOG_DIR) over, remove the spool. This
  # shell does it only when slk exited on its own, so the log is in place
  # when the script returns, or when the bridge is dead: the stop is then
  # the backstop nothing else would run. Running both is safe, since
  # stop-exec.sh signals a run only once.
  trap '' HUP INT TERM
  if [ -n "$signalled" ] && kill -0 "$bridge_pid" 2>/dev/null; then
    return
  fi
  docker exec "$backend" sh "$repo/tools/stop-exec.sh" "$exec_id" >/dev/null 2>&1 || true
  if [ -n "${SLK_LOG_DIR:-}" ]; then
    mv "$spool/slk-debug.log" "$SLK_LOG_DIR/" 2>/dev/null || true
  fi
  rm -rf "$spool"
}
trap cleanup EXIT
# A PID-targeted signal must not skip cleanup; `exit` re-routes through the
# EXIT trap. Bash delivers these traps only once the foreground docker exec
# returns.
trap 'signalled=1; exit 129' HUP
trap 'signalled=1; exit 130' INT
trap 'signalled=1; exit 143' TERM

# One host-side Python process per launch (tools/host-bridge.py) serves what
# the container can't reach on its own, the first two on ports it reaches
# as host.docker.internal:<port>:
#  - herdr: Docker Desktop does not forward bind-mounted unix sockets across
#    its VM boundary, so the herdr socket rides in over a TCP relay.
#  - clipboard: Ctrl+V smart paste reads the macOS clipboard over HTTP.
#    Darwin only: the bridge reads the clipboard with osascript.
#  - spool: it opens each URL slk drops in the spool.
#  - exec: an exec'd slk outlives its docker client, so when this shell is
#    gone, however it went, the bridge stops the launch in the backend.
# That last job is why a launch without its bridge does not start.
bridges=("exec=$backend:$exec_id:$spool")
if [ -n "$herdr_bridge" ] && [ "${HERDR_ENV:-}" = 1 ] && [ -n "${HERDR_SOCKET_PATH:-}" ] && [ -n "${HERDR_PANE_ID:-}" ]; then
  bridges+=("herdr=$HERDR_SOCKET_PATH")
fi
if [ -n "$tui" ] && [ "$(uname)" = Darwin ]; then
  bridges+=(clipboard)
fi
[ -n "$tui" ] && bridges+=("spool=$spool")
port_file=$(mktemp)
python3 "$repo/tools/host-bridge.py" $$ "${bridges[@]}" >"$port_file" &
bridge_pid=$!
ports=
ready=
for _ in $(seq 1 500); do
  read -r ports <"$port_file" && ready=1 && break
  sleep 0.01
done
rm -f "$port_file"
if [ -z "$ready" ]; then
  echo "host bridge did not start; not launching, since nothing would stop this launch's slk in $backend once it is gone" >&2
  exit 1
fi
herdr_port=
clipboard_port=
for p in $ports; do
  case "$p" in
    herdr=*) herdr_port=${p#herdr=} ;;
    clipboard=*) clipboard_port=${p#clipboard=} ;;
  esac
done

# Terminal identity rides into the container so graphics-protocol detection
# sees the real terminal; the kitty probe's reply comes back over the -it pty.
# The checkout is the cwd, so slk-debug.log (written to cwd under SLK_DEBUG)
# lands in it. No exec: the EXIT trap must run to clean up.
#
# Headless hooks (tools/smoke.sh composes all three):
#   no host tty  -> -t only: the app still gets a pty, stdin stays detached
#   SLK_LOG_DIR  -> host dir slk-debug.log lands in instead of the checkout,
#                   moved there from the launch's spool when slk exits
#   SLK_TIMEOUT  -> wraps slk in `timeout -s INT <secs>` for a bounded run
#                   that still shuts down cleanly (INT -> tea quit -> tally)
tty_args=(-it)
[ -t 0 ] || tty_args=(-t)
workdir=$repo
[ -n "${SLK_LOG_DIR:-}" ] && workdir=$spool
cmd=("$repo/bin/$bin_name")
# --foreground: slk is not PID 1, and timeout would otherwise move it out of
# the tty's foreground process group (SIGTTIN on read).
[ -n "${SLK_TIMEOUT:-}" ] && cmd=(timeout --foreground -s INT "$SLK_TIMEOUT" "${cmd[@]}")
# A key rotated in the host config reaches the volume's copy before slk
# starts. The key rides in by name, so argv stays secret-free; exec keeps
# slk the launch's root process, and a failed copy never blocks the launch.
SLK_HOST_ANTHROPIC_API_KEY=$(sed -n 's/^anthropic_api_key *= *"\(.*\)"$/\1/p' "$HOME/.config/slk/config.toml" 2>/dev/null || true)
export SLK_HOST_ANTHROPIC_API_KEY
cmd=(sh -c '"$0" || true; exec "$@"' "$repo/tools/sync-host-api-key.sh" "${cmd[@]}")
# GOMEMLIMIT: image-decode bursts on a warm cache measured a 974MB RSS
# peak per instance from GC lazily returning pages; the soft ceiling
# trades brief GC pressure during those bursts for a bounded footprint
# when many instances run at once. Wrong if a legitimately live heap
# approaches it (sustained GC thrash) — raise it then.
echo "container: $backend" >&2
echo "exec: $exec_id" >&2
docker exec "${tty_args[@]}" \
  -w "$workdir" \
  -e SLK_EXEC_ID="$exec_id" \
  -e SLK_SPOOL_DIR="$spool" \
  -e BROWSER="$repo/tools/spool-open" \
  ${tz:+-e TZ="$tz"} \
  ${SLK_DEBUG:+-e SLK_DEBUG="$SLK_DEBUG"} \
  ${ANTHROPIC_API_KEY:+-e ANTHROPIC_API_KEY} \
  ${SLK_HOST_ANTHROPIC_API_KEY:+-e SLK_HOST_ANTHROPIC_API_KEY} \
  ${herdr_port:+-e HERDR_ENV=1 -e HERDR_PANE_ID="$HERDR_PANE_ID" -e SLK_HERDR_ADDR="host.docker.internal:$herdr_port"} \
  ${herdr_port:+${HERDR_TAB_ID:+-e HERDR_TAB_ID="$HERDR_TAB_ID"}} \
  ${herdr_port:+${HERDR_WORKSPACE_ID:+-e HERDR_WORKSPACE_ID="$HERDR_WORKSPACE_ID"}} \
  ${clipboard_port:+-e SLK_CLIPBOARD_ADDR="host.docker.internal:$clipboard_port"} \
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
  "$backend" "${cmd[@]}" "$@"
