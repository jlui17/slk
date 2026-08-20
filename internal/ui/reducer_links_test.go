package ui

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
)

func linkTestApp(t *testing.T) (*App, *string) {
	t.Helper()
	app := NewApp()
	app.activeTeamID = "T1"
	app.workspaceDomains["T1"] = "myteam"
	var opened string
	app.browserOpener = func(url string) tea.Cmd {
		opened = url
		return nil
	}
	app.setChannelLookupFuncForTest(func(channelID ids.ChannelID) (string, string, bool) {
		if channelID == "C054JFCBN69" {
			return "general", "channel", true
		}
		return "", "", false
	})
	return app, &opened
}

func drainCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	var out []tea.Msg
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			out = append(out, drainCmd(c)...)
		}
		return out
	}
	if msg != nil {
		out = append(out, msg)
	}
	return out
}

func TestOpenLink_NonSlackURL_OpensBrowser(t *testing.T) {
	app, opened := linkTestApp(t)
	_, cmd := app.Update(OpenLinkMsg{URL: "https://github.com/foo/bar"})
	drainCmd(cmd)
	if *opened != "https://github.com/foo/bar" {
		t.Errorf("browser opened %q", *opened)
	}
}

func TestOpenLink_ForeignWorkspace_OpensBrowser(t *testing.T) {
	app, opened := linkTestApp(t)
	url := "https://otherteam.slack.com/archives/C054JFCBN69/p1779284733270139"
	_, cmd := app.Update(OpenLinkMsg{URL: url})
	drainCmd(cmd)
	if *opened != url {
		t.Errorf("browser opened %q, want %q", *opened, url)
	}
}

func TestOpenLink_UnknownChannel_OpensBrowser(t *testing.T) {
	app, opened := linkTestApp(t)
	url := "https://myteam.slack.com/archives/CUNKNOWN1/p1779284733270139"
	_, cmd := app.Update(OpenLinkMsg{URL: url})
	drainCmd(cmd)
	if *opened != url {
		t.Errorf("browser opened %q, want %q", *opened, url)
	}
}

func TestOpenLink_OtherChannel_DispatchesChannelSelected(t *testing.T) {
	app, opened := linkTestApp(t)
	app.activeChannelID = "CELSEWHERE"
	_, cmd := app.Update(OpenLinkMsg{URL: "https://myteam.slack.com/archives/C054JFCBN69/p1779284733270139"})
	msgs := drainCmd(cmd)
	var sel *ChannelSelectedMsg
	for _, m := range msgs {
		if cs, ok := m.(ChannelSelectedMsg); ok {
			sel = &cs
		}
	}
	if sel == nil {
		t.Fatalf("no ChannelSelectedMsg in %#v", msgs)
	}
	if sel.ID != "C054JFCBN69" || sel.Name != "general" || sel.Type != "channel" {
		t.Errorf("ChannelSelectedMsg = %+v", sel)
	}
	if app.pendingLinkNav == nil || app.pendingLinkNav.messageTS != "1779284733.270139" {
		t.Errorf("pendingLinkNav = %+v", app.pendingLinkNav)
	}
	if *opened != "" {
		t.Errorf("browser should not open, got %q", *opened)
	}
}

func TestOpenLink_ActiveChannel_SelectsMessage(t *testing.T) {
	app, _ := linkTestApp(t)
	app.activeChannelID = "C054JFCBN69"
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1779284733.270139", Text: "target"},
		{TS: "1779284734.000000", Text: "newer"},
	})
	_, cmd := app.Update(OpenLinkMsg{URL: "https://myteam.slack.com/archives/C054JFCBN69/p1779284733270139"})
	drainCmd(cmd)
	sel, ok := app.messagepane.SelectedMessage()
	if !ok || sel.TS != "1779284733.270139" {
		t.Errorf("selected = %+v ok=%v", sel, ok)
	}
	if app.pendingLinkNav != nil {
		t.Errorf("pendingLinkNav not cleared: %+v", app.pendingLinkNav)
	}
}

// Pressing o in an open thread panel on a permalink to a channel-level
// message of the SAME channel must land somewhere visible: the select
// happens in the messages pane, so the thread panel closes and focus
// moves there. Without this the cursor moved behind the panel and the
// key press looked like a no-op.
func TestOpenLink_ActiveChannel_FromThreadPanel_ClosesThreadAndSelects(t *testing.T) {
	app, _ := linkTestApp(t)
	app.activeChannelID = "C054JFCBN69"
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1779284733.270139", Text: "target"},
		{TS: "1779284734.000000", Text: "reply with the link"},
	})
	app.threadVisible = true
	app.focusedPanel = PanelThread
	_, cmd := app.Update(OpenLinkMsg{URL: "https://myteam.slack.com/archives/C054JFCBN69/p1779284733270139"})
	drainCmd(cmd)
	if app.threadVisible {
		t.Error("thread panel still open")
	}
	if app.focusedPanel != PanelMessages {
		t.Errorf("focusedPanel = %v, want PanelMessages", app.focusedPanel)
	}
	sel, ok := app.messagepane.SelectedMessage()
	if !ok || sel.TS != "1779284733.270139" {
		t.Errorf("selected = %+v ok=%v", sel, ok)
	}
}

// A thread parent's permalink carries no thread_ts; following it must
// open the thread, not stop at selecting the parent in-channel. This
// is the in-buffer case (the parent is already in the pane).
func TestOpenLink_ActiveChannel_ParentWithReplies_OpensThread(t *testing.T) {
	app, _ := linkTestApp(t)
	app.activeChannelID = "C054JFCBN69"
	var fetchedChannel, fetchedThread string
	app.setThreadFetcherForTest(func(channelID ids.ChannelID, threadTS ids.ThreadTS) tea.Msg {
		fetchedChannel, fetchedThread = string(channelID), string(threadTS)
		return nil
	})
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1779284733.270139", Text: "parent", ThreadTS: "1779284733.270139", ReplyCount: 3},
		{TS: "1779284734.000000", Text: "message with the link"},
	})
	// The user's own repro: reading some other thread when pressing o.
	app.threadVisible = true
	app.focusedPanel = PanelThread
	_, cmd := app.Update(OpenLinkMsg{URL: "https://myteam.slack.com/archives/C054JFCBN69/p1779284733270139"})
	drainCmd(cmd)
	if !app.threadVisible {
		t.Fatal("thread panel not visible")
	}
	if got := app.threadPanel.ThreadTS(); got != "1779284733.270139" {
		t.Errorf("ThreadTS = %q, want the parent's ts", got)
	}
	if fetchedChannel != "C054JFCBN69" || fetchedThread != "1779284733.270139" {
		t.Errorf("thread fetch = (%q, %q)", fetchedChannel, fetchedThread)
	}
	// The channel cursor lands on the parent so closing the thread
	// leaves the user at the linked message.
	sel, ok := app.messagepane.SelectedMessage()
	if !ok || sel.TS != "1779284733.270139" {
		t.Errorf("channel selection = %+v ok=%v", sel, ok)
	}
	if app.pendingLinkNav != nil {
		t.Errorf("pendingLinkNav not cleared: %+v", app.pendingLinkNav)
	}
}

// The off-buffer variant: the parent arrives via FetchAround, so the
// MessagesAroundLoadedMsg arm makes the thread-open call.
func TestMessagesAroundLoaded_ArmedNavParent_OpensThread(t *testing.T) {
	app, _ := linkTestApp(t)
	app.activeChannelID = "C054JFCBN69"
	var fetchedThread string
	app.setThreadFetcherForTest(func(channelID ids.ChannelID, threadTS ids.ThreadTS) tea.Msg {
		fetchedThread = string(threadTS)
		return nil
	})
	app.pendingLinkNav = &pendingLinkNav{channelID: "C054JFCBN69", messageTS: "1779284733.270139"}
	_, cmd := app.Update(MessagesAroundLoadedMsg{
		ChannelID: "C054JFCBN69",
		TargetTS:  "1779284733.270139",
		Messages: []messages.MessageItem{
			{TS: "1779284733.270139", Text: "parent", ThreadTS: "1779284733.270139", ReplyCount: 2},
			{TS: "1779284734.000000", Text: "newer"},
		},
	})
	drainCmd(cmd)
	if !app.threadVisible {
		t.Fatal("thread panel not visible")
	}
	if got := app.threadPanel.ThreadTS(); got != "1779284733.270139" {
		t.Errorf("ThreadTS = %q", got)
	}
	if fetchedThread != "1779284733.270139" {
		t.Errorf("thread fetch = %q", fetchedThread)
	}
	if app.pendingLinkNav != nil {
		t.Errorf("pendingLinkNav not cleared: %+v", app.pendingLinkNav)
	}
}

// The upload guard refuses the channel switch, so the nav it was
// carrying must die with it: the completion hook otherwise runs
// against the OLD channel's UI (closing the user's thread, dispatching
// FetchAround for a channel that never became active) and leaks the
// armed nav.
func TestOpenLink_UploadGuard_DropsNavAndKeepsThread(t *testing.T) {
	app, _ := linkTestApp(t)
	app.activeChannelID = "CELSEWHERE"
	app.compose.SetUploading(true)
	app.threadVisible = true
	app.focusedPanel = PanelThread
	var fetchedAround bool
	setChannelFetchAroundForTest(app, func(channelID ids.ChannelID, ts ids.MessageTS) tea.Msg {
		fetchedAround = true
		return nil
	})
	_, cmd := app.Update(OpenLinkMsg{URL: "https://myteam.slack.com/archives/C054JFCBN69/p1779284733270139"})
	for _, m := range drainCmd(cmd) {
		if cs, ok := m.(ChannelSelectedMsg); ok {
			_, c2 := app.Update(cs)
			drainCmd(c2)
		}
	}
	if !app.threadVisible || app.focusedPanel != PanelThread {
		t.Error("refused switch tore down the thread panel or moved focus")
	}
	if fetchedAround {
		t.Error("FetchAround dispatched for a channel that never became active")
	}
	if app.pendingLinkNav != nil {
		t.Errorf("pendingLinkNav leaked past the refused switch: %+v", app.pendingLinkNav)
	}
}

// Without an armed nav the arm keeps its plain select behavior — an
// in-channel search jump to a thread parent must NOT open the thread.
func TestMessagesAroundLoaded_NoNav_ParentStaysSelected(t *testing.T) {
	app := NewApp()
	app.activeChannelID = "C1"
	_, cmd := app.Update(MessagesAroundLoadedMsg{
		ChannelID: "C1",
		TargetTS:  "2.0",
		Messages:  []messages.MessageItem{{TS: "2.0", Text: "parent", ReplyCount: 5}},
	})
	drainCmd(cmd)
	if app.threadVisible {
		t.Fatal("thread panel opened on a non-permalink jump")
	}
	sel, ok := app.messagepane.SelectedMessage()
	if !ok || sel.TS != "2.0" {
		t.Errorf("selected = %+v ok=%v", sel, ok)
	}
}

// A ctrl+w focus change during the FetchAround round-trip retargets
// activeChannelID with no ChannelSelectedMsg, so nothing else drops
// the armed nav; the stale-window drop must retire it, or the next
// visit to the channel replays the jump.
func TestMessagesAroundLoaded_StaleWindowRetiresArmedNav(t *testing.T) {
	app, _ := linkTestApp(t)
	app.activeChannelID = "COTHER" // focus moved during the fetch
	app.pendingLinkNav = &pendingLinkNav{channelID: "C054JFCBN69", messageTS: "1.0"}
	_, cmd := app.Update(MessagesAroundLoadedMsg{
		ChannelID: "C054JFCBN69",
		TargetTS:  "1.0",
		Messages:  []messages.MessageItem{{TS: "1.0", Text: "target"}},
	})
	drainCmd(cmd)
	if app.pendingLinkNav != nil {
		t.Errorf("pendingLinkNav leaked past the stale drop: %+v", app.pendingLinkNav)
	}
}

// A failed window fetch must still retire the armed nav, or a later
// visit to the channel would replay the stale jump.
func TestMessagesAroundLoaded_FailureRetiresArmedNav(t *testing.T) {
	app, _ := linkTestApp(t)
	app.activeChannelID = "C054JFCBN69"
	app.pendingLinkNav = &pendingLinkNav{channelID: "C054JFCBN69", messageTS: "1.0"}
	_, cmd := app.Update(MessagesAroundLoadedMsg{ChannelID: "C054JFCBN69", TargetTS: "1.0", Err: errors.New("boom")})
	drainCmd(cmd)
	if app.pendingLinkNav != nil {
		t.Errorf("pendingLinkNav leaked: %+v", app.pendingLinkNav)
	}
}

func TestOpenLink_ActiveChannel_TSNotLoaded_FetchesAround(t *testing.T) {
	app, _ := linkTestApp(t)
	var fetchedChannel, fetchedTS string
	setChannelFetchAroundForTest(app, func(channelID ids.ChannelID, ts ids.MessageTS) tea.Msg {
		fetchedChannel, fetchedTS = string(channelID), string(ts)
		return nil
	})
	app.activeChannelID = "C054JFCBN69"
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1779284734.000000", Text: "only newer"},
	})
	_, cmd := app.Update(OpenLinkMsg{URL: "https://myteam.slack.com/archives/C054JFCBN69/p1779284733270139"})
	drainCmd(cmd)
	if fetchedChannel != "C054JFCBN69" || fetchedTS != "1779284733.270139" {
		t.Errorf("FetchAround not dispatched: ch=%q ts=%q", fetchedChannel, fetchedTS)
	}
	// The nav rides through FetchAround still armed: whether the target
	// is a thread parent is unknowable until the window lands, so the
	// MessagesAroundLoadedMsg arm retires it.
	if app.pendingLinkNav == nil {
		t.Error("pendingLinkNav should stay armed until MessagesAroundLoadedMsg")
	}
}

func TestOpenLink_ThreadPermalink_OpensThread(t *testing.T) {
	app, _ := linkTestApp(t)
	app.activeChannelID = "C054JFCBN69"
	var fetchedChannel, fetchedThread string
	app.setThreadFetcherForTest(func(channelID ids.ChannelID, threadTS ids.ThreadTS) tea.Msg {
		fetchedChannel, fetchedThread = string(channelID), string(threadTS)
		return nil
	})
	_, cmd := app.Update(OpenLinkMsg{URL: "https://myteam.slack.com/archives/C054JFCBN69/p1779284733270139?thread_ts=1779284700.000100"})
	drainCmd(cmd)
	if !app.threadVisible {
		t.Fatal("thread panel not visible")
	}
	if got := app.threadPanel.ThreadTS(); got != "1779284700.000100" {
		t.Errorf("ThreadTS = %q", got)
	}
	if fetchedChannel != "C054JFCBN69" || fetchedThread != "1779284700.000100" {
		t.Errorf("fetch = (%q, %q)", fetchedChannel, fetchedThread)
	}
}

func TestMessagesLoaded_CompletesPendingNav(t *testing.T) {
	app, _ := linkTestApp(t)
	app.activeChannelID = "C054JFCBN69"
	app.pendingLinkNav = &pendingLinkNav{
		channelID: "C054JFCBN69",
		messageTS: "1779284733.270139",
	}
	_, cmd := app.Update(MessagesLoadedMsg{
		ChannelID: "C054JFCBN69",
		Messages: []messages.MessageItem{
			{TS: "1779284733.270139", Text: "target"},
			{TS: "1779284734.000000", Text: "newer"},
		},
	})
	drainCmd(cmd)
	sel, ok := app.messagepane.SelectedMessage()
	if !ok || sel.TS != "1779284733.270139" {
		t.Errorf("selected = %+v ok=%v", sel, ok)
	}
	if app.pendingLinkNav != nil {
		t.Errorf("pendingLinkNav not cleared: %+v", app.pendingLinkNav)
	}
}

func TestOpenLink_OtherChannel_FreshCacheMissingTS_FetchesAround(t *testing.T) {
	app, _ := linkTestApp(t)
	app.activeChannelID = "CELSEWHERE"
	// Wire C054JFCBN69 as a tier-1 "fresh" channel (synced just now, so
	// reduceChannelSelected renders cache and fires NO fetch) whose
	// cached buffer does NOT contain the permalink's target ts.
	app.setChannelCacheReaderForTest(func(channelID ids.ChannelID) []messages.MessageItem {
		if channelID == "C054JFCBN69" {
			return []messages.MessageItem{{TS: "1779284734.000000", Text: "newer only"}}
		}
		return nil
	})
	app.setChannelSyncedAtReaderForTest(func(channelID ids.ChannelID) int64 {
		return time.Now().Unix()
	})
	var fetchedChannel, fetchedTS string
	setChannelFetchAroundForTest(app, func(channelID ids.ChannelID, ts ids.MessageTS) tea.Msg {
		fetchedChannel, fetchedTS = string(channelID), string(ts)
		return nil
	})

	_, cmd := app.Update(OpenLinkMsg{URL: "https://myteam.slack.com/archives/C054JFCBN69/p1779284733270139"})
	// routeLink dispatched a ChannelSelectedMsg; feed it back through Update
	// (as the real program loop would) so the tier-1 fresh-cache path
	// completes the pending nav authoritatively.
	for _, m := range drainCmd(cmd) {
		if cs, ok := m.(ChannelSelectedMsg); ok {
			_, c2 := app.Update(cs)
			drainCmd(c2)
		}
	}
	if fetchedChannel != "C054JFCBN69" || fetchedTS != "1779284733.270139" {
		t.Errorf("FetchAround not dispatched on tier-1 fresh path: ch=%q ts=%q", fetchedChannel, fetchedTS)
	}
	// Armed across FetchAround; the MessagesAroundLoadedMsg arm retires it.
	if app.pendingLinkNav == nil {
		t.Error("pendingLinkNav should stay armed until MessagesAroundLoadedMsg")
	}
	_, c3 := app.Update(MessagesAroundLoadedMsg{
		ChannelID: "C054JFCBN69",
		TargetTS:  "1779284733.270139",
		Messages:  []messages.MessageItem{{TS: "1779284733.270139", Text: "target"}},
	})
	drainCmd(c3)
	if app.pendingLinkNav != nil {
		t.Errorf("pendingLinkNav leaked after window landed: %+v", app.pendingLinkNav)
	}
}

func TestChannelSelected_DifferentChannel_DropsPendingNav(t *testing.T) {
	app, _ := linkTestApp(t)
	app.pendingLinkNav = &pendingLinkNav{channelID: "C054JFCBN69", messageTS: "1.0"}
	_, cmd := app.Update(ChannelSelectedMsg{ID: "COTHER", Name: "other", Type: "channel"})
	drainCmd(cmd)
	if app.pendingLinkNav != nil {
		t.Errorf("pendingLinkNav should be dropped on unrelated navigation: %+v", app.pendingLinkNav)
	}
}

func TestMessagesAroundLoaded_ReplacesBufferAndSelects(t *testing.T) {
	app := NewApp()
	app.activeChannelID = "C1"
	app.Update(MessagesAroundLoadedMsg{
		ChannelID: "C1",
		TargetTS:  "1700000004.000000",
		Messages: []messages.MessageItem{
			{TS: "1700000003.000000", Text: "a"},
			{TS: "1700000004.000000", Text: "b"},
			{TS: "1700000005.000000", Text: "c"},
		},
	})
	sel, ok := app.messagepane.SelectedMessage()
	if !ok || sel.TS != "1700000004.000000" {
		t.Fatalf("selected %v ok=%v, want target ts", sel.TS, ok)
	}
}

// A failed jump must be non-destructive: if the fetched window doesn't
// contain the target, the current buffer (and position) stays intact —
// per the spec's error table — and the user just gets a toast.
func TestMessagesAroundLoaded_TargetMissingKeepsBuffer(t *testing.T) {
	app := NewApp()
	app.activeChannelID = "C1"
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: "keep"}})
	_, cmd := app.Update(MessagesAroundLoadedMsg{
		ChannelID: "C1",
		TargetTS:  "9.0",
		Messages:  []messages.MessageItem{{TS: "2.0", Text: "window"}},
	})
	sel, ok := app.messagepane.SelectedMessage()
	if !ok || sel.Text != "keep" {
		t.Fatalf("buffer replaced on failed jump: sel=%+v ok=%v", sel, ok)
	}
	var toast string
	for _, m := range drainCmd(cmd) {
		if tm, ok := m.(ToastMsg); ok {
			toast = tm.Text
		}
	}
	if toast != "Message not found in loaded history" {
		t.Fatalf("toast = %q", toast)
	}
}

func TestMessagesAroundLoaded_ErrorToasts(t *testing.T) {
	app := NewApp()
	app.activeChannelID = "C1"
	_, cmd := app.Update(MessagesAroundLoadedMsg{ChannelID: "C1", TargetTS: "1", Err: errors.New("boom")})
	msgs := drainCmd(cmd)
	found := false
	for _, m := range msgs {
		if _, ok := m.(ToastMsg); ok {
			found = true
		}
	}
	if !found {
		t.Fatal("expected ToastMsg on fetch failure")
	}
}

func TestMessagesAroundLoaded_StaleChannelDropped(t *testing.T) {
	app := NewApp()
	app.activeChannelID = "C2"
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: "keep"}})
	app.Update(MessagesAroundLoadedMsg{
		ChannelID: "C1",
		TargetTS:  "2.0",
		Messages:  []messages.MessageItem{{TS: "2.0", Text: "stale"}},
	})
	sel, _ := app.messagepane.SelectedMessage()
	if sel.Text != "keep" {
		t.Fatal("stale MessagesAroundLoadedMsg replaced active channel buffer")
	}
}

// Permalink upgrade: target outside the buffer now triggers FetchAround
// instead of the "older than loaded history" toast.
func TestCompletePendingNav_OffBufferTriggersFetchAround(t *testing.T) {
	app, _ := linkTestApp(t)
	var fetchedChannel, fetchedTS string
	setChannelFetchAroundForTest(app, func(channelID ids.ChannelID, ts ids.MessageTS) tea.Msg {
		fetchedChannel, fetchedTS = string(channelID), string(ts)
		return nil
	})
	app.activeChannelID = "C054JFCBN69"
	app.pendingLinkNav = &pendingLinkNav{channelID: "C054JFCBN69", messageTS: "1700000001.000000"}

	_, cmd := app.Update(MessagesLoadedMsg{ChannelID: "C054JFCBN69", Messages: []messages.MessageItem{{TS: "1700000099.000000"}}})
	drainCmd(cmd)

	if fetchedChannel != "C054JFCBN69" || fetchedTS != "1700000001.000000" {
		t.Fatalf("FetchAround not dispatched: ch=%q ts=%q", fetchedChannel, fetchedTS)
	}
	if app.pendingLinkNav == nil {
		t.Fatal("pendingLinkNav should stay armed until MessagesAroundLoadedMsg")
	}
}
