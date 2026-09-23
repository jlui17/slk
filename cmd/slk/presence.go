package main

import (
	"context"
	"time"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/ui"
)

// bootstrapPresenceAndDND fetches the user's current presence and DND
// state from Slack, populates the WorkspaceContext, and sends an initial
// StatusChangeMsg. Also subscribes to presence_change events for the self
// user and every DM peer so external state changes arrive over the WS.
func bootstrapPresenceAndDND(ctx context.Context, wctx *WorkspaceContext, program teaSender, tok bootstrapToken) {
	if wctx == nil || wctx.Client == nil {
		return
	}

	// Bound the goroutine's life: it is unjoined, and its writes are
	// already token-vetoed once stale, so the timeout only stops a hung
	// REST call from holding a connection slot indefinitely.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Subscribe to presence for our own user plus every 1:1 DM peer so the
	// sidebar can show who is online. presence_sub REPLACES the prior
	// subscription set, so self and peers must go in one call. Failure is
	// non-fatal — manual_presence_change and dnd_updated work without it.
	//
	// This runs from OnConnect, which fires on the initial connect AND on
	// every reconnect. Re-subscribing per connection is required because
	// the subscription is connection-scoped.
	subscribeWorkspacePresence(wctx)

	presence, err := wctx.Client.GetUserPresence(ctx, wctx.UserID)
	if err != nil {
		presence = nil
	}
	dnd, err := wctx.Client.GetDNDInfo(ctx, wctx.UserID)
	if err != nil {
		dnd = nil
	}
	applyBootstrappedStatus(wctx, program, tok, presence, dnd)

	// DM peers' DND, which the sidebar marks. Later changes arrive as
	// dnd_invalidated and go through the same refresher. Runs on every
	// connect because the socket does not replay missed invalidations.
	wctx.PeerStatus.RefreshDND(ctx, workspacePresenceIDs(wctx))
}

// subscribeWorkspacePresence subscribes over the WebSocket to presence
// updates for the authenticated user plus every 1:1 DM peer, so the
// sidebar can show who is online. Slack's presence_sub REPLACES the prior
// subscription set and is connection-scoped, so this sends self + all DM
// peers in a single call and must be re-run on each (re)connect.
//
// Note: the WS is opened with no_query_on_subscribe=1, so Slack does not
// reply with each peer's current presence at subscribe time — DM rows are
// seeded from the local cache at build time and then updated live by
// presence_change events. Safe to call repeatedly.
func subscribeWorkspacePresence(wctx *WorkspaceContext) {
	if wctx == nil || wctx.Client == nil {
		return
	}
	ids := workspacePresenceIDs(wctx)
	if len(ids) == 0 {
		debuglog.General("subscribeWorkspacePresence: no ids to subscribe")
		return
	}
	if err := wctx.Client.SubscribePresence(ids); err != nil {
		debuglog.General("subscribeWorkspacePresence (%d ids) FAILED: %v", len(ids), err)
		return
	}
	debuglog.General("subscribeWorkspacePresence: sent presence_sub for %d ids (self+%d dm peers)", len(ids), len(ids)-1)
}

// workspacePresenceIDs returns the deduped list of user IDs to subscribe
// for presence: the authenticated user plus every 1:1 DM peer. Group DMs
// and app/bot DMs (which carry no human presence dot in the sidebar) are
// skipped. Pure function for testability.
func workspacePresenceIDs(wctx *WorkspaceContext) []string {
	seen := make(map[string]struct{})
	ids := make([]string, 0, len(wctx.Channels)+1)
	add := func(uid string) {
		if uid == "" {
			return
		}
		if _, ok := seen[uid]; ok {
			return
		}
		seen[uid] = struct{}{}
		ids = append(ids, uid)
	}
	add(wctx.UserID) // self — keeps the self presence subscription intact
	for _, ch := range wctx.Channels {
		if ch.Type == "dm" {
			add(ch.DMUserID)
		}
	}
	return ids
}

func (h *rtmEventHandler) OnPresenceChange(userID, presence string) {
	if !h.presenceDedupe.Changed(userID, presence) {
		return
	}
	_ = h.db.UpdatePresence(userID, presence)
	if h.program == nil {
		return
	}
	h.program.Send(ui.PresenceChangeMsg{
		UserID:   userID,
		Presence: presence,
	})
}

func (h *rtmEventHandler) OnSelfPresenceChange(presence string) {
	if h.wsCtx == nil {
		return
	}
	// Slack uses "active"/"away" in events; store verbatim.
	st := h.wsCtx.selfStatus.SetPresence(presence)
	if h.program == nil {
		return
	}
	h.program.Send(ui.StatusChangeMsg{
		TeamID:     h.workspaceID,
		Presence:   st.Presence,
		DNDEnabled: st.DNDEnabled,
		DNDEndTS:   st.DNDEndTS,
	})
}

func (h *rtmEventHandler) OnDNDChange(enabled bool, endUnix int64) {
	if h.wsCtx == nil {
		return
	}
	var endTS time.Time
	if endUnix > 0 {
		endTS = time.Unix(endUnix, 0)
	}
	st := h.wsCtx.selfStatus.SetDND(enabled, endTS)
	if h.program == nil {
		return
	}
	h.program.Send(ui.StatusChangeMsg{
		TeamID:     h.workspaceID,
		Presence:   st.Presence,
		DNDEnabled: st.DNDEnabled,
		DNDEndTS:   st.DNDEndTS,
	})
}
