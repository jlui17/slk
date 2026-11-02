package cache

import (
	"fmt"
	"strings"
	"testing"
)

// TestUnreadChannelsSeeksUnreadRows pins UnreadChannels to the partial
// index over unread rows: it reruns on every read-state change, and a
// scan reads every channel of every workspace.
func TestUnreadChannelsSeeksUnreadRows(t *testing.T) {
	db := setupDBWithWorkspace(t)
	rows, err := db.conn.Query(`EXPLAIN QUERY PLAN SELECT workspace_id, id, last_read_ts, has_unread, mention_count
		 FROM channels WHERE has_unread = 1
		 ORDER BY workspace_id, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if got := strings.Join(plan, "\n"); got != "SCAN channels USING INDEX idx_channels_unread" {
		t.Errorf("UnreadChannels plan:\n%s", got)
	}
}

// BenchmarkUnreadChannels is the workspace rail's unread read, which
// reruns on every read-state change in any workspace: a channels table
// the size of a large workspace's conversation list, a handful unread.
func BenchmarkUnreadChannels(b *testing.B) {
	db, err := New(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	const workspaces, channels, unread = 3, 4000, 20
	for w := range workspaces {
		ws := fmt.Sprintf("T%d", w)
		if err := db.UpsertWorkspace(Workspace{ID: ws, Name: ws}); err != nil {
			b.Fatal(err)
		}
		for c := range channels {
			id := fmt.Sprintf("C%d_%d", w, c)
			if err := db.UpsertChannel(Channel{ID: id, WorkspaceID: ws, Name: id, Type: "channel"}); err != nil {
				b.Fatal(err)
			}
			if c%(channels/unread) == 0 {
				if err := db.UpdateChannelReadState(id, "", true); err != nil {
					b.Fatal(err)
				}
			}
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		got, err := db.UnreadChannels()
		if err != nil || len(got) != workspaces*unread {
			b.Fatalf("got %d rows, err %v", len(got), err)
		}
	}
}
