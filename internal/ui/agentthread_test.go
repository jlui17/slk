package ui

import (
	"testing"

	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
)

type agentReportCall struct {
	agent       string
	displayName string
	title       string
	working     bool
	status      string
}

// newAgentTestApp wires an App with recording agent callbacks and a fixed
// user cache: UBOT is a bot ("Claude"), UHUMAN is not, everything else is
// uncached.
func newAgentTestApp() (*App, *[]agentReportCall, *int) {
	a, calls, releases, _ := newAgentTestAppWithTab()
	return a, calls, releases
}

func newAgentTestAppWithTab() (*App, *[]agentReportCall, *int, *[]string) {
	a := NewApp()
	calls := &[]agentReportCall{}
	releases := new(int)
	tabNames := &[]string{}
	a.SetAgentReporter(
		func(agent, displayName, title string, working bool, statusMessage string) {
			*calls = append(*calls, agentReportCall{agent, displayName, title, working, statusMessage})
		},
		func() { *releases++ },
		func(label string) { *tabNames = append(*tabNames, label) },
		func(userID string) (string, bool, bool) {
			switch userID {
			case "UBOT":
				return "Claude", true, true
			case "UHUMAN":
				return "justin", false, true
			}
			return "", false, false
		},
	)
	a.channelNames = map[string]string{"C1": "z-claude-dreams"}
	return a, calls, releases, tabNames
}

func openAgentThread(a *App, text string) {
	parent := messages.MessageItem{TS: "100.0", Text: text, UserID: "UHUMAN"}
	a.threadPanel.SetThread(parent, nil, "C1", "100.0")
	a.threadVisible = true
	a.updateAgentThread(parent, "C1", "100.0")
}

func TestAgentThreadDetectedFromBotMention(t *testing.T) {
	a, calls, _ := newAgentTestApp()
	openAgentThread(a, "<@UBOT> please fix the ingest retries")

	if len(*calls) != 1 {
		t.Fatalf("want 1 report, got %+v", *calls)
	}
	c := (*calls)[0]
	if c.agent != "slack-claude" || c.displayName != "Claude" || c.working {
		t.Errorf("unexpected report: %+v", c)
	}
	if want := "#z-claude-dreams @Claude please fix the ingest retries"; c.title != want {
		t.Errorf("title = %q, want %q", c.title, want)
	}
}

func TestAgentThreadDetectedFromBotAuthor(t *testing.T) {
	a, calls, _, tabNames := newAgentTestAppWithTab()
	parent := messages.MessageItem{TS: "100.0", Text: "kicking off the ingest retry fix", UserID: "UBOT"}
	a.threadPanel.SetThread(parent, nil, "C1", "100.0")
	a.threadVisible = true
	a.updateAgentThread(parent, "C1", "100.0")

	if len(*calls) != 1 {
		t.Fatalf("want 1 report, got %+v", *calls)
	}
	c := (*calls)[0]
	if c.agent != "slack-claude" || c.displayName != "Claude" || c.working {
		t.Errorf("unexpected report: %+v", c)
	}
	if want := "#z-claude-dreams kicking off the ingest retry fix"; c.title != want {
		t.Errorf("title = %q, want %q", c.title, want)
	}
	if len(*tabNames) != 1 || (*tabNames)[0] != "kicking off the ingest retry …" {
		t.Errorf("tab names = %+v", *tabNames)
	}
}

func TestAgentThreadNamesTab(t *testing.T) {
	a, _, _, tabNames := newAgentTestAppWithTab()
	openAgentThread(a, "<@UBOT> please fix the ingest retries in colony")

	if len(*tabNames) != 1 {
		t.Fatalf("want 1 tab name, got %+v", *tabNames)
	}
	// The leading agent mention is dropped and the label truncated to 30
	// cells; closing the thread does not rename back.
	if want := "please fix the ingest retries…"; (*tabNames)[0] != want {
		t.Errorf("tab name = %q, want %q", (*tabNames)[0], want)
	}
	a.CloseThread()
	if len(*tabNames) != 1 {
		t.Errorf("close must not rename the tab; got %+v", *tabNames)
	}
}

func TestAgentTabLabelHoistsTaskID(t *testing.T) {
	cases := []struct{ flat, want string }{
		{"colony-562: fix the flow viewer", "[colony-562] fix the flow viewer"},
		{"please babysit colony-71 until merge", "[colony-71] please babysit until merge"},
		{"colony-562", "[colony-562]"},
		{"fix the ingest retries", "fix the ingest retries"},
	}
	for _, c := range cases {
		if got := agentTabLabel(c.flat); got != c.want {
			t.Errorf("agentTabLabel(%q) = %q, want %q", c.flat, got, c.want)
		}
	}
}

func TestAgentTabNameIndependentOfNameSources(t *testing.T) {
	a, _, _, tabNames := newAgentTestAppWithTab()
	// The in-memory name map and the user cache disagree on the bot's
	// name; the tab label must not carry a mangled fragment of either.
	a.userNames = map[string]string{"UBOT": "Claude Tag"}
	openAgentThread(a, "<@UBOT> fix the retries")

	if len(*tabNames) != 1 || (*tabNames)[0] != "fix the retries" {
		t.Errorf("tab names = %+v, want [fix the retries]", *tabNames)
	}
}

func TestAgentThreadTitleFlattensMrkdwn(t *testing.T) {
	a, calls, _ := newAgentTestApp()
	openAgentThread(a, "<@UBOT> look at <#C1> &amp; <https://x.test|the link>")

	if len(*calls) != 1 {
		t.Fatalf("want 1 report, got %+v", *calls)
	}
	if want := "#z-claude-dreams @Claude look at #z-claude-dreams & the link"; (*calls)[0].title != want {
		t.Errorf("title = %q, want %q", (*calls)[0].title, want)
	}
}

func TestAgentThreadDetectedAfterPermalinkBackfill(t *testing.T) {
	a, calls, _ := newAgentTestApp()
	root := messages.MessageItem{TS: "100.0", Text: "<@UBOT> hello", UserID: "UHUMAN", ThreadTS: "100.0"}
	a.SetThreadService(NewThreadService(ThreadServiceFuncs{
		CacheRead: func(ids.ChannelID, ids.ThreadTS) []messages.MessageItem {
			return []messages.MessageItem{root}
		},
	}))

	// Permalink-style open: the parent is a stub with no text yet, so
	// detection finds nothing at open time.
	stub := messages.MessageItem{TS: "100.0"}
	a.threadPanel.SetThread(stub, nil, "C1", "100.0")
	a.threadVisible = true
	a.updateAgentThread(stub, "C1", "100.0")
	if len(*calls) != 0 {
		t.Fatalf("stub parent must not detect; calls=%+v", *calls)
	}

	// The fetch lands, the reducer backfills the real parent from cache,
	// and detection re-runs against it.
	reduceThreads(a, ThreadRepliesLoadedMsg{ThreadTS: "100.0", Replies: []messages.MessageItem{}})
	if len(*calls) != 1 || (*calls)[0].agent != "slack-claude" {
		t.Fatalf("backfill must re-run detection; calls=%+v", *calls)
	}
}

func TestAgentThreadReleasedOnAutoHide(t *testing.T) {
	a, _, releases := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")

	// Narrow enough that layout.Compute cannot fit the thread pane, so
	// View() auto-hides it — an effective close.
	a.width, a.height = 40, 20
	_ = a.View()
	if a.threadVisible {
		t.Fatal("precondition: thread should auto-hide at width 40")
	}
	if *releases != 1 {
		t.Fatalf("auto-hide should release the agent entry, got %d", *releases)
	}
}

func TestAgentThreadReplyReloadDoesNotStompWorkingState(t *testing.T) {
	a, calls, _ := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: "is thinking…"})
	*calls = nil

	// A replies reload re-enters setThreadPanel with the same parent;
	// unchanged detection must not re-report and reset the sidebar to idle.
	reduceThreads(a, ThreadRepliesLoadedMsg{ThreadTS: "100.0", Replies: []messages.MessageItem{{TS: "101.0", ThreadTS: "100.0", Text: "reply"}}})
	if len(*calls) != 0 {
		t.Fatalf("unchanged detection must not re-report; calls=%+v", *calls)
	}
}

func TestAgentThreadIgnoresHumanAndUncachedMentions(t *testing.T) {
	a, calls, releases := newAgentTestApp()
	openAgentThread(a, "<@UHUMAN> and <@USTRANGER> chatting")

	if len(*calls) != 0 || *releases != 0 {
		t.Errorf("non-bot thread must not report or release; calls=%+v releases=%d", *calls, *releases)
	}
}

func TestAgentThreadReleasedOnCloseAndReplacement(t *testing.T) {
	a, _, releases := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")

	// Opening a non-agent thread replaces the entry.
	openAgentThread(a, "no mentions here")
	if *releases != 1 {
		t.Fatalf("replacing with a non-agent thread should release once, got %d", *releases)
	}

	openAgentThread(a, "<@UBOT> hi again")
	a.CloseThread()
	if *releases != 2 {
		t.Fatalf("CloseThread should release, got %d", *releases)
	}
	a.CloseThread()
	if *releases != 2 {
		t.Errorf("second close must not double-release, got %d", *releases)
	}
}

func TestAgentThreadStatusTransitions(t *testing.T) {
	a, calls, _ := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	*calls = nil

	// A status for some other thread is swallowed without reporting.
	if _, handled := reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "999.0", BotUserID: "UBOT", Status: "is thinking…"}); !handled {
		t.Fatal("reducer must claim AssistantStatusMsg")
	}
	// So is a status from a different bot in the open thread.
	reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UOTHERBOT", Status: "is thinking…"})
	if len(*calls) != 0 {
		t.Fatalf("status for another thread or bot must not report; calls=%+v", *calls)
	}

	reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: "is thinking…"})
	reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: ""})

	if len(*calls) != 2 {
		t.Fatalf("want 2 reports, got %+v", *calls)
	}
	if !(*calls)[0].working || (*calls)[0].status != "is thinking…" {
		t.Errorf("set: %+v", (*calls)[0])
	}
	if (*calls)[1].working || (*calls)[1].status != "" {
		t.Errorf("clear: %+v", (*calls)[1])
	}
}
