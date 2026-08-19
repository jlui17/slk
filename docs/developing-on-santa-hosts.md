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

**Caveat:** the docker pty reports no pixel dimensions, so cell metrics fall
back to 8x16 and images render at low resolution regardless of terminal.
Judging image quality needs a native run, or `COLORTERM_CELL_WIDTH` /
`COLORTERM_CELL_HEIGHT` set before launching (the script forwards them).

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
