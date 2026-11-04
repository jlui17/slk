package messages

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// replaceBolds rewrites `*X*` runs through render when the asterisks sit
// on Slack's bold boundaries; asterisks that don't (the cron expression
// "37 10 * * *", the product "2 * 3 * 4", "a*b*c") stay literal. The old
// regex `\*([^*\n]+)\*` paired any two same-line asterisks.
func replaceBolds(text string, render func(inner string) string) string {
	return replaceDelimitedRuns(text, '*', render)
}

func markBold(inner string) string {
	return markSpan(spanBold, inner)
}

// DropZeroWidthSpaces removes the zero-width spaces that let a marker
// pair mid-word (blockkit.ZeroWidthSpace) from text leaving the mrkdwn:
// drawn, so the word reads and searches whole (inline code is still held
// then, so a code span keeps its own), copied, and exported, where a
// paste into a shell or a search box would break the word. Edit source
// never has them: it reads the rich text through RichTextToMarkdown.
func DropZeroWidthSpaces(text string) string {
	return strings.ReplaceAll(text, blockkit.ZeroWidthSpace, "")
}

// replaceDelimitedRuns rewrites the delim-wrapped runs of text through
// render, by Slack's rules for bold and strike: a delim opens a run only
// when preceded by start-of-text or a non-word rune and followed by
// non-whitespace; it closes one only when preceded by non-whitespace and
// followed by end-of-text or a non-word rune (runHugsText covers the
// whitespace half, which renderItalics shares). When the first delim after
// an opener fails the closer test, the opener is literal (mirroring
// renderItalics' scan); a guarded delim (isGuardedMarker) is not a
// candidate, so the scan looks past it.
//
// Inline code is held out of text while this runs (heldInlineCode), so
// its markers stay literal, and the hold's mark is a non-word rune, so a
// run may open right after a code span. A nested `*~x~*` opener follows
// bold's span mark, also a non-word rune.
func replaceDelimitedRuns(text string, delim rune, render func(inner string) string) string {
	if !strings.ContainsRune(text, delim) {
		return text
	}
	runes := []rune(text)
	var b strings.Builder
	b.Grow(len(text))
	i := 0
	for i < len(runes) {
		if runes[i] != delim {
			b.WriteRune(runes[i])
			i++
			continue
		}
		if i > 0 && isDelimWordRune(runes[i-1]) {
			b.WriteRune(runes[i])
			i++
			continue
		}
		j := i + 1
		for j < len(runes) && runes[j] != '\n' && (runes[j] != delim || isGuardedMarker(runes, j)) {
			j++
		}
		if j >= len(runes) || runes[j] != delim || j == i+1 || !runHugsText(runes, i, j) ||
			(j+1 < len(runes) && isDelimWordRune(runes[j+1])) {
			b.WriteRune(runes[i])
			i++
			continue
		}
		b.WriteString(render(string(runes[i+1 : j])))
		i = j + 1
	}
	return b.String()
}

// runHugsText reports whether the markers at opener and closer sit right
// against the text between them, as Slack requires of bold, italic and
// strike: "*bold*" is bold, "* not *" is not.
func runHugsText(runes []rune, opener, closer int) bool {
	return !unicode.IsSpace(runes[opener+1]) && !unicode.IsSpace(runes[closer-1])
}

// isGuardedMarker reports whether the marker at runes[i] has a zero-width
// space right before it: the rich-text converter guards a styled run's
// own markers inside its text that way ("*5 \u200b* 3 = 15*", see
// blockkit's guardInnerMarkers), so it never closes a run. Typed mrkdwn
// carries no such guard and pairs as before.
func isGuardedMarker(runes []rune, i int) bool {
	return i > 0 && string(runes[i-1]) == blockkit.ZeroWidthSpace
}

func isDelimWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// heldInlineCode keeps rendered inline code spans out of the text while
// the bold, italic and strike passes run, as Slack formats nothing
// inside backticks. Each span stands in the text as a marker: a
// noncharacter (see spanMarkFirst) around its index.
type heldInlineCode []string

const inlineCodeMark = '\uFDEF'

func inlineCodeMarker(i int) string {
	return string(inlineCodeMark) + strconv.Itoa(i) + string(inlineCodeMark)
}

func (h *heldInlineCode) hold(rendered string) string {
	*h = append(*h, rendered)
	return inlineCodeMarker(len(*h) - 1)
}

func (h heldInlineCode) restore(text string) string {
	for i, code := range h {
		text = strings.Replace(text, inlineCodeMarker(i), code, 1)
	}
	return text
}
