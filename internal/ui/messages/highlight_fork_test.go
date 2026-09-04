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

func TestHighlightSearchTerms_MatchesAcrossSGRBoundaries(t *testing.T) {
	const red = "\x1b[31m"
	got := HighlightSearchTerms("fu"+red+"nc main", []string{"func main"}, "<", ">")
	if want := "<func main>" + red; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHighlightSearchTerms_ReopensAfterResetInsideMatch(t *testing.T) {
	got := HighlightSearchTerms("fu\x1b[mnc", []string{"func"}, "<", ">")
	if want := "<fu\x1b[m<nc>"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHighlightSearchTerms_WithholdsColorChangesInsideAMatch(t *testing.T) {
	const blue, yellow = "\x1b[38;2;0;0;255m", "\x1b[38;2;255;255;0m"
	got := HighlightSearchTerms("x := "+blue+"[]"+yellow+"string"+blue+" tail", []string{"[]string"}, "<", ">")
	if want := "x := " + blue + "<[]string>" + blue + yellow + blue + " tail"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
