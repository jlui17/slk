package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/herdr"
)

// herdrMain runs `slk herdr <args>` and returns the process exit code.
func herdrMain(args []string) int {
	if err := herdrCommand(args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

// herdrCommand dispatches the `slk herdr` subcommands, which operate on
// slk across the panes of a herdr session rather than on one TUI.
func herdrCommand(args []string, w io.Writer) error {
	switch strings.Join(args, " ") {
	case "pane states":
		return printPaneStates(w)
	case "reboot":
		return rebootPanes(w, false)
	case "reboot --dry-run":
		return rebootPanes(w, true)
	}
	return errors.New(`usage:
  slk herdr pane states        print every saved pane state (tab-separated)
  slk herdr reboot [--dry-run]  relaunch slk in every herdr pane with a saved state`)
}

// openPaneStateDB opens the cache the way run() does.
func openPaneStateDB() (*cache.DB, error) {
	return cache.New(filepath.Join(xdgData(), "cache.db"))
}

// printPaneStates writes one line per saved pane state: pane key,
// workspace id, channel id, thread ts, updated at.
func printPaneStates(w io.Writer) error {
	db, err := openPaneStateDB()
	if err != nil {
		return fmt.Errorf("opening cache: %w", err)
	}
	defer db.Close()
	rows, err := db.ListPaneStates()
	if err != nil {
		return err
	}
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			r.PaneKey, r.State.WorkspaceID, r.State.ChannelID, r.State.ThreadTS,
			r.UpdatedAt.Local().Format(time.DateTime))
	}
	return nil
}

// paneStatus is what a reboot does with one saved pane state.
type paneStatus string

const (
	paneLaunch paneStatus = "launch" // shell at its prompt: relaunch slk here
	paneLive   paneStatus = "live"   // slk already running in the pane
	paneBusy   paneStatus = "busy"   // something else holds the foreground
	paneStale  paneStatus = "stale"  // no live pane has this key
)

type paneAction struct {
	PaneID string
	Status paneStatus
}

// planReboot classifies every saved pane state, in the order given.
// processInfo is consulted only for keys that name a live pane; command
// is the relaunch command a running slk's foreground would show.
func planReboot(states []cache.PaneStateRow, panes []herdr.Pane, processInfo func(paneID string) (herdr.ProcessInfo, error), command string) ([]paneAction, error) {
	live := map[string]bool{}
	for _, p := range panes {
		live[p.PaneID] = true
	}
	plan := make([]paneAction, 0, len(states))
	for _, s := range states {
		if !live[s.PaneKey] {
			plan = append(plan, paneAction{s.PaneKey, paneStale})
			continue
		}
		info, err := processInfo(s.PaneKey)
		if err != nil {
			return nil, fmt.Errorf("pane %s: %w", s.PaneKey, err)
		}
		plan = append(plan, paneAction{s.PaneKey, classifyForeground(info, command)})
	}
	return plan, nil
}

// classifyForeground reads a pane's foreground: a running slk (the
// relaunch command, or "slk" anywhere in a cmdline — run-docker.sh's
// docker run carries the container name and binary path) is live, any
// other process besides the shell itself is busy, else the pane is at its
// prompt and can be launched into.
func classifyForeground(info herdr.ProcessInfo, command string) paneStatus {
	for _, p := range info.Foreground {
		if strings.Contains(p.Cmdline, command) || strings.Contains(p.Cmdline, "slk") {
			return paneLive
		}
	}
	for _, p := range info.Foreground {
		if p.PID != info.ShellPID {
			return paneBusy
		}
	}
	return paneLaunch
}

// firstLaunchTimeout covers the first relaunch, which may rebuild the
// binary (run-docker.sh); the rest only start a container.
const (
	firstLaunchTimeout = 180 * time.Second
	launchTimeout      = 60 * time.Second
)

// rebootPanes relaunches slk in every herdr pane that has a saved pane
// state and is sitting at its shell prompt, printing the plan first and
// launching nothing under dryRun. Runs inside a herdr pane (the herdr
// address comes from the launch env).
func rebootPanes(w io.Writer, dryRun bool) error {
	r := herdr.NewReporterFromEnv()
	if r == nil {
		return errors.New("not running inside a herdr pane")
	}
	cfg, err := config.Load(filepath.Join(xdgConfig(), "config.toml"))
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	command := cfg.Herdr.OpenCommand
	if command == "" {
		command = "slk"
	}
	db, err := openPaneStateDB()
	if err != nil {
		return fmt.Errorf("opening cache: %w", err)
	}
	defer db.Close()
	states, err := db.ListPaneStates()
	if err != nil {
		return err
	}
	panes, err := r.ListPanes()
	if err != nil {
		return err
	}
	plan, err := planReboot(states, panes, r.PaneProcessInfo, command)
	if err != nil {
		return err
	}
	var launch []string
	for _, a := range plan {
		fmt.Fprintf(w, "%s\t%s\n", a.Status, a.PaneID)
		if a.Status == paneLaunch {
			launch = append(launch, a.PaneID)
		}
	}
	if dryRun {
		fmt.Fprintf(w, "dry run: %d pane(s) would be launched with %q\n", len(launch), command)
		return nil
	}
	if len(launch) == 0 {
		fmt.Fprintln(w, "nothing to launch")
		return nil
	}
	// The first relaunch runs alone: run-docker.sh rebuilds the binary
	// when sources are newer, and every other container executes that
	// same file.
	first, rest := launch[0], launch[1:]
	fmt.Fprintf(w, "launching %s (rebuilds the binary if sources changed)...\n", first)
	if err := r.RunInPane(first, command); err != nil {
		return fmt.Errorf("pane %s: %w", first, err)
	}
	if err := r.WaitForOutput(first, "NORMAL", firstLaunchTimeout); err != nil {
		return fmt.Errorf("pane %s did not reach NORMAL (read the pane for the error): %w", first, err)
	}
	for _, pane := range rest {
		fmt.Fprintf(w, "launching %s...\n", pane)
		if err := r.RunInPane(pane, command); err != nil {
			return fmt.Errorf("pane %s: %w", pane, err)
		}
	}
	var failed []string
	for _, pane := range rest {
		if err := r.WaitForOutput(pane, "NORMAL", launchTimeout); err != nil {
			failed = append(failed, pane)
			fmt.Fprintf(w, "pane %s did not reach NORMAL: %v\n", pane, err)
		}
	}
	fmt.Fprintf(w, "launched %d pane(s)\n", len(launch)-len(failed))
	if len(failed) > 0 {
		return fmt.Errorf("%d pane(s) did not come up: %s", len(failed), strings.Join(failed, " "))
	}
	return nil
}
