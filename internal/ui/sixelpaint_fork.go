package ui

import (
	"hash/fnv"
	"slices"
	"strconv"
	"strings"

	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/messages"
)

// sixelFrameMemo remembers the last published sixel frame so an
// identical View can reuse its frame ID instead of publishing again.
// Reuse keeps the marked WindowTitle byte-stable, which is what lets
// bubbletea's viewEquals early-out skip the full-screen re-parse and
// cell diff on messages that changed nothing. Reusing an ID the store
// may have pruned is safe — pinned by
// TestFrameOutput_ReusedFrameIDAfterPruneIsNoOp.
type sixelFrameMemo struct {
	valid      bool
	id         uint64
	placements []imgpkg.SixelPlacement
	eraseSGR   string
}

func (m *sixelFrameMemo) reusableID(placements []imgpkg.SixelPlacement, eraseSGR string) (uint64, bool) {
	if !m.valid || m.eraseSGR != eraseSGR || len(placements) != len(m.placements) {
		return 0, false
	}
	for i := range placements {
		if !placements[i].SameSlot(m.placements[i]) {
			return 0, false
		}
	}
	return m.id, true
}

func (m *sixelFrameMemo) remember(placements []imgpkg.SixelPlacement, eraseSGR string, id uint64) {
	m.valid = true
	m.id = id
	m.placements = slices.Clone(placements)
	m.eraseSGR = eraseSGR
}

func (a *App) publishSixelFrame(screen string, placements []imgpkg.SixelPlacement) uint64 {
	attachSixelGuards(screen, placements)
	eraseSGR := messages.BgANSI()
	if !a.forceSixelRepaint {
		if id, ok := a.sixelMemo.reusableID(placements, eraseSGR); ok {
			return id
		}
	}
	id := a.sixelFrames.Publish(imgpkg.SixelFrame{
		Placements: placements,
		EraseSGR:   eraseSGR,
		Force:      a.forceSixelRepaint,
	})
	a.forceSixelRepaint = false
	a.sixelMemo.remember(placements, eraseSGR, id)
	return id
}

// attachSixelGuards is frameGuard's hash with the strings.Split hoisted
// out of the per-placement loop; it supersedes upstream frameGuard
// (sixelpaint.go), which is left in place unused to keep merges cheap.
func attachSixelGuards(screen string, placements []imgpkg.SixelPlacement) {
	if len(placements) == 0 || screen == "" {
		return
	}
	lines := strings.Split(screen, "\n")
	for i := range placements {
		top := placements[i].Row - 1
		h := fnv.New64a()
		for r := top; r < top+placements[i].Rows && r < len(lines); r++ {
			h.Write([]byte(lines[r]))
			h.Write([]byte{0})
		}
		placements[i].Guard = strconv.FormatUint(h.Sum64(), 36)
	}
}
