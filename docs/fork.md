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
5. `tools/go.sh fmt ./...`: CI's lint job fails on gofmt drift (see "Keep
   the tree gofmt-clean" in CLAUDE.md).

When upstream pure-moves a diverged file into new files (the `cmd/slk/main.go`
split), don't merge across the refactor in one step: merge up to the commit
before it, then merge the move commits one at a time, porting the fork's
delta into each new file as it appears.

## Where the fork intentionally diverges in place

These upstream files carry real in-place behavior changes; expect conflicts
there and resolve them knowing what the fork wants:

- `cmd/slk/main.go` and the topical files upstream split out of it —
  `WorkspaceContext`'s shared maps and self presence/DND fields migrated to
  thread-safe stores (`internal/sharedmap`, `internal/usernames`,
  `selfStatusStore`), plus every call site downstream. `workspace.go` holds
  the struct; `connect.go` seeds the stores and carries `connectWorkspace`'s
  overlapped boot; `users.go`, `history.go`, `user_resolver.go`,
  `conversations.go`, `workspace_search.go`, `presence.go` and
  `rtm_handler.go` are call sites (`presence.go` also the token-guarded
  status bootstrap and presence dedupe, `rtm_handler.go` also the
  `TeamID`-tagged dispatch for background workspaces); `attachments.go` one
  line (`OriginalW/H`); `main.go` keeps `run()`'s fork wiring (permalink
  argument, `herdr` subcommand, pane restore, notify leader, the `Preview`
  service func, the `$BROWSER` opener). The single biggest divergence;
  irreducible.
- `cmd/slk/markread.go` — `OnThreadMarked` only: persistence is upstream's
  cursor-only writers; the fork derives `Read` (`threadMarkReadState` from
  `ThreadNewestActivity`) and dispatches `ThreadMarkedRemoteMsg`
  `TeamID`-tagged for every workspace, where upstream dispatches active-only.
- `internal/core/types.go`, `ports.go`, `adapters.go` — `Attachment`'s
  `OriginalW/H`, `MessageItem.IsEphemeral`, `MessageService.Preview`, and
  the `Preview` member of `MessageServiceFuncs`; the adapter method and
  `TableBlock` live in `adapters_fork.go` and `blocks/blocks_fork.go`.
- `internal/core/blocks/blocks.go`, `internal/ui/messages/blockkit/parse.go`
  — `LabelAccessory` and `ActionElement` embed `ButtonFields`
  (`blocks_fork.go`: a button's action_id, value, url and block_id), which
  one-line hooks in `parseSection` and `parseActions` fill
  (`parse_fork.go`).
- `internal/ui/boundary_test.go` — the slack-go import check consults
  `tuiForkExempt` (`boundary_fork_test.go`), whose one entry is
  `blockkittest.go` → slack-go. The fork's other outside-world code lives in
  `cmd/slk`: the `$BROWSER` launch wraps the opener `core.NewDesktopService`
  receives (`browserLauncher` in `launch_fork.go`), and the docker host
  clipboard reader (`clipboard_remote.go`) is handed to
  `SetAsyncClipboardReader`.
- `internal/ui/app.go` (live thread-reply read marking) — upstream's
  `recordThreadMark` + `scheduleMarkFlush` is the single issuer; the fork
  adds a `PaneViewed` gate at the top of `flushPendingMarks` and a
  `scheduleMarkFlush` call on herdr refocus (`agentthread.go`).
- `internal/ui/app.go`, `mode_confirm.go` (quit prompt) — `openQuitConfirm`
  notes whether the prompt opens over the `.` overlay, and a closed prompt
  hands the keys back to the overlay (`appshortcuts.go`), where upstream
  always lands in normal mode.
- `internal/ui/*` — new `App` fields, `TeamID` on message msgs, new key
  bindings and reducer switch arms; the usernames-store migration's
  mechanical call-site edits.
- `internal/ui/messages/model.go` — `PrependMessages` and `OldestTS` read
  past ephemerals (`ephemeral_fork.go`): paging anchors on the oldest
  message that is not one, the boundary guard compares against it, and the
  older page is merged by ts with the ephemerals ahead of it, the selection
  kept on its message. Here and in `thread/model.go`, the header row ends in
  `EphemeralMark` and the interact hint is `InteractHint` (`links_fork.go`),
  where upstream always says to open Slack.
- `internal/ui/messages/render.go` — a user mention is drawn inside an OSC 8
  link to the user's profile when `RenderSlackMarkdownOpts.WorkspaceDomain`
  is set (`profilelink_fork.go`), as the messages pane and thread panel set
  it, so a click on it opens the profile card (`profilecard.go`); upstream
  draws the mention plain.
- `internal/ui/messages/highlight.go` — `HighlightSearchTerms` matches on the
  visible rune stream across escape sequences, so a term spanning a styled
  boundary (a colored token, an inline span) highlights as one run; upstream
  matches within one segment. Upstreaming candidate.
- `internal/slack/events.go` — `OnAssistantStatus`, `OnEphemeralMessage`
  and `OnView` on `EventHandler`; the `ai_assistant_status` and
  `view_opened`/`view_updated` dispatch arms; and `is_ephemeral` on
  `wsMessageEvent` and `wsSubMsg`, with which the `message` and
  `message_changed` arms hand an ephemeral to `OnEphemeralMessage` in place
  of `OnMessage`.
- `internal/slack/client.go` — boot-path calls made cancellable in
  place: SlackAPI's `AuthTest`/`GetConversationsForUser` swapped for
  their `Context` variants, `GetUnreadCounts` takes a ctx, and the
  inline 429 retry sleeps route through `rateLimitWait`
  (ratelimit_fork.go); plus WebSocket conn-pointer locking under
  `wsMu`, the test-injectable `wsDialer` field, and `GetUserInfoContext`
  on `SlackAPI` for the profile card's bounded users.info
  (`client_fork.go`).
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
  requires `syncedAfterWatermark`; every load into the channel pane (fetch,
  channel switch, jump to a message) lays the session's ephemerals in
  (`ephemeral_fork.go`), and the cache tier is chosen before that.
- `internal/ui/mode_insert.go` (+ its test in `app_test.go`) — the
  Ctrl+U clear-compose intercept removed so Ctrl+U (kitty's
  cmd+backspace) falls through to the textarea's delete-before-cursor;
  the fork's expectation is pinned in `mode_insert_fork_test.go`.
- `internal/ui/keys.go`, `internal/ui/mode_insert.go` (+ the key in
  `editor_test.go` and `seams_test.go`'s `editDraft`) — the $EDITOR draft
  binding moved from `ctrl+e` to `ctrl+x` so Ctrl+E (kitty's cmd+right)
  reaches the textarea's LineEnd; pinned in `mode_insert_fork_test.go`.
  Upstream comments still say Ctrl+E.
- `internal/ui/sixelpaint_test.go` — one assertion updated for the
  sixel frame memo (a post-force identical frame reuses its ID).
- `internal/ui/messages/blockkit/render.go` (+ `render_test.go`) — the
  `rich_text` arm draws in place instead of returning, which also makes
  rich_text nested in legacy attachments visible (it was dropped before);
  the host's body-block
  skip lives in `RenderMessageBlocks` in `render_fork.go`, mirroring
  `messages.MessageTextSource` and pinned by the panes' "appears exactly
  once" tests), a `table` arm, and the unsupported-block marker restyled as
  a warning badge; the zero-lines test retargeted to `RenderMessageBlocks`.
  Both panes' selected-variant builders reassert the selection tint after
  every reset (`ReapplyBgAfterResets`) so bare runs from block renderers
  take the tint.
- `internal/ui/messages/blockkit/attachments.go` (+ `attachments_test.go`,
  `integration_test.go`, `messages/blockkit_integration_test.go`) —
  `RenderLegacy` draws each attachment through `appendCard`
  (`attachments_fork.go`), which wraps upstream's `appendLegacyAttachment`:
  it takes that function's `█ ` stripe prefix back off every row and puts
  the rows in a round box, drops the flat `Text` when the blocks carry the
  body, heads a Slack message unfurl with author, channel and local time
  in place of Slack's footer, and folds a long body. If upstream changes
  the stripe, `appendCard`'s `TrimPrefix` must follow. The stripe
  assertions in those tests now look for the frame, and the PagerDuty
  fixture renders expanded. `LegacyAttachment` gains `AuthorName` and
  `FromURL`, `RenderResult` `FoldRows`, `Context` `Card`; `golden_test.go`'s
  `█ deploy` assertion and the goldens with that card are re-blessed.
- `internal/ui/reducer_send.go` — `reduceNewMessage`'s edit-echo branch
  calls the panes' `ReplaceMessageContent` (`model_fork.go`) where upstream
  calls `UpdateMessageInPlace` / `UpdateParentInPlace` with the text alone:
  a `message_changed` event also carries blocks and attachments, and it is
  how Slack delivers a link's unfurl after the post. Upstreaming candidate.
  A live reply to the open thread goes through the thread panel's
  `AddIncomingReply` (`thread/select.go`) where upstream calls `AddReply`:
  the cursor follows only from the newest reply.
- `internal/cache/threads.go` — `ListSubscribedThreads` counts the parent
  row as newest activity.
- `internal/cache/messages.go`, `internal/cache/db.go` — one-line hooks into
  fork helpers (`retractLatestReply`, `migrateFork`).
- `internal/image/probe.go`, `kitty.go`, `cellmetrics.go` — id-matched
  probe replies logged verbatim, cell-size-keyed payload memo, measured
  cell metrics.
- `internal/bootstrap/*` — `Run`'s post-userBoot chain routed through the
  `overlapPhases` hook (`bootstrap_fork.go`); the `revalidate` entry point
  deleted, its nil-guards moved into the hook; the two serial-order tests
  in `bootstrap_test.go` relaxed or deleted. See "Overlapped boot" below.
- `internal/notify/*` — leader gate on notifications.
- `internal/config/config.go` — two fork struct fields (`Herdr`, `Restore`).
- Upstream tests the fork adjusts — `internal/cache/db_test.go`
  (`seedUsersTable` adds `display_name`, which `migrateFork` indexes);
  `cmd/slk/reconnect_sync_test.go`, `internal/slack/client_test.go` (ctx
  argument on `GetUnreadCounts`, `Context` variants on the mock);
  `cmd/slk/on_message_mention_test.go`, `rail_unread_test.go`,
  `internal/ui/seams_test.go`, `threadsview/model_test.go` (fixtures built
  on the stores); `internal/ui/mode_normal_keys_test.go` (`O` is
  OpenLinkTab here, image preview is `v` only); `modekeys_test.go` (the pinned
  mode list includes `ModeAppShortcuts` and `ModeProfileCard`); `mode_insert_keys_test.go`
  (no Ctrl+U intercept); `messages/codeblock_wrap_test.go` (the fork's
  bordered code box); `golden_test.go`, `mode_linkpicker_test.go` (extra
  argument on `layout.Compute` / `openLinksOfSelected`); seven
  `internal/ui/testdata/golden/*.ansi` re-blessed (selection tint reassert;
  `window_split.ansi` gains the "── new ──" line from
  `applyCachedLastRead`).
- `go.mod` / `go.sum` — `github.com/slack-go/slack` bumped past upstream's
  pin to v0.29.0 for typed table cells (`TableRawTextCell` etc.; v0.23.0
  decoded every cell as rich_text and dropped raw_text content). On an
  upstream bump, keep whichever is newer.
- `flake.nix` — `vendorHash` covers the fork's go.mod, not upstream's.
  Whenever go.mod changes (an upstream merge, a bump), the nix CI job
  prints the new hash in its `got:` line; paste it in.
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
