# Keeping this fork mergeable

This repo is a fork of [gammons/slk](https://github.com/gammons/slk) that
tracks `upstream/main`. The fork adds herdr integration, thread-unread
handling, link opening/downloading, reconnect UX, and concurrency hardening.
Everything here exists to keep `git merge upstream/main` cheap.

## Layout rule: fork code lives in fork-only files

Go allows any number of files per package, so fork-added declarations
(functions, methods, types, tests) never live in upstream files:

- **Fork additions to an upstream file's domain** go in a sibling file named
  `<base>_fork.go` / `<base>_fork_test.go` (e.g. fork tests for
  `reducer_links.go` live in `reducer_links_fork_test.go`). The suffix can't
  collide with anything upstream would add.
- **Whole fork features** get descriptively named files as usual
  (`herdr_wiring.go`, `preview_source.go`, `internal/sharedmap/`).
- **Upstream files** carry only what genuinely can't move: hook lines calling
  fork code, added struct/interface members, new switch arms, and in-place
  behavior changes. Prefer a one-line hook into a fork-file helper over an
  inline block (see `migrate()` calling `migrateFork()` in
  `internal/cache/db.go`).
- Never relocate or rename upstream code for fork convenience; call the
  upstream name from the fork file instead.

`tools/fork-footprint.sh` prints the current churn on upstream-owned files.
Run it before finishing a feature; a feature that grows the footprint where a
fork-only file would do isn't done.

## Merging upstream

1. `git fetch upstream`
2. `git merge upstream/main` — a true merge commit, never squashed or rebased
   (fork feature branches are the opposite: squash-merged, per CLAUDE.md).
3. Resolve conflicts. They should only appear in the intentionally-diverged
   files below; a conflict anywhere else means fork code leaked into an
   upstream file — fix the layout, not just the conflict.
4. `tools/go.sh vet ./...` and `tools/go.sh test ./...` (never bare `go`;
   see docs/developing-on-santa-hosts.md).
5. If the merge or its fixups touched files gofmt would reflow, revert
   reflow-only changes (see "Never run `go fmt` across the tree" in
   CLAUDE.md).

## Where the fork intentionally diverges in place

These upstream files carry real in-place behavior changes; expect conflicts
there and resolve them knowing what the fork wants:

- `cmd/slk/main.go` — `WorkspaceContext`'s shared maps migrated to
  thread-safe stores (`internal/sharedmap`, `internal/usernames`,
  `atomic.Pointer`), plus every call site downstream. The single biggest
  divergence; irreducible.
- `internal/ui/*` — new `App` fields, `TeamID` on message msgs, new key
  bindings and reducer switch arms; the usernames-store migration's
  mechanical call-site edits.
- `internal/slack/events.go` — `OnThreadMarked` passes the subscription's
  `active` flag through instead of inverting it into a bogus `read` bool.
  Upstreaming candidate: it fixes an upstream bug.
- `internal/slack/auth.go` — atomic token save.
- `internal/slack/connection.go` — reconnect/backoff rework in `Run`.
- `internal/avatar/avatar.go` — `preloadInner` hooks into fork helpers:
  sized-variant URL rewrite and a bounded kitty decode target.
- `internal/cache/users.go` — `UpsertUser` no longer updates `presence`
  on conflict; `UpdatePresence` is the column's only writer.
- `cmd/slk/reconnect_sync.go` (+ its test) — the shared-DB
  `MarkChannelsStale` call replaced by the per-instance watermark hook
  (`sendCacheWatermark`); the test pinning the old staling deleted.
- `cmd/slk/thread_subscriptions.go` — `sync` call routed through the
  cross-instance sweep-claim hook (`syncIfUnclaimed`).
- `internal/ui/reducer_channels.go` — tier-1 freshness additionally
  requires `syncedAfterWatermark`.
- `internal/ui/mode_insert.go` (+ its test in `app_test.go`) — the
  Ctrl+U clear-compose intercept removed so Ctrl+U (kitty's
  cmd+backspace) falls through to the textarea's delete-before-cursor;
  the fork's expectation is pinned in `mode_insert_fork_test.go`.
- `internal/ui/sixelpaint_test.go` — one assertion updated for the
  sixel frame memo (a post-force identical frame reuses its ID).
- `internal/cache/threads.go` — `ListSubscribedThreads` counts the parent
  row as newest activity.
- `internal/cache/messages.go`, `internal/cache/db.go` — one-line hooks into
  fork helpers (`retractLatestReply`, `migrateFork`).
- `internal/image/probe.go`, `kitty.go`, `cellmetrics.go` — probe result
  shape, cell-size-keyed payload memo, measured cell metrics.
- `internal/bootstrap/*` — `Run`'s post-userBoot chain routed through the
  `overlapPhases` hook (`bootstrap_fork.go`); the `revalidate` entry point
  deleted, its nil-guards moved into the hook; the two serial-order tests
  in `bootstrap_test.go` relaxed or deleted. See "Overlapped boot" below.
- `internal/notify/*` — leader gate on notifications.
- `internal/config/config.go` — two fork struct fields (`Herdr`, `Restore`).
- Docs (`README.md`, `wiki/*`, `docs/STATUS.md`, `docs/superpowers/*`) —
  fork features documented in place; markdown conflicts, resolve by hand.

## Overlapped boot, unchanged call pattern

The boot sequence exists to mimic the official web client's call
pattern, because deviating from it is what got slk's Enterprise Grid
users signed out for "data scraping" (see `internal/bootstrap`'s
package comment). The fork overlaps that chain's independent legs
without changing what is called: inside `bootstrap.Run`,
client.counts, the channel open, and the channels/info revalidation
run concurrently, with users/info still trailing the open; in
`connectWorkspace`, `bootstrap.Run`, the section-store bootstrap, and
users.conversations run concurrently. Every request, parameter, and
call count is identical to the serial chain — only timing changes,
and boot drops from ~9 sequential round trips to ~4.

The deliberate risk: request *concurrency* is a timing signature no
capture has been audited for, so Grid heuristics could in principle
score it. If a workspace gets flagged, the revert is one commit: the
overlap squash-merged to main as a single feature commit (`git log -S
overlapPhases` finds it), and reverting it restores the fully serial
chain — nothing else depends on the timing.
