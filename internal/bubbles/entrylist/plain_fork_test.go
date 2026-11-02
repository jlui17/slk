package entrylist

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// TestBuildPlainLine_ASCIIRunMatchesUniseg strings ASCII next to every
// kind of cluster an ASCII byte can border or join (combining marks,
// variation selectors, ZWJ sequences, flags, wide runes, CR LF, tabs,
// DEL) and checks the column map against the uniseg reference.
func TestBuildPlainLine_ASCIIRunMatchesUniseg(t *testing.T) {
	pieces := []string{"a", "Z", " ", "  ", "#", "1", "́", "️", "‍", "👩", "💻", "🇺", "🇸", "你", "\r", "\n", "\t", "\x7f", "é", "❤", "*️⃣"}
	rng := rand.New(rand.NewPCG(3, 4))
	for n := 0; n < 20000; n++ {
		var b strings.Builder
		for range rng.IntN(12) {
			b.WriteString(pieces[rng.IntN(len(pieces))])
		}
		s := b.String()
		got, want := buildPlainLine(s), buildPlainLineUniseg(s)
		if got.Text != want.Text || !slices.Equal(got.Bytes, want.Bytes) {
			t.Fatalf("%q: got %v, want %v", s, got.Bytes, want.Bytes)
		}
	}
}

// BenchmarkPlainLines is the plain mirror of one filled message: a few
// rows of text padded with spaces to a wide pane, the shape WrapRows
// hands PlainLines on every cache build.
func BenchmarkPlainLines(b *testing.B) {
	for _, width := range []int{120, 477} {
		row := "Hello world this is a moderately long message with bold and italic formatting 😀 done."
		rows := make([]string, 4)
		for i := range rows {
			rows[i] = row + strings.Repeat(" ", width-DisplayWidth(buildPlainLineUniseg(row)))
		}
		s := strings.Join(rows, "\n")
		b.Run(fmt.Sprintf("width=%d", width), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = PlainLines(s)
			}
		})
	}
}
