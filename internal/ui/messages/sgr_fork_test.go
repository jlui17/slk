package messages

import (
	"math/rand/v2"
	"strings"
	"testing"
)

// splitSGRParamsReference is the split-and-join tokenizer
// writeSubstitutedSGRParams replaced, kept as the oracle it must match.
func splitSGRParamsReference(params string) (texts []string, isBg []bool) {
	if params == "" {
		return nil, nil
	}
	parts := strings.Split(params, ";")
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if p == "38" || p == "48" {
			if i+1 < len(parts) {
				n := map[string]int{"5": 3, "2": 5}[parts[i+1]]
				if n > 0 && i+n-1 < len(parts) {
					texts = append(texts, strings.Join(parts[i:i+n], ";"))
					isBg = append(isBg, p == "48")
					i += n - 1
					continue
				}
			}
			texts, isBg = append(texts, p), append(isBg, false)
			continue
		}
		texts, isBg = append(texts, p), append(isBg, isBasicBgParam(p))
	}
	return texts, isBg
}

func substituteSGRParamsReference(params, from, to string) string {
	texts, isBg := splitSGRParamsReference(params)
	for i := range texts {
		if isBg[i] && texts[i] == from {
			texts[i] = to
		}
	}
	return strings.Join(texts, ";")
}

// TestWriteSubstitutedSGRParamsMatchesSplitJoin feeds random parameter
// lists built from the fields that matter (colour introducers, modes,
// basic backgrounds, empty fields) through both tokenizers.
func TestWriteSubstitutedSGRParamsMatchesSplitJoin(t *testing.T) {
	fields := []string{"38", "48", "5", "2", "40", "41", "100", "107", "26", "46", "1", "0", "", "255", "39", "49"}
	froms := []string{"48;2;26;26;46", "40", "48;5;40", "100"}
	rng := rand.New(rand.NewPCG(1, 2))
	for n := 0; n < 20000; n++ {
		k := rng.IntN(9)
		parts := make([]string, k)
		for i := range parts {
			parts[i] = fields[rng.IntN(len(fields))]
		}
		params := strings.Join(parts, ";")
		from := froms[rng.IntN(len(froms))]
		var b strings.Builder
		writeSubstitutedSGRParams(&b, params, from, "TO")
		if want := substituteSGRParamsReference(params, from, "TO"); b.String() != want {
			t.Fatalf("params %q from %q: got %q, want %q", params, from, b.String(), want)
		}
	}
}

// BenchmarkRepaintBgToSelectionTint is the selected variant's repaint
// of one span-heavy rendered body, run once per message per cache build.
func BenchmarkRepaintBgToSelectionTint(b *testing.B) {
	body := RenderSlackMarkdownWith(reapplyBenchText(10), reapplyBenchOpts)
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	for b.Loop() {
		_ = RepaintBgToSelectionTint(body, true)
	}
}
