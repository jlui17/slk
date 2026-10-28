package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/styles"
	"github.com/gammons/slk/internal/usernames"
)

// freshFrame renders a.View() with the compose-region caches emptied,
// so every bottom region is rebuilt from its inputs.
func freshFrame(a *App) string {
	a.renderCache.msgBottom = composeRegionCache{}
	a.renderCache.threadBottom = composeRegionCache{}
	return a.View().Content
}

// TestComposeRegionCache_MatchesFreshRender walks the inputs of the
// channel and thread panes' bottom regions (compose text, cursor
// blink, focus, typing row, app status row, width, theme) and checks
// that after each change the cached frame is byte-identical to a
// fresh render, and that the change reached the screen at all.
func TestComposeRegionCache_MatchesFreshRender(t *testing.T) {
	t.Cleanup(func() { styles.Apply("default", config.Theme{}) })
	msgs := benchMessages()[:20]
	a := newTestApp(t,
		withWindowSize(200, 40),
		withMessages(msgs...),
		withActiveChannel("C1"),
	)
	a.currentUserID = "U_SELF"
	a.messagepane.SetUserNames(usernames.FromMap(map[string]string{"U2": "Bob"}))

	typeInto := func(s string) func() {
		return func() {
			for _, r := range s {
				_, _ = a.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
			}
		}
	}
	// Focus's command is the textarea cursor's first blink; feeding it
	// back to the composer toggles the cursor.
	blink := func(focus func() tea.Cmd, update func(tea.Msg)) func() {
		return func() { update(focus()()) }
	}

	steps := []struct {
		name string
		do   func()
	}{
		{"insert mode", func() { a.SetMode(ModeInsert); a.focusedPanel = PanelMessages; a.compose.Focus() }},
		{"type in compose", typeInto("hello")},
		{"cursor blink", blink(a.compose.Focus, func(m tea.Msg) { a.compose, _ = a.compose.Update(m) })},
		{"typing row", func() { a.typing.SetEnabled(true); a.typing.Add("C1", "U2") }},
		{"normal mode", func() { a.SetMode(ModeNormal) }},
		// Only the border color changes: the composer is unfocused
		// both before and after.
		{"focus sidebar", func() { a.focusedPanel = PanelSidebar }},
		{"resize", func() { _, _ = a.Update(tea.WindowSizeMsg{Width: 240, Height: 45}) }},
		{"theme", func() { a.applyTheme("dracula") }},
		{"open thread", func() {
			a.threadVisible = true
			a.threadPanel.SetThread(msgs[0], msgs[1:3], "C1", msgs[0].TS)
		}},
		{"thread insert", func() { a.SetMode(ModeInsert); a.focusedPanel = PanelThread; a.threadCompose.Focus() }},
		{"type in thread compose", typeInto("reply")},
		{"thread cursor blink", blink(a.threadCompose.Focus, func(m tea.Msg) { a.threadCompose, _ = a.threadCompose.Update(m) })},
		{"thread app status", func() { a.typing.SetBotStatus("C1", msgs[0].TS, "B1", "is thinking") }},
	}

	prev := a.View().Content
	for _, s := range steps {
		s.do()
		got := a.View().Content
		if got == prev {
			t.Fatalf("%s: frame did not change", s.name)
		}
		if want := freshFrame(a); got != want {
			t.Fatalf("%s: cached frame differs from a fresh render", s.name)
		}
		prev = got
	}
}
