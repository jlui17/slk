package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	imgpkg "github.com/gammons/slk/internal/image"
)

// memoScreen puts the test placement's single row on screen line index 1
// (placement Row 2, 1-based CUP), so tests can edit text under or away
// from the image independently.
const memoScreen = "line1\nline2\nline3"

func memoPlacements() []imgpkg.SixelPlacement {
	return []imgpkg.SixelPlacement{{
		Key: "k", Row: 2, Col: 3, Rows: 1, Cols: 1,
		Bytes: []byte("\x1bPqk\x1b\\"),
	}}
}

// The memo must be sensitive to every field the painter's sameSlot
// compares (Key, Row, Col, Rows, Cols, Guard) plus EraseSGR and the
// placement count — and, like sameSlot, deliberately blind to Bytes
// (Key is the content identity).
func TestSixelFrameMemo_FieldSensitivity(t *testing.T) {
	base := imgpkg.SixelPlacement{Key: "k", Row: 2, Col: 3, Rows: 4, Cols: 5, Guard: "g", Bytes: []byte("payload")}
	m := &sixelFrameMemo{}
	m.remember([]imgpkg.SixelPlacement{base}, "sgr", 7)

	if id, ok := m.reusableID([]imgpkg.SixelPlacement{base}, "sgr"); !ok || id != 7 {
		t.Fatalf("identical frame: id=%d ok=%v, want 7 true", id, ok)
	}
	bytesOnly := base
	bytesOnly.Bytes = []byte("different payload")
	if _, ok := m.reusableID([]imgpkg.SixelPlacement{bytesOnly}, "sgr"); !ok {
		t.Fatal("Bytes must not participate in the comparison")
	}

	for name, mutate := range map[string]func(*imgpkg.SixelPlacement){
		"Key":   func(p *imgpkg.SixelPlacement) { p.Key = "other" },
		"Row":   func(p *imgpkg.SixelPlacement) { p.Row++ },
		"Col":   func(p *imgpkg.SixelPlacement) { p.Col++ },
		"Rows":  func(p *imgpkg.SixelPlacement) { p.Rows++ },
		"Cols":  func(p *imgpkg.SixelPlacement) { p.Cols++ },
		"Guard": func(p *imgpkg.SixelPlacement) { p.Guard = "other" },
	} {
		mutated := base
		mutate(&mutated)
		if _, ok := m.reusableID([]imgpkg.SixelPlacement{mutated}, "sgr"); ok {
			t.Errorf("%s change reused the frame", name)
		}
	}
	if _, ok := m.reusableID([]imgpkg.SixelPlacement{base}, "other-sgr"); ok {
		t.Error("EraseSGR change reused the frame")
	}
	if _, ok := m.reusableID([]imgpkg.SixelPlacement{base, base}, "sgr"); ok {
		t.Error("placement-count change reused the frame")
	}
	if _, ok := m.reusableID(nil, "sgr"); ok {
		t.Error("non-empty -> empty reused the frame")
	}
	if _, ok := (&sixelFrameMemo{}).reusableID(nil, "sgr"); ok {
		t.Error("unprimed memo offered a frame ID")
	}
}

// An identical frame must reuse the previously published frame ID, so
// the marked WindowTitle is byte-identical and bubbletea's viewEquals
// early-out can fire. Publish mints monotonic IDs, so an equal ID also
// proves the store did not grow.
func TestFinalizeSixelView_UnchangedFrameReusesID(t *testing.T) {
	a := sixelTestApp()
	first := a.finalizeSixelView(tea.NewView(memoScreen), memoPlacements())
	second := a.finalizeSixelView(tea.NewView(memoScreen), memoPlacements())
	if first.WindowTitle != second.WindowTitle {
		t.Fatalf("identical frame republished: title %q -> %q", first.WindowTitle, second.WindowTitle)
	}
	id := frameIDFromTitle(t, second.WindowTitle)
	if _, ok := a.sixelFrames.Take(id); !ok {
		t.Fatalf("frame %d not in store", id)
	}
	if _, ok := a.sixelFrames.Take(id + 1); ok {
		t.Fatal("a second frame was published for an identical view")
	}
}

// A text change on rows no image covers leaves every guard equal, so the
// frame ID is reused: this is the presence-event/tick case the memo
// exists for.
func TestFinalizeSixelView_TextChangeAwayFromImageReusesID(t *testing.T) {
	a := sixelTestApp()
	first := a.finalizeSixelView(tea.NewView(memoScreen), memoPlacements())
	second := a.finalizeSixelView(tea.NewView("CHANGED\nline2\nline3"), memoPlacements())
	if first.WindowTitle != second.WindowTitle {
		t.Fatalf("text change away from the image republished: %q -> %q", first.WindowTitle, second.WindowTitle)
	}
}

// A text change under the image changes that placement's guard, so a new
// frame must publish (the painter has to repaint over the rewrite).
func TestFinalizeSixelView_TextChangeUnderImagePublishesNewFrame(t *testing.T) {
	a := sixelTestApp()
	first := a.finalizeSixelView(tea.NewView(memoScreen), memoPlacements())
	second := a.finalizeSixelView(tea.NewView("line1\nCHANGED\nline3"), memoPlacements())
	if first.WindowTitle == second.WindowTitle {
		t.Fatal("guard change did not publish a new frame")
	}
	frame, ok := a.sixelFrames.Take(frameIDFromTitle(t, second.WindowTitle))
	if !ok || len(frame.Placements) != 1 {
		t.Fatalf("new frame not published: ok=%v %+v", ok, frame)
	}
}

// forceSixelRepaint must always publish, even for an otherwise identical
// frame; afterwards the flag is consumed and identical frames reuse the
// forced frame's ID.
func TestFinalizeSixelView_ForceRepublishesIdenticalFrame(t *testing.T) {
	a := sixelTestApp()
	first := a.finalizeSixelView(tea.NewView(memoScreen), memoPlacements())

	a.forceSixelRepaint = true
	forced := a.finalizeSixelView(tea.NewView(memoScreen), memoPlacements())
	if first.WindowTitle == forced.WindowTitle {
		t.Fatal("forced frame reused the previous ID")
	}
	frame, ok := a.sixelFrames.Take(frameIDFromTitle(t, forced.WindowTitle))
	if !ok || !frame.Force {
		t.Fatalf("forced frame: ok=%v Force=%v, want published and forced", ok, frame.Force)
	}
	if a.forceSixelRepaint {
		t.Fatal("force flag not consumed")
	}

	next := a.finalizeSixelView(tea.NewView(memoScreen), memoPlacements())
	if next.WindowTitle != forced.WindowTitle {
		t.Fatalf("identical frame after force republished: %q -> %q", forced.WindowTitle, next.WindowTitle)
	}
}

// Placements going non-empty -> empty must publish (that transition is
// what erases the old pixels); empty -> empty may then reuse.
func TestFinalizeSixelView_EmptyTransitionPublishesThenReuses(t *testing.T) {
	a := sixelTestApp()
	first := a.finalizeSixelView(tea.NewView(memoScreen), memoPlacements())

	empty1 := a.finalizeSixelView(tea.NewView(memoScreen), nil)
	if empty1.WindowTitle == first.WindowTitle {
		t.Fatal("non-empty -> empty transition did not publish")
	}
	frame, ok := a.sixelFrames.Take(frameIDFromTitle(t, empty1.WindowTitle))
	if !ok || len(frame.Placements) != 0 {
		t.Fatalf("erase frame: ok=%v placements=%d, want published empty", ok, len(frame.Placements))
	}

	empty2 := a.finalizeSixelView(tea.NewView(memoScreen), nil)
	if empty2.WindowTitle != empty1.WindowTitle {
		t.Fatalf("empty -> empty republished: %q -> %q", empty1.WindowTitle, empty2.WindowTitle)
	}
}
