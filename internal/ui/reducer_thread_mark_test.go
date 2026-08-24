package ui

import (
	"testing"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
)

// A live reply rendered by the open thread panel is a read event: it must
// advance the thread's read cursor on Slack (debounced, latest-TS-wins).
// Regression: watching a thread live was the one way to read replies that
// never marked them read — the cursor only advanced on open/refetch.

type markCall struct {
	channelID string
	threadTS  string
	ts        string
}

func setMarkRecorder(a *App) *[]markCall {
	calls := &[]markCall{}
	a.SetThreadService(NewThreadService(ThreadServiceFuncs{
		Mark: func(channelID ids.ChannelID, threadTS ids.ThreadTS, ts ids.MessageTS) {
			*calls = append(*calls, markCall{string(channelID), string(threadTS), string(ts)})
		},
	}))
	return calls
}

func liveReply(ts string) NewMessageMsg {
	return NewMessageMsg{
		ChannelID: "C1",
		Message:   messages.MessageItem{TS: ts, ThreadTS: "100.0", UserID: "U2", Text: "reply"},
	}
}

// fireMarkTick synthesizes the threadMarkDebounceMsg the most recent
// scheduleThreadMark would deliver (we can't observe the payload behind
// tea.Tick, and invoking the tick cmd would sleep for the real debounce)
// and executes any cmd the handler returns.
func fireMarkTick(a *App, gen uint64) {
	_, cmd := a.Update(threadMarkDebounceMsg{channelID: "C1", threadTS: "100.0", gen: gen})
	drainBatch(cmd)
}

func TestLiveReplyMarksThreadReadAfterDebounce(t *testing.T) {
	a := NewApp()
	calls := setMarkRecorder(a)
	openThreadPanel(a, "C1", "100.0")

	_ = reduceNewMessage(a, liveReply("101.0"))
	if a.pendingThreadMarkGen == 0 {
		t.Fatal("live reply into the open panel must schedule a debounced mark")
	}
	fireMarkTick(a, a.pendingThreadMarkGen)

	want := markCall{channelID: "C1", threadTS: "100.0", ts: "101.0"}
	if len(*calls) != 1 || (*calls)[0] != want {
		t.Fatalf("want exactly one Mark%+v; got %+v", want, *calls)
	}
}

func TestLiveReplyBurstCoalescesToOneMark(t *testing.T) {
	a := NewApp()
	calls := setMarkRecorder(a)
	openThreadPanel(a, "C1", "100.0")

	var gens []uint64
	for _, ts := range []string{"101.0", "102.0", "103.0"} {
		_ = reduceNewMessage(a, liveReply(ts))
		gens = append(gens, a.pendingThreadMarkGen)
	}
	// The burst's earlier ticks carry stale generations and must drop
	// without marking.
	for _, gen := range gens[:len(gens)-1] {
		fireMarkTick(a, gen)
	}
	if len(*calls) != 0 {
		t.Fatalf("stale ticks must not mark; got %+v", *calls)
	}
	// The last tick marks once, with the burst's newest reply TS.
	fireMarkTick(a, gens[len(gens)-1])
	want := markCall{channelID: "C1", threadTS: "100.0", ts: "103.0"}
	if len(*calls) != 1 || (*calls)[0] != want {
		t.Fatalf("want exactly one Mark%+v; got %+v", want, *calls)
	}
}

func TestMarkTickDroppedWhenPanelClosed(t *testing.T) {
	a := NewApp()
	calls := setMarkRecorder(a)
	openThreadPanel(a, "C1", "100.0")

	_ = reduceNewMessage(a, liveReply("101.0"))
	a.threadVisible = false
	fireMarkTick(a, a.pendingThreadMarkGen)

	if len(*calls) != 0 {
		t.Fatalf("closed panel must not mark; got %+v", *calls)
	}
}

func TestMarkTickDroppedWhenPanelSwitchedThreads(t *testing.T) {
	a := NewApp()
	calls := setMarkRecorder(a)
	openThreadPanel(a, "C1", "100.0")

	_ = reduceNewMessage(a, liveReply("101.0"))
	openThreadPanel(a, "C1", "200.0")
	fireMarkTick(a, a.pendingThreadMarkGen)

	if len(*calls) != 0 {
		t.Fatalf("a tick for a thread the panel no longer shows must not mark; got %+v", *calls)
	}
}

// Close-and-reopen the same thread inside the debounce window: the tick's
// generation and panel identity both still match, but the reopened panel
// holds no replies yet (repopulation is in flight). Marking would fall back
// to a stale TS and regress Slack's cursor to the thread start; the tick
// must drop and leave the mark to the reopen's ThreadRepliesLoadedMsg.
func TestMarkTickDroppedWhenReopenedPanelNotYetPopulated(t *testing.T) {
	a := NewApp()
	calls := setMarkRecorder(a)
	openThreadPanel(a, "C1", "100.0")

	_ = reduceNewMessage(a, liveReply("101.0"))
	a.threadPanel.Clear()
	openThreadPanel(a, "C1", "100.0")
	fireMarkTick(a, a.pendingThreadMarkGen)

	if len(*calls) != 0 {
		t.Fatalf("empty repopulating panel must not mark; got %+v", *calls)
	}
}

// The user's own optimistic reply ("local:<n>", no Slack TS yet) can be
// the panel's newest row when the tick fires. Slack would reject it as a
// mark TS; the mark must use the newest real reply instead.
func TestMarkSkipsOptimisticLocalPlaceholder(t *testing.T) {
	a := NewApp()
	calls := setMarkRecorder(a)
	openThreadPanel(a, "C1", "100.0")

	_ = reduceNewMessage(a, liveReply("101.0"))
	a.threadPanel.AddReply(messages.MessageItem{TS: "local:1", ThreadTS: "100.0", UserID: "U1", Text: "own reply"})
	fireMarkTick(a, a.pendingThreadMarkGen)

	want := markCall{channelID: "C1", threadTS: "100.0", ts: "101.0"}
	if len(*calls) != 1 || (*calls)[0] != want {
		t.Fatalf("want Mark%+v (newest real reply); got %+v", want, *calls)
	}
}

func TestLiveReplyMarkFlipsThreadsViewRowAndBadge(t *testing.T) {
	a := NewApp()
	setMarkRecorder(a)
	openThreadPanel(a, "C1", "100.0")
	a.threadsView.SetSummaries([]cache.ThreadSummary{
		{ChannelID: "C1", ThreadTS: "100.0", Unread: true},
		{ChannelID: "C2", ThreadTS: "300.0", Unread: true},
	})
	a.sidebar.SetThreadsUnreadCount(a.threadsView.UnreadCount())

	_ = reduceNewMessage(a, liveReply("101.0"))
	fireMarkTick(a, a.pendingThreadMarkGen)

	for _, s := range a.threadsView.Summaries() {
		if s.ThreadTS == "100.0" && s.Unread {
			t.Fatal("marked thread's row must flip to read in the threads view")
		}
	}
	if got := a.threadsView.UnreadCount(); got != 1 {
		t.Fatalf("threads unread count = %d after mark; want 1 (the other thread)", got)
	}
}

// The debounced mark must go through the markThreadReadLocally funnel so
// the tracked agent thread's sidebar row clears with the rest of the local
// read state — a direct threads-view flip would leave the herdr row
// claiming an unread reply the mark just declared read.
func TestMarkTickClearsTrackedAgentThreadRow(t *testing.T) {
	a, calls, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")

	_, _ = reduceSend(a, NewMessageMsg{
		TeamID:    "T1",
		ChannelID: "C1",
		Message:   messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "done"},
	})
	if len(*unreads) == 0 {
		t.Fatal("test setup broke: the bot reply was not counted unread")
	}

	before := len(*calls)
	fireMarkTick(a, a.pendingThreadMarkGen)
	if len(*calls) <= before {
		t.Fatal("mark tick must clear the tracked agent row via the read-state funnel")
	}
	last := (*calls)[len(*calls)-1]
	if last.working || last.status != "" {
		t.Fatalf("agent row after mark = %+v; want idle with empty status", last)
	}
}

// Inside herdr, an unviewed pane renders the reply but must not mark it
// read: the tab's unseen indicator is pointing the user at replies they
// haven't looked at, and a mark would clear the unread text out from
// under it. Outside herdr no HerdrTabViewMsg ever arrives and PaneViewed
// stays true, so this gate changes nothing there.
func TestNoMarkScheduledWhenPaneUnviewed(t *testing.T) {
	a := NewApp()
	openThreadPanel(a, "C1", "100.0")

	a.Update(HerdrTabViewMsg{Viewed: false})
	_ = reduceNewMessage(a, liveReply("101.0"))
	if a.pendingThreadMarkGen != 0 {
		t.Fatal("a reply rendered in an unviewed herdr pane must not schedule a mark")
	}

	a.Update(HerdrTabViewMsg{Viewed: true})
	_ = reduceNewMessage(a, liveReply("102.0"))
	if a.pendingThreadMarkGen == 0 {
		t.Fatal("a reply rendered in a viewed pane must schedule a mark")
	}
}

// Burst tail: a reply arrives viewed (mark scheduled), the user tabs away,
// and a second reply arrives unviewed before the tick fires. The tick's TS
// is resolved from the panel at fire time, so without a fire-time
// viewedness recheck it would mark the thread read past a reply the user
// never saw.
func TestMarkTickDroppedWhenPaneUnviewedAtFire(t *testing.T) {
	a := NewApp()
	calls := setMarkRecorder(a)
	openThreadPanel(a, "C1", "100.0")

	_ = reduceNewMessage(a, liveReply("101.0"))
	a.Update(HerdrTabViewMsg{Viewed: false})
	_ = reduceNewMessage(a, liveReply("102.0"))
	fireMarkTick(a, a.pendingThreadMarkGen)

	if len(*calls) != 0 {
		t.Fatalf("tick firing into an unviewed pane must not mark; got %+v", *calls)
	}
}

// Replies that arrive while the pane sits in a background herdr tab render
// unmarked (the unviewed gate above). Focusing the tab is what puts them on
// screen, so the refocus is the read event: it must schedule the same
// debounced mark a live reply gets. Regression: without it the thread
// stayed unread on Slack forever, and every tab switch away re-asserted
// the unread to herdr's agent sidebar.
func TestTabRefocusSchedulesMarkForOpenThread(t *testing.T) {
	a := NewApp()
	calls := setMarkRecorder(a)
	openThreadPanel(a, "C1", "100.0")

	a.Update(HerdrTabViewMsg{Viewed: false})
	_ = reduceNewMessage(a, liveReply("101.0"))
	if a.pendingThreadMarkGen != 0 {
		t.Fatal("test setup broke: unviewed reply must not schedule a mark")
	}

	a.Update(HerdrTabViewMsg{Viewed: true})
	if a.pendingThreadMarkGen == 0 {
		t.Fatal("refocusing the tab over an open thread panel must schedule a mark")
	}
	fireMarkTick(a, a.pendingThreadMarkGen)

	want := markCall{channelID: "C1", threadTS: "100.0", ts: "101.0"}
	if len(*calls) != 1 || (*calls)[0] != want {
		t.Fatalf("want exactly one Mark%+v; got %+v", want, *calls)
	}
}

// A quick flick through the tab must not mark: the refocus schedules, but
// the fire-time viewedness recheck sees the user already gone.
func TestTabRefocusMarkDroppedWhenUserFlicksAway(t *testing.T) {
	a := NewApp()
	calls := setMarkRecorder(a)
	openThreadPanel(a, "C1", "100.0")

	a.Update(HerdrTabViewMsg{Viewed: false})
	_ = reduceNewMessage(a, liveReply("101.0"))
	a.Update(HerdrTabViewMsg{Viewed: true})
	a.Update(HerdrTabViewMsg{Viewed: false})
	fireMarkTick(a, a.pendingThreadMarkGen)

	if len(*calls) != 0 {
		t.Fatalf("tick firing after the user flicked away must not mark; got %+v", *calls)
	}
}

func TestTabRefocusSchedulesNoMarkWithoutOpenThread(t *testing.T) {
	a := NewApp()
	setMarkRecorder(a)

	a.Update(HerdrTabViewMsg{Viewed: false})
	a.Update(HerdrTabViewMsg{Viewed: true})
	if a.pendingThreadMarkGen != 0 {
		t.Fatal("refocus with no thread panel open must not schedule a mark")
	}

	openThreadPanel(a, "C1", "100.0")
	a.threadVisible = false
	a.Update(HerdrTabViewMsg{Viewed: false})
	a.Update(HerdrTabViewMsg{Viewed: true})
	if a.pendingThreadMarkGen != 0 {
		t.Fatal("refocus with the panel hidden must not schedule a mark")
	}
}

func TestNoMarkScheduledWhenPanelHidden(t *testing.T) {
	a := NewApp()
	openThreadPanel(a, "C1", "100.0")
	a.threadVisible = false

	_ = reduceNewMessage(a, liveReply("101.0"))
	if a.pendingThreadMarkGen != 0 {
		t.Fatal("a reply the panel never rendered must not schedule a mark")
	}
}

func TestNoMarkScheduledForDifferentThread(t *testing.T) {
	a := NewApp()
	openThreadPanel(a, "C1", "100.0")

	_ = reduceNewMessage(a, NewMessageMsg{
		ChannelID: "C1",
		Message:   messages.MessageItem{TS: "201.0", ThreadTS: "200.0", UserID: "U2", Text: "other thread"},
	})
	if a.pendingThreadMarkGen != 0 {
		t.Fatal("a reply for a thread the panel is not showing must not schedule a mark")
	}
}
