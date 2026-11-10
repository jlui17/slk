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
			// stamp. With every item done and no stamp, nothing says work
			// goes on.
			name: "stampless all-done list is not a todo post",
			text: "✓ Look up this channel's info ✓ Check the member list ✓ Post three facts",
			todo: false,
		},
		{
			name: "stampless in-progress marker",
			text: "✱ Filing the Kaneo task and moving it in-progress. ○ Open the PR.",
			todo: true,
		},
		{
			// C0BCG30UGEP 1794256160.311769, stamp as Slack sends it.
			name: "in-progress todo post with a date-token stamp",
			text: "Finding why the daily run times out\n✱ Reading the slack hydrator's import wait.\n○ Find what makes the import run past 30 minutes.\n○ Open the fix as a WIP PR.\n_todos as of <!date^1794256160^{time} ({ago})|20:29 UTC>_",
			todo: true,
		},
		{
			// C0AJAL5UV34, a review's opening todo post.
			name: "review todo post, all pending",
			text: "Reviewing the QA un-flag stack.\n○ Check Justin's words in the hub thread.\n○ Review #4083.\n○ Post the verdict here and on both PRs.",
			todo: true,
		},
		{
			// C0AJAL5UV34, a review's todo post mid-way.
			name: "review todo post, in progress",
			text: "Reviewing #4031, the dashboard history prune\n✱ Re-running the author's check on the live store.\n○ Post the review on the PR.",
			todo: true,
		},
		{
			// C0CD70UR7FF 1794300224.638899.
			name: "finished report with a numbered done checklist",
			text: "dbmate is gone.\n\n*Goal:* dbmate is no longer downloaded or run anywhere.\n1. ✓ Runner written.\n2. ✓ Prod data-api start.\n3. ✓ CI schema step is green.\n4. ✓ An independent review found nothing left.\n5. ✓ #4237 closed unmerged.\nThis thread is done on my side, and nothing more will post here unless someone asks.",
			todo: false,
		},
		{
			// C0BS88WB9C0 1794255367.502889.
			name: "finished report, four done items",
			text: "Finding 13 is done.\n\nGoal: every answer key uses the same person placeholder.\n1. ✓ The cause.\n2. ✓ #4184 lets a bot root pass.\n3. ✓ c3 A, c3 B are rebuilt.\n4. ✓ One needed no rebuild.\nI need nothing from anyone. This thread waits on nothing and won't post again.",
			todo: false,
		},
		{
			// C0AJAL5UV34 1793839575.446109.
			name: "finished review, unnumbered done items",
			text: "Nothing left at `e8968200` for #4002.\n✓ The composed SQL is byte-identical to base.\n✓ Gate g passes.\n✓ The mutant turns a test red.\n✓ merge-tree is clean.\nThis thread is done. Nothing more will post here unless the head moves.",
			todo: false,
		},
		{
			// C0BS6HBB3R6 1793848925.663419.
			name: "pending mark in prose is not a todo post",
			text: "That's all the work from this thread. What's left:\n• *Anyone, after the upload:* check that one live Tag session each post a ✓/○ status after a subagent finishes.\nNothing more will post here from me unless someone asks.",
			todo: false,
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
// working state. The replies are the loaded ones: nil opens a thread with
// none, which loads as an empty list (a nil list is the root-only paint).
func openWorkingAgentThread(a *App, replies []messages.MessageItem) {
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> take a look at the CI workflows", UserID: "UHUMAN"}
	if replies == nil {
		replies = []messages.MessageItem{}
	}
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

func TestRemovingOneOfTwoAgentReactionsKeepsAck(t *testing.T) {
	a, calls, _ := newAgentTestApp(t)
	openWorkingAgentThread(a, nil)
	a.Update(ReactionAddedMsg{ChannelID: "C1", MessageTS: "100.0", UserID: "UBOT", Emoji: "eyes"})
	a.Update(ReactionAddedMsg{ChannelID: "C1", MessageTS: "100.0", UserID: "UBOT", Emoji: "+1"})
	a.Update(ReactionRemovedMsg{ChannelID: "C1", MessageTS: "100.0", UserID: "UBOT", Emoji: "eyes"})
	if got := lastReport(t, calls); got.working {
		t.Errorf("expected idle while the agent's 👍 stays, got %+v", got)
	}
	a.Update(ReactionRemovedMsg{ChannelID: "C1", MessageTS: "100.0", UserID: "UBOT", Emoji: "+1"})
	if got := lastReport(t, calls); !got.working {
		t.Errorf("expected working once the agent's last reaction is gone, got %+v", got)
	}
}

func TestRemovingOneOfTwoFetchedAgentReactionsKeepsAck(t *testing.T) {
	a, calls, _ := newAgentTestApp(t)
	openWorkingAgentThread(a, []messages.MessageItem{{
		TS: "101.0", ThreadTS: "100.0", UserID: "UHUMAN", Text: "ship it",
		Reactions: []messages.ReactionItem{
			{Emoji: "eyes", Count: 1, UserIDs: []string{"UBOT"}},
			{Emoji: "+1", Count: 1, UserIDs: []string{"UBOT"}},
		},
	}})
	a.Update(ReactionRemovedMsg{ChannelID: "C1", MessageTS: "101.0", UserID: "UBOT", Emoji: "eyes"})
	if got := lastReport(t, calls); got.working {
		t.Errorf("expected idle while the fetched 👍 stays, got %+v", got)
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
