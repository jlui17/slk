package threadsview

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/usernames"
)

// page returns n rows, newest first, whose LastReplyTS count down from
// start.
func page(start, n int) []core.ThreadSummary {
	out := make([]core.ThreadSummary, n)
	for i := range out {
		ts := fmt.Sprintf("%d.000000", start-i)
		out[i] = core.ThreadSummary{ChannelID: "C1", ThreadTS: ts, LastReplyTS: ts}
	}
	return out
}

func threadTSs(m *Model) []string {
	var out []string
	for _, s := range m.Summaries() {
		out = append(out, s.ThreadTS)
	}
	return out
}

func TestNextPage_NearTheEndOnce(t *testing.T) {
	m := New(usernames.FromMap(nil), "USELF")
	m.SetFirstPage(page(100, 8), "93.000000", false)

	if _, ok := m.NextPage(); ok {
		t.Fatal("NextPage at the top of 8 rows; want nothing until near the end")
	}
	m.GoToBottom()
	cursor, ok := m.NextPage()
	if !ok || cursor != "93.000000" {
		t.Fatalf("NextPage = %q, %v; want the page cursor", cursor, ok)
	}
	if _, ok := m.NextPage(); ok {
		t.Error("NextPage asked for the same page twice")
	}
	m.PageFailed("93.000000")
	if cursor, ok := m.NextPage(); !ok || cursor != "93.000000" {
		t.Errorf("after a failure NextPage = %q, %v; want the page asked for again", cursor, ok)
	}
}

// The end of the list on screen loads more even with the selection at
// the top: a tall view shows every loaded row.
func TestNextPage_EndOnScreen(t *testing.T) {
	m := New(usernames.FromMap(nil), "USELF")
	m.SetFirstPage(page(100, 8), "93.000000", false)
	m.View(80, 40)
	if cursor, ok := m.NextPage(); !ok || cursor != "93.000000" {
		t.Errorf("NextPage = %q, %v; want the next page while the list ends on screen", cursor, ok)
	}
}

func TestNextPage_NoMore(t *testing.T) {
	m := New(usernames.FromMap(nil), "USELF")
	m.SetFirstPage(page(100, 3), "", false)
	m.GoToBottom()
	if _, ok := m.NextPage(); ok {
		t.Error("NextPage with no next cursor")
	}
}

func TestAppendPage(t *testing.T) {
	m := New(usernames.FromMap(nil), "USELF")
	m.SetFirstPage(page(100, 3), "98.000000", false)

	m.AppendPage("50.000000", page(50, 2), "")
	if len(m.Summaries()) != 3 {
		t.Fatalf("a page for a stale cursor was appended: %v", threadTSs(&m))
	}
	m.AppendPage("98.000000", page(97, 2), "96.000000")
	if want := []string{"100.000000", "99.000000", "98.000000", "97.000000", "96.000000"}; !slices.Equal(threadTSs(&m), want) {
		t.Errorf("rows = %v, want %v", threadTSs(&m), want)
	}
	m.GoToBottom()
	if cursor, _ := m.NextPage(); cursor != "96.000000" {
		t.Errorf("next cursor = %q, want the appended page's", cursor)
	}
}

// A refresh while the user has scrolled keeps the older rows below the
// new first page; a row that moved up is taken from the page.
func TestSetFirstPage_KeepOlder(t *testing.T) {
	m := New(usernames.FromMap(nil), "USELF")
	m.SetFirstPage(page(100, 3), "98.000000", false)
	m.AppendPage("98.000000", page(97, 3), "95.000000")

	// 96 got a reply and moved to the top; the page now ends at 99.
	fresh := []core.ThreadSummary{
		{ChannelID: "C1", ThreadTS: "96.000000", LastReplyTS: "200.000000"},
		{ChannelID: "C1", ThreadTS: "100.000000", LastReplyTS: "100.000000"},
		{ChannelID: "C1", ThreadTS: "99.000000", LastReplyTS: "99.000000"},
	}
	m.SetFirstPage(fresh, "99.000000", true)
	if want := []string{"96.000000", "100.000000", "99.000000", "98.000000", "97.000000", "95.000000"}; !slices.Equal(threadTSs(&m), want) {
		t.Errorf("rows = %v, want %v", threadTSs(&m), want)
	}
	m.GoToBottom()
	if cursor, _ := m.NextPage(); cursor != "95.000000" {
		t.Errorf("next cursor = %q, want the old end of the list", cursor)
	}

	// An open of the view replaces the list with its first page.
	m.SetFirstPage(fresh, "99.000000", false)
	if len(m.Summaries()) != 3 {
		t.Errorf("rows = %v, want the page alone", threadTSs(&m))
	}
}

func TestApplyReply(t *testing.T) {
	m := New(usernames.FromMap(nil), "USELF")
	m.SetSummaries([]core.ThreadSummary{
		{ChannelID: "C1", ThreadTS: "5.0", LastReplyTS: "6.0", ReplyCount: 1},
		{ChannelID: "C1", ThreadTS: "1.0", LastReplyTS: "2.0", ReplyCount: 1},
	})
	m.MoveDown() // select thread 1.0

	if !m.ApplyReply("C1", "1.0", "7.0", "U2", true) {
		t.Fatal("ApplyReply on a loaded row returned false")
	}
	got := m.Summaries()[0]
	if got.ThreadTS != "1.0" || got.LastReplyTS != "7.0" || got.LastReplyBy != "U2" || got.ReplyCount != 2 || !got.Unread {
		t.Errorf("top row = %+v", got)
	}
	if _, ts, _ := m.Selected(); ts != "1.0" {
		t.Errorf("selection = %s, want it to follow its row", ts)
	}
	if m.ApplyReply("C1", "1.0", "7.0", "U2", true) {
		t.Error("a reply already applied changed the row again")
	}
	if m.ApplyReply("C1", "9.0", "8.0", "U2", true) {
		t.Error("a thread that is not loaded changed the list")
	}
}

func TestView_LoadingFirstPage(t *testing.T) {
	m := New(usernames.FromMap(nil), "USELF")
	m.SetLoading(true)
	if out := m.View(5, 40); !strings.Contains(out, "loading threads") {
		t.Errorf("view = %q, want the loading text", out)
	}
	m.SetFirstPage(nil, "", false)
	if out := m.View(5, 40); !strings.Contains(out, "no threads") {
		t.Errorf("view = %q, want the empty text once the page landed", out)
	}
}
