package herdr

import (
	"bufio"
	"encoding/json"
	"errors"
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
// onConnected (also on the watcher's goroutine, nil to skip) fires each
// time the subscription is established, the initial connection included.
// A reconnect means the server was restarting or unreachable, so any
// report sent in the gap is gone; the signal lets the UI republish the
// sidebar's current state instead of leaving it to the next organic
// report.
//
// One limit is inherent: herdr also treats a focused terminal window as
// part of being viewed, but publishes no event for terminal focus, so a
// seen-wipe caused purely by the user returning to the terminal window
// passes unnoticed until the next tab or workspace focus change.
//
// Runs a persistent events.subscribe connection with reconnects until
// Close, and is nil-safe.
func (r *Reporter) WatchFocus(onViewChange func(viewed bool), onConnected func()) {
	if r == nil {
		return
	}
	// In r.wg so Close waits for the watcher: it writes to the pane-id
	// cache, whose DB the caller closes after Close returns.
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for {
			r.watchFocusOnce(onViewChange, onConnected)
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
func (r *Reporter) watchFocusOnce(onViewChange func(viewed bool), onConnected func()) {
	// Resolved per connection rather than read from the launch
	// environment: herdr can move a pane to another tab or workspace, and
	// a stale tab id silently mistracks focus forever. Re-resolving also
	// re-seeds viewed, so a focus change missed while disconnected can't
	// leave the fold inverted.
	loc, err := r.locate()
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
	if onConnected != nil {
		onConnected()
	}
	// The resolved state, before any event: a consumer that gates on
	// viewedness needs it from the start, not from the first transition.
	onViewChange(loc.viewed())

	for scanner.Scan() {
		var ev struct {
			Data struct {
				Type        string   `json:"type"`
				TabID       string   `json:"tab_id"`
				WorkspaceID string   `json:"workspace_id"`
				PreviousID  string   `json:"previous_pane_id"`
				Pane        paneInfo `json:"pane"`
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
			// Matched by the terminal id first (stable within a server
			// run, which is as long as an event stream lives): a cross-workspace
			// move assigns a new public pane id, so after one move the id
			// comparisons alone would miss every later move. They stay as
			// the fallback for an event without a terminal id. The terminal
			// match must be positive — with no terminal id known, an empty
			// event terminal id "matching" the empty tracked one would
			// adopt every other pane's move.
			id := r.identity()
			if (id.TerminalID == "" || d.Pane.TerminalID != id.TerminalID) &&
				d.PreviousID != id.PaneID && d.Pane.PaneID != id.PaneID {
				continue
			}
			// The pane landed somewhere else: adopt the new coordinates
			// and re-read whether that location is viewed, rather than
			// folding further events against the tab it left.
			r.adopt(d.Pane)
			moved, err := r.locate()
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

// locate reads the pane's current coordinates, adopts them as the
// reporter's identity, and returns where the pane sits and whether that
// tab is the focused workspace's active tab.
func (r *Reporter) locate() (paneLocation, error) {
	pane, err := r.fetchPane()
	if err != nil {
		return paneLocation{}, err
	}
	if pane.TabID == "" || pane.WorkspaceID == "" {
		return paneLocation{}, fmt.Errorf("pane %s has no tab or workspace", pane.PaneID)
	}
	r.adopt(pane)
	r.persistPaneID(pane.PaneID)
	loc := paneLocation{
		tabID:       pane.TabID,
		workspaceID: pane.WorkspaceID,
	}
	var ws struct {
		Workspace struct {
			Focused     bool   `json:"focused"`
			ActiveTabID string `json:"active_tab_id"`
		} `json:"workspace"`
	}
	if err := r.call("workspace.get", workspaceGetParams{WorkspaceID: loc.workspaceID}, &ws); err != nil {
		return paneLocation{}, err
	}
	loc.wsFocused = ws.Workspace.Focused
	loc.tabFocused = ws.Workspace.ActiveTabID == loc.tabID
	return loc, nil
}

// fetchPane resolves the pane's current record by its tracked public id,
// falling back on pane_not_found. The recovery that fires in practice is
// the cached id: public pane ids are the only pane handle herdr persists
// across a restart, so a cold start whose launch-env id predates a
// cross-workspace move (`herdr update --handoff` keeps the shell and its
// stale HERDR_PANE_ID alive while wiping the in-memory alias map)
// resolves only through the id a previous run remembered. The pane.list
// scan by terminal id is functional only mid-run (a restart re-mints
// every terminal id) and has no known trigger there either: the alias
// map resolves every stale id until restart, so pane.get shouldn't fail
// mid-run at all. It stays because that "shouldn't" is an unverified
// belief about alias semantics and an idle fallback costs nothing;
// confirming in herdr's source that pane.get cannot fail mid-run is
// what would make the scan deletable.
func (r *Reporter) fetchPane() (paneInfo, error) {
	id := r.identity()
	pane, err := r.getPane(id.PaneID)
	if err == nil {
		return pane, nil
	}
	var srvErr serverError
	if errors.As(err, &srvErr) && srvErr.code == "pane_not_found" {
		if id.TerminalID != "" {
			pane, scanErr := r.findPaneByTerminal(id.TerminalID)
			if scanErr == nil {
				return pane, nil
			}
			debuglog.Notify("herdr: terminal scan for %s: %v", id.TerminalID, scanErr)
		}
		if r.loadPaneID != nil {
			if cached, ok := r.loadPaneID(); ok && cached != id.PaneID {
				pane, cacheErr := r.getPane(cached)
				if cacheErr == nil {
					return pane, nil
				}
				debuglog.Notify("herdr: cached pane id %s: %v", cached, cacheErr)
			}
		}
	}
	return paneInfo{}, err
}

func (r *Reporter) getPane(paneID string) (paneInfo, error) {
	var parsed struct {
		Pane paneInfo `json:"pane"`
	}
	if err := r.call("pane.get", paneGetParams{PaneID: paneID}, &parsed); err != nil {
		return paneInfo{}, err
	}
	return parsed.Pane, nil
}

func (r *Reporter) findPaneByTerminal(terminalID string) (paneInfo, error) {
	var parsed struct {
		Panes []paneInfo `json:"panes"`
	}
	if err := r.call("pane.list", struct{}{}, &parsed); err != nil {
		return paneInfo{}, err
	}
	for _, pane := range parsed.Panes {
		if pane.TerminalID == terminalID {
			return pane, nil
		}
	}
	return paneInfo{}, fmt.Errorf("no pane holds terminal %s", terminalID)
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

// call sends one request and decodes its result object into out (nil to
// discard it); a server error response comes back as a serverError.
func (r *Reporter) call(method string, params any, out any) error {
	result, err := r.roundTripTimeout(method, params, sendTimeout)
	if err != nil || out == nil {
		return err
	}
	return json.Unmarshal(result, out)
}
