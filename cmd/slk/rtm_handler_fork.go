package main

import (
	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/ui"
	"github.com/gammons/slk/internal/ui/messages"
)

// OnEphemeralMessage hands a message Slack shows to this user alone to
// the UI, which keeps it in memory for the session. Slack never returns
// one from history, so it writes nothing to SQLite (no row, no
// latest_synced_ts, no unread) and sends no notification: the user
// caused it, often by a click in another client.
func (h *rtmEventHandler) OnEphemeralMessage(channelID, userID, ts, text, threadTS, subtype string, files []slack.File, blocks slack.Blocks, attachments []slack.Attachment, botID, username string) {
	if h.program == nil {
		return
	}
	authorID := userID
	if authorID == "" && botID != "" {
		authorID = botID
		if h.wsCtx != nil && h.wsCtx.UserResolver != nil {
			h.wsCtx.UserResolver.RequestBot(botID, username)
		}
	}
	userName, ok := resolveUserCached(authorID, h.userNames, h.db)
	if !ok {
		userName = authorID
		if userID == "" && username != "" {
			userName = username
		} else if userID != "" && h.wsCtx != nil && h.wsCtx.UserResolver != nil {
			h.wsCtx.UserResolver.Request(userID)
		}
	}
	debuglog.Cache("OnEphemeralMessage: team=%s channel=%s ts=%s thread_ts=%s decision=dispatched_in_memory_only",
		h.workspaceID, channelID, ts, threadTS)
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
			Attachments:       extractAttachments(files),
			Blocks:            extractBlocks(blocks),
			LegacyAttachments: extractLegacyAttachments(attachments),
			IsEphemeral:       true,
		},
	})
}
