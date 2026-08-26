package main

import (
	"encoding/json"
	"time"

	"github.com/gammons/slk/internal/bootstrap"
	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/debuglog"
	"github.com/slack-go/slack"
)

// persistBootstrapHistory writes the opened channel's bootstrap-fetched
// history into the message cache and stamps the channel synced, so the
// channel-open tier dispatch renders it as fresh instead of refetching
// the history bootstrap.Run just fetched. Success means the server sent
// messages or confirmed cached ones unchanged (the incremental history
// fallback); empty on both means the load failed — nothing is stamped
// and the UI falls back to its own fetch.
//
// RawJSON keeps the server's original bytes rather than a re-encoded
// slack.Message, so the cached render loses nothing slack-go doesn't
// model.
func persistBootstrapHistory(db *cache.DB, teamID string, res *bootstrap.Result) {
	if res == nil || res.OpenedChannelID == "" {
		return
	}
	if len(res.Messages) == 0 && len(res.UnchangedTS) == 0 {
		return
	}
	now := time.Now().Unix()
	for _, raw := range res.Messages {
		var m slack.Message
		if err := json.Unmarshal(raw, &m); err != nil {
			debuglog.Cache("persistBootstrapHistory: channel=%s decode: %v", res.OpenedChannelID, err)
			continue
		}
		if m.Timestamp == "" {
			continue
		}
		authorID := m.User
		if authorID == "" {
			authorID = m.BotID
		}
		if err := db.UpsertMessage(cache.Message{
			TS:          m.Timestamp,
			ChannelID:   res.OpenedChannelID,
			WorkspaceID: teamID,
			UserID:      authorID,
			Text:        m.Text,
			ThreadTS:    m.ThreadTimestamp,
			ReplyCount:  m.ReplyCount,
			Subtype:     m.SubType,
			RawJSON:     string(raw),
			CreatedAt:   now,
		}); err != nil {
			debuglog.Cache("persistBootstrapHistory: channel=%s ts=%s upsert: %v", res.OpenedChannelID, m.Timestamp, err)
			continue
		}
		for _, r := range m.Reactions {
			_ = db.UpsertReaction(m.Timestamp, res.OpenedChannelID, r.Name, r.Users, r.Count)
		}
	}
	if err := db.SetChannelSyncedAt(res.OpenedChannelID, now); err != nil {
		debuglog.Cache("persistBootstrapHistory: SetChannelSyncedAt %s: %v", res.OpenedChannelID, err)
	}
}
