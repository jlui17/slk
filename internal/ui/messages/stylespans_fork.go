package messages

import (
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/styles"
)

// A bold, italic or struck span is found on the raw text, where Slack's
// markers are, but drawn last, once the link, mention and emoji passes
// have drawn what sits inside it: its pass wraps it in a pair of marks
// (markSpan), and drawSpans turns the marks into SGR at the end of
// renderInlineFormattingWith. Styling it on the spot would cut an inner
// <@U1> or :tada: apart before its pass could match it, and the reset
// that ends an inner span would end the outer style with it.
type spanAttr rune

const (
	spanBold spanAttr = iota
	spanItalic
	spanStrike
	spanAttrs
)

// The marks are noncharacters (U+FDD0 on), which Unicode keeps for a
// program's internal use, so Slack text never carries them. Each
// attribute takes two: open, then close.
const spanMarkFirst = '﷐'

var (
	spanStyles = [spanAttrs]func() lipgloss.Style{boldStyle, italicStyle, strikethroughStyle}
	spanSGR    = [spanAttrs]string{"1", "3", "9"}
	spanOffSGR = [spanAttrs]string{"22", "23", "29"}
)

func markSpan(a spanAttr, inner string) string {
	open := spanMarkFirst + 2*rune(a)
	return string(open) + inner + string(open+1)
}

func isSpanMark(r rune) bool {
	return r >= spanMarkFirst && r < spanMarkFirst+2*rune(spanAttrs)
}

// drawSpans styles the marked spans in text. A span with nothing drawn
// inside it renders through its lipgloss style, as it always has. One
// with an escape sequence in it keeps each sequence whole, turns its
// attributes back on after every reset, and turns strike on before each
// grapheme, so a row the wrapper starts mid-span, which carries no open
// style, is struck too.
func drawSpans(text string) string {
	if strings.IndexFunc(text, isSpanMark) < 0 {
		return text
	}
	var b strings.Builder
	var on [spanAttrs]int
	for {
		i := strings.IndexFunc(text, isSpanMark)
		if i < 0 {
			drawSpanText(&b, text, on)
			break
		}
		drawSpanText(&b, text[:i], on)
		mark, size := utf8.DecodeRuneInString(text[i:])
		text = text[i+size:]
		a, open := spanAttr((mark-spanMarkFirst)/2), (mark-spanMarkFirst)%2 == 0
		if open && !anySpanOn(on) {
			if inner, rest, ok := plainSpan(text, mark+1); ok {
				b.WriteString(spanStyles[a]().Render(inner))
				text = rest
				continue
			}
		}
		switch {
		case open:
			on[a]++
			b.WriteString(spanOnSGR(on))
		case on[a] > 0:
			on[a]--
			if !anySpanOn(on) {
				b.WriteString("\x1b[m")
			} else if on[a] == 0 {
				b.WriteString("\x1b[" + spanOffSGR[a] + "m")
			}
		}
	}
	if anySpanOn(on) {
		b.WriteString("\x1b[m")
	}
	return b.String()
}

// plainSpan returns the span text starts with when nothing in it is drawn
// yet: no escape sequence and no other span.
func plainSpan(text string, closeMark rune) (inner, rest string, ok bool) {
	end := strings.IndexRune(text, closeMark)
	if end < 0 {
		return "", "", false
	}
	inner = text[:end]
	if strings.ContainsRune(inner, '\x1b') || strings.IndexFunc(inner, isSpanMark) >= 0 {
		return "", "", false
	}
	return inner, text[end+utf8.RuneLen(closeMark):], true
}

func drawSpanText(b *strings.Builder, s string, on [spanAttrs]int) {
	if !anySpanOn(on) {
		b.WriteString(s)
		return
	}
	var state byte
	for s != "" {
		seq, _, n, newState := ansi.DecodeSequence(s, state, nil)
		state, s = newState, s[n:]
		if ansi.HasEscPrefix(seq) {
			b.WriteString(seq)
			if seq == "\x1b[m" || seq == "\x1b[0m" {
				b.WriteString(spanOnSGR(on))
			}
			continue
		}
		if on[spanStrike] > 0 {
			b.WriteString("\x1b[9m")
		}
		b.WriteString(seq)
	}
}

// spanOnSGR turns on every attribute open in on, in the text color the
// span styles set.
func spanOnSGR(on [spanAttrs]int) string {
	var params []string
	for a, n := range on {
		if n > 0 {
			params = append(params, spanSGR[a])
		}
	}
	return "\x1b[" + strings.Join(params, ";") + "m" + fgANSIFor(styles.TextPrimary)
}

func anySpanOn(on [spanAttrs]int) bool {
	for _, n := range on {
		if n > 0 {
			return true
		}
	}
	return false
}
