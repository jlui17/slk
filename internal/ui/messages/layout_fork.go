package messages

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/styles"
)

// The reading layout both content panes share: a gutter left of the
// text, a cap on the text's width, and messages grouped by author as
// Slack groups them. docs/fork.md, "Reading layout", says why.

// GutterCols is the width of the column between an entry's selection
// border and its text. In the channel pane it holds the avatar (4
// cells, blank with show_avatars off) and two spaces; in the thread
// pane (RenderOptions.TimeMargin), a time right-aligned in 5 and a
// space. A continuation shows its time there in either pane.
const GutterCols = 6

// TextCol is the entry column message text starts at: the selection
// border, then the gutter.
const TextCol = 1 + GutterCols

// GroupGap is how soon after a message the same author's next one
// continues it. Slack does not document its own gap; five minutes is
// an assumption.
const GroupGap = 5 * time.Minute

// messageWidth is the most cells a message's text wraps at; 0 is no
// cap. Set once at startup from the message_width setting, before
// anything renders (the same contract as nowFunc).
var messageWidth = 120

// SetMessageWidth sets the cap TextWidth applies; n <= 0 turns it off.
func SetMessageWidth(n int) { messageWidth = n }

// TextWidth is the width message text wraps at in an entry width cells
// wide: what the pane leaves right of the gutter, capped at the
// message_width setting. The new-messages line, the thread's replies
// line and its reply box share it; a code block starts at it
// (CodeWidth).
func TextWidth(width int) int {
	w := width - 4 - GutterCols
	if messageWidth > 0 {
		w = min(w, messageWidth)
	}
	return max(w, 20)
}

// CodeWidth is the widest a code block grows in an entry width cells
// wide: all the pane leaves right of the gutter, and never less than
// the text width. A block is the text width wide unless its longest
// line needs more (codeBlockWidth); code wraps only past this.
func CodeWidth(width int) int {
	return max(width-4-GutterCols, TextWidth(width))
}

// codeBlockWidth is the width of the block holding code: opts.Width,
// grown up to opts.CodeWidth to fit its longest line and padding.
func codeBlockWidth(code string, opts RenderSlackMarkdownOpts) int {
	if opts.CodeWidth <= opts.Width {
		return opts.Width
	}
	longest := 0
	for _, line := range strings.Split(code, "\n") {
		longest = max(longest, ansi.StringWidth(line))
	}
	return min(max(opts.Width, longest+codeBlockStyle().GetHorizontalFrameSize()), opts.CodeWidth)
}

// wrapProse wraps the rendered body at opts.Width while its code blocks
// are still held out, when a block may be wider than that: the caller's
// own wrap then runs at opts.CodeWidth, where prose already fits and a
// wide block is left whole.
func wrapProse(s string, opts RenderSlackMarkdownOpts) string {
	if opts.Width <= 0 || opts.CodeWidth <= opts.Width {
		return s
	}
	return WordWrap(s, opts.Width)
}

// Continues reports whether msg continues prev's group: the same
// author, the same local day and within GroupGap. A message with a
// label row (ephemeral, a broadcast reply) or a /me line, which reads
// as the author's name and the action, always starts a group. The
// caller also starts one after a divider between the two.
func Continues(prev, msg MessageItem) bool {
	if SpeakerChanges(prev, msg) || prev.Ephemeral || msg.Ephemeral ||
		msg.Subtype == "thread_broadcast" || msg.Subtype == "me_message" {
		return false
	}
	if DateFromTS(prev.TS) != DateFromTS(msg.TS) {
		return false
	}
	pt, ok1 := timeFromTS(prev.TS)
	mt, ok2 := timeFromTS(msg.TS)
	if !ok1 || !ok2 {
		return false
	}
	d := mt.Sub(pt)
	return d >= 0 && d < GroupGap
}

// SpeakerChanges reports whether msg is by someone other than prev: a
// bot posting under another name counts as another speaker, as Slack
// shows it.
func SpeakerChanges(prev, msg MessageItem) bool {
	return prev.UserID != msg.UserID || prev.UserName != msg.UserName
}

// SpacerRows is how many blank rows sit between two messages: one
// when the same person speaks again, two when the speaker changes.
func SpacerRows(prev, msg MessageItem) int {
	if SpeakerChanges(prev, msg) {
		return 2
	}
	return 1
}

// MarginTime is msg's time as the gutter shows it: the display time
// without AM or PM ("3:04"), like Slack's hover time. A configured
// format too long for the gutter falls back to the hour and minute on
// the same clock: "15:04" for a 24-hour format, else "3:04".
func MarginTime(msg MessageItem) string {
	s := strings.TrimSpace(msg.Timestamp)
	for _, suffix := range []string{"AM", "PM", "am", "pm"} {
		s = strings.TrimSpace(strings.TrimSuffix(s, suffix))
	}
	if s != "" && ansi.StringWidth(s) <= GutterCols-1 {
		return s
	}
	t, ok := timeFromTS(msg.TS)
	if !ok {
		return ""
	}
	if is24Hour(TimestampFormat()) {
		return t.Format("15:04")
	}
	return t.Format("3:04")
}

// is24Hour reports whether a Go time layout writes a 24-hour clock: it
// has the "15" hour and no AM/PM.
func is24Hour(layout string) bool {
	return strings.Contains(layout, "15") && !strings.Contains(layout, "PM") && !strings.Contains(layout, "pm")
}

// headerLines renders a group's first row: the author, their status
// glyph, and in a pane with avatars the time. In a pane with the time
// margin the time is in the gutter, so the row carries opt.HeaderDate
// (the thread parent's date) when it fits in budget cells; selected is
// that row as the cursor draws it, with the long date and time. A
// continuation has no header row.
func headerLines(msg MessageItem, rc *RenderContext, opt RenderOptions, budget int) (line, selected string) {
	if opt.Continuation {
		return "", ""
	}
	author := renderAuthor(msg, rc) + AuthorStatusSuffix(rc.UserStatuses, msg.UserID, time.Now())
	gap := lipgloss.NewStyle().Background(styles.Background).Render("  ")
	if !opt.TimeMargin {
		return author + gap + styles.Timestamp.Render(msg.Timestamp), ""
	}
	line = author
	if dated := author + gap + styles.Timestamp.Render(opt.HeaderDate); opt.HeaderDate != "" && lipgloss.Width(dated) <= budget {
		line = dated
	}
	if long := LongTimestamp(msg.TS, msg.Timestamp); long != "" {
		selected = author + gap + styles.Timestamp.Render(long)
	}
	return line, selected
}

// renderAuthor is the author's name in the Name role. A bracketed
// suffix, as a bot posting under a task name has ("Claude [review
// queue colours]"), keeps its text and drops to TextMuted, so the name
// reads first.
func renderAuthor(msg MessageItem, rc *RenderContext) string {
	name, tag := splitNameTag(msg.UserName)
	out := styles.Username(msg.UserID, rc.ColoredUsernames).Render(name)
	if tag != "" {
		out += lipgloss.NewStyle().Background(styles.Background).Foreground(styles.TextMuted).Render(tag)
	}
	return out
}

func splitNameTag(name string) (string, string) {
	if !strings.HasSuffix(name, "]") {
		return name, ""
	}
	if i := strings.LastIndex(name, " ["); i > 0 {
		return name[:i], name[i:]
	}
	return name, ""
}

// groupHeader settles the header row of a continuation, which has
// none: its "(edited)" moves to the end of its body text
// (editedAfterBody), or, with no body text, is the row by itself.
func groupHeader(line, editedMark string, hasBody bool, opt RenderOptions) (string, string) {
	if !opt.Continuation {
		return line, editedMark
	}
	if hasBody || editedMark == "" {
		return "", ""
	}
	return "", strings.TrimPrefix(editedMark, " ")
}

// headerRows is how many rows the header takes: none for a
// continuation that shows no "(edited)" of its own.
func headerRows(line, editedMark string) int {
	if line+editedMark == "" {
		return 0
	}
	return 1
}

// editedAfterBody appends a continuation's "(edited)" to its wrapped
// body text, on the last row when it fits and on a row of its own when
// not, as Slack draws it after the text.
func editedAfterBody(wrapped string, msg MessageItem, opt RenderOptions, width int) string {
	if !opt.Continuation || !opt.EditedMark || !msg.IsEdited || wrapped == "" {
		return wrapped
	}
	mark := styles.Timestamp.Render("(edited)")
	last := wrapped[strings.LastIndexByte(wrapped, '\n')+1:]
	if ansi.StringWidth(last)+1+lipgloss.Width(mark) <= width {
		return wrapped + " " + mark
	}
	return wrapped + "\n" + mark
}

// bodyText is the body's rows: wrapped (at codeWidth, where prose
// already fits at width: wrapProse), a continuation's "(edited)" after
// them, styled. For a continuation it records the width of the first
// row's text, which SelectedContent writes after.
func bodyText(msg MessageItem, rendered string, opt RenderOptions, width, codeWidth int, out *RenderedMessage) string {
	wrapped := editedAfterBody(WordWrap(rendered, codeWidth), msg, opt, width)
	if opt.Continuation {
		first, _, _ := strings.Cut(wrapped, "\n")
		out.firstRowWidth = ansi.StringWidth(first)
	}
	return renderBodyText(msg.Subtype == "me_message", wrapped)
}

// joinHeader puts the header row above the rest of the message's rows
// (each part of rest starts with its own newline), or leaves it out.
func joinHeader(header, rest string) string {
	if header == "" {
		return strings.TrimPrefix(rest, "\n")
	}
	return header + rest
}

// placeGutter puts the gutter left of every row of content. timeRow
// is the row the time goes on: the header row, or a continuation's
// first row.
func placeGutter(content string, msg MessageItem, opt RenderOptions, timeRow int) string {
	g := gutterCellsFor()
	rows := strings.Split(content, "\n")
	if !opt.TimeMargin && !opt.Continuation {
		avatar := opt.Avatar
		if avatar == "" {
			avatar = g.noAvatar // not loaded yet
		}
		lines := strings.Split(avatar, "\n")
		for len(rows) < len(lines) {
			rows = append(rows, "")
		}
		for i := range rows {
			if i < len(lines) {
				rows[i] = lines[i] + g.afterAvatar + rows[i]
			} else {
				rows[i] = g.blank + rows[i]
			}
		}
		return strings.Join(rows, "\n")
	}
	t := MarginTime(msg)
	stamp := g.timeStyle.Render(strings.Repeat(" ", max(GutterCols-1-ansi.StringWidth(t), 0))+t) + g.space
	for i := range rows {
		if i == timeRow {
			rows[i] = stamp + rows[i]
		} else {
			rows[i] = g.blank + rows[i]
		}
	}
	return strings.Join(rows, "\n")
}

// gutterCells are the gutter's rendered pieces for one theme version,
// built once rather than for every row of every message.
type gutterCells struct {
	version                             int64
	blank, afterAvatar, noAvatar, space string
	timeStyle                           lipgloss.Style
}

var gutterCache struct {
	sync.Mutex
	cells *gutterCells
}

func gutterCellsFor() *gutterCells {
	v := styles.Version()
	gutterCache.Lock()
	defer gutterCache.Unlock()
	if c := gutterCache.cells; c != nil && c.version == v {
		return c
	}
	bg := lipgloss.NewStyle().Background(styles.Background)
	c := &gutterCells{
		version:     v,
		blank:       bg.Render(strings.Repeat(" ", GutterCols)),
		afterAvatar: bg.Render(strings.Repeat(" ", GutterCols-4)),
		noAvatar:    bg.Render(strings.Repeat(" ", 4)),
		space:       bg.Render(" "),
		timeStyle:   bg.Foreground(styles.TimeColor),
	}
	gutterCache.cells = c
	return c
}

// SelectedContent is r's content as the cursor draws it: with the long
// date and time, as Slack shows any message's on hover. A pane with
// avatars swaps the header's time for the long one (SelectedHeader);
// one with the time margin shows the long form after the name; a
// continuation, which has no header, after its first body row, as
// "(edited)" sits. Unchanged when it would not fit.
func SelectedContent(msg MessageItem, r RenderedMessage) string {
	if r.Header == "" && r.firstRowWidth > 0 {
		return continuationSelected(msg, r)
	}
	if r.HeaderSelected == "" {
		return SelectedHeader(r.Content, r.Header, msg.TS, msg.Timestamp, r.HeaderBudget)
	}
	if lipgloss.Width(r.HeaderSelected) > r.HeaderBudget {
		return r.Content
	}
	return strings.Replace(r.Content, r.Header, r.HeaderSelected, 1)
}

func continuationSelected(msg MessageItem, r RenderedMessage) string {
	long := LongTimestamp(msg.TS, msg.Timestamp)
	stamp := styles.Timestamp.Render(long)
	if long == "" || r.firstRowWidth+2+lipgloss.Width(stamp) > r.HeaderBudget {
		return r.Content
	}
	first, rest, more := strings.Cut(r.Content, "\n")
	gap := lipgloss.NewStyle().Background(styles.Background).Render("  ")
	first = ansi.Truncate(first, GutterCols+r.firstRowWidth, "") + gap + stamp
	if !more {
		return first
	}
	return first + "\n" + rest
}

// blankRow is a full-width empty row in the pane background.
func blankRow(width int) string {
	return lipgloss.NewStyle().Background(styles.Background).Width(width).Render("")
}

// NewMessagesLine is the unread boundary as Slack draws it: a rule
// across the text width ending in " new", in the NewMessages role,
// starting at the text column of an entry width cells wide.
func NewMessagesLine(width int) string {
	tw := TextWidth(width)
	st := lipgloss.NewStyle().Background(styles.Background).Foreground(styles.NewMessages)
	line := st.Render(strings.Repeat("─", max(tw-4, 1))) + st.Bold(true).Render(" new")
	return padRow(line, width)
}

// RepliesLine is the row under a thread's parent: "N replies " in
// TextMuted, then a rule in Border out to the text width.
func RepliesLine(width, count int) string {
	label := "replies"
	if count == 1 {
		label = "reply"
	}
	text := fmt.Sprintf("%d %s ", count, label)
	bg := lipgloss.NewStyle().Background(styles.Background)
	line := bg.Foreground(styles.TextMuted).Render(text) +
		bg.Foreground(styles.Border).Render(strings.Repeat("─", max(TextWidth(width)-ansi.StringWidth(text), 1)))
	return padRow(line, width)
}

// padRow puts line at the text column of a row width cells wide.
func padRow(line string, width int) string {
	bg := lipgloss.NewStyle().Background(styles.Background)
	row := bg.Render(strings.Repeat(" ", TextCol)) + line
	if w := ansi.StringWidth(row); w > width {
		return ansi.Truncate(row, width, "")
	} else if w < width {
		row += bg.Render(strings.Repeat(" ", width-w))
	}
	return row
}
