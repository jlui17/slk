package cache

import (
	"strings"
	"testing"
)

func TestThreadNewestActivity(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	seedMsg := func(ts, threadTS string, deleted bool) {
		t.Helper()
		if err := db.UpsertMessage(Message{
			TS: ts, ChannelID: "C1", WorkspaceID: "T1", UserID: "U1",
			Text: "m", ThreadTS: threadTS, IsDeleted: deleted,
		}); err != nil {
			t.Fatalf("UpsertMessage %s: %v", ts, err)
		}
	}

	// Entirely unknown thread.
	got, err := db.ThreadNewestActivity("C1", "100.000000")
	if err != nil {
		t.Fatalf("ThreadNewestActivity: %v", err)
	}
	if got != "" {
		t.Fatalf("unknown thread: want \"\", got %q", got)
	}

	// Parent row only (cached with empty thread_ts, ts == thread_ts).
	seedMsg("100.000000", "", false)
	if got, _ = db.ThreadNewestActivity("C1", "100.000000"); got != "100.000000" {
		t.Fatalf("parent-only: want parent ts, got %q", got)
	}

	// Cached replies: newest wins.
	seedMsg("101.000000", "100.000000", false)
	seedMsg("102.000000", "100.000000", false)
	if got, _ = db.ThreadNewestActivity("C1", "100.000000"); got != "102.000000" {
		t.Fatalf("cached replies: want newest reply, got %q", got)
	}

	// Deleted newest reply doesn't count.
	seedMsg("103.000000", "100.000000", true)
	if got, _ = db.ThreadNewestActivity("C1", "100.000000"); got != "102.000000" {
		t.Fatalf("deleted reply counted: got %q", got)
	}
}

// A thread's newest-activity lookup must seek the thread, not read the
// channel: a plan through idx_messages_channel costs every cached
// message in the channel per call, which pinned a core per slk once
// busy channels held tens of thousands of messages.
func TestNewestActivityQuerySeeksTheThread(t *testing.T) {
	db := setupDBWithWorkspace(t)
	rows, err := db.conn.Query("EXPLAIN QUERY PLAN "+threadNewestActivityQuery, "C1", "1.0", "1.0")
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatalf("scan: %v", err)
		}
		plan = append(plan, detail)
	}
	rows.Close()
	if strings.Contains(strings.Join(plan, "\n"), "idx_messages_channel (channel_id=?)") {
		t.Errorf("threadNewestActivityQuery reads every cached message in the channel:\n%s", strings.Join(plan, "\n"))
	}
}
