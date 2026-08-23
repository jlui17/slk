package image

import (
	"bytes"
	"strings"
	"testing"
)

// The App reuses a published frame's ID when nothing sixel-relevant
// changed, so the same marked ID can reach FrameOutput again after its
// first flush already pruned it from the store (Take prunes <= id). That
// second flush must be a pure text passthrough: guard equality is what
// allowed the reuse, so the cells under every image are unchanged and
// the painter's on-screen state is still accurate — ok=false, empty
// plan, painter untouched.
func TestFrameOutput_ReusedFrameIDAfterPruneIsNoOp(t *testing.T) {
	var raw bytes.Buffer
	frames := NewSixelFrameStore()
	out := NewFrameOutput(&raw, frames)

	id := frames.Publish(SixelFrame{Placements: []SixelPlacement{pl("img", 2, 3, 1, 1)}})
	if _, err := out.Write(markedFlush("slk", id, "text-1")); err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	if out.painter.Live() != 1 {
		t.Fatalf("Live = %d after first flush, want 1", out.painter.Live())
	}
	if !strings.Contains(raw.String(), "\x1bPqimg\x1b\\") {
		t.Fatalf("first flush did not paint: %q", raw.String())
	}

	raw.Reset()
	if _, err := out.Write(markedFlush("slk", id, "text-2")); err != nil {
		t.Fatalf("Write 2: %v", err)
	}
	if got := raw.String(); got != "text-2" {
		t.Fatalf("reused-ID flush must pass text through untouched; got %q", got)
	}
	if out.painter.Live() != 1 {
		t.Fatalf("reused-ID flush mutated painter state; Live = %d, want 1", out.painter.Live())
	}
}
