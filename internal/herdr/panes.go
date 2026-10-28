package herdr

import (
	"fmt"
	"time"
)

// Process is one entry of a pane's foreground process list.
type Process struct {
	PID     int    `json:"pid"`
	Name    string `json:"name"`
	Cmdline string `json:"cmdline"`
}

// ProcessInfo is what a pane's shell has in the foreground. A shell at
// its prompt lists only itself (PID == ShellPID) or nothing.
type ProcessInfo struct {
	ShellPID   int       `json:"shell_pid"`
	Foreground []Process `json:"foreground_processes"`
}

// ListPanes returns every live pane across all workspaces: pane.list
// with no workspace_id is unscoped.
func (r *Reporter) ListPanes() ([]Pane, error) {
	panes, err := r.listPanes(openTabTimeout)
	if err != nil {
		return nil, fmt.Errorf("pane.list: %w", err)
	}
	return panes, nil
}

func (r *Reporter) listPanes(timeout time.Duration) ([]Pane, error) {
	var parsed struct {
		Panes []Pane `json:"panes"`
	}
	if err := r.call("pane.list", struct{}{}, timeout, &parsed); err != nil {
		return nil, err
	}
	return parsed.Panes, nil
}

// PaneProcessInfo reports what paneID's shell has in the foreground.
func (r *Reporter) PaneProcessInfo(paneID string) (ProcessInfo, error) {
	var parsed struct {
		ProcessInfo ProcessInfo `json:"process_info"`
	}
	if err := r.call("pane.process_info", paneGetParams{PaneID: paneID}, openTabTimeout, &parsed); err != nil {
		return ProcessInfo{}, fmt.Errorf("pane.process_info: %w", err)
	}
	return parsed.ProcessInfo, nil
}

// RunInPane types command into paneID and presses Enter — the same
// pane.send_input call the herdr CLI's `pane run` makes. The text goes
// to whatever is frontmost, so the pane's shell must be at its prompt.
func (r *Reporter) RunInPane(paneID, command string) error {
	err := r.call("pane.send_input", paneSendInputParams{
		PaneID: paneID,
		Text:   command,
		Keys:   []string{"Enter"},
	}, openTabTimeout, nil)
	if err != nil {
		return fmt.Errorf("pane.send_input: %w", err)
	}
	return nil
}

// WaitForOutput blocks until substring shows on paneID's visible screen,
// returning herdr's timeout error if it hasn't within timeout.
func (r *Reporter) WaitForOutput(paneID, substring string, timeout time.Duration) error {
	// The socket deadline must outlast the server-side wait.
	err := r.call("pane.wait_for_output", paneWaitForOutputParams{
		PaneID:    paneID,
		Source:    "visible",
		Match:     outputMatch{Type: "substring", Value: substring},
		TimeoutMS: timeout.Milliseconds(),
	}, timeout+openTabTimeout, nil)
	if err != nil {
		return fmt.Errorf("pane.wait_for_output: %w", err)
	}
	return nil
}
