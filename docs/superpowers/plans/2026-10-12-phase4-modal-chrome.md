# Phase 4 — Modal chrome substrate — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the box, layout, filter line, selected-row marker, list window and scrollbar that 15 dialogs and popups each re-implement into one stateless package, `internal/bubbles/chrome`, without changing a single cell on screen.

**Architecture:** `chrome` is pure functions plus a `Styles` struct (no state, no `tea.Model`). `messages.ChromeStyles()` builds the `Styles` from the current theme on every draw, each style built exactly as the dialogs build it inline today, so a migrated dialog emits the same bytes. Each dialog keeps its own fields, matching, row content, `Open` and `HandleKey`; it renders its rows as strings and hands them to `chrome.List` and `chrome.Frame`. App-level goldens, added first, lock every dialog's frame; each migration must leave them cell-identical.

**Tech Stack:** Go 1.26, bubbletea v2 (`charm.land/bubbletea/v2`), lipgloss v2 (`charm.land/lipgloss/v2`), `github.com/charmbracelet/x/ansi`, stdlib `testing` only.

**Spec:** [`../specs/2026-10-12-phase4-modal-chrome-design.md`](../specs/2026-10-12-phase4-modal-chrome-design.md). Read it first. Tracking doc: [`2026-09-06-architecture-refactor.md`](2026-09-06-architecture-refactor.md), Phase 4.

## Global Constraints

- Behaviour-preserving refactor. No visible change: every golden stays **cell-identical** to the Task 1 baseline (spec §6.1). A cell difference is a bug in the migration, never a re-bless.
- Do not even out per-dialog differences: width clamps, row caps, matchers, empty-state wording and the workspacefinder truncation quirk (spec §8.1) are all kept exactly.
- A bug found on the way is recorded in the spec's §8.1 and raised separately, never fixed in a migration commit.
- `internal/bubbles/chrome` imports no `internal/ui/...`, no `slack-go`, and does no I/O (`TestBubblesReachTheAppOnlyThroughCore` in `internal/ui/boundary_test.go` enforces it).
- `chrome` never reads `internal/ui/styles`; it draws only with the `chrome.Styles` it is given.
- Tests are stdlib `testing`, white-box (same package as the code), no testify.
- Each task's commit must pass: `go build ./...`, `go vet ./...`, `gofmt -l .` (empty), and the package tests named in the task. Run `go test ./... -race` and `golangci-lint run` at each PR checkpoint (after Tasks 4, 16 and 18).
- Package tests may change only from `listTopOffset` to `listTopOffset()` (spec §5). Any other test edit needs the reason in the commit message.
- One dialog per commit.

## Review Focus

1. **A theme change while a dialog is open.** The dialogs no longer read `styles.*` per span; they read `messages.ChromeStyles()` per draw. A reasonable person expects the open dialog to redraw in the new theme. Pinned by `TestChromeStyles_FollowTheTheme` (Task 4) and `TestRenderBox_FollowsTheTheme` (Task 6).
2. **A click on a row after migration lands on the same row.** `listTopOffset` becomes `chrome.BodyTop`; an off-by-one silently selects the wrong row. Pinned by `TestBodyTop_IsTheFirstBodyRow` (Task 3) and the existing `click_test.go` in each dialog package, which keep running against `listTopOffset()`.
3. **Narrow terminals.** help and linkpicker clamp the box to `termWidth-2` below 42 columns; no existing golden reaches that branch. Pinned by the `dialog_help_narrow` and `dialog_link_picker_narrow` goldens (Task 1).
4. **Rows too long for the box are truncated, not wrapped.** A wrapped row breaks the box and the click geometry. Pinned by the over-long item each list fixture carries (Task 1: `Theme with a name far too long…`, `channel-with-a-name-much-too-long…`, the long link URL and the long search snippet).
5. **Emoji drawn as kitty images** in reactionpicker and emojipicker. The migration must keep the per-draw upload flushes. Pinned by the existing `TestPicker_View_ImageMode_UsesPlacement`, `TestViewOverlayPreservesKittyPlacementSGR` (reactionpicker) and `TestDropdown_View_ImageMode_UsesPlacement` (emojipicker), which Tasks 9 and 17 must run.

## The golden check (used by every migration task)

After Task 1 tags the baseline, restore it into a scratch directory once per session:

```bash
rm -rf /tmp/opencode/phase4-baseline && mkdir -p /tmp/opencode/phase4-baseline
git archive phase4-golden-baseline internal/ui/testdata/golden | tar -x -C /tmp/opencode/phase4-baseline
```

Then, after each migration:

```bash
go test ./internal/ui -run 'TestGolden' -count=1
```

- **PASS:** the migration is byte-identical. Done.
- **FAIL on a golden:** re-bless, then prove the re-bless is cell-identical:

```bash
go test ./internal/ui -run 'TestGolden' -count=1 -update
SLK_GOLDEN_BASELINE=/tmp/opencode/phase4-baseline/internal/ui/testdata/golden \
  go test ./internal/ui -run TestGoldenCellsMatchBaseline -count=1 -v
```

  It must PASS with no `not cell-identical` line (`bytes differ, cells identical` lines are fine; list them in the commit message). If it fails, run `git checkout -- internal/ui/testdata/golden`, and fix the migration. Never commit a golden that fails this check.

## File map

| File | Change |
|---|---|
| `internal/ui/golden_dialogs_test.go` | **new**: one golden per dialog and popup state (Task 1) |
| `internal/ui/testdata/golden/dialog_*.ansi`, `popup_*.ansi` | **new** goldens (Task 1) |
| `internal/bubbles/scrollbar/scrollbar.go` | `Thumb` extracted from `Overlay` (Task 2) |
| `internal/bubbles/chrome/chrome.go` | **new**: `Styles`, `Box`, `Frame`, `BodyTop`, `FilterInput`, `Popup`, `PopupMarker` (Task 3) |
| `internal/bubbles/chrome/list.go` | **new**: `Window`, `Marker`, `List` (Task 3) |
| `internal/bubbles/chrome/contract.go` | **new**: `Dialog`, `Boxed`, `Clickable` (Task 3) |
| `internal/ui/messages/chrome_styles.go` | **new**: `ChromeStyles()` (Task 4) |
| `internal/ui/reducer_modal_click.go`, `internal/ui/confirm.go` | use `chrome.Boxed` / `chrome.Clickable` (Task 4); `confirmPromptStyles` box from `ChromeStyles` (Task 16) |
| `internal/ui/<dialog>/…` | one migration each (Tasks 5–17) |
| `AGENTS.md`, tracking doc, spec | close-out (Task 18) |

---

## PR 1 — goldens and the package

### Task 1: Characterize every dialog with a golden (test-only)

**Files:**
- Create: `internal/ui/golden_dialogs_test.go`
- Create: `internal/ui/testdata/golden/dialog_*.ansi`, `internal/ui/testdata/golden/popup_*.ansi` (blessed by the test)

**Interfaces:**
- Consumes: `newGoldenApp`, `goldenFixtureOpts`, `goldenFinderItems`, `compareGolden`, `stripANSI` (`internal/ui/golden_test.go`); `keyPress` (`internal/ui/modekeys_test.go`); `withSize` (`internal/ui/testapp_test.go`).
- Produces: `dialogGoldens() []dialogGolden`, `renderDialogGolden(t, d) string`, `TestGolden_Dialogs`, and the local git tag `phase4-golden-baseline`. Every later task's golden check depends on them.

Each scenario opens one dialog through its own exported API on top of the standard fixture (120×36 unless `w`/`h` say otherwise), then draws one frame. `shown` proves the dialog actually opened, so a scenario can never bless the bare fixture.

- [ ] **Step 1: Write the golden table and its guards**

Create `internal/ui/golden_dialogs_test.go`:

```go
package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ui/channelpicker"
	"github.com/gammons/slk/internal/ui/help"
	"github.com/gammons/slk/internal/ui/linkpicker"
	"github.com/gammons/slk/internal/ui/mentionpicker"
	"github.com/gammons/slk/internal/ui/newmessagepicker"
	"github.com/gammons/slk/internal/ui/reactionsview"
	"github.com/gammons/slk/internal/ui/searchresults"
	"github.com/gammons/slk/internal/ui/workspacefinder"
)

// dialogGolden is one dialog or composer popup drawn over the standard
// fixture and pinned as testdata/golden/<name>.ansi. Phase 4 moves every
// dialog's chrome into internal/bubbles/chrome; these goldens are what
// prove the move changed nothing on screen (Phase 4 spec §6).
type dialogGolden struct {
	name string
	w, h int // 0: the base fixture's size
	// open puts the dialog on screen, after the fixture is built and
	// before the frame is drawn.
	open func(t *testing.T, a *App)
	// shown reports whether the dialog is open, so a scenario whose
	// dialog failed to open cannot be blessed as coverage.
	shown func(a *App) bool
	// want is text the dialog draws, checked on the stripped frame.
	// Empty for popups, whose rows also appear in the sidebar;
	// TestGolden_DialogsChangeTheFrame covers those.
	want string
}

// numbered returns n labels "<prefix> 01" … "<prefix> NN".
func numbered(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s %02d", prefix, i+1)
	}
	return out
}

// repeat calls f n times.
func repeat(n int, f func()) {
	for range n {
		f()
	}
}

// typeKeys sends each rune of s to a string-keyed HandleKey.
func typeKeys[R any](handle func(string) R, s string) {
	for _, r := range s {
		handle(string(r))
	}
}

// typeInCompose types s into the main composer through its own Update,
// which is what opens the @, # and : popups.
func typeInCompose(a *App, s string) {
	for _, r := range s {
		a.compose, _ = a.compose.Update(keyPress(r))
	}
}

const longThemeName = "Theme with a name far too long to fit inside the switcher box"

func scrollingFinderItems() []core.ChannelFinderItem {
	out := make([]core.ChannelFinderItem, 0, 15)
	for i := range 15 {
		name := fmt.Sprintf("channel-%02d", i+1)
		if i == 7 {
			name = "channel-with-a-name-much-too-long-for-the-finder-box-to-hold"
		}
		typ := "channel"
		if i == 3 {
			typ = "private"
		}
		out = append(out, core.ChannelFinderItem{
			ID: fmt.Sprintf("C%02d", i+10), Name: name, Type: typ,
			Joined: i != 5, LastVisited: int64(100 - i),
		})
	}
	return out
}

func searchItems(n int) []searchresults.Item {
	out := make([]searchresults.Item, 0, n)
	for i := range n {
		out = append(out, searchresults.Item{
			ChannelID: "C1", ChannelName: fmt.Sprintf("eng-%d", i+1),
			UserName: "alice", TS: fmt.Sprintf("%d.0", i+1),
			Text: "we deploy the service on friday afternoons and then watch the " +
				"dashboards for a long while before anyone is allowed to go home",
			Timestamp: "Mar 14", IsDM: i == 2,
		})
	}
	return out
}

func dialogGoldens() []dialogGolden {
	themeShown := func(a *App) bool { return a.themeSwitcher.IsVisible() }
	wsShown := func(a *App) bool { return a.workspaceFinder.IsVisible() }
	presShown := func(a *App) bool { return a.presenceMenu.IsVisible() }
	finderShown := func(a *App) bool { return a.channelFinder.IsVisible() }
	reactShown := func(a *App) bool { return a.reactionPicker.IsVisible() }
	newMsgShown := func(a *App) bool { return a.newMessagePicker.IsVisible() }
	helpShown := func(a *App) bool { return a.help.IsVisible() }
	searchShown := func(a *App) bool { return a.searchResults.IsVisible() }
	linkShown := func(a *App) bool { return a.linkPicker.IsVisible() }

	openHelp := func(t *testing.T, a *App) {
		a.help.SetEntries(help.FromKeyMap(a.keys))
		a.help.SetFooter("slk golden")
		a.help.Open()
		repeat(25, func() { a.help.HandleKey("j") })
		a.SetMode(ModeHelp)
	}
	openLinks := func(t *testing.T, a *App) {
		a.linkPicker.Open("Open link", []linkpicker.Item{
			{URL: "https://example.com/a", Label: "Example"},
			{URL: "https://example.com/a/very/long/path/that/will/not/fit/inside/the/link/picker/box?query=1"},
			{URL: "https://slack.com/archives/C1/p1", InApp: true},
			{URL: "https://example.com/report.pdf", Label: "report.pdf", Detail: "1.2 MB"},
		})
		a.linkPicker.HandleKey("j")
		a.SetMode(ModeLinkPicker)
	}
	openSearch := func(a *App) {
		a.searchResults.Open()
		typeKeys(a.searchResults.HandleKey, "deploy")
		a.searchResults.HandleKey("enter") // input -> loading
		a.SetMode(ModeWorkspaceSearch)
	}

	return []dialogGolden{
		{
			name: "dialog_theme_switcher", want: "Switch Theme", shown: themeShown,
			open: func(t *testing.T, a *App) {
				items := numbered("Theme", 16)
				items[5] = longThemeName // inside the scrolled window [2, 14)
				a.themeSwitcher.SetItems(items)
				a.themeSwitcher.Open()
				repeat(13, func() { a.themeSwitcher.HandleKey("down") })
				a.SetMode(ModeThemeSwitcher)
			},
		},
		{
			name: "dialog_theme_switcher_no_match", want: "No matching themes", shown: themeShown,
			open: func(t *testing.T, a *App) {
				a.themeSwitcher.SetItems(numbered("Theme", 3))
				a.themeSwitcher.Open()
				typeKeys(a.themeSwitcher.HandleKey, "zz")
				a.SetMode(ModeThemeSwitcher)
			},
		},
		{
			name: "dialog_workspace_finder", want: "Switch Workspace", shown: wsShown,
			open: func(t *testing.T, a *App) {
				items := make([]workspacefinder.Item, 0, 14)
				for i, n := range numbered("Workspace", 14) {
					items = append(items, workspacefinder.Item{
						ID: fmt.Sprintf("T%02d", i+1), Name: n, Initials: fmt.Sprintf("W%d", (i+1)%10),
					})
				}
				a.workspaceFinder.SetItems(items)
				a.workspaceFinder.Open()
				repeat(11, func() { a.workspaceFinder.HandleKey("down") })
				a.SetMode(ModeWorkspaceFinder)
			},
		},
		{
			name: "dialog_workspace_finder_no_match", want: "No matching workspaces", shown: wsShown,
			open: func(t *testing.T, a *App) {
				a.workspaceFinder.SetItems([]workspacefinder.Item{{ID: "T1", Name: "Acme", Initials: "A"}})
				a.workspaceFinder.Open()
				typeKeys(a.workspaceFinder.HandleKey, "zz")
				a.SetMode(ModeWorkspaceFinder)
			},
		},
		{
			name: "dialog_presence_menu", want: "Status — Acme", shown: presShown,
			open: func(t *testing.T, a *App) {
				a.presenceMenu.OpenWith("Acme", "active", false, time.Time{})
				repeat(3, func() { a.presenceMenu.HandleKey("down") })
				a.SetMode(ModePresenceMenu)
			},
		},
		{
			name: "dialog_presence_menu_no_match", want: "No matching options", shown: presShown,
			open: func(t *testing.T, a *App) {
				a.presenceMenu.OpenWith("Acme", "active", false, time.Time{})
				typeKeys(a.presenceMenu.HandleKey, "zz")
				a.SetMode(ModePresenceMenu)
			},
		},
		{
			name: "dialog_presence_snooze", want: "Snooze for how many minutes?",
			shown: func(a *App) bool { return a.mode == ModePresenceCustomSnooze },
			open: func(t *testing.T, a *App) {
				a.SetMode(ModePresenceCustomSnooze)
				a.presence.AppendSnoozeDigit("4")
				a.presence.AppendSnoozeDigit("5")
			},
		},
		{
			name: "dialog_channel_finder_scroll", want: "Switch Channel", shown: finderShown,
			open: func(t *testing.T, a *App) {
				a.SetChannelFinderItems(scrollingFinderItems())
				a.channelFinder.Open()
				repeat(12, func() { a.channelFinder.HandleKey("down") })
				a.SetMode(ModeChannelFinder)
			},
		},
		{
			name: "dialog_channel_finder_no_match", want: "No matching channels", shown: finderShown,
			open: func(t *testing.T, a *App) {
				a.SetChannelFinderItems(goldenFinderItems())
				a.channelFinder.Open()
				typeKeys(a.channelFinder.HandleKey, "zqz")
				a.SetMode(ModeChannelFinder)
			},
		},
		{
			name: "dialog_reaction_picker", want: "Add Reaction", shown: reactShown,
			open: func(t *testing.T, a *App) {
				a.reactionPicker.Open("C1", "1.0", []string{"smile"})
				typeKeys(a.reactionPicker.HandleKey, "sm")
				repeat(12, func() { a.reactionPicker.HandleKey("down") })
				a.SetMode(ModeReactionPicker)
			},
		},
		{
			name: "dialog_reaction_picker_no_match", want: "No matching emoji", shown: reactShown,
			open: func(t *testing.T, a *App) {
				a.reactionPicker.Open("C1", "1.0", nil)
				typeKeys(a.reactionPicker.HandleKey, "zqzq")
				a.SetMode(ModeReactionPicker)
			},
		},
		{
			name: "dialog_new_message", want: "New message", shown: newMsgShown,
			open: func(t *testing.T, a *App) {
				users := make([]newmessagepicker.User, 0, 12)
				for i, n := range numbered("User", 12) {
					users = append(users, newmessagepicker.User{
						ID: fmt.Sprintf("U%02d", i+1), DisplayName: n,
						Username: fmt.Sprintf("user%02d", i+1), IsExternal: i == 4,
					})
				}
				a.newMessagePicker.SetUsers(users)
				a.newMessagePicker.Open()
				a.newMessagePicker.HandleKey(" ") // first user becomes a pill
				repeat(11, func() { a.newMessagePicker.HandleKey("down") })
				a.SetMode(ModeNewMessage)
			},
		},
		{
			name: "dialog_new_message_no_match", want: `No users match "zqz"`, shown: newMsgShown,
			open: func(t *testing.T, a *App) {
				a.newMessagePicker.SetUsers([]newmessagepicker.User{{ID: "U1", DisplayName: "alice", Username: "alice"}})
				a.newMessagePicker.Open()
				typeKeys(a.newMessagePicker.HandleKey, "zqz")
				a.SetMode(ModeNewMessage)
			},
		},
		{name: "dialog_help", want: "Keybindings", shown: helpShown, open: openHelp},
		{
			name: "dialog_help_search_no_match", want: "No matching keybindings", shown: helpShown,
			open: func(t *testing.T, a *App) {
				a.help.SetEntries(help.FromKeyMap(a.keys))
				a.help.Open()
				a.help.HandleKey("/")
				typeKeys(func(k string) struct{} { a.help.HandleKey(k); return struct{}{} }, "zqz")
				a.SetMode(ModeHelp)
			},
		},
		{name: "dialog_help_narrow", w: 40, h: 20, want: "Keybindings", shown: helpShown, open: openHelp},
		{
			name: "dialog_search_results", want: "Search Workspace", shown: searchShown,
			open: func(t *testing.T, a *App) {
				openSearch(a)
				a.searchResults.SetHighlightTerms([]string{"deploy"})
				a.searchResults.SetResults(searchItems(8), 20)
				repeat(5, func() { a.searchResults.HandleKey("down") })
			},
		},
		{
			name: "dialog_search_loading", want: "Searching…", shown: searchShown,
			open: func(t *testing.T, a *App) { openSearch(a) },
		},
		{
			name: "dialog_search_no_results", want: "No results", shown: searchShown,
			open: func(t *testing.T, a *App) {
				openSearch(a)
				a.searchResults.SetResults(nil, 0)
			},
		},
		{name: "dialog_link_picker", want: "Open link", shown: linkShown, open: openLinks},
		{name: "dialog_link_picker_narrow", w: 40, h: 20, want: "Open link", shown: linkShown, open: openLinks},
		{
			name: "dialog_reactions_view", want: "Reactions",
			shown: func(a *App) bool { return a.reactionsView.IsVisible() },
			open: func(t *testing.T, a *App) {
				var groups []reactionsview.ReactionGroup
				for i, e := range []string{"thumbsup", "tada", "eyes", "rocket", "heart", "fire"} {
					g := reactionsview.ReactionGroup{Emoji: e, Users: numbered("user", 4), Count: 4}
					if i == 2 {
						g.Count = 9
					}
					groups = append(groups, g)
				}
				a.reactionsView.Open(groups)
				a.SetMode(ModeReactionsView)
				_ = a.View() // HandleKey scrolls only after a draw has set the limit
				repeat(3, func() { a.reactionsView.HandleKey("j") })
			},
		},
		{
			name: "dialog_confirm", want: "Leave #general?",
			shown: func(a *App) bool { return a.confirmPrompt.IsVisible() },
			open: func(t *testing.T, a *App) {
				a.openConfirmPrompt("Leave #general?",
					"You will stop receiving messages from #general. You can rejoin at any time from the channel finder.",
					func() tea.Msg { return nil })
			},
		},
		{
			name:  "popup_mention",
			shown: func(a *App) bool { return a.compose.MentionPickerView(80) != "" },
			open: func(t *testing.T, a *App) {
				a.compose.SetUsers([]mentionpicker.User{
					{ID: "U1", DisplayName: "alice", Username: "alice", InChannel: true},
					{ID: "U2", DisplayName: "bob", Username: "bob", InChannel: true},
					{ID: "U3", DisplayName: "carol", Username: "carol"},
					{ID: "U4", DisplayName: "dave", Username: "dave", IsExternal: true, InChannel: true},
				})
				a.SetMode(ModeInsert)
				_ = a.compose.Focus()
				typeInCompose(a, "@")
			},
		},
		{
			name:  "popup_channel",
			shown: func(a *App) bool { return a.compose.ChannelPickerView(80) != "" },
			open: func(t *testing.T, a *App) {
				a.compose.SetChannels([]channelpicker.Channel{
					{ID: "C1", Name: "general", Type: "channel"},
					{ID: "C2", Name: "engineering", Type: "channel"},
					{ID: "G1", Name: "secret-plans", Type: "private"},
					{ID: "D1", Name: "bob", Type: "dm"},
				})
				a.SetMode(ModeInsert)
				_ = a.compose.Focus()
				typeInCompose(a, "#")
			},
		},
		{
			name:  "popup_emoji",
			shown: func(a *App) bool { return a.compose.EmojiPickerView(80) != "" },
			open: func(t *testing.T, a *App) {
				a.SetMode(ModeInsert)
				_ = a.compose.Focus()
				typeInCompose(a, ":sm")
			},
		},
	}
}

// renderDialogGolden builds the fixture at d's size, opens d and returns
// the frame, failing when the dialog is not actually on screen.
func renderDialogGolden(t *testing.T, d dialogGolden) string {
	t.Helper()
	opts := goldenFixtureOpts()
	if d.w != 0 {
		opts = append(opts, withSize(d.w, d.h))
	}
	a := newGoldenApp(t, opts...)
	d.open(t, a)
	frame := a.View().Content
	if !d.shown(a) {
		t.Fatalf("%s: the dialog is not open; the golden would pin the bare fixture", d.name)
	}
	if d.want != "" && !strings.Contains(stripANSI(frame), d.want) {
		t.Fatalf("%s: %q is not on screen", d.name, d.want)
	}
	return frame
}

func TestGolden_Dialogs(t *testing.T) {
	for _, d := range dialogGoldens() {
		t.Run(d.name, func(t *testing.T) {
			compareGolden(t, d.name, renderDialogGolden(t, d))
		})
	}
}

// TestGolden_DialogsChangeTheFrame proves each scenario draws something:
// its frame differs from the same fixture with nothing opened.
func TestGolden_DialogsChangeTheFrame(t *testing.T) {
	for _, d := range dialogGoldens() {
		t.Run(d.name, func(t *testing.T) {
			opts := goldenFixtureOpts()
			if d.w != 0 {
				opts = append(opts, withSize(d.w, d.h))
			}
			bare := newGoldenApp(t, opts...).View().Content
			if renderDialogGolden(t, d) == bare {
				t.Fatalf("%s renders the same frame as the bare fixture", d.name)
			}
		})
	}
}

func TestGolden_DialogsArePairwiseDistinct(t *testing.T) {
	seen := map[string]string{}
	for _, d := range dialogGoldens() {
		frame := renderDialogGolden(t, d)
		if other, ok := seen[frame]; ok {
			t.Errorf("%q and %q render byte-identical frames", other, d.name)
		}
		seen[frame] = d.name
	}
}
```

- [ ] **Step 2: Run it to see it fail for want of goldens**

Run: `go test ./internal/ui -run 'TestGolden_Dialogs' -count=1`
Expected: FAIL. `TestGolden_Dialogs/*` reports `golden testdata/golden/dialog_….ansi missing or unreadable`. Every other subtest PASSes. If any scenario fails `shown` or `want` instead, the scenario is wrong. Fix the scenario, not the dialog. For the two `_narrow` scenarios only: if the App cannot draw the dialog at 40 columns, raise `w` one column at a time up to 41 (both still take the `termWidth-2` branch). Note the width you used in the commit message.

- [ ] **Step 3: Bless and inspect**

Run: `go test ./internal/ui -run 'TestGolden_Dialogs' -count=1 -update`, then `go test ./internal/ui -run 'TestGolden' -count=1`
Expected: PASS. Then look at three of the files with the escapes stripped, to confirm they show the dialog you expect:

```bash
for f in dialog_theme_switcher dialog_search_results popup_mention; do
  sed 's/\x1b\[[0-9;]*m//g' internal/ui/testdata/golden/$f.ansi | head -30
done
```

The theme switcher should show a scrollbar (`█`/`│` gutter) and `▌` on `Theme 14`, with the long theme name (row 6) ending in `…`. The search results should show 4-line rows. The mention popup should sit above the composer.

- [ ] **Step 4: Run the package suite**

Run: `go test ./internal/ui -count=1`
Expected: PASS.

- [ ] **Step 5: Commit and tag the baseline**

```bash
git add internal/ui/golden_dialogs_test.go internal/ui/testdata/golden/
git commit -m "test(ui): golden for every dialog and composer popup (Phase 4 baseline)"
git tag phase4-golden-baseline
```

The tag is local and is never pushed. Then restore the baseline as described in "The golden check".

---

### Task 2: `scrollbar.Thumb` — one thumb formula

**Files:**
- Modify: `internal/bubbles/scrollbar/scrollbar.go:40-60`
- Test: `internal/bubbles/scrollbar/scrollbar_test.go`

**Interfaces:**
- Produces: `func Thumb(total, offset, visible int) (start, end int)`, the thumb's `[start, end)` rows within a window of `visible` rows. Used by `chrome.List.Gutter` (Task 3).

- [ ] **Step 1: Write the failing test**

Append to `internal/bubbles/scrollbar/scrollbar_test.go`:

```go
func TestThumb(t *testing.T) {
	cases := []struct {
		name                   string
		total, offset, visible int
		start, end             int
	}{
		{"top", 20, 0, 10, 0, 5},
		{"middle rounds down", 20, 5, 10, 2, 7},
		{"bottom", 20, 10, 10, 5, 10},
		{"thumb at least one row", 100, 97, 3, 2, 3},
		{"offset past the end clamps", 20, 50, 10, 5, 10},
		{"no overflow fills the track", 5, 0, 10, 0, 10},
		{"no window", 20, 0, 0, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, e := Thumb(c.total, c.offset, c.visible)
			if s != c.start || e != c.end {
				t.Errorf("Thumb(%d, %d, %d) = [%d, %d), want [%d, %d)",
					c.total, c.offset, c.visible, s, e, c.start, c.end)
			}
		})
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./internal/bubbles/scrollbar -run TestThumb -count=1`
Expected: FAIL to compile with `undefined: Thumb`.

- [ ] **Step 3: Extract `Thumb` and make `Overlay` call it**

In `internal/bubbles/scrollbar/scrollbar.go`, replace the block in `Overlay` from `// Proportional thumb sizing:` through `thumbEnd := thumbStart + thumbHeight` with:

```go
	thumbStart, thumbEnd := Thumb(total, yOffset, visibleHeight)
```

And add after `Overlay`:

```go
// Thumb returns the [start, end) rows of the thumb within a window of
// visible rows scrolled offset rows into total. The thumb is
// visible²/total rows tall, at least one, positioned in proportion to
// offset so it reaches the bottom when the window does. This is the
// one thumb formula: Overlay and the dialog chrome
// (internal/bubbles/chrome) both use it. With no overflow the thumb
// fills the track; with no window it is empty.
func Thumb(total, offset, visible int) (start, end int) {
	if visible <= 0 {
		return 0, 0
	}
	if total <= visible {
		return 0, visible
	}
	height := max(visible*visible/total, 1)
	denom := max(total-visible, 1)
	start = min(max(offset*(visible-height)/denom, 0), visible-height)
	return start, start + height
}
```

Update the package doc comment's first sentence to mention it: `// Package scrollbar renders a 1-column proportional scrollbar gutter onto pre-rendered visible lines, and computes its thumb (Thumb) for callers that draw the gutter themselves.` Keep the rest of the comment.

- [ ] **Step 4: Run the package tests**

Run: `go test ./internal/bubbles/scrollbar -count=1`
Expected: PASS, including every existing `Overlay` test (they prove `Overlay` is unchanged).

- [ ] **Step 5: Commit**

```bash
git add internal/bubbles/scrollbar/
git commit -m "refactor(scrollbar): extract Thumb, the one thumb formula"
```

---

### Task 3: the `chrome` package

**Files:**
- Create: `internal/bubbles/chrome/chrome.go`, `internal/bubbles/chrome/list.go`, `internal/bubbles/chrome/contract.go`
- Test: `internal/bubbles/chrome/chrome_test.go`, `internal/bubbles/chrome/list_test.go`

**Interfaces:**
- Consumes: `scrollbar.Thumb` (Task 2); `ansi.ReapplyAfterResets` (`internal/bubbles/ansi`).
- Produces, exactly as below (every later task uses these names):
  - `type Styles struct { Box, Popup, Title, Input, Placeholder, Row, RowSelected, Marker, PopupMarker, Empty, Footer, Thumb, Track lipgloss.Style; BaseANSI string }`
  - `func Box(s Styles, content string, width int) string`
  - `type Frame struct { Title, Input string; Body []string; Empty, Footer string }` and `func (f Frame) Render(s Styles, width int) string`
  - `func BodyTop(s Styles, hasInput bool) int`
  - `func FilterInput(s Styles, query, placeholder string) string`
  - `func Popup(s Styles, rows []string, width int) string`, `func PopupMarker(s Styles, selected bool) string`
  - `func Window(selected, total, maxRows int) (start, end int)`
  - `func Marker(s Styles, selected bool) string`
  - `type List struct { Start, End, Total int }` with `Scrolls() bool`, `ContentWidth(inner int) int`, `Gutter(s Styles, i int) string`, `Rows(s Styles, selected int, rows []string) []string`
  - `type Dialog interface { IsVisible() bool; Close() }`, `type Boxed interface { BoxSize(termWidth, termHeight int) (int, int) }`, `type Clickable interface { Boxed; ClickRow(termWidth, termHeight, localY int) bool }`

- [ ] **Step 1: Write the failing list tests**

Create `internal/bubbles/chrome/list_test.go`:

```go
package chrome

import (
	"testing"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/bubbles/scrollbar"
)

func TestWindow(t *testing.T) {
	cases := []struct {
		name                     string
		selected, total, maxRows int
		start, end               int
	}{
		{"empty list", 0, 0, 10, 0, 0},
		{"fits", 0, 5, 10, 0, 5},
		{"fits, selection at the end", 4, 5, 10, 0, 5},
		{"overflows, selection at the top", 0, 20, 10, 0, 10},
		{"last row of the first window", 9, 20, 10, 0, 10},
		{"one past the first window scrolls by one", 10, 20, 10, 1, 11},
		{"selection at the end", 19, 20, 10, 10, 20},
		{"no rows allowed", 5, 20, 0, 0, 0},
		{"negative rows allowed", 5, 20, -3, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, e := Window(c.selected, c.total, c.maxRows)
			if s != c.start || e != c.end {
				t.Errorf("Window(%d, %d, %d) = [%d, %d), want [%d, %d)",
					c.selected, c.total, c.maxRows, s, e, c.start, c.end)
			}
		})
	}
}

func TestMarker(t *testing.T) {
	s := testStyles()
	if got := xansi.Strip(Marker(s, true)); got != "▌" {
		t.Errorf("selected marker = %q, want ▌", got)
	}
	if got := Marker(s, false); got != " " {
		t.Errorf("unselected marker = %q, want a plain space", got)
	}
}

func TestList_ContentWidth(t *testing.T) {
	if got := (List{Start: 0, End: 5, Total: 5}).ContentWidth(20); got != 19 {
		t.Errorf("non-scrolling content width = %d, want 19 (marker only)", got)
	}
	if got := (List{Start: 0, End: 5, Total: 9}).ContentWidth(20); got != 18 {
		t.Errorf("scrolling content width = %d, want 18 (marker and gutter)", got)
	}
}

func TestList_GutterFollowsThumb(t *testing.T) {
	s := testStyles()
	l := List{Start: 5, End: 10, Total: 20}
	ts, te := scrollbar.Thumb(20, 5, 5)
	for i := l.Start; i < l.End; i++ {
		want := "│"
		if rel := i - l.Start; rel >= ts && rel < te {
			want = "█"
		}
		if got := xansi.Strip(l.Gutter(s, i)); got != want {
			t.Errorf("Gutter(row %d) = %q, want %q", i, got, want)
		}
	}
	if got := (List{Start: 0, End: 3, Total: 3}).Gutter(s, 1); got != "" {
		t.Errorf("a list that does not scroll drew gutter %q", got)
	}
}

func TestList_Rows(t *testing.T) {
	s := testStyles()
	cases := []struct {
		name     string
		l        List
		selected int
		want     []string
	}{
		{"no scroll", List{Start: 0, End: 2, Total: 2}, 1, []string{" aa", "▌bb"}},
		// Thumb(5, 3, 2) is [1, 2): the second visible row.
		{"scrolled; selected indexes the whole list", List{Start: 3, End: 5, Total: 5}, 4, []string{" aa│", "▌bb█"}},
		{"selection outside the window", List{Start: 0, End: 2, Total: 2}, 9, []string{" aa", " bb"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.l.Rows(s, c.selected, []string{"aa", "bb"})
			for i := range c.want {
				if g := xansi.Strip(got[i]); g != c.want[i] {
					t.Errorf("row %d = %q, want %q", i, g, c.want[i])
				}
			}
		})
	}
}
```

- [ ] **Step 2: Write the failing frame tests**

Create `internal/bubbles/chrome/chrome_test.go`:

```go
package chrome

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/bubbles/ansi"
)

// testStyles has the real geometry (a rounded border and one cell of
// padding, like the theme's box) and attributes in place of colours.
func testStyles() Styles {
	return Styles{
		Box:         lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 1),
		Popup:       lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()),
		Title:       lipgloss.NewStyle().Bold(true),
		Input:       lipgloss.NewStyle().BorderStyle(lipgloss.Border{Left: "▌"}).BorderLeft(true).PaddingLeft(1),
		Placeholder: lipgloss.NewStyle().Faint(true),
		Row:         lipgloss.NewStyle(),
		RowSelected: lipgloss.NewStyle().Bold(true),
		Marker:      lipgloss.NewStyle().Bold(true),
		PopupMarker: lipgloss.NewStyle().Bold(true),
		Empty:       lipgloss.NewStyle().Italic(true),
		Footer:      lipgloss.NewStyle().Faint(true),
		Thumb:       lipgloss.NewStyle(),
		Track:       lipgloss.NewStyle(),
	}
}

// inner returns the box's lines with the border and padding trimmed.
func inner(box string) []string {
	var out []string
	for _, l := range strings.Split(xansi.Strip(box), "\n") {
		out = append(out, strings.Trim(l, "╭╮╰╯─│ "))
	}
	return out
}

func equalLines(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("lines\n got %q\nwant %q", got, want)
	}
}

func TestBox_IsTheBoxStyleOverContentWithBaseReasserted(t *testing.T) {
	s := testStyles()
	s.BaseANSI = "\x1b[48;5;1m"
	content := "\x1b[1mx\x1b[m y"
	want := s.Box.Width(12).Render(ansi.ReapplyAfterResets(content, s.BaseANSI))
	if got := Box(s, content, 12); got != want {
		t.Errorf("Box = %q, want %q", got, want)
	}
	if w := lipgloss.Width(Box(s, "x", 12)); w != 12 {
		t.Errorf("Box width = %d, want 12", w)
	}
}

func TestFrame_Layout(t *testing.T) {
	s := testStyles()
	cases := []struct {
		name string
		f    Frame
		want []string
	}{
		{"input, body and footer",
			Frame{Title: "T", Input: "I", Body: []string{"a", "b"}, Footer: "F"},
			[]string{"", "", "T", "I", "", "a", "b", "", "F", "", ""}},
		{"no input, no footer",
			Frame{Title: "T", Body: []string{"a"}},
			[]string{"", "", "T", "", "a", "", ""}},
		{"empty body draws Empty",
			Frame{Title: "T", Input: "I", Empty: "none"},
			[]string{"", "", "T", "I", "", "none", "", ""}},
		{"empty body, no Empty, keeps the blank body line",
			Frame{Title: "T", Input: "I"},
			[]string{"", "", "T", "I", "", "", "", ""}},
		{"Empty ignored when there is a body",
			Frame{Title: "T", Body: []string{"a"}, Empty: "none"},
			[]string{"", "", "T", "", "a", "", ""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			equalLines(t, inner(c.f.Render(s, 20)), c.want)
		})
	}
}

func TestBodyTop_IsTheFirstBodyRow(t *testing.T) {
	s := testStyles()
	for _, hasInput := range []bool{true, false} {
		f := Frame{Title: "T", Body: []string{"BODY"}}
		if hasInput {
			f.Input = "I"
		}
		lines := strings.Split(xansi.Strip(f.Render(s, 20)), "\n")
		row := -1
		for i, l := range lines {
			if strings.Contains(l, "BODY") {
				row = i
			}
		}
		if got := BodyTop(s, hasInput); got != row {
			t.Errorf("BodyTop(hasInput=%v) = %d, body drawn on row %d", hasInput, got, row)
		}
	}
	if BodyTop(s, true) != 5 || BodyTop(s, false) != 4 {
		t.Errorf("BodyTop = %d/%d, want 5/4 for the theme's box", BodyTop(s, true), BodyTop(s, false))
	}
}

func TestFilterInput(t *testing.T) {
	s := testStyles()
	if got := xansi.Strip(FilterInput(s, "", "Type to filter...")); got != "▌ █ Type to filter..." {
		t.Errorf("empty query = %q", got)
	}
	if got := xansi.Strip(FilterInput(s, "abc", "Type to filter...")); got != "▌ abc█" {
		t.Errorf("query = %q", got)
	}
}

func TestPopup(t *testing.T) {
	s := testStyles()
	if got := Popup(s, nil, 20); got != "" {
		t.Errorf("Popup with no rows = %q, want empty", got)
	}
	box := Popup(s, []string{PopupMarker(s, true) + "a", PopupMarker(s, false) + "b"}, 20)
	if w := lipgloss.Width(box); w != 18 {
		t.Errorf("popup width = %d, want 18 (width-2)", w)
	}
	equalLines(t, inner(box), []string{"", "▌ a", "b", ""})
}
```

- [ ] **Step 3: Run them to see them fail**

Run: `go test ./internal/bubbles/chrome -count=1`
Expected: FAIL to compile (`undefined: Styles`, `Window`, …).

- [ ] **Step 4: Write `contract.go`**

```go
package chrome

// Dialog is what every dialog and composer popup provides: whether it
// is open, and a way to close it.
type Dialog interface {
	IsVisible() bool
	Close()
}

// Boxed is a dialog drawn as a centered box. BoxSize is the box's outer
// size for a terminal of the given size; the App's click router centers
// it the way the dimmed overlay does, to tell a click inside the box
// from one outside it.
type Boxed interface {
	BoxSize(termWidth, termHeight int) (int, int)
}

// Clickable is a Boxed dialog with a selectable list. ClickRow moves the
// selection to the row at box-local localY (0 is the top border) and
// reports whether a row was there.
type Clickable interface {
	Boxed
	ClickRow(termWidth, termHeight, localY int) bool
}
```

- [ ] **Step 5: Write `list.go`**

```go
package chrome

import "github.com/gammons/slk/internal/bubbles/scrollbar"

// Window returns the [start, end) rows of a list of total rows to show
// in at most maxRows rows so that selected is visible: the top of the
// list until selected passes the last row, then the window that ends at
// selected. It is the one list window every dialog uses.
func Window(selected, total, maxRows int) (start, end int) {
	n := min(maxRows, total)
	if n <= 0 {
		return 0, 0
	}
	if selected >= n {
		start = selected - n + 1
	}
	end = start + n
	if end > total {
		end = total
		start = max(end-n, 0)
	}
	return start, end
}

// Marker is the column in front of a list row: ▌ in s.Marker for the
// selected row, a plain space otherwise.
func Marker(s Styles, selected bool) string {
	if selected {
		return s.Marker.Render("▌")
	}
	return " "
}

// List is the visible window [Start, End) of a list of Total rows.
// A dialog renders each row's content ContentWidth wide; Rows (or
// Marker and Gutter, for rows that span several lines) adds the marker
// column and, when the list scrolls, the scrollbar gutter.
type List struct {
	Start, End, Total int
}

// Scrolls reports whether the list is longer than its window, which is
// when it gets a scrollbar.
func (l List) Scrolls() bool {
	return l.Total > l.End-l.Start
}

// ContentWidth is the width left for a row's content inside inner
// columns, after the marker column and, when the list scrolls, the
// gutter column.
func (l List) ContentWidth(inner int) int {
	w := inner - 1
	if l.Scrolls() {
		w--
	}
	return w
}

// Gutter is the scrollbar cell for row i (an index into the whole list,
// within [Start, End)): █ in s.Thumb on the thumb, │ in s.Track
// elsewhere, and "" when the list does not scroll. The thumb is
// scrollbar.Thumb in row units, so every line of a multi-line row gets
// the same cell.
func (l List) Gutter(s Styles, i int) string {
	if !l.Scrolls() {
		return ""
	}
	ts, te := scrollbar.Thumb(l.Total, l.Start, l.End-l.Start)
	if rel := i - l.Start; rel >= ts && rel < te {
		return s.Thumb.Render("█")
	}
	return s.Track.Render("│")
}

// Rows draws rows, the content of rows [Start, Start+len(rows)), each
// already ContentWidth wide: the marker, the content, the gutter.
// selected is an index into the whole list.
func (l List) Rows(s Styles, selected int, rows []string) []string {
	out := make([]string, len(rows))
	for k, r := range rows {
		i := l.Start + k
		out[k] = Marker(s, i == selected) + r + l.Gutter(s, i)
	}
	return out
}
```

- [ ] **Step 6: Write `chrome.go`**

```go
// Package chrome is the dialog chrome slk's dialogs and composer popups
// share: the bordered box, the title / input / body / footer layout, the
// filter line, the selected-row marker, the list window and the
// scrollbar gutter. It holds no state. A dialog keeps its own fields,
// matching, row content and keys, renders its rows as strings, and
// draws them through this package. The theme arrives as a Styles; the
// App builds one from its palette (messages.ChromeStyles).
package chrome

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/bubbles/ansi"
)

// Styles is everything chrome draws with. None of them sets a Width;
// chrome applies widths itself.
type Styles struct {
	Box         lipgloss.Style // a centered dialog's border, padding and background
	Popup       lipgloss.Style // a composer popup's border and background
	Title       lipgloss.Style
	Input       lipgloss.Style // the ▌-bordered filter line
	Placeholder lipgloss.Style // the filter line's placeholder
	Row         lipgloss.Style // a row's content
	RowSelected lipgloss.Style // the selected row's content
	Marker      lipgloss.Style // the ▌ before the selected row
	PopupMarker lipgloss.Style // the "▌ " before a popup's selected row
	Empty       lipgloss.Style // "No matching …", and other italic muted lines
	Footer      lipgloss.Style // footer hints, and other muted lines
	Thumb       lipgloss.Style // the scrollbar thumb █
	Track       lipgloss.Style // the scrollbar track │

	// BaseANSI is re-asserted after every SGR reset inside Box, so the
	// resets nested renders emit cannot drop the box's background.
	// Empty disables it.
	BaseANSI string
}

// Box draws content inside s.Box at the given outer width, with
// s.BaseANSI re-asserted after every reset in content. It is the one box
// renderer; Frame lays a dialog out on top of it.
func Box(s Styles, content string, width int) string {
	return s.Box.Width(width).Render(ansi.ReapplyAfterResets(content, s.BaseANSI))
}

// Frame is a dialog's layout: the title, the input line, a blank line,
// the body, then a blank line and the footer. Title and Empty are plain
// text, drawn in s.Title and s.Empty; Input, Body and Footer arrive
// rendered. An empty Input or Footer is left out with its line; Empty is
// drawn in place of an empty Body.
type Frame struct {
	Title  string
	Input  string
	Body   []string
	Empty  string
	Footer string
}

// Render draws the frame inside Box at the given outer width.
func (f Frame) Render(s Styles, width int) string {
	body := f.Body
	if len(body) == 0 && f.Empty != "" {
		body = []string{s.Empty.Render(f.Empty)}
	}
	var b strings.Builder
	b.WriteString(s.Title.Render(f.Title))
	if f.Input != "" {
		b.WriteString("\n")
		b.WriteString(f.Input)
	}
	b.WriteString("\n\n")
	b.WriteString(strings.Join(body, "\n"))
	if f.Footer != "" {
		b.WriteString("\n\n")
		b.WriteString(f.Footer)
	}
	return Box(s, b.String(), width)
}

// BodyTop is the box-local row of a Frame's first body line drawn in
// s.Box (0 is the top border): the border and padding, the title, the
// input line when hasInput, and the blank line. A dialog's ClickRow
// subtracts it from the clicked row.
func BodyTop(s Styles, hasInput bool) int {
	top := s.Box.GetBorderTopSize() + s.Box.GetPaddingTop() + 2
	if hasInput {
		top++
	}
	return top
}

// FilterInput is the filter line: the placeholder after a block cursor
// while query is empty, the query followed by the cursor after.
func FilterInput(s Styles, query, placeholder string) string {
	text := query + "█"
	if query == "" {
		text = "█ " + s.Placeholder.Render(placeholder)
	}
	return s.Input.Render(text)
}

// Popup draws a composer popup's rows inside s.Popup, width-2 wide so it
// lines up with the composer under it. No rows draws nothing.
func Popup(s Styles, rows []string, width int) string {
	if len(rows) == 0 {
		return ""
	}
	return s.Popup.Width(width - 2).Render(strings.Join(rows, "\n"))
}

// PopupMarker is the two-column marker in front of a popup row: "▌ " in
// s.PopupMarker for the selected row, two spaces otherwise.
func PopupMarker(s Styles, selected bool) string {
	if selected {
		return s.PopupMarker.Render("▌ ")
	}
	return "  "
}
```

- [ ] **Step 7: Run the tests**

Run: `go test ./internal/bubbles/chrome -count=1 && go test ./internal/ui -run 'TestBubblesReachTheAppOnlyThroughCore|TestTUIReachesTheAppOnlyThroughCore' -count=1`
Expected: PASS. If `TestFrame_Layout` or `TestPopup` disagree about blank lines, print `inner(...)` and fix the **test's expectation only** if `Render` matches the content layout in Step 6 exactly (title, `\n`, input, `\n\n`, body, `\n\n`, footer). That layout is what the dialogs draw today and is not negotiable.

- [ ] **Step 8: Commit**

```bash
git add internal/bubbles/chrome/
git commit -m "feat(bubbles): chrome, the stateless dialog chrome substrate"
```

---

### Task 4: `messages.ChromeStyles` and the declared contract in the click router

**Files:**
- Create: `internal/ui/messages/chrome_styles.go`
- Test: `internal/ui/messages/chrome_styles_test.go`
- Modify: `internal/ui/reducer_modal_click.go:33-70`, `internal/ui/confirm.go:72-90`

**Interfaces:**
- Consumes: `chrome.Styles`, `chrome.Boxed`, `chrome.Clickable` (Task 3); `messages.BgANSI`, `messages.FgANSI`.
- Produces: `func ChromeStyles() chrome.Styles` in package `messages`. Every dialog migration calls it once per draw.

- [ ] **Step 1: Write the failing tests**

Create `internal/ui/messages/chrome_styles_test.go`. The `want` expressions are copied verbatim from the dialogs' inline code; that is the point of the test.

```go
package messages

import (
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/styles"
)

// TestChromeStyles_MatchTheInlineStyles pins each chrome style to the
// expression the dialogs built inline before Phase 4, so a dialog moved
// onto chrome renders the same bytes.
func TestChromeStyles_MatchTheInlineStyles(t *testing.T) {
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	styles.Apply("dark", config.Theme{})
	bg := styles.Background
	s := ChromeStyles()

	cases := []struct {
		name      string
		got, want lipgloss.Style
	}{
		{"Box", s.Box, lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(styles.Primary).BorderBackground(bg).Background(bg).Padding(1, 1)},
		{"Popup", s.Popup, lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(styles.Primary).Background(styles.SurfaceDark)},
		{"Title", s.Title, lipgloss.NewStyle().Bold(true).Background(bg).Foreground(styles.Primary)},
		{"Input", s.Input, lipgloss.NewStyle().BorderStyle(lipgloss.Border{Left: "\u258c"}).BorderLeft(true).BorderForeground(styles.Primary).BorderBackground(bg).PaddingLeft(1).Background(bg).Foreground(styles.TextPrimary)},
		{"Placeholder", s.Placeholder, lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted)},
		{"Row", s.Row, lipgloss.NewStyle().Background(bg).Foreground(styles.TextPrimary)},
		{"RowSelected", s.RowSelected, lipgloss.NewStyle().Background(bg).Foreground(styles.Primary).Bold(true)},
		{"Marker", s.Marker, lipgloss.NewStyle().Background(bg).Foreground(styles.Accent)},
		{"PopupMarker", s.PopupMarker, lipgloss.NewStyle().Foreground(styles.Accent)},
		{"Empty", s.Empty, lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted).Italic(true)},
		{"Footer", s.Footer, lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted)},
		{"Thumb", s.Thumb, lipgloss.NewStyle().Background(bg).Foreground(styles.Primary)},
		{"Track", s.Track, lipgloss.NewStyle().Background(bg).Foreground(styles.Border)},
	}
	for _, c := range cases {
		got := c.got.Width(12).Render("ab\ncd")
		want := c.want.Width(12).Render("ab\ncd")
		if got != want {
			t.Errorf("%s renders %q, want %q", c.name, got, want)
		}
	}
	if want := BgANSI() + FgANSI(); s.BaseANSI != want {
		t.Errorf("BaseANSI = %q, want %q", s.BaseANSI, want)
	}
}

// TestChromeStyles_FollowTheTheme: built per call, so a dialog drawn
// after a theme change draws in the new theme with nothing pushed.
func TestChromeStyles_FollowTheTheme(t *testing.T) {
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	styles.Apply("dark", config.Theme{})
	dark := ChromeStyles()
	styles.Apply("light", config.Theme{})
	light := ChromeStyles()
	if dark.Title.Render("x") == light.Title.Render("x") {
		t.Error("Title did not change with the theme")
	}
	if dark.BaseANSI == light.BaseANSI {
		t.Error("BaseANSI did not change with the theme")
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/ui/messages -run TestChromeStyles -count=1`
Expected: FAIL to compile with `undefined: ChromeStyles`.

- [ ] **Step 3: Write `chrome_styles.go`**

```go
package messages

import (
	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/bubbles/chrome"
	"github.com/gammons/slk/internal/ui/styles"
)

// ChromeStyles maps the current theme onto the dialog chrome
// (internal/bubbles/chrome). It is built on every call, so a dialog that
// calls it while drawing always has the current theme and applyTheme
// pushes nothing. Each style is exactly the one the dialogs built inline
// before Phase 4 (TestChromeStyles_MatchTheInlineStyles).
func ChromeStyles() chrome.Styles {
	bg := styles.Background
	on := func() lipgloss.Style { return lipgloss.NewStyle().Background(bg) }
	return chrome.Styles{
		Box: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.Primary).
			BorderBackground(bg).
			Background(bg).
			Padding(1, 1),
		Popup: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(styles.Primary).
			Background(styles.SurfaceDark),
		Title: on().Foreground(styles.Primary).Bold(true),
		Input: lipgloss.NewStyle().
			BorderStyle(lipgloss.Border{Left: "\u258c"}).
			BorderLeft(true).
			BorderForeground(styles.Primary).
			BorderBackground(bg).
			PaddingLeft(1).
			Background(bg).
			Foreground(styles.TextPrimary),
		Placeholder: on().Foreground(styles.TextMuted),
		Row:         on().Foreground(styles.TextPrimary),
		RowSelected: on().Foreground(styles.Primary).Bold(true),
		Marker:      on().Foreground(styles.Accent),
		PopupMarker: lipgloss.NewStyle().Foreground(styles.Accent),
		Empty:       on().Foreground(styles.TextMuted).Italic(true),
		Footer:      on().Foreground(styles.TextMuted),
		Thumb:       on().Foreground(styles.Primary),
		Track:       on().Foreground(styles.Border),
		BaseANSI:    BgANSI() + FgANSI(),
	}
}
```

- [ ] **Step 4: Run them to see them pass**

Run: `go test ./internal/ui/messages -run TestChromeStyles -count=1`
Expected: PASS.

- [ ] **Step 5: Move the router onto the declared contract**

In `internal/ui/reducer_modal_click.go`:
- Add the import `"github.com/gammons/slk/internal/bubbles/chrome"`.
- Delete the `boxedOverlay` and `clickableOverlay` declarations and their comments (lines 37–50).
- Change `pointClickable` to embed `chrome.Boxed`:

```go
// pointClickable is a chrome.Boxed modal whose hot spot is a single cell
// range rather than a list row. ClickAt reports whether the box-local
// cell (localX, localY) is on it. A modal implements this or
// chrome.Clickable, not both.
type pointClickable interface {
	chrome.Boxed
	ClickAt(termWidth, termHeight, localX, localY int) bool
}
```

- In `modalClickTarget`, change the field types: `box chrome.Boxed` and `click chrome.Clickable // nil for non-list modals (e.g. confirm)`.
- Update the file comment line `// Geometry comes from each modal's BoxSize, …` to add: `The contract (chrome.Boxed, chrome.Clickable) is declared in internal/bubbles/chrome and asserted by each modal package.`

In `internal/ui/confirm.go`: change `var _ boxedOverlay = confirmPromptBox{}` to `var _ chrome.Boxed = confirmPromptBox{}`. Change the comment above `confirmPromptBox` to `// confirmPromptBox adapts the size-owning prompt to chrome.Boxed, the interface the modal click router shares with the other modals.`, and add the `chrome` import.

- [ ] **Step 6: Verify**

Run: `go build ./... && go vet ./internal/ui/... && go test ./internal/ui -run 'Modal|Click|Confirm|TestGolden' -count=1 && gofmt -l .`
Expected: build clean, tests PASS, `gofmt -l` prints nothing. `grep -rn 'boxedOverlay\|clickableOverlay' internal/` prints nothing.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/messages/chrome_styles.go internal/ui/messages/chrome_styles_test.go internal/ui/reducer_modal_click.go internal/ui/confirm.go
git commit -m "feat(ui): ChromeStyles maps the theme onto chrome; the click router uses chrome's contract"
```

- [ ] **Step 8: PR 1 checkpoint**

Run: `go test ./... -race && go vet ./... && golangci-lint run && gofmt -l .`
Expected: all clean. Then `git branch refactor/phase4/pr1-substrate`.

---

## PR 2 — the dialogs, one per task

Every task below follows the same shape: replace the dialog's box, filter line, marker and scrollbar code with `chrome`, keep everything else, add the compile-time assertion, change `listTopOffset` from a constant to a function, run the package tests, run **the golden check**, and commit. Every task's code is written out in full. A task's code is right only if the golden check passes.

`listTopOffset`, wherever a dialog has it, becomes:

```go
// listTopOffset is the box-local row of the first list row.
func listTopOffset() int { return chrome.BodyTop(messages.ChromeStyles(), true) }
```

Its uses in code change `listTopOffset` to `listTopOffset()`, and the package's tests are updated with:

```bash
perl -pi -e 's/\blistTopOffset\b(?!\()/listTopOffset()/g' internal/ui/<pkg>/*_test.go
```

### Task 5: themeswitcher

**Files:**
- Modify: `internal/ui/themeswitcher/model.go`
- Test: `internal/ui/themeswitcher/click_test.go` (the `listTopOffset()` rename only)

**Interfaces:**
- Consumes: `chrome.Window`, `chrome.List`, `chrome.Frame`, `chrome.FilterInput`, `chrome.BodyTop`, `chrome.Dialog`, `chrome.Clickable` (Task 3); `messages.ChromeStyles` (Task 4).
- Produces: nothing new. `visibleWindow`, `BoxSize`, `ClickRow`, `renderBox` and `ViewOverlay` keep their signatures.

- [ ] **Step 1: Replace `listTopOffset`, `visibleWindow` and `renderBox`**

In `internal/ui/themeswitcher/model.go`:

Replace `const listTopOffset = 5` (and its comment) with the `listTopOffset()` function above, and in `ClickRow` change `row := localY - listTopOffset` to `row := localY - listTopOffset()`.

Replace the body of `visibleWindow`:

```go
// visibleWindow returns the [start, end) slice of m.filtered currently
// shown, using the same scroll math as the renderer.
func (m *Model) visibleWindow() (int, int) {
	return chrome.Window(m.selected, len(m.filtered), maxVisibleRows)
}
```

Replace `renderBox` entirely:

```go
func (m Model) renderBox(termWidth int) string {
	if !m.visible {
		return ""
	}
	s := messages.ChromeStyles()
	overlayWidth := boxWidth(termWidth)
	innerWidth := overlayWidth - 4

	titleText := "Switch Theme"
	if m.headerText != "" {
		titleText = m.headerText
	}

	start, end := m.visibleWindow()
	list := chrome.List{Start: start, End: end, Total: len(m.filtered)}
	width := list.ContentWidth(innerWidth)
	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		line := m.items[m.filtered[i]]
		if lipgloss.Width(line) > width {
			line = truncate.StringWithTail(line, uint(width), "\u2026")
		}
		style := s.Row
		if i == m.selected {
			style = s.RowSelected
		}
		rows = append(rows, style.Width(width).Render(line))
	}

	f := chrome.Frame{
		Title: titleText,
		Input: chrome.FilterInput(s, m.query, "Type to filter..."),
		Body:  list.Rows(s, m.selected, rows),
	}
	if m.query != "" {
		f.Empty = "No matching themes"
	}
	return f.Render(s, overlayWidth)
}
```

After the `Model` type, add:

```go
var _ interface {
	chrome.Dialog
	chrome.Clickable
} = (*Model)(nil)
```

Imports: add `"github.com/gammons/slk/internal/bubbles/chrome"`; remove `"github.com/gammons/slk/internal/ui/styles"` (now unused). Keep `strings` (used by `filter`), `lipgloss`, `truncate`, `messages`, `overlay`, `text`, `core`.

- [ ] **Step 2: Update the package tests and run them**

```bash
perl -pi -e 's/\blistTopOffset\b(?!\()/listTopOffset()/g' internal/ui/themeswitcher/*_test.go
go build ./... && go test ./internal/ui/themeswitcher -count=1
```
Expected: PASS.

- [ ] **Step 3: Run the golden check**

Run: `go test ./internal/ui -run 'TestGolden' -count=1` (and the re-bless steps in "The golden check" only if it fails).
Expected: PASS. `dialog_theme_switcher` and `dialog_theme_switcher_no_match` are the goldens this touches.

- [ ] **Step 4: Commit**

```bash
gofmt -l . ; go vet ./internal/ui/themeswitcher
git add internal/ui/themeswitcher/ internal/ui/testdata/golden/
git commit -m "refactor(themeswitcher): draw through chrome"
```

---

### Task 6: workspacefinder (and the theme-follow test)

**Files:**
- Modify: `internal/ui/workspacefinder/model.go`
- Test: `internal/ui/workspacefinder/click_test.go` (rename); create `internal/ui/workspacefinder/theme_test.go`

**Interfaces:**
- Consumes: as Task 5.
- Produces: nothing new.

- [ ] **Step 1: Write the theme-follow test**

Create `internal/ui/workspacefinder/theme_test.go`. It passes before and after the migration. Its job is to keep passing: it proves that reading `ChromeStyles()` per draw follows a theme change made while the dialog is open (Review Focus 1).

```go
package workspacefinder

import (
	"strings"
	"testing"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/styles"
)

func TestRenderBox_FollowsTheTheme(t *testing.T) {
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	styles.Apply("dark", config.Theme{})
	m := New()
	m.SetItems([]Item{{ID: "T1", Name: "Acme", Initials: "A"}})
	m.Open()
	dark := m.renderBox(80)

	styles.Apply("light", config.Theme{})
	light := m.renderBox(80)
	if dark == light {
		t.Fatal("the open finder drew the same bytes after a theme change")
	}
	if !strings.Contains(light, messages.BgANSI()) {
		t.Error("the finder drawn after the change does not carry the new theme's background")
	}
}
```

Run: `go test ./internal/ui/workspacefinder -run TestRenderBox_FollowsTheTheme -count=1`. Expected: PASS (it pins today's behaviour).

- [ ] **Step 2: Replace `listTopOffset`, `visibleWindow` and `renderBox`**

`listTopOffset` and `ClickRow` change as in Task 5. Then:

```go
func (m *Model) visibleWindow() (int, int) {
	return chrome.Window(m.selected, len(m.filtered), maxVisibleRows)
}
```

```go
func (m Model) renderBox(termWidth int) string {
	if !m.visible {
		return ""
	}
	s := messages.ChromeStyles()
	overlayWidth := boxWidth(termWidth)
	innerWidth := overlayWidth - 4 // border + padding

	start, end := m.visibleWindow()
	list := chrome.List{Start: start, End: end, Total: len(m.filtered)}
	width := list.ContentWidth(innerWidth)
	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		item := m.items[m.filtered[i]]
		line := workspacePrefix(item) + " " + item.Name
		// Truncated one column wider than the row is drawn: preserved
		// as found (Phase 4 spec §8.1).
		if lipgloss.Width(line) > width+1 {
			line = truncate.StringWithTail(line, uint(width+1), "\u2026")
		}
		style := s.Row
		if i == m.selected {
			style = s.RowSelected
		}
		rows = append(rows, style.Width(width).Render(line))
	}

	f := chrome.Frame{
		Title: "Switch Workspace",
		Input: chrome.FilterInput(s, m.query, "Type to filter..."),
		Body:  list.Rows(s, m.selected, rows),
	}
	if m.query != "" {
		f.Empty = "No matching workspaces"
	}
	return f.Render(s, overlayWidth)
}
```

Add the same `var _ interface{ chrome.Dialog; chrome.Clickable } = (*Model)(nil)` assertion as Task 5. Imports: add `chrome`. Keep `styles`, because `workspacePrefix` uses `styles.WorkspaceActive`.

- [ ] **Step 3: Update tests, run the package and the golden check**

```bash
perl -pi -e 's/\blistTopOffset\b(?!\()/listTopOffset()/g' internal/ui/workspacefinder/*_test.go
go build ./... && go test ./internal/ui/workspacefinder -count=1
go test ./internal/ui -run 'TestGolden' -count=1
```
Expected: PASS (re-bless only per "The golden check").

- [ ] **Step 4: Commit**

```bash
git add internal/ui/workspacefinder/ internal/ui/testdata/golden/
git commit -m "refactor(workspacefinder): draw through chrome"
```

---

### Task 7: presencemenu and the custom-snooze box

**Files:**
- Modify: `internal/ui/presencemenu/model.go`
- Test: `internal/ui/presencemenu/click_test.go` (rename)

**Interfaces:**
- Consumes: as Task 5.
- Produces: nothing new. `CustomSnoozeView`'s signature is unchanged.

- [ ] **Step 1: Replace `listTopOffset`, `renderBox` and `CustomSnoozeView`**

`listTopOffset` becomes the function; in `ClickRow` change `row := localY - listTopOffset` to `row := localY - listTopOffset()`. presencemenu has no window: every row is drawn.

```go
func (m Model) renderBox(termWidth int) string {
	s := messages.ChromeStyles()
	overlayWidth := boxWidth(termWidth)
	innerWidth := overlayWidth - 4

	titleText := "Status"
	if m.workspaceName != "" {
		titleText = "Status — " + m.workspaceName
	}

	list := chrome.List{Start: 0, End: len(m.filtered), Total: len(m.filtered)}
	width := list.ContentWidth(innerWidth)
	rows := make([]string, 0, len(m.filtered))
	for i, idx := range m.filtered {
		it := m.items[idx]
		line := it.label
		if it.current {
			line = "✓ " + line
		}
		if lipgloss.Width(line) > width {
			line = truncate.StringWithTail(line, uint(width), "…")
		}
		style := s.Row
		switch {
		case i == m.selected:
			style = s.RowSelected
		case it.current:
			style = s.Row.Foreground(styles.Accent)
		}
		rows = append(rows, style.Width(width).Render(line))
	}

	return chrome.Frame{
		Title: titleText,
		Input: chrome.FilterInput(s, m.query, "Type to filter…"),
		Body:  list.Rows(s, m.selected, rows),
		Empty: "No matching options",
	}.Render(s, overlayWidth)
}
```

```go
func CustomSnoozeView(termWidth, termHeight int, background, query string) string {
	s := messages.ChromeStyles()
	box := chrome.Frame{
		Title:  "Snooze for how many minutes?",
		Body:   []string{s.Input.Render(query + "█")},
		Footer: s.Empty.Render("Enter to snooze · Esc to cancel"),
	}.Render(s, boxWidth(termWidth))
	return overlay.DimmedOverlay(termWidth, termHeight, background, box, 0.5)
}
```

(`boxWidth` is `termWidth/2` clamped to [36, 60], which is exactly the width `CustomSnoozeView` computed inline. The old layout `title + "\n\n" + input + "\n\n" + hint` is a `Frame` with no input, the input line as its body and the hint as its footer.)

Add `var _ interface{ chrome.Dialog; chrome.Clickable } = (*Model)(nil)`. Imports: add `chrome`. Keep `styles` (the current row's `Accent`).

- [ ] **Step 2: Update tests, run the package and the golden check**

```bash
perl -pi -e 's/\blistTopOffset\b(?!\()/listTopOffset()/g' internal/ui/presencemenu/*_test.go
go build ./... && go test ./internal/ui/presencemenu -count=1
go test ./internal/ui -run 'TestGolden' -count=1
```
Expected: PASS. The goldens touched are `dialog_presence_menu`, `dialog_presence_menu_no_match` and `dialog_presence_snooze`.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/presencemenu/ internal/ui/testdata/golden/
git commit -m "refactor(presencemenu): draw the menu and the snooze box through chrome"
```

---

### Task 8: channelfinder

**Files:**
- Modify: `internal/ui/channelfinder/model.go:210-253` (`listTopOffset`, `visibleWindow`), `:570-753` (`renderBox`)
- Test: `internal/ui/channelfinder/click_test.go`, `forwarding_test.go` (rename)

**Interfaces:**
- Consumes: as Task 5.
- Produces: nothing new.

- [ ] **Step 1: Replace `listTopOffset`, `visibleWindow` and `renderBox`**

`listTopOffset` becomes the function; in `ClickRow` change `row := localY - listTopOffset` to `row := localY - listTopOffset()`.

```go
func (m *Model) visibleWindow() (int, int) {
	return chrome.Window(m.selected, len(m.filtered), maxVisibleRows)
}
```

The row content code is unchanged; only the frame around it moves to `chrome`:

```go
func (m Model) renderBox(termWidth int) string {
	if !m.visible {
		return ""
	}
	s := messages.ChromeStyles()
	overlayWidth := boxWidth(termWidth)
	innerWidth := overlayWidth - 4 // border + padding

	// All inner spans share the modal bg so the dimmed app behind the
	// overlay doesn't bleed through where individual styled fragments end.
	bg := styles.Background

	titleText := "Switch Channel"
	if m.forwarding {
		titleText = "Forward message to…"
	}

	start, end := m.visibleWindow()
	list := chrome.List{Start: start, End: end, Total: len(m.filtered)}
	contentWidth := list.ContentWidth(innerWidth)

	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		idx := m.filtered[i]
		item := m.items[idx]
		st := m.statuses[item.ID]

		isSelected := i == m.selected

		// Render prefix and name as SEPARATE styled fragments. If we built a
		// single string and ran one outer style over it, the prefix's own
		// ANSI reset (\x1b[0m) would drop the outer foreground / faint
		// attributes for everything after it, defeating the dim treatment.
		var prefix, name string
		if item.Joined {
			prefix = channelPrefix(item, st)
			nameStyle := lipgloss.NewStyle().Background(bg).Foreground(styles.TextPrimary)
			if isSelected {
				nameStyle = nameStyle.Background(bg).Foreground(styles.Primary).Bold(true)
			}
			// Charge the status glyph to the name's own truncation
			// budget (like the sidebar's buildCache) so a long name
			// truncates before the glyph is lost.
			label := item.Name
			glyph := st.Glyph(time.Now())
			glyphCells := 0
			if glyph != "" {
				glyphCells = 1 + st.GlyphWidth(time.Now())
			}
			nameBudget := contentWidth - lipgloss.Width(prefix) - 1 - glyphCells
			if nameBudget < 1 {
				nameBudget = 1
			}
			if lipgloss.Width(label) > nameBudget {
				label = truncate.StringWithTail(label, uint(nameBudget), "…")
			}
			if glyph != "" {
				label += " " + glyph
			}
			name = nameStyle.Render(label)
		} else {
			// Non-joined: dim grey for everything, including the prefix.
			dim := lipgloss.NewStyle().Background(bg).Foreground(nonJoinedColor)
			prefix = dim.Render("#")
			name = dim.Render(item.Name)
		}

		line := prefix + " " + name
		// Backstop for the non-joined branch; the joined branch above
		// already budgets to fit.
		if lipgloss.Width(line) > contentWidth {
			line = truncate.StringWithTail(line, uint(contentWidth), "…")
		}
		// Right-pad with spaces to fill the row.
		if pad := contentWidth - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		rows = append(rows, line)
	}

	f := chrome.Frame{
		Title: titleText,
		Input: chrome.FilterInput(s, m.query, "Type to filter..."),
		Body:  list.Rows(s, m.selected, rows),
	}
	if m.query != "" {
		f.Empty = "No matching channels"
	}
	return f.Render(s, overlayWidth)
}
```

Add `var _ interface{ chrome.Dialog; chrome.Clickable } = (*Model)(nil)`. Imports: add `chrome`. Every other import stays.

- [ ] **Step 2: Update tests, run the package and the golden check**

```bash
perl -pi -e 's/\blistTopOffset\b(?!\()/listTopOffset()/g' internal/ui/channelfinder/*_test.go
go build ./... && go test ./internal/ui/channelfinder -count=1
go test ./internal/ui -run 'TestGolden' -count=1
```
Expected: PASS. The goldens touched are `overlay_finder`, `dialog_channel_finder_scroll` and `dialog_channel_finder_no_match`.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/channelfinder/ internal/ui/testdata/golden/
git commit -m "refactor(channelfinder): draw through chrome"
```

---

### Task 9: reactionpicker

**Files:**
- Modify: `internal/ui/reactionpicker/model.go:160-200` (`listTopOffset`, `visibleWindow`), `:353-553` (`renderBox`)
- Test: `internal/ui/reactionpicker/click_test.go` (rename)

**Interfaces:**
- Consumes: as Task 5.
- Produces: nothing new.

- [ ] **Step 1: Replace `listTopOffset`, `visibleWindow` and `renderBox`**

`listTopOffset` becomes the function; in `ClickRow` change `row := localY - listTopOffset` to `row := localY - listTopOffset()`.

```go
func (m *Model) visibleWindow() (int, int) {
	return chrome.Window(m.selected, len(m.displayedList()), maxVisibleRows)
}
```

```go
func (m *Model) renderBox(termWidth int) string {
	if !m.visible {
		return ""
	}
	s := messages.ChromeStyles()
	overlayWidth := boxWidth(termWidth)
	innerWidth := overlayWidth - 4 // border + padding

	list := m.displayedList()
	start, end := m.visibleWindow()
	win := chrome.List{Start: start, End: end, Total: len(list)}
	width := win.ContentWidth(innerWidth)

	// Image-aware emoji-as-image path: active only when the
	// process-global ImageMode is on AND a fetcher has been installed
	// via SetEmojiContext. Otherwise the legacy ShouldRenderUnicode /
	// shortcode-text branch renders (byte-identical to pre-Phase-8).
	imageOK := slkemoji.ImageModeActive() && m.emojiCtx.PlaceCtx.Fetcher != nil
	cells := m.emojiCtx.Cells
	if cells <= 0 {
		cells = 2
	}
	// Unlike messages.View, the picker has no per-frame Flushes slice
	// walked by the host renderer. We collect any kitty-upload
	// callbacks Place produced into this per-View local slice and
	// fire them against imgpkg.KittyOutput just before returning.
	// Most are no-ops in steady state (the messages-pane already
	// uploaded via the shared Registry); the picker still owns the
	// fire to handle the case where it's the first/only surface to
	// reference a given emoji this session.
	var pendingFlushes []func(io.Writer) error

	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		entry := list[i]
		// Display Unicode emoji when the resolved form is composition-
		// safe (single base codepoint, optionally + VS16). Multi-
		// codepoint sequences (ZWJ, regional-indicator flags, skin
		// tones) render as broken glyphs in many terminal fonts and
		// corrupt the picker's width arithmetic. See
		// internal/emoji/shouldrender.go.
		var preview string
		if imageOK {
			if url, ok := slkemoji.URLForShortcode(entry.Name, m.emojiCtx.Customs); ok {
				if placement, flush, ok := slkemoji.Place(m.emojiCtx.PlaceCtx, url, cells); ok {
					preview = placement
					if flush != nil {
						pendingFlushes = append(pendingFlushes, flush)
					}
				}
			}
		}
		if preview == "" {
			// Legacy fallback path (image mode off, no URL, or
			// Place returned false).
			if slkemoji.ShouldRenderUnicode(entry.Unicode) {
				preview = entry.Unicode
			} else {
				preview = ":" + entry.Name + ":"
			}
		}
		line := preview + " " + entry.Name

		if m.isExistingReaction(entry.Name) {
			line += " ✓"
		}

		// Truncated one column wider than the row and clipped by
		// MaxWidth: preserved as found (Phase 4 spec §8.1).
		if lipgloss.Width(line) > width+1 {
			line = truncate.StringWithTail(line, uint(width+1), "…")
		}
		style := s.Row
		if i == m.selected {
			style = s.RowSelected
		}
		rows = append(rows, style.Width(width).MaxWidth(width).Render(line))
	}

	f := chrome.Frame{
		Title: "Add Reaction",
		Input: chrome.FilterInput(s, m.query, "Type to filter..."),
		Body:  win.Rows(s, m.selected, rows),
	}
	if m.query != "" {
		f.Empty = "No matching emoji"
	}
	box := f.Render(s, overlayWidth)

	// Fire any kitty image upload callbacks the per-row Place calls
	// produced. Most are no-ops (the messages pane already triggered
	// the upload via the shared Registry); the picker still owns the
	// fire to handle the case where it's the first/only surface to
	// reference a given emoji this session. Done here (inside
	// renderBox) so both View() and ViewOverlay() benefit without
	// duplication.
	for _, fl := range pendingFlushes {
		_ = fl(imgpkg.KittyOutput)
	}

	return box
}
```

Add `var _ interface{ chrome.Dialog; chrome.Clickable } = (*Model)(nil)`. Imports: add `chrome`; remove `styles` if it is now unused (check with `go build`).

- [ ] **Step 2: Update tests, run the package (image-mode tests included) and the golden check**

```bash
perl -pi -e 's/\blistTopOffset\b(?!\()/listTopOffset()/g' internal/ui/reactionpicker/*_test.go
go build ./... && go test ./internal/ui/reactionpicker -count=1 -v -run 'ImageMode|Kitty|Click|View'
go test ./internal/ui/reactionpicker -count=1
go test ./internal/ui -run 'TestGolden' -count=1
```
Expected: PASS, including `TestPicker_View_ImageMode_UsesPlacement` and `TestViewOverlayPreservesKittyPlacementSGR` (Review Focus 5).

- [ ] **Step 3: Commit**

```bash
git add internal/ui/reactionpicker/ internal/ui/testdata/golden/
git commit -m "refactor(reactionpicker): draw through chrome"
```

---

### Task 10: newmessagepicker

**Files:**
- Modify: `internal/ui/newmessagepicker/model.go:96-136` (`listTopOffset`, `visibleWindow`), `internal/ui/newmessagepicker/view.go:43-229` (`renderBox`, `renderResultRows`, `renderRow`)
- Test: `internal/ui/newmessagepicker/click_test.go` (rename)

**Interfaces:**
- Consumes: as Task 5.
- Produces: `renderResultRows` gains a first parameter `s chrome.Styles`, and `renderRow` no longer draws the marker. Both are unexported and no test calls them.

- [ ] **Step 1: Replace `listTopOffset` and `visibleWindow` (model.go)**

`listTopOffset` becomes the function; in `ClickRow` change `row := localY - listTopOffset` to `row := localY - listTopOffset()`.

```go
func (m *Model) visibleWindow() (int, int) {
	return chrome.Window(m.highlight, len(m.filtered), maxVisibleRows)
}
```

Add `var _ interface{ chrome.Dialog; chrome.Clickable } = (*Model)(nil)` and the `chrome` and `messages` imports to model.go.

- [ ] **Step 2: Replace `renderBox`, `renderResultRows` and the end of `renderRow` (view.go)**

```go
func (m Model) renderBox(termWidth int) string {
	if !m.visible {
		return ""
	}
	s := messages.ChromeStyles()
	overlayWidth := boxWidth(termWidth)
	innerWidth := overlayWidth - 4
	bg := styles.Background

	f := chrome.Frame{
		Title:  "New message",
		Input:  m.renderInputRow(innerWidth, bg),
		Body:   m.renderResultRows(s, innerWidth, bg),
		Footer: m.renderFooter(innerWidth, bg),
	}
	if len(m.filtered) == 0 {
		f.Empty = "No users available"
		if m.query != "" {
			f.Empty = fmt.Sprintf("No users match %q", m.query)
		}
	}
	return f.Render(s, overlayWidth)
}
```

```go
// renderResultRows produces up to 10 visible result rows with a
// scrollbar when the list overflows. Highlighted row gets a left bar
// and the Primary foreground.
func (m Model) renderResultRows(s chrome.Styles, innerWidth int, bg color.Color) []string {
	start, end := m.visibleWindow()
	list := chrome.List{Start: start, End: end, Total: len(m.filtered)}
	width := list.ContentWidth(innerWidth)
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, m.renderRow(m.users[m.filtered[i]], width, i == m.highlight, bg))
	}
	return list.Rows(s, m.highlight, lines)
}
```

In `renderRow`, update the doc comment to `// renderRow renders a single user row's content, width wide: display name, @handle, [ext] tag if external, and a trailing "✓" if the user is in the pill bar. The marker column is the list's.`. Then replace its tail (from `if highlight {` to the end) with `return line`.

Imports in view.go: add `chrome`; keep `fmt`, `color`, `strings`, `lipgloss`, `messages`, `overlay`, `styles`, `truncate`.

- [ ] **Step 3: Update tests, run the package and the golden check**

```bash
perl -pi -e 's/\blistTopOffset\b(?!\()/listTopOffset()/g' internal/ui/newmessagepicker/*_test.go
go build ./... && go test ./internal/ui/newmessagepicker -count=1
go test ./internal/ui -run 'TestGolden' -count=1
```
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/newmessagepicker/ internal/ui/testdata/golden/
git commit -m "refactor(newmessagepicker): draw through chrome"
```

---

### Task 11: help

**Files:**
- Modify: `internal/ui/help/model.go:201-240` (`listTopOffset`, `visibleWindow`), `:280-489` (`renderBox`)
- Test: `internal/ui/help/click_test.go` (rename)

**Interfaces:**
- Consumes: as Task 5.
- Produces: nothing new.

- [ ] **Step 1: Replace `listTopOffset`, `visibleWindow` and `renderBox`**

`listTopOffset` becomes the function; in `ClickRow` change `row := localY - listTopOffset` to `row := localY - listTopOffset()`.

```go
func (m *Model) visibleWindow(termHeight int) (int, int) {
	return chrome.Window(m.selected, len(m.filtered), maxVisibleRows(termHeight))
}
```

```go
func (m Model) renderBox(termWidth, termHeight int) string {
	s := messages.ChromeStyles()
	// Sizing: roughly 60% of the terminal width, capped between 40 and 80
	// columns so the modal stays readable on both small and large terminals.
	overlayWidth := termWidth * 6 / 10
	if overlayWidth < 40 {
		overlayWidth = 40
	}
	if overlayWidth > 80 {
		overlayWidth = 80
	}
	if overlayWidth > termWidth-2 {
		overlayWidth = termWidth - 2
	}
	innerWidth := overlayWidth - 4 // account for border + padding

	// Search input line: the active prompt, the filter left behind after
	// leaving search, or a hint.
	var inputLine string
	switch {
	case m.searching:
		inputLine = chrome.FilterInput(s, m.query, "Type to filter...")
	case m.query != "":
		inputLine = s.Empty.Render("filter: " + m.query)
	default:
		inputLine = s.Footer.Render("Press / to search")
	}

	start, end := m.visibleWindow(termHeight)
	list := chrome.List{Start: start, End: end, Total: len(m.filtered)}
	// rowWidth includes the marker column: key, a space and the
	// description share rowWidth-1 columns.
	rowWidth := list.ContentWidth(innerWidth) + 1

	// Reserve enough left-column width to hold the widest key label so
	// descriptions line up. Cap at half the row to keep descriptions readable.
	maxKeyWidth := 0
	for _, e := range m.entries {
		if w := lipgloss.Width(e.Key); w > maxKeyWidth {
			maxKeyWidth = w
		}
	}
	if maxKeyWidth > rowWidth/2 {
		maxKeyWidth = rowWidth / 2
	}

	keyStyle := s.Row.Foreground(styles.Accent)
	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		e := m.entries[m.filtered[i]]

		keyLabel := e.Key
		if lipgloss.Width(keyLabel) > maxKeyWidth {
			keyLabel = truncate.StringWithTail(keyLabel, uint(maxKeyWidth), "\u2026")
		}
		descBudget := rowWidth - maxKeyWidth - 2 // gap between key and desc
		if descBudget < 1 {
			descBudget = 1
		}
		desc := e.Desc
		if lipgloss.Width(desc) > descBudget {
			desc = truncate.StringWithTail(desc, uint(descBudget), "\u2026")
		}

		ks, ds := keyStyle, s.Row
		if i == m.selected {
			ks, ds = keyStyle.Bold(true), s.RowSelected
		}
		rows = append(rows, ks.Width(maxKeyWidth).Render(keyLabel)+" "+ds.Width(descBudget).Render(desc))
	}

	footer := s.Footer.Render("/ search   esc/q close")
	if m.footer != "" {
		// Attribution sits at the very bottom of the modal, centered
		// across the inner width and below the controls line.
		footer += "\n\n" + s.Footer.Width(innerWidth).Align(lipgloss.Center).Render(m.footer)
	}

	f := chrome.Frame{
		Title:  "Keybindings",
		Input:  inputLine,
		Body:   list.Rows(s, m.selected, rows),
		Footer: footer,
	}
	if m.query != "" {
		f.Empty = "No matching keybindings"
	}
	return f.Render(s, overlayWidth)
}
```

(The old code decided the scrollbar by `total > maxVisibleRows(termHeight)`. That is the same as `list.Scrolls()`, because the window is `min(maxVisibleRows, total)` rows.)

Add `var _ interface{ chrome.Dialog; chrome.Clickable } = (*Model)(nil)`. Imports: add `chrome`; keep `styles` (key colour).

- [ ] **Step 2: Update tests, run the package and the golden check**

```bash
perl -pi -e 's/\blistTopOffset\b(?!\()/listTopOffset()/g' internal/ui/help/*_test.go
go build ./... && go test ./internal/ui/help -count=1
go test ./internal/ui -run 'TestGolden' -count=1
```
Expected: PASS. The goldens touched are `dialog_help`, `dialog_help_search_no_match` and `dialog_help_narrow`.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/help/ internal/ui/testdata/golden/
git commit -m "refactor(help): draw through chrome"
```

---

### Task 12: searchresults

**Files:**
- Modify: `internal/ui/searchresults/model.go:188-302` (`listTopOffset`, `visibleRowCap`, `visibleWindow`), `:336-617` (`renderBox`, `bodyLines`, `resultRows`)
- Test: `internal/ui/searchresults/*_test.go` (rename)

**Interfaces:**
- Consumes: `chrome.Window`, `chrome.List` (its `Gutter` and `ContentWidth`), `chrome.Marker`, `chrome.Frame`, `chrome.FilterInput`, `chrome.BodyTop` (Task 3); `messages.ChromeStyles` (Task 4).
- Produces: nothing new. `bodyLines(innerWidth, termHeight)` and `resultRows(innerWidth, termHeight)` keep their signatures.

- [ ] **Step 1: Replace `listTopOffset`, `visibleRowCap`'s local name and `visibleWindow`**

`listTopOffset` becomes the function; in `ClickRow` change `line := localY - listTopOffset` to `line := localY - listTopOffset()`. In `visibleRowCap`, rename the local variable `chrome` to `frameRows` (all three uses), because it would shadow the package. Then:

```go
func (m *Model) visibleWindow(termHeight int) (int, int) {
	return chrome.Window(m.selected, len(m.items), m.visibleRowCap(termHeight))
}
```

- [ ] **Step 2: Replace `renderBox`**

```go
func (m Model) renderBox(termWidth, termHeight int) string {
	if !m.visible {
		return ""
	}
	s := messages.ChromeStyles()
	overlayWidth := boxWidth(termWidth)
	innerWidth := overlayWidth - 4 // border + padding

	input := chrome.FilterInput(s, "", "Type a query, Enter to search...")
	if m.query != "" {
		// Truncate head-side (keep the tail and the cursor visible) so a
		// long query can't wrap the input line and desync BoxSize. The
		// input style spends 2 cols (left border + padding) of innerWidth.
		inputText := m.query + "█"
		if avail := innerWidth - 2; lipgloss.Width(inputText) > avail {
			r := []rune(inputText)
			for len(r) > 0 && lipgloss.Width("…"+string(r)) > avail {
				r = r[1:]
			}
			inputText = "…" + string(r)
		}
		input = s.Input.Render(inputText)
	}

	return chrome.Frame{
		Title: "Search Workspace",
		Input: input,
		Body:  m.bodyLines(innerWidth, termHeight),
	}.Render(s, overlayWidth)
}
```

- [ ] **Step 3: Move `bodyLines` and `resultRows` onto chrome's styles, marker and gutter**

In `bodyLines`, replace `bg := styles.Background` and `muted := …` with `s := messages.ChromeStyles()`. Then use `s.Empty.Render("Searching…")` and `s.Empty.Render("No results")`. The error line keeps `lipgloss.NewStyle().Background(styles.Background).Foreground(styles.Error)`.

In `resultRows`:
- Replace everything from `total := len(m.items)` through the `trackStyle := …` line with:

```go
	s := messages.ChromeStyles()
	bg := styles.Background
	start, end := m.visibleWindow(termHeight)
	list := chrome.List{Start: start, End: end, Total: len(m.items)}
	contentWidth := list.ContentWidth(innerWidth)
```

- Change the loop header to `for i := start; i < end; i++ {`.
- Replace the per-line block, from `var row string` through `rows = append(rows, row)`, with:

```go
			// The marker spans the content lines of the selected row,
			// not its blank separator; every line of a row shares the
			// row's gutter cell, so the thumb stays row-proportional.
			rows = append(rows, chrome.Marker(s, isSelected && !separator)+line+list.Gutter(s, i))
```

- The footer becomes `rows = append(rows, s.Footer.Render(fmt.Sprintf("showing %d of %d", len(m.items), m.total)))`.

Update the doc comment of `resultRows` from "(same pattern as channelfinder/workspacefinder/themeswitcher)" to "(chrome.List's gutter)".

Add `var _ interface{ chrome.Dialog; chrome.Clickable } = (*Model)(nil)`. Imports: add `chrome`.

- [ ] **Step 4: Update tests, run the package and the golden check**

```bash
perl -pi -e 's/\blistTopOffset\b(?!\()/listTopOffset()/g' internal/ui/searchresults/*_test.go
go build ./... && go test ./internal/ui/searchresults -count=1
go test ./internal/ui -run 'TestGolden' -count=1
```
Expected: PASS. The goldens touched are `dialog_search_results`, `dialog_search_loading` and `dialog_search_no_results`.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/searchresults/ internal/ui/testdata/golden/
git commit -m "refactor(searchresults): draw through chrome"
```

---

### Task 13: linkpicker

**Files:**
- Modify: `internal/ui/linkpicker/view.go:26-109`, `internal/ui/linkpicker/model.go` (assertion)

**Interfaces:**
- Consumes: `chrome.List`, `chrome.Frame` (Task 3); `messages.ChromeStyles` (Task 4).
- Produces: nothing new.

- [ ] **Step 1: Replace `renderBox`**

```go
func (m *Model) renderBox(termWidth int) string {
	s := messages.ChromeStyles()
	overlayWidth := termWidth * 6 / 10
	if overlayWidth < 40 {
		overlayWidth = 40
	}
	if overlayWidth > 80 {
		overlayWidth = 80
	}
	if overlayWidth > termWidth-2 {
		overlayWidth = termWidth - 2
	}
	innerWidth := overlayWidth - 4 // border + padding

	list := chrome.List{Start: 0, End: len(m.items), Total: len(m.items)}
	badgeStyle := s.Row.Foreground(styles.Accent)

	rows := make([]string, 0, len(m.items))
	for i, it := range m.items {
		var parts []string
		if it.Label != "" {
			parts = append(parts, it.Label)
		}
		if it.URL != "" && it.URL != it.Label {
			parts = append(parts, it.URL)
		}
		if it.Detail != "" {
			parts = append(parts, it.Detail)
		}
		text := strings.Join(parts, "  ")
		badge := ""
		if it.InApp {
			badge = " [slk]"
		}
		budget := list.ContentWidth(innerWidth) - lipgloss.Width(badge)
		if budget < 1 {
			budget = 1
		}
		if lipgloss.Width(text) > budget {
			text = truncate.StringWithTail(text, uint(budget), "\u2026")
		}
		style := s.Row
		if i == m.selected {
			style = s.RowSelected
		}
		rows = append(rows, style.Width(budget).Render(text)+badgeStyle.Render(badge))
	}

	return chrome.Frame{
		Title:  m.title,
		Body:   list.Rows(s, m.selected, rows),
		Footer: s.Footer.Render("j/k move   enter select   esc/q close"),
	}.Render(s, overlayWidth)
}
```

In `model.go` add `var _ chrome.Dialog = (*Model)(nil)` and the `chrome` import. linkpicker has no `BoxSize`, so it is not `Boxed`. Imports in view.go: add `chrome`; keep `styles` (badge colour).

- [ ] **Step 2: Run the package and the golden check**

```bash
go build ./... && go test ./internal/ui/linkpicker -count=1
go test ./internal/ui -run 'TestGolden' -count=1
```
Expected: PASS. The goldens touched are `dialog_link_picker` and `dialog_link_picker_narrow`.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/linkpicker/ internal/ui/testdata/golden/
git commit -m "refactor(linkpicker): draw through chrome"
```

---

### Task 14: reactionsview

**Files:**
- Modify: `internal/ui/reactionsview/model.go:159-228`

**Interfaces:**
- Consumes: `chrome.Frame`, `chrome.Dialog` (Task 3); `messages.ChromeStyles` (Task 4).
- Produces: nothing new. Its persistent `offset`/`maxOff` window is unchanged (spec §3.3).

- [ ] **Step 1: Replace the title, layout and box in `renderBox`**

Replace the `title := …` block with `s := messages.ChromeStyles()` (placed first in the function, after the visibility check). Replace everything from `footer := lipgloss.NewStyle()` to the end of the function with:

```go
	return chrome.Frame{
		Title:  "Reactions",
		Body:   window,
		Footer: s.Footer.Render("\u2191/\u2193 scroll   esc close"),
	}.Render(s, overlayWidth)
```

`contentLines`, the width computation, `maxVisible`, `m.maxOff`, `m.offset` and `window` stay exactly as they are. After the `Model` type, add `var _ chrome.Dialog = (*Model)(nil)`. Imports: add `chrome`.

- [ ] **Step 2: Run the package and the golden check**

```bash
go build ./... && go test ./internal/ui/reactionsview -count=1
go test ./internal/ui -run 'TestGolden' -count=1
```
Expected: PASS (`dialog_reactions_view`).

- [ ] **Step 3: Commit**

```bash
git add internal/ui/reactionsview/ internal/ui/testdata/golden/
git commit -m "refactor(reactionsview): draw through chrome"
```

---

### Task 15: userprofile

**Files:**
- Modify: `internal/ui/userprofile/view.go:115-223` (`layout`), `internal/ui/userprofile/model.go` (assertion)

**Interfaces:**
- Consumes: `chrome.Frame`, `chrome.BodyTop`, `chrome.Dialog`, `chrome.Boxed` (Task 3); `messages.ChromeStyles` (Task 4).
- Produces: nothing new.

- [ ] **Step 1: Draw the card with `Frame` and place the icon with `BodyTop`**

In `layout`, delete the `title := lipgloss.NewStyle()…Render("Profile")` block. Replace the block from `content := title + "\n\n" + …` through the `l.iconCol = …` assignment with:

```go
	s := messages.ChromeStyles()
	s.Box = s.Box.Padding(0, 1) // the card's own padding
	box := chrome.Frame{Title: "Profile", Body: body, Footer: footer}.Render(s, boxW)

	l := boxLayout{box: box, iconRow: -1}
	if iconBodyIdx >= 0 {
		// Box rows: the frame's chrome, then the body.
		l.iconRow = chrome.BodyTop(s, false) + iconBodyIdx
		// Box columns: left border, left padding, then the row.
		l.iconCol = 2 + emailIconCol
	}
	return l
```

(`chrome.BodyTop(s, false)` is 1 border + 0 padding + title + blank = 3, the literal it replaces.) In `model.go` add `var _ interface{ chrome.Dialog; chrome.Boxed } = (*Model)(nil)` and the `chrome` import; add `chrome` to view.go's imports.

- [ ] **Step 2: Run the package and the golden check**

```bash
go build ./... && go test ./internal/ui/userprofile -count=1
go test ./internal/ui -run 'TestGolden|CopyEmail|UserProfile' -count=1
```
Expected: PASS (`user_profile`, and the copy-icon click tests).

- [ ] **Step 3: Commit**

```bash
git add internal/ui/userprofile/ internal/ui/testdata/golden/
git commit -m "refactor(userprofile): draw the card through chrome"
```

---

### Task 16: confirmprompt

**Files:**
- Modify: `internal/bubbles/confirmprompt/view.go:79-92`, `internal/bubbles/confirmprompt/confirmprompt.go` (assertion), `internal/ui/confirm.go:25-41` (`confirmPromptStyles`)

**Interfaces:**
- Consumes: `chrome.Box`, `chrome.Styles`, `chrome.Dialog` (Task 3); `messages.ChromeStyles` (Task 4).
- Produces: nothing new. `confirmprompt.Styles` keeps its exported shape (spec §3.4).

- [ ] **Step 1: Draw the prompt's box with `chrome.Box`**

In `view.go`, change the last line of `View` to:

```go
	return chrome.Box(chrome.Styles{Box: m.styles.Box, BaseANSI: m.styles.BaseANSI}, content, width)
```

Replace the `internal/bubbles/ansi` import with `internal/bubbles/chrome` (ansi is no longer used in the package; confirm with `go build`). In `confirmprompt.go` add `var _ chrome.Dialog = (*Model)(nil)` and the import.

- [ ] **Step 2: Build the App's prompt box from `ChromeStyles`**

In `internal/ui/confirm.go`, in `confirmPromptStyles`, replace the `Box: lipgloss.NewStyle()…Padding(1, 1),` expression and the `BaseANSI: messages.BgANSI() + messages.FgANSI(),` line with values from one `ChromeStyles()` call:

```go
func confirmPromptStyles() confirmprompt.Styles {
	bg := styles.Background
	cs := messages.ChromeStyles()
	return confirmprompt.Styles{
		Box:          cs.Box,
		Title:        lipgloss.NewStyle().Background(bg).Foreground(styles.Primary).Bold(true),
		Body:         lipgloss.NewStyle().Background(bg).Foreground(styles.TextPrimary),
		Footer:       lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted),
		Button:       lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted),
		ButtonActive: lipgloss.NewStyle().Background(styles.Primary).Foreground(bg).Bold(true),
		BaseANSI:     cs.BaseANSI,
	}
}
```

Keep the comment above it.

- [ ] **Step 3: Run the packages and the golden check**

```bash
go build ./... && go test ./internal/bubbles/confirmprompt ./internal/ui -run 'Confirm|ActionPrompt|TestGolden' -count=1
go test ./internal/bubbles/... -count=1
```
Expected: PASS (`dialog_confirm`, the confirm click tests, every bubbles test).

- [ ] **Step 4: Commit**

```bash
git add internal/bubbles/confirmprompt/ internal/ui/confirm.go internal/ui/testdata/golden/
git commit -m "refactor(confirmprompt): draw the box through chrome"
```

- [ ] **Step 5: PR 2 checkpoint**

Run: `go test ./... -race && go vet ./... && golangci-lint run && gofmt -l .`
Expected: all clean. Then `git branch refactor/phase4/pr2-dialogs`.

---

## PR 3 — the composer popups and close-out

### Task 17: emojipicker, channelpicker, mentionpicker (one commit each)

**Files:**
- Modify: `internal/ui/emojipicker/model.go:157-237`, `internal/ui/channelpicker/model.go:151-178`, `internal/ui/mentionpicker/model.go:194-254`

**Interfaces:**
- Consumes: `chrome.Popup`, `chrome.PopupMarker`, `chrome.Dialog` (Task 3); `messages.ChromeStyles` (Task 4).
- Produces: nothing new. Each `View(width)` keeps its signature and returns `""` when hidden or empty, as now.

- [ ] **Step 1: emojipicker**

In `View`, add `s := messages.ChromeStyles()` after the visibility check. In the row loop, delete the `indicator := "  "` line and the `indicator = …` assignment, keep the `nameStyle` lines, and build the row as:

```go
		row := chrome.PopupMarker(s, i == m.selected) + preview + "  " + nameStyle.Render(":"+e.Name+":")
```

Replace the `content := …` and `box := lipgloss.NewStyle()…Render(content)` lines with `box := chrome.Popup(s, rows, width)`. Keep the flush loop and `return box`. Add `var _ chrome.Dialog = (*Model)(nil)`. Imports: add `chrome` and `messages`; drop `fmt` and `strings` if now unused (`go build` says).

```bash
go build ./... && go test ./internal/ui/emojipicker ./internal/ui/compose -count=1
go test ./internal/ui -run 'TestGolden' -count=1
git add internal/ui/emojipicker/ internal/ui/testdata/golden/
git commit -m "refactor(emojipicker): draw the popup through chrome"
```
Expected: PASS, including `TestDropdown_View_ImageMode_UsesPlacement` (Review Focus 5) and `popup_emoji`.

- [ ] **Step 2: channelpicker**

Replace `View` with:

```go
func (m *Model) View(width int) string {
	if !m.visible || len(m.filtered) == 0 {
		return ""
	}
	s := messages.ChromeStyles()
	rows := make([]string, 0, len(m.filtered))
	for i, c := range m.filtered {
		nameStyle := lipgloss.NewStyle().Foreground(styles.TextPrimary)
		if i == m.selected {
			nameStyle = nameStyle.Bold(true)
		}
		label := fmt.Sprintf("%s %s", glyphFor(c), c.Name)
		rows = append(rows, chrome.PopupMarker(s, i == m.selected)+nameStyle.Render(label))
	}
	return chrome.Popup(s, rows, width)
}
```

Add `var _ chrome.Dialog = (*Model)(nil)` and the `chrome` and `messages` imports.

```bash
go build ./... && go test ./internal/ui/channelpicker ./internal/ui/compose -count=1
go test ./internal/ui -run 'TestGolden' -count=1
git add internal/ui/channelpicker/ internal/ui/testdata/golden/
git commit -m "refactor(channelpicker): draw the popup through chrome"
```
Expected: PASS (`popup_channel`).

- [ ] **Step 3: mentionpicker**

In `View`, add `s := messages.ChromeStyles()` after the visibility check. Delete the `indicator := "  "` line and its `if selected { indicator = … }` block. Change `row := indicator + nameStyle.Render(label)` to:

```go
		row := chrome.PopupMarker(s, selected) + nameStyle.Render(label)
```

Replace the `content := …` / `box := lipgloss.NewStyle()…` / `return box` tail with `return chrome.Popup(s, rows, width)`. Add `var _ chrome.Dialog = (*Model)(nil)` and the imports.

```bash
go build ./... && go test ./internal/ui/mentionpicker ./internal/ui/compose -count=1
go test ./internal/ui -run 'TestGolden' -count=1
git add internal/ui/mentionpicker/ internal/ui/testdata/golden/
git commit -m "refactor(mentionpicker): draw the popup through chrome"
```
Expected: PASS (`popup_mention`).

---

### Task 18: close-out — measure, document, verify

**Files:**
- Modify: `AGENTS.md`, `docs/superpowers/plans/2026-09-06-architecture-refactor.md` (Phase 4 section, Status table, Target end state), `docs/superpowers/specs/2026-10-12-phase4-modal-chrome-design.md` (status line, addenda)

**Interfaces:**
- Consumes: everything above.
- Produces: the measured exit criteria (spec §7).

- [ ] **Step 1: Measure the exit criteria**

```bash
# Box drawing left in dialog packages: expect no output.
grep -ln 'RoundedBorder()' internal/ui/{channelfinder,newmessagepicker,searchresults,reactionpicker,help,themeswitcher,presencemenu,workspacefinder,mentionpicker,emojipicker,linkpicker,channelpicker,reactionsview,userprofile}/*.go | grep -v _test
# Hand-rolled thumb math: expect only internal/bubbles/scrollbar/scrollbar.go.
grep -rln 'thumbHeight\|thumbStart' --include=*.go internal | grep -v _test
# Hand-rolled list windows: expect no output.
grep -rn 'startIdx = m\.\(selected\|highlight\) - \|start = m\.selected - ' --include=*.go internal/ui | grep -v _test
# Compile-time assertions: expect all 15 packages plus confirmprompt.
grep -rln 'chrome\.Dialog' --include=*.go internal | grep -v _test | sort
# Line counts, same 13-package cluster as the spec's §2 (5,744 at cf9742c6), plus userprofile and reactionsview.
for p in channelfinder newmessagepicker searchresults reactionpicker help themeswitcher presencemenu workspacefinder mentionpicker emojipicker linkpicker channelpicker reactionsview userprofile; do
  printf '%s ' $p; cat $(ls internal/ui/$p/*.go | grep -v _test) | wc -l
done
cat $(ls internal/bubbles/confirmprompt/*.go | grep -v _test) | wc -l
cat $(ls internal/bubbles/chrome/*.go | grep -v _test) | wc -l
```

Record every number. If a grep that should be empty is not, that dialog's migration is incomplete; go back to its task.

- [ ] **Step 2: Update `AGENTS.md`**

- In the "UI chrome" table, add these rows:
  - `| Draw a dialog: its box, title / filter / body / footer layout, filter line, list window, selected-row marker, scrollbar gutter, and the composer popups' box and marker | `bubbles/chrome` (`Frame`, `Box`, `FilterInput`, `Window`, `List`, `Marker`, `BodyTop`, `Popup`, `PopupMarker`), drawn with `messages.ChromeStyles()` called per draw. A dialog keeps its own fields, matching and row content; see the package doc |`
  - `| A scrollbar thumb's rows, for a gutter you draw yourself | `bubbles/scrollbar.Thumb` (the one formula; `Overlay` and `chrome.List` use it) |`
- Change the "Modal geometry / row hit-testing" row to name `chrome.Boxed`, `chrome.Clickable` (declared in `internal/bubbles/chrome`, asserted by each dialog package) and `pointClickable` (`internal/ui/reducer_modal_click.go`).
- In "Test helpers", add: `| A golden for a dialog or composer popup over the standard fixture | `dialogGoldens()` / `renderDialogGolden(t, d)` (`internal/ui/golden_dialogs_test.go`); add a scenario there for a new dialog |`.
- In "Known duplication", delete the `renderBox` / `visibleWindow` bullet. In the architecture block, change `internal/bubbles/`'s line "Widgets move here … confirmprompt is the first" to also name `chrome` as the substrate every dialog draws through.

- [ ] **Step 3: Update the tracking doc and the spec**

In `docs/superpowers/plans/2026-09-06-architecture-refactor.md`:
- Under Phase 4, add `> **As built:**` pointing at the spec and this plan, and a `#### Achieved` table with each spec §7 criterion and the Step 1 numbers.
- Replace the Exit sentence's "modal cluster below ~3,000 lines (from 5,295)" with "modal cluster line count measured, not gated (see the Phase 4 spec §7)".
- Status table: Phase 4 row gets the spec and plan links and **complete**.
- Target end state: `renderBox implementations` → `1 — **done (Phase 4)**`, `visibleWindow implementations` → `1 — **done (Phase 4)**`, and the modal-cluster row gets the measured number with "target retired".

In the spec, change the status line to `implemented <the close-out commit's date> (see Addenda)` and add an `## Addenda (implementation)` section listing every golden that was re-blessed (with its `bytes differ, cells identical` proof), any scenario width changed in Task 1 Step 2, and any finding beyond §8.1.

- [ ] **Step 4: Full verification**

```bash
go build ./... && go vet ./... && gofmt -l . && golangci-lint run
go test ./... -race -count=1
SLK_GOLDEN_BASELINE=/tmp/opencode/phase4-baseline/internal/ui/testdata/golden \
  go test ./internal/ui -run TestGoldenCellsMatchBaseline -count=1 -v
```
Expected: everything clean. The baseline check lists `compared` for every golden and has no `not cell-identical` line.

- [ ] **Step 5: Commit**

```bash
git add AGENTS.md docs/
git commit -m "docs(phase4): close out — AGENTS.md map, tracking doc and spec updated"
```

The branch now holds three stacked PRs: `refactor/phase4/pr1-substrate` (Tasks 1–4), `refactor/phase4/pr2-dialogs` (Tasks 5–16) and this branch (Tasks 17–18). Hand off to superpowers:finishing-a-development-branch.
