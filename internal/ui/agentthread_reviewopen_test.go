package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
)

// reviewOpenRoot is a !review-open root as Slack sends it: the permalink
// names the annotated message 1787864174.931459, a reply in thread
// 1787794284.299719 of C0BCG30UGEP.
const reviewOpenRoot = "!review-open\n" +
	"annotation: <https://colony-pyo1658.slack.com/archives/C0BCG30UGEP/p1787864174931459?thread_ts=1787794284.299719&cid=C0BCG30UGEP>\n" +
	"author: Justin (U0BAY83R88M)\n" +
	"judge: <@UBOT|Claude>\n" +
	"reason: epistemic batch 10-07"

type previewCall struct {
	channelID ids.ChannelID
	ts        ids.MessageTS
	threadTS  ids.ThreadTS
}

// withPreview installs a message service whose Preview answers sender and
// text (or err), recording each call.
func withPreview(a *App, sender, text string, err error) *[]previewCall {
	calls := &[]previewCall{}
	a.SetMessageService(core.NewMessageService(core.MessageServiceFuncs{
		Preview: func(_ context.Context, channelID ids.ChannelID, ts ids.MessageTS, threadTS ids.ThreadTS) (string, string, error) {
			*calls = append(*calls, previewCall{channelID, ts, threadTS})
			return sender, text, err
		},
	}))
	return calls
}

func reviewOpenThread() (messages.MessageItem, []messages.MessageItem) {
	parent := messages.MessageItem{TS: "500.0", Text: reviewOpenRoot, UserID: "UHUMAN"}
	replies := []messages.MessageItem{{TS: "501.0", Text: "Blind judge: Justified.", UserID: "UBOT"}}
	return parent, replies
}

func TestReviewOpenLabelWaitsForAnnotatedMessage(t *testing.T) {
	a, calls, _ := newLLMLabelTestApp(t)
	previews := withPreview(a, "Justin Lui", "!annotate - pr 1231 is merged already", nil)
	parent, replies := reviewOpenThread()

	if cmd := a.setThreadPanel(parent, nil, "C1", "500.0"); cmd != nil {
		t.Fatalf("root-only paint returned a cmd")
	}
	cmd := a.setThreadPanel(parent, replies, "C1", "500.0")
	if cmd == nil || len(*calls) != 0 {
		t.Fatalf("want the fetch and no request yet; cmd nil=%t, calls %+v", cmd == nil, *calls)
	}
	if again := a.setThreadPanel(parent, replies, "C1", "500.0"); again != nil {
		t.Errorf("a replies reload started a second fetch")
	}

	_, _ = reduceAgentTabRelabel(a, cmd())

	want := previewCall{"C0BCG30UGEP", "1787864174.931459", "1787794284.299719"}
	if len(*previews) != 1 || (*previews)[0] != want {
		t.Errorf("preview calls = %+v, want [%+v]", *previews, want)
	}
	if len(*calls) != 1 {
		t.Fatalf("want 1 label request, got %+v", *calls)
	}
	c := (*calls)[0]
	lines := strings.Split(c.transcript, "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "justin: !review-open") ||
		lines[1] != "Justin Lui: pr 1231 is merged already" || lines[2] != "Claude: Blind judge: Justified." {
		t.Errorf("transcript:\n%s", c.transcript)
	}
	if c.teamID != "T1" || c.channelID != "C1" || c.threadTS != "500.0" || c.force || !c.reviewOpen {
		t.Errorf("request = %+v, want a review-open request", c)
	}
}

// The automatic path's fetch rides out of the replies load that fires it.
func TestReviewOpenLabelFetchReturnedByRepliesLoad(t *testing.T) {
	a, calls, _ := newLLMLabelTestApp(t)
	withPreview(a, "Justin Lui", "!annotate - pr 1231 is merged already", nil)
	parent, replies := reviewOpenThread()
	a.setThreadPanel(parent, nil, "C1", "500.0")
	a.threadVisible = true

	cmd, _ := reduceThreads(a, ThreadRepliesLoadedMsg{ThreadTS: "500.0", Replies: replies})
	for _, msg := range drainCmds(cmd) {
		if m, ok := msg.(agentTabAnnotationMsg); ok {
			reduceAgentTabRelabel(a, m)
		}
	}

	if len(*calls) != 1 || !strings.Contains((*calls)[0].transcript, "\nJustin Lui: pr 1231 is merged already\n") {
		t.Fatalf("want 1 request with the annotated line, got %+v", *calls)
	}
}

func TestReviewOpenRetitleWaitsForAnnotatedMessage(t *testing.T) {
	parent, replies := reviewOpenThread()
	a, calls, _ := newRetitleTestApp(t, parent, replies)
	withPreview(a, "Justin Lui", "!annotate - pr 1231 is merged already", nil)

	cmd := executeCommand(a, "retitle")
	if len(*calls) != 0 {
		t.Fatalf("requested before the fetch: %+v", *calls)
	}
	// The fetch is the batch's first cmd, the toast's clear the second.
	_, _ = reduceAgentTabRelabel(a, cmd().(tea.BatchMsg)[0]())

	if len(*calls) != 1 {
		t.Fatalf("want 1 label request, got %+v", *calls)
	}
	c := (*calls)[0]
	if lines := strings.Split(c.transcript, "\n"); len(lines) != 3 || lines[1] != "Justin Lui: pr 1231 is merged already" {
		t.Errorf("transcript:\n%s", c.transcript)
	}
	if !c.force || c.fallbackTaskID != "" || !c.reviewOpen {
		t.Errorf("request = %+v, want :retitle's force, no fallback and review-open", c)
	}
}

// Anything but a !review-open root with a parseable permalink sends at
// once, with the transcript as it always was.
func TestNonReviewOpenLabelSendsWithoutFetch(t *testing.T) {
	roots := []string{
		"<@UBOT> colony-562 fix the ingest retries",
		"<@UBOT> look at this\nannotation: <https://colony-pyo1658.slack.com/archives/C0BCG30UGEP/p1787864174931459>",
		"!review-open\nannotation: <https://example.com/x>\njudge: <@UBOT|Claude>",
	}
	for _, root := range roots {
		parent := messages.MessageItem{TS: "100.0", Text: root, UserID: "UHUMAN"}
		replies := []messages.MessageItem{{TS: "101.0", Text: "on it", UserID: "UBOT"}}

		a, calls, _ := newLLMLabelTestApp(t)
		previews := withPreview(a, "", "", errors.New("must not fetch"))
		a.setThreadPanel(parent, nil, "C1", "100.0")
		if cmd := a.setThreadPanel(parent, replies, "C1", "100.0"); cmd != nil {
			t.Errorf("%q: automatic path returned a cmd", root)
		}
		want := a.retitleTranscript(parent, replies, "UBOT", "")
		if len(*calls) != 1 || (*calls)[0].transcript != want || (*calls)[0].reviewOpen {
			t.Errorf("%q: automatic requests = %+v, want one with %q, not review-open", root, *calls, want)
		}

		r, rcalls, _ := newRetitleTestApp(t, parent, replies)
		rpreviews := withPreview(r, "", "", errors.New("must not fetch"))
		_ = executeCommand(r, "retitle")
		if len(*rcalls) != 1 || (*rcalls)[0].transcript != want || (*rcalls)[0].reviewOpen {
			t.Errorf("%q: :retitle requests = %+v, want one with %q, not review-open", root, *rcalls, want)
		}
		if len(*previews)+len(*rpreviews) != 0 {
			t.Errorf("%q: fetched %+v %+v", root, *previews, *rpreviews)
		}
	}
}

func TestReviewOpenFetchFailureSendsPlainTranscript(t *testing.T) {
	a, calls, _ := newLLMLabelTestApp(t)
	withPreview(a, "", "", errors.New("channel_not_found"))
	parent, replies := reviewOpenThread()
	a.setThreadPanel(parent, nil, "C1", "500.0")

	_, _ = reduceAgentTabRelabel(a, a.setThreadPanel(parent, replies, "C1", "500.0")())

	// The thread is still a review, so the request still is one.
	if want := a.retitleTranscript(parent, replies, "UBOT", ""); len(*calls) != 1 || (*calls)[0].transcript != want || !(*calls)[0].reviewOpen {
		t.Fatalf("requests = %+v, want one review-open request with %q", *calls, want)
	}
}

// A review-open result names the tab "!review", then the model's id and
// label; the root's hoisted id never rides, and a forced result still lands.
func TestReviewOpenResultRendersReviewLabel(t *testing.T) {
	rows := []struct {
		name string
		msg  AgentTabRelabelMsg
		want string
	}{
		{"id and label, echo stripped", AgentTabRelabelMsg{TaskID: "#1231", Label: "#1231 already merged"}, "!review #1231 already merged"},
		{"no id", AgentTabRelabelMsg{Label: "phantom composer job"}, "!review phantom composer job"},
		{"hoisted id ignored", AgentTabRelabelMsg{FallbackTaskID: "batch-10", Label: "phantom composer job"}, "!review phantom composer job"},
		{"id alone", AgentTabRelabelMsg{TaskID: "#1808", Label: "\"#1808\""}, "!review #1808"},
		{":retitle", AgentTabRelabelMsg{TaskID: "#1334", RetitleGen: 1, Label: "stale merge status"}, "!review #1334 stale merge status"},
	}
	for _, r := range rows {
		parent, _ := reviewOpenThread()
		a, _, tabNames := newRetitleTestApp(t, parent, nil)
		m := r.msg
		m.TeamID, m.ChannelID, m.ThreadTS, m.ReviewOpen = "T1", "C1", "500.0", true
		a.agentSidebar.retitleGen = m.RetitleGen // the :retitle row's is the latest

		_, _ = reduceAgentTabRelabel(a, m)

		if last := (*tabNames)[len(*tabNames)-1]; last != r.want {
			t.Errorf("%s: tab = %q, want %q", r.name, last, r.want)
		}
	}
}

// With no id and nothing left of the label, the deterministic label stands,
// as it does for any thread: a bare "!review" names nothing.
func TestReviewOpenResultUnusableLabelDropped(t *testing.T) {
	parent, _ := reviewOpenThread()
	a, _, tabNames := newRetitleTestApp(t, parent, nil)
	before := len(*tabNames)

	_, _ = reduceAgentTabRelabel(a, AgentTabRelabelMsg{
		TeamID: "T1", ChannelID: "C1", ThreadTS: "500.0", ReviewOpen: true, FallbackTaskID: "batch-10", Label: "\"\"",
	})

	if len(*tabNames) != before {
		t.Errorf("unusable result renamed the tab: %+v", *tabNames)
	}
}

func TestReviewOpenFetchDroppedAfterThreadSwitch(t *testing.T) {
	a, calls, _ := newLLMLabelTestApp(t)
	withPreview(a, "Justin Lui", "!annotate - pr 1231 is merged already", nil)
	parent, replies := reviewOpenThread()
	fetch := a.setThreadPanel(parent, replies, "C1", "500.0")

	a.setThreadPanel(messages.MessageItem{TS: "200.0", Text: "<@UBOT> ship the viewer", UserID: "UHUMAN"}, []messages.MessageItem{}, "C1", "200.0")
	_, _ = reduceAgentTabRelabel(a, fetch())

	if len(*calls) != 1 || (*calls)[0].threadTS != "200.0" {
		t.Fatalf("want only the new thread's request, got %+v", *calls)
	}
}

// Back on the same thread before the first fetch answers, only the fetch
// the return started sends: the thread's request still goes out once.
func TestReviewOpenFetchSupersededByNewerFetch(t *testing.T) {
	a, calls, _ := newLLMLabelTestApp(t)
	withPreview(a, "Justin Lui", "!annotate - pr 1231 is merged already", nil)
	parent, replies := reviewOpenThread()
	first := a.setThreadPanel(parent, replies, "C1", "500.0")
	a.setThreadPanel(messages.MessageItem{TS: "200.0", Text: "<@UBOT> ship the viewer", UserID: "UHUMAN"}, []messages.MessageItem{}, "C1", "200.0")
	second := a.setThreadPanel(parent, replies, "C1", "500.0")

	_, _ = reduceAgentTabRelabel(a, first())
	_, _ = reduceAgentTabRelabel(a, second())

	if len(*calls) != 2 || (*calls)[1].threadTS != "500.0" {
		t.Fatalf("want the other thread's request then one for 500.0, got %+v", *calls)
	}
}

func TestReviewOpenAnnotation(t *testing.T) {
	cases := []struct {
		root   string
		wantTS ids.MessageTS
		ok     bool
	}{
		{reviewOpenRoot, "1787864174.931459", true},
		{"  !review-open\nannotation: https://x.slack.com/archives/C1/p1787864174931459\n", "1787864174.931459", true},
		{"!review-open\nannotation: <https://x.slack.com/archives/C1/p1787864174931459|the annotation>", "1787864174.931459", true},
		{"!review-open\nauthor: Justin", "", false},
		{"!review-open\nannotation: <https://example.com/x>", "", false},
		{"review this\nannotation: <https://x.slack.com/archives/C1/p1787864174931459>", "", false},
	}
	for _, c := range cases {
		pl, ok := reviewOpenAnnotation(c.root)
		if ok != c.ok || pl.MessageTS != c.wantTS {
			t.Errorf("reviewOpenAnnotation(%q) = %+v, %t; want ts %q, %t", c.root, pl, ok, c.wantTS, c.ok)
		}
	}
}
