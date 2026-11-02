// internal/ui/mode_pinsbookmarks.go
//
// Key handler for ModePinsBookmarks: the pins and bookmarks dialog
// (pinsbookmarks.go). The status bar names the section the cursor is
// in, PINS or BOOKMARKS.
package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/pinsbookmarks"
)

func handlePinsBookmarksMode(a *App, msg tea.KeyMsg) tea.Cmd {
	s := &a.pinsBookmarks
	act := s.model.HandleKey(msg.String())
	switch act.Kind {
	case pinsbookmarks.Open, pinsbookmarks.OpenTab, pinsbookmarks.OpenPane:
		a.SetMode(ModeNormal)
		return a.openPinOrBookmark(act)
	case pinsbookmarks.Retry:
		return a.retryPinsBookmarks()
	}
	if !s.model.IsVisible() {
		a.SetMode(ModeNormal)
		return nil
	}
	a.statusbar.SetMode(&s.model)
	return nil
}
