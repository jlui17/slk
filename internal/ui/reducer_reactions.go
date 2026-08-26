// internal/ui/reducer_reactions.go
//
// Reaction-family reducer for App.Update (Phase 4g).
//
// Owns the three Update arms for reaction WS echoes / API results:
//
//	ReactionAddedMsg    - server confirmed a reaction was added.
//	                      Applied unconditionally; updateReactionOnMessage
//	                      is idempotent per (emoji, userID), so the echo
//	                      of our own optimistic update collapses to one
//	                      count while reactions from other users (or our
//	                      own from another device) still merge in live.
//	ReactionRemovedMsg  - server confirmed a reaction was removed
//	                      (same idempotent application as Added).
//	ReactionSentMsg     - our reaction API call completed. On failure the
//	                      optimistic update is rolled back (inverse op) and
//	                      logged, so a rejected reaction doesn't linger.
//
// Free reducer (no dedicated controller) because reactions are a
// per-message annotation with no cross-message invariant and no
// in-flight state to track. The update helper itself
// (updateReactionOnMessage) stays on App because it touches
// messagepane / threadPanel caches.
package ui

import (
	"log"

	tea "charm.land/bubbletea/v2"
)

var reduceReactions reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case ReactionAddedMsg:
		// Ahead of the background skip, like the tracked agent thread's
		// hooks in reduceSend: the agent's ack follows the thread's own
		// workspace, not the active one.
		a.noteAgentThreadReaction(m.TeamID, m.ChannelID, m.MessageTS, m.UserID, false)
		if m.TeamID != "" && m.TeamID != a.activeTeamID {
			// Background workspace — see NewMessageMsg.TeamID.
			return nil, true
		}
		// Apply every reaction event, including echoes of our own.
		// updateReactionOnMessage is idempotent per (emoji, userID), so
		// our optimistic update and the WS echo that follows it collapse
		// to a single count — while reactions we make from another device
		// (web/mobile), which have no optimistic update here, still show
		// up live instead of being dropped.
		a.updateReactionOnMessage(m.ChannelID, m.MessageTS, m.Emoji, m.UserID, false)
		return nil, true

	case ReactionRemovedMsg:
		a.noteAgentThreadReaction(m.TeamID, m.ChannelID, m.MessageTS, m.UserID, true)
		if m.TeamID != "" && m.TeamID != a.activeTeamID {
			return nil, true
		}
		a.updateReactionOnMessage(m.ChannelID, m.MessageTS, m.Emoji, m.UserID, true)
		return nil, true

	case ReactionSentMsg:
		// API call completed. On failure, roll back the optimistic
		// update (apply the inverse of the attempted op) so the UI
		// reflects reality instead of showing a reaction the server
		// never accepted, and log it for diagnosis.
		if m.Err != nil {
			log.Printf("Warning: reaction API call failed (emoji=%q remove=%v): %v", m.Emoji, m.Remove, m.Err)
			a.updateReactionOnMessage(m.ChannelID, m.MessageTS, m.Emoji, m.UserID, !m.Remove)
		}
		return nil, true
	}
	return nil, false
}
