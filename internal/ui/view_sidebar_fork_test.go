package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/styles"
)

// A focused sidebar draws the focused border every pane draws: the
// thick shape, in the focus_border role.
func TestFocusedSidebarBorderUsesTheFocusRole(t *testing.T) {
	styles.Apply("coffee", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	a := newTestApp(t, normalOpts()...)
	a.focusedPanel = PanelSidebar
	out := a.renderSidebar(20, 2, 10, styles.Version())
	top := strings.SplitN(out, "\n", 2)[0]
	r, g, b, _ := styles.FocusBorderColor.RGBA()
	if fg := fmt.Sprintf("38;2;%d;%d;%d", r>>8, g>>8, b>>8); !strings.Contains(top, fg) || !strings.Contains(top, "┏") {
		t.Errorf("focused sidebar's top border = %q, want ┏ in focus_border %s", top, fg)
	}
}
