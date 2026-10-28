# Phase 3 — Collapse the `messages`/`thread` fork — Design

> **Status:** design approved in brainstorming 2026-10-10; spec approved 2026-10-11;
> implemented 2026-10-11 (see "Addenda (implementation)" at the end for what
> changed in flight and which §9 exit criteria were not met).
> **Plan:** [`../plans/2026-10-11-phase3-messages-thread-fork.md`](../plans/2026-10-11-phase3-messages-thread-fork.md)
> (its "Decisions this plan needs from the maintainer" section refines §5 and §6.3).
> **Tracking doc:** [`../plans/2026-09-06-architecture-refactor.md`](../plans/2026-09-06-architecture-refactor.md), Phase 3 (addresses F3).
> **Measured at:** `afad298` (main, after #298 merged 2026-10-10).
> **Related:** RFC #236 (self-contained components in `internal/bubbles/`), and
> the maintainer's answers on it (2026-09-25), which this spec implements for
> the two content panes.

## 1. Goal

`internal/ui/messages.Model` (the channel pane) and `internal/ui/thread.Model`
(the thread pane) are a copy-fork. Phase 3 collapses it into shared code. Shared
behaviour gets one implementation, and genuinely different behaviour stays per
pane: *extract the substrate, not the widget*.

Success is behaviour-preserving and visible in numbers:

- the 4 cloned type declarations (`viewEntry`, `reactionEntryHit`,
  `reactionHitRect`, `EmojiContext`) reduced to one each;
- the three `app.go` mirror pairs (`handleReactionNav`/`handleThreadReactionNav`,
  `openPickerFromMessage`/`openPickerFromThread`,
  `toggleReactionOnSelectedMessage`/`toggleReactionOnSelectedThread`) and the
  mirror arms in `view_*`, `reducer_mouse.go`, `drag.go`, `mode_normal.go`
  and (from #304) `action_prompt.go` collapsed;
- `internal/ui/thread` no longer imports `charm.land/bubbles/v2/viewport`;
- every App golden byte-identical except the one exception in §6.3.

### 1.1 Non-goals

- Changing what either pane does. Every behavioural difference between the
  panes survives Phase 3 as an explicit per-pane option (§5). Converging
  behaviour happens in follow-up PRs, one per option (§9).
- The rest of RFC #236. Phase 3 does not introduce `Update`/`tea.Model` on the
  panes, does not dissolve `styles`, and does not move `blockkit`, `imgrender`
  or `peerstatus` to `slackui`. §3.3 says exactly where Phase 3 stops short of
  the RFC and why.
- Phase 4 (modal chrome). It depends on this phase and is not started here.

## 2. Measured state (re-measured 2026-10-10; the tracking doc's F3 is from 2026-09-06)

| | 2026-09-06 (`4184e60`) | 2026-10-10 (`afad298`) |
|---|---|---|
| `internal/ui/messages/model.go` | 3,559 | 3,795 |
| `internal/ui/thread/model.go` | 2,072 | 2,314 |
| `internal/ui/app.go` | 3,454 | 4,315 |
| Verbatim shared lines, thread→messages, ≥16 chars (§A.2) | 447 | **491** |

The fork is growing. The F3 figure of "377 verbatim lines" could not be
reproduced because its method was not recorded; §A.2 records this spec's, and
the exit criterion is stated against it.

What each model depends on for scrolling (production code):

- `messages` hand-rolls `yOffset` (57 `m.yOffset` sites). It does so to avoid
  `viewport`'s per-`SetContent` width pass (`messages/model.go:233-236`).
- `thread` uses `bubbles/viewport` (26 `m.vp.` sites), calling only `SetContent`,
  `View`, `YOffset`, `SetYOffset`, `ScrollUp`, `ScrollDown`, `Height`,
  `SetWidth`, `SetHeight` and `KeyMap`. It uses none of viewport's convenience
  features. It does, however, depend on four `viewport` behaviours, all of
  which §6.3 has to account for:
  1. `View()` pads the window to width×height with **unstyled** spaces.
  2. `View()` `ansi.Cut`s every visible line when any content line is wider
     than the pane.
  3. `SetYOffset`/`ScrollUp`/`ScrollDown` clamp **eagerly**: `ScrollDown` at the
     bottom, `ScrollUp` at the top and either one with no content are no-ops.
     `messages` clamps lazily in `View`.
  4. `SetContent` runs `ansi.StringWidth` over every content line, and thread
     calls it on every `View` (`thread/model.go:1880`).

## 3. Architecture

### 3.1 Three layers

| Layer | Package | Owns | Must not |
|---|---|---|---|
| **Substrate** | `internal/bubbles/entrylist` (new); `internal/bubbles/scrollbar` and `internal/bubbles/selection` (moved from `internal/ui/`) | pre-rendered `[]Entry`; line offsets and total; scroll offset, snap-to-selection, clamp, width-change re-anchor; the visible window with themed spacer padding; indicator-row slots; the scrollbar; text selection (anchors, extraction, overlay, and the bordered post-pass now called `ApplySelectionToBordered`); per-frame hit-rect translation; `PlainLine` and its helpers | import `internal/ui/**` or `internal/core`; read `styles.*`; know what a message is |
| **Message layer** | `internal/ui/messages` (shared, non-pane code; stays where it is) | rendering a `MessageItem` (one renderer with options); reaction navigation and the reaction/edit/delete mutations; the unified `EmojiContext`; kitty/sixel side effects keyed by entry; `partialRebuild` | own scroll or selection math |
| **Panes** | `messages.Model` (channel), `thread.Model` | the parts that really are different: chrome, empty/loading states, divider and landmark generation, the parent row, unread-boundary lifecycle, the reply affordance | duplicate anything in the two layers above |

`thread` already imports `messages`. That import direction is unchanged.

### 3.2 The substrate: `internal/bubbles/entrylist`

An `entrylist.Model` is a scrollable, selectable list of pre-rendered entries.
It knows nothing about messages.

**`Entry`** (shape. The plan fixes exact names, but not these responsibilities):

| Field | Meaning |
|---|---|
| `ID string` | stable key (a Slack ts). `""` marks a **decoration** row: day divider, unread landmark, thread's `─` separator |
| `Lines []string` | rendered rows, each exactly the list width |
| `Selected []string` | selected variant, same length; nil for decorations |
| `Plain []PlainLine` | ANSI-stripped, column-aligned mirror for text selection; nil for decorations |
| `ColOffset int` | display columns of entry chrome before content (the `▌` border) |
| `Trail int` | trailing rows that belong to the entry but are excluded from snap-to-selection (the channel pane's spacer; 0 in thread) |
| `Ref int` | the owner's index: msgIdx, replyIdx, or −1 for thread's parent |
| `Hits []Hit` | entry-relative clickable rects, each tagged with a kind (reaction, image) and key |

The owner supplies the lines to draw for the selected entry (the
long-/short-timestamp swap stays in the message layer), and is told
synchronously when a substrate behaviour moves the selection (U1, §5.2).

**Owns its size and styles, per the RFC.** `New(opts ...Option) *Model`;
a local `Styles` struct (background, spacer, scrollbar track
and thumb, selection highlight) with `DefaultStyles()` and `SetStyles`. *(As
built, there is no `SetSize` and `App.applyTheme` does not push styles to the
panes; each owner pulls them every frame. See "Departures from §5" in the
addenda.)*

**`PlainLine` moves here.** `messages.PlainLine`, `PlainLines`,
`DisplayWidthOfPlain` and `SliceColumns` (`messages/render.go:1120-1227`)
depend only on `x/ansi` and `uniseg`. They move to `entrylist`, and `messages`
keeps aliases/delegating wrappers for existing callers, as
`ReapplyBgAfterResets` did when it moved to `bubbles/ansi`.

**Version.** The substrate exposes a monotonic `Version()`, and each pane's
`Version()` combines it with its own counter. App-level render caches keep
working unchanged.

### 3.3 Fit with RFC #236

Conforms:

- New code goes to `internal/bubbles/`, as a flat sibling of the substrate
  already there (`ansi`). `scrollbar` and `selection` move beside it, as the
  RFC's target layout lists them.
- `entrylist` reads no `styles.*`, imports nothing under `internal/ui`, and
  takes size and styles from its owner.
- `TestBubblesReachTheAppOnlyThroughCore` covers it from the first commit.
- Substrate packages may import each other (`entrylist` imports `scrollbar` and
  `selection`). Widgets still may not import widgets. That is the RFC's "flat
  siblings" substrate, not a sibling-widget import.

Deliberately stops short (each is an RFC stage outside F3):

- **No `Update`/`tea.Model`** on `entrylist` or the panes. Keys stay routed by
  `App`. Adding `Update` is the RFC's widget-migration stage.
- **The message layer stays in `internal/ui/messages`.** It reads `styles` (123
  sites across the two models today) and calls `blockkit`, `imgrender` and
  `peerstatus`, all under `internal/ui`, which `boundary_test.go` keeps out of
  `internal/bubbles`. It moves when the RFC's theme-snapshot and `slackui`
  stages land. The RFC's `bubbles/message` and `bubbles/thread` are the end
  state, not Phase 3's.
- **Pane `View(height, width)` keeps its geometry parameters.** Panes call
  `entrylist.SetSize` internally. Moving size ownership up to the panes is the
  RFC's stage too.

This is a step towards the RFC that takes on none of its prerequisites, and the
RFC's own rule, "every stage reduces the number of implementations, never adds
one", holds at every PR in §7.

### 3.4 The App side

One unexported interface in `internal/ui`, `contentPane`, covering exactly the
methods the mirror pairs and arms call (selected message, reaction navigation,
hit-testing, selection, scroll, focus). Both models are asserted against it:

```go
var (
	_ contentPane = (*messages.Model)(nil)
	_ contentPane = (*thread.Model)(nil)
)
```

The three `app.go` pairs become one function each, taking a `contentPane` and
the channel ID (`activeChannelID` or `threadPanel.ChannelID()`). The mirror
arms in `view_*`, `reducer_mouse.go`, `drag.go` and `mode_normal.go` collapse
the same way, as does `selectedMessageInFocus` (`action_prompt.go`, added by
#304, §7.1). `App.messagepane`, `App.threadPanel` and `App.winModels` keep
their types, so this costs about zero `App` field churn (§8).

`HitTest` (image click-to-preview) is **not** in `contentPane`. Only the channel
pane has it (divergence 15).

## 4. Decisions

| # | Decision | Chosen | Rejected, and why |
|---|---|---|---|
| D1 | Substrate home | Two layers: neutral substrate in `internal/bubbles/entrylist`; shared message layer stays in `internal/ui/messages` | `internal/ui/pane` (the tracking doc's plan): a sibling-package home the RFC removes, so it would be moved twice. Full RFC now: blocks on theme-snapshot and `slackui` work outside F3 |
| D2 | Scroll mechanism | Hand-rolled `yOffset` (messages' algorithm) in the substrate, as agreed on #236 | Two scroll backends behind an interface keeps the fork at the substrate's core |
| D3 | Thread's `viewport` framing | **Converge now**: thread adopts messages' themed spacer padding. A deliberate, scoped exception to the byte-identical rule, proven cell-identical (§6.3) | A thread-only "viewport-compatible frame" option keeps dead compatibility code alive only to preserve SGR bytes no cell depends on |
| D4 | Divergence policy | **Union of options, behaviour frozen**: every behavioural difference becomes an option, set so each pane behaves exactly as today. Only API and encoding drift converges | Converging behaviour inside the refactor: a refactor PR that also changes behaviour cannot be reviewed (ground rule 2) |
| D5 | Staging | Nine PRs in §7's order, risk first and isolated | Renderer-first: unifies types before the entry type is settled. Fewer, larger PRs: the golden exception would share a PR with other changes |

## 5. The divergence specification

`internal/ui/thread/lockstep_test.go` documents 15 divergences. Brainstorming
measurement found six more behaviours the list omits (U1–U6). Every one is
assigned below. "Option" means implemented once, with each pane choosing its
setting; the setting shown is today's behaviour.

### 5.1 The 15 documented divergences

| # | Divergence | Lands in | Phase 3 outcome |
|---|---|---|---|
| 1 | Construction: `messages.New` returns a value, `thread.New` a pointer | API | **Converge** (PR 7): both return `*Model` |
| 2 | Parent pseudo-row, `parentSelected = −1`, reply-index base | thread pane | Parent is entry 0 with `Ref = −1`; `HitTestReaction` keeps returning reply indices |
| 3 | Leading day-divider seed | panes, through one shared divider generator | The generator takes a seed date: `""` for channel, `DateFromTS(parent.TS)` for thread. The parent's "Yesterday, …" header prefix stays in the thread pane |
| 4 | Inter-row separator: spacer **inside** the entry vs a standalone `─` rule | message-layer entry-builder option | Channel: spacer rows inside the entry (`Entry.Trail`). Thread: rule decoration entries |
| 5 | Pane chrome (channel header vs breadcrumb) | panes | Unchanged |
| 6 | Reaction hit-test row frame | substrate | **Converge** (PR 5) on content-relative (messages' frame). The App's thread caller subtracts chrome like the channel caller does. `TestLockstep_ReactionHitTestFrames` is **deleted**, as its comment requires |
| 7 | Scroll mechanism | substrate | **Converge** (PR 2+3, §6.3) |
| 8 | Unread landmark: (a) setter name, (b) lifecycle, (c) structure | (a) API; (b) thread pane; (c) substrate option `SnapDecorationAnchors` | (a) **converge** on one setter (PR 7); (b) thread still clears on identity change; (c) channel on (a drag over the landmark snaps to a message), thread off (no-op) |
| 9 | Reply-count affordance `[N replies ->]` | renderer option | Channel on, thread off |
| 10 | Search-term highlighting | renderer option | Channel on; thread **off** (capable, not enabled) |
| 11 | Loading state and spinner | channel pane, plus the substrate's top-row slot | Thread never sets it |
| 12 | Empty states (arity, chrome retention, loading fusion) | panes | Unchanged |
| 13 | `View`/`ViewBare`/`ApplySelectionToBordered`; `ClickAt` signature | substrate provides all three | **Converge** API (PR 7): both expose the trio and `ClickAt(y) bool`. The App keeps calling `thread.View`, so no new caching path |
| 14 | Thread's outer `Width/Height/MaxHeight/Background` wrapper | — | **Removed** (PR 2+3, inside the §6.3 exception). It does real work today: the parent row is never width-filled (26 columns in an 80-column pane) and the wrapper pads it. Making the parent a filled entry removes the need |
| 15 | Inline-image click-to-preview | renderer option | Channel on; thread off (keeps discarding `imgrender` hits); `HitTest` stays channel-only |

### 5.2 Behaviours the list omits (added by this spec)

Each is pinned per pane in PR 0 before any production change.

| | Behaviour | Citation | Option | Channel / thread |
|---|---|---|---|---|
| U1 | Selection follows scroll: scrolling the selected message off-screen drags the cursor to the nearest visible message | `messages/model.go:3176-3224` | `CursorFollowsScroll` | on / off |
| U2 | Bottom-pin when the pane gets shorter (e.g. the composer grows) | `messages/model.go:3044-3053` | `PinBottomOnShrink` | on / off |
| U3 | `-- more below --` row on the last line; top-row override (loading-older hint) | `messages/model.go:3476-3485` | `MoreBelowRow`, plus the top-row slot | on / off (slot unused) |
| U4 | Drag-select bumps `Version` | thread `:1068`, `:1093`; messages deliberately does not (`:2815`) | `DirtyOnDrag` | off / on |
| U5 | Width-change re-anchor variants: thread does not re-anchor when scrolled inside the parent (#254, `thread/model.go:1883`); the bottom test uses the new height (channel) vs the old one (thread); channel requires an unchanged message count | `messages/model.go:3071-3130`, `thread/model.go:1567-1579, 1882-1901` | a `Reanchor` policy value | each pane's current policy |
| U6 | Eager vs lazy scroll clamp (§2 item 3). Observable at the model API: drag autoscroll (`drag.go:304-313`), wheel notches and the coalesced flush can call `ScrollUp`/`ScrollDown` with no `View` between | `viewport.go` `ScrollDown`/`ScrollUp`; `messages/model.go:901-929` | `EagerClamp` | off / on |

### 5.3 Performance-only differences (output-neutral; out of scope)

- `SetFocused` invalidates one entry in `messages` (O(1)) and the whole cache in
  `thread`. Left as is.
- Sixel painting exists only in the channel pane (thread renders
  placeholders). It becomes a renderer option, off for thread.
- **A side effect Phase 3 does introduce:** thread re-renders its parent on
  every `View` today (`thread/model.go:1491`). Once the parent is a cached
  entry it no longer does, so the parent's cache invalidation moves onto
  selection, focus and text-selection changes. PR 0 pins all three triggers,
  so a missed invalidation fails a test rather than leaving the parent stale.

## 6. Safety

### 6.1 Ground rules (unchanged from the tracking doc)

- Behaviour-preserving. Every App golden in `internal/ui/testdata/golden/` stays
  **byte-identical** at every commit, except the single exception in §6.3.
- `go build ./... && go vet ./... && gofmt -l . && golangci-lint run && go test ./... -race -count=1`
  before every commit. CI pins golangci-lint **v2.14.0**
  (`.github/workflows/ci.yml`). `AGENTS.md` and the tracking doc still say
  v2.13.1; PR 8 corrects both.
- A bug found mid-refactor is recorded, annotated and raised separately, not
  fixed. #254 is the live example: U5 preserves it.
- `internal/ui` gains no I/O. `internal/bubbles` stays inside
  `TestBubblesReachTheAppOnlyThroughCore`.
- Every new shared helper goes into `AGENTS.md`'s tables in the same commit.

### 6.2 New characterization (PR 0, test-only)

Scroll, selection, scrollbar, search highlighting and loading are pinned only
*within* each model today, never across. PR 0 adds, per pane:

- scroll: wheel, page and snap-to-selection; eager vs lazy clamp (U6); scroll
  with no content;
- scrollbar thumb size and position at several offsets;
- text selection: a drag across a day divider, the landmark and a separator;
  extracted text; the short-timestamp swap; `Version` on drag (U4);
- search highlighting and the loading/more-below rows (channel; U3);
- width-change re-anchor, including the #254 gap (U5); `PinBottomOnShrink`
  (U2); selection-follows-scroll (U1);
- the thread parent's invalidation triggers (§5.3);
- **the overwide-line pin.** `viewport`'s column `Cut` (§2 item 2) can only fire
  when a content line is wider than the pane. Brainstorming tested 9 hostile
  reply bodies (wide CJK; ZWJ, flag and skin-tone emoji; stacked combining
  marks; tabs; zero-width spaces; a 200-column code block; a 120-character URL;
  30× 🎉) at width 40. The widest content line was exactly 40 every time: reply
  rows are filled with `lipgloss` `Width(width-1)` plus the 1-column border,
  and the unfilled parent is narrower, never wider. PR 0 commits that test,
  asserting `max line width ≤ pane width`. If it ever fails, the dropped `Cut` matters
  and PR 2+3 must stop.

And across panes:

- **A scrolling lockstep** at 80×10 with both panes scrolled. It compares only
  shared properties: window rows other than U3's indicator rows, the scrollbar
  column, and the extracted selection text for the same drag.

And tooling:

- a **cell-level golden comparison** helper (per-cell glyph, foreground,
  background, attributes), with a self-test proving it fails on a single
  perturbed cell;
- a benchmark baseline: `BenchmarkViewScroll` (both packages),
  `BenchmarkBuildCache` and `BenchmarkViewFocusFlip` at `-count=10`, recorded in
  the PR.

### 6.3 The one golden exception (PR 2+3)

Converging thread's padding (D3) and making its parent a filled entry (needed
to remove divergence 14) both change **only SGR encoding**. Measured in
brainstorming on throwaway copies of `afad298`:

| Change | Goldens affected | Rows changed (thread_open / wide / narrow) | Cell differences |
|---|---|---|---|
| Hand-rolled window with themed spacer padding, replacing `vp.View()` | `thread_open`, `wide`, `narrow` | 12 / 32 / 3 | **0 / 0 / 0** |
| Parent filled to width (`Width(width-1)` + background) | the same three | — | **0 / 0 / 0** |
| Outer wrapper removed (with padding converged) | the same three | — | **0 / 0 / 0** |

All other goldens are untouched, and the `thread` and `messages` package tests
pass unchanged under the padding change. Removing the wrapper alone, without
filling the parent, fails 7 `thread` assertions (the parent row is narrower
than the pane), which is why the three changes ship together.

**The exception, precisely:** PR 2+3 re-blesses `thread_open.ansi`, `wide.ansi`
and `narrow.ansi` once. The PR proves the cell diff against the previous
goldens is zero, using PR 0's helper, and records the command. No other PR
re-blesses any golden.

### 6.4 Per-PR proof

| PR | Proof of safety |
|---|---|
| 0 | Production diff empty (`git diff --stat -- ':!*_test.go'`); every new test passes on `main`; cell-diff self-test; bench baseline recorded |
| 1 | Phase 1's purity proof: multiset of declaration bytes and import union identical modulo the path rewrite; `boundary_test` green; goldens byte-identical |
| 2+3 | §6.3. U6 and the overwide-line pin unchanged in assertion |
| 4–7 | Goldens byte-identical; every PR 0 test unchanged in assertion (edits limited to how private state is reached, §8); benches within ±10% of the PR 0 baseline (benchstat, `-count=10`) |
| 6 | Additionally `TestLockstep_SharedRenderBehaviour` green |
| 7 | Additionally all 16 mode-handler tables green |
| 8 | Docs only |

## 7. Staging

Each PR is green, behaviour-preserving and reviewable on its own.

| PR | Content |
|---|---|
| **0** | Characterization (§6.2). Test-only |
| **1** | Pure move: `internal/ui/scrollbar` → `internal/bubbles/scrollbar`, `internal/ui/selection` → `internal/bubbles/selection`. Only `messages` and `thread` import them; both are pure (no `internal/ui` imports). `AGENTS.md` tables updated |
| **2+3** | Thread: the parent becomes a filled entry; `bubbles/viewport` is replaced by a hand-rolled offset with eager clamp (U6); the outer wrapper is removed. **The golden exception** (§6.3) |
| **4** | `internal/bubbles/entrylist` with its own tests and `Styles`; `PlainLine` moves in. **Adopted by `messages` first**, because it is the reference algorithm, so the substrate is proven by the pane it was extracted from |
| **5** | `thread` adopts `entrylist` with its frozen-behaviour options. The cloned `viewEntry`/`reactionEntryHit`/`reactionHitRect` reduce to one; the hit-test frame converges (6); `TestLockstep_ReactionHitTestFrames` deleted |
| **6** | Shared message layer: `renderThreadMessage` folds into the one renderer via options (9, 10, 15, sixel); reaction navigation and `updateMessageReaction` are shared; `EmojiContext` unified |
| **7** | App: `contentPane` plus assertions; the three pairs and the mirror arms collapse; divergences 1, 8(a) and 13 converge |
| **8** | Close-out: delete `thread/lockstep_test.go`. Its divergence list lives on as `entrylist`'s option docs plus the per-pane option tests. Update `AGENTS.md` ("Known duplication", the shared-code tables, golangci-lint v2.14.0), and fill in the tracking doc's Phase 3 status and target-state rows |

### 7.1 Sequencing against in-flight work

- **#298 merged** 2026-10-10 (touched both `model.go` files).
- **Part B is open as #304** (`feat/ephemeral-action-dialog` @ `23e09abb`,
  mergeable). **Implementation starts after #304 merges**, and the plan re-runs
  §A's measurements at that point. Where it overlaps Phase 3:
  - it edits **both** `messages/model.go:2364` and `thread/model.go:2109` with
    the same one-line change (`InteractiveHint(msg)`), which is the mirrored
    edit Phase 3 exists to remove. PR 6's single renderer absorbs it;
  - it adds **one new mirror arm**, `selectedMessageInFocus` in
    `internal/ui/action_prompt.go`, which switches on `focusedPanel` between
    `messagepane.SelectedMessage()` and `threadPanel.SelectedReply()`. PR 7
    collapses it with the others, and `contentPane` must cover it;
  - it also changes `app.go` (+28), `mode_normal.go` (+9),
    `messages/ephemeral.go`, `blockkit/render.go` and
    `internal/bubbles/confirmprompt`, and adds `internal/bubbles/buttonrow`.
    None of these change Phase 3's design.
- **Open PRs #300 and #271** touch the App mirror files: #300 `app.go` and
  `mode_normal.go`; #271 those two plus `reducer_mouse.go`. Neither touches
  either model.
  PRs 0–6 are unaffected; **PR 7 rebases onto whichever has merged.** No other
  open PR touches the fork.

## 8. Test churn budget

Moving state is what costs. Counted with the tracking doc's method
(type-checked with `golang.org/x/tools/go/packages`, `Tests: true`, each
selector or composite-literal key in a `_test.go` file whose receiver resolves
to the model and whose member is unexported, counted once; §A.1):

| | White-box test refs (all) | Refs to state that moves to the substrate |
|---|---|---|
| `messages.Model` | 159 refs / 24 members | **100**: `cache` 43, `yOffset` 16, `messageIDToEntryIdx` 8, `lastViewHeight` 6, `lastReactionHits` 6, `selRange` 4, `selectedStartLine` 3, `selectedEndLine` 3, `hasSnapped` 2, `totalLines` 2, `lastHits` 2, `entryOffsets` 2, `cacheWidth` 1, `hasSelection` 1, `resolveAnchor` 1 |
| `thread.Model` | 136 refs / 20 members | **55**: `vp` 16, `entryOffsets` 10, `cache` 7, `selRange` 6, `lastReactionHits` 5, `viewContent` 2, `hasSnapped` 2, `hasSelection` 2, `totalLines` 2, `parentEntry` 1, `lastViewHeight` 1, `snappedSelection` 1 |

`selected`, `chromeHeight`, `replies`, `parent`, `messages`, `version` and
`userNamesV` stay in the panes and cost nothing. The heaviest files are
`messages/model_test.go` (102) and `thread/model_test.go` (71).

Exported-API edits: `HitTestReaction` frame (14 test calls), `ClickAt` (2),
`messages.New` to a pointer (3 test, 2 production), and divergence 8(a): thread's
`SetUnreadBoundary`/`UnreadBoundaryTS` renamed to the channel pane's
`SetLastReadTS`/`LastReadTS` (5 + 12 test calls, 3 production). `App`'s
`messagepane` (302), `threadPanel` (164) and `winModels` (92) refs are
unaffected because the fields keep their types.

**Budget: 250 mechanical test-line edits; measured ≈ 190** (155 state + 36
API). For about 28 relocated members, the `AGENTS.md` rule of thumb (220 per 10)
would predict ~600, so this is well under it. Test reaches into moved state are
rewritten to `entrylist`'s exported API (`YOffset()`, `Entries()`, …) or to an
`export_test.go` accessor in the pane package. Assertions do not change.

## 9. Exit criteria

- 4 cloned type declarations → 1 each; 3 `app.go` mirror pairs → 0.
- `internal/ui/thread` does not import `bubbles/viewport`.
- `thread/model.go` ≤ 1,000 lines (tracking-doc stretch target: < 900).
- Verbatim shared thread→messages lines (§A.2, ≥16 chars): 491 → **< 50**.
- `contentPane` asserted against both models.
- Benches no worse than the PR 0 baseline beyond noise (±10%).
- Goldens: byte-identical except §6.3's single, cell-proven re-bless.
- `go test ./... -race` green at every commit.

**Follow-ups**, each its own issue filed after Phase 3, each a one-option flip
with its own reviewed test changes: U1, U2, U3, U4, U6, 8(c) for thread; #254;
and whether thread should gain 10/11/15.

## Appendix A — Reproducing the measurements

### A.1 Type-checked test-reference count

Walks every `_test.go` file in `./internal/ui/...` with
`packages.Load(&packages.Config{Mode: NeedName|NeedFiles|NeedSyntax|NeedTypes|NeedTypesInfo, Tests: true})`.
It counts (a) every `*ast.SelectorExpr` whose `TypesInfo.Selections` receiver,
after pointer dereference, is `messages.Model`, `thread.Model` or `ui.App` and
whose selected name is unexported, and (b) every unexported key in a composite
literal of those types. Positions are deduplicated, because a package and its
test variant share files. Results at `afad298`: `messages.Model` 159/24,
`thread.Model` 136/20, `ui.App` 3,858/182 (2,905/142 at the end of Phase 0).
The plan carries the program into `docs/superpowers/plans/` alongside the
Phase 3 plan.

### A.2 Verbatim shared lines

For each line of `thread/model.go`: strip whitespace, skip blank and `//`
lines, skip lines shorter than N characters, and count it shared if the same
stripped line occurs anywhere in `messages/model.go ∪ messages/render.go`.
At `afad298`: N=16 → 491 of 986 (50%); N=1 → 995 of 1,515 (66%). At `4184e60`
(`messages/model.go` only): N=16 → 447 of 890.

### A.3 Cell-level golden comparison (§6.3)

Parse each golden row into cells by tracking SGR state (reset, 38/48 truecolor
and 256-colour, 39/49, 22, and other attributes as flags), skipping OSC and
other escapes, and compare `(glyph, sorted attribute set)` per cell. PR 0
commits this as a test helper with a self-test.

## Addenda (implementation)

Recorded at close-out (PR 8, 2026-10-11) from the implementation ledger. The
spec above is left as approved; where the build departed from it, the departure
is here.

### Decisions P1–P4 (the plan's "Decisions this plan needs from the maintainer")

- **P1, zero-reply totals.** `thread.View`'s "No replies yet" branch used to read
  `totalLines`, `entryOffsets` and `cache` left by the previous frame, possibly
  another thread's. It now calls `SetEntries` with the parent and the empty
  block (and renders through `entrylist.Static()`), so the substrate's totals
  describe what is on screen. The only behaviour change is a mouse click below
  the content of a zero-reply thread whose previous frame was taller. Pinned by
  `TestChar_ThreadZeroRepliesParentSelectable` and
  `TestChar_ThreadZeroRepliesTallParentScrolls`.
- **P2, the stale focused border.** `thread.SetFocused` clears only the view
  cache, never the reply cache, so the selected reply keeps its **focused**
  border colour after focus leaves the thread, until something else rebuilds the
  cache. `messages.SetFocused` marks the selected entry stale and is correct.
  Phase 3 preserves the bug. It is pinned by
  `TestChar_ThreadFocusFlipKeepsFocusedBorder` (a `// BUG?:` row; re-pin it, do
  not delete it, when the bug is fixed). **No issue has been filed yet**: the
  maintainer has not decided whether or how to fix it.
- **P3, renderer differences the divergence list missed (U7–U9).** The thread
  renderer drew no `(edited)` mark (**U7**, `RenderOptions.EditedMark`), no
  `↳ replied to a thread` label for `thread_broadcast` replies (**U8**,
  `RenderOptions.BroadcastLabel`) and no avatar although `thread` has a
  `SetAvatarFunc` (**U9**, `RenderOptions.Avatar`, always `""` in thread). Two
  more differences are `RenderContext` fields: the image channel key is the
  channel **ID** in thread and the channel **name** in `messages`
  (`RenderContext.ImageChannel`), and the kitty flush order differs (thread:
  body, blocks, reactions, attachments; channel: blocks, attachments, body,
  reactions), which stays in each pane's entry builder, because
  `RenderedMessage` hands back the four flush slices separately. All are set to
  today's behaviour.
- **P4, four goldens.** No existing golden selected the thread parent, so PR 2+3
  re-blessed `thread_open`, `wide`, `narrow` **and** the new
  `thread_parent_selected`, each proven cell-identical against the pre-PR
  snapshot with `cellDiff`. No other PR re-blessed a golden.

### Departures from §5

- **U5's count gate is unobservable and does not exist.** "The channel pane
  requires an unchanged message count to re-anchor" cannot be observed: every
  `messages` method that changes the message count also sets `m.cache = nil`,
  and a nil cache already disables re-anchoring; the thread's `AddReply` and
  friends all invalidate the same way. `entrylist.ReanchorPolicy` therefore has
  no count gate, only `BottomUsesNewHeight` and `AnchorRefsOnly`.
- **No `SetSize`; styles are pulled per frame, not pushed (supersedes §3.2).**
  The list takes its width through `SetEntries` and its height through
  `SetHeight`. Its owners call `SetStyles(themeListStyles())` (channel) /
  `SetStyles(messages.ThemeListStyles())` (thread) at the top of every `View`
  and in `ApplySelectionToBordered`; `App.applyTheme` only invalidates render
  caches. Pulling means a theme change, in production or in a test that calls
  `styles.Apply` directly, shows on the next frame without the App having to
  reach through to each pane, and no pane can render with a stale snapshot.
- **U3 is a pair of slots, not an option.** `entrylist.Compose(frame, top,
  bottom, applySelection)` takes the top-row override (loading-older hint) and
  the more-below row as arguments. The channel pane passes both; thread passes
  `""`. There is no `MoreBelowRow` option.
- **`KeepLookups`.** Thread rebuilds its replies lazily but must keep answering
  lookups for the parent meanwhile. `entrylist.KeepLookups(n)` makes entry
  lookups see only the first `n` entries until the next `SetEntries` while the
  extent stays; `thread.dropCache` uses `KeepLookups(1)` and the channel pane
  `KeepLookups(0)`. It exists because the lookup-after-invalidate behaviour is
  pinned (`TestChar_ThreadLookupsSeeOnlyTheParentUntilRebuilt`,
  `TestChar_MessagesLookupsMissUntilRebuilt`).
- **The thread parent is still re-rendered every frame** (§5.3 predicted it
  would become a cached entry). `View` rebuilds entry 0 each time, exactly as
  before, which removes the parent-invalidation risk outright. Caching it is a
  follow-up performance change.
- **No shared divider generator.** §5.1 item 3 expected one generator taking a
  seed date. The two panes still run their own loops (`messages/model.go`
  seeds `""`; `thread/model.go` seeds `DateFromTS(parent.TS)`), because the
  loops also differ in landmark placement and separator entries.

### Rulings that changed the plan in flight

- **Stacked on #304.** The plan said to branch from `main` after #304 merged.
  The implementation stacks on #304's head (`23e09abb`) instead, with the spec
  and plan commits at the bottom; if #304 changes in review, every Phase 3
  branch rebases. PR 7 therefore checked whether #300, #271 and #304 had merged
  and did not rebase.
- **Entrylist option tests strengthened beyond the plan.** The plan's
  `TestReanchorRefsOnlySkipsLeadingNonRefEntries`, `TestTrailRowsAreNotSnappedIntoView`
  and `TestPressOnDecoration` passed with their option switched off, so they
  were rewritten to fail without it (spec §5 requires each option to be tested
  per setting, which binds over the plan's test text).
- **`WrapRows` folded into Task 6.3.** The entry-wrapping step (fill, invisible
  or tinted left border, long-timestamp selected variant, plain-text mirror) was
  still a near-copy in both panes after the renderer merged; it became
  `messages.WrapRows`, with each pane keeping its own `Entry` construction.
- **Task 7.2 added tests and one parity fix.** A test that clicks a thread
  reaction pill through `reduceMouseClick` was added before the arm was
  rewritten; Task 7.1's `ClickAt` result, `ViewBare` and `SelectedMessage` got
  behavioural tests; and `thread.ApplySelectionToBordered` now sets the theme
  list styles first, as the channel pane's version already did (a one-line
  parity fix on a `contentPane`-adjacent method).
- **Two exit criteria not met (§9).** `thread/model.go` is 1,353 lines against
  the ≤ 1,000 bar (stretch < 900), down from 2,314. And shared lines < 50: see
  "Achieved" in the tracking document: 491 → 206, not < 50. Also, the literal
  `go test ./... -race -count=5` could not run for `internal/ui` as one process
  (OOM-killed under the 8 GiB sandbox cgroup; its race binary retains about
  1.2 GB per iteration, and the #304 head grows the same); every other package
  passed, and five separate single-iteration race runs of `internal/ui` passed.
  On shared lines: after Task 6.2 it was 233; the remainder is
  per-pane setters, selection wrappers and chrome that no Phase 3 task unified.
  Unifying the setters overlaps RFC #236's directory/pull stage and was kept out
  of scope (controller ruling).

### Where each divergence landed (the lockstep test's list, retired)

`internal/ui/thread/lockstep_test.go` is deleted. Every entry has a home;
"converged" means the difference no longer exists.

| # | Home | Pinned by |
|---|---|---|
| 1 construction | converged: both `New(...) *Model` | `contentPane` assertions (`internal/ui/content_pane.go`) |
| 2 parent pseudo-row | thread pane: entry 0 with `Ref = parentSelected` (−1); `HitTestReaction` returns reply indices | `parent_selection_test.go`, `TestHitTestReaction_OnPill` |
| 3 leading day divider | panes: seed `""` (`messages/model.go`) vs `DateFromTS(parent.TS)` (`thread/model.go`) | `TestViewInsertsDateSeparatorAcrossDays`, `dateseparator_test.go` |
| 4 separator | `entrylist.Entry.Trail` (channel spacer rows) vs rule decoration entries (thread) | `TestTrailRowsAreNotSnappedIntoView`, `TestChar_ThreadSelectionSkipsSeparators`, `TestChar_ThreadCopyExcludesSeparators` |
| 5 chrome | panes: `ChromeHeight()` and the channel header (messages); `renderBreadcrumb` (thread, `ChromeHeight()` now exported) | `breadcrumb_test.go` |
| 6 hit-test row frame | converged: content-relative in both; the App subtracts chrome | `TestHitTestReaction_OnPill` (both panes) |
| 7 scroll mechanism | converged: `bubbles/entrylist` (thread keeps the eager clamp, U6) | `entrylist_test.go` |
| 8a landmark setter | converged: `SetLastReadTS` / `LastReadTS` on both | App-level boundary tests in `app_test.go`, `reducer_focus_test.go` |
| 8b landmark lifecycle | thread pane: `SetThread` clears the boundary when the thread identity changes (`thread/model.go`) | `TestSetThread_ClearsBoundaryOnlyWhenTheThreadChanges` |
| 8c landmark structure | `entrylist.WithSnapDecorationAnchors` (channel on, thread off) | `TestPressOnDecoration`, `TestChar_MessagesSelectionSnapsFromDivider` |
| 9 reply count | `RenderOptions.ReplyAffordance` (channel on) | goldens; no per-setting test |
| 10 search highlight | `RenderContext.SearchTerms` (channel sets, thread leaves nil) | `highlight_test.go`, `TestSetSearchTerms_*` |
| 11 loading state | channel pane `SetLoading`, plus `Compose`'s top-row slot (thread passes `""`) | `TestEmptyStateRendersSpinnerWhenLoading`, `TestLoadingOlderMessagesHintAnimatesAcrossFrames` |
| 12 empty states | panes: "No messages yet" (channel); "No thread selected" and "No replies yet" (thread, via `Static()`) | `TestChar_ThreadZeroReplies*` |
| 13 View / ViewBare / ApplySelectionToBordered / `ClickAt` | converged: both expose the trio and `ClickAt(y) bool` | `contentPane` assertions, `content_pane_api_test.go` |
| 14 outer wrapper | removed in PR 2+3 | the four re-blessed goldens, cell-proven |
| 15 image click-to-preview | `RenderOptions.ImageHits` (channel on); `HitTest` and `HandleImageReady` stay channel-only | `model_test.go` `HitTest` tests |
| U1 | `entrylist.WithCursorFollowsScroll` | `TestCursorFollowsScroll`, `TestChar_MessagesCursorFollowsScroll`, `TestChar_ThreadDoesNotFollowCursorOnScroll` |
| U2 | `entrylist.WithPinBottomOnShrink` | `TestPinBottomOnShrink`, `TestChar_MessagesPinsBottomOnShrink`, `TestChar_ThreadDoesNotPinBottomOnShrink` |
| U3 | `Compose`'s `top` / `bottom` arguments (no option) | `TestComposePadsAndPlacesIndicators`, `TestChar_MessagesMoreBelowRow`, `TestChar_ThreadHasNoMoreBelowRow` |
| U4 | `entrylist.WithDirtyOnDrag` (thread on) | `TestDirtyOnDrag`, `TestChar_MessagesDragBumpsVersionOnce`, `TestChar_ThreadDragBumpsVersion` |
| U5 | `entrylist.WithReanchor(ReanchorPolicy{…})`; no count gate | `TestReanchorBottomUsesOldOrNewHeight`, `TestReanchorRefsOnlySkipsLeadingNonRefEntries`; at pane level `TestChar_MessagesReanchorMeasuresBottomWithNewHeight` (`BottomUsesNewHeight`) and `TestScrollAnchorInsideParentPrefixNotPreservedAcrossWidthResize` (#254; asserts the exact offset, which pins `AnchorRefsOnly`) |
| U6 | `entrylist.WithEagerClamp` (thread on) | `TestEagerClampMakesOvershootANoOp`, `TestLazyClampAbsorbsOvershoot`, `TestChar_ThreadScrollClampsEagerly`, `TestChar_MessagesScrollClampsLazily` |
| U7–U9 | `RenderOptions.EditedMark`, `BroadcastLabel`, `Avatar` | goldens; `TestThreadBroadcastLabel` and `TestSelectedTimestamp_EditedMarkCountsTowardWidth` cover the channel-on side |

Gaps, stated plainly:

- Most `RenderOptions` flips are guarded: the goldens, the thread render tests
  and a 720-input old/new differential run during PR 6 catch them (flipping the
  thread's `EditedMark` fails `TestGolden`, for one). The unguarded ones are the
  thread-side flips of `BroadcastLabel` and `ImageHits`: no test fails when
  either is flipped.
- The retired file was the only test that asserted the rendered
  `── new ──` unread-landmark row, and thread's boundary lifecycle (8b). Both are
  pinned again in PR 8, with each test shown to fail under a mutant that
  changes the text, disables the insertion, or removes/always applies the
  clear: `TestUnreadLandmark_RendersBetweenReadAndUnread` (channel),
  `TestUnreadLandmark_RendersBetweenReadAndUnreadReply` (thread) and
  `TestSetThread_ClearsBoundaryOnlyWhenTheThreadChanges`. The retired file's
  byte-identical comparison of a day-transition divider row beside a wrapped
  line and a reaction pill across the two panes is not restored.

The retired file's two cross-pane tests, `TestLockstep_SharedRenderBehaviour`
and `TestLockstep_ScrolledWindowParity`, go with it: the first compared two
renderers that are now one function, the second compared two scroll
implementations that are now one package.
