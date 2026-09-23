package ui

import (
	"testing"

	"github.com/gammons/slk/internal/ui/messages"
)

// Inside herdr, a pane in a background tab renders replies the user has
// not looked at. Upstream's staged read marks (recordThreadMark,
// scheduleMarkFlush) are held by flushPendingMarks' PaneViewed gate until
// the tab is viewed again. Outside herdr no HerdrTabViewMsg ever arrives
// and PaneViewed stays true, so the gate changes nothing there.

func liveReply(ts string) NewMessageMsg {
	return NewMessageMsg{
		ChannelID: "C1",
		Message:   messages.MessageItem{TS: ts, ThreadTS: "100.0", UserID: "U2", Text: "reply"},
	}
}

// markCaptureOpenThread is markCapture with thread 100.0 open in C1 while
// another channel is active, so only the thread leg stages a mark.
func markCaptureOpenThread(t *testing.T) (*App, *[]string) {
	t.Helper()
	a, calls := markCapture(t)
	a.activeChannelID = "C9"
	openThreadPanel(a, "C1", "100.0")
	return a, calls
}

func TestNoMarkWhenPaneUnviewed(t *testing.T) {
	a, calls := markCaptureOpenThread(t)

	a.Update(HerdrTabViewMsg{Viewed: false})
	_, cmd := a.Update(liveReply("101.0"))
	feed(t, a, cmd, 0)

	if len(*calls) != 0 {
		t.Fatalf("a reply rendered in an unviewed herdr pane must not mark; got %v", *calls)
	}
	if got := a.pendingThreadMark.ts; got != "101.0" {
		t.Fatalf("pendingThreadMark.ts = %q; the held mark must stay staged at 101.0", got)
	}
}

// Burst tail: a reply arrives viewed (flush scheduled), the user tabs away,
// and a second reply arrives unviewed before the tick fires. Without the
// fire-time viewedness check the flush would mark the thread read past a
// reply the user never saw.
func TestMarkFlushHeldWhenPaneUnviewedAtFire(t *testing.T) {
	a, calls := markCaptureOpenThread(t)

	_, tick := a.Update(liveReply("101.0"))
	a.Update(HerdrTabViewMsg{Viewed: false})
	a.Update(liveReply("102.0"))
	feed(t, a, tick, 0)

	if len(*calls) != 0 {
		t.Fatalf("a flush firing into an unviewed pane must not mark; got %v", *calls)
	}
}

// Focusing the tab is what puts the held replies on screen, so the refocus
// is the read event. Regression: without it the thread stayed unread on
// Slack forever, and every tab switch away re-asserted the unread to
// herdr's agent sidebar.
func TestTabRefocusFlushesHeldThreadMark(t *testing.T) {
	a, calls := markCaptureOpenThread(t)

	a.Update(HerdrTabViewMsg{Viewed: false})
	_, cmd := a.Update(liveReply("101.0"))
	feed(t, a, cmd, 0)
	if len(*calls) != 0 {
		t.Fatalf("test setup broke: unviewed reply marked %v", *calls)
	}

	_, cmd = a.Update(HerdrTabViewMsg{Viewed: true})
	feed(t, a, cmd, 0)

	if len(*calls) != 1 || (*calls)[0] != "th:C1/100.0/101.0" {
		t.Fatalf("calls = %v, want one thread mark at 101.0", *calls)
	}
}

// A quick flick through the tab must not mark: the refocus schedules the
// flush, but the gate sees the user already gone when it fires.
func TestTabRefocusMarkHeldWhenUserFlicksAway(t *testing.T) {
	a, calls := markCaptureOpenThread(t)

	a.Update(HerdrTabViewMsg{Viewed: false})
	_, cmd := a.Update(liveReply("101.0"))
	feed(t, a, cmd, 0)
	_, tick := a.Update(HerdrTabViewMsg{Viewed: true})
	a.Update(HerdrTabViewMsg{Viewed: false})
	feed(t, a, tick, 0)

	if len(*calls) != 0 {
		t.Fatalf("a flush firing after the user flicked away must not mark; got %v", *calls)
	}
}

func TestTabRefocusSchedulesNothingWithoutStagedMark(t *testing.T) {
	a, _ := markCaptureOpenThread(t)

	a.Update(HerdrTabViewMsg{Viewed: false})
	if _, cmd := a.Update(HerdrTabViewMsg{Viewed: true}); cmd != nil {
		t.Fatal("refocus with nothing staged must not schedule a flush")
	}
}

// The issued mark comes back as ThreadMarkedLocalMsg, which must clear the
// tracked agent thread's sidebar row with the rest of the local read
// state. Otherwise the herdr row keeps claiming an unread reply the mark
// just declared read.
func TestThreadMarkedLocalClearsTrackedAgentThreadRow(t *testing.T) {
	a, calls, unreads := newAgentTestApp(t)
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
	reduceThreads(a, ThreadMarkedLocalMsg{ChannelID: "C1", ThreadTS: "100.0", TS: "101.0"})
	if len(*calls) <= before {
		t.Fatal("a local thread mark must clear the tracked agent row")
	}
	last := (*calls)[len(*calls)-1]
	if last.working || last.status != "" {
		t.Fatalf("agent row after mark = %+v; want idle with empty status", last)
	}
}
