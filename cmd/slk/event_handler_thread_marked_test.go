package main

import (
	"testing"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/ui"
)

// A remote thread_marked can mean "read to the end" or "marked unread",
// and the payload's active flag distinguishes neither (it is subscription
// state — wire captures 2026-08-21 showed active=true on every frame,
// read and unread alike, and no count fields). These tests pin the
// decision rule: compare last_read against the newest activity the cache
// knows. The previous handler trusted a fabricated read bool and rendered
// every remote read as unread.

func TestThreadMarkReadState(t *testing.T) {
	// The captured-ts cases come from a live 3-reply thread (newest
	// reply 1787352903.834189): Slack sets a mark-unread boundary 1µs
	// below the marked message even when that message is the newest,
	// so equality can only be produced by reading to the end.
	cases := []struct {
		name             string
		lastRead, newest string
		read, known      bool
	}{
		{"read to the end", "1787352903.834189", "1787352903.834189", true, true},
		{"mark-unread behind newest", "1787352880.790458", "1787352903.834189", false, true},
		{"mark-unread at the newest reply (1µs decrement)", "1787352903.834188", "1787352903.834189", false, true},
		{"fresh-subscription zero sentinel", "0000000000.000000", "1787352903.834189", false, true},
		{"never read anything", "", "102.0", false, true},
		{"parent-only read", "100.0", "100.0", true, true},
		{"parent-only mark-unread", "099.0", "100.0", false, true},
		{"watermark past everything known", "103.0", "102.0", false, false},
		{"nothing known", "102.0", "", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			read, known := threadMarkReadState(c.lastRead, c.newest)
			if read != c.read || known != c.known {
				t.Fatalf("threadMarkReadState(%q, %q) = (read=%v, known=%v), want (read=%v, known=%v)",
					c.lastRead, c.newest, read, known, c.read, c.known)
			}
		})
	}
}

func seedThread(t *testing.T, db *cache.DB, channelID, threadTS string, replyTSs ...string) {
	t.Helper()
	if err := db.UpsertMessage(cache.Message{
		TS: threadTS, ChannelID: channelID, WorkspaceID: "T1", UserID: "U1", Text: "parent",
	}); err != nil {
		t.Fatalf("UpsertMessage parent: %v", err)
	}
	for _, ts := range replyTSs {
		if err := db.UpsertMessage(cache.Message{
			TS: ts, ChannelID: channelID, WorkspaceID: "T1", UserID: "U2",
			Text: "reply", ThreadTS: threadTS,
		}); err != nil {
			t.Fatalf("UpsertMessage reply %s: %v", ts, err)
		}
	}
}

func TestOnThreadMarked_ReadToEnd_DispatchesRead(t *testing.T) {
	db := newTestDB(t)
	seedThread(t, db, "C1", "100.0", "101.0", "102.0")
	sender := &captureSender{}
	h := &rtmEventHandler{db: db, program: sender, workspaceID: "T1"}

	// Second client read to the newest reply; subscription stays active.
	h.OnThreadMarked("C1", "100.0", "102.0", true)

	msgs := sentOfType[ui.ThreadMarkedRemoteMsg](sender)
	if len(msgs) != 1 {
		t.Fatalf("want 1 ThreadMarkedRemoteMsg, got %d", len(msgs))
	}
	if m := msgs[0]; !m.Read || m.TeamID != "T1" || m.ChannelID != "C1" || m.ThreadTS != "100.0" || m.TS != "102.0" {
		t.Fatalf("read-to-end mis-dispatched: %+v", m)
	}
	subs, _ := db.ListActiveThreadSubscriptions("T1")
	if len(subs) != 1 || subs[0].LastRead != "102.0" {
		t.Fatalf("subscription not persisted: %+v", subs)
	}
}

func TestOnThreadMarked_RemoteMarkUnread_DispatchesUnread(t *testing.T) {
	// The inverse direction is a deliberate user signal and must not be
	// flattened into read: a mark-unread carries last_read strictly
	// behind the message being marked, still with active=true.
	db := newTestDB(t)
	seedThread(t, db, "C1", "100.0", "101.0", "102.0")
	sender := &captureSender{}
	h := &rtmEventHandler{db: db, program: sender, workspaceID: "T1"}

	h.OnThreadMarked("C1", "100.0", "101.0", true)

	msgs := sentOfType[ui.ThreadMarkedRemoteMsg](sender)
	if len(msgs) != 1 {
		t.Fatalf("want 1 ThreadMarkedRemoteMsg, got %d", len(msgs))
	}
	if m := msgs[0]; m.Read || m.TS != "101.0" {
		t.Fatalf("mark-unread mis-dispatched: %+v", m)
	}
}

func TestOnThreadMarked_ParentOnlyThread_BothDirections(t *testing.T) {
	db := newTestDB(t)
	seedThread(t, db, "C1", "100.0") // no replies
	sender := &captureSender{}
	h := &rtmEventHandler{db: db, program: sender, workspaceID: "T1"}

	h.OnThreadMarked("C1", "100.0", "100.0", true) // read the parent
	h.OnThreadMarked("C1", "100.0", "099.0", true) // mark parent unread

	msgs := sentOfType[ui.ThreadMarkedRemoteMsg](sender)
	if len(msgs) != 2 {
		t.Fatalf("want 2 dispatches, got %d", len(msgs))
	}
	if !msgs[0].Read {
		t.Fatalf("read-of-parent should dispatch Read=true: %+v", msgs[0])
	}
	if msgs[1].Read {
		t.Fatalf("mark-unread-of-parent should dispatch Read=false: %+v", msgs[1])
	}
}

func TestOnThreadMarked_SubscribedFlagNeverInfluencesReadDecision(t *testing.T) {
	// The flag is persistence-only: a read-to-end mark on a thread
	// being unsubscribed still dispatches Read=true (and tombstones
	// the row). No wire capture ever showed active=false on a
	// thread_marked, so this pins the design boundary, not a Slack
	// behavior.
	db := newTestDB(t)
	seedThread(t, db, "C1", "100.0", "101.0")
	sender := &captureSender{}
	h := &rtmEventHandler{db: db, program: sender, workspaceID: "T1"}

	h.OnThreadMarked("C1", "100.0", "101.0", false)

	msgs := sentOfType[ui.ThreadMarkedRemoteMsg](sender)
	if len(msgs) != 1 || !msgs[0].Read {
		t.Fatalf("read decision must ignore subscribed: %+v", msgs)
	}
	if subs, _ := db.ListActiveThreadSubscriptions("T1"); len(subs) != 0 {
		t.Fatalf("subscribed=false must still tombstone the row, got %+v", subs)
	}
}

func TestOnThreadMarked_WatermarkQueryError_SkipsDispatch(t *testing.T) {
	// A failed newest-activity lookup is the undecidable case, not a
	// license to guess: no dispatch, same as an unknown watermark.
	db := newTestDB(t)
	seedThread(t, db, "C1", "100.0", "101.0")
	db.Close()
	sender := &captureSender{}
	h := &rtmEventHandler{db: db, program: sender, workspaceID: "T1"}

	h.OnThreadMarked("C1", "100.0", "101.0", true)

	if msgs := sentOfType[ui.ThreadMarkedRemoteMsg](sender); len(msgs) != 0 {
		t.Fatalf("watermark query error must not dispatch, got %d", len(msgs))
	}
}

func TestOnThreadMarked_StaleWatermark_PersistsButSkipsDispatch(t *testing.T) {
	// last_read past everything the cache knows: a read-to-end is
	// indistinguishable from a mark-unread at an uncached reply, so
	// guessing either way recreates half the original bug. Persist the
	// facts, dispatch nothing; the next getView reconcile settles it.
	db := newTestDB(t)
	seedThread(t, db, "C1", "100.0", "101.0")
	sender := &captureSender{}
	h := &rtmEventHandler{db: db, program: sender, workspaceID: "T1"}

	h.OnThreadMarked("C1", "100.0", "105.0", true)

	if msgs := sentOfType[ui.ThreadMarkedRemoteMsg](sender); len(msgs) != 0 {
		t.Fatalf("stale watermark must not dispatch, got %d", len(msgs))
	}
	subs, _ := db.ListActiveThreadSubscriptions("T1")
	if len(subs) != 1 || subs[0].LastRead != "105.0" {
		t.Fatalf("stale-watermark mark must still persist: %+v", subs)
	}
}

func TestOnThreadMarked_UnknownThread_PersistsButSkipsDispatch(t *testing.T) {
	db := newTestDB(t) // no messages, no subscription row
	sender := &captureSender{}
	h := &rtmEventHandler{db: db, program: sender, workspaceID: "T1"}

	h.OnThreadMarked("C1", "100.0", "100.0", true)

	if msgs := sentOfType[ui.ThreadMarkedRemoteMsg](sender); len(msgs) != 0 {
		t.Fatalf("unknown thread must not dispatch, got %d", len(msgs))
	}
	subs, _ := db.ListActiveThreadSubscriptions("T1")
	if len(subs) != 1 {
		t.Fatalf("unknown-thread mark must still persist: %+v", subs)
	}
}

