# Sourced by tools/run-docker.sh, tools/refresh-sessions.sh and tools/go.sh
# (bash, set -euo pipefail),
# with $repo set to the checkout's physical path: the role a run belongs
# to, and that role's shared backend container, slk-<role>-backend.
#
# Agent sessions (CLAUDECODE set, or SLK_ROLE=agent) run fully isolated from
# interactive use: their own state volume, their own binary, and containers
# named/labeled by role. The separate binary is load-bearing, not tidiness:
# a rebuild writes the file a live session is executing over the bind
# mount, which can crash it — so an agent build must never touch the file a
# user session runs. Cleanup must only ever match label slk.role=agent.

image=slk-go:1.26

role=${SLK_ROLE:-${CLAUDECODE:+agent}}
role=${role:-user}
case "$role" in
  agent) state_vol=slk-agent-state; bin_name=slk-linux-agent ;;
  user)  state_vol=slk-test-state;  bin_name=slk-linux ;;
  *) echo "SLK_ROLE must be 'agent' or 'user', got '$role'" >&2; exit 1 ;;
esac
# A caller that needs a sandbox of its own (tools/bench.py deletes cache.db
# between runs) names its volume; the role still picks binary and labels.
state_vol=${SLK_STATE_VOL:-$state_vol}
stamp_dir=${XDG_CACHE_HOME:-$HOME/.cache}/slk-sandbox

# The backend mounts the main checkout at its host path, so this checkout
# and every worktree under it run in the one container, at the same paths
# as on the host. Its name is the role's (plus a caller's own state volume,
# as tools/bench.py names one), so runs find it without any lookup. It
# sleeps between runs, costing nothing, and --restart brings it back after
# a docker restart. A container takes no new mounts, so each slk launch
# gets its own directory under one host directory the backend mounts at its
# host path ($launches): link-open spool, and the cwd that slk writes
# slk-debug.log to under SLK_LOG_DIR. The go caches are the named volumes
# every checkout shares.
#
# Every run in it carries SLK_EXEC_ID, which marks the processes it starts
# there; tools/stop-exec.sh stops a run by it.
main=$(cd "$repo" && cd "$(git rev-parse --git-common-dir)/.." && pwd -P)
backend=slk-${role}-backend${SLK_STATE_VOL:+-$SLK_STATE_VOL}
launches=$stamp_dir/$backend

# Creates the backend, or starts it when stopped. The slk.config label
# fingerprints everything it was created with but the image, whose ID is
# compared directly: a run that finds it stale recreates it while nothing
# runs in it, else warns and uses it as it is.
ensure_backend() {
  local backend_args config out image_id cid cimage running cconfig err
  case "$repo/" in
    "$main"/*) ;;
    *) echo "the shared backend needs this checkout under its main checkout ($main), got $repo" >&2; exit 1 ;;
  esac
  mkdir -p "$launches"
  backend_args=(--init --restart unless-stopped --label "slk.role=${role}"
    -v "$main":"$main" -v "$launches":"$launches" -v "$state_vol":/state
    -v slk-gomodcache:/go/pkg/mod -v slk-gobuildcache:/root/.cache/go-build)
  config=$(printf '%s\n' "${backend_args[@]}" | shasum | cut -c1-12)
  out=$(docker inspect -f '{{if index . "State"}}{{.Id}} {{.Image}} {{.State.Running}} {{index .Config.Labels "slk.config"}}{{else}}{{.Id}}{{end}}' \
    "$image" "$backend" 2>/dev/null || true)
  { read -r image_id; read -r cid cimage running cconfig; } <<<"$out" || true
  if [ -n "$cid" ] && { [ "$cimage" != "$image_id" ] || [ "$cconfig" != "$config" ]; }; then
    if [ "$running" = true ] && docker exec "$cid" sh -c 'grep -qlz ^SLK_EXEC_ID= /proc/[0-9]*/environ' 2>/dev/null; then
      echo "warning: $backend predates the current image or launcher; it is recreated on a run once nothing runs in it" >&2
      return
    fi
    # By ID: a run racing this one may already have replaced it.
    docker rm -f "$cid" >/dev/null 2>&1 || true
    cid=
  fi
  if [ -n "$cid" ]; then
    [ "$running" = true ] || docker start "$cid" >/dev/null
    return
  fi
  # Panes launched together race to create it; the losers wait for the
  # winner's container to run.
  if ! err=$(docker run -d --name "$backend" --label "slk.config=$config" "${backend_args[@]}" \
    "$image" sleep infinity 2>&1 >/dev/null); then
    case "$err" in
      *"is already in use"*) ;;
      *) echo "$err" >&2; exit 1 ;;
    esac
    for _ in $(seq 50); do
      [ "$(docker inspect -f '{{.State.Running}}' "$backend" 2>/dev/null)" = true ] && return
      sleep 0.1
    done
    echo "$backend did not start" >&2
    exit 1
  fi
}

# Builds this checkout's slk for the role in the backend when the binary is
# missing or older than any Go source. Every launch runs it, and so does
# tools/refresh-sessions.sh, once, while none of its sessions runs.
ensure_binary() {
  local bin="$repo/bin/$bin_name"
  if [ ! -x "$bin" ] || [ -n "$(find "$repo/cmd" "$repo/internal" "$repo/go.mod" "$repo/go.sum" \( -name '*.go' -o -name go.mod -o -name go.sum \) -newer "$bin" -print -quit 2>/dev/null)" ]; then
    echo "slk: $bin_name missing or older than source, rebuilding in docker (10-60s)..." >&2
    docker exec -w "$repo" -e GOFLAGS=-buildvcs=false "$backend" \
      go build -ldflags="-s -w" -trimpath -o "bin/$bin_name" ./cmd/slk
  fi
}
