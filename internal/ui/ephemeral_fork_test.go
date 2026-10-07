package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
)

func paneTSes(a *App) (ts []string) {
	for _, m := range a.messagepane.Messages() {
		ts = append(ts, m.TS)
	}
	return ts
}

// An ephemeral that lands in the open channel stays there, in ts order,
// through what replaces the pane's messages from history in a session:
// the fetch that lands, and a switch away and back that reads the cache.
// Neither holds it, as Slack's history and slk's cache never do.
func TestEphemeral_StaysThroughChannelReloads(t *testing.T) {
	history := []messages.MessageItem{{TS: "100.0", Text: "before"}, {TS: "300.0", Text: "after"}}
	a := newTestApp(t, withActiveTeam("T1"), withActiveChannel("C1"), withMessages(history...),
		withChannelService(core.ChannelServiceFuncs{
			ReadCache: func(id ids.ChannelID) []messages.MessageItem {
				if id == "C1" {
					return history
				}
				return nil
			},
		}))
	eph := messages.MessageItem{TS: "200.0", Text: "only you", IsEphemeral: true}

	a.Update(NewMessageMsg{TeamID: "T1", ChannelID: "C1", Message: eph})
	if got := paneTSes(a); len(got) != 3 || got[2] != "200.0" {
		t.Fatalf("pane = %v, want the ephemeral appended", got)
	}

	a.Update(MessagesLoadedMsg{ChannelID: "C1", Messages: history})
	want := []string{"100.0", "200.0", "300.0"}
	if got := paneTSes(a); !slices.Equal(got, want) {
		t.Errorf("after the fetch lands: pane = %v, want %v", got, want)
	}

	a.Update(ChannelSelectedMsg{ID: "C2", Name: "random"})
	a.Update(ChannelSelectedMsg{ID: "C1", Name: "general"})
	if got := paneTSes(a); !slices.Equal(got, want) {
		t.Errorf("after a switch back: pane = %v, want %v", got, want)
	}
	if len(history) != 2 {
		t.Errorf("history slice written to: %v", history)
	}
}

// An ephemeral posted in a thread shows in that thread's panel, live and
// on every reload of its replies, and never in the channel pane; it does
// not count as a reply of its parent.
func TestEphemeral_InThreadShowsInItsPanelOnly(t *testing.T) {
	parent := messages.MessageItem{TS: "100.0", ThreadTS: "100.0", Text: "parent", ReplyCount: 1}
	reply := messages.MessageItem{TS: "150.0", ThreadTS: "100.0", Text: "reply"}
	a := newTestApp(t, withActiveTeam("T1"), withActiveChannel("C1"), withMessages(parent))
	a.setThreadFetcherForTest(func(ids.ChannelID, ids.ThreadTS) core.Msg { return nil })
	eph := messages.MessageItem{TS: "200.0", ThreadTS: "100.0", Text: "only you", IsEphemeral: true}

	a.messagepane.SelectByTS(parent.TS)
	drainCmd(a.openThreadForSelectedMessage())
	a.Update(ThreadRepliesLoadedMsg{ThreadTS: "100.0", Replies: []messages.MessageItem{reply}})
	a.Update(NewMessageMsg{TeamID: "T1", ChannelID: "C1", Message: eph})

	replyTSes := func() (ts []string) {
		for _, r := range a.threadPanel.Replies() {
			ts = append(ts, r.TS)
		}
		return ts
	}
	if got := replyTSes(); !slices.Equal(got, []string{"150.0", "200.0"}) {
		t.Fatalf("thread replies = %v, want the ephemeral after the reply", got)
	}
	if got := paneTSes(a); !slices.Equal(got, []string{"100.0"}) {
		t.Errorf("channel pane = %v, want the parent alone", got)
	}
	if got := a.messagepane.Messages()[0].ReplyCount; got != 1 {
		t.Errorf("parent ReplyCount = %d, want 1", got)
	}

	a.Update(ThreadRepliesLoadedMsg{ThreadTS: "100.0", Replies: []messages.MessageItem{reply}})
	if got := replyTSes(); !slices.Equal(got, []string{"150.0", "200.0"}) {
		t.Errorf("after the replies reload: thread replies = %v, want the ephemeral kept", got)
	}

	a.CloseThread()
	drainCmd(a.openThreadForSelectedMessage())
	if got := replyTSes(); !slices.Equal(got, []string{"200.0"}) {
		t.Errorf("on reopen, before replies load: thread replies = %v, want the ephemeral", got)
	}
}

// An ephemeral older than every loaded message shows in the pane, and
// history paging still anchors on the oldest loaded message, so nothing
// between the two is skipped; the older page lands around it in ts order.
func TestEphemeral_OlderThanLoadedHistoryKeepsPagingAnchor(t *testing.T) {
	var anchor ids.MessageTS
	a := newTestApp(t, withActiveTeam("T1"), withActiveChannel("C1"),
		withChannelService(core.ChannelServiceFuncs{
			FetchOlder: func(_ ids.ChannelID, oldestTS ids.MessageTS) core.Msg {
				anchor = oldestTS
				return nil
			},
		}))
	a.ephemerals.remember("C1", messages.MessageItem{TS: "200.0", IsEphemeral: true})

	a.Update(MessagesLoadedMsg{ChannelID: "C1", Messages: []messages.MessageItem{{TS: "500.0"}, {TS: "600.0"}}})
	if got, want := paneTSes(a), []string{"200.0", "500.0", "600.0"}; !slices.Equal(got, want) {
		t.Fatalf("pane = %v, want %v", got, want)
	}

	a.messagepane.SelectByTS("200.0")
	drainCmd(a.maybeFetchOlderHistory(true))
	if anchor != "500.0" {
		t.Errorf("FetchOlder anchored on %q, want the oldest loaded message 500.0", anchor)
	}

	a.Update(OlderMessagesLoadedMsg{ChannelID: "C1", AnchorTS: "500.0", Messages: []messages.MessageItem{{TS: "100.0"}, {TS: "300.0"}}})
	if got, want := paneTSes(a), []string{"100.0", "200.0", "300.0", "500.0", "600.0"}; !slices.Equal(got, want) {
		t.Errorf("after the older page lands: pane = %v, want %v", got, want)
	}
	if sel, _ := a.messagepane.SelectedMessage(); sel.TS != "200.0" {
		t.Errorf("selected %q after the older page lands, want the ephemeral 200.0 kept", sel.TS)
	}
}

// A jump to a message lays the channel's ephemerals into the window it
// loads, as every other load does.
func TestEphemeral_LaidIntoJumpWindow(t *testing.T) {
	a := newTestApp(t, withActiveTeam("T1"), withActiveChannel("C1"))
	a.ephemerals.remember("C1", messages.MessageItem{TS: "250.0", IsEphemeral: true})

	a.Update(MessagesAroundLoadedMsg{ChannelID: "C1", TargetTS: "200.0", Messages: []messages.MessageItem{{TS: "200.0"}, {TS: "300.0"}}})
	if got, want := paneTSes(a), []string{"200.0", "250.0", "300.0"}; !slices.Equal(got, want) {
		t.Errorf("pane = %v, want %v", got, want)
	}
}

// A channel with no cache and one ephemeral still loads like a channel
// never opened: spinner and fetch, not the ephemeral alone as if it were
// the whole history.
func TestEphemeral_UncachedChannelStillShowsSpinner(t *testing.T) {
	fetched := false
	a := newTestApp(t, withActiveTeam("T1"), withChannelService(core.ChannelServiceFuncs{
		Fetch: func(ids.ChannelID, string) core.Msg {
			fetched = true
			return nil
		},
	}))
	a.ephemerals.remember("C1", messages.MessageItem{TS: "200.0", IsEphemeral: true})

	_, cmd := a.Update(ChannelSelectedMsg{ID: "C1", Name: "general"})
	drainCmd(cmd)
	if !a.messagepane.IsLoading() || !fetched {
		t.Errorf("loading %v fetched %v, want the spinner and a fetch", a.messagepane.IsLoading(), fetched)
	}
}

// Every pane gets its own copy of a remembered ephemeral, as the rule
// for loads into windows has it (cloneMessageItem), so a write in one
// reaches neither the others nor what the next reload lays in.
func TestEphemeral_EachPaneGetsItsOwnCopy(t *testing.T) {
	reactions := func() []messages.ReactionItem { return []messages.ReactionItem{{Emoji: "eyes", Count: 1}} }
	a := newTestApp(t, withActiveTeam("T1"), withActiveChannel("C1"))
	a.ephemerals.remember("C1", messages.MessageItem{TS: "200.0", IsEphemeral: true, Reactions: reactions()})
	out := a.ephemerals.inChannel("C1", nil)
	out[0].Reactions[0].Count = 9
	if got := a.ephemerals["C1"][0].Reactions[0].Count; got != 1 {
		t.Errorf("a write in a pane's copy reached the remembered ephemeral: count %d", got)
	}

	parent := messages.MessageItem{TS: "100.0", ThreadTS: "100.0", ReplyCount: 1}
	b := newTestApp(t, withActiveTeam("T1"), withActiveChannel("C1"), withMessages(parent))
	b.setThreadFetcherForTest(func(ids.ChannelID, ids.ThreadTS) core.Msg { return nil })
	b.messagepane.SelectByTS(parent.TS)
	drainCmd(b.openThreadForSelectedMessage())
	b.Update(NewMessageMsg{TeamID: "T1", ChannelID: "C1", Message: messages.MessageItem{TS: "200.0", ThreadTS: "100.0", IsEphemeral: true, Reactions: reactions()}})
	b.threadPanel.Replies()[0].Reactions[0].Count = 9
	if got := b.ephemerals["C1"][0].Reactions[0].Count; got != 1 {
		t.Errorf("a write in the thread panel reached the remembered ephemeral: count %d", got)
	}
}

// A fresh cache is marked read to its newest message that is not an
// ephemeral, and not at all when it holds only ephemerals.
func TestEphemeral_FreshCacheMarksReadPastIt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cached []messages.MessageItem
		want   []string
	}{
		{"newest is ephemeral", []messages.MessageItem{{TS: "100.0"}}, []string{"100.0"}},
		{"only ephemerals", nil, nil},
	} {
		var marks []string
		cached := tc.cached
		a := newTestApp(t, withActiveTeam("T1"), withChannelService(core.ChannelServiceFuncs{
			ReadCache: func(ids.ChannelID) []messages.MessageItem { return cached },
			SyncedAt:  func(ids.ChannelID) int64 { return time.Now().Unix() },
			MarkRead: func(_ ids.ChannelID, ts ids.MessageTS) core.Msg {
				marks = append(marks, string(ts))
				return nil
			},
		}))
		a.ephemerals.remember("C1", messages.MessageItem{TS: "200.0", IsEphemeral: true})
		_, cmd := a.Update(ChannelSelectedMsg{ID: "C1", Name: "general"})
		drainCmd(cmd)
		if !slices.Equal(marks, tc.want) {
			t.Errorf("%s: marked read to %v, want %v", tc.name, marks, tc.want)
		}
	}
}

// A deleted ephemeral does not come back on the next reload.
func TestEphemeral_DeletedStaysDeleted(t *testing.T) {
	a := newTestApp(t, withActiveTeam("T1"), withActiveChannel("C1"), withMessages(messages.MessageItem{TS: "100.0"}))
	a.Update(NewMessageMsg{TeamID: "T1", ChannelID: "C1", Message: messages.MessageItem{TS: "200.0", IsEphemeral: true}})
	a.Update(WSMessageDeletedMsg{TeamID: "T1", ChannelID: "C1", TS: "200.0"})

	a.Update(MessagesLoadedMsg{ChannelID: "C1", Messages: []messages.MessageItem{{TS: "100.0"}}})
	if got := paneTSes(a); !slices.Equal(got, []string{"100.0"}) {
		t.Errorf("pane = %v, want the deleted ephemeral gone", got)
	}
}

// An app that replaces its ephemeral sends the same ts with new content:
// the pane shows the new content in the ephemeral's place, and so does
// the next reload.
func TestEphemeral_ReplacedInPlace(t *testing.T) {
	a := newTestApp(t, withActiveTeam("T1"), withActiveChannel("C1"), withMessages(messages.MessageItem{TS: "100.0"}))
	a.Update(NewMessageMsg{TeamID: "T1", ChannelID: "C1", Message: messages.MessageItem{TS: "200.0", Text: "first", IsEphemeral: true}})
	a.Update(NewMessageMsg{TeamID: "T1", ChannelID: "C1", Message: messages.MessageItem{TS: "200.0", Text: "replaced", IsEphemeral: true}})

	texts := func() (out []string) {
		for _, m := range a.messagepane.Messages() {
			out = append(out, m.TS+" "+m.Text)
		}
		return out
	}
	want := []string{"100.0 ", "200.0 replaced"}
	if got := texts(); !slices.Equal(got, want) {
		t.Errorf("pane = %q, want %q", got, want)
	}
	if a.messagepane.Messages()[1].IsEdited {
		t.Error("the replaced ephemeral is marked edited; Slack marks none, and a reload would not")
	}
	a.Update(MessagesLoadedMsg{ChannelID: "C1", Messages: []messages.MessageItem{{TS: "100.0"}}})
	if got := texts(); !slices.Equal(got, want) {
		t.Errorf("after a reload: pane = %q, want %q", got, want)
	}
}
