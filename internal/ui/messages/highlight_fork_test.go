package messages

import (
	"testing"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/styles"
)

// Both highlight callers run ReapplyBgAfterResets after highlighting, so
// the close must be the bare reset that pass rewrites.
func TestSearchHighlightSGR_CloseIsBareReset(t *testing.T) {
	styles.Apply("dark", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	start, end, ok := SearchHighlightSGR()
	if !ok || start == "" {
		t.Fatalf("SearchHighlightSGR: ok=%v start=%q", ok, start)
	}
	if end != "\x1b[m" {
		t.Errorf("close = %q, want bare reset", end)
	}
}
