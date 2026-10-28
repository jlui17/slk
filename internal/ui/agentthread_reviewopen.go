// A !review-open thread's root is a form (annotation permalink, author,
// judge, reason) that names nothing once its link and mentions are
// stripped: every such thread reads the same to the label model. Its
// subject is the annotated message the permalink points to, so a label
// request for one fetches that message first and sends its text as the
// line after the root. A kickoff made from Colony's Annotate button has no
// annotation: permalink: it links the clicked message with where: and
// carries the annotation itself as its note, which is sent as that line
// with no fetch.
package ui

import (
	"context"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/slackurl"
	"github.com/gammons/slk/internal/ui/messages"
)

// reviewOpenAnnotationRe captures the permalink on a !review-open root's
// annotation line, raw (<url> or <url|label>) or bare. A key may follow a
// space rather than a newline: a kickoff Claude posts is cached on one line.
var reviewOpenAnnotationRe = regexp.MustCompile(`(?:^|\s)annotation:\s*<?(https://[^\s>|]+)`)

// reviewOpenNoteRe captures a kickoff's note, its last field, and
// reviewOpenAuthorRe the author's user id ("author: Justin (U0BAY83R88M)").
var (
	reviewOpenNoteRe   = regexp.MustCompile(`(?s)(?:^|\s)note:\s*(.+)`)
	reviewOpenAuthorRe = regexp.MustCompile(`(?:^|\s)author:[^\n]*?\b(U[A-Z0-9]+)\b`)
)

// reviewOpenNoteQuoteRe matches the quote marker that opens each note line,
// and reviewOpenNoteLabelRe the [positive] or [negative] line after it,
// with the next line's marker when the kickoff is cached on one line.
var (
	reviewOpenNoteQuoteRe = regexp.MustCompile(`(?m)^\s*(?:&gt;|>) ?`)
	reviewOpenNoteLabelRe = regexp.MustCompile(`^\[(?:positive|negative)\]\s*(?:(?:&gt;|>)\s*)?`)
)

// reviewOpenNote returns the annotation a !review-open root carries as its
// note, and its author's user id, when rootText is one.
func reviewOpenNote(rootText string) (authorID, note string, ok bool) {
	text := strings.TrimSpace(rootText)
	if !strings.HasPrefix(text, "!review-open") {
		return "", "", false
	}
	m := reviewOpenNoteRe.FindStringSubmatch(text)
	if m == nil {
		return "", "", false
	}
	note = reviewOpenNoteQuoteRe.ReplaceAllString(m[1], "")
	note = strings.TrimSpace(reviewOpenNoteLabelRe.ReplaceAllString(note, ""))
	if a := reviewOpenAuthorRe.FindStringSubmatch(text); a != nil {
		authorID = a[1]
	}
	return authorID, note, note != ""
}

// reviewOpenAnnotation returns the annotated message's permalink when
// rootText is a !review-open root.
func reviewOpenAnnotation(rootText string) (slackurl.Permalink, bool) {
	text := strings.TrimSpace(rootText)
	if !strings.HasPrefix(text, "!review-open") {
		return slackurl.Permalink{}, false
	}
	m := reviewOpenAnnotationRe.FindStringSubmatch(text)
	if m == nil {
		return slackurl.Permalink{}, false
	}
	return slackurl.Parse(m[1])
}

// labelTarget names the thread a label request is for, and the bot whose
// mention its root line drops (empty when none is known).
type labelTarget struct {
	teamID    string
	channelID string
	threadTS  string
	botUserID string
}

// agentTabAnnotationMsg carries a fetched annotated message back into the
// loop, with the label request it was fetched for. Empty Text means the
// fetch failed or found nothing: the request goes out without the line.
type agentTabAnnotationMsg struct {
	gen            uint64
	target         labelTarget
	parent         messages.MessageItem
	replies        []messages.MessageItem
	fallbackTaskID string
	retitleGen     uint64
	sender         string
	text           string
}

// requestAgentTabLabel sends transcript, target's label transcript, to the
// generator; retitleGen numbers a :retitle request and is zero for the
// automatic one. For a !review-open root it instead returns the fetch of the
// annotated message; sendAnnotatedAgentTabLabel sends the transcript
// rebuilt with it. A root that carries its annotation as a note sends the
// transcript rebuilt with the note at once.
func (a *App) requestAgentTabLabel(target labelTarget, parent messages.MessageItem, replies []messages.MessageItem, transcript, fallbackTaskID string, retitleGen uint64) tea.Cmd {
	pl, ok := reviewOpenAnnotation(parent.Text)
	if !ok {
		authorID, note, reviewOpen := reviewOpenNote(parent.Text)
		if reviewOpen {
			transcript = a.retitleTranscript(parent, replies, target.botUserID, a.retitleLine(authorID, note, maxRetitleReply))
		}
		a.agentSidebar.requestLabel(target.teamID, target.channelID, target.threadTS, transcript, fallbackTaskID, retitleGen, reviewOpen)
		return nil
	}
	a.agentSidebar.labelFetchGen++
	msg := agentTabAnnotationMsg{
		gen: a.agentSidebar.labelFetchGen, target: target,
		parent: parent, replies: replies, fallbackTaskID: fallbackTaskID, retitleGen: retitleGen,
	}
	messageSvc := a.messageSvc
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		sender, text, err := messageSvc.Preview(ctx, pl.ChannelID, pl.MessageTS, pl.ThreadTS)
		if err != nil {
			debuglog.General("tablabel: annotation %s/%s: %v", pl.ChannelID, pl.MessageTS, err)
			return msg
		}
		msg.sender, msg.text = sender, text
		return msg
	}
}

// sendAnnotatedAgentTabLabel sends the label request a fetch was for,
// unless its result would be dropped (see labelStillWanted) or, for the
// automatic request, a later fetch superseded it. A :retitle fetch answers
// to the :retitle numbering alone, so an agent thread opened meanwhile
// can't cancel it.
func (a *App) sendAnnotatedAgentTabLabel(m agentTabAnnotationMsg) {
	t := m.target
	if (m.retitleGen == 0 && m.gen != a.agentSidebar.labelFetchGen) || a.agentSidebar.requestLabel == nil ||
		!a.labelStillWanted(t.teamID, t.channelID, t.threadTS, m.retitleGen) {
		return
	}
	text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(m.text), "!annotate"))
	line := a.speakerRetitleLine(stripSessionLabel(m.sender), strings.TrimPrefix(text, "-"), maxRetitleReply)
	transcript := a.retitleTranscript(m.parent, m.replies, t.botUserID, line)
	a.agentSidebar.requestLabel(t.teamID, t.channelID, t.threadTS, transcript, m.fallbackTaskID, m.retitleGen, true)
}

// reviewOpenTabLabel renders a !review-open thread's tab: "!review", then
// the model's id and label when it gave them ("!review #1231 already
// merged"). The root's hoisted id never rides: the root is a form whose
// fields are not the review's subject.
func reviewOpenTabLabel(id, label string) string {
	tab := "!review"
	if id != "" {
		tab += " " + id
	}
	if label != "" {
		tab += " " + label
	}
	return tab
}
