package main

import (
	"errors"
	"testing"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/herdr"
)

func TestPlanRebootClassifiesEverySavedPane(t *testing.T) {
	states := []cache.PaneStateRow{
		{PaneKey: "default"},
		{PaneKey: "w1:p1"},
		{PaneKey: "w1:p2"},
		{PaneKey: "w1:p3"},
		{PaneKey: "w1:p4"},
		{PaneKey: "w2:p9"},
	}
	panes := []herdr.Pane{
		{PaneID: "w1:p1", WorkspaceID: "w1"},
		{PaneID: "w1:p2", WorkspaceID: "w1"},
		{PaneID: "w1:p3", WorkspaceID: "w1"},
		{PaneID: "w1:p4", WorkspaceID: "w1"},
		{PaneID: "w1:p5", WorkspaceID: "w1"}, // live, no saved state: not in the plan
	}
	info := map[string]herdr.ProcessInfo{
		// Shell at its prompt lists only itself.
		"w1:p1": {ShellPID: 10, Foreground: []herdr.Process{{PID: 10, Name: "zsh", Cmdline: "-zsh"}}},
		// A running slk: run-docker.sh's docker run in the foreground.
		"w1:p2": {ShellPID: 20, Foreground: []herdr.Process{
			{PID: 21, Name: "bash", Cmdline: "bash /repo/tools/run-docker.sh"},
			{PID: 22, Name: "docker", Cmdline: "docker run --rm -it --name slk-user-21 --label slk.role=user slk-go:1.26 /src/bin/slk-linux"},
		}},
		// Something else in the foreground.
		"w1:p3": {ShellPID: 30, Foreground: []herdr.Process{{PID: 31, Name: "vim", Cmdline: "vim notes.md"}}},
		// An empty foreground list is also a prompt.
		"w1:p4": {ShellPID: 40},
	}
	processInfo := func(paneID string) (herdr.ProcessInfo, error) {
		i, ok := info[paneID]
		if !ok {
			t.Errorf("process info asked for %s, which has no live pane", paneID)
		}
		return i, nil
	}

	plan, err := planReboot(states, panes, processInfo, "slk")
	if err != nil {
		t.Fatal(err)
	}
	want := []paneAction{
		{"default", paneStale},
		{"w1:p1", paneLaunch},
		{"w1:p2", paneLive},
		{"w1:p3", paneBusy},
		{"w1:p4", paneLaunch},
		{"w2:p9", paneStale},
	}
	if len(plan) != len(want) {
		t.Fatalf("plan = %v, want %v", plan, want)
	}
	for i := range want {
		if plan[i] != want[i] {
			t.Errorf("plan[%d] = %v, want %v", i, plan[i], want[i])
		}
	}
}

func TestClassifyForegroundMatchesTheConfiguredCommand(t *testing.T) {
	// A custom open_command with no "slk" in it still reads as live.
	info := herdr.ProcessInfo{ShellPID: 1, Foreground: []herdr.Process{
		{PID: 2, Name: "bash", Cmdline: "bash /home/me/bin/chat-tui"},
	}}
	if got := classifyForeground(info, "/home/me/bin/chat-tui"); got != paneLive {
		t.Errorf("configured command in the foreground = %s, want live", got)
	}
	if got := classifyForeground(info, "slk"); got != paneBusy {
		t.Errorf("unrelated foreground process = %s, want busy", got)
	}
}

func TestPlanRebootStopsOnProcessInfoError(t *testing.T) {
	boom := errors.New("pane.process_info: connection refused")
	_, err := planReboot(
		[]cache.PaneStateRow{{PaneKey: "w1:p1"}},
		[]herdr.Pane{{PaneID: "w1:p1"}},
		func(string) (herdr.ProcessInfo, error) { return herdr.ProcessInfo{}, boom },
		"slk",
	)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapped %v", err, boom)
	}
}
