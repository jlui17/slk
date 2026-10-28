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

// A terminal keeps SGR state across a newline, so a bold left open on one
// row is still bold on the next. Two screens that differ only there look
// different and must compare different.
func TestCellDiff_StyleCarriesAcrossNewlines(t *testing.T) {
	if cellDiff("\x1b[1mA\nB", "\x1b[1mA\n\x1b[0mB") == "" {
		t.Fatal("a style still open at a newline was dropped for the next row")
	}
	if d := cellDiff("\x1b[1mA\nB", "\x1b[1mA\n\x1b[1mB"); d != "" {
		t.Fatalf("re-asserting a style that was already carried reported a difference: %s", d)
	}
}

// compareGoldenDirs compares only the goldens present in both directories,
// lists goldens new since the baseline separately, and fails a run that
// compared nothing.
func TestCompareGoldenDirs(t *testing.T) {
	base, cur := t.TempDir(), t.TempDir()
	write := func(dir, name, s string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(base, "same.ansi", "\x1b[1mab\x1b[m")
	write(cur, "same.ansi", "\x1b[1ma\x1b[m\x1b[1mb\x1b[m")
	write(base, "moved.ansi", "ab")
	write(cur, "moved.ansi", "ax")
	write(cur, "added.ansi", "new")

	r, err := compareGoldenDirs(base, cur)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"moved.ansi", "same.ansi"}; !slices.Equal(r.compared, want) {
		t.Errorf("compared %v, want %v", r.compared, want)
	}
	if want := []string{"added.ansi"}; !slices.Equal(r.added, want) {
		t.Errorf("added %v, want %v", r.added, want)
	}
	if len(r.diffs) != 1 || !strings.HasPrefix(r.diffs[0], "moved.ansi") {
		t.Errorf("diffs %q, want exactly moved.ansi", r.diffs)
	}
	if want := []string{"same.ansi"}; !slices.Equal(r.reencoded, want) {
		t.Errorf("re-encoded %v, want %v", r.reencoded, want)
	}

	if _, err := compareGoldenDirs(t.TempDir(), cur); err == nil {
		t.Error("a baseline with no golden in common compared nothing and still passed")
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
// The SGR state carries across newlines, as it does on a terminal.
// Returns "" when equal, else the first difference.
//
// Used to prove the Phase 3 golden re-bless (thread_open, wide, narrow)
// changes no visible cell. See TestGoldenCellsMatchBaseline.
func cellDiff(want, got string) string {
	wr, gr := screenCells(want), screenCells(got)
	if len(wr) != len(gr) {
		return "row count: want " + strconv.Itoa(len(wr)) + ", got " + strconv.Itoa(len(gr))
	}
	for r := range wr {
		wc, gc := wr[r], gr[r]
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

// screenCells splits a screen into rows of cells. The SGR state is shared
// by every row, so a style left open at the end of one row applies to the
// next.
func screenCells(s string) [][]screenCell {
	st := map[string]string{}
	var rows [][]screenCell
	for _, line := range strings.Split(s, "\n") {
		rows = append(rows, lineCells(line, st))
	}
	return rows
}

// lineCells splits one row into cells: one per rune, each carrying the
// SGR state in force when it was drawn. It reads and updates st, the
// state in force at the start of the row. CSI sequences update the state
// (only SGR, final byte 'm', is interpreted); OSC and other escapes are
// skipped.
func lineCells(line string, st map[string]string) []screenCell {
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

// goldenDirComparison is the outcome of comparing a directory of goldens
// against a baseline directory. All lists hold file names, sorted.
type goldenDirComparison struct {
	compared  []string // in both directories
	reencoded []string // compared: bytes differ, cells identical
	diffs     []string // compared: "<name>: <first differing cell>"
	added     []string // only in the current directory: nothing to compare
}

// compareGoldenDirs compares every .ansi file in cur with the same-named
// file in base, cell by cell. A golden with no baseline (a scenario added
// since) is listed as added, not compared. It is an error to compare
// nothing: a wrong baseline path must not pass.
func compareGoldenDirs(base, cur string) (goldenDirComparison, error) {
	var r goldenDirComparison
	files, err := filepath.Glob(filepath.Join(cur, "*.ansi"))
	if err != nil {
		return r, err
	}
	for _, f := range files {
		name := filepath.Base(f)
		want, err := os.ReadFile(filepath.Join(base, name))
		if os.IsNotExist(err) {
			r.added = append(r.added, name)
			continue
		}
		if err != nil {
			return r, err
		}
		got, err := os.ReadFile(f)
		if err != nil {
			return r, err
		}
		r.compared = append(r.compared, name)
		switch d := cellDiff(string(want), string(got)); {
		case d != "":
			r.diffs = append(r.diffs, name+": "+d)
		case string(want) != string(got):
			r.reencoded = append(r.reencoded, name)
		}
	}
	if len(r.compared) == 0 {
		return r, os.ErrNotExist
	}
	return r, nil
}

// TestGoldenCellsMatchBaseline proves every golden in testdata/golden is
// cell-identical to the same-named file in $SLK_GOLDEN_BASELINE. Goldens
// the baseline does not have are new scenarios and are listed, not
// compared. It is opt-in: skipped unless the variable names a directory.
// Phase 3's thread goldens were proven this way against the goldens from
// before their re-bless.
func TestGoldenCellsMatchBaseline(t *testing.T) {
	dir := os.Getenv("SLK_GOLDEN_BASELINE")
	if dir == "" {
		t.Skip("set SLK_GOLDEN_BASELINE to a directory of baseline .ansi goldens")
	}
	r, err := compareGoldenDirs(dir, goldenDir)
	if err != nil {
		t.Fatalf("no golden in %s has a baseline in %s: %v", goldenDir, dir, err)
	}
	for _, d := range r.diffs {
		t.Errorf("not cell-identical to its baseline: %s", d)
	}
	for _, name := range r.reencoded {
		t.Logf("%s: bytes differ, cells identical", name)
	}
	for _, name := range r.added {
		t.Logf("%s: new golden, no baseline to compare", name)
	}
}
