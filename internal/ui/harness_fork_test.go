// internal/ui/harness_fork_test.go
//
// Shared fixture builder and command drainer for fork-side App tests.
// newHarnessApp builds the App the way the runtime does — sized via a
// tea.WindowSizeMsg through Update, setup options applied in order,
// layout primed with one View render — instead of poking fields the
// way the older per-file builders did.
package ui

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/sidebar"
)

type harnessConfig struct {
	width  int
	height int
	setup  []func(*App)
}

type harnessOption func(*harnessConfig)

func withSize(width, height int) harnessOption {
	return func(c *harnessConfig) {
		c.width = width
		c.height = height
	}
}

func withMessages(items ...messages.MessageItem) harnessOption {
	return func(c *harnessConfig) {
		c.setup = append(c.setup, func(a *App) {
			a.messagepane.SetMessages(items)
		})
	}
}

func withWorkspace(teamID string, channels ...sidebar.ChannelItem) harnessOption {
	return func(c *harnessConfig) {
		c.setup = append(c.setup, func(a *App) {
			_, _ = a.Update(WorkspaceReadyMsg{
				TeamID:        teamID,
				InitialActive: true,
				Channels:      channels,
			})
		})
	}
}

// withApp runs fn during setup, after sizing and any earlier options —
// the escape hatch for test-specific wiring (services, callbacks,
// direct field pokes).
func withApp(fn func(*App)) harnessOption {
	return func(c *harnessConfig) {
		c.setup = append(c.setup, fn)
	}
}

func newHarnessApp(t *testing.T, opts ...harnessOption) *App {
	t.Helper()
	cfg := harnessConfig{width: 120, height: 30}
	for _, opt := range opts {
		opt(&cfg)
	}
	a := NewApp()
	_, _ = a.Update(tea.WindowSizeMsg{Width: cfg.width, Height: cfg.height})
	for _, fn := range cfg.setup {
		fn(a)
	}
	_ = a.View()
	return a
}

// drainCmds executes cmd and every command nested inside tea.Batch /
// tea.Sequence trees, returning the collected leaf messages. Nil cmds
// and nil messages are dropped. tea.Sequence's message type is
// unexported, but like tea.BatchMsg it is a []tea.Cmd — both are
// detected by convertibility.
func drainCmds(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	batchType := reflect.TypeOf(tea.BatchMsg(nil))
	if rv := reflect.ValueOf(msg); rv.Kind() == reflect.Slice && rv.Type().ConvertibleTo(batchType) {
		var out []tea.Msg
		for _, c := range rv.Convert(batchType).Interface().(tea.BatchMsg) {
			out = append(out, drainCmds(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestDrainCmds_NestedBatchAndSequence(t *testing.T) {
	leaf := func(s string) tea.Cmd {
		return func() tea.Msg { return s }
	}
	cmd := tea.Batch(
		leaf("a"),
		tea.Sequence(leaf("b"), nil, tea.Batch(leaf("c"), leaf("d"))),
		func() tea.Msg { return nil },
	)
	got := drainCmds(cmd)
	want := []string{"a", "b", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("drained %d msgs (%v), want %d", len(got), got, len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("msg[%d] = %v, want %q", i, got[i], w)
		}
	}
}
