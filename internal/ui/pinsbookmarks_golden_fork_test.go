package ui

import (
	"testing"

	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/pinsbookmarks"
)

// The pins and bookmarks dialog over the standard golden fixture, and
// the count line it leaves under the channel name when closed.
func TestGolden_PinsBookmarks(t *testing.T) {
	seed := func(a *App) {
		a.setChannelLookupFuncForTest(func(ids.ChannelID) (string, string, bool) { return "general", "channel", true })
		a.pinsBookmarks.lists = map[pinsKey]*channelLists{a.activePinsKey(): {
			pinCount: 3,
			lists:    [2]listRead{{loaded: true}, {loaded: true}},
			pins: []pinsbookmarks.Pin{
				{TS: "1700000000.000100", Username: "maya", Text: "Rollback: run deploy rollback, then check p99"},
				{TS: "1700000500.000300", ThreadTS: "1700000400.000200", Username: "sam", Text: "Root cause: the pool ran dry"},
				{Username: "PagerDuty", FileName: "latency.pdf"},
			},
			bookmarks: []pinsbookmarks.Bookmark{
				{Title: "Runbook", Link: "https://wiki.acme.com/runbook"},
				{Title: "Latency", Link: "https://grafana.acme.com/d/latency", Folder: "Dashboards"},
				{Title: "Status page", Link: "https://status.acme.com"},
			},
		}}
		a.setPinsCountRow(a.activePinsKey())
	}
	for _, d := range []dialogGolden{
		{
			name:  "dialog_pins_bookmarks",
			open:  func(t *testing.T, a *App) { seed(a); a.openPinsBookmarks(pinsbookmarks.Pins) },
			shown: func(a *App) bool { return a.pinsBookmarks.model.IsVisible() },
			want:  "Pins and bookmarks · #general",
		},
		{
			name:  "pins_count_line",
			open:  func(t *testing.T, a *App) { seed(a) },
			shown: func(a *App) bool { return true },
			want:  "📌 3 pins   🔖 3 bookmarks",
		},
	} {
		t.Run(d.name, func(t *testing.T) {
			compareGolden(t, d.name, renderDialogGolden(t, d))
		})
	}
}
