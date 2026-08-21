package herdr

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/gammons/slk/internal/debuglog"
)

// watchRetryDelay paces reconnects after the subscription connection
// drops (herdr restart, transient socket error).
const watchRetryDelay = 5 * time.Second

type subscribeParams struct {
	Subscriptions []subscription `json:"subscriptions"`
}

type subscription struct {
	Type string `json:"type"`
}

// paneLocation is where a pane currently sits and the two focus
// dimensions that decide whether the user is looking at it. Both are kept
// because focus events move one at a time.
type paneLocation struct {
	tabID       string
	workspaceID string
	wsFocused   bool
	tabFocused  bool
}

// viewed reports herdr's notion of the pane being looked at: its tab is
// the active tab of the focused workspace.
func (l paneLocation) viewed() bool { return l.wsFocused && l.tabFocused }

// WatchFocus invokes onViewChange (on the watcher's goroutine) with the
// pane's viewed state — whether its tab is the focused workspace's active
// tab — once at startup and on every transition after. Two consumers
// need it: herdr marks the pane's agent entry seen while it is viewed,
// wiping the unseen indicator even if the tracked thread is still unread,
// so the unviewed edge is the cue to re-assert it; and slk treats
// viewedness as the user having actually seen what the pane renders.
//
// One limit is inherent: herdr also treats a focused terminal window as
// part of being viewed, but publishes no event for terminal focus, so a
// seen-wipe caused purely by the user returning to the terminal window
// passes unnoticed until the next tab or workspace focus change.
//
// Runs a persistent events.subscribe connection with reconnects until
// Close, and is nil-safe.
func (r *Reporter) WatchFocus(onViewChange func(viewed bool)) {
	if r == nil {
		return
	}
	go func() {
		for {
			r.watchFocusOnce(onViewChange)
			select {
			case <-r.stop:
				return
			case <-time.After(r.retryDelay):
			}
		}
	}()
}

// watchFocusOnce runs one subscription connection to exhaustion: resolve
// where the pane currently sits and whether it is viewed, subscribe, then
// fold the event stream into that viewed flag. Returns on any error, and
// the caller reconnects.
func (r *Reporter) watchFocusOnce(onViewChange func(viewed bool)) {
	// Resolved per connection rather than read from the launch
	// environment: herdr can move a pane to another tab or workspace, and
	// a stale tab id silently mistracks focus forever. Re-resolving also
	// re-seeds viewed, so a focus change missed while disconnected can't
	// leave the fold inverted.
	loc, err := r.paneLocation()
	if err != nil {
		debuglog.Notify("herdr: focus watch locate: %v", err)
		return
	}

	conn, err := net.DialTimeout(r.network, r.addr, sendTimeout)
	if err != nil {
		debuglog.Notify("herdr: focus watch dial: %v", err)
		return
	}
	r.mu.Lock()
	select {
	case <-r.stop:
		r.mu.Unlock()
		conn.Close()
		return
	default:
	}
	r.watchConn = conn
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.watchConn = nil
		r.mu.Unlock()
		conn.Close()
	}()

	// One scanner for the whole connection, ack included: a second one
	// would start with an empty buffer and lose whatever the first had
	// already read past the ack -- silently eating the first event.
	scanner := bufio.NewScanner(conn)
	if err := r.subscribeFocus(conn, scanner); err != nil {
		debuglog.Notify("herdr: focus watch subscribe: %v", err)
		return
	}
	// The resolved state, before any event: a consumer that gates on
	// viewedness needs it from the start, not from the first transition.
	onViewChange(loc.viewed())

	for scanner.Scan() {
		var ev struct {
			Data struct {
				Type        string `json:"type"`
				TabID       string `json:"tab_id"`
				WorkspaceID string `json:"workspace_id"`
				PreviousID  string `json:"previous_pane_id"`
				Pane        struct {
					PaneID      string `json:"pane_id"`
					TabID       string `json:"tab_id"`
					WorkspaceID string `json:"workspace_id"`
				} `json:"pane"`
			} `json:"data"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		d := ev.Data
		was := loc.viewed()
		switch d.Type {
		case "tab_focused":
			// Another workspace's tab focus doesn't change which tab is
			// focused within ours; herdr keeps a focused tab per workspace.
			if d.WorkspaceID != loc.workspaceID {
				continue
			}
			loc.tabFocused = d.TabID == loc.tabID
		case "workspace_focused":
			loc.wsFocused = d.WorkspaceID == loc.workspaceID
		case "pane_moved":
			if d.PreviousID != r.paneID && d.Pane.PaneID != r.paneID {
				continue
			}
			// The pane landed somewhere else: adopt the new coordinates
			// and re-read whether that location is viewed, rather than
			// folding further events against the tab it left.
			moved, err := r.paneLocation()
			if err != nil {
				debuglog.Notify("herdr: focus watch relocate: %v", err)
				return
			}
			loc = moved
		default:
			continue
		}
		if was != loc.viewed() {
			onViewChange(loc.viewed())
		}
	}
}

// paneLocation reads the pane's current tab and workspace, and whether
// that tab is the focused workspace's active tab.
func (r *Reporter) paneLocation() (paneLocation, error) {
	var pane struct {
		Result struct {
			Pane struct {
				TabID       string `json:"tab_id"`
				WorkspaceID string `json:"workspace_id"`
			} `json:"pane"`
		} `json:"result"`
	}
	if err := r.call("pane.get", paneGetParams{PaneID: r.paneID}, &pane); err != nil {
		return paneLocation{}, err
	}
	loc := paneLocation{
		tabID:       pane.Result.Pane.TabID,
		workspaceID: pane.Result.Pane.WorkspaceID,
	}
	if loc.tabID == "" || loc.workspaceID == "" {
		return paneLocation{}, fmt.Errorf("pane %s has no tab or workspace", r.paneID)
	}
	var ws struct {
		Result struct {
			Workspace struct {
				Focused     bool   `json:"focused"`
				ActiveTabID string `json:"active_tab_id"`
			} `json:"workspace"`
		} `json:"result"`
	}
	if err := r.call("workspace.get", workspaceGetParams{WorkspaceID: loc.workspaceID}, &ws); err != nil {
		return paneLocation{}, err
	}
	loc.wsFocused = ws.Result.Workspace.Focused
	loc.tabFocused = ws.Result.Workspace.ActiveTabID == loc.tabID
	return loc, nil
}

// subscribeFocus sends the subscription request and consumes the ack,
// which herdr answers with a subscription_started result before any
// event. Matching that type positively (rather than treating any
// non-error line as success) keeps a stray line from being mistaken for
// the ack and swallowed.
func (r *Reporter) subscribeFocus(conn net.Conn, scanner *bufio.Scanner) error {
	req, err := json.Marshal(request{
		ID:     fmt.Sprintf("slk:%d", nextSeq()),
		Method: "events.subscribe",
		Params: subscribeParams{Subscriptions: []subscription{
			{Type: "tab.focused"},
			{Type: "workspace.focused"},
			{Type: "pane.moved"},
		}},
	})
	if err != nil {
		return err
	}
	conn.SetWriteDeadline(time.Now().Add(sendTimeout))
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return err
	}
	conn.SetReadDeadline(time.Now().Add(sendTimeout))
	if !scanner.Scan() {
		return fmt.Errorf("no subscribe ack: %w", scanner.Err())
	}
	var ack struct {
		Result struct {
			Type string `json:"type"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &ack); err != nil {
		return fmt.Errorf("undecodable subscribe ack: %w", err)
	}
	if ack.Error != nil {
		return fmt.Errorf("refused: %s", ack.Error.Message)
	}
	if ack.Result.Type != "subscription_started" {
		return fmt.Errorf("unexpected subscribe ack %q", ack.Result.Type)
	}
	// The events stream has no deadline: it is idle most of the time.
	return conn.SetReadDeadline(time.Time{})
}

// call sends one request and decodes its response into out.
func (r *Reporter) call(method string, params any, out any) error {
	req, err := json.Marshal(request{
		ID:     fmt.Sprintf("slk:%d", nextSeq()),
		Method: method,
		Params: params,
	})
	if err != nil {
		return err
	}
	resp, err := r.deliver(append(req, '\n'))
	if err != nil {
		return err
	}
	return json.Unmarshal(resp, out)
}
