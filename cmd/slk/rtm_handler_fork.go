package main

import (
	"github.com/gammons/slk/internal/debuglog"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/ui"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/slack-go/slack"
)

// dispatchBackgroundThreadReply sends a thread reply that arrived in a
// background workspace to the UI, tagged with its workspace, so
// thread-scoped consumers (the tracked agent thread) see it. Background
// dispatch is thread-replies-only (contract: ui.NewMessageMsg.TeamID):
// dispatching top-level messages would force a discarded Update+View
// cycle per message the user can't see. The name is a store lookup with
// the raw ID as fallback — resolveUserCached would SELECT on misses
// that can never be repaired here (the resolver fill is active-only) —
// so it does not go through messageItemFromEvent.
func (h *rtmEventHandler) dispatchBackgroundThreadReply(channelID, authorID, userID, username, ts, text, threadTS, subtype string, edited bool, files []slack.File, blocks slack.Blocks, attachments []slack.Attachment) {
	if h.program == nil {
		return
	}
	userName := authorID
	if resolved, ok := h.userNames.Get(authorID); ok {
		userName = resolved
	} else if userID == "" && username != "" {
		userName = username
	}
	debuglog.Cache("OnMessage: team=%s channel=%s ts=%s subtype=%q thread_ts=%s decision=dispatched_background_thread_reply",
		h.workspaceID, channelID, ts, subtype, threadTS)
	h.program.Send(ui.NewMessageMsg{
		TeamID:    h.workspaceID,
		ChannelID: channelID,
		Message: messages.MessageItem{
			TS:                ts,
			UserID:            authorID,
			UserName:          userName,
			Text:              text,
			Timestamp:         formatTimestamp(ts, h.tsFormat),
			ThreadTS:          threadTS,
			Subtype:           subtype,
			IsEdited:          edited,
			Attachments:       extractAttachments(files),
			Blocks:            extractBlocks(blocks),
			LegacyAttachments: extractLegacyAttachments(attachments),
		},
	})
}

func (h *rtmEventHandler) OnView(evt slackclient.ViewEvent) {
	if h.program == nil {
		return
	}
	h.program.Send(ui.AppViewMsg{
		TeamID:      h.workspaceID,
		ClientToken: evt.ClientToken,
		Updated:     evt.Updated,
		View:        evt.View,
	})
}

func (h *rtmEventHandler) sendAssistantStatus(s slackclient.AssistantStatus) {
	if h.program == nil {
		return
	}
	h.program.Send(ui.AssistantStatusMsg{
		TeamID:    h.workspaceID,
		ChannelID: s.ChannelID,
		ThreadTS:  s.ThreadTS,
		BotUserID: s.BotUserID,
		Status:    s.Status,
	})
}

// reactionReachesUI reports whether a reaction event is sent to the UI. A
// background workspace's reactions matter there only as the tracked agent
// thread's ack, which only a bot gives, and each one sent costs a full
// Update and View. Bot-ness is read where agent-thread detection reads it,
// the user cache, so the tracked thread's bot always passes.
func (h *rtmEventHandler) reactionReachesUI(userID string) bool {
	if h.isActive == nil || h.isActive() {
		return true
	}
	u, err := h.db.GetUser(userID)
	return err == nil && u.IsBot
}
