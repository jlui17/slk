package blockkit

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/slackurl"
)

const (
	// A card folds only when the fold hides at least as many lines as
	// the preview shows.
	foldMinBodyLines = 6
	foldPreviewLines = 3

	// "│ " on the left, " │" on the right. The left side is as wide as
	// the stripe upstream draws, so the columns of its image hits hold.
	cardFrameCols = stripeCol
)

// CardContext is what the host knows and a card needs.
type CardContext struct {
	Expanded     bool              // the message's cards show in full
	ChannelNames map[string]string // channel ID -> name, for a linked message's header
	Now          time.Time         // a linked message's time reads against it
	TimeFormat   string            // the host's clock format; "" means "3:04 PM"
}

// LinkedMessage reports the Slack message a unfurls.
func LinkedMessage(a LegacyAttachment) (slackurl.Permalink, bool) {
	if a.AuthorName == "" {
		return slackurl.Permalink{}, false
	}
	return slackurl.Parse(a.FromURL)
}

// ChannelLabel is "#name", or "" for a channel the host cannot name.
func ChannelLabel(link slackurl.Permalink, channelNames map[string]string) string {
	if name := channelNames[string(link.ChannelID)]; name != "" {
		return "#" + name
	}
	return ""
}

// appendCard draws one attachment as a box: pretext above it, then the
// linked message's header or the title, the body, and the fold row.
// Upstream's appendLegacyAttachment renders the rows; its stripe comes
// off and the frame goes on.
func appendCard(out *RenderResult, a LegacyAttachment, ctx Context, width int) {
	if a.Pretext != "" {
		out.Lines = append(out.Lines, renderTextLines(a.Pretext, ctx, width)...)
		a.Pretext = ""
	}
	innerW := max(width-2*cardFrameCols, 1)

	var head []string
	if link, ok := LinkedMessage(a); ok {
		head = append(head, linkedMessageHeader(a, link, ctx.Card, innerW))
		// The header says what Slack's footer and ts say.
		a.Footer, a.TS = "", 0
	}
	if blocksCarryAttachmentBody(a.Blocks) {
		a.Text = ""
	}

	var inner RenderResult
	appendLegacyAttachment(&inner, a, ctx, innerW+stripeCol)
	stripe := lipgloss.NewStyle().Foreground(lipgloss.Color(ResolveAttachmentColor(a.Color))).Render(stripeGlyph) + " "
	for i, l := range inner.Lines {
		inner.Lines[i] = strings.TrimPrefix(l, stripe)
	}
	titleRows := 0
	if a.Title != "" {
		titleRows = 1
	}
	head = append(head, inner.Lines[:titleRows]...)
	body := inner.Lines[titleRows:]

	// The fold is for long text: an image's rows do not count toward it.
	textLines := len(body)
	for _, h := range inner.Hits {
		textLines -= h.RowEnd - h.RowStart
	}
	foldRow := ""
	if textLines >= foldMinBodyLines {
		foldRow = "▾ z to collapse"
		if !ctx.Card.Expanded {
			shown := foldPreviewLines
			for _, h := range inner.Hits {
				shown = min(shown, h.RowStart-titleRows)
			}
			foldRow = fmt.Sprintf("▸ %d more lines · z to expand", len(body)-shown)
			body = body[:shown]
			if shown > 0 {
				// Rewrapped two columns short, so " …" fits after a whole word.
				last := body[shown-1]
				if ctx.WrapText != nil {
					last, _, _ = strings.Cut(ctx.WrapText(last, innerW-2), "\n")
				}
				body[shown-1] = last + mutedStyle().Render(" …")
			}
			// The preview ends before the first image, so none is left.
			inner.Flushes, inner.SixelRows, inner.Hits = nil, nil, nil
		}
	}

	border := mutedStyle()
	if c := ResolveAttachmentColor(a.Color); c != ResolveAttachmentColor("") {
		border = lipgloss.NewStyle().Foreground(lipgloss.Color(c))
	}
	rule := strings.Repeat("─", max(width-2, 0))
	left, right := border.Render("│")+" ", " "+border.Render("│")
	boxed := func(l string) string {
		if lipgloss.Width(l) > innerW {
			l = ansi.Truncate(l, innerW, "…")
		}
		return left + padRight(l, innerW) + right
	}

	out.Lines = append(out.Lines, border.Render("╭"+rule+"╮"))
	for _, l := range head {
		out.Lines = append(out.Lines, boxed(l))
	}
	// inner counts rows from its title row; its first body row lands here.
	rowOffset := len(out.Lines) - titleRows
	for _, l := range body {
		out.Lines = append(out.Lines, boxed(l))
	}
	if foldRow != "" {
		out.FoldRows = append(out.FoldRows, len(out.Lines))
		out.Lines = append(out.Lines, boxed(mutedStyle().Render(foldRow)))
	}
	out.Lines = append(out.Lines, border.Render("╰"+rule+"╯"))

	out.Interactive = out.Interactive || inner.Interactive
	out.Flushes = append(out.Flushes, inner.Flushes...)
	for k, v := range inner.SixelRows {
		if out.SixelRows == nil {
			out.SixelRows = map[int]SixelEntry{}
		}
		out.SixelRows[k+rowOffset] = v
	}
	for _, h := range inner.Hits {
		h.RowStart += rowOffset
		h.RowEnd += rowOffset
		out.Hits = append(out.Hits, h)
	}
}

// A non-empty rich_text block counts here, unlike in RendersBody: an
// attachment has no host body row, so Render draws it in place.
func blocksCarryAttachmentBody(blocks []Block) bool {
	for _, b := range blocks {
		if rt, ok := b.(RichTextBlock); ok && RichTextToMrkdwn(rt) != "" {
			return true
		}
	}
	return RendersBody(blocks)
}

func linkedMessageHeader(a LegacyAttachment, link slackurl.Permalink, card CardContext, width int) string {
	place := ChannelLabel(link, card.ChannelNames)
	// A thread's first message links without thread_ts; Slack's footer
	// still calls it a thread.
	if link.ThreadTS != "" || strings.HasPrefix(a.Footer, "Thread in ") {
		place = strings.TrimSuffix("Thread in "+place, " in ")
	}
	meta := ""
	for _, part := range []string{place, linkedMessageTime(string(link.MessageTS), card.Now, card.TimeFormat)} {
		if part != "" {
			meta += " · " + part
		}
	}
	header := lipgloss.NewStyle().Bold(true).Render(a.AuthorName) + mutedStyle().Render(meta)
	return ansi.Truncate(header, width, "…")
}

// The same shape as cmd/slk's formatSearchTimestamp, which this package
// cannot import: today's messages show the time, older ones the date too.
func linkedMessageTime(ts string, now time.Time, timeFormat string) string {
	sec, err := strconv.ParseInt(strings.SplitN(ts, ".", 2)[0], 10, 64)
	if err != nil {
		return ""
	}
	if timeFormat == "" {
		timeFormat = "3:04 PM"
	}
	t := time.Unix(sec, 0)
	ny, nm, nd := now.Date()
	ty, tm, td := t.Date()
	switch {
	case ty == ny && tm == nm && td == nd:
		return t.Format(timeFormat)
	case ty == ny:
		return t.Format("Jan 2, ") + t.Format(timeFormat)
	default:
		return t.Format("Jan 2 2006, ") + t.Format(timeFormat)
	}
}
