package sidebar

import "testing"

// TestPageMovesCursorByNRowsWhenViewportCannot asserts tmux's half-page
// contract at the edges of the list: the cursor moves n rows on every
// press, whether or not the viewport can scroll with it. Rows are
// Threads, Activity, a blank, the "Channels" header, then C01..C40.
func TestPageMovesCursorByNRowsWhenViewportCannot(t *testing.T) {
	t.Run("down in a list that fits the window", func(t *testing.T) {
		const height = 60
		m := scrollFollowModel(t)
		_ = m.View(height, 30)

		m.PageDown(5)
		_ = m.View(height, 30)

		if got := m.SelectedID(); got != "C02" {
			t.Errorf("selected = %q, want C02 (five rows below the Threads row)", got)
		}
	})

	t.Run("up in a list that fits the window", func(t *testing.T) {
		const height = 60
		m := scrollFollowModel(t)
		m.GoToBottom()
		_ = m.View(height, 30)

		m.PageUp(5)
		_ = m.View(height, 30)

		if got := m.SelectedID(); got != "C35" {
			t.Errorf("selected = %q, want C35 (five rows above C40)", got)
		}
	})

	t.Run("down with the viewport already at the bottom", func(t *testing.T) {
		const height = 10
		m := scrollFollowModel(t)
		m.GoToBottom()
		_ = m.View(height, 30)
		for range 9 {
			m.MoveUp()
		}
		_ = m.View(height, 30)
		if got := m.SelectedID(); got != "C31" {
			t.Fatalf("setup: expected C31 selected, got %q", got)
		}
		startOff := m.yOffset

		m.PageDown(5)
		_ = m.View(height, 30)

		if got := m.yOffset; got != startOff {
			t.Errorf("yOffset = %d, want %d (already at the bottom)", got, startOff)
		}
		if got := m.SelectedID(); got != "C36" {
			t.Errorf("selected = %q, want C36 (five rows below C31)", got)
		}
	})

	t.Run("down with two rows of scroll left", func(t *testing.T) {
		const height = 10
		m := scrollFollowModel(t)
		m.GoToBottom()
		_ = m.View(height, 30)
		for range 9 {
			m.MoveUp()
		}
		_ = m.View(height, 30)
		m.ScrollUp(2) // wheel: the viewport moves, the cursor stays on C31
		_ = m.View(height, 30)
		if got := m.SelectedID(); got != "C31" {
			t.Fatalf("setup: expected C31 selected, got %q", got)
		}
		startOff := m.yOffset

		m.PageDown(5)
		_ = m.View(height, 30)

		if got := m.yOffset; got != startOff+2 {
			t.Errorf("yOffset = %d, want %d (the two rows that were left)", got, startOff+2)
		}
		if got := m.SelectedID(); got != "C36" {
			t.Errorf("selected = %q, want C36 (five rows below C31)", got)
		}
	})
}
