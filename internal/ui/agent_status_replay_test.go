package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"text/tabwriter"

	"github.com/gammons/slk/internal/ui/messages"
)

// agentStatusReplaySequence is one thread of
// testdata/agent_status_replay.json: the events the layer above the judge
// receives, in order, each with the state herdr should show after it.
// KnownOpen marks a sequence that misses on purpose until a known bug is
// fixed, and says which.
type agentStatusReplaySequence struct {
	Name      string                   `json:"name"`
	Source    string                   `json:"source"`
	Note      string                   `json:"note"`
	KnownOpen string                   `json:"known_open,omitempty"`
	Events    []agentStatusReplayEvent `json:"events"`
}

// agentStatusReplayEvent is exactly one of: a new message (the first one
// is the thread's root), an edit of the newest message, a reaction added to
// or removed from the newest message, or the judge's answer to the newest
// question it was asked ("working", "blocked", "idle" or "failed").
type agentStatusReplayEvent struct {
	By      string     `json:"by,omitempty"`
	Message *string    `json:"message,omitempty"`
	Edit    *string    `json:"edit,omitempty"`
	React   string     `json:"react,omitempty"`
	Unreact string     `json:"unreact,omitempty"`
	Judge   string     `json:"judge,omitempty"`
	Want    AgentState `json:"want"`
	Why     string     `json:"why,omitempty"`
}

// Tag's two ids in Justin's workspace: its bot_message posts carry the bot
// id, its reactions and its replies the bot user's id.
const (
	tagBotID  = "B0AJJ4EGEMC"
	tagUserID = "U0AJPPX8SE8"
)

// replayAuthors maps a dataset author to the Slack id the layer sees. The
// user is the account slk runs as (newAgentTestApp's current user), who
// writes in their own agent threads; the agent is newAgentTestApp's bot.
// The replay's user cache adds Tag's ids as bots.
var replayAuthors = map[string]string{
	"user":    "USELF",
	"agent":   "UBOT",
	tagBotID:  tagBotID,
	tagUserID: tagUserID,
}

func loadAgentStatusReplay(t *testing.T) []agentStatusReplaySequence {
	t.Helper()
	const path = "testdata/agent_status_replay.json"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var seqs []agentStatusReplaySequence
	if err := json.Unmarshal(data, &seqs); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	for _, s := range seqs {
		if len(s.Events) == 0 || s.Events[0].Message == nil {
			t.Fatalf("%s: %q: the first event must be the root message", path, s.Name)
		}
		for i, e := range s.Events {
			kinds := 0
			for _, set := range []bool{e.Message != nil, e.Edit != nil, e.React != "", e.Unreact != "", e.Judge != ""} {
				if set {
					kinds++
				}
			}
			_, knownAuthor := replayAuthors[e.By]
			needsAuthor := e.Message != nil || e.React != "" || e.Unreact != ""
			switch {
			case kinds != 1:
				t.Fatalf("%s: %q event %d: want exactly one of message, edit, react, unreact, judge", path, s.Name, i)
			case needsAuthor && !knownAuthor:
				t.Fatalf("%s: %q event %d: by %q is not one of user, agent, %s, %s", path, s.Name, i, e.By, tagBotID, tagUserID)
			case e.Judge != "" && !strings.Contains(" working blocked idle failed ", " "+e.Judge+" "):
				t.Fatalf("%s: %q event %d: judge %q is not working, blocked, idle or failed", path, s.Name, i, e.Judge)
			case e.Want != AgentWorking && e.Want != AgentBlocked && e.Want != AgentIdle:
				t.Fatalf("%s: %q event %d: want %q is not working, blocked or idle", path, s.Name, i, e.Want)
			}
		}
	}
	return seqs
}

// agentStatusReplayResult is what herdr saw for one event.
type agentStatusReplayResult struct {
	event     string
	published []AgentState
	got       AgentState
	// notAsked is a judge event with no question to answer: the layer
	// never asked the judge about the newest message.
	notAsked bool
}

func (r agentStatusReplayResult) miss(want AgentState) bool {
	return r.notAsked || r.got != want
}

// replayAgentStatus feeds s through a fresh App, with a judge that answers
// exactly what the dataset says, and records what herdr shows after each
// event.
func replayAgentStatus(t *testing.T, s agentStatusReplaySequence) []agentStatusReplayResult {
	t.Helper()
	a, reports, unreads := newAgentTestApp(t)
	cached := a.agentSidebar.userInfo
	a.agentSidebar.userInfo = func(id string) (string, bool, bool) {
		if id == tagBotID || id == tagUserID {
			return "Claude", true, true
		}
		return cached(id)
	}
	judged := withWorkingJudge(a)
	seq := withHerdrSequence(a, reports, unreads)

	const channelID, threadTS = "C1", "100.0"
	var newest messages.MessageItem
	answered := 0
	var results []agentStatusReplayResult
	for i, e := range s.Events {
		before := len(*seq)
		r := agentStatusReplayResult{}
		switch {
		case e.Message != nil && i == 0:
			r.event = fmt.Sprintf("%s: %s", e.By, replayQuote(*e.Message))
			newest = messages.MessageItem{TS: threadTS, UserID: replayAuthors[e.By], Text: *e.Message}
			a.setThreadPanel(newest, []messages.MessageItem{}, channelID, threadTS)
			a.threadVisible = true
			if !a.agentSidebar.thread.active {
				t.Fatalf("%q: the root does not make an agent thread", s.Name)
			}
		case e.Message != nil:
			r.event = fmt.Sprintf("%s: %s", e.By, replayQuote(*e.Message))
			newest = messages.MessageItem{TS: fmt.Sprintf("%d.0", 100+i), ThreadTS: threadTS, UserID: replayAuthors[e.By], Text: *e.Message}
			a.Update(NewMessageMsg{ChannelID: channelID, Message: newest})
		case e.Edit != nil:
			r.event = "edit: " + replayQuote(*e.Edit)
			newest.Text, newest.IsEdited = *e.Edit, true
			a.Update(NewMessageMsg{ChannelID: channelID, Message: newest, Changed: true})
		case e.React != "":
			r.event = fmt.Sprintf("%s reacts :%s:", e.By, e.React)
			a.Update(ReactionAddedMsg{ChannelID: channelID, MessageTS: newest.TS, UserID: replayAuthors[e.By], Emoji: e.React})
		case e.Unreact != "":
			r.event = fmt.Sprintf("%s removes :%s:", e.By, e.Unreact)
			a.Update(ReactionRemovedMsg{ChannelID: channelID, MessageTS: newest.TS, UserID: replayAuthors[e.By], Emoji: e.Unreact})
		case e.Judge != "":
			r.event = "judge: " + e.Judge
			if answered == len(*judged) {
				r.notAsked = true
				break
			}
			// The perfect judge answers the newest question; any older one
			// still unanswered is for text the thread has moved past.
			n := len(*judged) - 1
			answered = len(*judged)
			if e.Judge == "failed" {
				a.Update(failureFor(t, judged, n))
			} else {
				a.Update(verdictFor(t, judged, n, AgentState(e.Judge)))
			}
		}
		r.published = append([]AgentState(nil), (*seq)[before:]...)
		if len(*seq) > 0 {
			r.got = (*seq)[len(*seq)-1]
		}
		results = append(results, r)
	}
	return results
}

func replayQuote(text string) string {
	const width = 40
	if r := []rune(text); len(r) > width {
		text = string(r[:width]) + "…"
	}
	return fmt.Sprintf("%q", text)
}

// TestAgentStatusReplay replays every sequence of
// testdata/agent_status_replay.json through the layer above the judge, with
// a judge that always gives the dataset's verdict, and fails on any event
// whose published state misses. With a perfect judge, a miss is the
// layer's. A known_open sequence must miss: once it passes, the test fails
// until its mark is removed. Run with -v for the table.
func TestAgentStatusReplay(t *testing.T) {
	seqs := loadAgentStatusReplay(t)
	var table strings.Builder
	w := tabwriter.NewWriter(&table, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "sequence\t#\tevent\tpublished\twant\tgot\t\n")
	passed, total := 0, 0
	var knownOpen []string
	for _, s := range seqs {
		var results []agentStatusReplayResult
		t.Run(s.Name, func(t *testing.T) {
			results = replayAgentStatus(t, s)
			misses := 0
			for i, r := range results {
				if r.miss(s.Events[i].Want) {
					misses++
				}
			}
			switch {
			case s.KnownOpen == "" && misses > 0:
				t.Errorf("%d of %d events miss (table below)", misses, len(results))
			case s.KnownOpen != "" && misses == 0:
				t.Errorf("passes now: remove its known_open mark from testdata/agent_status_replay.json (%s)", s.KnownOpen)
			}
		})
		if s.KnownOpen != "" {
			knownOpen = append(knownOpen, fmt.Sprintf("%s: %s", s.Name, s.KnownOpen))
		}
		for i, r := range results {
			want := s.Events[i].Want
			published := "-"
			if len(r.published) > 0 {
				published = fmt.Sprint(r.published)
			}
			got := string(r.got)
			if r.notAsked {
				got += ", judge not asked"
			}
			mark := ""
			switch {
			case !r.miss(want):
				passed++
			case s.KnownOpen != "":
				mark = "MISS (known open)"
			default:
				mark = "MISS"
			}
			total++
			fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\t%s\n", s.Name, i, r.event, published, want, got, mark)
		}
	}
	w.Flush()
	t.Logf("\n%s\npassed %d of %d events\nknown open:\n  %s", table.String(), passed, total, strings.Join(knownOpen, "\n  "))
}
