# Phase 3 — Collapse the `messages`/`thread` fork — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the copy-forked scroll, selection, window and render code in
`internal/ui/messages.Model` and `internal/ui/thread.Model` with one substrate
(`internal/bubbles/entrylist`) and one message layer, without changing what
either pane does.

**Architecture:** Three layers (spec §3). `entrylist` is a domain-neutral list of
pre-rendered entries that owns offsets, scrolling, the visible window, text
selection, hit translation and the scrollbar. The message layer in
`internal/ui/messages` renders `MessageItem`s (one renderer with options) and owns
reactions. The two panes are thin owners of chrome, empty states, decorations and
lifecycle. Every behavioural difference between the panes is an `entrylist` or
renderer option, set to today's behaviour.

**Tech Stack:** Go 1.26.5, bubbletea v2, lipgloss v2 (`charm.land/*`),
`github.com/charmbracelet/x/ansi`, `github.com/rivo/uniseg`. Tests are stdlib
`testing` only.

**Spec:** [`../specs/2026-10-10-phase3-messages-thread-fork-design.md`](../specs/2026-10-10-phase3-messages-thread-fork-design.md).
Read it first. Section references below (§N) are to the spec.

## Global Constraints

- Behaviour-preserving. Every App golden in `internal/ui/testdata/golden/` stays
  **byte-identical** at every commit, except the single exception in §6.3:
  `thread_open.ansi`, `wide.ansi`, `narrow.ansi` and (decision P4)
  `thread_parent_selected.ansi` are re-blessed in PR 2+3 only, proven
  cell-identical.
- Gates before **every** commit, all green:
  `go build ./... && go vet ./... && gofmt -l . && golangci-lint run && go test ./... -race -count=1`
  (`gofmt -l .` must print nothing).
- golangci-lint **v2.14.0** (CI pins it in `.github/workflows/ci.yml`). The box
  this plan was written on has v2.13.1; Task 0.0 installs v2.14.0.
- `internal/ui` gains no I/O (`internal/ui/boundary_test.go`). `internal/bubbles/**`
  imports nothing under `internal/ui` and reads no `styles.*`
  (`TestBubblesReachTheAppOnlyThroughCore`).
- Found a bug? Record it, annotate it (`// BUG?:` characterization row) and raise
  it with the maintainer. Do not fix it in this plan.
- Every new shared helper goes into `AGENTS.md`'s shared-code tables **in the same
  commit**.
- Express any two implementations that must stay parallel as an interface with a
  compile-time assertion or a test, never a "keep in sync" comment.
- Test edits that only change *how* private state is reached are allowed (§8,
  budget 250). Changing what a test *asserts* is not, outside the PR 2+3 golden
  re-bless.
- No pushing, no PRs, no Phase 4.

## Review Focus

Inputs the spec implies but no ordinary unit test exercises, most likely to bite
first. Each has a pinning test in the task named.

1. **Theme switch while the thread is open.** The spacer rows and scrollbar must
   take the new theme on the next frame, as they do today. Pinned in Task 0.2
   (`TestThread_ThemeSwitchRestylesWindow`); the substrate's styles come from
   `messages.ThemeListStyles()` at the top of every `View`, which is what keeps
   it passing after PR 5.
2. **A thread with zero replies.** Drag-selecting the parent must still copy its
   text, and the "No replies yet" block must still scroll when the parent is
   taller than the pane. Pinned in Task 0.2 (`TestChar_ThreadZeroReplies*`).
3. **Wheel past the bottom of the thread, then wheel up.** Thread clamps eagerly;
   the channel pane clamps lazily (U6). Pinned in Task 0.2 and Task 0.3.
4. **Resizing the terminal during a drag selection.** The anchors survive and the
   copied text is unchanged in both panes. Pinned in Task 0.2 and Task 0.3
   (`TestChar_*SelectionSurvivesResize`).
5. **A reply arriving while the thread is scrolled up.** `AddReply` moves the
   cursor to the newest reply and the next frame snaps to it. Pinned in Task 0.2
   (`TestChar_ThreadNewReplySnapsToBottom`).

## Decisions this plan needs from the maintainer (sign off with the plan)

The plan found three things the spec did not settle. Each is resolved below; the
plan proceeds as written unless the maintainer says otherwise at review.

- **P1. Thread's zero-reply branch reads stale layout state.** In
  `thread.View`'s "No replies yet" branch (`thread/model.go:1537-1565`),
  `m.totalLines`, `m.entryOffsets` and `m.cache` are never updated; they keep
  whatever the *previous* normal frame left (possibly a different thread's).
  `absoluteLineAt` clamps mouse rows against that stale `m.totalLines`. Carrying
  stale state through a shared substrate is not reasonable. **Resolution:** in
  PR 5 the zero-reply branch calls `SetEntries` with the parent and the empty
  block, so the substrate's totals describe what is on screen. This changes
  behaviour only for a mouse click below the content of a zero-reply thread
  whose previous frame was a taller thread. Task 0.2 pins today's behaviour for
  the cases that matter (selecting the parent works); the stale-clamp edge is
  recorded as a bug, not preserved.
- **P2. A bug found while planning:** `thread.SetFocused` clears only the view
  cache, never the reply cache, so the selected reply keeps its **focused**
  border colour after focus leaves the thread, until something else rebuilds
  the cache. `messages.SetFocused` marks the selected entry stale and is
  correct. Verified on a throwaway copy of `afad298`: rendering after
  `SetFocused(true)` → `View` → `SetFocused(false)` → `View` is byte-identical to
  a focused render. **Resolution:** pinned as-is in Task 0.2 with a `// BUG?:`
  row. Raise it with the maintainer for an issue; do not fix it here. PR 5
  preserves it (the thread pane keeps not rebuilding replies on a focus flip).
- **P4. The golden exception covers four goldens, not three.** No existing
  golden selects the thread's parent, and PR 2+3 changes how the parent is
  padded. Task 0.5 adds `thread_parent_selected.ansi` *before* any change, so
  PR 2+3 re-blesses `thread_open`, `wide`, `narrow` **and**
  `thread_parent_selected`, all proven cell-identical against the pre-PR
  snapshot. This widens the exception's coverage; it does not loosen it.
- **P3. Three renderer differences missing from the spec's divergence list.**
  `renderThreadMessage` (`thread/model.go:2046`) also differs from
  `renderMessagePlain` (`messages/model.go:2021`) in that it draws **no
  `(edited)` mark**, **no `↳ replied to a thread` label** for
  `thread_broadcast` replies, and **no avatar** even though `thread` has a
  `SetAvatarFunc`. Its image-channel argument is the channel **ID** where
  `messages` passes the channel **name**, and it orders kitty flushes
  differently. **Resolution:** all of these become renderer options in Task 6.2,
  set to today's behaviour. Task 8.1 adds them to the spec as U7–U9.

Also corrected from the spec, without changing its intent:

- **Spec §5.2 U5 "channel requires an unchanged message count" is
  unobservable.** Every `messages` method that changes the message count also
  sets `m.cache = nil` (`AppendMessage:692`, `SwapLocalSent:721`,
  `RemoveLocalSent:744`, `UpsertSelfSent:780,791`, `RemoveMessageByTS:1173`,
  `PrependMessages:1011`, `SetMessages:660`), and a nil cache already disables
  re-anchoring. The same holds for thread (`AddReply` and friends all call
  `InvalidateCache`). The substrate therefore has no count gate. Task 8.1 adds a
  note to the spec.
- **Spec §5.3 said the thread parent becomes a cached entry.** To remove
  risk, PR 5 keeps re-rendering the parent on **every** `View`, exactly as
  today (`thread/model.go:1491`), and replaces entry 0 each frame. Caching it is
  a later performance PR. This removes the "parent invalidation triggers" risk
  outright.

## Before you start

- **Sequencing (§7.1):** start only after #304 (Part B) has merged to `main`.
  When it has, re-run the measurements in Task 0.0 step 3 and update the numbers
  this plan quotes if they moved.
- **Worktree:** use the `using-git-worktrees` skill. Do not touch the main
  checkout (`/home/dev/local_code/slk`) or anything under `.worktrees/`. One
  stacked branch per PR:
  `refactor/phase3/pr0-characterize` → `pr1-move-substrate` →
  `pr23-thread-viewport` → `pr4-entrylist` → `pr5-thread-entrylist` →
  `pr6-message-layer` → `pr7-app-pane` → `pr8-close-out`, each created from the
  previous one's tip.
- **Iterating:** run one package while you work
  (`go test ./internal/ui/thread -count=1`); run the full gate before each
  commit.
- **Line numbers** quoted below are at `afad298` plus #304. They drift; find code
  by the quoted identifiers.

## File structure

| File | PR | Responsibility |
|---|---|---|
| `internal/ui/goldencells_test.go` (new) | 0 | Cell-level comparison of two ANSI screens, and the env-driven proof that the current goldens are cell-identical to a baseline directory |
| `internal/ui/thread/phase3_char_test.go` (new) | 0 | Thread-pane characterization: scroll, clamp, drag `Version`, selection over decorations, zero-reply branch, overwide lines, the P2 bug pin |
| `internal/ui/messages/phase3_char_test.go` (new) | 0 | Channel-pane characterization: lazy clamp, cursor-follows-scroll, pin-on-shrink, more-below row, separator snapping, divider text in copies |
| `internal/ui/thread/lockstep_test.go` (modified in 0, deleted in 8) | 0, 5, 8 | Adds the scrolled-window parity test; tripwire deleted in PR 5; file deleted in PR 8 |
| `docs/superpowers/plans/phase3-baseline/` (new) | 0 | `bench.txt` (benchmark baseline), `refcount.go` and `shared.go` (measurement programs, `//go:build ignore`) |
| `internal/bubbles/scrollbar/`, `internal/bubbles/selection/` (moved) | 1 | Unchanged code, new import paths |
| `internal/ui/thread/model.go` | 2+3, 5, 6 | Loses `viewport`, the outer wrapper, scroll/selection math, cloned types, its renderer |
| `internal/bubbles/entrylist/plain.go` (new; code moved from `messages/render.go`) | 4 | `PlainLine` and helpers |
| `internal/bubbles/entrylist/entrylist.go` (new) | 4 | `Entry`, `Hit`, `Model`, options, `Styles`, offsets and scrolling |
| `internal/bubbles/entrylist/window.go` (new) | 4 | `Layout`, `Compose`, the visible window, indicator rows, hits |
| `internal/bubbles/entrylist/select.go` (new) | 4 | Anchors, drag selection, extraction, overlay, `ApplySelectionToBordered` |
| `internal/bubbles/entrylist/*_test.go` (new) | 4 | Substrate tests, one per option per setting |
| `internal/ui/messages/model.go` | 4, 6, 7 | Adopts `entrylist`; keeps rendering, payload, sixel/kitty |
| `internal/ui/messages/pane_list.go` (new) | 4 | `themeListStyles()`, which builds `entrylist.Styles` from `styles.*` (one place, used by both panes) |
| `internal/ui/messages/render_message.go` (new) | 6 | The one `MessageItem` renderer (`RenderMessage`) and `RenderOptions` |
| `internal/ui/messages/reactions.go` (new) | 6 | `ApplyReaction`, `ReactionNav` (shared reaction state and mutation) |
| `internal/ui/content_pane.go` (new) | 7 | `contentPane` interface, assertions, `App.focusedContentPane` |
| `internal/ui/app.go`, `drag.go`, `reducer_mouse.go`, `mode_normal.go`, `action_prompt.go` | 7 | Mirror pairs and arms collapsed |
| `AGENTS.md`, `docs/superpowers/plans/2026-09-06-architecture-refactor.md`, the spec | 1, 4, 6, 8 | Tables, status, lint version, spec addenda |

---

## PR 0 — Characterize (test-only)

Branch `refactor/phase3/pr0-characterize`. **No production `.go` file changes in
this PR.** Proof: `git diff --stat main... -- '*.go' ':!*_test.go'` prints nothing.
Every test added here passes on `main` as-is. These tests are the safety net for
PRs 1–7; a later task may change *how* one reaches private state, never what it
asserts.

### Task 0.0: Workspace, lint version, baseline measurements

**Files:**
- Create: `docs/superpowers/plans/phase3-baseline/bench.txt`
- Create: `docs/superpowers/plans/phase3-baseline/measurements.md`

- [ ] **Step 1: Create the worktree and branch**

Use the `using-git-worktrees` skill to create a worktree from `main` (after #304
merged) on branch `refactor/phase3/pr0-characterize`. Then:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
golangci-lint version   # must print 2.14.0
```

- [ ] **Step 2: Run the full gate once on the untouched tree**

```bash
go build ./... && go vet ./... && test -z "$(gofmt -l .)" && golangci-lint run && go test ./... -race -count=1
```

Expected: all green. If not, stop: the baseline itself is broken.

- [ ] **Step 3: Re-measure (spec Appendix A) and record**

Build the reference counter outside the repo (it needs `golang.org/x/tools`, which
the module does not require):

```bash
mkdir -p /tmp/opencode/refcount && cd /tmp/opencode/refcount
cat > go.mod <<'EOF'
module refcount

go 1.26

require golang.org/x/tools v0.48.0
EOF
```

Write `/tmp/opencode/refcount/main.go`:

```go
// Type-checked count of test-file references to unexported members of
// messages.Model, thread.Model and ui.App (spec Appendix A.1).
package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

type key struct{ typ, member string }

func main() {
	dir := os.Args[1]
	targets := map[string]string{
		"github.com/gammons/slk/internal/ui/messages.Model": "messages.Model",
		"github.com/gammons/slk/internal/ui/thread.Model":   "thread.Model",
		"github.com/gammons/slk/internal/ui.App":            "ui.App",
	}
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:   dir,
		Tests: true,
	}
	pkgs, err := packages.Load(cfg, "./internal/ui/...")
	if err != nil {
		panic(err)
	}
	counts := map[key]int{}
	seen := map[token.Position]bool{}
	named := func(t types.Type) string {
		if p, ok := t.(*types.Pointer); ok {
			t = p.Elem()
		}
		n, ok := t.(*types.Named)
		if !ok || n.Obj().Pkg() == nil {
			return ""
		}
		return targets[n.Obj().Pkg().Path()+"."+n.Obj().Name()]
	}
	record := func(fset *token.FileSet, pos token.Pos, label, member string) {
		p := fset.Position(pos)
		if seen[p] {
			return
		}
		seen[p] = true
		counts[key{label, member}]++
	}
	for _, pkg := range pkgs {
		for _, f := range pkg.Syntax {
			if !strings.HasSuffix(pkg.Fset.Position(f.Pos()).Filename, "_test.go") {
				continue
			}
			ast.Inspect(f, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.SelectorExpr:
					sel, ok := pkg.TypesInfo.Selections[x]
					if !ok || ast.IsExported(x.Sel.Name) {
						return true
					}
					if label := named(sel.Recv()); label != "" {
						record(pkg.Fset, x.Sel.Pos(), label, x.Sel.Name)
					}
				case *ast.CompositeLit:
					tv, ok := pkg.TypesInfo.Types[x]
					if !ok {
						return true
					}
					label := named(tv.Type)
					if label == "" {
						return true
					}
					for _, e := range x.Elts {
						if kv, ok := e.(*ast.KeyValueExpr); ok {
							if id, ok := kv.Key.(*ast.Ident); ok && !ast.IsExported(id.Name) {
								record(pkg.Fset, id.Pos(), label, id.Name)
							}
						}
					}
				}
				return true
			})
		}
	}
	tot := map[string]int{}
	byLabel := map[string][]key{}
	for k, n := range counts {
		tot[k.typ] += n
		byLabel[k.typ] = append(byLabel[k.typ], k)
	}
	for _, label := range []string{"messages.Model", "thread.Model", "ui.App"} {
		ks := byLabel[label]
		sort.Slice(ks, func(i, j int) bool { return counts[ks[i]] > counts[ks[j]] })
		fmt.Printf("== %s: %d refs, %d distinct members\n", label, tot[label], len(ks))
		if label == "ui.App" {
			for _, m := range []string{"messagepane", "threadPanel", "winModels"} {
				fmt.Printf("   %-22s %d\n", m, counts[key{label, m}])
			}
			continue
		}
		for _, k := range ks {
			fmt.Printf("   %-22s %d\n", k.member, counts[k])
		}
	}
}
```

Run it and the shared-line count:

```bash
cd /tmp/opencode/refcount && go mod tidy && go run . "$WORKTREE"
cd "$WORKTREE"
python3 - <<'EOF'
def lines(p):
    return open(p).read().split('\n')
def keep(ls, n):
    return [l.strip() for l in ls if l.strip() and not l.strip().startswith('//') and len(l.strip()) >= n]
m = set(keep(lines('internal/ui/messages/model.go') + lines('internal/ui/messages/render.go'), 0))
t = keep(lines('internal/ui/thread/model.go'), 16)
print('shared(>=16):', sum(1 for x in t if x in m), 'of', len(t))
EOF
wc -l internal/ui/messages/model.go internal/ui/thread/model.go internal/ui/app.go
```

(`$WORKTREE` is the worktree's absolute path.) Expected, give or take #304's
small additions: `messages.Model` ≈159 refs, `thread.Model` ≈136, shared ≈491.
Write `docs/superpowers/plans/phase3-baseline/measurements.md` with the date, the
commit (`git rev-parse --short HEAD`) and these outputs pasted verbatim in fenced
blocks.

- [ ] **Step 4: Record the benchmark baseline**

```bash
go test ./internal/ui/messages -run '^$' -bench 'BenchmarkViewScroll|BenchmarkBuildCache|BenchmarkViewFocusFlip' -benchmem -count=10 > /tmp/opencode/bench-msgs.txt
go test ./internal/ui/thread   -run '^$' -bench 'BenchmarkViewScroll' -benchmem -count=10 > /tmp/opencode/bench-thread.txt
cat /tmp/opencode/bench-msgs.txt /tmp/opencode/bench-thread.txt > docs/superpowers/plans/phase3-baseline/bench.txt
```

Every later "benches within ±10%" check runs the same two commands into new files
and compares with `benchstat` (`go install golang.org/x/perf/cmd/benchstat@latest`;
if the network is unavailable, compare medians by hand and paste both).

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/plans/phase3-baseline
git commit -m "docs(phase3): record the measurement and benchmark baseline"
```

### Task 0.1: Cell-level golden comparison

The one golden exception (PR 2+3) must be **proven** cell-identical. This adds the
comparison, a self-test that it catches a single changed cell, and an opt-in test
that compares the current goldens against a baseline directory.

**Files:**
- Create: `internal/ui/goldencells_test.go`

**Interfaces:**
- Produces: `cellDiff(want, got string) string` (`""` when every cell matches;
  otherwise a description of the first differing cell);
  `TestGoldenCellsMatchBaseline`, driven by `SLK_GOLDEN_BASELINE=<dir>`.

- [ ] **Step 1: Write the self-tests (they fail: `cellDiff` is undefined)**

```go
package ui

import (
	"os"
	"path/filepath"
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
```

- [ ] **Step 2: Run them and see them fail**

Run: `go test ./internal/ui -run TestCellDiff -count=1`
Expected: FAIL to compile, `undefined: cellDiff`.

- [ ] **Step 3: Implement the comparison and the baseline test (same file, after the imports)**

```go
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
```

The file's import block is `os`, `path/filepath`, `slices`, `strconv`,
`strings`, `testing`, `unicode/utf8` (update the one in step 1 to match).

- [ ] **Step 4: Run the self-tests and a baseline run against itself**

```bash
go test ./internal/ui -run 'TestCellDiff|TestGoldenCellsMatchBaseline' -count=1 -v
mkdir -p /tmp/opencode/golden-self && cp internal/ui/testdata/golden/*.ansi /tmp/opencode/golden-self/
SLK_GOLDEN_BASELINE=/tmp/opencode/golden-self go test ./internal/ui -run TestGoldenCellsMatchBaseline -count=1 -v
```

Expected: all `TestCellDiff_*` PASS; the baseline test SKIPs in the first command
and PASSes in the second, with no "bytes differ" lines.

- [ ] **Step 5: Gate and commit**

Run the full gate. Then add a row to `AGENTS.md`'s **Test helpers** table:

| Need | Use |
|---|---|
| Prove two ANSI screens look identical even when their bytes differ (re-encoded SGR), or that every golden is cell-identical to a baseline directory | `cellDiff(want, got)` and `SLK_GOLDEN_BASELINE=<dir> go test ./internal/ui -run TestGoldenCellsMatchBaseline` (`internal/ui/goldencells_test.go`) |

```bash
git add internal/ui/goldencells_test.go AGENTS.md
git commit -m "test(ui): compare goldens cell by cell, not byte by byte"
```

### Task 0.2: Thread-pane characterization

**Files:**
- Create: `internal/ui/thread/phase3_char_test.go`

**Interfaces:**
- Consumes: `markedReplies(n)` (`thread/model_test.go`, same package): replies
  whose text starts `markNNN` followed by 30 words, TS `1700000000+i`, author
  `bob`, `Timestamp: "10:30 AM"`.
- Produces: helpers `p3Thread(t, n) *Model`, `p3Rows(view) []string`,
  `p3RowWith(t, rows, anchor) int` used only in this file.

These tests pin today's behaviour through the exported API wherever possible, so
they survive the refactor unchanged. Two of them read unexported state
(`m.viewContent`, `m.chromeHeight`); `viewContent` moves in PR 5, which rewrites
that one line.

- [ ] **Step 1: Write the file**

```go
package thread

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/styles"
)

// Phase 3 characterization (plan Task 0.2). Each test pins one behaviour
// the shared substrate must preserve for the thread pane; the comment
// names the spec item (§5) it pins.

func p3Thread(t *testing.T, n int) *Model {
	t.Helper()
	styles.Apply("dark", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	m := New()
	m.SetThread(
		messages.MessageItem{TS: "100.0", UserName: "alice", Text: "parent", Timestamp: "9:00 AM"},
		markedReplies(n), "C1", "100.0",
	)
	return m
}

func p3Rows(view string) []string {
	rows := strings.Split(view, "\n")
	for i := range rows {
		rows[i] = ansi.Strip(rows[i])
	}
	return rows
}

// p3RowWith returns the first row containing anchor, failing if none does.
func p3RowWith(t *testing.T, rows []string, anchor string) int {
	t.Helper()
	for i, r := range rows {
		if strings.Contains(r, anchor) {
			return i
		}
	}
	t.Fatalf("no row contains %q:\n%s", anchor, strings.Join(rows, "\n"))
	return -1
}

// U6: thread clamps the scroll offset eagerly (bubbles/viewport today), so
// scrolling past the bottom is a no-op and a following ScrollUp(1) moves
// one line up from the bottom.
func TestChar_ThreadScrollClampsEagerly(t *testing.T) {
	a := p3Thread(t, 40)
	bottom := a.View(20, 80)
	a.ScrollDown(50)
	a.ScrollUp(1)
	got := a.View(20, 80)

	b := p3Thread(t, 40)
	_ = b.View(20, 80)
	b.ScrollUp(1)
	want := b.View(20, 80)

	if want == bottom {
		t.Fatal("test setup: ScrollUp(1) from the bottom did not move the view")
	}
	if got != want {
		t.Error("ScrollDown past the bottom was not a no-op (thread clamps eagerly, U6)")
	}
}

// U6: with no content yet, scrolling is a no-op but still bumps Version.
func TestChar_ThreadScrollWithoutContentStillDirties(t *testing.T) {
	m := New()
	v := m.Version()
	m.ScrollUp(3)
	m.ScrollDown(3)
	if got := m.Version(); got != v+2 {
		t.Errorf("Version after two scrolls on an empty thread = %d, want %d", got, v+2)
	}
}

// U1 off: scrolling the selected reply off-screen leaves the cursor alone.
func TestChar_ThreadDoesNotFollowCursorOnScroll(t *testing.T) {
	m := p3Thread(t, 40)
	_ = m.View(20, 80)
	before := m.SelectedReply().TS
	m.ScrollUp(200)
	_ = m.View(20, 80)
	if got := m.SelectedReply().TS; got != before {
		t.Errorf("selection followed the scroll: %s -> %s (thread must not, U1)", before, got)
	}
}

// U2 off: a shorter pane keeps the same top row (no bottom pin).
func TestChar_ThreadDoesNotPinBottomOnShrink(t *testing.T) {
	m := p3Thread(t, 40)
	before := p3Rows(m.View(20, 80))
	after := p3Rows(m.View(17, 80))
	top := m.chromeHeight
	if before[top] != after[top] {
		t.Errorf("shrinking moved the top content row (thread must not pin, U2):\n before %q\n after  %q",
			before[top], after[top])
	}
}

// U3 off: no "-- more below --" row, however far up the pane is scrolled.
func TestChar_ThreadHasNoMoreBelowRow(t *testing.T) {
	m := p3Thread(t, 40)
	_ = m.View(20, 80)
	m.ScrollUp(30)
	if v := strings.Join(p3Rows(m.View(20, 80)), "\n"); strings.Contains(v, "more below") {
		t.Errorf("thread drew a more-below row (U3):\n%s", v)
	}
}

// U4 on: every drag step that changes the selection bumps Version.
func TestChar_ThreadDragBumpsVersion(t *testing.T) {
	m := p3Thread(t, 3)
	rows := p3Rows(m.View(40, 80))
	r := p3RowWith(t, rows, "mark001")
	v0 := m.Version()
	m.BeginSelectionAt(r, 3)
	v1 := m.Version()
	m.ExtendSelectionAt(r, 10)
	v2 := m.Version()
	m.ExtendSelectionAt(r, 10)
	v3 := m.Version()
	if _, ok := m.EndSelection(); !ok {
		t.Fatal("test setup: the drag selected nothing")
	}
	v4 := m.Version()
	if !(v1 > v0 && v2 > v1 && v3 == v2 && v4 > v3) {
		t.Errorf("Version over a drag = %d,%d,%d,%d,%d; want begin+, extend+, no-op extend =, end+ (U4)",
			v0, v1, v2, v3, v4)
	}
}

// 8(c) off: a press on a separator rule anchors nothing.
func TestChar_ThreadSelectionSkipsSeparators(t *testing.T) {
	m := p3Thread(t, 3)
	rows := p3Rows(m.View(40, 80))
	rule := -1
	for i := m.chromeHeight; i < len(rows); i++ {
		if r := strings.TrimSpace(rows[i]); r != "" && strings.Trim(r, "─") == "" {
			rule = i
			break
		}
	}
	if rule < 0 {
		t.Fatalf("test setup: no separator rule found:\n%s", strings.Join(rows, "\n"))
	}
	m.BeginSelectionAt(rule, 3)
	if m.HasSelection() {
		t.Error("a press on a separator started a selection (thread must not snap, 8c)")
	}
}

// 8(c) off: copied text never contains a separator rule.
func TestChar_ThreadCopyExcludesSeparators(t *testing.T) {
	m := p3Thread(t, 3)
	rows := p3Rows(m.View(40, 80))
	from := p3RowWith(t, rows, "mark000")
	to := p3RowWith(t, rows, "mark001")
	m.BeginSelectionAt(from, 3)
	m.ExtendSelectionAt(to, 8)
	text, ok := m.EndSelection()
	if !ok || !strings.HasPrefix(text, "rk000") {
		t.Fatalf("test setup: copy = %q, ok=%v", text, ok)
	}
	if strings.Contains(text, "─") {
		t.Errorf("copy contains a separator rule:\n%s", text)
	}
}

// Review focus 5: a new reply while scrolled up snaps to the bottom.
func TestChar_ThreadNewReplySnapsToBottom(t *testing.T) {
	m := p3Thread(t, 40)
	_ = m.View(20, 80)
	m.ScrollUp(100)
	_ = m.View(20, 80)
	m.AddReply(messages.MessageItem{TS: "1800000000.000000", UserName: "carol", Text: "mark999 fresh", Timestamp: "11:00 AM"})
	if v := strings.Join(p3Rows(m.View(20, 80)), "\n"); !strings.Contains(v, "mark999") {
		t.Errorf("new reply not on screen after AddReply:\n%s", v)
	}
}

// Review focus 4: a resize during a selection keeps it, and the copy
// still starts where the drag started.
func TestChar_ThreadSelectionSurvivesResize(t *testing.T) {
	m := p3Thread(t, 3)
	rows := p3Rows(m.View(40, 80))
	m.BeginSelectionAt(p3RowWith(t, rows, "mark000"), 3)
	m.ExtendSelectionAt(p3RowWith(t, rows, "mark001"), 8)
	_ = m.View(40, 60)
	if !m.HasSelection() {
		t.Fatal("resize dropped the selection")
	}
	if got := m.SelectionText(); !strings.HasPrefix(got, "rk000") {
		t.Errorf("after resize the copy = %q, want prefix %q", got, "rk000")
	}
}

// Review focus 2: in a zero-reply thread the parent is selectable.
func TestChar_ThreadZeroRepliesParentSelectable(t *testing.T) {
	styles.Apply("dark", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	m := New()
	m.SetThread(messages.MessageItem{TS: "100.0", UserName: "alice", Text: "zero-reply parent text", Timestamp: "9:00 AM"},
		nil, "C1", "100.0")
	rows := p3Rows(m.View(20, 60))
	r := p3RowWith(t, rows, "zero-reply")
	m.BeginSelectionAt(r, 3)
	m.ExtendSelectionAt(r, 12)
	if text, ok := m.EndSelection(); !ok || text != "ro-reply" {
		t.Errorf("copy from a zero-reply parent = %q, ok=%v; want %q", text, ok, "ro-reply")
	}
}

// Review focus 2: a parent taller than the pane still scrolls with no replies.
func TestChar_ThreadZeroRepliesTallParentScrolls(t *testing.T) {
	styles.Apply("dark", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	m := New()
	m.SetThread(messages.MessageItem{TS: "100.0", UserName: "alice", Text: strings.Repeat("word ", 200), Timestamp: "9:00 AM"},
		nil, "C1", "100.0")
	before := m.View(10, 30)
	m.ScrollDown(3)
	if after := m.View(10, 30); after == before {
		t.Error("a tall zero-reply parent did not scroll")
	}
}

// §2 item 2: no content line is ever wider than the pane, so dropping
// viewport's ansi.Cut (PR 2+3) cannot change output. If this fails, stop
// PR 2+3: the Cut matters.
func TestChar_ThreadOverwideLinesNeverExceedPane(t *testing.T) {
	bodies := []string{
		"☺ ❤ ☀ ✔ ✈ ♥ ⚠ ↔ ™ ©", "🇺🇸🏳️‍🌈👩‍👩‍👧 x", "```\n" + strings.Repeat("abcdefghij", 20) + "\n```",
		strings.Repeat("ｗ", 50), "e\u0301\u0301\u0301 zalgo", "\t\ttabs\there",
		"a" + strings.Repeat("\u200b", 40) + "b", "<https://example.com/" + strings.Repeat("x", 120) + "|link>",
		strings.Repeat("🎉", 30),
	}
	for _, body := range bodies {
		m := p3Thread(t, 0)
		m.SetThread(messages.MessageItem{TS: "1.0", UserName: "p", Text: "parent", Timestamp: "9:00"},
			[]messages.MessageItem{{TS: "2.0", UserName: "u", Text: body, Timestamp: "9:01"}}, "C", "1.0")
		_ = m.View(30, 40)
		for _, l := range strings.Split(m.viewContent, "\n") {
			if w := ansi.StringWidth(l); w > 40 {
				t.Errorf("body %q: a content line is %d columns in a 40-column pane", body, w)
			}
		}
	}
}

// BUG?: plan decision P2. thread.SetFocused clears only the view cache, so
// the selected reply keeps its focused border after focus leaves. Pinned
// as-is; when fixed, re-pin to "flipped == freshly unfocused render".
func TestChar_ThreadFocusFlipKeepsFocusedBorder(t *testing.T) {
	mk := func() *Model { return p3Thread(t, 1) }
	a := mk()
	a.SetFocused(true)
	_ = a.View(20, 40)
	a.SetFocused(false)
	flipped := a.View(20, 40)

	c := mk()
	c.SetFocused(true)
	if focused := c.View(20, 40); flipped != focused {
		t.Error("the stale-border behaviour changed; if intended, re-pin this BUG? row and raise it")
	}
}

// Between an invalidation (here SelectByIndex) and the next View, the
// reply cache is gone: lookups see only the parent, whose entry is
// re-rendered every frame. The substrate keeps this via KeepLookups(1).
func TestChar_ThreadLookupsSeeOnlyTheParentUntilRebuilt(t *testing.T) {
	m := p3Thread(t, 2)
	rows := p3Rows(m.View(40, 80))
	reply := p3RowWith(t, rows, "mark001")
	parent := p3RowWith(t, rows, "parent")
	m.SelectByIndex(0) // the cursor was on reply 1, so this invalidates
	m.ClickAt(reply)
	if got := m.SelectedReply(); got == nil || got.TS != markedReplies(2)[0].TS {
		t.Errorf("a click on a discarded reply moved the cursor to %+v", got)
	}
	m.ClickAt(parent)
	if got := m.SelectedReply(); got == nil || got.TS != "100.0" {
		t.Errorf("a click on the parent did not select it: %+v", got)
	}
}

// Review focus 1: a theme switch reaches the window padding and the
// scrollbar on the next frame (App.applyTheme invalidates the pane).
func TestThread_ThemeSwitchRestylesWindow(t *testing.T) {
	m := p3Thread(t, 1)
	before := strings.Split(m.View(30, 60), "\n")
	styles.Apply("light", config.Theme{}) // p3Thread's cleanup restores dark
	m.InvalidateCache()                   // what App.applyTheme does
	after := strings.Split(m.View(30, 60), "\n")
	last := len(after) - 1
	if ansi.Strip(before[last]) != ansi.Strip(after[last]) {
		t.Fatalf("test setup: the padding row's text changed: %q -> %q", ansi.Strip(before[last]), ansi.Strip(after[last]))
	}
	if before[last] == after[last] {
		t.Error("the padding row kept the old theme's background after a theme switch")
	}
}

// Thread clamps a click below the content to the last line, so it
// selects the last reply (messages' ClickAt misses instead).
func TestChar_ThreadClickBelowContentSelectsLastReply(t *testing.T) {
	m := p3Thread(t, 2)
	_ = m.View(40, 80)
	m.SelectByIndex(0)
	_ = m.View(40, 80) // SelectByIndex invalidates the reply cache; ClickAt reads it
	m.ClickAt(39)
	if got := m.SelectedReply(); got == nil || got.TS != markedReplies(2)[1].TS {
		t.Errorf("click below the content selected %+v, want the last reply", got)
	}
}
```

- [ ] **Step 2: Run them**

Run: `go test ./internal/ui/thread -run TestChar_ -count=1 -v`
Expected: every test PASSES on the untouched tree. A failure here means the test
is wrong about today's behaviour; fix the test, never production code, and note
what you learned in the commit message.

- [ ] **Step 3: Gate and commit**

```bash
git add internal/ui/thread/phase3_char_test.go
git commit -m "test(thread): characterize scroll, selection and empty states for Phase 3"
```

### Task 0.3: Channel-pane characterization

**Files:**
- Create: `internal/ui/messages/phase3_char_test.go`

**Interfaces:**
- Produces: `p3Messages(t, n) *Model`, `p3MsgRows(view) []string`,
  `p3MsgRowWith(t, rows, anchor) int`, used only in this file.

- [ ] **Step 1: Write the file**

```go
package messages

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/styles"
)

// Phase 3 characterization (plan Task 0.3): the channel pane's side of
// every divergence the shared substrate turns into an option.

func p3Messages(t *testing.T, n int, dayStep int64) *Model {
	t.Helper()
	styles.Apply("dark", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	items := make([]MessageItem, n)
	for i := range items {
		items[i] = MessageItem{
			TS:        fmt.Sprintf("%d.000000", 1700000000+int64(i)*dayStep+int64(i)),
			UserName:  "bob",
			Text:      fmt.Sprintf("mark%03d %s", i, strings.Repeat("word ", 30)),
			Timestamp: "10:30 AM",
		}
	}
	m := New(items, "general")
	return &m
}

func p3MsgRows(view string) []string {
	rows := strings.Split(view, "\n")
	for i := range rows {
		rows[i] = ansi.Strip(rows[i])
	}
	return rows
}

func p3MsgRowWith(t *testing.T, rows []string, anchor string) int {
	t.Helper()
	for i, r := range rows {
		if strings.Contains(r, anchor) {
			return i
		}
	}
	t.Fatalf("no row contains %q:\n%s", anchor, strings.Join(rows, "\n"))
	return -1
}

// U6: the channel pane clamps lazily, in View, so an overshoot below the
// bottom absorbs a following ScrollUp(1).
func TestChar_MessagesScrollClampsLazily(t *testing.T) {
	a := p3Messages(t, 40, 0)
	bottom := a.View(20, 80)
	a.ScrollDown(50)
	a.ScrollUp(1)
	if got := a.View(20, 80); got != bottom {
		t.Error("ScrollDown(50)+ScrollUp(1) left the bottom (channel pane clamps lazily, U6)")
	}
}

// U1 on: scrolling the selection off-screen drags it onto the screen.
func TestChar_MessagesCursorFollowsScroll(t *testing.T) {
	m := p3Messages(t, 40, 0)
	_ = m.View(20, 80)
	before := m.SelectedIndex()
	m.ScrollUp(200)
	_ = m.View(20, 80)
	if m.SelectedIndex() >= before {
		t.Errorf("selection did not follow the scroll: %d -> %d (U1)", before, m.SelectedIndex())
	}
}

// U2 on: a bottom-anchored pane that shrinks keeps its bottom pinned.
func TestChar_MessagesPinsBottomOnShrink(t *testing.T) {
	m := p3Messages(t, 40, 0)
	_ = m.View(20, 80)
	y0 := m.YOffset()
	_ = m.View(17, 80)
	if got := m.YOffset(); got != y0+3 {
		t.Errorf("YOffset after shrinking by 3 = %d, want %d (U2)", got, y0+3)
	}
}

// U3 on: a pane with content below draws "-- more below --" last.
func TestChar_MessagesMoreBelowRow(t *testing.T) {
	m := p3Messages(t, 40, 0)
	_ = m.View(20, 80)
	m.ScrollUp(30)
	rows := p3MsgRows(m.View(20, 80))
	if !strings.Contains(rows[len(rows)-1], "-- more below --") {
		t.Errorf("last row = %q, want the more-below indicator (U3)", rows[len(rows)-1])
	}
}

// U4 off: only the first press of a drag bumps Version.
func TestChar_MessagesDragBumpsVersionOnce(t *testing.T) {
	m := p3Messages(t, 3, 0)
	rows := p3MsgRows(m.View(40, 80))
	r := p3MsgRowWith(t, rows, "mark001")
	v0 := m.Version()
	m.BeginSelectionAt(r, 3)
	v1 := m.Version()
	m.ExtendSelectionAt(r, 10)
	v2 := m.Version()
	if _, ok := m.EndSelection(); !ok {
		t.Fatal("test setup: the drag selected nothing")
	}
	v3 := m.Version()
	if !(v1 == v0+1 && v2 == v1 && v3 == v2) {
		t.Errorf("Version over a drag = %d,%d,%d,%d; want +1 on begin only (U4)", v0, v1, v2, v3)
	}
}

// 8(c) on: a press on a day divider snaps to the next message.
func TestChar_MessagesSelectionSnapsFromDivider(t *testing.T) {
	m := p3Messages(t, 3, 0)
	rows := p3MsgRows(m.View(40, 80))
	div := -1
	for i := m.ChromeHeight(); i < len(rows); i++ {
		if strings.Contains(rows[i], "── ") && !strings.Contains(rows[i], "▌") {
			div = i
			break
		}
	}
	if div < 0 {
		t.Fatalf("test setup: no day divider:\n%s", strings.Join(rows, "\n"))
	}
	m.BeginSelectionAt(div, 3)
	if !m.HasSelection() {
		t.Error("a press on a day divider did not snap to a message (8c)")
	}
}

// 8(c) on: a copy across a day boundary includes the divider's text.
func TestChar_MessagesCopyIncludesDividerText(t *testing.T) {
	m := p3Messages(t, 2, 2*86400)
	rows := p3MsgRows(m.View(40, 80))
	m.BeginSelectionAt(p3MsgRowWith(t, rows, "mark000"), 3)
	m.ExtendSelectionAt(p3MsgRowWith(t, rows, "mark001"), 8)
	text, ok := m.EndSelection()
	if !ok || !strings.Contains(text, "──") {
		t.Errorf("copy across a day boundary = %q (ok=%v), want the divider text in it", text, ok)
	}
}

// Review focus 4: a resize during a selection keeps it.
func TestChar_MessagesSelectionSurvivesResize(t *testing.T) {
	m := p3Messages(t, 3, 0)
	rows := p3MsgRows(m.View(40, 80))
	m.BeginSelectionAt(p3MsgRowWith(t, rows, "mark000"), 3)
	m.ExtendSelectionAt(p3MsgRowWith(t, rows, "mark001"), 8)
	_ = m.View(40, 60)
	if !m.HasSelection() {
		t.Fatal("resize dropped the selection")
	}
	if got := m.SelectionText(); !strings.HasPrefix(got, "rk000") {
		t.Errorf("after resize the copy = %q, want prefix %q", got, "rk000")
	}
}

// Between a cache discard (here AppendMessage) and the next View, clicks
// and selections find no message. The substrate keeps this via
// KeepLookups(0).
func TestChar_MessagesLookupsMissUntilRebuilt(t *testing.T) {
	m := p3Messages(t, 3, 0)
	rows := p3MsgRows(m.View(40, 80))
	r := p3MsgRowWith(t, rows, "mark000")
	m.AppendMessage(MessageItem{TS: "1800000000.000000", UserName: "carol", Text: "fresh", Timestamp: "11:00 AM"})
	if m.ClickAt(r) {
		t.Error("ClickAt hit a message while the render cache was discarded")
	}
	m.BeginSelectionAt(r, 3)
	if m.HasSelection() {
		t.Error("a selection started while the render cache was discarded")
	}
}

// The channel pane's ClickAt misses below the content (thread's selects
// the last reply).
func TestChar_MessagesClickBelowContentMisses(t *testing.T) {
	m := p3Messages(t, 2, 0)
	_ = m.View(40, 80)
	m.SelectByIndex(0)
	if m.ClickAt(39) || m.SelectedIndex() != 0 {
		t.Errorf("click below the content hit (selected=%d)", m.SelectedIndex())
	}
}
```

- [ ] **Step 2: Run them**

Run: `go test ./internal/ui/messages -run TestChar_ -count=1 -v`
Expected: all PASS on the untouched tree. Same rule as Task 0.2 step 2.

Note on `TestChar_MessagesCopyIncludesDividerText`: the first message is at
`1700000000`, the second two days later, so a day divider sits between them in
any time zone. Run the package once under `TZ=Pacific/Kiritimati` and once under
`TZ=America/Anchorage` to confirm.

- [ ] **Step 3: Gate and commit**

```bash
git add internal/ui/messages/phase3_char_test.go
git commit -m "test(messages): characterize scroll, selection and indicators for Phase 3"
```

### Task 0.4: Scrolled-window parity across panes

`lockstep_test.go` compares the panes in one static state. This adds the
comparison in a **scrolled** state: after both panes are scrolled so the same
body row is on top, every body row below it is at the same row with the same
text, and a drag over the same rows copies the same text once blank lines are
ignored (the channel pane's spacer contributes an empty line; thread's rule is
not part of any entry).

**Files:**
- Modify: `internal/ui/thread/lockstep_test.go`

- [ ] **Step 1: Extract the global setup and add the test**

Move the first block of `lockstepRender` (the `styles.Apply` /
`emojiutil.SetImageMode` / `messages.SetNowFunc` setup and its `t.Cleanup`) into:

```go
// lockstepGlobals pins the process-global render inputs both panes read.
func lockstepGlobals(t *testing.T) {
	t.Helper()
	styles.Apply("dark", config.Theme{})
	emojiutil.SetImageMode(false, 2)
	messages.SetNowFunc(lockstepClock)
	t.Cleanup(func() {
		styles.Apply("dark", config.Theme{})
		emojiutil.SetImageMode(false, 2)
		messages.SetNowFunc(nil)
	})
}
```

and call `lockstepGlobals(t)` at the top of `lockstepRender`. Then append:

```go
// TestLockstep_ScrolledWindowParity compares the panes while scrolled:
// the window rows, and the text a drag copies, must agree on every body
// row. Header rows legitimately differ (the channel pane's cursor follows
// the scroll, U1, and the selected header shows a long timestamp), as do
// the rows between messages (spacer vs rule) and the channel pane's last
// row (more-below, U3), so only body rows are compared.
func TestLockstep_ScrolledWindowParity(t *testing.T) {
	lockstepGlobals(t)
	const h = 10
	items := make([]messages.MessageItem, 14)
	for i := range items {
		items[i] = messages.MessageItem{
			TS: lockstepTS(time.Duration(i) * time.Minute), UserID: fmt.Sprintf("U%02d", i),
			UserName: fmt.Sprintf("u%02d", i), Text: fmt.Sprintf("body-%02d", i),
			Timestamp: fmt.Sprintf("9:%02d AM", i),
		}
	}
	mv := messages.New(items, "general")
	mm := &mv
	mm.SetFocused(true)
	tm := New()
	tm.SetThread(items[0], items[1:], "C1", items[0].TS)
	tm.SetFocused(true)

	align := func(name string, view func() string, chrome func() int, up func(int)) []string {
		for range 200 {
			rows := lockstepStrip(view())
			if c := rows[chrome():]; strings.Contains(c[0], "body-06") {
				return c[:len(c)-1] // drop the last row: more-below in messages
			}
			up(1)
		}
		t.Fatalf("%s: body-06 never reached the top row", name)
		return nil
	}
	msgRows := align("messages", func() string { return mm.View(h, lockstepWidth) }, mm.ChromeHeight, mm.ScrollUp)
	thrRows := align("thread", func() string { return tm.View(h, lockstepWidth) }, func() int { return tm.chromeHeight }, tm.ScrollUp)

	bodies := func(rows []string) []string {
		var out []string
		for i, r := range rows {
			if strings.Contains(r, "body-") {
				out = append(out, fmt.Sprintf("%d:%s", i, r))
			}
		}
		return out
	}
	if mb, tb := bodies(msgRows), bodies(thrRows); !slices.Equal(mb, tb) {
		t.Errorf("scrolled body rows differ:\n messages %q\n thread   %q", mb, tb)
	}

	drag := func(begin, extend func(int, int), end func() (string, bool), chrome int) string {
		begin(chrome+0, 2)
		extend(chrome+3, 9)
		text, _ := end()
		var kept []string
		for _, l := range strings.Split(text, "\n") {
			if l != "" {
				kept = append(kept, l)
			}
		}
		return strings.Join(kept, "\n")
	}
	mt := drag(mm.BeginSelectionAt, mm.ExtendSelectionAt, mm.EndSelection, mm.ChromeHeight())
	tt := drag(tm.BeginSelectionAt, tm.ExtendSelectionAt, tm.EndSelection, tm.chromeHeight)
	if mt == "" || mt != tt {
		t.Errorf("scrolled drag copies differ:\n messages %q\n thread   %q", mt, tt)
	}
}
```

Add `fmt` and `time` to the imports if they are not already there (`slices`,
`strings`, `time` are; check `fmt`).

- [ ] **Step 2: Run the lockstep tests**

Run: `go test ./internal/ui/thread -run TestLockstep -count=1 -v`
Expected: all three PASS.

- [ ] **Step 3: Gate and commit**

```bash
git add internal/ui/thread/lockstep_test.go
git commit -m "test(thread): compare the panes while scrolled, not only at rest"
```

### Task 0.5: A golden for the selected thread parent

No golden selects the thread's parent row, so PR 2+3's change to how the parent is
padded (Task 2.2) would be proven cell-identical only for the unselected parent.
This adds the missing scenario **before** anything changes, so PR 2+3 can prove it
too (plan decision P4).

**Files:**
- Modify: `internal/ui/golden_test.go` (`goldenScenarios`)
- Create: `internal/ui/testdata/golden/thread_parent_selected.ansi`

- [ ] **Step 1: Add the scenario after `thread_open`**

```go
		{
			// Phase 3 plan Task 0.5: the cursor on the thread's parent.
			// Pins the selected parent's tint and its padding to the pane
			// width, which Phase 3 PR 2+3 moves from an outer wrapper to
			// the parent row itself.
			name: "thread_parent_selected", w: 120, h: 30, layout: goldenStackedThread,
			build: func(t *testing.T) *App {
				a := goldenThreadApp(t, 120, 30)
				a.focusedPanel = PanelThread
				a.threadPanel.GoToTop() // reply 0
				a.threadPanel.MoveUp()  // the parent (index -1)
				_ = a.View()
				return a
			},
		},
```

- [ ] **Step 2: Bless it and check it shows what it claims**

```bash
go test ./internal/ui -run 'TestGolden$/thread_parent_selected' -update -count=1
go test ./internal/ui -run 'TestGolden' -count=1
```

Expected: the file is created; the second run passes, including
`TestGolden_ScenarioLayouts/thread_parent_selected`. Then open the file with
`cat -v` or `less -R` and confirm that the parent row (the one after the
breadcrumb rule, containing the parent's text) carries the selection tint and the
thick `▌`. A golden that silently selects a reply instead would pin the wrong
thing.

- [ ] **Step 3: Gate and commit**

```bash
git add internal/ui/golden_test.go internal/ui/testdata/golden/thread_parent_selected.ansi
git commit -m "test(ui): golden for the thread with its parent selected"
```

PR 0 is complete. Proof for the PR description:
`git diff --stat main... -- '*.go' ':!*_test.go'` is empty.

---

## PR 1 — Move `scrollbar` and `selection` to `internal/bubbles`

Branch `refactor/phase3/pr1-move-substrate`, from PR 0's tip. Pure motion: no
file's contents change except import paths and `AGENTS.md`.

### Task 1.1: `git mv` both packages and rewrite importers

**Files:**
- Move: `internal/ui/scrollbar/` → `internal/bubbles/scrollbar/`
- Move: `internal/ui/selection/` → `internal/bubbles/selection/`
- Modify (import path only): `internal/ui/messages/model.go`,
  `internal/ui/thread/model.go`, `internal/ui/messages/selection_test.go`,
  `internal/ui/thread/parent_selection_test.go`
- Modify: `AGENTS.md`

- [ ] **Step 1: Confirm the importer list is still exactly these four files**

```bash
grep -rln '"github.com/gammons/slk/internal/ui/scrollbar"\|"github.com/gammons/slk/internal/ui/selection"' --include='*.go' . | grep -v '^./.worktrees'
```

Expected: the four files listed above. If more appear, add them to this task.

- [ ] **Step 2: Move and rewrite**

```bash
git mv internal/ui/scrollbar internal/bubbles/scrollbar
git mv internal/ui/selection internal/bubbles/selection
grep -rl 'internal/ui/scrollbar"\|internal/ui/selection"' --include='*.go' internal \
  | xargs sed -i 's#internal/ui/scrollbar"#internal/bubbles/scrollbar"#; s#internal/ui/selection"#internal/bubbles/selection"#'
gofmt -w internal/ui/messages internal/ui/thread
```

`gofmt -w` regroups the import blocks if needed. Check that neither moved package
imports anything under `internal/ui` (they import only `image/color`, lipgloss and
`x/ansi`, or nothing):

```bash
go test ./internal/ui -run 'TestBubblesReachTheAppOnlyThroughCore|TestTUIReachesTheAppOnlyThroughCore' -count=1
```

Expected: PASS.

- [ ] **Step 3: Prove it is pure motion**

```bash
git diff -M100% --cached --summary main... -- internal/bubbles internal/ui/scrollbar internal/ui/selection
git diff --cached -U0 main... -- internal/ui/messages internal/ui/thread | grep '^[-+][^-+]'
```

Expected: the first prints only `rename ... (100%)` lines (five files); the second
prints only `-`/`+` pairs of import lines. Paste both outputs into the PR
description.

- [ ] **Step 4: Update `AGENTS.md`**

In "Architecture in one screen", change the `internal/bubbles/` line's example
from `e.g. ansi` to `e.g. ansi, scrollbar, selection`. In the **UI chrome** table
replace the two rows:

| Need | Use |
|---|---|
| Scrollbar gutter on a rendered pane | `bubbles/scrollbar.Overlay`, `bubbles/scrollbar.Visible` |
| Text selection ranges and anchors | `bubbles/selection` (`Range`, `Anchor`, `LessOrEqual`) |

- [ ] **Step 5: Gate and commit**

```bash
git add -A internal AGENTS.md
git commit -m "refactor(bubbles): move scrollbar and selection to internal/bubbles"
```

Goldens: byte-identical (the gate's `go test` proves it).

---

## PR 2+3 — Thread drops `bubbles/viewport` (the one golden exception)

Branch `refactor/phase3/pr23-thread-viewport`, from PR 1's tip.

**Before the first change on this branch**, snapshot the goldens. Every task in
this PR proves cell identity against this snapshot, not against the previous
commit:

```bash
mkdir -p /tmp/opencode/golden-pre23 && cp internal/ui/testdata/golden/*.ansi /tmp/opencode/golden-pre23/
```

### Task 2.1: Hand-rolled scroll offset with viewport's eager clamp; converged padding

**Files:**
- Modify: `internal/ui/thread/model.go` (the `vp` field, `ScrollUp`, `ScrollDown`,
  `ViewportAtTop`, `absoluteLineAt`, both `View` branches, imports)
- Modify (state reach only): `internal/ui/thread/selected_timestamp_test.go`,
  `open_bottom_test.go`, `selection_test.go`, `model_test.go`
- Re-bless: `internal/ui/testdata/golden/{thread_open,wide,narrow,thread_parent_selected}.ansi`

**Interfaces:**
- Produces (unexported, thread package): fields `yOffset`, `contentLines`,
  `areaHeight` (not `viewHeight`: that name is already the view cache's key); methods `maxYOffset() int`, `setYOffset(n int)`,
  `setContent(lines, height int)`; func `windowLines(content string, yOffset, height, width int) []string`.
  PR 5 replaces all of them with `entrylist`.

What `bubbles/viewport` did for thread, and what replaces it:

| `viewport` call | Behaviour relied on | Replacement |
|---|---|---|
| `SetWidth` / `SetHeight` / `KeyMap = {}` | store size; no key handling | `areaHeight` (width is the `View` argument) |
| `SetContent(s)` | `len(lines)` becomes the clamp bound; offset pulled back to `max(0, lines-height)` if past it | `setContent(len(lines), height)` |
| `SetYOffset(n)` | clamp to `[0, max(0, lines-height)]` | `setYOffset(n)` |
| `ScrollUp(n)` | no-op when offset ≤ 0, n == 0 or no content; else `SetYOffset(y-n)` | same, inline |
| `ScrollDown(n)` | no-op when offset ≥ max, n == 0 or no content; else `SetYOffset(y+n)` | same, inline |
| `View()` | lines `[y, y+h)`, each padded to width with **unstyled** spaces, then padded to `h` rows with unstyled blank rows; `ansi.Cut` to width if any line is wider (never true, Task 0.2) | `windowLines`: same rows, padded with a **themed** spacer row (`lipgloss.NewStyle().Width(width).Background(styles.Background).Render("")`), the channel pane's padding (D3) |

- [ ] **Step 1: Replace the field**

In `Model`, replace `vp viewport.Model` with:

```go
	// Scroll offset into viewContent. Hand-rolled since Phase 3; it keeps
	// the eager clamp bubbles/viewport had (spec §5.2 U6): every
	// assignment goes through setYOffset, which bounds it to
	// [0, maxYOffset()]. contentLines/areaHeight are the extent the
	// clamp uses, recorded by setContent each frame.
	yOffset      int
	contentLines int
	areaHeight   int
```

and add, next to `entryAndOffsetForLine`:

```go
// maxYOffset is the largest offset that still fills the reply area.
func (m *Model) maxYOffset() int { return max(0, m.contentLines-m.areaHeight) }

// setYOffset assigns the scroll offset, clamped to [0, maxYOffset()].
func (m *Model) setYOffset(n int) { m.yOffset = min(max(n, 0), m.maxYOffset()) }

// setContent records the extent of the content about to be shown and
// pulls the offset back inside it, as viewport.SetContent did.
func (m *Model) setContent(lines, height int) {
	m.contentLines, m.areaHeight = lines, height
	if m.yOffset > m.maxYOffset() {
		m.yOffset = m.maxYOffset()
	}
}

// windowLines returns rows [yOffset, yOffset+height) of content, padded
// to height with full-width background rows (the channel pane's padding).
func windowLines(content string, yOffset, height, width int) []string {
	lines := strings.Split(content, "\n")
	start := min(max(yOffset, 0), len(lines))
	end := min(start+height, len(lines))
	out := make([]string, 0, max(height, 0))
	out = append(out, lines[start:end]...)
	spacer := lipgloss.NewStyle().Width(width).Background(styles.Background).Render("")
	for len(out) < height {
		out = append(out, spacer)
	}
	return out
}
```

- [ ] **Step 2: Rewrite the scroll methods**

```go
func (m *Model) ScrollUp(n int) {
	if n > 0 {
		if m.yOffset > 0 && m.contentLines > 0 {
			m.setYOffset(m.yOffset - n)
		}
		m.snappedSelection = m.selected
		m.hasSnapped = true
		m.dirty()
	}
}

func (m *Model) ScrollDown(n int) {
	if n > 0 {
		if m.yOffset < m.maxYOffset() && m.contentLines > 0 {
			m.setYOffset(m.yOffset + n)
		}
		m.snappedSelection = m.selected
		m.hasSnapped = true
		m.dirty()
	}
}

func (m *Model) ViewportAtTop() bool { return m.yOffset == 0 }
```

Keep the existing doc comments. In `absoluteLineAt`, replace `m.vp.YOffset()` with
`m.yOffset`.

- [ ] **Step 3: Rewrite the two `View` call sites**

In the "No replies yet" branch replace the five `m.vp.*` lines and the
`strings.Split(m.vp.View(), "\n")` with:

```go
		m.setContent(emptyTotalLines, replyAreaHeight)
		visibleLines := windowLines(emptyContent, m.yOffset, replyAreaHeight, width)
```

and `m.vp.YOffset()` in its `scrollbar.Overlay` call with `m.yOffset`.

In the main path:

- `wasAnchoredAtBottom := m.totalLines > 0 && m.vp.YOffset()+m.vp.Height() >= m.totalLines`
  becomes `... m.yOffset+m.areaHeight >= m.totalLines` (the **old** height,
  recorded last frame; U5).
- `entryAndOffsetForLine(m.vp.YOffset())` becomes `entryAndOffsetForLine(m.yOffset)`.
- The block `m.vp.SetWidth(width)` … `m.vp.SetContent(m.viewContent)` becomes
  `m.setContent(m.totalLines, replyAreaHeight)`. (`viewContent` has exactly
  `m.totalLines` lines; `TestChar_ThreadScrollClampsEagerly` fails if not.)
- Every `m.vp.SetYOffset(x)` becomes `m.setYOffset(x)`; every
  `m.vp.YOffset()` becomes `m.yOffset`; every `m.vp.Height()` becomes
  `replyAreaHeight` (in the snap block it was just set to that).
- Replace the overlay-and-window tail:

```go
	content := m.viewContent
	if m.hasSelection {
		content = m.applySelectionOverlay(m.viewContent)
	}
	visibleLines := windowLines(content, m.yOffset, replyAreaHeight, width)
```

and `m.vp.YOffset()` in the final `scrollbar.Overlay` call with `m.yOffset`.

Remove the `charm.land/bubbles/v2/viewport` import.

- [ ] **Step 4: Rewrite the 14 test reaches into `m.vp`**

```bash
grep -rn 'm\.vp\.' internal/ui/thread/*_test.go
sed -i 's/m\.vp\.YOffset()/m.yOffset/g; s/m\.vp\.SetYOffset(/m.setYOffset(/g; s/m\.vp\.Height()/m.areaHeight/g' internal/ui/thread/*_test.go
grep -rn 'm\.vp\b' internal/ui/thread   # expect nothing
```

These are reach-only edits: no assertion changes.

- [ ] **Step 5: Run the thread package**

Run: `go test ./internal/ui/thread -count=1`
Expected: PASS, including every `TestChar_Thread*` and `TestLockstep_*`.

- [ ] **Step 6: Re-bless the three goldens and prove cell identity**

```bash
go test ./internal/ui -run TestGolden -count=1        # expect FAIL: thread goldens, "content identical, STYLING differs"
go test ./internal/ui -run TestGolden -update -count=1
git status --short internal/ui/testdata/golden        # expect only thread_open, wide, narrow, thread_parent_selected
SLK_GOLDEN_BASELINE=/tmp/opencode/golden-pre23 go test ./internal/ui -run TestGoldenCellsMatchBaseline -count=1 -v
```

Expected for the last command: PASS, with "bytes differ, cells identical" logged
only for files among `narrow`, `thread_open`, `thread_parent_selected`, `wide`.
If any other golden moved, or any cell differs, **stop**: the change is not the
one this task allows.

- [ ] **Step 7: Gate and commit**

`go mod tidy` must leave `go.mod`/`go.sum` unchanged (other `bubbles/v2`
packages are still used). Run the full gate.

```bash
git add internal/ui/thread internal/ui/testdata/golden
git commit -m "refactor(thread): replace bubbles/viewport with a hand-rolled offset

Keeps viewport's eager clamp (spec §5.2 U6). Pads the window with the
channel pane's themed spacer instead of viewport's unstyled spaces (spec
D3): the thread goldens are re-blessed; every cell is identical to the
pre-change goldens (TestGoldenCellsMatchBaseline)."
```

### Task 2.2: Fill the parent row; remove the outer wrapper (divergence 14)

`thread.View` wraps its whole output in
`lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Background(styles.Background)`.
It does real work in one place: the parent row is never padded to the pane
width, and the wrapper pads it. Pad the parent block directly, then drop the
wrapper.

**Files:**
- Modify: `internal/ui/thread/model.go` (`View`)
- Re-bless: the same three goldens

- [ ] **Step 1: Run the geometry test that will guard this**

Run: `go test ./internal/ui/thread -run 'TestLockstep_SharedRenderBehaviour|TestChar_' -count=1`
Expected: PASS now. Its geometry layer asserts every row is exactly the pane
width, which is what removing the wrapper alone breaks (7 assertions, measured).

- [ ] **Step 2: Pad the parent block**

In `View`, after the `if parentIsSelected { ... } else { ... }` that borders
`parentContent`, add:

```go
	// Pad every parent row to the pane width with the theme background.
	// The outer wrapper used to do this (the parent, unlike a reply, is
	// not width-filled when rendered); this is the same per-row padding,
	// applied where it is needed.
	parentContent = lipgloss.NewStyle().Width(width).Background(styles.Background).Render(parentContent)
```

This is the wrapper's own operation restricted to the parent rows, so both the
selected parent (tinted up to its widest line, theme background beyond) and the
unselected one keep every cell.

- [ ] **Step 3: Remove the wrapper at both returns**

Both `return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Background(styles.Background).Render(result)`
become:

```go
	return fitHeight(result, height)
```

and add:

```go
// fitHeight keeps the first height rows of s, as the old outer
// MaxHeight(height) did when a floored reply area (height < chrome + 1)
// made the pane one row too tall.
func fitHeight(s string, height int) string {
	if height <= 0 {
		return s
	}
	if rows := strings.Split(s, "\n"); len(rows) > height {
		return strings.Join(rows[:height], "\n")
	}
	return s
}
```

Leave the `IsEmpty` branch ("No thread selected") untouched: it is a sized block,
not the wrapper.

- [ ] **Step 4: Run the thread package**

Run: `go test ./internal/ui/thread -count=1`
Expected: PASS. If the geometry layer of `TestLockstep_SharedRenderBehaviour`
reports a short row, some row other than the parent relied on the wrapper; pad it
the same way and name it in the commit message.

- [ ] **Step 5: Re-bless and prove cell identity against the PR's snapshot**

```bash
go test ./internal/ui -run TestGolden -update -count=1
git status --short internal/ui/testdata/golden        # only the same four may appear
SLK_GOLDEN_BASELINE=/tmp/opencode/golden-pre23 go test ./internal/ui -run TestGoldenCellsMatchBaseline -count=1 -v
```

Expected: PASS against the pre-PR snapshot.

- [ ] **Step 6: Benches, gate and commit**

Run the Task 0.0 step 4 commands into `/tmp/opencode/bench-pr23-*.txt` and compare
with the baseline: the thread `BenchmarkViewScroll` should be **faster** (no
per-frame `ansi.StringWidth` pass over the content); no messages bench may move
beyond ±10%. Paste the comparison into the PR description. Run the full gate.

```bash
git add internal/ui/thread internal/ui/testdata/golden
git commit -m "refactor(thread): pad the parent row and drop the outer wrapper

Divergence 14. The wrapper's only effect on visible cells was padding
the parent row, which is now padded where it is rendered. Goldens are
cell-identical to the pre-PR snapshot."
```

PR 2+3 is complete. The PR description carries: the four re-blessed files, the
`TestGoldenCellsMatchBaseline` output against `/tmp/opencode/golden-pre23`, and the
bench comparison.

---

## PR 4 — The `entrylist` substrate, adopted by the channel pane

Branch `refactor/phase3/pr4-entrylist`, from PR 2+3's tip. Goldens:
byte-identical at every commit (no golden may change in this PR).

The code in this PR was implemented and validated on a throwaway copy of the
tree while this plan was written: every `messages` and `internal/ui` test
(including PR 0's) passed, all goldens were byte-identical, `go test ./... -race`
and `golangci-lint` were clean, and the messages benchmarks moved +1.7%
(`ViewScroll`), +0.0% (`BuildCache`) and +3.0% (`ViewFocusFlip`). Reproduce it
exactly; if something does not fit, the tree has moved since and the
characterization tests are the arbiter.

### Task 4.1: `internal/bubbles/entrylist`, with `PlainLine` moved in

**Files:**
- Create: `internal/bubbles/entrylist/entrylist.go`, `window.go`, `select.go`, `plain.go`
- Create: `internal/bubbles/entrylist/entrylist_test.go`
- Move: `internal/ui/messages/buildplainline_test.go` → `internal/bubbles/entrylist/plain_test.go`
- Modify: `internal/ui/messages/render.go` (`plainLine` and its helpers become
  aliases/delegates)
- Modify: `AGENTS.md`

**Interfaces (produced; every later task uses these names):**

```go
package entrylist

type Entry struct { ID string; Ref int; Selectable bool; Lines, Selected []string
	Plain []PlainLine; ColOffset, Trail int; Hits []Hit }
type HitKind uint8 // HitReaction, HitImage
type Hit struct { RowStart, RowEnd, ColStart, ColEnd int; Kind HitKind; Key string; Aux int }
type FrameHit struct { RowStart, RowEnd, ColStart, ColEnd int; Kind HitKind; Key string; Aux, Ref int }
type Styles struct { Background, Track, Thumb color.Color; Selection lipgloss.Style }
type ReanchorPolicy struct { BottomUsesNewHeight, AnchorRefsOnly bool }
type Visible struct { Index, From, To, Row int }
type Frame struct { Rows []string; Visible []Visible; Cursor int; MoreBelow bool }
const NoCursor = math.MinInt

func New(opts ...Option) *Model
func WithStyles(Styles) Option; func WithCursorFollowsScroll() Option   // U1
func WithPinBottomOnShrink() Option                                     // U2
func WithSnapDecorationAnchors() Option                                 // 8(c)
func WithDirtyOnDrag() Option                                           // U4
func WithEagerClamp() Option                                            // U6
func WithReanchor(ReanchorPolicy) Option                                // U5

func (m *Model) SetStyles(Styles); Version() int64
func (m *Model) SetHeight(h, cursor int)
func (m *Model) SetEntries(es []Entry, width int, keepPosition bool)
func (m *Model) ReplaceEntry(i int, e Entry); KeepLookups(n int)
func (m *Model) Layout(cursor int, lines func(i int) []string) *Frame
func (m *Model) Static() *Frame
func (m *Model) Compose(f *Frame, top, bottom string, applySelection bool) []string
func (m *Model) ScrollUp(n, cursor int); ScrollDown(n, cursor int); ScrollHint(contentY int) int
func (m *Model) SetYOffset(n int); YOffset() int; Height() int; Total() int; Width() int
func (m *Model) Entries() []Entry; Offsets() []int; IndexOf(id string) (int, bool)
func (m *Model) ResetSnap(); MarkSnapped(cursor int); HasSnapped() bool; CursorSpan() (start, end int)
func (m *Model) AbsLine(contentY int) int; EntryAt(abs int) (idx, within int, ok bool)
func (m *Model) BeginSelection(abs, col int); ExtendSelection(abs, col int)
func (m *Model) EndSelection() (string, bool); ClearSelection(); HasSelection() bool
func (m *Model) Selection() selection.Range; SelectionText() string
func (m *Model) Resolve(a selection.Anchor) (abs, col int, ok bool)
func (m *Model) ApplySelectionToBordered(bordered string, startRow, leftCols int) string
func (m *Model) HitTest(kind HitKind, row, col int) (FrameHit, bool); Hits() []FrameHit

type PlainLine struct { Text string; Bytes []int }
func PlainLines(s string) []PlainLine; DisplayWidth(PlainLine) int; SliceColumns(PlainLine, from, to int) string
```

Every algorithm here is a port of the channel pane's code (`messages/model.go`,
`viewInternal`, `buildCache`, the selection functions): the same arithmetic in
the same order. What is new is only where it lives and the options. The mapping,
for review:

| Substrate | Ported from (`messages/model.go` at `afad298`) |
|---|---|
| `SetHeight` pin | `viewInternal` 3044-3054 |
| `SetEntries` re-anchor | `viewInternal` 3071-3080, 3116-3130; `entryAndOffsetForLine` |
| `recomputeOffsets` | `recomputeEntryOffsets` |
| `Layout` locate / snap / clamp / follow / slice / hits | `viewInternal` 3134-3386 |
| `Compose` pad / indicators / overlay / scrollbar | `viewInternal` 3464-3502 |
| `overlaySelection`, `ApplySelectionToBordered` | `applySelectionToRows`, `ApplySelectionToBordered` |
| `anchorAt`, `snapAnchor`, `Resolve`, `SelectionText` | same-named functions |
| eager clamp, `ScrollUp`/`ScrollDown` eager arms | thread's Task 2.1 code (viewport's semantics) |
| `KeepLookups` | the `m.cache == nil` / reply-cache-nil lookup behaviour pinned by `TestChar_*LookupsMiss*`/`*SeeOnlyTheParent*` |

- [ ] **Step 1: Move the `PlainLine` test and write the substrate's tests**

```bash
mkdir -p internal/bubbles/entrylist
git mv internal/ui/messages/buildplainline_test.go internal/bubbles/entrylist/plain_test.go
sed -i -e 's/^package messages/package entrylist/' -e 's/plainLine{/PlainLine{/g; s/) plainLine {/) PlainLine {/g' internal/bubbles/entrylist/plain_test.go
```

Create `internal/bubbles/entrylist/entrylist_test.go`:

```go
package entrylist

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// rows builds n plain rows "<id>-<k>" padded to width w.
func rows(id string, n, w int) []string {
	out := make([]string, n)
	for k := range out {
		s := fmt.Sprintf("%s-%d", id, k)
		out[k] = s + strings.Repeat(" ", max(w-len(s), 0))
	}
	return out
}

// msg is a selectable entry of h rows; trail of them excluded from snapping.
func msg(id string, ref, h, trail int) Entry {
	ls := rows(id, h, 20)
	return Entry{ID: id, Ref: ref, Selectable: true, Lines: ls,
		Plain: PlainLines(strings.Join(ls, "\n")), Trail: trail}
}

// deco is a one-row decoration; withPlain controls whether it can be
// text-selected (the channel pane's dividers can; thread's rules cannot).
func deco(label string, withPlain bool) Entry {
	ls := rows(label, 1, 20)
	e := Entry{Ref: -1, Lines: ls}
	if withPlain {
		e.Plain = PlainLines(ls[0])
	}
	return e
}

// list builds n messages of h rows each.
func list(t *testing.T, n, h int, opts ...Option) *Model {
	t.Helper()
	m := New(opts...)
	es := make([]Entry, n)
	for i := range es {
		es[i] = msg(fmt.Sprintf("m%02d", i), i, h, 0)
	}
	m.SetHeight(10, n-1)
	m.SetEntries(es, 20, false)
	return m
}

func strip(rs []string) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = strings.TrimRight(ansi.Strip(r), " │█")
	}
	return out
}

func TestOffsetsAndTotal(t *testing.T) {
	m := list(t, 3, 2)
	if got := m.Offsets(); fmt.Sprint(got) != "[0 2 4]" || m.Total() != 6 {
		t.Fatalf("offsets %v total %d", got, m.Total())
	}
	if i, ok := m.IndexOf("m01"); !ok || i != 1 {
		t.Fatalf("IndexOf(m01) = %d, %v", i, ok)
	}
}

func TestLayoutSnapsToCursorOnlyWhenItMoves(t *testing.T) {
	m := list(t, 20, 2)
	m.Layout(19, nil)
	if m.YOffset() != 30 {
		t.Fatalf("first layout did not snap to the last entry: y=%d", m.YOffset())
	}
	m.ScrollUp(5, 19)
	m.Layout(19, nil)
	if m.YOffset() != 25 {
		t.Fatalf("an unmoved cursor pulled the window back: y=%d", m.YOffset())
	}
	m.Layout(16, nil) // lines 32-33, inside [25, 35)
	if m.YOffset() != 25 {
		t.Fatalf("cursor 16 is visible at y=25, no snap needed: y=%d", m.YOffset())
	}
	m.Layout(18, nil) // lines 36-37, below the window
	if m.YOffset() != 28 {
		t.Fatalf("cursor 18 is below the window: y=%d, want 28", m.YOffset())
	}
	m.Layout(2, nil)
	if m.YOffset() != 4 {
		t.Fatalf("moving the cursor above the window did not snap: y=%d", m.YOffset())
	}
}

func TestTrailRowsAreNotSnappedIntoView(t *testing.T) {
	m := New()
	m.SetHeight(3, 0)
	m.SetEntries([]Entry{msg("a", 0, 3, 1), msg("b", 1, 3, 0)}, 20, false)
	m.Layout(0, nil)
	m.Layout(1, nil) // rows 3..5
	if m.YOffset() != 3 {
		t.Fatalf("y=%d, want 3", m.YOffset())
	}
	m.Layout(0, nil) // a is rows 0..2, but its last row is trail: target [0,2)
	if m.YOffset() != 0 {
		t.Fatalf("y=%d, want 0", m.YOffset())
	}
}

func TestLazyClampAbsorbsOvershoot(t *testing.T) {
	m := list(t, 20, 2)
	m.Layout(19, nil)
	m.ScrollDown(50, 19)
	m.ScrollUp(1, 19)
	m.Layout(19, nil)
	if m.YOffset() != 30 {
		t.Fatalf("lazy clamp: y=%d, want 30 (overshoot absorbs the ScrollUp)", m.YOffset())
	}
}

func TestEagerClampMakesOvershootANoOp(t *testing.T) {
	m := list(t, 20, 2, WithEagerClamp())
	m.Layout(19, nil)
	m.ScrollDown(50, 19)
	m.ScrollUp(1, 19)
	m.Layout(19, nil)
	if m.YOffset() != 29 {
		t.Fatalf("eager clamp: y=%d, want 29", m.YOffset())
	}
}

func TestEagerScrollWithoutContentIsANoOpThatBumpsVersion(t *testing.T) {
	m := New(WithEagerClamp())
	v := m.Version()
	m.ScrollUp(3, 0)
	m.ScrollDown(3, 0)
	if m.YOffset() != 0 || m.Version() != v+2 {
		t.Fatalf("y=%d version=%d, want 0 and %d", m.YOffset(), m.Version(), v+2)
	}
}

func TestCursorFollowsScroll(t *testing.T) {
	for _, follow := range []bool{false, true} {
		var opts []Option
		if follow {
			opts = append(opts, WithCursorFollowsScroll())
		}
		m := list(t, 20, 2, opts...)
		m.Layout(19, nil)
		m.ScrollUp(20, 19)
		f := m.Layout(19, nil)
		want := 19
		if follow {
			want = 9 // bottommost entry visible in lines [10, 20)
		}
		if f.Cursor != want {
			t.Errorf("follow=%v: cursor %d, want %d", follow, f.Cursor, want)
		}
	}
}

func TestPinBottomOnShrink(t *testing.T) {
	for _, pin := range []bool{false, true} {
		var opts []Option
		if pin {
			opts = append(opts, WithPinBottomOnShrink())
		}
		m := list(t, 20, 2, opts...)
		m.Layout(19, nil)
		m.SetHeight(7, 19)
		want := 30
		if pin {
			want = 33
		}
		if m.YOffset() != want {
			t.Errorf("pin=%v: y=%d, want %d", pin, m.YOffset(), want)
		}
	}
}

func TestReanchorKeepsTopEntryAcrossWidthChange(t *testing.T) {
	m := list(t, 20, 2)
	m.Layout(19, nil)
	m.ScrollUp(10, 19)
	m.Layout(19, nil) // y=20: entry 10, row 0
	es := make([]Entry, 20)
	for i := range es {
		es[i] = msg(fmt.Sprintf("m%02d", i), i, 3, 0) // rewrapped taller
	}
	m.SetHeight(10, 19)
	m.SetEntries(es, 15, true)
	if m.YOffset() != 30 {
		t.Fatalf("y=%d, want 30 (entry 10 at the top)", m.YOffset())
	}
}

func TestReanchorBottomUsesOldOrNewHeight(t *testing.T) {
	// 20 entries of 2 rows (40 lines), window 10, three lines up from
	// the bottom: y=27. The next frame is taller (14) and narrower.
	for _, useNew := range []bool{false, true} {
		m := list(t, 20, 2, WithReanchor(ReanchorPolicy{BottomUsesNewHeight: useNew}))
		m.Layout(19, nil)
		m.ScrollUp(3, 19)
		m.Layout(19, nil)
		m.SetHeight(14, 19)
		es := make([]Entry, 20)
		for i := range es {
			es[i] = msg(fmt.Sprintf("m%02d", i), i, 2, 0)
		}
		m.SetEntries(es, 15, true)
		atBottom := m.YOffset() >= m.Total() // lazy: bottom sets y=total
		if atBottom != useNew {
			t.Errorf("BottomUsesNewHeight=%v: pinned to bottom=%v (y=%d)", useNew, atBottom, m.YOffset())
		}
	}
}

func TestReanchorRefsOnlySkipsLeadingNonRefEntries(t *testing.T) {
	m := New(WithReanchor(ReanchorPolicy{AnchorRefsOnly: true}))
	m.SetHeight(3, 0)
	parent := msg("p", -1, 4, 0)
	m.SetEntries([]Entry{parent, msg("a", 0, 4, 0), msg("b", 1, 4, 0)}, 20, false)
	m.SetYOffset(2) // inside the parent (Ref -1)
	m.SetEntries([]Entry{msg("p", -1, 6, 0), msg("a", 0, 6, 0), msg("b", 1, 6, 0)}, 15, true)
	if m.YOffset() != 2 {
		t.Fatalf("a line inside a non-ref entry re-anchored: y=%d, want 2 (kept)", m.YOffset())
	}
}

func TestComposePadsAndPlacesIndicators(t *testing.T) {
	m := New()
	m.SetHeight(4, 0)
	m.SetEntries([]Entry{msg("a", 0, 2, 0)}, 20, false)
	got := strip(m.Compose(m.Layout(0, nil), "TOP", "BOTTOM", false))
	if len(got) != 4 || got[0] != "TOP" || got[3] != "" {
		t.Fatalf("short content: %q (no more-below, so no bottom row)", got)
	}
	m2 := list(t, 20, 2)
	f := m2.Layout(0, nil)
	got = strip(m2.Compose(f, "", "BOTTOM", false))
	if got[len(got)-1] != "BOTTOM" || got[0] != "m00-0" {
		t.Fatalf("overflowing content: %q", got)
	}
}

func TestComposeDrawsScrollbarOnlyWhenOverflowing(t *testing.T) {
	m := list(t, 20, 2)
	out := m.Compose(m.Layout(19, nil), "", "", false)
	if !strings.ContainsAny(ansi.Strip(out[0]), "│█") {
		t.Fatalf("no scrollbar on an overflowing list: %q", ansi.Strip(out[0]))
	}
	for _, r := range out {
		if w := ansi.StringWidth(r); w != 20 {
			t.Fatalf("row width %d, want 20: %q", w, r)
		}
	}
}

func TestHitsAreContentRelativeAndClipped(t *testing.T) {
	m := New()
	m.SetHeight(3, 0)
	a := msg("a", 7, 5, 0) // 5 rows, window 3: y=2 is in range
	a.Hits = []Hit{{RowStart: 1, RowEnd: 3, ColStart: 2, ColEnd: 5, Kind: HitReaction, Key: "tada"}}
	m.SetEntries([]Entry{a}, 20, false)
	m.SetYOffset(2)
	m.MarkSnapped(7)
	m.Layout(7, nil)
	h, ok := m.HitTest(HitReaction, 0, 3)
	if !ok || h.Ref != 7 || h.Key != "tada" || h.RowStart != 0 || h.RowEnd != 1 {
		t.Fatalf("hit = %+v, %v; want row [0,1) ref 7", h, ok)
	}
	if _, ok := m.HitTest(HitImage, 0, 3); ok {
		t.Fatal("a reaction hit answered an image hit test")
	}
}

func TestSelectionTextAndPlainlessEntries(t *testing.T) {
	m := New()
	m.SetHeight(10, 0)
	m.SetEntries([]Entry{msg("a", 0, 2, 0), deco("rule", false), msg("b", 1, 2, 0)}, 20, false)
	m.Layout(1, nil)
	m.BeginSelection(0, 2) // a-0, col 2
	m.ExtendSelection(4, 3)
	text, ok := m.EndSelection()
	// Row a-0 from column 2 is "0"; a-1 whole; the rule has no Plain and
	// is skipped; b-0 whole; b-1 up to column 3 is "b-1".
	if !ok || text != "0\na-1\nb-0\nb-1" {
		t.Fatalf("text %q, ok %v", text, ok)
	}
}

func TestPressOnDecoration(t *testing.T) {
	for _, snap := range []bool{false, true} {
		var opts []Option
		if snap {
			opts = append(opts, WithSnapDecorationAnchors())
		}
		m := New(opts...)
		m.SetHeight(10, 0)
		// The channel pane's dividers carry Plain and snap; the thread
		// pane's decorations carry none and do not.
		m.SetEntries([]Entry{deco("day", snap), msg("a", 0, 2, 0)}, 20, false)
		m.Layout(0, nil)
		m.BeginSelection(0, 3)
		if m.HasSelection() != snap {
			t.Errorf("snap=%v: HasSelection=%v", snap, m.HasSelection())
		}
		if snap && m.Selection().Start.MessageID != "a" {
			t.Errorf("snapped to %q, want a", m.Selection().Start.MessageID)
		}
	}
}

func TestDirtyOnDrag(t *testing.T) {
	for _, dirty := range []bool{false, true} {
		var opts []Option
		if dirty {
			opts = append(opts, WithDirtyOnDrag())
		}
		m := New(opts...)
		m.SetHeight(10, 0)
		m.SetEntries([]Entry{msg("a", 0, 2, 0)}, 20, false)
		m.Layout(0, nil)
		v0 := m.Version()
		m.BeginSelection(0, 1)
		m.ExtendSelection(0, 4)
		m.ExtendSelection(0, 4)
		m.EndSelection()
		want := v0 + 1
		if dirty {
			want = v0 + 3 // begin, one real extend, end
		}
		if m.Version() != want {
			t.Errorf("dirty=%v: version %d, want %d", dirty, m.Version(), want)
		}
	}
}

func TestApplySelectionToBorderedSkipsIndicatorRows(t *testing.T) {
	m := list(t, 20, 2)
	f := m.Layout(0, nil)
	_ = m.Compose(f, "", "MORE", false)
	m.BeginSelection(0, 0)
	m.ExtendSelection(9, 4)
	bordered := strings.Join(rows("x", 10, 20), "\n")
	got := strings.Split(m.ApplySelectionToBordered(bordered, 0, 0), "\n")
	if got[9] != rows("x", 10, 20)[9] {
		t.Fatalf("the more-below row was painted: %q", got[9])
	}
	if got[0] == rows("x", 10, 20)[0] {
		t.Fatal("a selected row was not painted")
	}
}

func TestAbsLineAndEntryAt(t *testing.T) {
	m := list(t, 3, 2)
	if m.AbsLine(-5) != 0 || m.AbsLine(99) != 5 {
		t.Fatalf("AbsLine clamp: %d %d", m.AbsLine(-5), m.AbsLine(99))
	}
	if i, w, ok := m.EntryAt(3); !ok || i != 1 || w != 1 {
		t.Fatalf("EntryAt(3) = %d %d %v", i, w, ok)
	}
}

func TestKeepLookupsHidesDiscardedEntriesButKeepsExtent(t *testing.T) {
	m := list(t, 3, 2)
	m.Layout(2, nil)
	m.KeepLookups(1)
	if _, _, ok := m.EntryAt(3); ok {
		t.Fatal("a discarded entry is still found")
	}
	if _, _, ok := m.EntryAt(1); !ok {
		t.Fatal("a kept entry is no longer found")
	}
	if m.Total() != 6 {
		t.Fatalf("total changed: %d", m.Total())
	}
	m.BeginSelection(3, 0)
	if m.HasSelection() {
		t.Fatal("a press on a discarded entry started a selection")
	}
	m.SetEntries(m.Entries(), 20, false)
	if _, _, ok := m.EntryAt(3); !ok {
		t.Fatal("SetEntries did not restore lookups")
	}
}
```

- [ ] **Step 2: Run them and see them fail**

Run: `go test ./internal/bubbles/entrylist -count=1`
Expected: FAIL to compile (`undefined: New`, `undefined: PlainLine`, …).

- [ ] **Step 3: Write `plain.go` (moved from `messages/render.go`)**

```go
package entrylist

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// PlainLine is the ANSI-stripped text of one rendered row plus a
// column→byte index into it, so text selection can slice by display
// column without splitting a grapheme cluster.
//
// Bytes has one entry per display column plus a final sentinel equal to
// len(Text). A wide cluster occupies several columns that all point at
// its first byte. Zero-width clusters (combining marks, standalone ZWJ)
// are part of Text but take no column; their bytes attach to the next
// column (or to the previous one when they trail a base cluster).
type PlainLine struct {
	Text  string
	Bytes []int
}

// PlainLines returns the column-aligned plain mirror of each line of s.
//
// Widths come from uniseg (wcwidth-style cell counting), the model the
// rest of the message pipeline uses. A custom emoji whose terminal width
// disagrees may be off by one column; for selection the worst case is a
// one-cell drift between the highlight and the copied text.
func PlainLines(s string) []PlainLine {
	rawLines := strings.Split(ansi.Strip(s), "\n")
	out := make([]PlainLine, len(rawLines))
	for i, line := range rawLines {
		out[i] = buildPlainLine(line)
	}
	return out
}

// buildPlainLine computes the column→byte map for one ANSI-free line.
//
// It uses the grapheme-only iterator (FirstGraphemeClusterInString)
// rather than uniseg.NewGraphemes, whose full state machine also computes
// word, sentence and line breaks; that extra work was ~67% of a full
// message-cache build. Boundaries and widths are identical (pinned by
// TestBuildPlainLine_MatchesUnisegGraphemes).
func buildPlainLine(line string) PlainLine {
	if line == "" {
		return PlainLine{Text: "", Bytes: []int{0}}
	}
	bytesMap := make([]int, 0, len(line))
	byteOffset := 0
	state := -1
	rest := line
	for len(rest) > 0 {
		var cluster string
		var w int
		cluster, rest, w, state = uniseg.FirstGraphemeClusterInString(rest, state)
		if w <= 0 {
			byteOffset += len(cluster)
			continue
		}
		for k := 0; k < w; k++ {
			bytesMap = append(bytesMap, byteOffset)
		}
		byteOffset += len(cluster)
	}
	bytesMap = append(bytesMap, len(line))
	return PlainLine{Text: line, Bytes: bytesMap}
}

// DisplayWidth returns the number of display columns in p.
func DisplayWidth(p PlainLine) int {
	if len(p.Bytes) == 0 {
		return 0
	}
	return len(p.Bytes) - 1
}

// SliceColumns returns the text covering display columns [from, to) of
// p. Out-of-range arguments are clamped. Slicing into the middle of a
// wide cluster includes the whole cluster, so copied text reads as
// written.
func SliceColumns(p PlainLine, from, to int) string {
	width := DisplayWidth(p)
	if from < 0 {
		from = 0
	}
	if to > width {
		to = width
	}
	if from >= to {
		return ""
	}
	return p.Text[p.Bytes[from]:p.Bytes[to]]
}
```

- [ ] **Step 4: Write `entrylist.go`**

```go
// Package entrylist is a scrollable, selectable list of pre-rendered
// entries: the substrate the channel and thread panes share.
//
// It owns line offsets, the scroll offset, the visible window, text
// selection, hit-rectangle translation and the scrollbar. It knows
// nothing about messages: an owner renders each item to an Entry and
// keeps everything domain-specific (chrome, empty states, what an entry
// means) to itself.
//
// Every behaviour the two panes do differently is an Option, set by each
// owner so that it behaves exactly as it did before this package existed.
// The options are the executable form of the divergence list that
// internal/ui/thread/lockstep_test.go used to carry.
package entrylist

import (
	"image/color"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/bubbles/selection"
)

// Entry is one pre-rendered item: a message, or a decoration row such as
// a day divider.
type Entry struct {
	// ID is the stable key a text selection anchors to (a Slack ts).
	// "" marks a decoration.
	ID string
	// Ref is the owner's index for the entry (a message or reply index),
	// or -1. Hit tests and cursor moves report it.
	Ref int
	// Selectable entries can hold the cursor. Snapping, ClickAt-style
	// lookups and cursor-follows-scroll consider only these.
	Selectable bool
	// Lines are the rendered rows, each exactly the list width.
	Lines []string
	// Selected is the cursor-on variant of Lines, the same length. nil
	// means draw Lines.
	Selected []string
	// Plain mirrors the rows a text selection can cover. Rows at or past
	// len(Plain) (every row, when Plain is nil) are never selected.
	Plain []PlainLine
	// ColOffset is the number of display columns of entry chrome (the
	// "▌" border) before the content Plain mirrors.
	ColOffset int
	// Trail is the number of trailing rows that belong to the entry but
	// are not scrolled into view when the cursor lands on it (the channel
	// pane's spacer row).
	Trail int
	// Hits are clickable rectangles, relative to Lines.
	Hits []Hit
}

// HitKind says what a clickable rectangle is.
type HitKind uint8

const (
	// HitReaction is a reaction pill; Key is the emoji name.
	HitReaction HitKind = iota + 1
	// HitImage is an inline image; Key is the file ID, Aux the
	// attachment index.
	HitImage
)

// Hit is a clickable rectangle relative to an Entry's Lines.
type Hit struct {
	RowStart, RowEnd int // [RowStart, RowEnd)
	ColStart, ColEnd int // [ColStart, ColEnd), display columns
	Kind             HitKind
	Key              string
	Aux              int
}

// FrameHit is a Hit translated into the last frame's window: Row 0 is
// the first row of content under the owner's chrome. Ref is the owning
// entry's Ref.
type FrameHit struct {
	RowStart, RowEnd int
	ColStart, ColEnd int
	Kind             HitKind
	Key              string
	Aux              int
	Ref              int
}

// Styles are the colours the list draws with itself; entries carry their
// own styling.
type Styles struct {
	// Background fills padding rows and the scrollbar gutter.
	Background color.Color
	// Track and Thumb colour the scrollbar.
	Track, Thumb color.Color
	// Selection highlights text-selected columns.
	Selection lipgloss.Style
}

// DefaultStyles returns usable styles for a dark background. Owners with
// a theme build Styles directly.
func DefaultStyles() Styles {
	return Styles{
		Background: lipgloss.Color("#1A1A2E"),
		Track:      lipgloss.Color("#3A3A5C"),
		Thumb:      lipgloss.Color("#4A9EFF"),
		Selection:  lipgloss.NewStyle().Reverse(true),
	}
}

// ReanchorPolicy says how SetEntries keeps the scroll position when the
// width changes (every entry rewraps): a viewport at the bottom stays at
// the bottom, otherwise the entry under the first visible line stays
// there, at the same row within it.
type ReanchorPolicy struct {
	// BottomUsesNewHeight measures "was at the bottom" with this frame's
	// height instead of the previous frame's.
	BottomUsesNewHeight bool
	// AnchorRefsOnly anchors only on entries with Ref >= 0. A first
	// visible line above the first such entry keeps the raw offset.
	AnchorRefsOnly bool
}

// Option configures a Model.
type Option func(*Model)

// WithStyles sets the initial styles.
func WithStyles(s Styles) Option { return func(m *Model) { m.styles = s } }

// WithCursorFollowsScroll moves the cursor onto the window when a scroll
// leaves it off-screen (the channel pane).
func WithCursorFollowsScroll() Option { return func(m *Model) { m.follow = true } }

// WithPinBottomOnShrink keeps a bottom-anchored window at the bottom when
// SetHeight shrinks it (the channel pane).
func WithPinBottomOnShrink() Option { return func(m *Model) { m.pinBottom = true } }

// WithSnapDecorationAnchors lets a selection that starts or extends on a
// decoration snap to the nearest selectable entry (the channel pane).
// Without it such a press anchors nothing.
func WithSnapDecorationAnchors() Option { return func(m *Model) { m.snapDecor = true } }

// WithDirtyOnDrag bumps Version on every selection change during a drag
// (the thread pane). Without it only the first press and an empty
// release do, because the channel pane repaints the selection as a
// post-pass (ApplySelectionToBordered).
func WithDirtyOnDrag() Option { return func(m *Model) { m.dirtyOnDrag = true } }

// WithEagerClamp clamps the offset on every assignment and makes a scroll
// past either end a no-op (the thread pane, which used bubbles/viewport).
// Without it the offset is clamped once per frame, in Layout.
func WithEagerClamp() Option { return func(m *Model) { m.eager = true } }

// WithReanchor sets the width-change policy.
func WithReanchor(p ReanchorPolicy) Option { return func(m *Model) { m.reanchor = p } }

// Model is the list. The zero value is not usable; call New.
type Model struct {
	styles                                           Styles
	follow, pinBottom, snapDecor, dirtyOnDrag, eager bool
	reanchor                                         ReanchorPolicy

	entries []Entry
	live    int // entries lookups may see; -1 = all (see KeepLookups)
	offsets []int
	total   int
	ids     map[string]int
	width   int

	height, prevHeight int
	extent             int // lines the eager clamp bounds against
	yOffset            int
	snapped            int
	hasSnapped         bool
	selStart, selEnd   int

	sel    selection.Range
	hasSel bool

	hits                []FrameHit
	overTop, overBottom bool

	spacer      string
	spacerWidth int
	spacerOK    bool

	version int64
}

// New returns an empty list.
func New(opts ...Option) *Model {
	m := &Model{styles: DefaultStyles(), ids: map[string]int{}, live: -1}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// SetStyles replaces the styles. It does not bump Version: owners call it
// every frame with their theme, and bump their own version when the
// theme changes.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.spacerOK = false
}

// Version increases whenever the list's own state changes in a way that
// can change the next frame (scrolling, selection).
func (m *Model) Version() int64 { return m.version }

// Entries returns the current entries. The slice is the list's own.
func (m *Model) Entries() []Entry { return m.entries }

// Offsets returns the absolute line each entry starts on.
func (m *Model) Offsets() []int { return m.offsets }

// Total returns the number of lines in all entries.
func (m *Model) Total() int { return m.total }

// Width returns the width of the current entries.
func (m *Model) Width() int { return m.width }

// Height returns the window height last set.
func (m *Model) Height() int { return m.height }

// YOffset returns the first visible line.
func (m *Model) YOffset() int { return m.yOffset }

// SetYOffset assigns the first visible line (clamped when eager). It does
// not bump Version, matching viewport.SetYOffset.
func (m *Model) SetYOffset(n int) { m.setY(n) }

// IndexOf returns the index of the entry with id.
func (m *Model) IndexOf(id string) (int, bool) {
	i, ok := m.ids[id]
	return i, ok
}

// ResetSnap makes the next Layout snap the window to the cursor even if
// the cursor has not moved (used when the content is replaced).
func (m *Model) ResetSnap() { m.hasSnapped = false }

// HasSnapped reports whether a Layout has snapped since the last
// ResetSnap.
func (m *Model) HasSnapped() bool { return m.hasSnapped }

// CursorSpan returns the last Layout's scroll target for the cursor:
// lines [start, end) of its entry, excluding Trail rows. Both are 0 when
// no entry matched.
func (m *Model) CursorSpan() (start, end int) { return m.selStart, m.selEnd }

// MarkSnapped records sel as already snapped, so the next Layout leaves
// the offset alone unless the cursor moves.
func (m *Model) MarkSnapped(sel int) { m.snapped, m.hasSnapped = sel, true }

// SetHeight sets this frame's window height. With WithPinBottomOnShrink, a
// window that was at the bottom and gets shorter moves down by the
// difference and marks sel snapped, so the cursor does not drag it back.
func (m *Model) SetHeight(h, sel int) {
	if m.pinBottom && m.height > 0 && h < m.height && m.total > 0 && m.yOffset+m.height >= m.total {
		m.yOffset += m.height - h
		m.MarkSnapped(sel)
	}
	m.prevHeight, m.height = m.height, h
}

// SetEntries replaces the entries, rendered at width. The list takes
// ownership of es.
//
// When keepPosition is true and the width changed, the scroll position is
// kept per the ReanchorPolicy. Owners pass true only when the previous
// layout is still a valid description of the screen (the channel pane:
// its render cache was not discarded; the thread pane: its view cache is
// valid).
func (m *Model) SetEntries(es []Entry, width int, keepPosition bool) {
	reanchor := keepPosition && m.width != width
	var bottom, anchored bool
	var ord, within int
	if reanchor {
		h := m.prevHeight
		if m.reanchor.BottomUsesNewHeight {
			h = m.height
		}
		bottom = m.total > 0 && m.yOffset+h >= m.total
		ord, within, anchored = m.lineAnchor(m.yOffset)
	}
	m.entries, m.width, m.live = es, width, -1
	m.recompute()
	if m.eager {
		m.setY(m.yOffset)
	}
	if !reanchor {
		return
	}
	switch {
	case bottom:
		m.setY(m.total)
	case anchored:
		if i, ok := m.nthAnchor(ord); ok {
			h := len(m.entries[i].Lines)
			within = max(min(within, h-1), 0)
			m.setY(m.offsets[i] + within)
		}
	}
}

// KeepLookups says that, until the next SetEntries, only the first n
// entries still describe the screen: EntryAt, text selection and anchor
// resolution ignore the rest. The scroll extent (offsets, total) is kept.
// Owners call it when they discard their render cache but have not
// rebuilt it yet (0 for the channel pane; 1 for the thread pane, whose
// parent entry stays current).
func (m *Model) KeepLookups(n int) { m.live = max(n, 0) }

// lookupLen is the number of entries lookups may see.
func (m *Model) lookupLen() int {
	if m.live < 0 || m.live > len(m.entries) {
		return len(m.entries)
	}
	return m.live
}

// ReplaceEntry swaps entry i for e, keeping every other entry (a partial
// rebuild). Offsets are recomputed; the position is not re-anchored.
func (m *Model) ReplaceEntry(i int, e Entry) {
	if old := m.entries[i].ID; old != e.ID {
		if old != "" && m.ids[old] == i {
			delete(m.ids, old)
		}
		if e.ID != "" {
			m.ids[e.ID] = i
		}
	}
	m.entries[i] = e
	m.recomputeOffsets()
}

// ScrollUp moves the window up n lines and marks sel snapped. Eager: a
// no-op at the top or with no content (Version still bumps).
func (m *Model) ScrollUp(n, sel int) {
	if n <= 0 {
		return
	}
	if m.eager {
		if m.yOffset > 0 && m.extent > 0 {
			m.setY(m.yOffset - n)
		}
	} else {
		m.yOffset = max(m.yOffset-n, 0)
	}
	m.MarkSnapped(sel)
	m.version++
}

// ScrollDown moves the window down n lines and marks sel snapped. Eager:
// a no-op at the bottom or with no content. Lazy: may overshoot until the
// next Layout clamps it.
func (m *Model) ScrollDown(n, sel int) {
	if n <= 0 {
		return
	}
	if m.eager {
		if m.yOffset < m.maxOffset() && m.extent > 0 {
			m.setY(m.yOffset + n)
		}
	} else {
		m.yOffset += n
	}
	m.MarkSnapped(sel)
	m.version++
}

// ScrollHint reports whether a drag at content row contentY should
// auto-scroll: -1 at (or above) the top row, +1 at the bottom row, else 0.
func (m *Model) ScrollHint(contentY int) int {
	switch {
	case m.height <= 0:
		return 0
	case contentY <= 0:
		return -1
	case contentY >= m.height-1:
		return +1
	}
	return 0
}

func (m *Model) maxOffset() int { return max(0, m.extent-m.height) }

func (m *Model) setY(n int) {
	if m.eager {
		n = min(max(n, 0), m.maxOffset())
	}
	m.yOffset = n
}

// clamp bounds the offset to [0, total-height] once per frame.
func (m *Model) clamp() {
	if m.yOffset < 0 {
		m.yOffset = 0
	}
	if mx := max(0, m.total-m.height); m.yOffset > mx {
		m.yOffset = mx
	}
}

func (m *Model) recompute() {
	m.recomputeOffsets()
	clear(m.ids)
	for i := range m.entries {
		if id := m.entries[i].ID; id != "" {
			m.ids[id] = i
		}
	}
}

func (m *Model) recomputeOffsets() {
	m.offsets = m.offsets[:0]
	off := 0
	for i := range m.entries {
		m.offsets = append(m.offsets, off)
		off += len(m.entries[i].Lines)
	}
	m.total = off
	m.extent = off
}

// lineAnchor maps an absolute line to the anchor candidate covering it:
// its ordinal among candidates and the line's offset from its start.
func (m *Model) lineAnchor(line int) (ord, within int, ok bool) {
	best, n := -1, 0
	for i := range m.entries {
		if m.reanchor.AnchorRefsOnly && m.entries[i].Ref < 0 {
			continue
		}
		if m.offsets[i] > line {
			break
		}
		best, ord = i, n
		n++
	}
	if best < 0 {
		return 0, 0, false
	}
	return ord, line - m.offsets[best], true
}

// nthAnchor returns the index of the ord-th anchor candidate.
func (m *Model) nthAnchor(ord int) (int, bool) {
	n := 0
	for i := range m.entries {
		if m.reanchor.AnchorRefsOnly && m.entries[i].Ref < 0 {
			continue
		}
		if n == ord {
			return i, true
		}
		n++
	}
	return 0, false
}
```

- [ ] **Step 5: Write `window.go`**

```go
package entrylist

import (
	"math"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/bubbles/scrollbar"
)

// NoCursor is a cursor value that matches no entry.
const NoCursor = math.MinInt

// Visible describes one entry's rows in a frame.
type Visible struct {
	Index    int // entry index
	From, To int // the entry's rows [From, To) that are on screen
	Row      int // window row the entry's row From lands on
}

// Frame is one laid-out window. An owner may rewrite Rows in place (to
// substitute image fallbacks, say) before passing the frame to Compose.
type Frame struct {
	Rows    []string
	Visible []Visible
	// Cursor is the cursor's Ref after this frame; WithCursorFollowsScroll
	// can move it, and the owner must adopt the new value.
	Cursor int
	// MoreBelow reports content below the window.
	MoreBelow bool
}

// Layout positions the window for this frame. It locates the cursor's
// entry (the selectable entry whose Ref is cursor), snaps the offset to it
// if the cursor moved since the last snap, clamps, lets the cursor follow
// the scroll when enabled, then slices the visible rows and translates the
// visible entries' hits.
//
// lines(i) returns the rows to draw for the cursor's entry i; a nil lines
// draws Entry.Selected, or Lines when that is nil.
func (m *Model) Layout(cursor int, lines func(i int) []string) *Frame {
	m.locate(cursor)
	if !m.hasSnapped || m.snapped != cursor {
		if m.selEnd > m.yOffset+m.height {
			m.setY(m.selEnd - m.height)
		}
		if m.selStart < m.yOffset {
			m.setY(m.selStart)
		}
		m.MarkSnapped(cursor)
	}
	m.clamp()
	if m.follow {
		cursor = m.followCursor(cursor)
	}
	f := &Frame{Cursor: cursor, MoreBelow: m.yOffset+m.height < m.total}
	m.hits = m.hits[:0]
	f.Rows, f.Visible = m.slice(cursor, lines, true)
	return f
}

// Static slices the window at the current offset with no snapping, no
// clamping beyond the eager clamp, no cursor and no hit translation. The
// thread pane's zero-reply state uses it.
func (m *Model) Static() *Frame {
	f := &Frame{Cursor: NoCursor, MoreBelow: m.yOffset+m.height < m.total}
	f.Rows, f.Visible = m.slice(NoCursor, nil, false)
	return f
}

// Compose finishes a frame. It pads Rows to the window height with
// background rows; replaces the first row with top, and the last with
// bottom when there is content below the window; overlays the text
// selection when applySelection is set; and draws the scrollbar. An empty
// top or bottom leaves its row alone.
func (m *Model) Compose(f *Frame, top, bottom string, applySelection bool) []string {
	rows := f.Rows
	if len(rows) < m.height {
		sp := m.spacerRow()
		for len(rows) < m.height {
			rows = append(rows, sp)
		}
	}
	m.overTop = top != "" && len(rows) > 0
	if m.overTop {
		rows[0] = top
	}
	m.overBottom = bottom != "" && f.MoreBelow && len(rows) > 0
	if m.overBottom {
		rows[len(rows)-1] = bottom
	}
	if applySelection && m.hasSel {
		m.overlaySelection(rows, 0, 0)
	}
	return scrollbar.Overlay(rows, m.width, m.total, m.yOffset, m.height,
		m.styles.Background, m.styles.Track, m.styles.Thumb)
}

// HitTest returns the last frame's first hit of kind covering (row, col),
// with row relative to the first content row.
func (m *Model) HitTest(kind HitKind, row, col int) (FrameHit, bool) {
	for _, h := range m.hits {
		if h.Kind == kind && row >= h.RowStart && row < h.RowEnd && col >= h.ColStart && col < h.ColEnd {
			return h, true
		}
	}
	return FrameHit{}, false
}

// Hits returns the last frame's hits.
func (m *Model) Hits() []FrameHit { return m.hits }

// locate records the cursor entry's scroll target [selStart, selEnd),
// which excludes its Trail rows. Both are 0 when no entry matches.
func (m *Model) locate(cursor int) {
	m.selStart, m.selEnd = 0, 0
	for i := range m.entries {
		e := &m.entries[i]
		if e.Selectable && e.Ref == cursor {
			m.selStart = m.offsets[i]
			m.selEnd = m.selStart + len(e.Lines) - e.Trail
			return
		}
	}
}

// followCursor moves an off-screen cursor to the nearest visible
// selectable entry: the topmost when it went off the top, the bottommost
// when it went off the bottom.
func (m *Model) followCursor(cursor int) int {
	top, bottom := m.yOffset, m.yOffset+m.height
	above, below := m.selEnd <= top, m.selStart >= bottom
	if !above && !below {
		return cursor
	}
	next, found := 0, false
	for i := range m.entries {
		e := &m.entries[i]
		if !e.Selectable {
			continue
		}
		start := m.offsets[i]
		if start+len(e.Lines) <= top || start >= bottom {
			continue
		}
		next, found = e.Ref, true
		if above {
			break
		}
	}
	if found && next != cursor {
		cursor = next
		m.snapped = cursor
		m.locate(cursor)
	}
	return cursor
}

func (m *Model) slice(cursor int, lines func(int) []string, withHits bool) ([]string, []Visible) {
	rows := make([]string, 0, max(m.height, 0))
	var vis []Visible
	top, bottom := m.yOffset, m.yOffset+m.height
	for i := range m.entries {
		if len(rows) >= m.height {
			break
		}
		e := &m.entries[i]
		start := m.offsets[i]
		end := start + len(e.Lines)
		if end <= top {
			continue
		}
		if start >= bottom {
			break
		}
		ls := e.Lines
		if e.Selectable && e.Ref == cursor {
			switch {
			case lines != nil:
				ls = lines(i)
			case e.Selected != nil:
				ls = e.Selected
			}
		}
		from, to := 0, len(ls)
		if start < top {
			from = top - start
		}
		if end > bottom {
			to = len(ls) - (end - bottom)
		}
		vis = append(vis, Visible{Index: i, From: from, To: to, Row: len(rows)})
		rows = append(rows, ls[from:to]...)
		if !withHits {
			continue
		}
		for _, h := range e.Hits {
			as, ae := start+h.RowStart, start+h.RowEnd
			if ae <= top || as >= bottom {
				continue
			}
			m.hits = append(m.hits, FrameHit{
				RowStart: max(as-top, 0), RowEnd: min(ae-top, m.height),
				ColStart: h.ColStart, ColEnd: h.ColEnd,
				Kind: h.Kind, Key: h.Key, Aux: h.Aux, Ref: e.Ref,
			})
		}
	}
	return rows, vis
}

// spacerRow is a full-width background row, cached per width and styles.
func (m *Model) spacerRow() string {
	if !m.spacerOK || m.spacerWidth != m.width {
		m.spacer = lipgloss.NewStyle().Background(m.styles.Background).Width(m.width).Render("")
		m.spacerWidth, m.spacerOK = m.width, true
	}
	return m.spacer
}
```

- [ ] **Step 6: Write `select.go`**

```go
package entrylist

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/bubbles/selection"
)

// AbsLine maps a content row (0 = first row under the owner's chrome) to
// an absolute line, clamped to the content.
func (m *Model) AbsLine(contentY int) int {
	abs := max(contentY, 0) + m.yOffset
	abs = max(abs, 0)
	if m.total > 0 && abs >= m.total {
		abs = m.total - 1
	}
	return abs
}

// EntryAt returns the entry covering absolute line abs and the row within
// it, among the entries lookups may see (KeepLookups).
func (m *Model) EntryAt(abs int) (idx, within int, ok bool) {
	for i := range m.lookupLen() {
		start := m.offsets[i]
		if abs >= start && abs < start+len(m.entries[i].Lines) {
			return i, abs - start, true
		}
	}
	return 0, 0, false
}

// HasSelection reports whether a text selection is active or pinned.
func (m *Model) HasSelection() bool { return m.hasSel }

// Selection returns the current selection range.
func (m *Model) Selection() selection.Range { return m.sel }

// BeginSelection anchors a new selection at absolute line abs, display
// column col. A press that anchors nothing is a no-op.
func (m *Model) BeginSelection(abs, col int) {
	a, ok := m.anchorAt(abs, col)
	if !ok {
		return
	}
	if a, ok = m.snapAnchor(a, abs); !ok {
		return
	}
	m.sel = selection.Range{Start: a, End: a, Active: true}
	if m.dirtyOnDrag || !m.hasSel {
		// The channel pane bumps once per drag: the selected row swaps
		// to its short-timestamp variant. The thread pane bumps on every
		// change (WithDirtyOnDrag).
		m.version++
	}
	m.hasSel = true
}

// ExtendSelection moves the selection's end.
func (m *Model) ExtendSelection(abs, col int) {
	if !m.hasSel {
		return
	}
	a, ok := m.anchorAt(abs, col)
	if !ok {
		return
	}
	if a, ok = m.snapAnchor(a, abs); !ok {
		return
	}
	if a == m.sel.End {
		return
	}
	m.sel.End = a
	if m.dirtyOnDrag {
		m.version++
	}
}

// EndSelection finishes a drag and returns the selected text; ok is false
// when nothing was selected. An empty selection is cleared.
func (m *Model) EndSelection() (string, bool) {
	if !m.hasSel {
		return "", false
	}
	m.sel.Active = false
	if m.sel.IsEmpty() {
		m.hasSel = false
		m.sel = selection.Range{}
		m.version++
		return "", false
	}
	text := m.SelectionText()
	if m.dirtyOnDrag {
		m.version++
	}
	if text == "" {
		return "", false
	}
	return text, true
}

// ClearSelection removes the selection, if any.
func (m *Model) ClearSelection() {
	if !m.hasSel {
		return
	}
	m.hasSel = false
	m.sel = selection.Range{}
	m.version++
}

// SelectionText returns the selected text: each row trimmed of trailing
// spaces, rows joined by newlines, no trailing newline.
func (m *Model) SelectionText() string {
	if !m.hasSel || m.sel.IsEmpty() {
		return ""
	}
	loLine, loCol, hiLine, hiCol, ok := m.selectedSpan()
	if !ok {
		return ""
	}
	var b strings.Builder
	for i := range m.lookupLen() {
		e := &m.entries[i]
		start := m.offsets[i]
		if start+len(e.Lines) <= loLine {
			continue
		}
		if start > hiLine {
			break
		}
		for j, plain := range e.Plain {
			abs := start + j
			if abs < loLine {
				continue
			}
			if abs > hiLine {
				break
			}
			from, to := 0, DisplayWidth(plain)
			if abs == loLine {
				from = loCol
			}
			if abs == hiLine {
				to = hiCol
			}
			b.WriteString(strings.TrimRight(SliceColumns(plain, from, to), " "))
			if abs != hiLine {
				b.WriteByte('\n')
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// ApplySelectionToBordered paints the selection onto an owner's cached,
// bordered frame: startRow is the frame row of the first content row
// (border plus chrome rows above it), leftCols the border columns.
func (m *Model) ApplySelectionToBordered(bordered string, startRow, leftCols int) string {
	if !m.hasSel || m.height <= 0 {
		return bordered
	}
	lines := strings.Split(bordered, "\n")
	m.overlaySelection(lines, startRow, leftCols)
	return strings.Join(lines, "\n")
}

// selectedSpan resolves the normalized selection to absolute lines and
// plain columns; ok is false when either end no longer resolves or the
// span is empty.
func (m *Model) selectedSpan() (loLine, loCol, hiLine, hiCol int, ok bool) {
	lo, hi := m.sel.Normalize()
	loLine, loCol, ok1 := m.Resolve(lo)
	hiLine, hiCol, ok2 := m.Resolve(hi)
	if !ok1 || !ok2 || loLine > hiLine || (loLine == hiLine && loCol >= hiCol) {
		return 0, 0, 0, 0, false
	}
	return loLine, loCol, hiLine, hiCol, true
}

// overlaySelection highlights the selected columns of the window rows in
// lines, which start at index startRow; columns shift by leftCols. Rows
// Compose replaced with indicators are skipped.
func (m *Model) overlaySelection(lines []string, startRow, leftCols int) {
	loLine, loCol, hiLine, hiCol, ok := m.selectedSpan()
	if !ok {
		return
	}
	for vRow := 0; vRow < m.height; vRow++ {
		row := startRow + vRow
		if row < 0 || row >= len(lines) {
			continue
		}
		if (vRow == 0 && m.overTop) || (vRow == m.height-1 && m.overBottom) {
			continue
		}
		abs := m.yOffset + vRow
		if abs < loLine || abs > hiLine {
			continue
		}
		i, j, found := m.EntryAt(abs)
		if !found {
			continue
		}
		e := &m.entries[i]
		if j >= len(e.Plain) {
			continue
		}
		plain := e.Plain[j]
		from, to := 0, DisplayWidth(plain)
		if abs == loLine {
			from = loCol
		}
		if abs == hiLine {
			to = hiCol
		}
		from, to = max(from, 0), min(to, DisplayWidth(plain))
		if from >= to {
			continue
		}
		styled := lines[row]
		dispFrom := from + e.ColOffset + leftCols
		dispTo := to + e.ColOffset + leftCols
		w := ansi.StringWidth(styled)
		if dispFrom >= w {
			continue
		}
		dispTo = min(dispTo, w)
		lines[row] = ansi.Cut(styled, 0, dispFrom) +
			m.styles.Selection.Render(SliceColumns(plain, from, to)) +
			ansi.Cut(styled, dispTo, w)
	}
}

func (m *Model) anchorAt(abs, col int) (selection.Anchor, bool) {
	i, within, ok := m.EntryAt(abs)
	if !ok {
		return selection.Anchor{}, false
	}
	e := &m.entries[i]
	if within >= len(e.Plain) {
		return selection.Anchor{}, false
	}
	plainCol := min(max(col-e.ColOffset, 0), DisplayWidth(e.Plain[within]))
	return selection.Anchor{MessageID: e.ID, Line: within, Col: plainCol}, true
}

// snapAnchor moves an anchor on a decoration (empty MessageID) to the
// next selectable entry's first row, or failing that the previous one's
// last row. Without WithSnapDecorationAnchors anchors pass through.
func (m *Model) snapAnchor(a selection.Anchor, abs int) (selection.Anchor, bool) {
	if !m.snapDecor || a.MessageID != "" {
		return a, true
	}
	start, _, ok := m.EntryAt(abs)
	if !ok {
		return selection.Anchor{}, false
	}
	for i := start + 1; i < m.lookupLen(); i++ {
		if e := &m.entries[i]; e.Selectable {
			return selection.Anchor{MessageID: e.ID}, true
		}
	}
	for i := start - 1; i >= 0; i-- {
		if e := &m.entries[i]; e.Selectable {
			last := len(e.Lines) - 1
			col := 0
			if last < len(e.Plain) {
				col = DisplayWidth(e.Plain[last])
			}
			return selection.Anchor{MessageID: e.ID, Line: last, Col: col}, true
		}
	}
	return selection.Anchor{}, false
}

// Resolve returns the absolute line and plain column an anchor points
// at; ok is false when its entry is gone (or hidden by KeepLookups).
func (m *Model) Resolve(a selection.Anchor) (abs, col int, ok bool) {
	if a.MessageID == "" {
		return 0, 0, false
	}
	i, found := m.ids[a.MessageID]
	if !found || i >= m.lookupLen() {
		return 0, 0, false
	}
	if a.Line < 0 || a.Line >= len(m.entries[i].Plain) {
		return 0, 0, false
	}
	return m.offsets[i] + a.Line, a.Col, true
}
```

- [ ] **Step 7: Run the substrate's tests**

```bash
go test ./internal/bubbles/entrylist -race -count=1 -v
go test ./internal/ui -run 'TestBubblesReachTheAppOnlyThroughCore' -count=1
```

Expected: all PASS. The boundary test proves `entrylist` imports nothing under
`internal/ui` and no `slack-go`.

- [ ] **Step 8: Make `messages`' plain-line helpers delegate**

In `internal/ui/messages/render.go`, replace everything from the doc comment
above `type plainLine struct` through the end of `func SliceColumns` (just before
`// WithBackground makes each line self-sufficient`) with:

```go
// plainLine is the ANSI-stripped, column-aligned mirror of one rendered
// row. It lives in internal/bubbles/entrylist since Phase 3; these names
// remain for the messages package's existing callers.
type plainLine = entrylist.PlainLine

func plainLines(s string) []plainLine               { return entrylist.PlainLines(s) }
func displayWidthOfPlain(p plainLine) int           { return entrylist.DisplayWidth(p) }
func sliceColumns(p plainLine, from, to int) string { return entrylist.SliceColumns(p, from, to) }

// PlainLine is the exported alias for sibling UI packages (thread).
type PlainLine = entrylist.PlainLine

// PlainLines is the exported form of plainLines.
func PlainLines(s string) []PlainLine { return entrylist.PlainLines(s) }

// DisplayWidthOfPlain is the exported form of displayWidthOfPlain.
func DisplayWidthOfPlain(p PlainLine) int { return entrylist.DisplayWidth(p) }

// SliceColumns is the exported form of sliceColumns.
func SliceColumns(p PlainLine, from, to int) string { return entrylist.SliceColumns(p, from, to) }
```

Add the import `"github.com/gammons/slk/internal/bubbles/entrylist"` and remove
`"github.com/rivo/uniseg"` (now unused there). Run
`go test ./internal/ui/messages ./internal/ui/thread -count=1`: PASS, unchanged.

- [ ] **Step 9: `AGENTS.md`**

In **Text and rendering**, change the plain-text row's "Use" to
`entrylist.PlainLines`, `entrylist.DisplayWidth`, `entrylist.SliceColumns`
(`bubbles/entrylist`; `messages.PlainLines`/`DisplayWidthOfPlain`/`SliceColumns`
delegate for existing callers). In **UI chrome**, add:

| Need | Use |
|---|---|
| A scrollable, selectable list of pre-rendered rows: line offsets, scroll offset and snapping, the visible window, text selection and copy, clickable-rect translation, the scrollbar | `bubbles/entrylist` (`New` with options, `SetHeight`, `SetEntries`, `Layout`, `Compose`). The channel and thread panes are its two owners; each divergence between them is an `entrylist` option, documented on the option |

- [ ] **Step 10: Gate and commit**

```bash
git add internal/bubbles/entrylist internal/ui/messages/render.go AGENTS.md
git commit -m "feat(bubbles): entrylist, the list substrate the content panes will share

Ports the channel pane's offsets, scroll, window, selection, hit and
scrollbar code into internal/bubbles/entrylist, with an option for
every behaviour the thread pane does differently (spec §5). PlainLine
moves in; messages keeps delegating names. No caller yet."
```

### Task 4.2: The channel pane adopts `entrylist`

**Files:**
- Modify: `internal/ui/messages/model.go`
- Create: `internal/ui/messages/pane_list.go`
- Modify (state reach only): `bench_test.go`, `blockkit_background_test.go`,
  `blockkit_integration_test.go`, `model_test.go`, `plain_test.go`,
  `selected_timestamp_test.go`, `selection_test.go` (all in `internal/ui/messages`)

**Interfaces:**
- Consumes: Task 4.1's API.
- Produces: `messages.ThemeListStyles() entrylist.Styles` (Task 5.1 uses it);
  `(*messages.Model).dropCache()` replaces every `m.cache = nil`.

- [ ] **Step 1: Add `pane_list.go`**

```go
package messages

import (
	"github.com/gammons/slk/internal/bubbles/entrylist"
	"github.com/gammons/slk/internal/ui/styles"
)

// themeListStyles maps the current theme onto the shared list substrate's
// styles. Both content panes call it at the top of every View, so a
// theme change reaches the window padding, scrollbar and selection
// highlight on the next frame, exactly when the panes' own styles.* reads
// do.
func themeListStyles() entrylist.Styles {
	return entrylist.Styles{
		Background: styles.Background,
		Track:      styles.Border,
		Thumb:      styles.Primary,
		Selection:  styles.SelectionStyle(),
	}
}

// ThemeListStyles is themeListStyles for the thread pane.
func ThemeListStyles() entrylist.Styles { return themeListStyles() }
```

- [ ] **Step 2: Shrink `viewEntry` to the message layer's payload**

Replace the `viewEntry`, `reactionEntryHit`, `entryHit`, `hitRect` and
`reactionHitRect` declarations (from `// viewEntry is a pre-rendered row` up to
`// sixelEntry holds the pre-computed sixel bytes`) with:

```go
// viewEntry is the message layer's payload for one entry of m.list,
// index-aligned with m.list.Entries(): the parts the substrate does not
// need. m.cache == nil means the render cache was discarded and the
// next View rebuilds it (see dropCache).
type viewEntry struct {
	// shortSrc is the selected row's content with the short timestamp,
	// set only when the selected variant carries a long one;
	// linesSelectedShort is built from it on demand (selectedLines) and
	// shown while a text selection exists, because the drag overlay
	// splices the short-form plain text into the row by column.
	shortSrc           string
	linesSelectedShort []string

	// flushes are per-frame kitty image uploads, invoked once per frame
	// for visible entries. nil when the entry has no inline images.
	flushes []func(io.Writer) error

	// sixelRows maps a row within the entry to the sixel image whose
	// top-left cell is on it. View paints an image out-of-band when it
	// is fully visible and substitutes its halfblock fallback otherwise.
	sixelRows map[int]sixelEntry
}

// entryHit is one inline-image hit-rect relative to a rendered entry,
// in display columns that already include the avatar gutter and the
// thick left border.
type entryHit struct {
	rowStartInEntry int
	rowEndInEntry   int // exclusive
	colStart        int
	colEnd          int // exclusive
	fileID          string
	attIdx          int // index into MessageItem.Attachments
}

// reactionEntryHit is one reaction-pill hit-rect relative to a rendered
// entry, carrying the emoji name the click toggles.
type reactionEntryHit struct {
	rowStartInEntry int
	rowEndInEntry   int // exclusive
	colStart        int
	colEnd          int // exclusive
	emoji           string
}
```

(`entryHit` and `reactionEntryHit` stay: `renderMessagePlain` still returns them;
PR 6 replaces them with `entrylist.Hit`.)

- [ ] **Step 3: Move the state into the substrate**

In `Model`:
- delete the fields `entryOffsets`, `totalLines`, `yOffset`, `snappedSelection`,
  `hasSnapped`, `selectedStartLine`, `selectedEndLine`, `selRange`,
  `hasSelection`, `messageIDToEntryIdx`, `lastViewHeight`, `lastHits`,
  `lastReactionHits`, with their comments;
- after `cacheMoreBelow`, add:

```go
	// list is the shared substrate: entries, offsets, scroll, the
	// visible window, text selection, hit translation and the scrollbar.
	// Configured for the channel pane's behaviour in New.
	list *entrylist.Model
```

Replace the `scrollbar` and `selection` imports with
`"github.com/gammons/slk/internal/bubbles/entrylist"`, and drop
`"github.com/charmbracelet/x/ansi"` once unused.

`Version` becomes `func (m *Model) Version() int64 { return m.version + m.list.Version() }`
(scrolling and selection now bump the substrate's counter; the sum still
increases whenever either does). In `New`, add `list: newChannelList(),`. After
`dirty`, add:

```go
// dropCache discards the render cache; the next View rebuilds it. Until
// then the entries on screen are no longer current, so clicks and
// selections find none (the scroll extent is kept).
func (m *Model) dropCache() {
	m.cache = nil
	m.list.KeepLookups(0)
}

// newChannelList configures the substrate for the channel pane (spec
// §5): the cursor follows the scroll (U1), a bottom-anchored pane stays
// pinned when it shrinks (U2), a press on a divider snaps to a message
// (8c), and the bottom test on resize uses the new height (U5). The
// offset clamps lazily (U6) and a drag bumps Version once (U4): the
// defaults.
func newChannelList() *entrylist.Model {
	return entrylist.New(
		entrylist.WithStyles(themeListStyles()),
		entrylist.WithCursorFollowsScroll(),
		entrylist.WithPinBottomOnShrink(),
		entrylist.WithSnapDecorationAnchors(),
		entrylist.WithReanchor(entrylist.ReanchorPolicy{BottomUsesNewHeight: true}),
	)
}
```

Then, mechanically across the file:
- every `m.cache = nil` (32 sites) → `m.dropCache()` (not inside `dropCache`);
- every `m.hasSnapped = false` → `m.list.ResetSnap()`;
- `ScrollUp`'s body → `m.list.ScrollUp(n, m.selected)` and `ScrollDown`'s →
  `m.list.ScrollDown(n, m.selected)` (keep the doc comments; the substrate
  handles `n <= 0`, the snap mark and the version bump);
- `ViewportAtTop` → `return m.list.YOffset() == 0 && len(m.messages) > 0`;
  `YOffset` → `return m.list.YOffset()`;
- `LastHitsForTest` builds `HitRect`s from `m.list.Hits()` entries whose
  `Kind == entrylist.HitImage` (`FileID: h.Key, MsgIdx: h.Ref, AttIdx: h.Aux`),
  returning `[]HitRect{}` rather than nil when there are none;
- in `IncrementReplyCount` and `UpdateReaction`: the bottom-anchor capture
  becomes

```go
	oldTotalLines := m.list.Total()
	if m.cache != nil && m.list.Height() > 0 && m.list.Total() > 0 {
		wasAnchoredAtBottom = m.list.YOffset()+m.list.Height() >= m.list.Total()
	}
```

  `m.buildCache(m.cacheWidth)` becomes `m.buildCache(m.cacheWidth, false)`, and
  the offset shift becomes

```go
				if delta := m.list.Total() - oldTotalLines; delta > 0 {
					y := min(m.list.YOffset()+delta, m.list.Total()-m.list.Height())
					m.list.SetYOffset(max(y, 0))
					m.list.MarkSnapped(m.selected)
				}
```

- [ ] **Step 4: Entries are built for the substrate**

`renderMessageEntry` now returns `(entrylist.Entry, viewEntry)`. Replace its tail,
from the spacer `if i < len(m.messages)-1 {` to the end of the function, with:

```go
	trail := 0
	if i < len(m.messages)-1 {
		linesN = append(linesN, cs.spacerLines...)
		linesS = append(linesS, cs.spacerLines...)
		linesP = append(linesP, plainLine{Text: "", Bytes: []int{0}})
		trail = len(cs.spacerLines)
	}
	hits := make([]entrylist.Hit, 0, len(attachHits)+len(reactHits))
	for _, h := range attachHits {
		hits = append(hits, entrylist.Hit{
			RowStart: h.rowStartInEntry, RowEnd: h.rowEndInEntry,
			ColStart: h.colStart, ColEnd: h.colEnd,
			Kind: entrylist.HitImage, Key: h.fileID, Aux: h.attIdx,
		})
	}
	for _, h := range reactHits {
		hits = append(hits, entrylist.Hit{
			RowStart: h.rowStartInEntry, RowEnd: h.rowEndInEntry,
			ColStart: h.colStart, ColEnd: h.colEnd,
			Kind: entrylist.HitReaction, Key: h.emoji,
		})
	}
	entry := entrylist.Entry{
		ID:         msg.TS,
		Ref:        i,
		Selectable: true,
		Lines:      linesN,
		Selected:   linesS,
		Plain:      linesP,
		ColOffset:  1, // thick left border ▌ occupies column 0 of Lines
		Trail:      trail,
		Hits:       hits,
	}
	return entry, viewEntry{shortSrc: shortSrc, flushes: attachFlushes, sixelRows: attachSixel}
}
```

Delete `recomputeEntryOffsets` and `entryAndOffsetForLine`. Replace `buildCache`
(with its doc comment) and `partialRebuild` with:

```go
// buildCache pre-renders all messages and day separators, splitting each
// rendered string on "\n" so View() can flatten everything into the visible
// window with zero string-scanning per frame. Runs only on width / message-set
// / theme / reaction changes -- never on simple j/k navigation, and never
// on a single-image arrival (HandleImageReady marks one TS stale and the
// View()-time guard dispatches to partialRebuild instead).
func (m *Model) buildCache(width int, keepPosition bool) {
	m.cache = m.cache[:0]
	m.cacheWidth = width
	m.cacheMsgLen = len(m.messages)
	// A full rebuild supersedes any pending per-entry invalidations.
	for k := range m.staleEntries {
		delete(m.staleEntries, k)
	}

	cs := m.buildCacheStyles(width)

	// Perf instrumentation: allocate the per-sub-step stats sink when
	// SLK_DEBUG is on so we can attribute buildCache's wall-clock across
	// body / reactions / blockKit / legacy / attachments / borderWrap /
	// plainLines. Aggregated into a single [perf] line after the loop
	// to keep log volume O(1) per buildCache rather than O(N) per
	// message. nil disables every call site's timing (see
	// renderMessageEntry / renderMessagePlain).
	var stats *entryPerfStats
	if debuglog.Enabled() {
		stats = &entryPerfStats{}
	}

	if cap(m.cache) < len(m.messages)+8 {
		m.cache = make([]viewEntry, 0, len(m.messages)+8)
	}
	entries := make([]entrylist.Entry, 0, cap(m.cache))

	appendSeparator := func(rendered string) {
		lines := strings.Split(rendered, "\n")
		entries = append(entries, entrylist.Entry{
			Ref:      -1,
			Lines:    lines,
			Selected: lines,
			Plain:    plainLines(rendered),
		})
		m.cache = append(m.cache, viewEntry{})
	}

	var lastDate string
	newMsgLandmarkInserted := false
	for i, msg := range m.messages {
		msgDate := DateFromTS(msg.TS)
		if msgDate != "" && msgDate != lastDate {
			label := FormatDateSeparator(msgDate)
			sepStr := "── " + label + " ──"
			sep := lipgloss.NewStyle().Background(styles.Background).Foreground(styles.TextMuted).Bold(true).
				Width(width).Align(lipgloss.Center).
				Render(sepStr)
			appendSeparator(sep)
			lastDate = msgDate
		}

		// New message landmark: insert before the first unread message
		if m.lastReadTS != "" && !newMsgLandmarkInserted && msg.TS > m.lastReadTS {
			newStr := "── new ──"
			label := lipgloss.NewStyle().Background(styles.Background).Foreground(styles.Error).Bold(true).
				Width(width).Align(lipgloss.Center).
				Render(newStr)
			appendSeparator(label)
			newMsgLandmarkInserted = true
		}

		e, p := m.renderMessageEntry(i, width, cs, stats)
		entries = append(entries, e)
		m.cache = append(m.cache, p)
	}

	m.list.SetEntries(entries, width, keepPosition)

	if stats != nil {
		debuglog.Perf("messages.buildCache.breakdown N=%d body=%s reactions(n=%d)=%s blockKit(n=%d)=%s legacy(n=%d, atts=%d)=%s [text=%s image=%s other=%s] img[cachedCheck(n=%d)=%s kitty(n=%d)=%s renderImage(n=%d)=%s placeholder(n=%d)=%s] attachments(n=%d)=%s borderWrap=%s plainLines=%s",
			stats.count, stats.bodyTotal,
			stats.reactionsCount, stats.reactionsTotal,
			stats.blockKitCount, stats.blockKitTotal,
			stats.legacyCount, stats.legacyAttCount, stats.legacyTotal,
			stats.legacyTextTotal, stats.legacyImageTotal, stats.legacyOtherTotal,
			stats.legacyImgCachedCheckCount, stats.legacyImgCachedCheckTotal,
			stats.legacyImgKittyCount, stats.legacyImgKittyTotal,
			stats.legacyImgRenderImageCount, stats.legacyImgRenderImageTotal,
			stats.legacyImgPlaceholderCount, stats.legacyImgPlaceholderTotal,
			stats.attachmentsCount, stats.attachmentsTotal,
			stats.borderWrapTotal, stats.plainLinesTotal)
	}
}

// partialRebuild re-renders only the cache slots whose source-message
// TSes are in m.staleEntries, leaving sibling slots (other messages
// AND date / "── new ──" separators) intact. Called by View() when an
// ImageReadyMsg has marked one or more entries stale but neither the
// pane width nor the message-set length has changed since the last
// full buildCache.
//
// Critical perf invariant: this function MUST NOT call
// renderMessagePlain for any message that isn't in m.staleEntries.
// The whole point of per-entry invalidation is to skip the per-message
// lipgloss / wordwrap / blockkit work for siblings during an image
// burst.
//
// Preconditions enforced by the caller (the View()-time guard):
//   - m.cache != nil and m.cacheWidth == width and m.cacheMsgLen == len(m.messages)
//   - len(m.staleEntries) > 0
//
// Postcondition: m.staleEntries is empty.
func (m *Model) partialRebuild(width int) {
	cs := m.buildCacheStyles(width)
	for ts := range m.staleEntries {
		idx, ok := m.list.IndexOf(ts)
		if !ok {
			// Entry has gone away (message removed since the
			// invalidation was queued); silently drop.
			continue
		}
		if idx < 0 || idx >= len(m.cache) {
			continue
		}
		// Sanity: the slot must still belong to a message (msgIdx >= 0).
		// Separators are not addressable by TS and must never appear here.
		old := m.list.Entries()[idx]
		if !old.Selectable || old.Ref >= len(m.messages) {
			continue
		}
		e, p := m.renderMessageEntry(old.Ref, width, cs, nil)
		// ReplaceEntry recomputes offsets: heights may have changed (a
		// placeholder swapped for a real image). No render work for the
		// siblings.
		m.list.ReplaceEntry(idx, e)
		m.cache[idx] = p
	}
	for k := range m.staleEntries {
		delete(m.staleEntries, k)
	}
}
```

- [ ] **Step 5: Clicks, selection and the frame go through the substrate**

Replace everything from `// ClickAt handles a mouse click at the given
y-coordinate` up to (not including) `// DateFromTS returns the local-day date
string` with the following. This removes `absoluteLineAt`, `anchorAt`,
`snapToMessageAnchor`, `resolveAnchor` and `applySelectionToRows` (now in the
substrate) and keeps `var thickLeftBorder`:

```go
// ClickAt handles a mouse click at the given y-coordinate (the pane-local
// y returned by App.panelAt, which is measured from the panel's top border —
// so y=0..chromeHeight-1 is the channel header / separator chrome and
// y=chromeHeight onward is message content). Selects the message at that
// position; clicks in the chrome are ignored. Returns true when the click
// landed on a real message row (selection was either updated or already
// matched the hit), false when the click missed -- callers use the bool to
// distinguish "clicked a message" from "clicked empty space" so that
// click-to-open-thread doesn't fire on chrome / dead-space clicks.
//
// Unlike the thread pane, a click below the content misses: the line is
// not clamped to the content.
func (m *Model) ClickAt(y int) bool {
	contentY := y - m.chromeHeight
	if contentY < 0 {
		return false // click on chrome (header / separator) — ignore
	}
	i, _, ok := m.list.EntryAt(contentY + m.list.YOffset())
	if !ok {
		return false
	}
	e := m.list.Entries()[i]
	if !e.Selectable {
		return false // date separator or "new messages" line
	}
	if m.selected != e.Ref {
		m.selected = e.Ref
		m.dirty()
	}
	return true
}

// HitTest returns the message index, attachment index, and Slack file
// ID of an inline image rendered at (row, col) within the messages-pane
// content area, or ok=false when no image footprint covers that cell.
//
// Coordinate frame: row=0 is the FIRST row of message content (i.e.,
// just below the channel-header chrome); col=0 is the leftmost column
// of the messages pane. The app-level mouse handler subtracts the
// chrome height before calling. Hits come from the most recent View().
func (m *Model) HitTest(row, col int) (msgIdx, attIdx int, fileID string, ok bool) {
	h, ok := m.list.HitTest(entrylist.HitImage, row, col)
	if !ok {
		return 0, 0, "", false
	}
	return h.Ref, h.Aux, h.Key, true
}

// HitTestReaction returns the (message index, reaction emoji name) at
// (row, col) within the messages-pane content area, or ok=false when
// no reaction pill covers that cell. Coordinate frame mirrors HitTest.
func (m *Model) HitTestReaction(row, col int) (msgIdx int, emoji string, ok bool) {
	h, ok := m.list.HitTest(entrylist.HitReaction, row, col)
	if !ok {
		return 0, "", false
	}
	return h.Ref, h.Key, true
}

var thickLeftBorder = lipgloss.Border{Left: "▌"}

// BeginSelectionAt anchors a new selection at the given pane-local
// coordinates. Clicks on the chrome are ignored; a press on a date
// separator snaps to the nearest message (the channel pane's
// WithSnapDecorationAnchors).
func (m *Model) BeginSelectionAt(viewportY, x int) {
	if viewportY < m.chromeHeight {
		return
	}
	m.list.BeginSelection(m.list.AbsLine(viewportY-m.chromeHeight), x)
}

// ExtendSelectionAt updates the End anchor of the active selection.
func (m *Model) ExtendSelectionAt(viewportY, x int) {
	m.list.ExtendSelection(m.list.AbsLine(viewportY-m.chromeHeight), x)
}

// EndSelection finalizes the drag, returning the plain-text contents of
// the selection. Returns ok=false when the selection is empty (a click
// without drag). The selection itself remains visible until ClearSelection
// is called or a new drag begins.
func (m *Model) EndSelection() (string, bool) { return m.list.EndSelection() }

// ClearSelection removes the current selection, if any.
func (m *Model) ClearSelection() { m.list.ClearSelection() }

// HasSelection reports whether a selection is currently active or
// pinned-on-screen post-drag.
func (m *Model) HasSelection() bool { return m.list.HasSelection() }

// SelectionText extracts the plain-text contents of the current
// selection.
func (m *Model) SelectionText() string { return m.list.SelectionText() }

// ScrollHintForDrag returns -1 if the cursor is within 1 row of the top
// edge of the message-content area, +1 if within 1 row of the bottom, else 0.
func (m *Model) ScrollHintForDrag(viewportY int) int {
	return m.list.ScrollHint(viewportY - m.chromeHeight)
}

// selectedLines returns the rows to draw for the selected entry i: the
// long-timestamp variant, or the short one while a text selection exists.
// The drag overlay splices the plain text, which holds the short form,
// into the displayed row by column; the long header would come out
// garbled. The short variant is memoised in m.cache[i].
func (m *Model) selectedLines(i int) []string {
	e := &m.list.Entries()[i]
	p := &m.cache[i]
	if !m.list.HasSelection() || p.shortSrc == "" {
		return e.Selected
	}
	if p.linesSelectedShort == nil {
		short := m.selectedVariant(p.shortSrc, m.cacheWidth, m.borderSelectStyle())
		if len(short) > len(e.Selected) {
			return e.Selected // unreachable: variants share a height
		}
		// Trailing spacer rows are shared by every variant.
		p.linesSelectedShort = append(short, e.Selected[len(short):]...)
	}
	return p.linesSelectedShort
}

// viewInternal is the body shared by View and ViewBare. When
// applySelection is true, the selection-highlight overlay is applied
// to the inner content before the scrollbar overlay. When false, the
// overlay is skipped so the App-layer cache can store a selection-free
// bordered output and run ApplySelectionToBordered as a cheap post-pass.
func (m *Model) viewInternal(height, width int, applySelection bool) string {
	m.list.SetStyles(themeListStyles())
	// Chrome (header + separator) is cached; only rebuilt on width / channel
	// name / topic change. This avoids per-keypress strings.Repeat + lipgloss
	// renders that don't depend on the selection.
	if !m.chromeCacheValid || m.chromeWidth != width || m.chromeChannel != m.channelName || m.chromeTopic != m.channelTopic || m.chromeChannelType != m.channelType {
		// Channel title sits in the message pane, immediately inside the
		// panel's top border. Bold + TextPrimary on Background matches
		// the surrounding messages. The panel border itself provides
		// visual separation from the sidebar -- we deliberately do NOT
		// emit a horizontal separator below the title here, because that
		// produced a "border above the channel name and another border-
		// looking line below it" effect that visually separated the
		// header from the messages and made the title look like it sat
		// outside the panel.
		headerStyle := lipgloss.NewStyle().
			Width(width).
			Background(styles.Background).
			Foreground(styles.TextPrimary).
			Bold(true).
			Padding(0, 1)
		header := headerStyle.Render(fmt.Sprintf("%s %s", ChannelGlyph(m.channelType), m.channelName))
		if m.channelTopic != "" {
			// Width(width) pads every wrapped topic line to the full pane
			// width. This keeps the invariant that EVERY chrome line is
			// exactly `width` display cells, which lets the app layer
			// assemble the pane border by concatenation (no per-frame
			// grapheme re-measurement). See ui.borderedTopPane.
			topicStyle := styles.Timestamp.Width(width).Background(styles.Background)
			header += "\n" + topicStyle.Render(WordWrap(m.channelTopic, width))
		}
		m.chromeCache = header
		m.chromeHeight = lipgloss.Height(m.chromeCache)
		m.chromeWidth = width
		m.chromeChannel = m.channelName
		m.chromeTopic = m.channelTopic
		m.chromeChannelType = m.channelType
		m.chromeCacheValid = true
	}
	chrome := m.chromeCache
	chromeHeight := m.chromeHeight

	msgAreaHeight := height - chromeHeight
	if msgAreaHeight < 1 {
		msgAreaHeight = 1
	}
	// A bottom-anchored pane that got shorter (the compose box grew a
	// row) stays pinned to the bottom: WithPinBottomOnShrink.
	m.list.SetHeight(msgAreaHeight, m.selected)

	if len(m.messages) == 0 {
		text := "No messages yet"
		if m.loading {
			frame := styles.SpinnerChars[m.spinnerFrame%len(styles.SpinnerChars)]
			text = string(frame) + " Loading messages..."
		}
		empty := lipgloss.NewStyle().
			Width(width).
			Height(msgAreaHeight).
			Foreground(styles.TextMuted).
			Background(styles.Background).
			Render(text)
		return chrome + "\n" + empty
	}

	// Rebuild cache if messages or width changed. If only individual
	// message TSes have been marked stale (e.g. by HandleImageReady
	// landing one image at a time), take the targeted partial-rebuild
	// path that re-renders only those slots and reuses every other
	// entry verbatim -- the perf invariant for image bursts. A width
	// change with the previous cache intact re-anchors the scroll
	// position (spec §5.2 U5), which SetEntries does.
	switch {
	case m.cache == nil || m.cacheWidth != width || m.cacheMsgLen != len(m.messages):
		// Perf instrumentation: classify which predicate fired so we
		// can attribute focus-flip churn vs width-resize vs new-message
		// rebuilds in slk-debug.log. SLK_DEBUG=1 only; zero cost
		// otherwise (debuglog.Enabled() is an atomic.Bool load).
		var reason string
		if debuglog.Enabled() {
			switch {
			case m.cache == nil:
				reason = "cache-nil"
			case m.cacheWidth != width:
				reason = fmt.Sprintf("width-changed %d->%d", m.cacheWidth, width)
			default:
				reason = fmt.Sprintf("len-changed %d->%d", m.cacheMsgLen, len(m.messages))
			}
		}
		start := time.Now()
		m.buildCache(width, m.cache != nil)
		debuglog.Perf("messages.buildCache N=%d width=%d took=%s reason=%s",
			len(m.messages), width, time.Since(start), reason)
	case len(m.staleEntries) > 0:
		stale := len(m.staleEntries)
		start := time.Now()
		m.partialRebuild(width)
		debuglog.Perf("messages.partialRebuild N=%d stale=%d width=%d took=%s",
			len(m.messages), stale, width, time.Since(start))
	}

	// Snap to the selection if it moved, clamp, let the cursor follow the
	// scroll (WithCursorFollowsScroll), and slice the visible rows.
	frame := m.list.Layout(m.selected, m.selectedLines)
	m.selected = frame.Cursor

	// Per-frame image side effects:
	//   * Kitty image uploads (entry.flushes) are collected from every
	//     visible entry and written straight to the terminal (below).
	//   * Sixel images are painted out-of-band when the image's full
	//     vertical extent is in the window; under partial visibility, or
	//     where an indicator row would cover it, the per-row halfblock
	//     fallback substitutes for the visible rows.
	//
	// kittyFlushBuf collects the APC upload bytes for any kitty images
	// that need uploading this frame. They are written DIRECTLY to the
	// terminal output rather than embedded in the View() string —
	// lipgloss / bubbletea v2's renderer is known to mangle APC sequences
	// embedded in line content.
	var kittyFlushBuf bytes.Buffer
	var sixelFallback map[int]string
	// Placements for this frame, pane-local. Reset every render: the App
	// reconciles the full set against what is currently on screen, so a
	// stale entry here would keep a vanished image alive.
	pendingSixel := make([]imgpkg.SixelPaint, 0, 2)
	yOffset := m.list.YOffset()
	offsets := m.list.Offsets()
	for _, v := range frame.Visible {
		p := &m.cache[v.Index]
		for _, fl := range p.flushes {
			if fl != nil {
				_ = fl(&kittyFlushBuf)
			}
		}
		entryStart := offsets[v.Index]
		for startRowInEntry, sx := range p.sixelRows {
			absStart := entryStart + startRowInEntry
			absEnd := absStart + sx.height
			fullyVisible := absStart >= yOffset && absEnd <= yOffset+msgAreaHeight
			windowRow := v.Row + (startRowInEntry - v.From)
			// Indicator rows replace window row 0 (loading hint) and
			// row msgAreaHeight-1 (more below). An image whose footprint
			// includes either is not fully paintable: the indicator is
			// authoritative and must stay visible, so the image takes
			// the partial-visibility fallback path instead.
			intersectsLoading := m.loading && windowRow <= 0 && windowRow+sx.height > 0
			intersectsMoreBelow := frame.MoreBelow && windowRow <= msgAreaHeight-1 && windowRow+sx.height > msgAreaHeight-1
			if fullyVisible && !intersectsLoading && !intersectsMoreBelow {
				if windowRow >= 0 && windowRow < len(frame.Rows) {
					// Record the placement instead of appending the bytes
					// to the line; the App paints these out-of-band after
					// the frame is flushed. See SixelPlacements.
					pendingSixel = append(pendingSixel, imgpkg.SixelPaint{
						Key:   imgpkg.PlacementKey(sx.key, sx.bytes, sx.width, sx.height),
						Row:   windowRow,
						Col:   sx.col,
						Rows:  sx.height,
						Cols:  sx.width,
						Bytes: sx.bytes,
					})
				}
				continue
			}
			for k := 0; k < sx.height; k++ {
				abs := absStart + k
				if abs < yOffset || abs >= yOffset+msgAreaHeight {
					continue
				}
				row := abs - yOffset
				if row >= 0 && row < len(frame.Rows) && k < len(sx.fallback) {
					if sixelFallback == nil {
						sixelFallback = make(map[int]string)
					}
					sixelFallback[row] = sx.fallback[k]
				}
			}
		}
	}
	for row, fb := range sixelFallback {
		if fb != "" {
			frame.Rows[row] = fb
		}
	}
	if kittyFlushBuf.Len() > 0 {
		_, _ = imgpkg.KittyOutput.Write(kittyFlushBuf.Bytes())
	}

	// Pad to the pane height, put the loading hint on the first row and
	// "-- more below --" on the last (when content continues below),
	// overlay the selection unless the App paints it as a post-pass, and
	// draw the scrollbar. The loading hint is rendered fresh per frame:
	// it MUST reflect the current spinner frame.
	top := ""
	if m.loading {
		top = m.renderLoadingOlderHint(width)
	}
	visible := m.list.Compose(frame, top, m.cacheMoreBelow, applySelection)

	m.sixelPaints = pendingSixel
	return chrome + "\n" + strings.Join(visible, "\n")
}

// View renders the messages pane with the selection-highlight overlay
// applied to the inner content (unbordered). Direct callers (tests,
// callers not going through the App-layer bordered cache) should call
// View; the App layer calls ViewBare instead so the cached bordered
// output stays selection-free and the cheap ApplySelectionToBordered
// post-pass paints the selection at draw time.
func (m *Model) View(height, width int) string {
	return m.viewInternal(height, width, true)
}

// ViewBare renders the messages pane WITHOUT the selection-highlight
// overlay. Used by the App-layer bordered render cache: caching the
// selection-free output and applying the overlay via
// ApplySelectionToBordered as a post-pass means a mouse-drag selection
// no longer invalidates the bordered render every cell of motion.
func (m *Model) ViewBare(height, width int) string {
	return m.viewInternal(height, width, false)
}

// ApplySelectionToBordered overlays the active selection on top of
// a pre-bordered, pre-cached pane output produced by the App layer.
// Splitting the overlay out of View() lets the App layer cache the
// (selection-free) bordered output keyed on messagepane.Version()
// and survive selection-only mutations.
//
// topBorderRows is the number of border rows above the chrome
// (1 for the standard top-bordered panel). leftBorderCols is the
// number of border columns at the start of each row (1 for the
// standard side-bordered panel). Indicator rows are left alone.
func (m *Model) ApplySelectionToBordered(bordered string, topBorderRows, leftBorderCols int) string {
	m.list.SetStyles(themeListStyles())
	return m.list.ApplySelectionToBordered(bordered, topBorderRows+m.chromeHeight, leftBorderCols)
}
```

Note `ClickAt` reads `contentY + m.list.YOffset()` **unclamped**: the channel
pane's click below the content misses (`TestChar_MessagesClickBelowContentMisses`),
unlike thread's.

- [ ] **Step 6: Build**

Run: `go build ./... && go vet ./internal/ui/messages`
Expected: the build passes; `vet` lists the test files' reaches into moved state.

- [ ] **Step 7: Rewrite the test reaches (state access only)**

Apply these rules to the seven test files listed above. They change how a test
reaches state, never what it asserts:

| Old reach | New reach |
|---|---|
| `range m.cache {` (iterating rendered entries) | `range m.list.Entries() {` |
| `e.linesNormal`, `e.linesSelected`, `e.linesPlain`, `e.msgIdx`, `e.height` | `e.Lines`, `e.Selected`, `e.Plain`, `e.Ref`, `len(e.Lines)` |
| `m.cache[i].linesNormal` / `.height` | `m.list.Entries()[i].Lines` / `len(m.list.Entries()[i].Lines)` |
| `e.flushes`, `e.linesSelectedShort` (payload) | iterate `m.cache` instead (`for _, e := range m.cache`, or `m.cache[i]`) |
| `e.reactionHits` | `reactionHitsOf(e)` (helper below); `.rowStartInEntry` → `.RowStart` |
| `m.yOffset` (read) / `m.yOffset = x` | `m.list.YOffset()` / `m.list.SetYOffset(x)` |
| `m.totalLines`, `m.entryOffsets`, `m.lastViewHeight` | `m.list.Total()`, `m.list.Offsets()`, `m.list.Height()` |
| `m.hasSnapped` | `m.list.HasSnapped()` |
| `m.selRange` | `m.list.Selection()` |
| `m.hasSelection` | `m.HasSelection()` |
| `m.resolveAnchor(a)` | `m.list.Resolve(a)` |
| `m.selectedStartLine`, `m.selectedEndLine` | `cursorStart(m)`, `cursorEnd(m)` (helpers below; `selectionVisible` becomes `start, end := m.list.CursorSpan(); return start < bottom && end > top`) |
| `x, ok := m.messageIDToEntryIdx["ts"]; if !ok {` | `x := m.entryIdx("ts"); if x < 0 {` |
| `m.messageIDToEntryIdx["ts"]` (as an index) | `m.entryIdx("ts")` |
| `m.lastHits`, `m.lastReactionHits` | `frameHits(m.list, entrylist.HitImage)`, `frameHits(m.list, entrylist.HitReaction)`; fields `rowStart`/`rowEnd`/`colStart`/`colEnd`/`fileID`/`msgIdx`/`attIdx` → `RowStart`/`RowEnd`/`ColStart`/`ColEnd`/`Key`/`Ref`/`Aux` |
| `m.buildCache(w)` | `m.buildCache(w, false)` |
| `selectedEntry` helper returning `viewEntry` | returns `entrylist.Entry`, matching `e.Selectable && e.Ref == m.selected` |
| **any read of `m.version`** | `m.Version()`: scrolling and selection now bump the substrate's counter, so a raw `m.version` read would pass vacuously |

Append the helpers to `model_test.go`:

```go
// entryIdx is the list index of the entry for ts (-1 when absent), for
// tests that used to read messageIDToEntryIdx directly.
func (m *Model) entryIdx(ts string) int {
	i, ok := m.list.IndexOf(ts)
	if !ok {
		return -1
	}
	return i
}

// frameHits is the last frame's hits of kind, for tests that used to read
// lastHits / lastReactionHits.
func frameHits(l *entrylist.Model, kind entrylist.HitKind) []entrylist.FrameHit {
	var out []entrylist.FrameHit
	for _, h := range l.Hits() {
		if h.Kind == kind {
			out = append(out, h)
		}
	}
	return out
}

// reactionHitsOf is an entry's reaction hits.
func reactionHitsOf(e entrylist.Entry) []entrylist.Hit {
	var out []entrylist.Hit
	for _, h := range e.Hits {
		if h.Kind == entrylist.HitReaction {
			out = append(out, h)
		}
	}
	return out
}

func cursorStart(m *Model) int { s, _ := m.list.CursorSpan(); return s }
func cursorEnd(m *Model) int   { _, e := m.list.CursorSpan(); return e }
```

Count the edits as you go (`git diff --stat` on the test files); record the number
in the commit message against the §8 budget (≈100 for this task).

- [ ] **Step 8: Run everything that could notice**

```bash
go test ./internal/ui/messages -count=1
go test ./internal/ui -run 'TestGolden|TestChar|TestLockstep' -count=1
go test ./internal/ui/thread -count=1
```

Expected: all PASS; goldens byte-identical (the `TestGolden` run would fail with
"STYLING differs" otherwise; never re-bless in this PR).

- [ ] **Step 9: Benches, gate, commit**

Run the Task 0.0 step 4 benchmark commands and compare with the baseline: each
messages benchmark within ±10%. Then the full gate.

```bash
git add internal/ui/messages
git commit -m "refactor(messages): adopt entrylist for scroll, window and selection

The channel pane's offsets, scroll, snapping, window, indicators,
selection and hit translation now live in internal/bubbles/entrylist,
configured with the channel pane's options. Goldens byte-identical.
Test reaches into moved state: <N> edits (spec §8 budget)."
```

---

## PR 5 — The thread pane adopts `entrylist`

Branch `refactor/phase3/pr5-thread-entrylist`, from PR 4's tip. Goldens:
byte-identical to PR 2+3's (no golden may change in this PR).

Validated the same way as PR 4: on the throwaway copy every `thread` and
`internal/ui` test passed (PR 0's pins included), goldens were byte-identical to
the post-PR 2+3 set, `go test ./... -race` and `golangci-lint` were clean, and
thread's `BenchmarkViewScroll` went from 3.80 ms to 0.49 ms per frame (the
per-frame `viewport` width pass and content re-split are gone).

### Task 5.1: Thread's parent, replies and decorations become list entries

**Files:**
- Modify: `internal/ui/thread/model.go`
- Modify: `internal/ui/reducer_mouse.go` (thread reaction hit test: subtract the chrome)
- Modify: `internal/ui/thread/lockstep_test.go` (delete the tripwire; update
  divergences 6, 7, 14)
- Modify (state reach only): `model_test.go`, `open_bottom_test.go`,
  `parent_selection_test.go`, `phase3_char_test.go`,
  `selected_timestamp_test.go`, `selection_test.go` (all in `internal/ui/thread`)

**Interfaces:**
- Consumes: Task 4.1's `entrylist` API; `messages.ThemeListStyles()` (Task 4.2).
- Produces: `(*thread.Model).ChromeHeight() int` (exported now: the App needs it
  to convert pane rows to content rows); `thread.HitTestReaction` takes a
  **content-relative** row (divergence 6 converged on the channel pane's frame).

How the thread pane maps onto the substrate:

| Thread concept | As list entries |
|---|---|
| Parent message + the `─` rule under it | entry 0: `Ref: parentSelected` (−1), `Selectable: true`, `Lines` = the parent rows **as drawn for the current state** plus the rule row, `Selected: nil`, `Plain` = the parent's plain rows only (so the rule is not selectable, and is not a click target), `Plain: nil` when the parent has no ts. Re-rendered every `View`, as today |
| Reply `i` | `Ref: i`, `Selectable: true`, `Lines`/`Selected` = the cached normal/selected variants, `Plain` = all rows, `Hits` = its reaction pills |
| Day divider, `── new ──` landmark, rule between replies | decorations: `Ref: -1`, one row, no `Plain` |
| "No replies yet" block | one decoration entry of `emptyHeight` rows, after the parent; laid out with `Static()` (no snap, no hits, no overlay) |
| `viewCacheValid` (gates width re-anchoring) | `layoutValid`, cleared by the same events, passed as `keepPosition` |
| `m.cache = nil` | `dropCache()`: also `KeepLookups(1)` (lookups see only the parent until the next `View`) |
| kitty uploads on view-cache rebuild | same trigger, kept in `flushKey` |

- [ ] **Step 1: Replace the types and the state**

In `internal/ui/thread/model.go`, replace the `viewEntry`, `reactionEntryHit` and
`reactionHitRect` declarations (from `// viewEntry is a pre-rendered reply.` up to
`// Model represents the thread panel UI component.`) with:

```go
// viewEntry is the thread pane's payload for one reply, index-aligned
// with m.replies (and with m.replyEntries): the parts the list substrate
// does not need. m.cache == nil means the reply cache was discarded and
// the next View rebuilds it.
type viewEntry struct {
	// shortSrc is the selected reply's content with the short timestamp,
	// set only when the selected variant carries a long one;
	// linesSelectedShort is built from it on demand (selectedLines) and
	// shown while a text selection exists.
	shortSrc           string
	linesSelectedShort []string

	// flushes are per-frame kitty image uploads for the reply's inline
	// images, invoked when the frame's layout is rebuilt. v1 scope: sixel
	// renders its placeholder only, so no sixel rows are captured.
	flushes []func(io.Writer) error
}

// reactionEntryHit is one reaction-pill hit-rect relative to a rendered
// reply, carrying the emoji name the click toggles.
type reactionEntryHit struct {
	rowStartInEntry int
	rowEndInEntry   int // exclusive
	colStart        int
	colEnd          int // exclusive
	emoji           string
}
```

In `Model`, delete the PR 2+3 offset fields (`yOffset`, `contentLines`,
`areaHeight` and their comment), and replace everything from
`	// Render cache -- pre-rendered reply entries` up to
`	// version increments on every state change` with:

```go
	// Reply cache: the payload and the list entry of every reply,
	// rebuilt when the replies or the width change.
	cache         []viewEntry
	replyEntries  []entrylist.Entry
	cacheWidth    int
	cacheReplyLen int

	// list is the shared substrate: entries (the parent, the replies and
	// the decoration rows between them), scroll, the visible window, text
	// selection, hit translation and the scrollbar. Configured for the
	// thread pane's behaviour in New.
	list *entrylist.Model

	// layoutValid says the previous frame's layout still describes the
	// screen: a width change then keeps the scroll position (U5). Cleared
	// by every invalidation, by SetFocused, SetUnreadBoundary and ClickAt
	// (the events that cleared the old view cache).
	layoutValid bool

	// flushKey is the layout the reply flushes were last written for;
	// kitty uploads are re-emitted only when it changes, as when the old
	// view cache was rebuilt.
	flushKey struct {
		selected, width, height int
		hasSelection            bool
	}

	// chromeCache holds the rendered "header + separator" prefix that
	// View() prepends to the reply area. Rebuilt only when its inputs
	// (width, replyCount, userNames, channelNames, breadcrumb) change.
	chromeCache         string
	chromeCacheValid    bool
	chromeWidth         int
	chromeReplyCount    int
	chromeUserNamesV    uint64 // version of the userNames map at build time
	chromeChannelNamesV uint64 // version of the channelNames map at build time

	// Breadcrumb inputs (SetBreadcrumb), and the values the chrome
	// cache was last built from.
	crumbChannel       string
	crumbType          string
	chromeCrumbChannel string
	chromeCrumbType    string
	chromeAuthor       string

	// userNamesV / channelNamesV are bumped every time SetUserNames /
	// SetChannelNames replaces the map. Used by chromeCache (and any other
	// cache that depends on these maps) to detect changes without hashing.
	userNamesV    uint64
	channelNamesV uint64
	userGroups    map[string]string

	// replyIDToIdx maps reply TS -> reply index; rebuilt with the reply
	// cache. HasReply reads it.
	replyIDToIdx map[string]int
	// chromeHeight is the number of rows the thread's chrome (header +
	// separator) occupies at the top of View()'s output. Click and drag
	// handlers subtract it from pane-local rows.
	chromeHeight int

	// unreadBoundaryTS is the Slack timestamp the user has already read up
	// to in this thread. Replies whose TS > unreadBoundaryTS are considered
	// new; a "── new ──" landmark is inserted between the last read reply
	// and the first new one. Empty string disables the landmark. Set via
	// SetUnreadBoundary, typically with the parent channel's last_read_ts
	// at the moment the thread is opened.
	unreadBoundaryTS string
```

(This drops `entryOffsets`, `totalLines`, `parentEntry`, `viewContent`,
`viewSelected`, `viewHasSelection`, `viewWidth`, `viewHeight`, `viewCacheValid`,
`selectedStartLine`, `selectedEndLine`, `selRange`, `hasSelection`,
`lastViewHeight`, `lastReactionHits`, `snappedSelection` and `hasSnapped`.)

Imports: replace `bubbles/scrollbar` and `bubbles/selection` with
`"github.com/gammons/slk/internal/bubbles/entrylist"`; replace
`"github.com/charmbracelet/x/ansi"` with `"math"`.

- [ ] **Step 2: Construction, version, invalidation, scrolling**

```go
func New() *Model {
	return &Model{
		imgRenderer: imgrender.NewRenderer(),
		list:        newThreadList(),
	}
}

// newThreadList configures the substrate for the thread pane (spec §5):
// the offset clamps eagerly, as bubbles/viewport did (U6); every drag
// step bumps Version (U4); a width change anchors only on replies, so a
// first visible line inside the parent keeps its raw offset (#254) and
// "at the bottom" is measured with the previous frame's height (U5). The
// cursor does not follow the scroll (U1), a shrinking pane is not pinned
// (U2) and a press on a separator anchors nothing (8c): the defaults.
func newThreadList() *entrylist.Model {
	return entrylist.New(
		entrylist.WithStyles(messages.ThemeListStyles()),
		entrylist.WithEagerClamp(),
		entrylist.WithDirtyOnDrag(),
		entrylist.WithReanchor(entrylist.ReanchorPolicy{AnchorRefsOnly: true}),
	)
}

// dropCache discards the reply cache. Until the next View, lookups see
// only the parent, whose entry stays current.
func (m *Model) dropCache() {
	m.cache = nil
	m.replyEntries = nil
	m.list.KeepLookups(1)
}
```

```go
func (m *Model) Version() int64 { return m.version + m.list.Version() }

// ChromeHeight returns the number of rows the breadcrumb chrome occupies
// at the top of the pane, as of the last View. The app-level mouse
// handler subtracts it from a pane-local row before HitTestReaction.
func (m *Model) ChromeHeight() int { return m.chromeHeight }
```

Then:
- `InvalidateCache`: `m.cache = nil; m.viewCacheValid = false` → `m.dropCache(); m.layoutValid = false`.
- `invalidateAuthors` and `PatchUserName`: the same pair, the same replacement.
- `SetThread`: `m.hasSnapped = false` → `m.list.ResetSnap()`.
- `SetUnreadBoundary`, `SetFocused`: `m.viewCacheValid = false` → `m.layoutValid = false`.
  (`SetFocused` still does **not** drop the reply cache: plan decision P2.)
- `ViewportAtTop`: `return m.list.YOffset() == 0`.
- `ScrollUp` / `ScrollDown` (keep their doc comments):

```go
func (m *Model) ScrollUp(n int) {
	m.list.ScrollUp(n, m.selected)
}

func (m *Model) ScrollDown(n int) {
	m.list.ScrollDown(n, m.selected)
}
```

- [ ] **Step 3: Clicks and selection through the substrate**

Replace everything from `// ClickAt handles a mouse click at the given
y-coordinate` up to (not including) `// View renders the thread panel content
without a border.` with the following. It removes `absoluteLineAt`,
`selectionEntryAt`, `anchorAt`, `resolveAnchor`, `appendSelectionText`,
`applySelectionOverlay`, `entryAndOffsetForLine` and PR 2+3's `maxYOffset`,
`setYOffset`, `setContent`, `windowLines`:

```go
// ClickAt handles a mouse click at the given y-coordinate (the pane-local
// y returned by App.panelAt — measured from the panel's top border, so
// y=0..chromeHeight-1 sits inside the chrome (header + separator) and
// y=chromeHeight onward is the scrolling parent+replies content). Clicks
// in the chrome are ignored. A click below the content lands on the last
// line (the row is clamped), so it selects the last reply; a click on a
// separator, a divider or the rule under the parent selects nothing.
func (m *Model) ClickAt(y int) {
	if y < m.chromeHeight {
		return
	}
	i, within, ok := m.list.EntryAt(m.list.AbsLine(y - m.chromeHeight))
	if !ok {
		return
	}
	e := m.list.Entries()[i]
	if !e.Selectable || within >= len(e.Plain) {
		return
	}
	if m.selected != e.Ref {
		m.selected = e.Ref
		m.layoutValid = false
		m.dirty()
	}
}

// BeginSelectionAt anchors a new selection at the given pane-local
// coordinates (App.panelAt's coordinate system). Chrome rows are ignored;
// a press on a separator or divider anchors nothing (thread does not snap
// to the nearest message).
func (m *Model) BeginSelectionAt(viewportY, x int) {
	if viewportY < m.chromeHeight {
		return
	}
	m.list.BeginSelection(m.list.AbsLine(viewportY-m.chromeHeight), x)
}

// ExtendSelectionAt updates the End anchor of the active selection.
// No-op if BeginSelectionAt was never called or the coordinates fall
// on a non-entry row (inter-reply separator).
func (m *Model) ExtendSelectionAt(viewportY, x int) {
	m.list.ExtendSelection(m.list.AbsLine(viewportY-m.chromeHeight), x)
}

// EndSelection finalizes the drag, returning the plain-text contents
// of the selection. Returns ok=false when the selection is empty
// (a click without drag).
func (m *Model) EndSelection() (string, bool) { return m.list.EndSelection() }

// ClearSelection removes the current selection, if any.
func (m *Model) ClearSelection() { m.list.ClearSelection() }

// HasSelection reports whether a selection is currently active or
// pinned-on-screen post-drag.
func (m *Model) HasSelection() bool { return m.list.HasSelection() }

// SelectionText extracts the plain-text contents of the current
// selection.
func (m *Model) SelectionText() string { return m.list.SelectionText() }

// selectedLines returns the rows to draw for the cursor's entry i. For a
// reply: the long-timestamp variant, or the short one while a text
// selection exists (the drag overlay splices the short-form plain text
// into the row by column). The parent's rows are re-rendered every frame
// for its current state, so they are drawn as they are.
//
// The short variant is memoised in m.cache.
func (m *Model) selectedLines(i int) []string {
	e := &m.list.Entries()[i]
	if e.Ref < 0 || e.Ref >= len(m.cache) {
		return e.Lines
	}
	p := &m.cache[e.Ref]
	if !m.list.HasSelection() || p.shortSrc == "" {
		return e.Selected
	}
	if p.linesSelectedShort == nil {
		short := m.selectedVariant(p.shortSrc, m.cacheWidth, m.borderSelectStyle())
		if len(short) != len(e.Selected) {
			return e.Selected // unreachable: variants share a height
		}
		p.linesSelectedShort = short
	}
	return p.linesSelectedShort
}

// borderSelectStyle is the thick, tinted left border of the selected reply.
func (m *Model) borderSelectStyle() lipgloss.Style {
	return lipgloss.NewStyle().BorderStyle(thickLeftBorder).BorderLeft(true).
		BorderForeground(styles.SelectionBorderColor(m.focused)).
		BorderBackground(styles.SelectionTintColor(m.focused)).
		Background(styles.SelectionTintColor(m.focused))
}

// selectedVariant renders content r as the selected reply: the selection
// tint repainted over inner spans, filled to the row width, then the
// selection border. Split on "\n". Mirrors messages.Model.selectedVariant.
func (m *Model) selectedVariant(r string, width int, borderSelect lipgloss.Style) []string {
	fill := lipgloss.NewStyle().Background(styles.SelectionTintColor(m.focused)).Width(width - 1)
	return strings.Split(borderSelect.Render(fill.Render(messages.RepaintBgToSelectionTint(r, m.focused))), "\n")
}

// ScrollHintForDrag returns -1 if the cursor is within 1 row of the top
// edge of the scrolling content, +1 if within 1 row of the bottom, else 0.
// The incoming viewportY is pane-local; a cursor on the chrome counts as
// the top content row, so an upward drag keeps auto-scrolling.
func (m *Model) ScrollHintForDrag(viewportY int) int {
	return m.list.ScrollHint(viewportY - m.chromeHeight)
}
```

- [ ] **Step 4: The frame**

In `View`, keep everything up to and including `chromeHeight := m.chromeHeight`.
Replace the rest of `View`, `HitTestReaction` and `fitHeight` (up to, not
including, `// renderThreadMessage renders a single message`) with:

```go
	// The reply area is everything under the chrome.
	replyAreaHeight := max(height-chromeHeight, 1)
	m.list.SetStyles(messages.ThemeListStyles())
	m.list.SetHeight(replyAreaHeight, m.selected)

	// The parent is rendered every frame for its current state (selected
	// or not, focused or not) and is entry 0 of the list: its rows, then
	// the rule under it. The rule is not selectable text (Plain stops
	// before it). We discard the parent's image flushes and reaction hits
	// in v1, as before.
	parentIsSelected := m.selected == parentSelected
	parentContent, _, _, parentHeader, parentHeaderBudget := m.renderThreadMessage(m.parent, width, m.userNames, m.channelNames, parentIsSelected)
	// Replies only get a day divider when their day differs from the
	// parent's, so the parent header carries its own date; otherwise a
	// thread whose first replies share the parent's day shows no date.
	// Before the plain mirror, so copy and drag highlight match what is
	// drawn. Skipped when the header would not fit, and for a
	// summary-built parent that has no time yet.
	if !parentIsSelected && m.parent.Timestamp != "" {
		if date := messages.DateFromTS(m.parent.TS); date != "" {
			dated := messages.FormatShortDate(date) + ", " + m.parent.Timestamp
			parentContent = messages.ReplaceHeaderTimestamp(parentContent, parentHeader, m.parent.Timestamp, dated, parentHeaderBudget)
		}
	}
	parentPlain := messages.PlainLines(parentContent)
	// Selection border for the parent row mirrors the per-reply
	// cache-build treatment (borderSelect / borderInvis below): thick
	// left border + tint when the cursor sits on the parent, invisible
	// border otherwise so the parent's width matches the reply rows.
	if parentIsSelected {
		// After the plain mirror, so it keeps the short form. Skipped
		// while a text selection exists; see selectedLines.
		if !m.list.HasSelection() {
			parentContent = messages.SelectedHeader(parentContent, parentHeader, m.parent.TS, m.parent.Timestamp, parentHeaderBudget)
		}
		parentContent = lipgloss.NewStyle().BorderStyle(thickLeftBorder).BorderLeft(true).
			BorderForeground(styles.SelectionBorderColor(m.focused)).
			BorderBackground(styles.SelectionTintColor(m.focused)).
			Background(styles.SelectionTintColor(m.focused)).
			Render(parentContent)
	} else {
		parentContent = lipgloss.NewStyle().BorderStyle(thickLeftBorder).BorderLeft(true).
			BorderForeground(styles.Background).BorderBackground(styles.Background).
			Render(parentContent)
	}
	// Pad every parent row to the pane width with the theme background.
	// The outer wrapper used to do this (the parent, unlike a reply, is
	// not width-filled when rendered); this is the same per-row padding,
	// applied where it is needed.
	parentContent = lipgloss.NewStyle().Width(width).Background(styles.Background).Render(parentContent)
	parentSeparator := lipgloss.NewStyle().
		Width(width).
		Background(styles.Background).
		Foreground(styles.Border).
		Render(strings.Repeat("─", width))
	parent := entrylist.Entry{
		ID:         m.parent.TS,
		Ref:        parentSelected,
		Selectable: true,
		Lines:      append(strings.Split(parentContent, "\n"), parentSeparator),
		ColOffset:  1,
	}
	if m.parent.TS != "" {
		// A parent without a ts cannot be clicked or text-selected.
		parent.Plain = parentPlain
	}

	if len(m.replies) == 0 {
		// "No replies yet" under the parent. It goes through the list so
		// a parent taller than the pane still scrolls; the scrollbar
		// reflects the parent's own height. No snap, no hits, no
		// selection overlay: as before (plan decision P1 for the totals).
		emptyHeight := max(replyAreaHeight-len(parent.Lines), 1)
		empty := lipgloss.NewStyle().
			Width(width).
			Height(emptyHeight).
			Background(styles.Background).
			Foreground(styles.TextMuted).
			Render("No replies yet")
		m.list.SetEntries([]entrylist.Entry{parent, {Ref: -1, Lines: strings.Split(empty, "\n")}}, width, false)
		rows := m.list.Compose(m.list.Static(), "", "", false)
		return fitHeight(chrome+"\n"+strings.Join(rows, "\n"), height)
	}

	// A width change with the previous layout intact keeps the scroll
	// position (SetEntries, per the reanchor policy). Captured before the
	// reply-cache rebuild.
	keepPosition := m.layoutValid

	// Rebuild the reply cache if the replies or the width changed.
	//
	// The predicate intentionally excludes m.selected, m.focused,
	// m.reactionNavActive/Index and the theme version. The cache holds
	// renderThreadMessage output, which depends on those fields, so any
	// state that can change rendered output for a given (reply, width)
	// MUST call InvalidateCache() on its mutation path. EnterReactionNav /
	// ExitReactionNav / ReactionNavLeft / ReactionNavRight all do;
	// MoveUp/MoveDown call ExitReactionNav before mutating selected.
	// (SetFocused does not: plan decision P2, a known stale-border bug.)
	if m.cache == nil || m.cacheWidth != width || m.cacheReplyLen != len(m.replies) {
		var perfReason string
		var perfStart time.Time
		if debuglog.Enabled() {
			switch {
			case m.cache == nil:
				perfReason = "cache-nil"
			case m.cacheWidth != width:
				perfReason = fmt.Sprintf("width-changed %d->%d", m.cacheWidth, width)
			default:
				perfReason = fmt.Sprintf("len-changed %d->%d", m.cacheReplyLen, len(m.replies))
			}
			perfStart = time.Now()
		}
		m.cache = m.cache[:0]
		m.replyEntries = m.replyEntries[:0]
		if m.replyIDToIdx == nil {
			m.replyIDToIdx = make(map[string]int, len(m.replies))
		} else {
			clear(m.replyIDToIdx)
		}
		// Pre-build border styles ONCE (don't allocate per reply): the
		// thick left border ▌ is applied at cache-build time so View()
		// picks a slice (normal vs selected) instead of running lipgloss
		// for every visible reply on every j/k.
		borderFill := lipgloss.NewStyle().Background(styles.Background)
		borderInvis := lipgloss.NewStyle().BorderStyle(thickLeftBorder).BorderLeft(true).
			BorderForeground(styles.Background).BorderBackground(styles.Background)
		borderSelect := m.borderSelectStyle()
		for i, reply := range m.replies {
			// renderThreadMessage's last arg ("isSelected") drives reaction-
			// nav pill highlighting, so it MUST be i == m.selected.
			rendered, attachFlushes, reactHits, header, headerBudget := m.renderThreadMessage(reply, width, m.userNames, m.channelNames, i == m.selected)
			// Selected variant only; Lines / Plain keep the short form.
			renderedSel := messages.SelectedHeader(rendered, header, reply.TS, reply.Timestamp, headerBudget)
			// Two filled variants, so the trailing whitespace of every
			// wrapped line carries the right background and the tint
			// reaches the end of the row. Plain mirrors the untinted
			// filledNormal, so clipboard text never carries the tint.
			filledNormal := borderFill.Width(width - 1).Render(rendered)
			linesN := strings.Split(borderInvis.Render(filledNormal), "\n")
			linesS := m.selectedVariant(renderedSel, width, borderSelect)
			// The short-timestamp selected variant is built on demand
			// from shortSrc (see selectedLines).
			var shortSrc string
			if renderedSel != rendered {
				shortSrc = rendered
			}
			hits := make([]entrylist.Hit, 0, len(reactHits))
			for _, h := range reactHits {
				hits = append(hits, entrylist.Hit{
					RowStart: h.rowStartInEntry, RowEnd: h.rowEndInEntry,
					ColStart: h.colStart, ColEnd: h.colEnd,
					Kind: entrylist.HitReaction, Key: h.emoji,
				})
			}
			m.replyEntries = append(m.replyEntries, entrylist.Entry{
				ID:         reply.TS,
				Ref:        i,
				Selectable: true,
				Lines:      linesN,
				Selected:   linesS,
				Plain:      messages.PlainLines(filledNormal),
				ColOffset:  1,
				Hits:       hits,
			})
			m.cache = append(m.cache, viewEntry{shortSrc: shortSrc, flushes: attachFlushes})
			m.replyIDToIdx[reply.TS] = i
		}
		m.cacheWidth = width
		m.cacheReplyLen = len(m.replies)
		m.flushKey.selected = math.MinInt // re-emit the kitty uploads
		if debuglog.Enabled() {
			debuglog.Perf("thread.buildCache N=%d width=%d took=%s reason=%s",
				len(m.replies), width, time.Since(perfStart), perfReason)
		}
	}

	// The list: the parent, then for each reply its day divider (when
	// its local day differs from the previous one, seeded with the
	// parent's), the "── new ──" landmark before the first reply past
	// the unread boundary, the reply, and a full-width rule between
	// replies. Dividers, landmark and rules are decorations: not
	// selectable, not text-selectable.
	ruleStyle := lipgloss.NewStyle().
		Width(width).
		Background(styles.Background).
		Foreground(styles.Border)
	replySeparator := ruleStyle.Render(strings.Repeat("─", width))
	var newLandmark string
	if m.unreadBoundaryTS != "" {
		newLandmark = lipgloss.NewStyle().
			Width(width).
			Background(styles.Background).
			Foreground(styles.Error).
			Bold(true).
			Align(lipgloss.Center).
			Render("── new ──")
	}
	dateSeparatorStyle := lipgloss.NewStyle().
		Width(width).
		Background(styles.Background).
		Foreground(styles.TextMuted).
		Bold(true).
		Align(lipgloss.Center)
	decoration := func(row string) entrylist.Entry { return entrylist.Entry{Ref: -1, Lines: []string{row}} }
	entries := make([]entrylist.Entry, 0, 2*len(m.replyEntries)+2)
	entries = append(entries, parent)
	lastDate := messages.DateFromTS(m.parent.TS)
	landmarkInserted := false
	for i, e := range m.replyEntries {
		if replyDate := messages.DateFromTS(m.replies[i].TS); replyDate != "" && replyDate != lastDate {
			entries = append(entries, decoration(dateSeparatorStyle.Render("── "+messages.FormatDateSeparator(replyDate)+" ──")))
			lastDate = replyDate
		}
		if !landmarkInserted && newLandmark != "" && m.replies[i].TS > m.unreadBoundaryTS {
			entries = append(entries, decoration(newLandmark))
			landmarkInserted = true
		}
		entries = append(entries, e)
		if i < len(m.replyEntries)-1 {
			entries = append(entries, decoration(replySeparator))
		}
	}
	m.list.SetEntries(entries, width, keepPosition)

	// Kitty uploads for every reply (the thread cache is small, so no
	// visibility scoping), written straight to the terminal output: APC
	// sequences embedded in line content get mangled by the renderer.
	// Re-emitted only when the layout changes, as before.
	key := m.flushKey
	key.selected, key.width, key.height, key.hasSelection = m.selected, width, replyAreaHeight, m.list.HasSelection()
	if key != m.flushKey || !m.layoutValid {
		var kittyFlushBuf bytes.Buffer
		for _, p := range m.cache {
			for _, fl := range p.flushes {
				if fl != nil {
					_ = fl(&kittyFlushBuf)
				}
			}
		}
		if kittyFlushBuf.Len() > 0 {
			_, _ = imgpkg.KittyOutput.Write(kittyFlushBuf.Bytes())
		}
		m.flushKey = key
	}

	// Snap to the cursor if it moved, slice the window, translate the
	// reply hits (content-relative), overlay the selection, draw the
	// scrollbar.
	frame := m.list.Layout(m.selected, m.selectedLines)
	rows := m.list.Compose(frame, "", "", true)
	m.layoutValid = true
	return fitHeight(chrome+"\n"+strings.Join(rows, "\n"), height)
}

// HitTestReaction returns the (reply index, reaction emoji name) at
// (row, col) within the thread's content area, or ok=false when no
// reaction pill covers that cell. Coordinate frame: row 0 is the first
// row under the chrome, as for messages.Model.HitTestReaction (the
// app-level mouse handler subtracts ChromeHeight). Hits come from the
// most recent View().
func (m *Model) HitTestReaction(row, col int) (replyIdx int, emoji string, ok bool) {
	h, ok := m.list.HitTest(entrylist.HitReaction, row, col)
	if !ok {
		return 0, "", false
	}
	return h.Ref, h.Key, true
}

// fitHeight keeps the first height rows of s, as the old outer
// MaxHeight(height) did when a floored reply area (height < chrome + 1)
// made the pane one row too tall.
func fitHeight(s string, height int) string {
	if height <= 0 {
		return s
	}
	if rows := strings.Split(s, "\n"); len(rows) > height {
		return strings.Join(rows[:height], "\n")
	}
	return s
}
```

- [ ] **Step 5: The App converts the thread's pane row to a content row**

In `internal/ui/reducer_mouse.go`, thread branch of `reduceMouseClick`, replace
the reaction hit test with:

```go
		// Hit-test reactions first on the thread pane too. Same
		// content-relative frame as the messages pane: subtract the
		// chrome first.
		if contentY := py - a.threadPanel.ChromeHeight(); contentY >= 0 {
			if hitReplyIdx, emojiName, hit := a.threadPanel.HitTestReaction(contentY, px); hit && emojiName != "" {
				replies := a.threadPanel.Replies()
				if hitReplyIdx >= 0 && hitReplyIdx < len(replies) {
					return a.toggleReactionOnMessageItem(a.threadPanel.ChannelID(), replies[hitReplyIdx], emojiName)
				}
			}
		}
```

Check there is no other production caller: `grep -rn 'threadPanel.HitTestReaction' internal --include='*.go'`.

- [ ] **Step 6: Build, then delete the tripwire**

Run: `go build ./... && go test ./internal/ui/thread -run TestLockstep -count=1`
Expected: build passes; `TestLockstep_ReactionHitTestFrames` FAILS with "the
frames have converged -- update divergence 6". That is its purpose. Delete the
test function (from its doc comment through the closing brace of
`lockstepHitColumns`). In the "Documented divergences" comment, rewrite items 6,
7 and 14 to one line each:

```go
//  6. Reaction hit-test row frame. CONVERGED in Phase 3 PR 5: both take a
//     content-relative row; the App subtracts ChromeHeight() for both.
//
//  7. Scroll mechanism. CONVERGED in Phase 3 PR 2+3/5: both scroll through
//     internal/bubbles/entrylist (thread keeps viewport's eager clamp, U6).
//
// 14. Outer background wrapper. REMOVED in Phase 3 PR 2+3.
```

- [ ] **Step 7: Rewrite the test reaches (state access only)**

| Old reach | New reach |
|---|---|
| `m.yOffset`, `m.setYOffset(x)` | `m.list.YOffset()`, `m.list.SetYOffset(x)` |
| `m.areaHeight`, `m.lastViewHeight` | `m.list.Height()` |
| `m.totalLines` | `m.list.Total()` |
| `m.entryOffsets[k]` (reply k) | `replyOffset(m, k)` (helper below) |
| `len(m.entryOffsets)` | `len(m.replyEntries)` |
| `m.cache[k].height` | `len(m.replyEntries[k].Lines)` |
| `e.linesNormal` / `e.linesSelected` over `m.cache` | `e.Lines` / `e.Selected` over `m.replyEntries` |
| `e.linesSelectedShort` | over `m.cache` (payload) |
| `m.parentEntry.linesPlain` | `m.list.Entries()[0].Plain` |
| `m.viewContent` | `joinedContent(m)` |
| `m.lastReactionHits`, `h.rowStart` … | `reactionFrameHits(m)`, `h.RowStart` … |
| `m.selRange`, `m.hasSelection` | `m.list.Selection()`, `m.HasSelection()` |
| `m.hasSnapped = true; m.snappedSelection = m.selected` | `m.list.MarkSnapped(m.selected)` |
| `if m.hasSnapped` | `if m.list.HasSnapped()` |
| a test that **assigns** `m.selRange`/`m.hasSelection` (`TestSelectionTextIncludesParent`) | drive it: `m.BeginSelectionAt(m.chromeHeight+1, 1); m.ExtendSelectionAt(m.chromeHeight+1, 2)` (parent line 1, plain columns [0,1)) |
| **any read of `m.version`** | `m.Version()` |

One assertion changes, by design: `TestHitTestReaction` (model_test.go) checked
that a hit's row is ≥ `chromeHeight` (the old pane-local frame). In the converged
frame the meaningful check is that a reply's pill is below the parent block:

```go
	// Rows are content-relative since Phase 3 (divergence 6 converged
	// on messages' frame): a reply's pill sits below the parent block.
	if parentRows := len(m.list.Entries()[0].Lines); h.RowStart < parentRows {
		t.Errorf("reaction hit rowStart=%d should be >= the parent block's %d rows", h.RowStart, parentRows)
	}
```

Append the helpers to `model_test.go` (and import `bubbles/entrylist` there):

```go
// replyOffset is the absolute line reply k starts on, for tests that used
// to read entryOffsets (which then held only replies).
func replyOffset(m *Model, k int) int {
	i, ok := m.list.IndexOf(m.replies[k].TS)
	if !ok {
		return -1
	}
	return m.list.Offsets()[i]
}

// joinedContent is every laid-out row, for tests that used to read
// viewContent.
func joinedContent(m *Model) string {
	var rows []string
	for _, e := range m.list.Entries() {
		rows = append(rows, e.Lines...)
	}
	return strings.Join(rows, "\n")
}

// reactionFrameHits is the last frame's reaction hits, for tests that
// used to read lastReactionHits.
func reactionFrameHits(m *Model) []entrylist.FrameHit {
	var out []entrylist.FrameHit
	for _, h := range m.list.Hits() {
		if h.Kind == entrylist.HitReaction {
			out = append(out, h)
		}
	}
	return out
}
```

- [ ] **Step 8: Run everything that could notice**

```bash
go test ./internal/ui/thread -count=1
go test ./internal/ui -count=1
go test ./internal/ui -run TestGolden -count=1   # byte-identical to PR 2+3's goldens; never re-bless here
```

Expected: all PASS.

- [ ] **Step 9: Benches, gate, commit**

Run the Task 0.0 step 4 benchmarks: thread `BenchmarkViewScroll` must be no
slower than the baseline (expect several times faster); messages unchanged.
`go mod tidy` must not change `go.mod`/`go.sum`. Full gate.

```bash
git add internal/ui/thread internal/ui/reducer_mouse.go
git commit -m "refactor(thread): adopt entrylist; converge the reaction hit frame

The parent, replies, dividers, landmark and rules become entrylist
entries configured with the thread pane's options (eager clamp, drag
dirties, re-anchor on replies only). HitTestReaction takes a
content-relative row like the channel pane's; the App subtracts the
chrome. TestLockstep_ReactionHitTestFrames is deleted: it fired on
convergence, as designed. Goldens byte-identical.
Test reaches into moved state: <N> edits (spec §8 budget)."
```

---

## PR 6 — One message layer

Branch `refactor/phase3/pr6-message-layer`, from PR 5's tip. Goldens:
byte-identical. Unlike PRs 2+3, 4 and 5, this PR was **not** pre-validated on a
throwaway copy; its safety net is the goldens, both panes' render tests,
`TestLockstep_SharedRenderBehaviour` and PR 0's pins. Run them after every step.

### Task 6.1: Shared reaction mutation and reaction-cursor arithmetic

**Files:**
- Create: `internal/ui/messages/reactions.go`
- Create: `internal/ui/messages/reactions_test.go`
- Modify: `internal/ui/messages/model.go` (`UpdateReaction`, the reaction-nav methods)
- Modify: `internal/ui/thread/model.go` (`UpdateReaction`, `updateMessageReaction`, the reaction-nav methods)

**Interfaces:**
- Produces: `messages.ApplyReaction(msg *MessageItem, emojiName, userID, currentUserID string, remove bool)`;
  `type messages.ReactionNav struct{ Active bool; Index int }` with `Left(n int)`,
  `Right(n int)`, `Clamp(n int)`, `Selected(rs []ReactionItem) (emoji string, isPlus bool)`
  (n = number of reactions; the `+` pill is index n).

The two panes' reaction mutation is the same algorithm (`messages/model.go`
`UpdateReaction` inner loop; `thread/model.go` `updateMessageReaction`), and so is
the cursor arithmetic. What differs stays in the panes: thread invalidates on every
`ExitReactionNav`, the channel pane skips a redundant one (`messages/model.go:1025`).

- [ ] **Step 1: Write the tests**

```go
package messages

import "testing"

func TestApplyReaction(t *testing.T) {
	m := MessageItem{}
	ApplyReaction(&m, "tada", "U1", "U1", false)
	ApplyReaction(&m, "tada", "U1", "U1", false) // duplicate echo: no double count
	if len(m.Reactions) != 1 || m.Reactions[0].Count != 1 || !m.Reactions[0].HasReacted {
		t.Fatalf("add: %+v", m.Reactions)
	}
	ApplyReaction(&m, "tada", "U2", "U1", false)
	if m.Reactions[0].Count != 2 {
		t.Fatalf("second user: %+v", m.Reactions)
	}
	ApplyReaction(&m, "tada", "U9", "U1", true) // not a reactor, list not truncated: ignored
	if m.Reactions[0].Count != 2 {
		t.Fatalf("spurious remove changed the count: %+v", m.Reactions)
	}
	ApplyReaction(&m, "tada", "U1", "U1", true)
	if m.Reactions[0].Count != 1 || m.Reactions[0].HasReacted {
		t.Fatalf("remove own: %+v", m.Reactions)
	}
	ApplyReaction(&m, "tada", "U2", "U1", true)
	if len(m.Reactions) != 0 {
		t.Fatalf("last remove did not drop the pill: %+v", m.Reactions)
	}
	m.Reactions = []ReactionItem{{Emoji: "eyes", Count: 5, UserIDs: []string{"U1"}}}
	ApplyReaction(&m, "eyes", "U7", "U1", true) // truncated list: the removal is real
	if m.Reactions[0].Count != 4 {
		t.Fatalf("truncated remove: %+v", m.Reactions)
	}
}

func TestReactionNav(t *testing.T) {
	var n ReactionNav
	n.Left(2) // two pills + "+": wraps to the "+"
	if n.Index != 2 {
		t.Fatalf("Left wrap: %d", n.Index)
	}
	n.Right(2)
	if n.Index != 0 {
		t.Fatalf("Right wrap: %d", n.Index)
	}
	rs := []ReactionItem{{Emoji: "a"}, {Emoji: "b"}}
	if e, plus := n.Selected(rs); e != "a" || plus {
		t.Fatalf("Selected = %q, %v", e, plus)
	}
	n.Index = 5
	n.Clamp(2)
	if n.Index != 2 {
		t.Fatalf("Clamp: %d", n.Index)
	}
	if _, plus := n.Selected(rs); !plus {
		t.Fatal("index past the pills must be the + pill")
	}
}
```

- [ ] **Step 2: Run, see them fail** — `go test ./internal/ui/messages -run 'TestApplyReaction|TestReactionNav' -count=1` → undefined.

- [ ] **Step 3: Implement `reactions.go`**

```go
package messages

// ApplyReaction applies one reaction event to msg, idempotently: the
// optimistic local update and the WebSocket echo of the same event must
// not double-count. A removal by a user not among the listed reactors is
// ignored, unless Slack truncated the list (Count > len(UserIDs)), in
// which case it is real. HasReacted tracks currentUserID only.
func ApplyReaction(msg *MessageItem, emojiName, userID, currentUserID string, remove bool) {
	if remove {
		for j, r := range msg.Reactions {
			if r.Emoji != emojiName {
				continue
			}
			newIDs := RemoveUserID(r.UserIDs, userID)
			if len(newIDs) == len(r.UserIDs) && r.Count <= len(r.UserIDs) {
				return
			}
			r.UserIDs = newIDs
			r.Count--
			if userID == currentUserID {
				r.HasReacted = false
			}
			if r.Count <= 0 {
				msg.Reactions = append(msg.Reactions[:j], msg.Reactions[j+1:]...)
			} else {
				msg.Reactions[j] = r
			}
			return
		}
		return
	}
	for j, r := range msg.Reactions {
		if r.Emoji != emojiName {
			continue
		}
		newIDs := AppendUserID(r.UserIDs, userID)
		if len(newIDs) != len(r.UserIDs) {
			r.UserIDs = newIDs
			r.Count++
		}
		if userID == currentUserID {
			r.HasReacted = true
		}
		msg.Reactions[j] = r
		return
	}
	msg.Reactions = append(msg.Reactions, ReactionItem{
		Emoji:      emojiName,
		Count:      1,
		HasReacted: userID == currentUserID,
		UserIDs:    AppendUserID(nil, userID),
	})
}

// ReactionNav is the reaction-pill cursor a pane keeps for its selected
// message. Index counts pills; index n (the number of reactions) is the
// trailing "+" pill. Invalidation stays with the pane.
type ReactionNav struct {
	Active bool
	Index  int
}

// Left moves one pill left over n reactions plus "+", wrapping.
func (r *ReactionNav) Left(n int) { total := n + 1; r.Index = (r.Index - 1 + total) % total }

// Right moves one pill right over n reactions plus "+", wrapping.
func (r *ReactionNav) Right(n int) { total := n + 1; r.Index = (r.Index + 1) % total }

// Clamp pulls the index back onto the "+" pill when reactions went away.
func (r *ReactionNav) Clamp(n int) {
	if total := n + 1; r.Index >= total {
		r.Index = total - 1
	}
}

// Selected returns the highlighted reaction, or isPlus for the "+" pill.
func (r ReactionNav) Selected(rs []ReactionItem) (emoji string, isPlus bool) {
	if r.Index >= len(rs) {
		return "", true
	}
	return rs[r.Index].Emoji, false
}
```

- [ ] **Step 4: Use them in both panes**

In both models replace the fields `reactionNavActive bool` / `reactionNavIndex int`
with `nav messages.ReactionNav` (`nav ReactionNav` in `messages`), rewriting
`m.reactionNavActive` → `m.nav.Active` and `m.reactionNavIndex` → `m.nav.Index`
throughout (production and tests: the reach rule). In `ReactionNavLeft` /
`ReactionNavRight` replace the index arithmetic with `m.nav.Left(len(msg.Reactions))`
/ `m.nav.Right(len(msg.Reactions))`; in `SelectedReaction` return
`m.nav.Selected(msg.Reactions)` after the nil/!ok guard; in `ClampReactionNav`
replace the clamp with `m.nav.Clamp(len(msg.Reactions))`. Keep every invalidation
call exactly where it is. In `messages.UpdateReaction` replace the inner
add/remove block with `ApplyReaction(&m.messages[i], emojiName, userID, m.currentUserID, remove)`;
in `thread.UpdateReaction` call `messages.ApplyReaction(msg, emojiName, userID, m.currentUserID, remove)`
and delete `updateMessageReaction`.

- [ ] **Step 5: Run, gate, commit**

`go test ./internal/ui/... -count=1` (goldens byte-identical), full gate, then:

```bash
git add internal/ui/messages internal/ui/thread
git commit -m "refactor(messages): one reaction mutation and cursor for both panes"
```

### Task 6.2: One `MessageItem` renderer

**Files:**
- Create: `internal/ui/messages/render_message.go`
- Modify: `internal/ui/messages/model.go` (`renderMessagePlain`, `blockkitContext`, `renderMessageEntry`)
- Modify: `internal/ui/thread/model.go` (delete `renderThreadMessage` and `blockkitContext`)
- Modify (reach only): thread tests calling `renderThreadMessage` (6 sites)

**Interfaces:**
- Produces:

```go
// RenderContext is what rendering a message reads besides the message.
// Each pane builds one from its own fields when it rebuilds its cache.
type RenderContext struct {
	ColoredUsernames bool
	UserStatuses     map[string]peerstatus.Status
	UserNames        map[string]string
	ChannelNames     map[string]string
	UserGroups       map[string]string
	Emoji            EmojiContext
	Images           *imgrender.Renderer // non-nil; the pane creates it
	ImageChannel     string              // the key image fetches and ImageReadyMsg use (P3)
	SearchTerms      []string
	Nav              ReactionNav // drawn only when RenderOptions.IsSelected
}

// RenderOptions are the per-pane differences in how a message renders:
// spec §5.1 items 9, 10 and 15, and plan decision P3.
type RenderOptions struct {
	Avatar          string // the author's avatar block; "" for none (always, in thread)
	IsSelected      bool   // the cursor is on this message (reaction-nav highlight)
	ReplyAffordance bool   // "[N replies ->]" under a parent (channel pane)
	EditedMark      bool   // "(edited)" after the timestamp (channel pane)
	BroadcastLabel  bool   // "↳ replied to a thread" label (channel pane)
	ImageHits       bool   // record image hit rects and sixel rows (channel pane)
	stats           *entryPerfStats
}

// RenderedMessage is one message, rendered, before the pane's border and fill.
type RenderedMessage struct {
	Content      string
	Header       string // the author line (with "(edited)"), for SelectedHeader
	HeaderBudget int
	Hits         []entrylist.Hit // image hits (when ImageHits), then reaction pills
	SixelRows    map[int]sixelEntry
	// Kitty uploads by source: each pane keeps its own upload order.
	BodyFlushes, BlockFlushes, ReactionFlushes, AttachmentFlushes []func(io.Writer) error
}

func RenderMessage(msg MessageItem, width int, rc *RenderContext, opt RenderOptions) RenderedMessage
func BlockkitContext(msg MessageItem, rc *RenderContext) blockkit.Context
```

**How.** `RenderMessage` is `renderMessagePlain` moved to a free function, its body
unchanged except at the points below. Do it as a move first (commit nothing yet),
then make exactly these edits:

| Point in `renderMessagePlain` | Becomes |
|---|---|
| `styles.Username(msg.UserID, m.coloredUsernames)`, `AuthorStatusSuffix(m.userStatuses, …)` | `rc.ColoredUsernames`, `rc.UserStatuses` |
| `avatarStr` parameter | `opt.Avatar` |
| `bodyOpts` from `m.userGroups`, `m.emojiCtx`, `EmojiFlushes: &flushes` | from `rc`, `EmojiFlushes: &out.BodyFlushes` |
| `if len(m.searchTerms) > 0` | `if len(rc.SearchTerms) > 0` |
| `if msg.ReplyCount > 0 {` (threadLine) | `if opt.ReplyAffordance && msg.ReplyCount > 0 {` |
| `isSelected && m.reactionNavActive && i == m.reactionNavIndex` (and the `+` pill's two uses) | `opt.IsSelected && rc.Nav.Active && i == rc.Nav.Index` (`rc.Nav.Index >= len(...)`) |
| reaction `placedFlush` appended to `flushes` | appended to `out.ReactionFlushes` |
| `if msg.IsEdited {` | `if opt.EditedMark && msg.IsEdited {` |
| `if msg.Subtype == "thread_broadcast" {` | `if opt.BroadcastLabel && msg.Subtype == "thread_broadcast" {` |
| `m.blockkitContext(msg, userNames, channelNames)` | `BlockkitContext(msg, rc)` (the old method's body, reading `rc.Images.Context()`, `rc.UserNames`, `rc.ImageChannel`, `rc.UserGroups`, `rc.Emoji`; `Channel: rc.ImageChannel`) |
| block-kit / legacy `res.Flushes` → `allFlushes` | → `out.BlockFlushes` |
| block-kit / legacy `res.SixelRows` loops | only `if opt.ImageHits` |
| `m.imgRenderer` (nil-check-and-create) | `rc.Images` (the pane ensures non-nil before building `rc`, which is what both did) |
| `RenderBlock(…, m.channelName, …)` | `RenderBlock(…, rc.ImageChannel, …)`; its `Lines` do not depend on `baseRow`/`contentColBase` (`imgrender.go:307-420`: only `Hit` and the sixel key do), so thread may pass the channel pane's values |
| attachment `res.Flushes` → `allFlushes`; `convertSixelMap` / `hits = append(hits, convertHit(res.Hit))` | → `out.AttachmentFlushes`; sixel and hit only `if opt.ImageHits` |
| `reactionHits` (`reactionEntryHit`) and `hits` (`entryHit`) | `out.Hits`: image hits as `entrylist.Hit{Kind: HitImage, Key: fileID, Aux: attIdx}` first, then pills as `entrylist.Hit{Kind: HitReaction, Key: emoji}`. Delete `entryHit`, `reactionEntryHit` and `convertHit` |
| `stats` parameter | `opt.stats` |
| the return | `out.Content`, `out.Header = line + editedMark`, `out.HeaderBudget`, `out.SixelRows` (nil when empty) |

With every option off and no avatar, this is exactly `renderThreadMessage`: compare
them side by side before deleting the latter (header without `(edited)`, label rows
only for ephemerals, no affordance line, the same reaction row base, the same
`headerBudget` since `contentColBase` is 1).

Each pane then assembles its flushes in its **current** order:

```go
// messages.renderMessageEntry (channel pane): blocks, attachments, then
// body and reaction emoji, as renderMessagePlain returned them.
flushes := slices.Concat(r.BlockFlushes, r.AttachmentFlushes, r.BodyFlushes, r.ReactionFlushes)

// thread (reply cache build): body, blocks, reactions, attachments, the
// order renderThreadMessage appended them in.
flushes := slices.Concat(r.BodyFlushes, r.BlockFlushes, r.ReactionFlushes, r.AttachmentFlushes)
```

The channel pane's options: `Avatar: m.avatarFn(msg.UserID)` (when set),
`IsSelected: i == m.selected`, `ReplyAffordance`, `EditedMark`, `BroadcastLabel`,
`ImageHits` all true; `ImageChannel: m.channelName`. The thread pane's: all false
and no avatar (even though `thread.SetAvatarFunc` exists; P3); `ImageChannel:
m.channelID`. Add `renderContext()` to each pane building `RenderContext` from its
fields. In thread add

```go
// renderMessage renders msg with the thread pane's options.
func (m *Model) renderMessage(msg messages.MessageItem, width int, isSelected bool) messages.RenderedMessage {
	return messages.RenderMessage(msg, width, m.renderContext(), messages.RenderOptions{IsSelected: isSelected})
}
```

and rewrite thread's two call sites and its 6 test call sites of
`renderThreadMessage(msg, width, userNames, channelNames, sel)` to use it
(`content, flushes, hits, header, budget` → the struct's fields).

- [ ] **Step 1:** Move and edit as above. Build.
- [ ] **Step 2:** `go test ./internal/ui/messages ./internal/ui/thread ./internal/ui -count=1`. Expected: PASS,
  goldens byte-identical, `TestLockstep_SharedRenderBehaviour` PASS. A golden diff
  here means an option point was missed; find it from the diff, never re-bless.
- [ ] **Step 3:** Measure: `python3` shared-line count from Task 0.0 step 3 must now
  report < 50. Benches within ±10%.
- [ ] **Step 4:** Gate and commit:

```bash
git add internal/ui/messages internal/ui/thread
git commit -m "refactor(messages): one MessageItem renderer for both panes

renderThreadMessage was a near-copy of renderMessagePlain. Both are now
messages.RenderMessage; the differences (reply affordance, (edited),
broadcast label, avatar, image hits/sixel, image channel key, flush
order) are RenderOptions/RenderContext, set to each pane's behaviour."
```

### Task 6.3: One payload type, one `EmojiContext`

**Files:**
- Modify: `internal/ui/messages/model.go`, `internal/ui/thread/model.go`,
  `internal/ui/app.go` (`SetEmojiContext`: `thread.EmojiContext{…}` → the same value)
- Modify: `AGENTS.md`

- [ ] **Step 1:** Rename `messages.viewEntry` to exported `EntryPayload` (fields
  `ShortSrc`, `SelectedShort`, `Flushes`, and the unexported `sixelRows`), and give it

```go
// SelectedLines returns the rows to draw for a cursor entry e: its
// selected variant, or (while a text selection exists) the short-
// timestamp variant built once from ShortSrc by build. Rows past the
// short variant (the channel pane's spacer) are shared by every variant.
func (p *EntryPayload) SelectedLines(e *entrylist.Entry, hasSelection bool, build func(src string) []string) []string {
	if !hasSelection || p.ShortSrc == "" {
		return e.Selected
	}
	if p.SelectedShort == nil {
		short := build(p.ShortSrc)
		if len(short) > len(e.Selected) {
			return e.Selected // unreachable: variants share a height
		}
		p.SelectedShort = append(short, e.Selected[len(short):]...)
	}
	return p.SelectedShort
}
```

  Both panes' `selectedLines` call it; thread's `cache` becomes
  `[]messages.EntryPayload` and its `viewEntry` is deleted. Test reaches follow
  the renames (`.shortSrc` → `.ShortSrc`, `.linesSelectedShort` →
  `.SelectedShort`, `.flushes` → `.Flushes`).
- [ ] **Step 2:** Delete `thread.EmojiContext`; `thread.SetEmojiContext` takes
  `messages.EmojiContext`. Update `App.SetEmojiContext` (the one constructor, at
  `app.go`, `a.threadPanel.SetEmojiContext(thread.EmojiContext{…})` → pass `ctx`)
  and the stale comment in `reactionpicker/model.go:43`.
- [ ] **Step 3:** Verify the exit criterion:

```bash
grep -rn '^type viewEntry\|^type reactionEntryHit\|^type reactionHitRect\|^type EmojiContext\|^type EntryPayload' internal --include='*.go'
```

  Expected: exactly `messages.EntryPayload` and `messages.EmojiContext`.
- [ ] **Step 4:** `AGENTS.md` **Text and rendering**, add:

| Need | Use |
|---|---|
| Render a `MessageItem` the way a content pane does (author line, body, Block Kit, attachments, reaction pills, hit rects) | `messages.RenderMessage(msg, width, rc, opts)`; per-pane differences are `RenderOptions` |
| Apply a reaction add/remove event idempotently; the reaction-pill cursor | `messages.ApplyReaction`, `messages.ReactionNav` |

- [ ] **Step 5:** Tests, gate, commit:
  `git commit -m "refactor(messages): one entry payload and one EmojiContext for both panes"`.

---

## PR 7 — The App talks to one pane interface

Branch `refactor/phase3/pr7-app-pane`, from PR 6's tip. **Before starting, rebase
onto `main` and check whether #300 and #271 have merged**: both touch `app.go` and
`mode_normal.go`, and #271 `reducer_mouse.go`. Goldens byte-identical.

### Task 7.1: API convergence (divergences 1, 8a, 13) and `contentPane`

**Files:**
- Create: `internal/ui/content_pane.go`
- Create: `internal/ui/content_pane_test.go`
- Modify: `internal/ui/messages/model.go` (`New` returns `*Model`)
- Modify: `internal/ui/thread/model.go` (`ClickAt` returns bool; `SelectedMessage`;
  `ViewBare`; `ApplySelectionToBordered`; `SetUnreadBoundary`/`UnreadBoundaryTS`
  renamed `SetLastReadTS`/`LastReadTS`)
- Modify: `internal/ui/app.go`, `internal/ui/winmodels.go` (callers of `messages.New`,
  `SetUnreadBoundary`)
- Modify (API reach only): tests using `&m` on a `messages.New` result (≈12), the
  thread unread-boundary tests (5 + 12 calls), `lockstep_test.go`

- [ ] **Step 1: Converge the APIs**

- `messages.New(msgs, channelName) *Model` (return `&Model{…}`). Callers:
  `app.go` `rootModel := messages.New(nil, "")` then `&rootModel` → use the pointer;
  `winmodels.go` likewise. In tests, `go vet ./internal/ui/...` lists every `&m`
  that is now a `**Model`; drop the `&`.
- `thread.ClickAt(y int) bool`: true when the click landed on a selectable row
  (selection updated or already there), false on a miss, the channel pane's
  contract. Return values from the existing branches.
- `thread.SelectedMessage() (messages.MessageItem, bool)`:

```go
// SelectedMessage returns the message under the cursor (the parent when
// it is selected), the channel pane's accessor.
func (m *Model) SelectedMessage() (messages.MessageItem, bool) {
	if r := m.SelectedReply(); r != nil {
		return *r, true
	}
	return messages.MessageItem{}, false
}
```

- `thread.ViewBare` and `thread.ApplySelectionToBordered`, mirroring the channel
  pane: split `View` into `viewInternal(height, width int, applySelection bool)`
  (pass `applySelection` to `Compose`) and

```go
func (m *Model) View(height, width int) string     { return m.viewInternal(height, width, true) }
func (m *Model) ViewBare(height, width int) string { return m.viewInternal(height, width, false) }

// ApplySelectionToBordered paints the selection onto a cached bordered
// render; see messages.Model.ApplySelectionToBordered.
func (m *Model) ApplySelectionToBordered(bordered string, topBorderRows, leftBorderCols int) string {
	return m.list.ApplySelectionToBordered(bordered, topBorderRows+m.chromeHeight, leftBorderCols)
}
```

  The App keeps calling `thread.View` (spec §5.1 item 13: no new caching path).
- Rename thread's `SetUnreadBoundary` → `SetLastReadTS` and `UnreadBoundaryTS` →
  `LastReadTS` (keep the field `unreadBoundaryTS` and its identity-change clearing,
  8b). Update the 3 production callers and the tests.

- [ ] **Step 2: Declare the interface (test first)**

`internal/ui/content_pane_test.go`:

```go
package ui

import "testing"

func TestFocusedContentPane(t *testing.T) {
	a := newTestApp(t, withMessages(testMessageItems(3)...), withActiveChannel("C1"))
	a.focusedPanel = PanelMessages
	if p, ch, ok := a.focusedContentPane(); !ok || p != contentPane(a.messagepane) || ch != "C1" {
		t.Fatalf("messages: %v %q %v", p, ch, ok)
	}
	a.threadPanel.SetThread(testMessageItems(1)[0], nil, "C9", "1.0")
	a.focusedPanel = PanelThread
	if p, ch, ok := a.focusedContentPane(); !ok || p != contentPane(a.threadPanel) || ch != "C9" {
		t.Fatalf("thread: %v %q %v", p, ch, ok)
	}
	a.focusedPanel = PanelSidebar
	if _, _, ok := a.focusedContentPane(); ok {
		t.Fatal("the sidebar is not a content pane")
	}
}
```

`internal/ui/content_pane.go`:

```go
package ui

import (
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/thread"
)

// contentPane is what the App does to either message pane: the channel
// pane (messages.Model) and the thread pane (thread.Model). The two
// share their scroll, selection and rendering code (Phase 3), and the
// App's mirror pairs collapse onto this interface. What only one pane
// has (images, loading, the thread's parent) stays off it.
type contentPane interface {
	SelectedMessage() (messages.MessageItem, bool)
	ReactionNavActive() bool
	EnterReactionNav()
	ExitReactionNav()
	ReactionNavLeft()
	ReactionNavRight()
	SelectedReaction() (emoji string, isPlus bool)
	ChromeHeight() int
	HitTestReaction(row, col int) (idx int, emoji string, ok bool)
	BeginSelectionAt(y, x int)
	ExtendSelectionAt(y, x int)
	EndSelection() (string, bool)
	ClearSelection()
	ClickAt(y int) bool
	ScrollHintForDrag(y int) int
	ScrollUp(n int)
	ScrollDown(n int)
}

var (
	_ contentPane = (*messages.Model)(nil)
	_ contentPane = (*thread.Model)(nil)
)

// contentPaneFor returns the pane behind panel, or nil.
func (a *App) contentPaneFor(panel Panel) contentPane {
	switch panel {
	case PanelMessages:
		return a.messagepane
	case PanelThread:
		return a.threadPanel
	}
	return nil
}

// focusedContentPane returns the focused message pane and the channel its
// messages belong to (reactions and pickers need it). ok is false when
// neither message pane has focus.
func (a *App) focusedContentPane() (contentPane, string, bool) {
	switch a.focusedPanel {
	case PanelMessages:
		return a.messagepane, a.activeChannelID, true
	case PanelThread:
		return a.threadPanel, a.threadPanel.ChannelID(), true
	}
	return nil, "", false
}
```

Run `go test ./internal/ui -run TestFocusedContentPane -count=1`: PASS after the
file exists. The two `var _` lines are the compile-time assertion the spec asks for.

- [ ] **Step 3: Gate and commit**

```bash
git add internal/ui
git commit -m "refactor(ui): converge the pane APIs and declare contentPane"
```

### Task 7.2: Collapse the mirror pairs and arms

**Files:**
- Modify: `internal/ui/app.go`, `internal/ui/mode_normal.go`, `internal/ui/drag.go`,
  `internal/ui/reducer_mouse.go`, `internal/ui/action_prompt.go`

- [ ] **Step 1: The three pairs become three functions**

Replace `handleReactionNav`/`handleThreadReactionNav`,
`openPickerFromMessage`/`openPickerFromThread` and
`toggleReactionOnSelectedMessage`/`toggleReactionOnSelectedThread` with:

```go
// handleReactionNav routes a key while reaction navigation is active on
// pane, whose messages belong to channelID.
func (a *App) handleReactionNav(p contentPane, channelID string, msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, a.keys.Left):
		p.ReactionNavLeft()
	case key.Matches(msg, a.keys.Right):
		p.ReactionNavRight()
	case key.Matches(msg, a.keys.Enter):
		emojiName, isPlus := p.SelectedReaction()
		if isPlus {
			return a.openReactionPicker(p, channelID)
		}
		return a.toggleReactionOnSelected(p, channelID, emojiName)
	case key.Matches(msg, a.keys.Reaction):
		return a.openReactionPicker(p, channelID)
	case key.Matches(msg, a.keys.Escape):
		p.ExitReactionNav()
	}
	return nil
}

// openReactionPicker opens the emoji picker for pane's selected message.
func (a *App) openReactionPicker(p contentPane, channelID string) tea.Cmd {
	msg, ok := p.SelectedMessage()
	if !ok {
		return nil
	}
	var existing []string
	for _, r := range msg.Reactions {
		if r.HasReacted {
			existing = append(existing, r.Emoji)
		}
	}
	p.ExitReactionNav()
	a.reactionPicker.SetFrecentEmoji(a.reactions.LoadFrecent(10))
	a.reactionPicker.Open(channelID, msg.TS, existing)
	a.SetMode(ModeReactionPicker)
	return nil
}

// toggleReactionOnSelected toggles the current user's emojiName reaction
// on pane's selected message.
func (a *App) toggleReactionOnSelected(p contentPane, channelID, emojiName string) tea.Cmd {
	msg, ok := p.SelectedMessage()
	if !ok {
		return nil
	}
	return a.toggleReactionOnMessageItem(channelID, msg, emojiName)
}
```

(`toggleReactionOnMessageItem`, `app.go:1394`, was already a third copy of both
toggle bodies.) Find every caller with
`grep -rn 'handleReactionNav\|handleThreadReactionNav\|openPickerFrom\|toggleReactionOnSelected' internal/ui --include='*.go'`
and route each through `a.focusedContentPane()` or the pane it already named,
passing `a.activeChannelID` for the channel pane and `a.threadPanel.ChannelID()`
for the thread pane, exactly the IDs the old pairs used. In `mode_normal.go` the
two arms

```go
	if a.focusedPanel == PanelMessages && a.messagepane.ReactionNavActive() {
		return a.handleReactionNav(msg)
	}
	if a.focusedPanel == PanelThread && a.threadPanel.ReactionNavActive() {
		return a.handleThreadReactionNav(msg)
	}
```

become

```go
	if p, channelID, ok := a.focusedContentPane(); ok && p.ReactionNavActive() {
		return a.handleReactionNav(p, channelID, msg)
	}
```

and, in the same file, the two `if a.focusedPanel == PanelMessages { … } else if a.focusedPanel == PanelThread { … }`
arms (the `Reaction` key opening the picker, and the `ReactionNav` key) become:

```go
		if p, channelID, ok := a.focusedContentPane(); ok {
			return a.openReactionPicker(p, channelID)
		}
```

```go
		if p, _, ok := a.focusedContentPane(); ok {
			p.EnterReactionNav()
		}
```

- [ ] **Step 2: `drag.go`**

Every `switch d.Panel() { case PanelMessages: a.messagepane.X(…); case PanelThread: a.threadPanel.X(…) }`
whose two arms call the same method becomes
`if p := a.contentPaneFor(d.Panel()); p != nil { p.X(…) }`. That covers
`ScrollHintForDrag` (twice), `ExtendSelectionAt` (three times), the
`ScrollUp(1)`/`ScrollDown(1)` arm, and `EndSelection`. The release arm's
`!moved` branch keeps its switch: only the channel pane opens a thread on a plain
click; its `ClearSelection` calls stay per arm.

- [ ] **Step 3: `reducer_mouse.go`**

In the wheel handler, the thread arm keeps its own case (the messages arm also
triggers history backfill). In the click handler, the two panes' common sequence
(reaction hit test on content rows, then `drag.Begin`, `BeginSelectionAt`, `ClickAt`)
becomes a helper:

```go
// toggleReactionAt toggles the reaction whose pill is at pane-local
// (px, py) in p, if there is one. items are the pane's messages as
// HitTestReaction indexes them (Messages() or Replies()).
func (a *App) toggleReactionAt(p contentPane, channelID string, items []messages.MessageItem, px, py int) (tea.Cmd, bool) {
	if contentY := py - p.ChromeHeight(); contentY >= 0 {
		if idx, emojiName, hit := p.HitTestReaction(contentY, px); hit && emojiName != "" {
			if idx >= 0 && idx < len(items) {
				return a.toggleReactionOnMessageItem(channelID, items[idx], emojiName), true
			}
		}
	}
	return nil, false
}
```

Each arm calls it first and returns its command when the second result is true.
The channel arm then does its image hit test (channel only), then
`a.drag.Begin(PanelMessages, px, py)`, `BeginSelectionAt`, `a.drag.SetClickedMessage(a.messagepane.ClickAt(py))`.
The thread arm then does `a.drag.Begin(PanelThread, px, py)`, `BeginSelectionAt`,
`ClickAt` (result ignored, as before). Keep the order of operations in each arm
exactly as it is today; `TestChar_*` and `reducer_mouse_test.go` pin it.

- [ ] **Step 4: `action_prompt.go` (from #304)**

`selectedMessageInFocus` keeps its switch for building the `actionPromptRef` (the
thread arm needs `ThreadTS()`), but reads the message through the pane:
`msg, ok := p.SelectedMessage()` from `a.focusedContentPane()`.

- [ ] **Step 5: Any remaining arm**

```bash
grep -n 'case PanelThread:' internal/ui/*.go | grep -v _test
```

For each remaining `case PanelMessages:` / `case PanelThread:` pair whose two
bodies differ only in which pane (and channel ID) they use, collapse it through
`focusedContentPane()`. Leave a pair alone, and list it in the commit message,
when its bodies genuinely differ (threads view, activity view, image preview,
history backfill). Do not widen `contentPane` to absorb a one-pane behaviour.

- [ ] **Step 6: Tests, gate, commit**

`go test ./internal/ui -count=1` (all mode tables, goldens byte-identical), full gate:

```bash
git add internal/ui
git commit -m "refactor(ui): collapse the messages/thread mirror pairs onto contentPane"
```

---

## PR 8 — Close-out

Branch `refactor/phase3/pr8-close-out`, from PR 7's tip.

### Task 8.1: Retire the lockstep test; bring the documents up to date

**Files:**
- Delete: `internal/ui/thread/lockstep_test.go`
- Modify: `AGENTS.md`, `docs/superpowers/plans/2026-09-06-architecture-refactor.md`,
  `docs/superpowers/specs/2026-10-10-phase3-messages-thread-fork-design.md`

- [ ] **Step 1: Delete the lockstep file**

Its own header says it exists to make the merge verifiable. The divergence list
now lives where it cannot drift: each entry is an `entrylist` option or a
`RenderOptions` field, documented on the declaration and tested per setting
(`entrylist_test.go`, `TestChar_*`, `TestApplyReaction`). Before deleting, check
that every numbered divergence (1–15) and U1–U6 has such a home; write the mapping
into the commit message. `git rm internal/ui/thread/lockstep_test.go`.

- [ ] **Step 2: `AGENTS.md`**

- golangci-lint: `v2.13.1` → `v2.14.0`.
- **Known duplication**: replace the `messages.Model and thread.Model` bullet with
  one sentence: the panes share `bubbles/entrylist` and `messages.RenderMessage`;
  their differences are options; do not add a pane-specific copy of either.
- **Conventions**: the "two implementations that must stay parallel" paragraph
  cites `thread/lockstep_test.go` as the example. Replace it with `contentPane`
  (`internal/ui/content_pane.go`) and the `entrylist` options.

- [ ] **Step 3: The tracking document**

In `docs/superpowers/plans/2026-09-06-architecture-refactor.md`: ground rule 5
`v2.13.1` → `v2.14.0`; Phase 3's status row → **complete**; add an "Achieved"
subsection under Phase 3 with the measured exit criteria (re-run Task 0.0 step 3:
shared lines, `thread/model.go` lines, test refs) and the PR list; update the
"Target end state" rows for `thread/model.go`.

- [ ] **Step 4: Spec addenda**

Append to the spec a short "Addenda (implementation)" section recording: decision
P1 (zero-reply totals), P2 (the stale-border bug and its issue number), P3 (U7
`(edited)` mark, U8 broadcast label, U9 avatar, plus the image channel key and flush
order, all `RenderOptions`/`RenderContext`), P4 (four goldens), the unobservable
count gate, `KeepLookups`, and that the thread parent is still re-rendered every
frame (caching it is a follow-up).

- [ ] **Step 5: Final verification**

```bash
go build ./... && go vet ./... && test -z "$(gofmt -l .)" && golangci-lint run && go test ./... -race -count=1
go test ./... -race -count=5   # the tracking doc's phase exit bar
grep -rn 'bubbles/v2/viewport' internal/ui/thread                  # nothing
grep -n '^func (a \*App) \(handleThreadReactionNav\|openPickerFromThread\|toggleReactionOnSelectedThread\)' internal/ui/app.go  # nothing
wc -l internal/ui/thread/model.go                                 # ≤ 1,000 (stretch < 900)
```

and the shared-line count (< 50) and the type grep from Task 6.3 step 3. Paste
everything into the PR description.

- [ ] **Step 6: Commit**

```bash
git add -A AGENTS.md docs internal/ui/thread
git commit -m "docs(phase3): close out — lockstep test retired, docs and status updated"
```

Follow-ups to file after Phase 3 (spec §9), one issue each: thread gaining U1,
U2, U3 and 8(c); thread dropping U4 and U6; #254; the P2 stale border; caching
the thread parent; whether thread should get search highlighting, a loading state
or image click-to-preview (10, 11, 15).

---

## Self-review

Checked against the spec after writing:

- **Spec coverage.** §3 layers: PRs 1, 4, 5, 6. §3.4 `contentPane`: Task 7.1.
  §4 D1–D5: PR structure. §5.1 items: 1, 8(a), 13 Task 7.1; 2, 3, 4, 5, 12 Task 5.1;
  6 Task 5.1; 7 PRs 2+3 and 5; 8(b) Task 5.1; 8(c) options in Tasks 4.2/5.1; 9, 10,
  15 Task 6.2; 11 Task 4.2; 14 Task 2.2. §5.2 U1–U6: options, pinned in Tasks
  0.2/0.3 and tested in Task 4.1. §6.2 characterization: Tasks 0.1–0.5. §6.3 the
  exception: PR 2+3 (four goldens, decision P4). §7.1 sequencing: "Before you
  start" and PR 7's rebase note. §8 budget: counted per commit (Tasks 4.2, 5.1, 7.1).
  §9 exits: Task 8.1 step 5.
- **Placeholders.** The `<N>` in two commit messages is a count the executor
  measures at that commit; nothing else is deferred.
- **Type consistency.** `entrylist` names are the validated code's. `dropCache`,
  `layoutValid`, `replyEntries`, `flushKey`, `ThemeListStyles`, `EntryPayload`,
  `RenderContext`, `RenderOptions`, `RenderedMessage`, `ReactionNav`,
  `ApplyReaction`, `contentPane`, `contentPaneFor`, `focusedContentPane`,
  `toggleReactionAt` are each
  defined once and used with the same signature.
- **Review Focus.** Each of the five has a pinning test (Tasks 0.2/0.3), all
  validated against today's code.



