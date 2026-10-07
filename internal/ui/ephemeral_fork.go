package ui

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/messages"
)

// ephemerals holds, per channel ID, the messages Slack showed to this user
// alone this session. Slack returns none of them from history and slk
// never writes them to its cache, so every load that replaces a pane's
// messages (a channel switch, the fetch that follows it, a reconnect
// refresh, a thread opening) lays them back in. They are gone after a
// restart, as they are from Slack.
type ephemerals map[string][]messages.MessageItem

// reduceEphemeral handles a NewMessageMsg carrying an ephemeral: it is
// remembered, and shown in the panes on its channel or its thread, in
// place of the one with its ts when it replaces one. It is not a reply to
// count, a message to mark read, or agent-thread activity.
func reduceEphemeral(a *App, m NewMessageMsg) tea.Cmd {
	a.ephemerals.remember(m.ChannelID, m.Message)
	if m.TeamID != "" && m.TeamID != a.activeTeamID {
		return nil
	}
	if threadTS := m.Message.ThreadTS; threadTS == "" || threadTS == m.Message.TS {
		for _, mm := range a.modelsForChannel(m.ChannelID) {
			if !mm.ReplaceEphemeral(cloneMessageItem(m.Message)) {
				mm.AppendMessage(cloneMessageItem(m.Message))
			}
		}
		return nil
	}
	if a.threadVisible && m.ChannelID == a.threadPanel.ChannelID() && m.Message.ThreadTS == a.threadPanel.ThreadTS() {
		if !a.threadPanel.ReplaceEphemeral(cloneMessageItem(m.Message)) {
			a.threadPanel.AddIncomingReply(cloneMessageItem(m.Message))
		}
	}
	return nil
}

// remember keeps msg, in place of the ephemeral with its ts if there is one.
func (e *ephemerals) remember(channelID string, msg messages.MessageItem) {
	if *e == nil {
		*e = ephemerals{}
	}
	list := (*e)[channelID]
	if i := slices.IndexFunc(list, func(m messages.MessageItem) bool { return m.TS == msg.TS }); i >= 0 {
		list[i] = msg
	} else {
		list = append(list, msg)
	}
	(*e)[channelID] = list
}

// forget drops the ephemeral with ts, which Slack deleted.
func (e ephemerals) forget(channelID, ts string) {
	if len(e[channelID]) == 0 {
		return
	}
	e[channelID] = slices.DeleteFunc(e[channelID], func(m messages.MessageItem) bool { return m.TS == ts })
}

// inChannel returns msgs, channelID's messages for its pane, with the
// channel's top-level ephemerals in ts order.
func (e ephemerals) inChannel(channelID string, msgs []messages.MessageItem) []messages.MessageItem {
	return e.layIn(channelID, msgs, func(m messages.MessageItem) bool {
		return m.ThreadTS == "" || m.ThreadTS == m.TS
	})
}

// inThread returns replies, threadTS's replies, with the ephemerals
// posted in that thread in ts order.
func (e ephemerals) inThread(channelID, threadTS string, replies []messages.MessageItem) []messages.MessageItem {
	return e.layIn(channelID, replies, func(m messages.MessageItem) bool {
		return m.ThreadTS == threadTS && m.TS != threadTS
	})
}

// layIn returns msgs with each of channelID's ephemerals that belongs and
// that msgs lacks inserted by ts, each a copy of its own as every load
// into a pane gets (cloneMessageItem). msgs itself, nil included, comes
// back when there is nothing to add, and is never written to.
func (e ephemerals) layIn(channelID string, msgs []messages.MessageItem, belongs func(messages.MessageItem) bool) []messages.MessageItem {
	out := msgs
	for _, eph := range e[channelID] {
		if !belongs(eph) || slices.ContainsFunc(out, func(m messages.MessageItem) bool { return m.TS == eph.TS }) {
			continue
		}
		at := slices.IndexFunc(out, func(m messages.MessageItem) bool { return m.TS > eph.TS })
		if at < 0 {
			at = len(out)
		}
		out = slices.Insert(slices.Clip(out), at, cloneMessageItem(eph))
	}
	return out
}
