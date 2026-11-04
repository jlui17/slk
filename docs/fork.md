# Keeping this fork mergeable

This repo is a fork of [gammons/slk](https://github.com/gammons/slk) that
tracks `upstream/main`. The fork adds herdr integration, thread-unread
handling, link opening/downloading, reconnect UX, and concurrency hardening.
Most of this doc exists to keep `git merge upstream/main` cheap. The
design sections ("Many slks, one cache" through "One slk per pane,
measured and kept") record how the fork works: what each design does,
why, and the alternatives that were measured and rejected.

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
  the struct; `connect.go` seeds the stores and hooks `connectWorkspace`'s
  overlapped boot (`runBootChains`, `connect_fork.go`); `users.go`, `history.go`, `user_resolver.go`,
  `conversations.go`, `workspace_search.go`, `presence.go` and
  `rtm_handler.go` are call sites (`presence.go` also the token-guarded
  status bootstrap and presence dedupe, `rtm_handler.go` also the
  `TeamID`-tagged dispatch for background workspaces, ephemerals and
  deletes included, and the `sendAssistantStatus` hook that feeds the
  tracked agent thread); upstream's own `userNameStore` (`usernames.go`
  and its three tests) is deleted, `*usernames.Store` taking its place in
  `export_collect.go`, `demo.go` and every upstream test that built one
  (through the `newUserNameStore` shim in `usernames_fork_test.go`; the
  race test is ported to `user_names_race_test.go`); `attachments.go` one
  line (`OriginalW/H`); `main.go` keeps `run()`'s fork wiring (permalink
  argument, `herdr` subcommand, pane restore, notify leader, the `Preview`
  service func, the `$BROWSER` opener). The single biggest divergence;
  irreducible.
- `cmd/slk/markread.go` — `OnThreadMarked` only: persistence is upstream's
  cursor-only writers; the fork derives `Read` (`threadMarkReadState` from
  `ThreadNewestActivity`) and dispatches `ThreadMarkedRemoteMsg`
  `TeamID`-tagged for every workspace, where upstream dispatches active-only.
- `cmd/slk/rtm_handler.go`, `history.go` — a bot message shows its own
  username over the bot's stored name, live and from the cache
  (`authorName`, `users_fork.go`), and a WebSocket message is cached with
  the event's `user`, `bot_id` and `username` in raw_json, as Slack sends
  them, where upstream stores the author ID as `user`.
- `cmd/slk/rtm_handler.go` — `OnReactionAdded` and `OnReactionRemoved`
  write the cache through `cacheReaction` (`rtm_handler_fork.go`), which
  keeps one entry per user per emoji, where upstream appends the user and
  adds one to the count for every event. Every slk on the machine shares
  the cache and gets every event, so upstream's write counted a reaction
  once per slk.
- `cmd/slk/rtm_handler.go` — `OnMessage` badges a mention through
  `CountChannelMention` (`channels_read_state_fork.go`), which counts each
  message once, where upstream's `IncrementChannelMentionCount` adds one
  per call; upstream's is left in place, unused. Upstream's badged a DM
  message once per slk.
- `internal/core/types.go`, `ports.go`, `adapters.go` — `Attachment`'s
  `OriginalW/H`, `MessageService.Preview`, and
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
- `internal/ui/*` — `App` embeds `appFork` (`app_fork.go`, the fork's
  App state), `TeamID` on message msgs, new key bindings and reducer
  switch arms; the usernames-store migration's mechanical call-site
  edits.
- App Home (`apphome.go`, `internal/ui/apphome/`) — a `ViewAppHome` value
  on `View`, and one-line hooks where upstream branches on `ViewThreads`:
  `renderMessagesRegion`, the mouse wheel and click arms of
  `reducer_mouse.go` (plus a click guard while its confirm box is up),
  `handleNormalMode`, the `ChannelSelectedMsg` arm, `applyOverlays` and
  `reduceWorkspaceSwitched`. `messages/model.go` draws a `headerRow` under
  the channel name (`apphome_fork.go`, the app DM's tab row);
  `statusbar/model.go` colors the toast through `toastColor`
  (`model_fork.go`, the warning toast); `ChannelGlyph` and the status
  bar's glyph gain an `app` arm (▣), where upstream draws `#`.
- `internal/ui/messages/model.go` — `PrependMessages` and `OldestTS` read
  past ephemerals (`ephemeral_fork.go`): paging anchors on the oldest
  message that is not one, the boundary guard compares against it, and the
  older page is merged by ts with the ephemerals ahead of it, the selection
  kept on its message.
- `internal/ui/messages/render_message.go` — `RenderMessage` carries the
  fork's render inputs and outputs: `RenderContext` gains
  `WorkspaceDomain` and `ExpandedAttachments`, `RenderedMessage`
  `CodeBlockCopyLabels` and `AttachmentFoldRows`, and the body render takes
  the search terms, permalink chips and code-block copy labels, so the
  search highlight is drawn in the body rather than over the finished
  rows. Blocks go through `RenderMessageBlocks` with
  `BlockkitContextFork` (`render_message_fork.go`), and the interact hint
  is `InteractHint` (`links_fork.go`), which names `o` for a button's
  link, where upstream always says to open Slack. `wrap_rows.go`'s
  `SelectedVariant` reasserts the selection tint after every reset
  (`ReapplyBgAfterResets`), so bare runs from block renderers take it;
  `wrap_rows_oracle_test.go`'s lipgloss oracle for it does the same.
- `internal/ui/messages/render.go` — a user mention is drawn inside an OSC 8
  link to the user's profile when `RenderSlackMarkdownOpts.WorkspaceDomain`
  is set (`profilelink_fork.go`), as the messages pane and thread panel set
  it, so a click on it opens the profile card (`profilecard.go`); upstream
  draws the mention plain. A `<url|label>` whose label is the URL of a
  Slack message draws as that message's chip (`PermalinkChips` in
  `permalinkchip_fork.go`), checked on the raw label before upstream's
  entity decode.
  Bold, italic (`renderItalics`) and strikethrough (`markStrike`,
  `strike_fork.go`) only mark their spans (`markSpan`), and `drawSpans`
  (`stylespans_fork.go`), the pass's last line, styles them once the
  links, mentions and emoji inside are drawn, keeping their escape
  sequences whole and turning the style back on after each inner reset;
  upstream styles on the spot, so lipgloss's strike cuts a `<@U1>` apart
  and prints it as text, the text after an inner span loses bold or
  italic, and `*_x_*` draws its underscores.
  Bold (`replaceBolds`) and strike share one scanner,
  `replaceDelimitedRuns` (`emphasis_fork.go`), so a `*` pairs only on a
  word boundary and against text, and `renderItalics` takes the same
  against-text check (`runHugsText`); upstream's bold regex pairs any two
  asterisks on a line, so "37 10 * * *" drew a bold space. Inline code is
  held out of the bold, italic and strike passes (`heldInlineCode`), where
  upstream draws it first and lets them style what's inside it. Rich text
  that styles part of a word reaches the scanner with a zero-width space
  between marker and letter (`runsToMrkdwn`, `richtext_runs_fork.go`), so
  "*un*" of "unbelievable" still draws bold; `DropZeroWidthSpaces` removes
  the space once the markers are paired, and `y` (`app.go`) and the export
  (`export/markdown.go`, `export/channel.go`) call it so a paste carries
  none. Edit source reads `RichTextToMarkdown` and never has it. A styled
  run whose own text holds its marker (a bold "5 * 3 = 15", an italic
  "snake_case") gets the same space before each inner marker
  (`guardInnerMarkers`), and the scanners never close on a guarded marker
  (`isGuardedMarker`, also hooked into `renderItalics`); typed mrkdwn has no
  guard, so a typed "*5 * 3 = 15*" stays literal.
  After each reset the text color put back is `RenderSlackMarkdownOpts.TextColor`
  (`textcolor_fork.go`), where upstream always puts back the primary
  color, and blockkit's `appendContext` renders through
  `Context.RenderMutedText`, so a context block's text after a bold span
  stays muted, as Slack draws it.
- `internal/ui/messages/highlight.go` — `HighlightSearchTerms` matches on the
  visible rune stream across escape sequences, so a term spanning a styled
  boundary (a colored token, an inline span) highlights as one run; upstream
  matches within one segment. Upstreaming candidate.
- `internal/slack/events.go` — `OnView` on `EventHandler` in place of
  upstream's `OnViewOpened` (its handler and tests, `rtm_view_opened_test.go`
  among them, deleted): `view_opened` and `view_updated` go to
  `dispatchViewEvent` (`appactions_fork.go`), and the UI says a modal it
  didn't ask for can't be shown (`unshownViewToast` in `appshortcuts.go`),
  as upstream's handler did. A `message_changed` on an ephemeral goes to
  `dispatchChangedEphemeral` (`events_fork.go`), where upstream drops it.
- `internal/slack/client.go` — boot-path calls made cancellable in
  place: SlackAPI's `AuthTest`/`GetConversationsForUser` swapped for
  their `Context` variants, `GetUnreadCounts` takes a ctx, and the
  inline 429 retry sleeps route through `rateLimitWait`
  (ratelimit_fork.go); plus WebSocket conn-pointer locking under
  `wsMu`, the test-injectable `wsDialer` field, and `GetUserInfoContext`
  on `SlackAPI` for the profile card's bounded users.info
  (`client_fork.go`); and `StartWebSocket`'s read loop closes its socket
  when it ends, and records the socket's hello, `retry_after` and read
  error in `wsOutcome` (`websocket_fork.go`); the read deadline comes from
  `wsReadTimeout`, the read loop runs `keepAlive`, and the event owner
  drops Slack's pongs before dispatch (`keepalive_fork.go`; "One socket
  per workspace").
- `internal/slack/client.go` (+ `client_test.go`) — `GetStarredChannels`
  keeps every conversation star (`starredConversationID` in
  `client_fork.go`: channel and im under `channel`, group under the
  `Group` member `starsListItem` gains), so a starred DM, app, private
  channel or group DM sits in Starred as in Slack; upstream keeps channel
  stars only, and its parse test now expects the DM.
- `internal/slack/auth.go` — atomic token save.
- `internal/slack/connection.go` — reconnect/backoff rework in `Run`:
  the backoff resets only after a socket that proved healthy, and waits
  are jittered and floored at Slack's `retry_after` ("One socket per
  workspace").
- `internal/avatar/avatar.go` — `preloadInner` fetches at the fork's
  `decodeTarget` (`avatar_fork.go`), which bounds the kitty decode where
  upstream decodes the source at full size.
- `internal/cache/users.go` — `UpsertUser` no longer updates `presence`
  on conflict; `UpdatePresence` is the column's only writer.
- `cmd/slk/reconnect_sync.go` (+ its test) — the shared-DB
  `MarkChannelsStale` call replaced by the per-instance watermark hook
  (`sendCacheWatermark`); the test pinning the old staling deleted.
- `internal/ui/reducer_channels.go` — tier-1 freshness additionally
  requires `syncedAfterWatermark`; every load into the channel pane (fetch,
  channel switch, jump to a message) lays the session's ephemerals in
  (`ephemeral_fork.go`), and the cache tier is chosen before that. The
  reconnect refresh (`KeepLastRead`, set in `main.go`) leaves a pane's
  last-read boundary where it is (`loadedLastRead`,
  `reducer_channels_fork.go`).
- `internal/ui/mode_insert.go` (+ its test in `app_test.go`) — the
  Ctrl+U clear-compose intercept removed so Ctrl+U (kitty's
  cmd+backspace) falls through to the textarea's delete-before-cursor;
  the fork's expectation is pinned in `mode_insert_fork_test.go`. The
  stay-in-insert tests in `action_prompt_queue_test.go` that empty the
  composer with Ctrl+U focus it first, which the textarea needs.
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
- `internal/ui/messages/blockkit/richtext.go` (+ `richtext_test.go`) —
  `listToMrkdwn` draws each item behind the head `listHeads`
  (`richtext_lists_fork.go`) gives it, as Slack draws nested lists: an
  ordered list numbers on across the deeper lists between its parts,
  markers cycle by indent (•, ◦, ▪ and 1., a., i.), and a nested list
  starts where its parent item's text starts. `TestRichTextToMrkdwn_IndentedList`
  now wants `▪` at indent 2.
  `sectionToMrkdwn` puts one marker pair around neighbouring text and link
  elements that share bold, italic and strike (`richtext_runs_fork.go`), so
  a struck phrase with inline code or a link in it reads ``~keep `x` y~``
  or `~for <url|#1> are~`; upstream wraps each text element in its own
  pair, which the renderer cannot pair up, and drops a link's style.
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
- `internal/ui/reducer_send.go` — an ephemeral `NewMessageMsg` goes to
  `reduceEphemeral` (`ephemeral_fork.go`); the tracked agent thread's
  reply, activity and delete hooks run ahead of the background-workspace
  skip; a live reply to the open thread goes through the thread panel's
  `AddIncomingReply` (`thread/select.go`) where upstream calls `AddReply`:
  the cursor follows only from the newest reply.
- `internal/ui/reducer_links.go` — permalink navigation keeps the fork's
  mechanism inside upstream's `applyLocation`: `applyLinkNav` and a
  `pendingLinkNav` that embeds `Location` pin the target with
  `SetPendingSelectTS` and open its thread (`openThreadForPermalink` takes
  the reply to select), where upstream wraps thread loads in a
  `permalinkThreadResultMsg` envelope (dropped, with its four tests in
  `reducer_links_test.go`). `routeLink` takes the herdr target.
- `internal/ui/panellayout.go`, `app.go` — upstream's stacked thread
  layout, with `t` (`ZoomThread`) as `Compute`'s extra `threadFullscreen`
  argument: the thread stacks in front at any width. The fork's own
  cramped auto-zoom is gone.
- `internal/ui/linkpicker/view.go` — `renderBox` lives in `view_fork.go`
  and keeps the fork's frame, where upstream draws the picker in its
  shared chrome frame. Provisional.
- `internal/ui/presencemenu/model.go` — an injectable clock (`nowFn`) feeds
  the DND check and the snooze presets.
- `internal/ui/mode_linkpicker.go` — a "copy" picker commits through
  `copyPickedCopyable` ahead of upstream's clear-on-commit.
- `internal/ui/mode_normal.go` — `o`, `O` and `alt+o` call the fork's
  `openLinksOfSelectedIn` (`mode_linkpicker_fork.go`); upstream's
  `openLinksOfSelected` is kept as upstream wrote it, and only
  `mode_linkpicker_test.go` still calls it.
- `internal/ui/thread/model.go` (+ `TestNavigation` in `model_test.go` and
  `TestMoveUpFromFirstReplySelectsParent`) — `GoToTop`, which `gg` calls,
  selects the parent row, the thread's topmost, where upstream selects the
  first reply; pinned in `keyrouting_fork_test.go`.
- `internal/ui/thread/model.go`, `internal/ui/reducer_io.go` — the
  thread's parent entry and its list of entries are kept between frames
  (`parentRow`, `setReplyList` in `viewmemo_fork.go`), where upstream
  renders the parent and hands the list new entries on every View. An
  image landing for the parent invalidates the thread
  (`ShowsMessage`), as one for a reply does.
- `internal/ui/messages/model.go`, `internal/ui/thread/model.go` — a
  name resolving, a status change, a mark from another client, a
  reaction-nav step, a reload that brings back what is shown, older
  messages, a sent message's ts and (in the thread) a new reply, an edit
  or a reaction re-render only the entries they change
  (`invalidate_fork.go` in each package), where upstream drops the whole
  render cache. The thread's reply render moved to `renderReplyEntry`.
  Every one is held to a full rebuild by `invalidate_oracle_fork_test.go`;
  the four upstream tests that asserted a nil cache after
  `PatchUserName` assert the re-resolved mention instead.
- `internal/ui/messages/model.go` — the channel pane keeps the render
  cache of the width it was drawn at before (`widthSlot`,
  `widthcache_fork.go`), so a thread opening and closing switches
  between two cached widths, where upstream renders every message again
  at each width. `buildCache` and `partialRebuild` hook into it.
- `internal/ui/messages/wrap_rows.go`, `model.go` — a pane's cache build
  renders no selected variant: `WrapStyles.DeferSelected` has `WrapRows`
  leave `Selected` nil and fill `Lazy`, and `EntryPayload.SelectedLines`
  builds the cursor entry's rows on first use, byte for byte the ones
  `WrapRows` builds (`lazyselected_fork.go`). Upstream's tests that read
  `Entry.Selected` read the rows through the payload instead.
- `internal/ui/messages/model.go` — each `EntryPayload` records the focus
  its rows were built with, and `selectedLines` first calls
  `refocusEntry` (`focus_fork.go`), which renders the entry again when it
  is drawn selected under the other focus; upstream re-renders only the
  entry selected when the focus changes.
- `internal/ui/messages/render_message.go` — the body is styled by
  `renderBodyText` (`bodytext_fork.go`), which measures each line once and
  writes the bytes `styles.MessageText.Render` writes, where upstream
  calls lipgloss (`TestRenderBodyTextMatchesLipgloss` pins the bytes).
- `internal/ui/messages/render.go` — `substituteBgSGR` writes each
  escape's parameters through `writeSubstitutedSGRParams`
  (`sgr_fork.go`), which walks them in place; upstream's tokenizer
  (`splitSGRParams`, `sgrParam`) is deleted.
- `internal/bubbles/entrylist/plain.go` — `buildPlainLine` maps runs of
  ASCII through `plainASCIIRun` (`plain_fork.go`) and asks uniseg only
  from the byte before a non-ASCII one.
- `internal/cache/db.go` — one-line hooks into fork helpers
  (`migrateFork`, `removeThreadSweepLocks`).
- `internal/ui/reducer_threads.go` — a fetched thread is marked through
  `markFetchedThreadRead`.
- `internal/image/probe.go`, `kitty.go`, `cellmetrics.go` — id-matched
  probe replies logged verbatim, cell-size-keyed payload memo, measured
  cell metrics.
- `internal/image/registry.go`, `kitty.go` — a kitty image is sent again
  once more than `kittyStoreBudgetBytes` of uploads went out after it
  (`registry_fork.go`), where upstream sends each image once per process.
  herdr keeps 64 MiB of a pane's images and drops the oldest past that,
  so an emoji sent early in a long session drew blank.
- `internal/ui/thread/model.go` — the thread pane sends the kitty uploads
  of the parent and replies on screen each frame (`flushVisible`,
  `kittyflush_fork.go`), where upstream sends every reply's on each
  layout change; the `flushKey` that gated it is gone. See "Kitty
  uploads follow the screen" below.
- `internal/image/fetcher.go`, `preview.go`, `internal/ui/imgrender/imgrender.go`,
  `internal/ui/messages/blockkit/image.go` — a prerendered image keeps no
  decoded raster: the fetcher stores a decoded entry only when it did not
  prerender, both readers try `Prerendered` before `Cached`, and the
  kitty paths that own their key render through `RenderSource`
  (`kitty_fork.go`). See "Images keep no decoded raster" below.
- `internal/image/sixelframe.go`, `sixelpaint.go` — `SixelFrame` carries
  the frame's `Screen` text and `SixelPlan` the placements it erases
  (`gone`), so the erase rewrites the cells it cleared from the frame
  (`appendFrameCells`, `sixelframe_fork.go`); upstream blanks them to
  the theme background, which Bubble Tea's diff then never repaints.
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
  `cmd/slk/user_resolver_test.go`, the `starred_*_test.go` files,
  `export_collect_test.go` and `internal/ui/username_fanout_test.go`
  (`*usernames.Store` for upstream's `userNameStore`);
  `internal/ui/command_help_test.go` passes because `keys.go` gains help
  entries for `:agent` and `:retitle`;
  `cmd/slk/on_message_mention_test.go`, `rail_unread_test.go`,
  `internal/ui/seams_test.go`, `threadsview/model_test.go` (fixtures built
  on the stores); `internal/ui/mode_normal_keys_test.go` (`O` is
  OpenLinkTab here, image preview is `v` only); `modekeys_test.go` (the pinned
  mode list includes `ModeAppShortcuts` and `ModeProfileCard`); `mode_insert_keys_test.go`
  (no Ctrl+U intercept); `messages/codeblock_wrap_test.go` (the fork's
  filled code block); `golden_test.go` (extra argument on
  `layout.Compute`); most
  `internal/ui/testdata/golden/*.ansi` re-blessed: the deploy card draws
  in the fork's round box (which also shifts the dialogs' backdrops), the
  help overlay lists the fork's keys, the selection tint is reasserted,
  and `window_split.ansi` gains the new-messages line from
  `applyCachedLastRead`. Re-bless with `tools/go.sh test ./internal/ui
  -run TestGolden -update` and check the diff against upstream's goldens
  is only those.
- The reading layout (see "Reading layout") — one-line hooks in
  `messages/render_message.go`, `model.go`, `wrap_rows.go`,
  `thread/model.go` and `view_thread.go`; the theme roles in
  `styles/styles.go` (`FocusedBorder` in `FocusBorderColor`,
  `MessageText` in `TextBody`, `PresenceOnline` and `Username` read
  their roles, `applyRoles` hooked into `Apply`), `messages/render.go`
  (bold, italic, strike and code styles), `statusbar/model.go` and
  `channelfinder/model.go` (online in `Online`), `view_sidebar.go`
  (the focused sidebar draws `FocusedBorder` on its own background);
  `ThemeRoles` embedded
  in `core.Theme` and `styles.ThemeColors`; `MessageWidth` in
  `config.Appearance`. Upstream tests adjusted to the layout: column
  and row positions past the gutter and the blank rows
  (`messages/model_test.go`, `phase3_char_test.go`, `selection_test.go`,
  `sixel_block_integration_test.go`, `partial_rebuild_test.go`,
  `unread_landmark_test.go` and their thread twins,
  `thread/content_pane_api_test.go`, `parent_selection_test.go`), the
  thread header and margin times (`thread/breadcrumb_test.go`,
  `selected_timestamp_test.go`, `internal/ui/thread_breadcrumb_test.go`),
  the theme count (`styles/themes_test.go`), and `golden_test.go`
  (base height 44, the drag at row 9 column 17); every golden
  re-blessed. Two upstream functions the layout no longer calls are
  kept on purpose, so an upstream merge into them stays clean:
  `placeAvatarBeside` (`messages/model.go`; `placeGutter` draws the
  gutter now) and `renderBreadcrumb` (`thread/breadcrumb.go`;
  `renderThreadHeader` draws the header). Their upstream tests still
  run against them.
- `internal/bubbles/entrylist/window.go` — a scroll that leaves only an
  entry's trailing spacer rows on screen moves the cursor past it
  (`followCursor` subtracts `Trail`). The fork's two-row spacers made
  the old behaviour easy to hit.
- `go.mod` / `go.sum` — `github.com/slack-go/slack` bumped past upstream's
  pin to v0.29.0 for typed table cells (`TableRawTextCell` etc.; v0.23.0
  decoded every cell as rich_text and dropped raw_text content). On an
  upstream bump, keep whichever is newer.
- `flake.nix` — `vendorHash` covers the fork's go.mod, not upstream's.
  Whenever go.mod changes (an upstream merge, a bump), the nix CI job
  prints the new hash in its `got:` line; paste it in.
- Docs (`README.md`, `wiki/*`, `docs/STATUS.md`, `docs/superpowers/*`) —
  fork features documented in place; markdown conflicts, resolve by hand.

## Many slks, one cache

Several slks often run at once on one machine, sharing one `cache.db`,
and every one of them gets every Slack event. So a cache write driven by
a Slack event must leave the same result however many slks apply it: set
what the event carries, or count once per thing the event identifies,
never add to what is there.

- `cacheReaction` keeps one entry per user per emoji.
- `CountChannelMention` adds a message to its channel's mention badge
  only when its `(channel_id, ts)` row in `counted_mentions` is new, in
  one transaction. The row outlives a read of the channel, so a slk that
  applies the message after the read doesn't badge it again. Rows a day
  older than the newest counted message in the channel are pruned as
  it is counted.

Their test applies the event through several handlers on one cache file
(`TestOnMessage_MentionCountsEachMessageOnce`; with upstream's
increment, 17 handlers badged one DM as 17).

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

## One backend per role, one bridge per pane

Santa kills locally built binaries, so on a Santa host slk runs in
docker; the commands and checks are in
`docs/developing-on-santa-hosts.md`. Each role (user, agent) has one
long-lived container, `slk-<role>-backend` (`tools/backend.sh`), and
every slk of that role runs in it as an exec. A pane holds two host
processes: its launcher shell (`tools/run-docker.sh`) and
`tools/host-bridge.py`. The bridge runs the exec over the Docker Engine
API on docker's unix socket and relays the pane's terminal to it in raw
mode. It also serves what the container can't reach: the herdr socket,
the macOS clipboard for Ctrl+V, the host's ant login for the model calls
(next section), and the link spool that `o` writes to.
When the pane or its launcher goes away, the bridge stops the pane's
slk.

Why:

- **An exec reaches slk's `main()` in 0.04s, where a `docker run` took
  0.18s.** `tools/go.sh` runs in the same backend: 0.15s per command
  instead of 0.35s.
- **A pane costs 21.4 MiB of host memory, down from 64.9 MiB** with a
  docker CLI process per pane, about 0.9 GiB less across 20 panes. One
  backend held 20 slks in 787 MiB, where 17 per-pane containers took
  about 900 MiB.
- **Docker is in one place.** Every launch, stop, refresh and check of a
  role goes to one container known by name, and the bridge is the only
  process in a pane that talks to docker.

Rejected:

- **A container per pane** (the path before this one): the slower
  `docker run` above, and a container plus a docker CLI process per pane.
- **A host daemon owning every pane** (prototyped): a crash of the one
  process ends every pane, it needs a reaper of its own, and
  `refresh-sessions.sh` and the slk-herdr skill would have to be
  rewritten. A bridge per pane fails one pane at a time.
- **The docker CLI per pane in front of the shared backend** (shipped
  first, replaced the same day): 46 MiB of host memory per pane for a
  process that only relays bytes.

Invariants:

- **Never stop, restart or remove a backend while it runs anything.** It
  holds every pane of its role, and `slk-agent-backend` holds every
  agent's sessions. `backend.sh` recreates a stale backend only while
  nothing runs in it; otherwise it warns and uses it as it is.
- **A run gets one SIGINT.** Bubble Tea stops listening after the first,
  so a second kills slk mid-shutdown. `tools/stop-exec.sh` claims a run
  with a marker directory before it signals, so racing stoppers (the
  launcher and the bridge) send one between them, and whatever is left
  5s later gets SIGKILL. A run is found by the `SLK_EXEC_ID` in its
  processes' environment, never by name or pattern.
- **A pane's slk is stopped within two minutes of its pane closing**, as
  long as docker answers by then. Closing a herdr tab sends HUP, TERM,
  then KILL within half a second, faster than slk shuts down, and an
  exec'd process outlives its client. So the bridge leaves the pane's
  session and ignores those signals, then stops slk: each try gets 20s,
  a failed try repeats every 3s through a paused or hung docker, and at
  120s the bridge gives up and leaves the launch directory to a later
  launch's sweep.
- **No key slk uses is docker's.** Docker holds back each key that could
  begin an attached exec's detach sequence until the next one arrives,
  and the default sequence begins with ctrl+p, slk's channel switcher.
  Detaching can't be turned off, so the bridge moves it to four control
  keys slk binds to nothing (`DETACH_KEYS`).
- **A rebuild never writes the file a session runs.** `ensure_binary`
  builds `bin/<bin>.new` and renames it over the binary, so the panes of
  a role keep running while a launch rebuilds, and no launch runs a
  half-written binary (`go build -o` writes its output in place). Builds
  queue on a flock on `bin/<bin>.lock`, so panes launched together build
  once. Staleness is a fingerprint of the sources' contents
  (`bin/<bin>.src`), not mtimes, which miss a deleted or renamed file.

## Model calls: a key, else the host's ant login

The herdr tab-label and agent status calls (`internal/tablabel`) pick
their credential in one place, `tablabel.ResolveCredential`: a configured
key (`herdr.anthropic_api_key`, then `ANTHROPIC_API_KEY`) wins; without
one, the host's `ant` login; with neither, no model calls and one debug
line. In docker, each pane's bridge serves the login over HTTP: `GET
/token` at `SLK_ANTHROPIC_TOKEN_ADDR` runs `ant auth print-credentials`
on the host and answers with the access token and its expiry alone, or a
503 saying why there is none (ant missing, not logged in). The bridge
fetches as it starts, so slk's startup check finds a token ready. Both
sides reuse a token for a minute at most, so a new `ant auth login` (another
org) or a logout reaches slk within two minutes, and never past 90s
before its expiry; ant refreshes in a token's last two minutes, so the
fetch after that gets a new one. slk
sends it as `Authorization: Bearer` with the `oauth-2025-04-20` beta,
and drops it after a 401 so the next call fetches again.

Why:

- **No long-lived secret in the container.** The refresh token never
  leaves the host; the container sees 8-hour access tokens only.
- **Long sessions keep working.** The token is fetched again near expiry,
  where a token copied in at launch would die 8 hours in.

Rejected:

- **Mounting `~/.config/anthropic` and letting the SDK read the profile**
  (anthropic-sdk-go's `option.WithProfile`): the refresh token in every
  container, and every slk plus the host's ant refreshing and rewriting
  one credentials file.
- **Copying the access token into the state volume at launch**, the way
  the key is: it expires mid-session.
- **Falling back to the ant login when a configured key is rejected**:
  it hides a dead key. A rejected key fails every call with "the
  configured key (...) was rejected; remove it to use the ant login".

Invariants:

- **The host config owns the key.** Every launch makes the state
  volume's `anthropic_api_key` match the host's, deleting it when the
  host has none (`tools/sync-host-api-key.sh`); otherwise a deleted key
  would keep winning from the volume.
- **The endpoint serves only the ant login.** ant runs without an
  exported `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN`, which would
  override its profile. Like the clipboard endpoint, it listens on the
  host's loopback without auth, so a local process can read the token,
  as it can read ant's own credentials file.

## Threads load on demand

The Threads view loads the way it does in the Slack app, and boot makes
no `subscriptions.thread.getView` call.

- **The Threads badge and the rail's thread dot come from Slack.** Boot
  and reconnect ask `client.counts` with `thread_counts_by_channel`, as
  the web client does, and the `update_global_thread_state` event keeps
  them current (`threadsBadge`, `cmd/slk/thread_subscriptions.go`). The
  badge counts unread followed threads: the sum of
  `unread_count_by_channel` (`ThreadsAggregate`).
- **Opening Threads fetches one getView page** (`limit=8`,
  `_x_reason=fetch-threads-view-via-refresh`), and moving near the end of
  the loaded rows fetches the next (`fetch-threads-view-via-load-more`).
  While the view is open, a change Slack reports refetches the first
  page, at most once a second.
- **`thread_subscriptions` is only a cache of per-thread read cursors**,
  for the thread panel's `── new ──` line. `thread_marked` events,
  getView pages and the root of each replies fetch feed it.

Upstream sweeps every followed thread with getView at boot and on
reconnect, to count the badge and the rail from that table. **The sweep
was about 70% of a boot's requests**: 88–116 of 126–160 on a large
workspace, and 18–21s of paging. Without it, boot on that workspace
makes 40 requests warm and 41–43 cold, where it made 51–56 and 86–133
with the fork's cross-pane sweep claim.

Upstream loaded on demand once and went back to the sweep (`2693c094`):
the view and badge read the table, the socket replays nothing after an
offline gap, and the one getView ran once per session, so after a gap
the table stayed stale for the rest of the session. **Here nothing reads
the table for the list or the badge**: the list comes from Slack on
every open, and the badge from Slack at boot, on every reconnect and on
every event. A stale cursor in the table can only misplace a `── new ──`
line.

**A page load never opens a thread.** Slack lists unread threads first.
When a page load opened the selected thread, slk marked it read, the
next first-page refresh no longer held it, the selection fell to the
next unread thread, and the page load opened that one too: a cascade
that marked real threads read in testing. Now only the user opens a
thread, a refresh keeps every loaded row the new page doesn't hold, and
the end-of-list guard accepts the cursor's forward jump where a page
crosses from unread to read threads. `TestThreadsPageLoad_OpensNoThread`,
`TestThreadsRefresh_ReadThreadLeavingFirstPageOpensNothing` and
`TestGetThreadsView_CursorJumpingForwardContinues` pin it.

Rejected: keeping the sweep behind its cross-pane claim (every pane
restart outside its 30-minute window still paid about 100 calls), and
fetching several pages on open (Slack loads on scroll). Not yet done:
dropping `thread_subscriptions` and taking every cursor from the root's
`last_read`, which would also change `OnThreadMarked` and
`markThreadRead`.

The same change, cut from `upstream/main`, is on
`threads/on-demand-upstream` and not yet offered (see "Changes offered
upstream").

## Render caches: an update equals a full build

The channel and thread panes keep each message's rendered rows and
re-render only what an event changes. The rule that keeps this correct:
**an incrementally updated cache equals a fresh full build, byte for
byte.** Tests hold every path to it by applying the event and comparing
each entry with a pane rebuilt from scratch:

- `internal/ui/messages/invalidate_oracle_fork_test.go` and
  `internal/ui/thread/invalidate_oracle_fork_test.go`: every targeted
  invalidation, each entry's selected rows included. A new one gets a
  case there.
- `messages/widthcache_fork_test.go`: the restored width, and after each
  kind of list change, a pane that never opened a thread.
- `messages/lazyselected_fork_test.go`: lazily built selected rows
  against `WrapRows`.
- `thread/viewmemo_fork_test.go` and `messages/focus_fork_test.go`.

The pieces:

- **Row-level invalidation** (`invalidate_fork.go` in each pane). A name
  resolving, a status change, a mark from another client or slk, a
  reaction-nav key, a reload, an older page and a sent message's ts (and
  in the thread a new reply, an edit or a reaction) render only the
  entries they change. Width, theme, emoji, channel names and user
  groups still drop the whole cache.
- **Two widths** (`widthcache_fork.go`). The channel pane keeps the
  cache of the width it was drawn at before, so opening and closing a
  thread restores rows instead of rendering them again. **Any change to
  the message list drops the other width**: a restore carries the scroll
  position and cursor over entry for entry, so its entries must be the
  messages on screen, one for one. A version that kept it across a list
  change moved the cursor to the wrong message.
- **Lazy selected rows** (`lazyselected_fork.go`). A build renders no
  selected variant; the cursor's entry builds its own when first drawn.
- **Thread parent memo** (`viewmemo_fork.go`). The parent's entry and the
  list are kept until the cache, the width, the cursor on the parent,
  the focus, a text selection, the unread boundary or the day changes.
- **Focus recorded per row** (`focus_fork.go`). Each channel entry
  records the focus its selected rows were built with and renders again
  the first time it is drawn selected under the other focus. The thread
  pane rebuilds its replies when the focus changes.

Why: before, each of these events re-rendered the whole pane. Median of
6, 200 messages in a 477×130 window, one View per event:

| Event | Before | After |
|---|---|---|
| Name of someone not on screen resolves | 10.4ms | 0.23ms |
| Mark from another client | 10.3ms | 1.26ms |
| Reaction-nav key | 10.3ms | 0.42ms |
| Channel fetch lands unchanged | 10.2ms | 1.35ms |
| Older page of 50 | 12.4ms | 4.1ms |
| Thread open plus close | 22.9ms | 5.5ms |
| Full build (`BenchmarkBuildCache`) | 8.8ms | 6.1ms |
| Idle thread frame, 200 replies | 300µs | 17µs |

A name change by the author of every message still renders them all.

## Images keep no decoded raster

Upstream keeps every drawn inline image's decoded RGBA at full
cell-pixel size for the rest of the session: in the fetcher's decoded
memo and, for kitty, again as the kitty renderer's source. A 69×20-cell
image at a 17×37 cell is 3.5 MB, against the 400 MiB `GOMEMLIMIT`
`run-docker.sh` sets. The fork keeps neither once the image is
prerendered: the fetch worker stores a decoded entry only when it did
not prerender, both readers (`imgrender.RenderBlock`, blockkit's
`fetchOrPlaceholder`) try `Prerendered` first, and the kitty paths that
own their key render through `KittyRenderer.RenderSource`, which lets
the source go once the render is built.

Heap kept per image, 20 photos of 1280×800 at 69×20 cells: kitty 5.54 →
2.07 MB, halfblock 3.54 → 0.07 MB. The same steps through a thread of
105 images left a pane at 254 MiB, against 408 MiB before.
`TestFetcher_PrerenderedFetchKeepsNoDecodedRaster` and
`TestRenderBlock_PrerenderedImageRendersWithoutDecodedCopy` pin it.

**What stays is the kitty PNG payload**, about 2 MB per image (`payloads`
in `kitty.go`). The registry sends an image again once 48 MiB of uploads
went out after it (`registry_fork.go`), and that re-send uploads the
kept payload. Avatars still set their source with `SetSource`, since
another goroutine may set the same key.

No cap on the payloads: an evicted payload means a fetch and an encode
again when the image is next drawn, a placeholder for about 100ms, and
since this change a pane sits well under its memory limit. Revisit if
an image-heavy pane nears `GOMEMLIMIT`.

## Kitty uploads follow the screen

A kitty image is drawn as placeholder cells naming an image id, and the
terminal must hold that id's pixels. herdr keeps 64 MiB of a pane's
images and drops the oldest upload past that, even one on screen, so
the registry sends an image again once 48 MiB of uploads went out after
it (`registry_fork.go`). **That rule holds only while what slk uploads
at a time fits the terminal's store.** So both panes send only the
uploads of the entries a frame shows, every frame: the channel pane
always did (`messages.Model.View`), and the thread pane does through
`flushVisible` (`thread/kittyflush_fork.go`), the parent included. A
flush sends nothing when the registry thinks the terminal still holds
its image, so an unchanged frame writes nothing, and an image that
scrolls back into view after the terminal dropped it goes out once.

Why: upstream's thread pane sent every reply's images on each cursor
move or resize. On the 105-image #eng thread (200 MiB decoded) the
first pass overflowed herdr's store, the registry then counted every
image as dropped, and every key press sent the whole thread again:
45,707 uploads, 9.4 GiB in one run, ~400 ms per redraw, and the images
sent first drawn blank. `TestKitty_ThreadUploadsOnlyTheImagesOnScreen`
drives 65 images of 1.6 MiB through `kittytest.Terminal`, a stand-in
with herdr's store, down with `j` and back up with the wheel: 4,291
uploads (6.7 GiB) and 261 frames with a blank image before, 104 uploads
and none blank after. `TestKitty_ChannelUploadsOnlyTheImagesOnScreen`
holds the channel pane to the same bounds. Live under herdr on that
thread (2026-11-02), j to the bottom and k back, 370 presses each way:
63,636 uploads and 12.7 GiB written, redraws at p90 512 ms, before;
170 uploads and 35 MiB, p90 5.4 ms, after.

Rejected:

- **Dropping the re-send rule** (upstream's send-once). The thread
  stops looping but an image the store dropped stays blank, which is
  what the rule fixed.
- **Sending the whole thread less often**, or a larger budget. A thread
  bigger than the terminal's store cannot be held at once however often
  it is sent; whatever went first is dropped.
- **Gating on the image's own rows** rather than its entry's. The
  payload carries one flush list per entry, not per row, and the
  entry-level scope costs at most the images of the two entries the
  window's edges cut through, the same as the channel pane.

## Caches that must stay

A study (2026-11-02, headless boots warm against cold, one slk at a
time) asked whether slk could drop its caches and be fast without them.
Its conclusions, so no cache goes without these numbers:

- **The render memos are the performance.** `App.View` from its memo
  takes about 30µs, and `messages.buildCache` for 50 real messages
  13–23ms. Without the entry caches every frame pays that: 4–12% of a
  core per idle pane and about 20ms on every key. The same holds for
  `panelcache.go`, the scrollbar cache, and the avatar and image memos.
- **`cache.db` buys paint, not rate limits.** A cold boot made 155–160
  requests against 126–128 warm, with no 429 either way. What it buys is
  the sidebar at 0.41s instead of 1.08–1.20s (the fork's cache-first
  boot, `cachefirst_fork.go`), and every channel switch or thread open
  painted from SQLite in 7–19ms instead of waiting 120–190ms on the
  network.
- **Smaller caches each save something measured.** `channel_members`
  saves a `conversations.members` call per channel open, the on-disk
  image cache a CDN download (about 365ms) per image per restart, and
  the edge version stores keep payloads small in the official client's
  call shape.
- **Some tables are state, not caches**: `marks`, `frecent`,
  `panestate`, `herdrpane`, `channelvisits`, `agentthreadmarks`. A cold
  boot without them opened the wrong channel.

The one cache that cost more than it saved was `thread_subscriptions`
as the source of the Threads list; see "Threads load on demand". Dead
weight upstream owns (the `files` table, the `channels.latest_synced_ts`
write on every message, `bootstrap.Result.EmojiCacheTS`) is left in
place, since removing it would grow the footprint; it's an upstream PR.

## One slk per pane, measured and kept

The shared backend shares a container, not a process: each pane still
runs its own slk, with its own Slack websockets and its own writes to
the shared `cache.db`. Measured with 17 panes (2026-11-02):

- 34 websockets, one per workspace per slk, and about 50,000 duplicate
  WebSocket frames an hour.
- About 60,000 SQLite writes an hour, 16 of every 17 rewriting a row
  another slk just wrote.
- About 900 MiB and 3–13% of one core across the fleet (since then, 787
  MiB for 20 slks in one backend).
- About 470 lines of fork code that exist only because many slks share
  one state volume: `internal/filelock`, the notification leader
  (`internal/notify/leader.go`), `config_lock.go`, the cache watermark,
  `cacheReaction`, `CountChannelMention`, and atomic image-cache and
  token writes.

Options:

- **A. One slk per role, each pane attaching to it**, as the Slack
  desktop app runs. It saves 32 of 34 sockets, the duplicate events and
  writes, an estimated 500–650 MiB, and the whole cross-process bug
  class.
  - The active workspace is process-global (`router.Active()` at 51
    sites, 40 in `main.go`), and every `p.Send` (57 sites) would fan out
    to N programs, in the upstream files that change most (74 upstream
    commits in 30 days). Weeks of work and permanent merge pain.
  - One crash would take every pane down.
- **B. One websocket relay per role.** About 200–300 fork-only lines and
  one dial hook buy 2 Slack sockets instead of 34. Events, writes and
  memory stay per pane.
- **C. A writer lease per workspace.** It saves only the duplicate
  writes, and loses events while the leader's socket reconnects and the
  followers' are live. Idempotent writes ("Many slks, one cache") do
  better.

None is built: on these numbers the fleet is affordable. Revisit:

- **B**, if Slack's `Ratelimited` socket drops grow with the number of
  sockets. One instance saw 2 in 10 minutes. Measuring one slk's
  reconnect rate with the user panes closed, against the rate with the
  fleet up, decides it.
- **A**, if cross-process bugs keep coming despite the idempotency rule,
  or the pane count grows until memory matters.

## One socket per workspace

Each slk keeps one Slack websocket per workspace, and Slack counts every
socket it still holds open against the user. When `StartWebSocket`'s read
loop ends on an error (Slack's close, a read deadline, a bad frame),
the loop closes its socket. Upstream's loop just returned, and the next
connect dropped the old `*websocket.Conn` for the garbage collector: Slack
still held the socket open, sometimes for minutes, and kept writing to it.
Measured on 21 slks (2026-11-02): up to 15 such sockets per slk.

`ConnectionManager.Run` dials again only after `WsDone`, which closes
after the read loop has ended, so closing in the loop is enough: when the
socket pointer is replaced, the old socket is already closed.
`TestReadLoopExitClosesSocket` holds the server end open and counts it.

When the user holds too many sockets, Slack accepts a new one, sends
`{"type":"error","error":{"msg":"Ratelimited","code":17}}` before any
`hello`, and closes it about 5 s later. Upstream's `Run` reset its backoff
to 1 s on every successful upgrade, so a rejected socket was redialed
every few seconds forever: 375 sockets in 212 s across 21 slks, and their
second workspace never stayed connected. Now:

- The backoff resets only after a socket that got `hello` and then
  stayed up for `healthySocketAge` (60 s, one read-deadline window).
  Anything shorter doubles the wait toward the 30 s cap, like a failed
  dial. A healthy socket that drops still redials within a second.
- Each wait is a random point in the upper half of the backoff, so slks
  that lost their sockets together don't redial together.
- A `retry_after` (seconds) on an error frame, top level or inside
  `error`, sets the shortest wait. The Ratelimited frames seen so far
  carry none.

Reconnect catch-up (`syncOnReconnect`) runs from `OnConnect`, which only
`hello` triggers, so a rejected socket never started one and nothing there
changed.

A quiet socket stays up because slk pings it. Slack's gateway sends
nothing on a quiet workspace, not even a websocket ping, and the read loop
gives a socket up after `wsReadTimeout` (60 s) without a frame. Measured
2026-11-02: a Scorecard socket got `hello` and three frames, then nothing
for 60 s, and ended on the deadline. Every slk gets the same last frame,
so all their sockets for a workspace reached the deadline in the same
instant and redialed together, and Slack answered that wave with
Ratelimited (`retry_after` 10). Now each socket runs `keepAlive`
(`keepalive_fork.go`): Slack's RTM ping, `{"type":"ping","id":N}`, every
15-25 s, a fresh random wait each time so slks whose sockets opened
together drift apart. Slack answers `{"type":"pong","reply_to":N}` within
about 30 ms; the pong extends the deadline like any frame, and the event
owner drops it before dispatch. The deadline is still the dead-socket
check: two pings fit in one window, so a socket ends only after 60 s with
no pong and no frame. With `SLK_DEBUG`, each ping logs the pongs so far,
and each socket's end logs its uptime, read error, and ping and pong
counts.

Rejected:

- **Close the previous socket when `StartWebSocket` replaces it.** `Run`
  only redials after `WsDone`, so the old socket is always closed by then;
  the close would guard a path no caller takes.
- **Reset the backoff on `hello` alone.** Slack can say hello and close
  seconds later; the uptime requirement keeps that from looping at 1 s.
- **Wait the full cap after any Ratelimited frame.** Exponential backoff
  reaches the cap within five rejections, and a slk whose rejection was a
  one-off reconnects sooner.
- **RFC 6455 control pings.** Slack's gateway answers them too (measured),
  but the official client runs in a browser or Electron, whose WebSocket
  API cannot send a control frame, and Slack's RTM docs give the JSON ping
  for keeping a socket alive. slk sends what the official client can send.
- **Ping only after a quiet spell.** A fixed schedule costs a busy socket
  about three small frames a minute, and keeps one code path.
- **A longer read deadline.** Quiet stretches on a small workspace run for
  minutes, and a longer deadline also leaves a dead socket unnoticed
  longer.

Backoff makes a rejected slk cheap; it doesn't get it connected. Each
slk still asks for one socket per workspace, and if Slack's limit sits
below the pane count, option B in "One slk per pane, measured and kept"
(one relay per role) is the fix.

## Reading layout

Both content panes lay messages out for reading, in every theme:

- **A line cap.** Message text wraps at `min(message_width, what the
  pane leaves)` (`messages.TextWidth`; `message_width` defaults to 80,
  0 turns it off). The new-messages line, the thread's replies line
  and its reply box share that width. A code block is at least that
  wide and grows, up to all the pane leaves, to fit its longest line
  (`CodeWidth`): code wraps only where the pane is too narrow, since a
  split identifier or path is worse than a long block. To leave blocks
  whole, prose is wrapped while the blocks are still held out
  (`wrapProse`).
- **A gutter** of 6 columns left of the text (`GutterCols`): the avatar
  in the channel pane (blank with `show_avatars = false`), the time
  right-aligned in the thread pane, which has no avatars.
- **Grouping as Slack groups.** A message from the same author (user ID
  and display name, so a bot posting under two task names is two
  speakers) on the same day, under 5 minutes after the one above, with
  no divider between, continues it: no name row, no avatar, its time in
  the gutter on its first row (`Continues`). Slack documents no gap; 5
  minutes is an assumption.
- **Spacing instead of rules**: one blank row between one person's
  messages, two when the speaker changes, one above and below each day
  divider and the new-messages line.

Why: slk was tiring to read. Text ran to 113 characters a line, every
reply carried a rule and a full name row, and one colour sat on names,
links and borders alike.

The cache rule ("Render caches") still holds with entries that depend
on their neighbours: a channel entry records whether it was drawn as a
continuation (`EntryPayload.Continuation`) and its spacer rows
(`Entry.Trail`), and a rebuild keeps an entry only while its neighbours
give the same answers (`groupingChanged`). The thread pane's replies
carry the same mark, and `setReplyList` redraws any reply whose mark no
longer matches (`regroupReplies`); its blank rows and dividers are
decorations built fresh. `layout_fork_test.go` and
`thread/grouping_fork_test.go` hold each grouping change to a full
rebuild.

**Focus is a shape and a colour.** A focused pane keeps the thick
border (`┏━┓`), in the `focus_border` role; unfocused panes stay rounded
in `border`. The colour alone would not do: coffee's focus border is
1.33:1 against its unfocused one, and meaning never rides on colour
alone (CLAUDE.md, "Status colors"). The sidebar draws the same
focused style as the other panes.

**Theme roles.** Colours come only from theme roles. Eight optional
roles (`core.ThemeRoles`: name, text_body, code, code_background,
focus_border, new_messages, online, time) each fall back to the colour
slk used for that job before (`styles.applyRoles`), so a theme that
sets none renders the same colours; `TestRolesFallBackToTodaysColors`
checks every built-in theme. A config override beats the theme's role,
which beats the fallback. The built-in `coffee` theme sets them all.

Rejected:

- **A cap as a share of the pane.** Reading comfort depends on
  characters per line, not on how much of the window the text fills; a
  percentage gives 50 characters in a narrow pane and 150 in a wide one.
- **Layout per theme.** Other people use this fork; the layout applies
  to every theme, and only colours vary.
- **New roles that replace existing keys.** Every role is optional with
  today's colour as its fallback, so no existing theme or override
  changes.

## Changes offered upstream

Upstream-file changes the fork carries ahead of upstream, each also on
its own branch cut from `upstream/main`. When upstream takes one, the
merge brings the same lines; if upstream takes a different form, keep
upstream's and drop the fork's.

- `internal/bubbles/scrollbar/cache.go`, `scrollbar.go`,
  `internal/bubbles/entrylist/entrylist.go`, `window.go` — `scrollbar.Cache`
  cuts each pane row once instead of on every frame (branch
  `perf-scrollbar-fullwidth`). Since then `SetEntries` calls
  `Cache.Trim(total)` where the branch calls `Reset`, so an appended
  message keeps the rows above it cut; the branch doesn't have it yet.
- `internal/ui/panelcache.go`, `view_messages.go`, `view_thread.go` —
  the channel and thread panes' compose region (typing or status row,
  compose box, bottom edge) is cached on its rendered strings instead of
  re-rendered every frame (branch `perf-compose-region`). The fork's
  `view_compose_region_test.go` passes a `usernames.FromMap` store to
  `SetUserNames` where upstream's passes a map.
- `internal/ui/view_sidebar.go`, `panelcache.go` — the bordered sidebar
  is kept when a read-state refresh leaves every row as it was (branch
  `perf-sidebar-border`).
- `internal/ui/view_helpers.go` — `joinPanelsHorizontal`,
  `stackContentStatus` and `borderedPaneCore` size their builders up
  front (branch `perf-frame-grow`).
- `internal/ui/messages/model.go` — a reaction, edit, reply count, new or
  deleted message, or emoji image landing re-renders only the entries it
  changes (`markEntryStale`, `relayout`), where upstream renders the whole
  channel again (branch `perf-messages-partial-rebuild`). The fork's
  `freshenCache` and `CodeBlockAt` bring a pending relayout up to date
  before they read the entries.
- `internal/ui/messages/wrap_rows.go` — a message's rows are assembled
  directly instead of through four lipgloss renders, byte for byte the
  same (`fillRows`, `wrapRowBreak`; branch `perf-messages-wrap-rows`).
- `cmd/slk/thread_subscriptions.go`, `rail_unread.go`, `connect.go`,
  `reconnect_sync.go`, `internal/slack/client.go`, `events.go`,
  `internal/ui/reducer_threads.go`, `threadsview/model.go` and their
  neighbours — the Threads view loads from Slack a page at a time when it
  opens, and the Threads badge and rail come from Slack's own thread
  counts, where upstream sweeps every followed thread with getView at
  boot and on reconnect (branch `threads/on-demand-upstream`; not yet
  offered, so the fork carries it as an in-place divergence until then).
  The neighbours: `cmd/slk/bootstrap_adapters.go`, `workspace.go`,
  `markread.go`, `main.go`, `internal/bootstrap/bootstrap.go`,
  `internal/cache/thread_subscriptions.go`, `internal/core/ports.go`,
  `adapters.go`, `internal/demo/*`, `internal/slack/activity.go`, and in
  `internal/ui` `app.go`, `msgs.go`, `reducer_send.go`,
  `reducer_workspace.go`, `reducer_mouse.go`. Upstream's
  `cmd/slk/rail_threads_cache.go` and `internal/cache/threads.go` (and
  their tests) are deleted. See "Threads load on demand".
