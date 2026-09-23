// internal/ui/frame_golden_fork_test.go
//
// Full-frame goldens for canonical App states: the screen an agent (or a
// reviewer) can diff to see a rendering change. Frames are ANSI-stripped —
// layout and content regress loudly here; color fidelity stays with the
// lipgloss differential tests (view_composite_test.go). Regenerate after an
// intentional rendering change with:
//
//	tools/go.sh test ./internal/ui/ -run TestFrameGolden -update
package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/sidebar"
)

func goldenApp(t *testing.T) *App {
	t.Helper()
	messages.SetNowFunc(func() time.Time {
		return time.Date(2026, 8, 25, 15, 0, 0, 0, time.UTC)
	})
	t.Cleanup(func() { messages.SetNowFunc(nil) })

	a := newHarnessApp(t,
		withHarnessSize(100, 28),
		withWorkspace("T1",
			sidebar.ChannelItem{ID: "C1", Name: "general", Type: "channel"},
			sidebar.ChannelItem{ID: "C2", Name: "incidents", Type: "channel"},
			sidebar.ChannelItem{ID: "D1", Name: "alice", Type: "dm"},
		),
		withApp(func(a *App) {
			_, _ = a.Update(ChannelSelectedMsg{ID: "C1", Name: "general", Type: "channel"})
		}),
		withHarnessMessages(
			messages.MessageItem{TS: "1787860800.000100", UserID: "U1", UserName: "alice",
				Text: "deploy went out clean", Timestamp: "10:00 AM", DateStr: "2026-08-24"},
			messages.MessageItem{TS: "1787864400.000200", UserID: "U2", UserName: "bob",
				Text: "confirming, dashboards look flat", Timestamp: "11:00 AM", DateStr: "2026-08-24"},
			messages.MessageItem{TS: "1787943600.000300", UserID: "U1", UserName: "alice",
				Text: "retry queue is draining again", Timestamp: "9:00 AM", DateStr: "2026-08-25"},
			messages.MessageItem{TS: "1787950800.000400", UserID: "U2", UserName: "bob",
				Text: "nice — closing the incident", Timestamp: "11:00 AM", DateStr: "2026-08-25"},
		),
	)
	return a
}

func goldenFrame(a *App) []byte {
	return []byte(ansi.Strip(a.View().Content))
}

// requireFrameGolden compares got with testdata/<test name>.golden, or
// rewrites it under golden_test.go's -update flag. It stands in for
// charmbracelet/x/exp/golden, which registers an -update flag of its own
// and so cannot share a test binary with golden_test.go.
func requireFrameGolden(t *testing.T, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", t.Name()+".golden")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("creating %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s missing or unreadable: %v", path, err)
	}
	if !bytes.Equal(want, got) {
		t.Errorf("frame differs from %s\n--- want\n%s\n--- got\n%s", path, want, got)
	}
}

func TestFrameGolden(t *testing.T) {
	t.Run("base", func(t *testing.T) {
		a := goldenApp(t)
		requireFrameGolden(t, goldenFrame(a))
	})

	t.Run("insert_compose", func(t *testing.T) {
		a := goldenApp(t)
		pressKeys(a, keyRune('i'))
		for _, r := range "drafting a reply" {
			pressKeys(a, keyRune(r))
		}
		requireFrameGolden(t, goldenFrame(a))
	})

	t.Run("command_line", func(t *testing.T) {
		a := goldenApp(t)
		pressKeys(a, keyRune(':'), keyRune('w'), keyRune('s'))
		requireFrameGolden(t, goldenFrame(a))
	})

	t.Run("help_overlay", func(t *testing.T) {
		a := goldenApp(t)
		pressKeys(a, keyRune('?'))
		requireFrameGolden(t, goldenFrame(a))
	})

	t.Run("sidebar_hidden", func(t *testing.T) {
		a := goldenApp(t)
		_, _ = a.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
		requireFrameGolden(t, goldenFrame(a))
	})
}
