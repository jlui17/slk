package ui

import (
	"testing"

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
	a := NewApp()
	calls := &[]agentReportCall{}
	releases := new(int)
	a.SetAgentReporter(
		func(agent, displayName, title string, working bool, statusMessage string) {
			*calls = append(*calls, agentReportCall{agent, displayName, title, working, statusMessage})
		},
		func() { *releases++ },
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
	return a, calls, releases
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
	if _, handled := reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "999.0", Status: "is thinking…"}); !handled {
		t.Fatal("reducer must claim AssistantStatusMsg")
	}
	if len(*calls) != 0 {
		t.Fatalf("status for another thread must not report; calls=%+v", *calls)
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
