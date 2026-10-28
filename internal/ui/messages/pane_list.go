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
