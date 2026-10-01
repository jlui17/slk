package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// A synthetic bot card long enough to fold.
func longCardMessage(ts string) messages.MessageItem {
	body := make([]string, 8)
	for i := range body {
		body[i] = fmt.Sprintf("card line %d", i+1)
	}
	return messages.MessageItem{TS: ts, UserID: "U1", UserName: "alice", Timestamp: "4:10 PM", Text: "deploy finished",
		LegacyAttachments: []blockkit.LegacyAttachment{{Title: "Deploy", Text: strings.Join(body, "\n")}}}
}

func foldTestApp() *App {
	app := NewApp()
	app.width, app.height = 120, 40
	app.activeChannelID = "C1"
	app.messagepane.SetMessages([]messages.MessageItem{longCardMessage("1.0")})
	return app
}

func screen(app *App) string { return ansi.Strip(app.View().Content) }

func pressZ(app *App) { app.handleNormalMode(tea.KeyPressMsg{Code: 'z', Text: "z"}) }

func TestToggleAttachmentFoldKey_MessagesPane(t *testing.T) {
	app := foldTestApp()
	app.focusedPanel = PanelMessages
	if s := screen(app); !strings.Contains(s, "▸ 5 more lines · z to expand") {
		t.Fatalf("card must start collapsed:\n%s", s)
	}
	app.messagepane.InvalidateCache() // as an event between two draws does
	pressZ(app)
	if s := screen(app); !strings.Contains(s, "card line 8") || !strings.Contains(s, "▾ z to collapse") {
		t.Errorf("z must expand the selected message's card:\n%s", s)
	}
	pressZ(app)
	if s := screen(app); !strings.Contains(s, "▸ 5 more lines · z to expand") {
		t.Errorf("z must collapse it again:\n%s", s)
	}
}

func TestToggleAttachmentFoldKey_ThreadPane(t *testing.T) {
	app := foldTestApp()
	app.threadPanel.SetThread(messages.MessageItem{TS: "1.0", UserName: "alice", Text: "parent"}, []messages.MessageItem{longCardMessage("1.001")}, "C1", "1.0")
	app.threadVisible = true
	screen(app)
	app.threadPanel.View(40, 60)
	app.threadPanel.InvalidateCache() // as an event between two draws does
	app.focusedPanel = PanelThread
	pressZ(app)
	if got := ansi.Strip(app.threadPanel.View(40, 60)); !strings.Contains(got, "▾ z to collapse") {
		t.Errorf("z must expand the selected reply's card:\n%s", got)
	}
	if s := screen(app); !strings.Contains(s, "▸ 5 more lines · z to expand") {
		t.Errorf("the messages pane keeps its own fold:\n%s", s)
	}
}

func TestClickOnFoldRowTogglesTheCard(t *testing.T) {
	app := foldTestApp()
	x, y := -1, -1
	for row, l := range strings.Split(screen(app), "\n") {
		if before, _, found := strings.Cut(l, "▸ 5 more lines"); found {
			x, y = ansi.StringWidth(before), row
		}
	}
	if y < 0 {
		t.Fatal("no fold row on screen")
	}
	app.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if s := screen(app); !strings.Contains(s, "card line 8") {
		t.Errorf("a click on the fold row must expand the card:\n%s", s)
	}
	if app.drag.IsActive() {
		t.Error("the click must not start a drag selection")
	}
}
