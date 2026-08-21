package herdr

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// openTabTimeout is the per-request socket deadline for OpenTab's
// round-trips. Deliberately larger than sendTimeout: tab.create spawns
// a tab, PTY, and shell server-side, and pane.wait_for_output blocks
// until the shell paints — deadlines sized for fire-and-forget sidebar
// reports would abandon requests the server is still serving.
const openTabTimeout = 5 * time.Second

// shellReadyTimeout bounds pane.wait_for_output's wait for the new
// shell's first output. A shell that painted nothing in this window is
// assumed slow rather than broken: OpenTab falls back to
// shellStartupDelay and sends anyway.
const shellReadyTimeout = 3 * time.Second

// shellStartupDelay is the fallback pause before pane.send_input when
// pane.wait_for_output fails (older herdr, timeout): input typed before
// the shell starts reading can be dropped by line editors that flush
// pending input on init. Lower risks losing the command on slow shell
// startups; higher only delays the link opening. Var so tests can zero
// it.
var shellStartupDelay = 400 * time.Millisecond

type tabCreateParams struct {
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
	Focus       bool   `json:"focus"`
}

type outputMatch struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type paneWaitForOutputParams struct {
	PaneID    string      `json:"pane_id"`
	Source    string      `json:"source"`
	Match     outputMatch `json:"match"`
	TimeoutMS int64       `json:"timeout_ms"`
}

type paneSendInputParams struct {
	PaneID string   `json:"pane_id"`
	Text   string   `json:"text"`
	Keys   []string `json:"keys"`
}

type tabCloseParams struct {
	TabID string `json:"tab_id"`
}

// CanOpenTab reports whether OpenTab has a target space: a workspace id
// from the launch env, or one the focus watcher has adopted since. The
// wiring gates on it once at startup — before the watcher first
// resolves — so a pane whose env lacked HERDR_WORKSPACE_ID never gets
// the opener installed even after a workspace id is adopted.
func (r *Reporter) CanOpenTab() bool {
	if r == nil {
		return false
	}
	return r.identity().WorkspaceID != ""
}

// OpenTab creates a focused tab in the pane's current herdr space labeled
// label and runs `<openCommand> '<rawURL>'` in the tab's root-pane shell.
// Blocking (several socket round-trips, waiting for the tab's shell to
// paint its prompt); call it off the UI goroutine. Not tracked by
// Close: quitting slk mid-open abandons the sequence, worst case
// leaving the created tab with no command sent.
func (r *Reporter) OpenTab(label, openCommand, rawURL string) error {
	if !r.CanOpenTab() {
		return errors.New("no herdr workspace id")
	}
	created, err := r.roundTrip("tab.create", tabCreateParams{
		WorkspaceID: r.identity().WorkspaceID,
		Label:       label,
		Focus:       true,
	})
	if err != nil {
		return fmt.Errorf("tab.create: %w", err)
	}
	var parsed struct {
		Tab struct {
			TabID string `json:"tab_id"`
		} `json:"tab"`
		RootPane struct {
			PaneID string `json:"pane_id"`
		} `json:"root_pane"`
	}
	if err := json.Unmarshal(created, &parsed); err != nil {
		return fmt.Errorf("tab.create: %w", err)
	}
	if parsed.RootPane.PaneID == "" {
		return errors.New("tab.create: no root pane in response")
	}
	// Send only once the shell is reading input: wait for its first
	// visible output (the prompt). Any wait failure — the method absent
	// on an older herdr, or a shell quiet past the timeout — degrades
	// to the fixed fallback delay rather than aborting the open.
	if _, err := r.roundTrip("pane.wait_for_output", paneWaitForOutputParams{
		PaneID:    parsed.RootPane.PaneID,
		Source:    "visible",
		Match:     outputMatch{Type: "regex", Value: `\S`},
		TimeoutMS: shellReadyTimeout.Milliseconds(),
	}); err != nil {
		time.Sleep(shellStartupDelay)
	}
	command := openCommand + " '" + strings.ReplaceAll(rawURL, "'", `'\''`) + "'"
	if _, err := r.roundTrip("pane.send_input", paneSendInputParams{
		PaneID: parsed.RootPane.PaneID,
		Text:   command,
		Keys:   []string{"Enter"},
	}); err != nil {
		// Roll back the empty tab only when herdr definitively
		// rejected the send; on a transport failure (timeout, dropped
		// reply) the input may have been delivered, and closing would
		// kill a successful open. Residue beats destruction.
		if parsed.Tab.TabID != "" && errors.As(err, &serverError{}) {
			_, _ = r.roundTrip("tab.close", tabCloseParams{TabID: parsed.Tab.TabID})
		}
		return fmt.Errorf("pane.send_input: %w", err)
	}
	return nil
}

// serverError is an error response from herdr — the request was
// received and rejected — as opposed to a transport failure, where the
// request's server-side effect is unknown.
type serverError struct {
	code string
	msg  string
}

func (e serverError) Error() string { return e.msg }

// roundTrip sends one request and returns the raw result object, or the
// server's error as a Go error.
func (r *Reporter) roundTrip(method string, params any) (json.RawMessage, error) {
	return r.roundTripTimeout(method, params, openTabTimeout)
}

func (r *Reporter) roundTripTimeout(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	req, err := json.Marshal(request{
		ID:     fmt.Sprintf("slk:%d", nextSeq()),
		Method: method,
		Params: params,
	})
	if err != nil {
		return nil, err
	}
	resp, err := r.deliverTimeout(append(req, '\n'), timeout)
	if err != nil {
		return nil, err
	}
	if len(resp) == 0 {
		return nil, errors.New("connection closed without a reply")
	}
	var parsed struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return nil, err
	}
	if parsed.Error != nil {
		return nil, serverError{code: parsed.Error.Code, msg: parsed.Error.Message}
	}
	return parsed.Result, nil
}
