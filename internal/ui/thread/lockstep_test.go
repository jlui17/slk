// Lockstep parity between messages.Model and thread.Model.
//
// thread/model.go:35-36 asks humans to keep this package's viewEntry --
// and, transitively, ~377 lines of scroll, selection and render logic --
// in lockstep with internal/ui/messages by hand. AGENTS.md names that
// comment as the counter-example for its own convention: "two
// implementations that must stay parallel" belong in an interface
// assertion or a lockstep test, not a comment. This file is that test.
//
// Phase 3 of the architecture refactor merges the two models behind a
// shared pane package. When it does, this file is deleted. That
// lifecycle is intended: the file exists to make the merge verifiable,
// not to be maintained forever.

package thread

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/gammons/slk/internal/config"
	emojiutil "github.com/gammons/slk/internal/emoji"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/styles"
)

// Pane geometry. 80 columns is wide enough that the fixture's long line
// wraps exactly once; 20 rows is tall enough that neither pane scrolls,
// so every fixture row is on screen and the scrollbar gutter stays off.
const (
	lockstepWidth  = 80
	lockstepHeight = 20
)

// lockstepClock is the instant both panes render against: Sunday
// 2026-03-15, midday, in the process's LOCAL zone.
//
// Local, not UTC, for the same reason the goldens are local
// (internal/ui/golden_test.go:321-343). The day-divider label compares
// DateFromTS(msg.TS), which formats in the local zone
// (messages/model.go:3473), against the calendar fields of nowFunc()
// (messages/model.go:3522). A UTC anchor makes those two sides disagree
// in every zone east of UTC+11: the fixture's parent row silently moves
// onto the same calendar day as the replies, the day-transition divider
// stops rendering, and the assertion that looks for it fails. Anchoring
// the clock to the local zone moves both sides together, so the labels
// are identical in every zone on earth.
//
// Midday, not midnight, so the fixture's derived timestamps stay clear
// of a day boundary across the +-14h spread of real UTC offsets.
//
// Verified by running this package under TZ=UTC, TZ=Pacific/Auckland,
// TZ=Pacific/Kiritimati, TZ=Pacific/Chatham, TZ=America/Anchorage and
// TZ=Asia/Kathmandu.
func lockstepClock() time.Time {
	return time.Date(2026, 3, 15, 12, 0, 0, 0, time.Local)
}

// lockstepTS builds a Slack timestamp offset from lockstepClock.
//
// Derived, never a literal. The day-divider path reads
// DateFromTS(msg.TS) and never DateStr (messages/model.go:1776,
// thread/model.go:1602), so a literal epoch that disagrees with the
// clock moves every row onto the wrong calendar day while leaving the
// fixture's DateStr fields looking correct. Deriving from a local clock
// also keeps every row on a fixed local calendar day in any zone.
func lockstepTS(offset time.Duration) string {
	return fmt.Sprintf("%d.000100", lockstepClock().Add(offset).Unix())
}

// lockstepItems exercises only behaviour BOTH models claim to share:
// author/timestamp rows, message bodies, a day-transition divider, the
// unread landmark, a reaction pill, and a line long enough to wrap.
//
// Row 0 lands on the day before lockstepClock and row 1/2 on
// lockstepClock itself, so the day-transition divider fires between
// them in both panes. In the thread pane row 0 is the parent and rows
// 1..2 are the replies, which is why the transition -- rather than the
// leading divider -- is the shared property (see divergence 3).
//
// DateStr is inert in the render path and is set only because
// internal/export/markdown.go:33 writes it into exported threads.
func lockstepItems() []messages.MessageItem {
	return []messages.MessageItem{
		{
			TS: lockstepTS(-24 * time.Hour), UserID: "U1", UserName: "alice",
			Text: "first", Timestamp: "9:00 AM", DateStr: "2026-03-14",
		},
		{
			TS: lockstepTS(0), UserID: "U2", UserName: "bob",
			Text: "second", Timestamp: "9:00 AM", DateStr: "2026-03-15",
			Reactions: []messages.ReactionItem{
				{Emoji: "tada", Count: 2, UserIDs: []string{"U1", "U3"}},
			},
		},
		{
			TS: lockstepTS(time.Minute), UserID: "U3", UserName: "carol",
			Text:      "a deliberately long line that has to wrap identically in both panes or the two renderers have drifted apart",
			Timestamp: "9:01 AM", DateStr: "2026-03-15",
		},
	}
}

// lockstepGlobals pins the process-global render inputs both panes read.
// Both are mutated by other tests in this package and by production code,
// and both change every glyph on screen. Reverting means re-applying the
// dark theme and the default emoji mode -- styles.Apply has no inverse;
// re-applying dark is the convention the rest of the tree uses.
func lockstepGlobals(t *testing.T) {
	t.Helper()
	styles.Apply("dark", config.Theme{})
	emojiutil.SetImageMode(false, 2)
	messages.SetNowFunc(lockstepClock)
	t.Cleanup(func() {
		styles.Apply("dark", config.Theme{})
		emojiutil.SetImageMode(false, 2)
		messages.SetNowFunc(nil)
	})
}

// lockstepRender builds both panes from the same fixture, in the same
// state, and returns the models alongside their ANSI-stripped rows.
//
// Stripped, not raw: thread.View wraps its whole output in an outer
// Background style that messages.View does not emit (divergence 14), so
// the raw byte streams differ even where the panes are pixel-identical.
// Stripping isolates the rendered glyph grid, which is the thing that
// actually has to stay in lockstep.
//
// The two panes are put into equivalent states, not identical calls:
// each model's last row is selected (messages.New selects the newest
// message; SetThread selects the newest reply), both are focused, and
// both are given an unread boundary at the first item so the
// "── new ──" landmark renders.
func lockstepRender(t *testing.T) (mm *messages.Model, tm *Model, msgPane, thrPane []string) {
	t.Helper()
	lockstepGlobals(t)

	items := lockstepItems()

	msgModel := messages.New(items, "general")
	mm = &msgModel
	mm.SetFocused(true)
	mm.SetLastReadTS(items[0].TS)

	tm = New()
	tm.SetThread(items[0], items[1:], "C1", items[0].TS)
	tm.SetFocused(true)
	tm.SetUnreadBoundary(items[0].TS)

	msgPane = lockstepStrip(mm.View(lockstepHeight, lockstepWidth))
	thrPane = lockstepStrip(tm.View(lockstepHeight, lockstepWidth))
	return mm, tm, msgPane, thrPane
}

// lockstepStrip splits a rendered pane into ANSI-stripped rows.
func lockstepStrip(view string) []string {
	rows := strings.Split(view, "\n")
	for i, r := range rows {
		rows[i] = ansi.Strip(r)
	}
	return rows
}

// lockstepRow returns the index of the ONLY row in pane containing
// anchor, and fails the test when the count is not exactly one.
//
// The "exactly one" requirement is load-bearing. The obvious cheaper
// shape,
//
//	strings.Contains(a, x) != strings.Contains(b, x)
//
// passes when BOTH sides are false -- a pane that rendered nothing at
// all satisfies it. Resolving each anchor to a concrete row index first
// means a missing row is a failure, not a silent pass, and gives the
// comparison something to be exact about.
func lockstepRow(t *testing.T, pane []string, paneName, anchor string) int {
	t.Helper()
	idx := -1
	n := 0
	for i, row := range pane {
		if strings.Contains(row, anchor) {
			n++
			idx = i
		}
	}
	if n != 1 {
		t.Fatalf("%s: want exactly 1 row containing %q, found %d\npane:\n%s",
			paneName, anchor, n, strings.Join(pane, "\n"))
	}
	return idx
}

// TestLockstep_SharedRenderBehaviour pins the parity that
// thread/model.go:35-36 currently asks humans to maintain by hand.
//
// It asserts on the SHARED SUBSET only, in three layers:
//
//   - geometry: both panes fill exactly lockstepHeight rows of exactly
//     lockstepWidth display columns;
//   - content: every shared row is byte-identical after ANSI stripping,
//     which pins glyphs, centring, padding and the wrap column;
//   - structure: the row-index gaps WITHIN a message are equal, which
//     pins ordering (day divider above unread landmark above the
//     message) and intra-message spacing.
//
// Every legitimate difference is enumerated in the "Documented
// divergences" comment below. That list is the contract Phase 3's pane
// extraction has to honour.
func TestLockstep_SharedRenderBehaviour(t *testing.T) {
	_, _, msgPane, thrPane := lockstepRender(t)

	// --- geometry -------------------------------------------------
	for _, p := range []struct {
		name string
		rows []string
	}{{"messages", msgPane}, {"thread", thrPane}} {
		if len(p.rows) != lockstepHeight {
			t.Errorf("%s pane: got %d rows, want %d", p.name, len(p.rows), lockstepHeight)
		}
		for i, row := range p.rows {
			if w := ansi.StringWidth(row); w != lockstepWidth {
				t.Errorf("%s pane row %d: width %d, want %d (%q)", p.name, i, w, lockstepWidth, row)
			}
		}
	}

	// --- content --------------------------------------------------
	//
	// Anchors are chosen to be unique within a pane; lockstepRow
	// enforces that. The assertion is full-row equality, so a drift in
	// centring, padding, glyph choice or wrap column fails even though
	// the anchor still resolves.
	shared := []struct{ name, anchor string }{
		{"body row (first)", "▌first"},
		{"day-transition divider", "── Today ──"},
		{"unread landmark", "── new ──"},
		{"author row (bob)", "bob  9:00 AM"},
		{"body row (second)", "▌second"},
		{"reaction pill", "🎉 2"},
		{"author row (carol)", "carol  Sun Mar 15, 9:01 AM"},
		{"wrapped line 1", "in both panes or the"},
		{"wrapped line 2", "two renderers have drifted apart"},
	}
	// Resolved once, here, and reused by the structure layer below:
	// every gap anchor is also a shared anchor, so re-resolving would
	// re-scan both panes for a row index already in hand.
	msgAt := make(map[string]int, len(shared))
	thrAt := make(map[string]int, len(shared))
	for _, s := range shared {
		mi := lockstepRow(t, msgPane, "messages pane/"+s.name, s.anchor)
		ti := lockstepRow(t, thrPane, "thread pane/"+s.name, s.anchor)
		msgAt[s.anchor], thrAt[s.anchor] = mi, ti
		if msgPane[mi] != thrPane[ti] {
			t.Errorf("%s differs between panes:\n messages[%d] = %q\n thread[%d]   = %q",
				s.name, mi, msgPane[mi], ti, thrPane[ti])
		}
	}

	// The parent's author row carries its date in thread only
	// (divergence 3), so it is checked per pane rather than shared.
	lockstepRow(t, msgPane, "messages pane/author row (alice)", "alice  9:00 AM")
	lockstepRow(t, thrPane, "thread pane/author row (alice)", "alice  Yesterday, 9:00 AM")

	// --- structure ------------------------------------------------
	//
	// Only gaps WITHIN a message (or between a divider and the message
	// it introduces) are compared. Gaps BETWEEN messages legitimately
	// differ: messages separates with a blank row, thread with a
	// full-width rule (divergence 4).
	gaps := []struct{ name, from, to string }{
		{"day divider -> unread landmark", "── Today ──", "── new ──"},
		{"unread landmark -> author (bob)", "── new ──", "bob  9:00 AM"},
		{"author -> body (bob)", "bob  9:00 AM", "▌second"},
		{"body -> reaction pill (bob)", "▌second", "🎉 2"},
		{"author -> wrapped line 1 (carol)", "carol  Sun Mar 15, 9:01 AM", "in both panes or the"},
		{"wrapped line 1 -> line 2", "in both panes or the", "two renderers have drifted apart"},
	}
	// gapIn reads the two endpoints out of an already-resolved pane
	// index. A missing key means a gap names an anchor the content
	// layer never resolved, which is a bug in the table above, not a
	// parity failure -- so it is fatal and names the gap.
	gapIn := func(at map[string]int, paneName, name, from, to string) int {
		t.Helper()
		f, okFrom := at[from]
		to2, okTo := at[to]
		if !okFrom || !okTo {
			t.Fatalf("%s: row gap %q names anchor(s) missing from the shared table "+
				"(from=%q resolved=%v, to=%q resolved=%v); add them to `shared` so "+
				"they are resolved exactly once", paneName, name, from, okFrom, to, okTo)
		}
		return to2 - f
	}
	for _, g := range gaps {
		msgGap := gapIn(msgAt, "messages pane", g.name, g.from, g.to)
		thrGap := gapIn(thrAt, "thread pane", g.name, g.from, g.to)
		if msgGap != thrGap {
			t.Errorf("row gap %q: messages=%d thread=%d", g.name, msgGap, thrGap)
		}
	}
}

// Documented divergences between messages.Model and thread.Model.
//
// This list is the specification for the divergence hooks Phase 3's
// shared pane package must provide. Everything NOT listed here is
// expected to render identically and is asserted above.
//
// Adding an entry is a design decision: it declares "this difference is
// intended". If behaviour drifts and you cannot justify it as intended,
// the fix belongs in the model, not in this list.
//
// Every citation below was opened and read, not grepped. Where an item
// gives a rendered string it gives the SHAPE, not one observed instance:
// a hook designer reading a literal as a template ships the literal.
//
//  1. Construction. messages.New(msgs, channelName) returns a VALUE
//     Model preloaded with content (messages/model.go:597); thread.New()
//     returns a *Model that is empty until SetThread
//     (thread/model.go:226). Phase 3 must pick one shape; the value
//     return is why callers of messages.New must take an address before
//     calling any method.
//
//  2. Parent row. thread renders m.parent as a pseudo-row above the
//     replies, addressable through the parentSelected = -1 sentinel
//     (thread/model.go:94, 1333-1361); messages has no such row. The
//     consequence is an index-base divergence: for the same on-screen
//     pill, HitTestReaction returns a REPLY index in thread and a
//     MESSAGE index in messages. (Pinned by the convergence tripwire
//     deleted in Phase 3 PR 5, so no longer asserted.)
//
//  3. Leading day divider. messages seeds lastDate empty
//     (messages/model.go:1773), so the FIRST message gets a day divider
//     above it whenever its TS parses (DateFromTS returns "" otherwise,
//     and the empty date is skipped). thread seeds lastDate from
//     DateFromTS(parent.TS) (thread/model.go:1559), so no divider is
//     ever drawn above the parent, and none above a first reply that
//     shares the parent's day -- unless the parent's TS is itself
//     unparseable, in which case lastDate seeds "" and the first reply
//     with a parseable date DOES get a divider
//     (thread/model.go:1601-1608), degenerating to the messages
//     behaviour. Only the day TRANSITION is shared, which is what the
//     fixture above exercises. To compensate, thread prefixes the
//     parent's header time with messages.FormatShortDate
//     ("Yesterday, 9:00 AM"), so the parent's author row is not shared.
//
//  4. Inter-row separator, and where it lives structurally. messages
//     separates messages with a blank full-width spacer row
//     (m.cacheSpacer, messages/model.go:1528, reached as
//     cs.spacerLines) that is appended INSIDE the message's own
//     viewEntry for every message but the last
//     (messages/model.go:1683-1687): the spacer line goes onto
//     linesNormal/linesSelected and a plainLine{Text: ""} mirror onto
//     linesPlain, so it counts toward that entry's height and carries
//     that entry's msgIdx (messages/model.go:1688-1699). thread instead
//     draws a full-width "─" rule as a STANDALONE row outside any cache
//     entry -- after the parent (built thread/model.go:1355-1359, then
//     joined into parentBlock at 1360, after parentEntry's height was
//     taken from parentContent alone at 1337) and between replies
//     (built 1518-1522, appended to allRows at 1655-1658, whose comment
//     states the rule is deliberately not inside any entry so selection
//     overlay and extraction skip it). This is the same entry-vs-bare-row
//     distinction item 8(c) draws for the unread landmark, and it falls
//     the same way round: separator inside an entry in messages, outside
//     every entry in thread. It is also why only intra-message row gaps
//     are compared above.
//
//  5. Pane chrome. messages renders
//     fmt.Sprintf("%s %s", messages.ChannelGlyph(channelType), channelName)
//     under Padding(0, 1), plus an optional wrapped topic. The glyph is
//     TYPE-dependent, not a constant "#": messages.ChannelGlyph returns
//     "◆" for "private", "●" for "dm"/"group_dm" and "#" otherwise, so a
//     chrome hook must take the channel type, not a pre-formatted title.
//     Height is 1 row without a topic (the fixture's case) and 1+wrapped
//     topic height with one; ChromeHeight() is exported.
//     thread renders a breadcrumb, renderBreadcrumb in
//     thread/breadcrumb.go: "<glyph> <channel> › Thread from <author>
//     · N replies" plus a right-aligned "esc close", fed by
//     SetBreadcrumb(channelName, channelType) -- the TYPE, not a
//     pre-formatted title, as the note above asks of a chrome hook. The
//     label is pluralised, "reply" at n==1. Then a "-" rule. Height is always
//     2 rows and chromeHeight stays unexported.
//
//  6. Reaction hit-test row frame. CONVERGED in Phase 3 PR 5: both take a
//     content-relative row; the App subtracts ChromeHeight() for both.
//
//  7. Scroll mechanism. CONVERGED in Phase 3 PR 2+3/5: both scroll through
//     internal/bubbles/entrylist (thread keeps viewport's eager clamp, U6).
//
//
//  8. Unread landmark -- API and structure, NOT rendering. The
//     rendering is shared: both panes emit a centred bold "── new ──"
//     row that is byte-identical (messages/model.go:1788-1793,
//     thread/model.go:1532-1540), and the assertions above pin that.
//     Only the non-rendering half is a divergence. What diverges is
//     (a) the setter -- messages.SetLastReadTS(ts) vs
//     thread.SetUnreadBoundary(ts); (b) lifecycle -- thread clears its
//     boundary when the thread identity changes
//     (thread/model.go:310-312), messages never self-clears; and
//     (c) structure -- messages inserts the landmark as a viewEntry with
//     msgIdx -1 (messages/model.go:1762-1771), so it participates in
//     selection anchor resolution, while thread appends a bare row
//     outside any entry (thread/model.go:1616).
//
//  9. Reply-count affordance. messages renders
//     fmt.Sprintf("[%d %s ->]", n, word) when msg.ReplyCount > 0 --
//     the word is pluralised, "[1 reply ->]" at n==1
//     (messages/model.go:1982-1989) -- and exposes
//     IncrementReplyCount; thread renders no such affordance -- it IS
//     the thread. thread.ReplyCount() is unrelated, and it is NOT the
//     header's source: it is len(m.replies)
//     (thread/model.go:498-501), exported but with no production
//     caller -- every call site is a test (thread/model_test.go:41, 42,
//     68, 69, 89, 95; internal/ui/app_test.go:1221, 1247, 1267, 1292,
//     1293, 1304, 1316). The chrome header computes len(m.replies)
//     itself into a local (thread/model.go:1280) and renders from that
//     (thread/model.go:1301), so wiring ReplyCount() into a chrome hook
//     would be introducing a caller, not preserving one.
//
// 10. Search-term highlighting. messages has SetSearchTerms and calls
//     HighlightSearchTerms in the body render path
//     (messages/model.go:1968-1974); thread has neither.
//
// 11. Loading state. messages has SetLoading / IsLoading /
//     SetSpinnerFrame and renders a braille spinner with
//     "Loading messages..." (messages/model.go:2892-2894); thread has
//     none of them.
//
// 12. Empty state. BOTH panes have one; the divergence is threefold,
//     and none of the three parts is "messages lacks an empty state".
//     (a) Arity. thread has TWO empty states: "No thread selected" when
//     IsEmpty, i.e. no thread is open (thread/model.go:1253-1260), and
//     "No replies yet" when a thread is open but has no replies
//     (thread/model.go:1363-1373). messages has ONE: "No messages yet"
//     when len(m.messages) == 0 (messages/model.go:2890-2902). A hook
//     with a single empty-state slot cannot express thread's pair.
//     (b) Chrome retention. messages' empty branch returns
//     chrome + "\n" + empty, so the header still renders and the empty
//     block is sized height-chromeHeight (messages/model.go:2896-2902).
//     thread's "No replies yet" branch does the same
//     (thread/model.go:1389), but its IsEmpty branch returns the bare
//     block at full height with NO chrome at all
//     (thread/model.go:1254-1259). The two sides therefore disagree
//     about whether chrome renders in the empty case, and the
//     disagreement is internal to thread as well.
//     (c) Loading fusion. messages' empty state doubles as its loading
//     state -- same branch, same block, the text swapped for
//     spinner + "Loading messages..." when m.loading
//     (messages/model.go:2891-2895). thread has no loading concept at
//     all (item 11), so a merged pane cannot model "empty" and
//     "loading" as one slot without inventing a loading state for
//     thread, nor as two without splitting messages' branch.
//
// 13. Selection entry points. messages splits View / ViewBare and
//     offers ApplySelectionToBordered so the App layer can cache a
//     selection-free bordered render and repaint the selection as a
//     cheap post-pass (messages/model.go:3315-3332, 3457); thread has
//     only View, which always paints the selection. ClickAt also
//     diverges in signature: messages.ClickAt(y) returns bool,
//     thread.ClickAt(y) returns nothing.
//
// 14. Outer background wrapper. REMOVED in Phase 3 PR 2+3.
//
// 15. Inline-image click-to-preview. messages has HitTest
//     (messages/model.go:2501) and HandleImageReady
//     (messages/model.go:461) for opening an image preview from a
//     message; thread deliberately discards imgrender's per-block hits
//     (thread/model.go:1788-1790) and has no HitTest. Scoped out in
//     thread/model.go:202-206.

// TestLockstep_ScrolledWindowParity compares the panes while scrolled:
// the window rows, and the text a drag copies, must agree on every body
// row. Header rows legitimately differ (the channel pane's cursor follows
// the scroll, U1, and the selected header shows a long timestamp), as do
// the rows between messages (spacer vs rule) and the channel pane's last
// row (more-below, U3), so only body rows are compared.
func TestLockstep_ScrolledWindowParity(t *testing.T) {
	lockstepGlobals(t)
	const h = 10
	items := make([]messages.MessageItem, 14)
	for i := range items {
		items[i] = messages.MessageItem{
			TS: lockstepTS(time.Duration(i) * time.Minute), UserID: fmt.Sprintf("U%02d", i),
			UserName: fmt.Sprintf("u%02d", i), Text: fmt.Sprintf("body-%02d", i),
			Timestamp: fmt.Sprintf("9:%02d AM", i),
		}
	}
	mv := messages.New(items, "general")
	mm := &mv
	mm.SetFocused(true)
	tm := New()
	tm.SetThread(items[0], items[1:], "C1", items[0].TS)
	tm.SetFocused(true)

	align := func(name string, view func() string, chrome func() int, up func(int)) []string {
		for range 200 {
			rows := lockstepStrip(view())
			if c := rows[chrome():]; strings.Contains(c[0], "body-06") {
				return c[:len(c)-1] // drop the last row: more-below in messages
			}
			up(1)
		}
		t.Fatalf("%s: body-06 never reached the top row", name)
		return nil
	}
	msgRows := align("messages", func() string { return mm.View(h, lockstepWidth) }, mm.ChromeHeight, mm.ScrollUp)
	thrRows := align("thread", func() string { return tm.View(h, lockstepWidth) }, func() int { return tm.chromeHeight }, tm.ScrollUp)

	bodies := func(rows []string) []string {
		var out []string
		for i, r := range rows {
			if strings.Contains(r, "body-") {
				out = append(out, fmt.Sprintf("%d:%s", i, r))
			}
		}
		return out
	}
	if mb, tb := bodies(msgRows), bodies(thrRows); !slices.Equal(mb, tb) {
		t.Errorf("scrolled body rows differ:\n messages %q\n thread   %q", mb, tb)
	}

	drag := func(begin, extend func(int, int), end func() (string, bool), chrome int) string {
		begin(chrome+0, 2)
		extend(chrome+3, 9)
		text, _ := end()
		var kept []string
		for _, l := range strings.Split(text, "\n") {
			if l != "" {
				kept = append(kept, l)
			}
		}
		return strings.Join(kept, "\n")
	}
	mt := drag(mm.BeginSelectionAt, mm.ExtendSelectionAt, mm.EndSelection, mm.ChromeHeight())
	tt := drag(tm.BeginSelectionAt, tm.ExtendSelectionAt, tm.EndSelection, tm.chromeHeight)
	if mt == "" || mt != tt {
		t.Errorf("scrolled drag copies differ:\n messages %q\n thread   %q", mt, tt)
	}
}
