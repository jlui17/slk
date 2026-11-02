#!/usr/bin/env bash
# The default way to run any go command in this repo on managed macOS. Santa
# SIGKILLs locally built Go binaries — every `go test`/`go run` builds a fresh
# ad-hoc-signed binary with a new hash, so no allowlist rule can cover them —
# so on Santa hosts this runs go inside a golang container; on other hosts it
# execs native go untouched. That container is the role's long-lived slk
# backend (tools/backend.sh), entered with `docker exec`.
#
# Usage, from the repo or any worktree: tools/go.sh test ./...
set -euo pipefail

# Detect Santa by its CLI instead of executing anything: Santa kills the
# probe binary and pops a user notification per blocked execution.
if ! command -v santactl >/dev/null 2>&1; then
  exec go "$@"
fi

command -v docker >/dev/null || {
  echo "docker is required on this host: Santa blocks locally built Go binaries" >&2
  exit 1
}

repo_root=$(cd "$(git rev-parse --show-toplevel)" && pwd -P)
prefix=$(git rev-parse --show-prefix)

# Role, image and the shared backend; the role label mirrors run-docker.sh,
# so agent-session containers are discriminable from interactive ones and
# nothing cleaning up agent work can match a user's container.
repo=$repo_root
. "$repo_root/tools/backend.sh"

# The image derives from golang (tag tracks the go.mod `go` directive; bump
# them together): golang.design/x/clipboard compiles C that needs X11
# headers, so cgo builds — including anything with -race — fail on the
# stock image.
if ! docker image inspect "$image" >/dev/null 2>&1; then
  echo "Building $image (golang + libx11-dev for the clipboard cgo dep)" >&2
  docker build -t "$image" - <<'EOF' >&2
FROM golang:1.26
RUN apt-get update && apt-get install -y --no-install-recommends libx11-dev \
  && rm -rf /var/lib/apt/lists/*
EOF
fi

docker_args=(
  --rm
  --label "slk.role=${role}"
  # Keep stdin attached so `go run` programs that read it work under the
  # wrapper too.
  -i
  -v "$repo_root":/src
  -w "/src/${prefix%/}"
  # Named volumes shared by every checkout and worktree of this repo so
  # module downloads and build artifacts persist across runs.
  -v slk-gomodcache:/go/pkg/mod
  -v slk-gobuildcache:/root/.cache/go-build
  # A worktree's .git is a file pointing at a host path outside the mount,
  # which breaks go's VCS stamping of main-package builds; caller GOFLAGS
  # are appended so they still apply.
  -e GOFLAGS="-buildvcs=false${GOFLAGS:+ $GOFLAGS}"
)

# Cross-compile and cgo knobs must ride into the container, or a Santa host
# silently builds a container-native binary where every other host honors
# them.
for var in GOOS GOARCH CGO_ENABLED; do
  if [[ -n "${!var:-}" ]]; then
    docker_args+=(-e "$var=${!var}")
  fi
done

# The tablabel live tests (internal/tablabel/live_test.go,
# agent_status_eval_test.go) are env-gated and env-tuned; without these they
# silently skip, or run with defaults, inside the container. Passed by name
# so docker reads the value from its own environment: a key on the command
# line is visible to ps and to anything that wraps docker.
for var in SLK_TABLABEL_LIVE ANTHROPIC_API_KEY SLK_AGENT_STATUS_EVAL_RUNS \
  SLK_AGENT_STATUS_EVAL_MODEL SLK_AGENT_STATUS_EVAL_EFFORT; do
  if [[ -n "${!var:-}" ]]; then
    docker_args+=(-e "$var")
  fi
done

# The live tests take the key from slk's config first (herdr.anthropic_api_key),
# so a live run gets that one file, read-only, where the container's root
# user looks for it. Only a live run: every other go command keeps a
# container with no user config in it.
slk_config="${XDG_CONFIG_HOME:-$HOME/.config}/slk/config.toml"
if [[ -n "${SLK_TABLABEL_LIVE:-}" && -f "$slk_config" ]]; then
  docker_args+=(-v "$slk_config":/root/.config/slk/config.toml:ro)
fi

# A live run needs its config mount, which the backend can't take, so it
# keeps a container of its own.
if [[ -z "${SLK_TABLABEL_LIVE:-}" && "$repo_root/" == "$main"/* ]]; then
  echo "Santa host — running go in $backend" >&2
  ensure_backend
  exec_id=go-$$-$RANDOM
  exec_args=(-i -w "$repo_root/${prefix%/}" -e SLK_EXEC_ID="$exec_id")
  for ((i = 0; i < ${#docker_args[@]}; i++)); do
    [[ "${docker_args[i]}" == -e ]] && exec_args+=(-e "${docker_args[i + 1]}")
  done
  # docker exec forwards no signals, and go outlives a client that dies, so
  # Ctrl+C (or a kill) stops go through its exec id. The client runs in the
  # background so a trapped signal interrupts `wait` at once, with stdin
  # handed over explicitly (a background job's stdin is /dev/null).
  stop() { docker exec "$backend" sh "$repo_root/tools/stop-exec.sh" -g "$exec_id"; exit "$1"; }
  trap 'stop 130' INT
  trap 'stop 143' TERM
  trap 'stop 129' HUP
  docker exec "${exec_args[@]}" "$backend" go "$@" <&0 &
  rc=0
  wait $! || rc=$?
  exit "$rc"
fi
echo "Santa host — running go in docker (${image})" >&2
exec docker run "${docker_args[@]}" "$image" go "$@"
