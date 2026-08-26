package ui

import (
	"testing"

	"github.com/gammons/slk/internal/ui/messages"
)

// The todo texts are lifted verbatim from live Claude-in-Slack threads in
// #eng (conversations.replies, 2026-08-26): the text field flattens
// newlines to spaces and ends with the italic stamp.
func TestIsAgentTodoText(t *testing.T) {
	cases := []struct {
		name string
		text string
		todo bool
	}{
		{
			name: "in-progress todo post",
			text: "Picking this up — fail-fast for the loki replay's read-back verify.  ✱ Cloning colony/colony. ○ Implement: keep retrying while the count climbs. _todos as of 19:04 UTC_",
			todo: true,
		},
		{
			name: "all-done todo post still counts",
			text: "Taking the independent review of #1183. ✓ Review skills read. ✓ Verdict posted below. _todos as of 21:20 UTC_",
			todo: true,
		},
		{
			// Observed live in the e2e: not every todo post carries the
			// stamp.
			name: "stampless all-done todo post",
			text: "✓ Look up this channel's info ✓ Check the member list ✓ Post three facts",
			todo: true,
		},
		{
			name: "stampless in-progress marker",
			text: "✱ Filing the Kaneo task and moving it in-progress. ○ Open the PR.",
			todo: true,
		},
		{
			name: "plain reply",
			text: "No dev-VM runs for this PR by either of us.",
			todo: false,
		},
		{
			name: "single checkmark in prose is not a todo post",
			text: "CI is green ✓ and the PR is mergeable at the reviewed head.",
			todo: false,
		},
		{
			name: "stamp mid-text alone is not a todo post",
			text: "the _todos as of 19:04 UTC_ stamp is how I format todo posts, by the way",
			todo: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAgentTodoText(tc.text); got != tc.todo {
				t.Errorf("isAgentTodoText() = %v, want %v", got, tc.todo)
			}
		})
	}
}

// openWorkingAgentThread opens a tracked agent thread through the real
// panel path (setThreadPanel, so the last-message snapshot runs) with the
// user's unanswered root as the newest message — the canonical derived
// working state.
func openWorkingAgentThread(a *App, replies []messages.MessageItem) {
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> take a look at the CI workflows", UserID: "UHUMAN"}
	a.setThreadPanel(parent, replies, "C1", "100.0")
	a.threadVisible = true
}

func lastReport(t *testing.T, calls *[]agentReportCall) agentReportCall {
	t.Helper()
	if len(*calls) == 0 {
		t.Fatal("no agent reports published")
	}
	return (*calls)[len(*calls)-1]
}

func TestDerivedWorkingOnUnansweredRoot(t *testing.T) {
	a, calls, _ := newAgentTestApp(t)
	openWorkingAgentThread(a, nil)
	if got := lastReport(t, calls); !got.working || got.status != "" {
		t.Errorf("expected working report with empty status, got %+v", got)
	}
}

func TestAgentAckReactionIdles(t *testing.T) {
	a, calls, _ := newAgentTestApp(t)
	openWorkingAgentThread(a, nil)
	// Any emoji from the bot user is an ack; the emoji name is irrelevant.
	a.Update(ReactionAddedMsg{ChannelID: "C1", MessageTS: "100.0", UserID: "UBOT", Emoji: "rocket"})
	if got := lastReport(t, calls); got.working {
		t.Errorf("expected idle after agent ack, got %+v", got)
	}
	// Removing the ack re-arms the working state.
	a.Update(ReactionRemovedMsg{ChannelID: "C1", MessageTS: "100.0", UserID: "UBOT", Emoji: "rocket"})
	if got := lastReport(t, calls); !got.working {
		t.Errorf("expected working after ack removal, got %+v", got)
	}
}

func TestOtherHumansReactionIsNoAck(t *testing.T) {
	a, calls, _ := newAgentTestApp(t)
	openWorkingAgentThread(a, nil)
	a.Update(ReactionAddedMsg{ChannelID: "C1", MessageTS: "100.0", UserID: "UHUMAN", Emoji: "+1"})
	if got := lastReport(t, calls); !got.working {
		t.Errorf("expected still working after non-bot reaction, got %+v", got)
	}
}

func TestAgentReplyFlow(t *testing.T) {
	a, calls, _ := newAgentTestApp(t)
	openWorkingAgentThread(a, nil)

	// A todo post keeps the agent working.
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT",
		Text: "Picking this up. ✱ Reading the workflows. ○ Report findings. _todos as of 19:04 UTC_",
	}})
	if got := lastReport(t, calls); !got.working {
		t.Errorf("expected working after todo post, got %+v", got)
	}

	// The plain answer idles it, carrying the deferred unread count as
	// the completion edge's status.
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "102.0", ThreadTS: "100.0", UserID: "UBOT",
		Text: "Done — the workflows are already parallel where it matters.",
	}})
	got := lastReport(t, calls)
	if got.working {
		t.Errorf("expected idle after plain agent answer, got %+v", got)
	}
	if got.status != "2 unread replies" {
		t.Errorf("expected deferred unread count on the completion edge, got %+v", got)
	}

	// The user asking again re-arms working.
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "103.0", ThreadTS: "100.0", UserID: "UHUMAN", Text: "what about the spanner tests?",
	}})
	if got := lastReport(t, calls); !got.working {
		t.Errorf("expected working after user follow-up, got %+v", got)
	}
}

func TestAuthorlessReplyReadsIdle(t *testing.T) {
	a, calls, _ := newAgentTestApp(t)
	openWorkingAgentThread(a, nil)
	// A message with no author must not read as a human's: that would
	// latch working with no reply ever able to clear it.
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", Text: "x",
	}})
	if got := lastReport(t, calls); got.working {
		t.Errorf("expected idle for authorless newest message, got %+v", got)
	}
}

func TestLateBotResolutionCorrectsHumanRead(t *testing.T) {
	a, calls, _ := newAgentTestApp(t)
	openWorkingAgentThread(a, nil)
	// bot_message authors carry a bare bot_id the user cache can't answer
	// for yet; the reply provisionally reads as a human's (working).
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "BRAW", Text: "Done, merged.",
	}})
	if got := lastReport(t, calls); !got.working {
		t.Errorf("expected provisional working for unresolved author, got %+v", got)
	}
	a.Update(UserResolvedMsg{TeamID: "T1", UserID: "BRAW", DisplayName: "Claude", IsBot: true})
	if got := lastReport(t, calls); got.working {
		t.Errorf("expected idle once author resolved as bot, got %+v", got)
	}
}

func TestEditTogglesTodoState(t *testing.T) {
	a, calls, _ := newAgentTestApp(t)
	openWorkingAgentThread(a, nil)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "On it.",
	}})
	if got := lastReport(t, calls); got.working {
		t.Errorf("expected idle after plain reply, got %+v", got)
	}
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", IsEdited: true,
		Text: "On it. ✱ Reading. ○ Fixing. _todos as of 20:40 UTC_",
	}})
	if got := lastReport(t, calls); !got.working {
		t.Errorf("expected working after edit added the todo stamp, got %+v", got)
	}
}

func TestDeletedLastMessageReadsIdle(t *testing.T) {
	a, calls, _ := newAgentTestApp(t)
	openWorkingAgentThread(a, nil)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UHUMAN", Text: "also this please",
	}})
	a.Update(WSMessageDeletedMsg{ChannelID: "C1", TS: "101.0"})
	if got := lastReport(t, calls); got.working {
		t.Errorf("expected idle after the newest message was deleted, got %+v", got)
	}
}

func TestWSTurnEndKeepsDerivedWorking(t *testing.T) {
	a, calls, _ := newAgentTestApp(t)
	openWorkingAgentThread(a, nil)
	a.Update(AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: "is thinking…"})
	if got := lastReport(t, calls); !got.working || got.status != "is thinking…" {
		t.Errorf("expected working with status text, got %+v", got)
	}
	// The WS turn ends, but the user's message is still unanswered: the
	// row stays working, with the dead turn's status text cleared.
	a.Update(AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: ""})
	if got := lastReport(t, calls); !got.working || got.status != "" {
		t.Errorf("expected working with cleared status after turn end, got %+v", got)
	}
}

func TestSnapshotSeesFetchedAck(t *testing.T) {
	a, calls, _ := newAgentTestApp(t)
	// The ack happened while slk wasn't running: the fetched replies carry
	// it, so the thread opens idle.
	openWorkingAgentThread(a, []messages.MessageItem{{
		TS: "101.0", ThreadTS: "100.0", UserID: "UHUMAN", Text: "ship it",
		Reactions: []messages.ReactionItem{{Emoji: "rocket", Count: 1, UserIDs: []string{"UBOT"}}},
	}})
	if got := lastReport(t, calls); got.working {
		t.Errorf("expected idle when fetched last reply is acked, got %+v", got)
	}
}
