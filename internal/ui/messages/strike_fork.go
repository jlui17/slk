package messages

import (
	"strings"
	"unicode"
)

// replaceStrikethroughs rewrites `~X~` runs through render when the
// tildes sit on Slack's strike boundaries; tildes that don't pair up
// (the prose "~45%" meaning approximately) stay literal. The old
// regex `~([^~\n]+)~` paired any two same-line tildes, crossing out
// everything between two "approximately" markers.
//
// A `~` opens a run only when preceded by start-of-text or a non-word
// rune and followed by non-whitespace; it closes one only when
// preceded by non-whitespace and followed by end-of-text or a
// non-word rune. When the first `~` after an opener fails the closer
// test, the opener is literal (mirroring renderItalics' scan).
//
// This pass runs after bold/italic are ANSI-rendered, so a nested
// `*~x~*` opener is preceded by an SGR sequence's terminating "m";
// strikeBoundaryBefore treats that terminator as a boundary.
func replaceStrikethroughs(text string, render func(inner string) string) string {
	if !strings.ContainsRune(text, '~') {
		return text
	}
	runes := []rune(text)
	var b strings.Builder
	b.Grow(len(text))
	i := 0
	for i < len(runes) {
		if runes[i] != '~' {
			b.WriteRune(runes[i])
			i++
			continue
		}
		if i > 0 && !strikeBoundaryBefore(runes, i) {
			b.WriteRune(runes[i])
			i++
			continue
		}
		if i+1 < len(runes) && unicode.IsSpace(runes[i+1]) {
			b.WriteRune(runes[i])
			i++
			continue
		}
		j := i + 1
		for j < len(runes) && runes[j] != '~' && runes[j] != '\n' {
			j++
		}
		if j >= len(runes) || runes[j] != '~' || j == i+1 ||
			unicode.IsSpace(runes[j-1]) ||
			(j+1 < len(runes) && isStrikeWordRune(runes[j+1])) {
			b.WriteRune(runes[i])
			i++
			continue
		}
		b.WriteString(render(string(runes[i+1 : j])))
		i = j + 1
	}
	return b.String()
}

// strikeBoundaryBefore reports whether the rune before position i acts
// as a word boundary for a `~` opener: a non-word rune, or the
// terminating "m" of an SGR escape sequence ("\x1b[…m") emitted by an
// earlier styling pass.
func strikeBoundaryBefore(runes []rune, i int) bool {
	if !isStrikeWordRune(runes[i-1]) {
		return true
	}
	if runes[i-1] != 'm' {
		return false
	}
	for k := i - 2; k >= 0; k-- {
		switch {
		case runes[k] >= '0' && runes[k] <= '9', runes[k] == ';':
			continue
		case runes[k] == '[':
			return k > 0 && runes[k-1] == '\x1b'
		default:
			return false
		}
	}
	return false
}

func isStrikeWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
