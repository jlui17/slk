package messages

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// LinkAt returns the target of the OSC 8 hyperlink drawn at display
// column col of a rendered row, or "" when that cell is outside any
// link. The wrapper closes a link at a row's end and opens it again on
// the next, so a row alone says which link each of its cells is in.
// Columns are counted as buildPlainLine counts them for the selection:
// uniseg grapheme widths over the whole row with its escapes stripped,
// so a cluster a style escape splits still counts once.
func LinkAt(line string, col int) string {
	type linkStart struct {
		at  int // byte offset in plain
		url string
	}
	var plain strings.Builder
	var starts []linkStart
	for line != "" {
		if line[0] == ansi.ESC {
			_, _, n, _ := ansi.DecodeSequence(line, ansi.NormalState, nil)
			if url, ok := osc8Target(line[:n]); ok {
				starts = append(starts, linkStart{plain.Len(), url})
			}
			line = line[n:]
			continue
		}
		end := strings.IndexByte(line, ansi.ESC)
		if end < 0 {
			end = len(line)
		}
		plain.WriteString(line[:end])
		line = line[end:]
	}
	rest, at, x, state := plain.String(), 0, 0, -1
	for rest != "" {
		var cluster string
		var w int
		cluster, rest, w, state = uniseg.FirstGraphemeClusterInString(rest, state)
		if col >= x && col < x+w {
			url := ""
			for _, s := range starts {
				if s.at <= at {
					url = s.url
				}
			}
			return url
		}
		x += w
		at += len(cluster)
	}
	return ""
}

// osc8Target reads an OSC 8 sequence (ESC ] 8 ; params ; URL ST): ok is
// false for any other escape, and the URL is "" for the one that closes
// a link.
func osc8Target(seq string) (url string, ok bool) {
	body, ok := strings.CutPrefix(seq, "\x1b]8;")
	if !ok {
		return "", false
	}
	body = strings.TrimSuffix(strings.TrimSuffix(body, "\x1b\\"), "\a")
	_, url, ok = strings.Cut(body, ";")
	return url, ok
}

// LinkURLAt returns the URL of the hyperlink drawn at pane-local
// (viewportY, x), the frame BeginSelectionAt takes, or "" when no link
// is drawn there.
func (m *Model) LinkURLAt(viewportY, x int) string {
	if viewportY < m.chromeHeight {
		return ""
	}
	abs := viewportY - m.chromeHeight + m.yOffset
	for i, e := range m.cache {
		if start := m.entryOffsets[i]; abs >= start && abs < start+e.height {
			return LinkAt(e.linesNormal[abs-start], x)
		}
	}
	return ""
}
