#!/usr/bin/env bash
# The default way to run any go command in this repo on managed macOS. Santa
# SIGKILLs locally built Go binaries — every `go test`/`go run` builds a fresh
# ad-hoc-signed binary with a new hash, so no allowlist rule can cover them —
# so on Santa hosts this runs go inside a golang container; on other hosts it
# execs native go untouched.
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

repo_root=$(git rev-parse --show-toplevel)
prefix=$(git rev-parse --show-prefix)

# Tag tracks the go.mod `go` directive; bump them together.
image="golang:1.26"

docker_args=(
  --rm
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

echo "Santa host — running go in docker (${image})" >&2
exec docker run "${docker_args[@]}" "$image" go "$@"
