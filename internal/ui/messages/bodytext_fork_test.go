package messages

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/styles"
)

// TestRenderBodyTextMatchesLipgloss checks renderBodyText byte for byte
// against MessageText.Render, plain and italic, in every theme, over
// bodies of styled spans, links, wide runes and uneven lines.
func TestRenderBodyTextMatchesLipgloss(t *testing.T) {
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	pieces := []string{"word", " ", "  ", "\n", "\n\n", "\x1b[1m", "\x1b[m", "\x1b[0m", "\x1b[38;2;1;2;3m", "\x1b]8;;https://x.y\x1b\\", "\x1b]8;;\x1b\\", "你好", "😀", "e\u0301", "\t", "\r\n"}
	rng := rand.New(rand.NewPCG(5, 6))
	for _, theme := range styles.ThemeNames() {
		styles.Apply(theme, config.Theme{})
		for n := 0; n < 300; n++ {
			var b strings.Builder
			for range rng.IntN(16) {
				b.WriteString(pieces[rng.IntN(len(pieces))])
			}
			s := b.String()
			for _, italic := range []bool{false, true} {
				st := styles.MessageText
				if italic {
					st = st.Italic(true)
				}
				if got, want := renderBodyText(italic, s), st.Render(s); got != want {
					t.Fatalf("theme %q italic=%v body %q:\ngot  %q\nwant %q", theme, italic, s, got, want)
				}
			}
		}
	}
}

// BenchmarkRenderBodyText is the styling of one wrapped multi-line
// body, run once per message on every cache build.
func BenchmarkRenderBodyText(b *testing.B) {
	body := WordWrap(RenderSlackMarkdownWith(reapplyBenchText(6), reapplyBenchOpts), 100)
	for _, impl := range []struct {
		name   string
		render func(string) string
	}{
		{"lipgloss", func(s string) string { return styles.MessageText.Render(s) }},
		{"renderBodyText", func(s string) string { return renderBodyText(false, s) }},
	} {
		b.Run(fmt.Sprintf("impl=%s", impl.name), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = impl.render(body)
			}
		})
	}
}
