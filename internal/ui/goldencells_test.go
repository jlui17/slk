package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCellDiff_EqualCellsDifferentBytes(t *testing.T) {
	a := "\x1b[48;2;1;2;3mab\x1b[m"
	b := "\x1b[48;2;1;2;3ma\x1b[m\x1b[48;2;1;2;3mb\x1b[m"
	if d := cellDiff(a, b); d != "" {
		t.Fatalf("re-encoded but identical cells reported as different: %s", d)
	}
}

func TestCellDiff_CatchesOnePerturbedCell(t *testing.T) {
	a := "\x1b[48;2;1;2;3mab\x1b[m\nxy"
	b := "\x1b[48;2;1;2;3ma\x1b[48;2;9;9;9mb\x1b[m\nxy"
	d := cellDiff(a, b)
	if d == "" {
		t.Fatal("a cell whose background changed was not reported")
	}
	if !strings.Contains(d, "row 0 col 1") {
		t.Fatalf("wrong location reported: %s", d)
	}
}

func TestCellDiff_ResetEndsAttributes(t *testing.T) {
	if cellDiff("\x1b[1mA\x1b[0mB", "\x1b[1mAB") == "" {
		t.Fatal("a reset that ends bold was ignored")
	}
}

func TestCellDiff_IgnoresOSC(t *testing.T) {
	if d := cellDiff("\x1b]8;;https://x\x07a\x1b]8;;\x07", "a"); d != "" {
		t.Fatalf("an OSC 8 hyperlink was counted as cells: %s", d)
	}
}

func TestCellDiff_CatchesRowCountAndLength(t *testing.T) {
	if cellDiff("a\nb", "a") == "" {
		t.Fatal("a missing row was not reported")
	}
	if cellDiff("ab", "a") == "" {
		t.Fatal("a missing cell was not reported")
	}
}

// cellDiff compares two ANSI screens cell by cell: each rune with the SGR
// state it was drawn in. Escape-sequence encoding is ignored, so two
// screens that look identical compare equal even when their bytes differ.
// Returns "" when equal, else the first difference.
//
// Used to prove the Phase 3 golden re-bless (thread_open, wide, narrow)
// changes no visible cell. See TestGoldenCellsMatchBaseline.
func cellDiff(want, got string) string {
	wr, gr := strings.Split(want, "\n"), strings.Split(got, "\n")
	if len(wr) != len(gr) {
		return "row count: want " + strconv.Itoa(len(wr)) + ", got " + strconv.Itoa(len(gr))
	}
	for r := range wr {
		wc, gc := lineCells(wr[r]), lineCells(gr[r])
		for c := 0; c < min(len(wc), len(gc)); c++ {
			if wc[c] != gc[c] {
				return "row " + strconv.Itoa(r) + " col " + strconv.Itoa(c) +
					": want " + wc[c].String() + ", got " + gc[c].String()
			}
		}
		if len(wc) != len(gc) {
			return "row " + strconv.Itoa(r) + ": want " + strconv.Itoa(len(wc)) +
				" cells, got " + strconv.Itoa(len(gc))
		}
	}
	return ""
}

type screenCell struct {
	glyph string
	attrs string // canonical SGR state
}

func (c screenCell) String() string { return "{" + c.glyph + " " + c.attrs + "}" }

// lineCells splits one row into cells: one per rune, each carrying the
// SGR state in force when it was drawn. CSI sequences update the state
// (only SGR, final byte 'm', is interpreted); OSC and other escapes are
// skipped.
func lineCells(line string) []screenCell {
	st := map[string]string{}
	var out []screenCell
	for i := 0; i < len(line); {
		if line[i] == 0x1b && i+1 < len(line) {
			switch line[i+1] {
			case '[':
				j := i + 2
				for j < len(line) && (line[j] < 0x40 || line[j] > 0x7e) {
					j++
				}
				if j < len(line) && line[j] == 'm' {
					applySGR(st, line[i+2:j])
				}
				i = j + 1
			case ']':
				j := i + 2
				for j < len(line) {
					if line[j] == 0x07 {
						j++
						break
					}
					if line[j] == 0x1b && j+1 < len(line) && line[j+1] == '\\' {
						j += 2
						break
					}
					j++
				}
				i = j
			default:
				i += 2
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		out = append(out, screenCell{glyph: string(r), attrs: canonicalSGR(st)})
		i += size
	}
	return out
}

// applySGR folds one SGR parameter list into st. Colours are kept by
// value (38/48 truecolor or 256-colour); every other parameter is a flag
// that its "off" code clears.
func applySGR(st map[string]string, params string) {
	if params == "" {
		clear(st)
		return
	}
	off := map[string][]string{
		"22": {"1", "2"}, "23": {"3"}, "24": {"4"}, "25": {"5"},
		"27": {"7"}, "28": {"8"}, "29": {"9"}, "39": {"38"}, "49": {"48"},
	}
	ps := strings.Split(params, ";")
	for k := 0; k < len(ps); k++ {
		p := ps[k]
		if p == "" {
			p = "0"
		}
		switch {
		case p == "0":
			clear(st)
		case (p == "38" || p == "48") && k+4 < len(ps) && ps[k+1] == "2":
			st[p] = "2;" + strings.Join(ps[k+2:k+5], ";")
			k += 4
		case (p == "38" || p == "48") && k+2 < len(ps) && ps[k+1] == "5":
			st[p] = "5;" + ps[k+2]
			k += 2
		case off[p] != nil:
			for _, q := range off[p] {
				delete(st, q)
			}
		default:
			st[p] = "on"
		}
	}
}

func canonicalSGR(st map[string]string) string {
	keys := make([]string, 0, len(st))
	for k := range st {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + "=" + st[k] + " ")
	}
	return b.String()
}

// TestGoldenCellsMatchBaseline proves every golden in testdata/golden is
// cell-identical to the same-named file in $SLK_GOLDEN_BASELINE. It is
// opt-in: skipped unless the variable names a directory. PR 2+3 runs it
// against the goldens from before its re-bless.
func TestGoldenCellsMatchBaseline(t *testing.T) {
	dir := os.Getenv("SLK_GOLDEN_BASELINE")
	if dir == "" {
		t.Skip("set SLK_GOLDEN_BASELINE to a directory of baseline .ansi goldens")
	}
	files, err := filepath.Glob(filepath.Join(goldenDir, "*.ansi"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no goldens under %s: %v", goldenDir, err)
	}
	for _, f := range files {
		cur, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		base, err := os.ReadFile(filepath.Join(dir, filepath.Base(f)))
		if err != nil {
			t.Fatalf("baseline for %s: %v", filepath.Base(f), err)
		}
		if d := cellDiff(string(base), string(cur)); d != "" {
			t.Errorf("%s is not cell-identical to its baseline: %s", filepath.Base(f), d)
		} else if string(base) != string(cur) {
			t.Logf("%s: bytes differ, cells identical", filepath.Base(f))
		}
	}
}
