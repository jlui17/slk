package messages

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gammons/slk/internal/text"
	"github.com/gammons/slk/internal/ui/styles"
)

// SearchHighlightSGR derives the raw open/close SGR sequences for
// SearchHighlightStyle by rendering a sentinel and splitting on it —
// works for any lipgloss color profile without hand-building escapes.
//
// The close is a bare reset; callers restore their own bg/fg after it
// with ReapplyBgAfterResets.
func SearchHighlightSGR() (start, end string, ok bool) {
	parts := strings.SplitN(styles.SearchHighlightStyle().Render("\x00"), "\x00", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// HighlightSearchTerms wraps case- and accent-insensitive word-prefix
// occurrences of terms in s with hlStart/hlEnd. s may contain ANSI
// escape sequences (CSI/SGR, OSC hyperlinks, other escapes): they are
// skipped during matching, so a term may span them. Outside a match they
// are preserved byte-identical; a CSI SGR inside a match is withheld so
// it cannot paint over the highlight, and every SGR active at the match's
// end (including the withheld ones) is re-emitted after hlEnd so the
// highlight does not clobber surrounding styling.
//
// terms must already be folded (text.Fold). Matching is per-rune
// folded comparison, which keeps a 1:1 position mapping for the
// diacritics Fold removes.
func HighlightSearchTerms(s string, terms []string, hlStart, hlEnd string) string {
	if len(terms) == 0 || s == "" {
		return s
	}

	type seg struct {
		text   string // visible run or escape sequence
		isANSI bool
		opaque bool // ANSI but not CSI: never reset/re-applied (OSC, 2-byte escapes)
	}
	// Segment s into visible-text runs and ANSI escapes. Every branch
	// below advances i by at least one byte, so zero-length segments
	// (and the infinite loop they caused on non-CSI escapes) are
	// structurally impossible.
	var segs []seg
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			j := strings.IndexByte(s[i:], 0x1b)
			if j < 0 {
				j = len(s) - i
			}
			segs = append(segs, seg{text: s[i : i+j]})
			i += j
			continue
		}
		if i+1 >= len(s) {
			// Bare trailing ESC: opaque 1-byte segment.
			segs = append(segs, seg{text: s[i:], isANSI: true, opaque: true})
			break
		}
		switch s[i+1] {
		case '[': // CSI: parameter bytes through final byte in 0x40-0x7e
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j < len(s) {
				j++ // include final byte (truncated CSI: take what's there)
			}
			segs = append(segs, seg{text: s[i:j], isANSI: true})
			i = j
		case ']': // OSC: terminated by BEL or ST (\x1b\), terminator included
			j := i + 2
			end := len(s) // unterminated OSC consumes the rest of the string
			for j < len(s) {
				if s[j] == 0x07 {
					end = j + 1
					break
				}
				if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
					end = j + 2
					break
				}
				j++
			}
			// Opaque: the payload (e.g. an OSC-8 URL) must never be
			// matched or highlighted — corrupting it breaks the
			// hyperlink — and must not enter the SGR re-apply list.
			segs = append(segs, seg{text: s[i:end], isANSI: true, opaque: true})
			i = end
		default:
			// Any other escape: consume ESC plus the next byte as an
			// opaque 2-byte segment (e.g. \x1b(B charset designation).
			segs = append(segs, seg{text: s[i : i+2], isANSI: true, opaque: true})
			i += 2
		}
	}

	// Match on the visible rune stream, ignoring escapes, so a term that
	// spans a styled boundary (a colored token, an inline span) still lights
	// up as one run.
	var runes []rune
	var folded []string
	for _, sg := range segs {
		if sg.isANSI {
			continue
		}
		for _, r := range sg.text {
			runes = append(runes, r)
			folded = append(folded, foldRune(r))
		}
	}
	matchLen := make([]int, len(runes))
	prevRune := rune(0)
	for g := 0; g < len(runes); {
		n := 0
		if !unicode.IsLetter(prevRune) && !unicode.IsDigit(prevRune) {
			for _, term := range terms {
				if n = prefixMatchLen(folded, g, term); n > 0 {
					break
				}
			}
		}
		if n > 0 {
			matchLen[g] = n
			prevRune = runes[g+n-1]
			g += n
			continue
		}
		prevRune = runes[g]
		g++
	}

	var out strings.Builder
	var active []string // SGR sequences since last reset, for re-apply
	g, remaining := 0, 0
	for _, sg := range segs {
		if sg.isANSI {
			switch {
			case sg.opaque:
				out.WriteString(sg.text)
			case sg.text == "\x1b[0m" || sg.text == "\x1b[m":
				out.WriteString(sg.text)
				active = active[:0]
				if remaining > 0 {
					out.WriteString(hlStart)
				}
			case remaining > 0:
				// Withheld: a color change inside the match would paint over
				// the highlight. It is replayed with active after hlEnd.
				active = append(active, sg.text)
			default:
				out.WriteString(sg.text)
				active = append(active, sg.text)
			}
			continue
		}
		for _, r := range sg.text {
			if remaining == 0 && matchLen[g] > 0 {
				out.WriteString(hlStart)
				remaining = matchLen[g]
			}
			out.WriteRune(r)
			g++
			if remaining > 0 {
				if remaining--; remaining == 0 {
					out.WriteString(hlEnd)
					for _, a := range active {
						out.WriteString(a)
					}
				}
			}
		}
	}
	return out.String()
}

func foldRune(r rune) string {
	if r < utf8.RuneSelf {
		// ASCII fast path: text.Fold allocates a transform chain per
		// call (see fold.go); for ASCII, folding is just lowercasing.
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		return string(r)
	}
	return text.Fold(string(r))
}

// prefixMatchLen reports how many runes starting at folded[i] are
// consumed matching term, or 0 if term is not a prefix there.
func prefixMatchLen(folded []string, i int, term string) int {
	rest := term
	n := 0
	for i+n < len(folded) && rest != "" {
		f := folded[i+n]
		if !strings.HasPrefix(rest, f) {
			return 0
		}
		rest = rest[len(f):]
		n++
	}
	if rest != "" {
		return 0
	}
	return n
}
