package ui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/peerstatus"
	"github.com/gammons/slk/internal/ui/profilecard"
	"github.com/gammons/slk/internal/usernames"
)

// fakeProfiles answers every fetch with dana's full profile, or err.
type fakeProfiles struct {
	fetched []string
	err     error
}

func (f *fakeProfiles) Profile(_ context.Context, teamID, userID string) (profilecard.Profile, error) {
	f.fetched = append(f.fetched, teamID+"/"+userID)
	if f.err != nil {
		return profilecard.Profile{}, f.err
	}
	return profilecard.Profile{
		UserID: userID, RealName: "Dana Kim", DisplayName: "dana", Title: "Staff Engineer",
		Presence: "active", TZ: "America/Los_Angeles",
		Status: peerstatus.Status{Emoji: ":palm_tree:", Text: "Out until Monday"},
	}, nil
}

type profileCardFixture struct {
	a        *App
	opened   []string // URLs sent to the browser
	threads  []string // thread fetches
	dms      [][]string
	profiles *fakeProfiles
}

// profileCardApp shows, in workspace acme, a message that mentions dana
// (U2) beside a #channel mention and an ordinary link.
func profileCardApp(t *testing.T) *profileCardFixture {
	t.Helper()
	f := &profileCardFixture{profiles: &fakeProfiles{}}
	f.a = newHarnessApp(t,
		withHarnessSize(200, 40),
		withApp(func(a *App) {
			_, _ = a.Update(WorkspaceReadyMsg{
				TeamID: "T1", Domain: "acme", InitialActive: true,
				UserNames: usernames.FromMap(map[string]string{"U2": "dana"}),
			})
		}),
		withHarnessMessages(messages.MessageItem{
			TS: "1.0", UserID: "U1", UserName: "alice", Timestamp: "1:00 PM",
			Text: "ping <@U2> in <#C9|general> about <" + clickLinkURL + "|clickme>",
		}),
		withApp(func(a *App) {
			a.activeChannelID = "C1"
			a.SetNowFunc(func() time.Time { return time.Date(2026, 10, 7, 21, 41, 0, 0, time.UTC) })
			a.SetProfileService(f.profiles)
			a.browserOpener = func(url string) tea.Cmd {
				f.opened = append(f.opened, url)
				return nil
			}
			a.setThreadFetcherForTest(func(_ ids.ChannelID, ts ids.ThreadTS) core.Msg {
				f.threads = append(f.threads, string(ts))
				return nil
			})
			fns := channelFuncsForTest(a)
			fns.OpenConversation = func(userIDs []string, _ uint64) core.Cmd {
				f.dms = append(f.dms, userIDs)
				return nil
			}
			setChannelFuncsForTest(a, fns)
		}),
	)
	return f
}

func (f *profileCardFixture) assertCardFor(t *testing.T, what string) {
	t.Helper()
	if f.a.mode != ModeProfileCard {
		t.Fatalf("%s: mode %v, want the profile card", what, f.a.mode)
	}
	if !reflect.DeepEqual(f.profiles.fetched, []string{"T1/U2"}) {
		t.Errorf("%s: fetched %v, want [T1/U2]", what, f.profiles.fetched)
	}
	frame := screen(f.a)
	for _, want := range []string{"╭─ Dana Kim ─", "@dana · Staff Engineer", "● Active · 2:41 PM local (PDT)", "Out until Monday", profilecard.Footer} {
		if !strings.Contains(frame, want) {
			t.Errorf("%s: card lacks %q:\n%s", what, want, frame)
		}
	}
	if len(f.threads) != 0 || len(f.opened) != 0 {
		t.Errorf("%s: also fetched threads %v, opened %v", what, f.threads, f.opened)
	}
}

func TestClickOnMentionInMessagesPaneOpensProfileCard(t *testing.T) {
	f := profileCardApp(t)
	x, y := screenPos(t, f.a, "@dana")
	click(f.a, x+2, y)
	f.assertCardFor(t, "click on @dana")
	if f.a.messagepane.HasSelection() {
		t.Error("the click left a selection")
	}
}

func TestClickOnMentionInThreadOpensProfileCard(t *testing.T) {
	for _, target := range []string{"reply", "parent"} {
		t.Run(target, func(t *testing.T) {
			f := profileCardApp(t)
			f.a.threadVisible = true
			f.a.threadPanel.SetThread(
				messages.MessageItem{TS: "5.0", UserID: "U1", UserName: "alice", Timestamp: "1:05 PM", Text: "parent asks <@U2>"},
				[]messages.MessageItem{{TS: "5.1", ThreadTS: "5.0", UserID: "U3", UserName: "bob", Timestamp: "1:06 PM", Text: "reply asks <@U2>"}},
				"C1", "5.0")
			x, y := screenPos(t, f.a, target+" asks @dana")
			click(f.a, x+len(target+" asks "), y)
			f.assertCardFor(t, "click on the "+target+"'s @dana")
		})
	}
}

func TestClickBesideMentionOpensThreadAsBefore(t *testing.T) {
	f := profileCardApp(t)
	x, y := screenPos(t, f.a, "ping @dana")
	click(f.a, x, y)
	if f.a.mode == ModeProfileCard || len(f.profiles.fetched) != 0 {
		t.Errorf("a click beside the mention opened the card (fetched %v)", f.profiles.fetched)
	}
	if !reflect.DeepEqual(f.threads, []string{"1.0"}) {
		t.Errorf("a click beside the mention fetched threads %v, want [1.0]", f.threads)
	}
}

func TestClickOnChannelMentionOpensThreadAsBefore(t *testing.T) {
	f := profileCardApp(t)
	x, y := screenPos(t, f.a, "#general")
	click(f.a, x+2, y)
	if f.a.mode == ModeProfileCard || len(f.opened) != 0 {
		t.Errorf("a click on #general: mode %v, opened %v", f.a.mode, f.opened)
	}
	if !reflect.DeepEqual(f.threads, []string{"1.0"}) {
		t.Errorf("a click on #general fetched threads %v, want [1.0]", f.threads)
	}
}

func TestClickOnLinkBesideMentionStillOpensIt(t *testing.T) {
	f := profileCardApp(t)
	x, y := screenPos(t, f.a, "clickme")
	click(f.a, x, y)
	if !reflect.DeepEqual(f.opened, []string{clickLinkURL}) || f.a.mode == ModeProfileCard {
		t.Errorf("a click on the link opened %v, mode %v; want [%s]", f.opened, f.a.mode, clickLinkURL)
	}
}

// openCard clicks dana's mention and checks the card opened.
func (f *profileCardFixture) openCard(t *testing.T) {
	t.Helper()
	x, y := screenPos(t, f.a, "@dana")
	click(f.a, x+2, y)
	if f.a.mode != ModeProfileCard {
		t.Fatalf("setup: the card did not open")
	}
}

func TestProfileCardKeys(t *testing.T) {
	t.Run("esc closes", func(t *testing.T) {
		f := profileCardApp(t)
		f.openCard(t)
		_, _ = f.a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if f.a.mode != ModeNormal || strings.Contains(screen(f.a), "Dana Kim") {
			t.Errorf("after esc: mode %v, card drawn %v", f.a.mode, strings.Contains(screen(f.a), "Dana Kim"))
		}
	})
	t.Run("m opens the DM", func(t *testing.T) {
		f := profileCardApp(t)
		f.openCard(t)
		_, cmd := f.a.Update(keyPress('m'))
		drainCmds(cmd)
		if !reflect.DeepEqual(f.dms, [][]string{{"U2"}}) || f.a.mode == ModeProfileCard {
			t.Errorf("after m: opened DMs %v, mode %v; want [[U2]]", f.dms, f.a.mode)
		}
		// The new-message picker's reducer takes Slack's answer: it
		// switches to the DM and focuses compose.
		_, cmd = f.a.Update(NewMessageOpenedMsg{RequestID: f.a.newMessageInFlightID, ChannelID: "D2", UserIDs: []string{"U2"}, AlreadyOpen: true})
		var selected []string
		for _, msg := range drainCmds(cmd) {
			if sel, ok := msg.(ChannelSelectedMsg); ok {
				selected = append(selected, sel.ID)
			}
		}
		if !reflect.DeepEqual(selected, []string{"D2"}) || f.a.mode != ModeInsert {
			t.Errorf("Slack's answer selected %v, mode %v; want [D2] in insert mode", selected, f.a.mode)
		}
	})
	t.Run("o opens the profile in the browser", func(t *testing.T) {
		f := profileCardApp(t)
		f.openCard(t)
		_, _ = f.a.Update(keyPress('o'))
		if !reflect.DeepEqual(f.opened, []string{"https://acme.slack.com/team/U2"}) || f.a.mode == ModeProfileCard {
			t.Errorf("after o: opened %v, mode %v", f.opened, f.a.mode)
		}
	})
	t.Run("other keys stay in the card", func(t *testing.T) {
		f := profileCardApp(t)
		f.openCard(t)
		_, _ = f.a.Update(keyPress('j'))
		_, _ = f.a.Update(keyPress('i'))
		if f.a.mode != ModeProfileCard {
			t.Errorf("j then i left the card for mode %v", f.a.mode)
		}
	})
}

func TestClickOutsideProfileCardClosesItAndNothingElse(t *testing.T) {
	f := profileCardApp(t)
	f.openCard(t)
	click(f.a, 1, 1)
	if f.a.mode != ModeNormal {
		t.Errorf("a click outside the card left mode %v", f.a.mode)
	}
	if len(f.threads) != 0 || len(f.opened) != 0 {
		t.Errorf("a click outside the card reached the panes: threads %v, opened %v", f.threads, f.opened)
	}

	f.openCard(t)
	x, y := screenPos(t, f.a, "@dana · Staff")
	click(f.a, x, y)
	if f.a.mode != ModeProfileCard {
		t.Errorf("a click inside the card closed it (mode %v)", f.a.mode)
	}
}

// The card shows what slk knows at once; a fetch that lands after the
// card closed, or for someone else, changes nothing.
func TestProfileCardKnownFirstAndStaleFetchDropped(t *testing.T) {
	f := profileCardApp(t)
	cmd := f.a.openProfileCard("U2")
	if got := f.a.profileCard.model.Lines(); !reflect.DeepEqual(got, []string{"@dana"}) {
		t.Errorf("before the fetch the card shows %q, want [@dana]", got)
	}
	_, _ = f.a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	for _, msg := range drainCmds(cmd) {
		_, _ = f.a.Update(msg)
	}
	if f.a.mode != ModeNormal || f.a.profileCard.model.Profile().RealName != "" {
		t.Errorf("a fetch after esc: mode %v, profile %+v", f.a.mode, f.a.profileCard.model.Profile())
	}

	cmd = f.a.openProfileCard("U2")
	_ = f.a.openProfileCard("U3")
	for _, msg := range drainCmds(cmd) {
		_, _ = f.a.Update(msg)
	}
	if p := f.a.profileCard.model.Profile(); p.UserID != "U3" || p.RealName != "" {
		t.Errorf("U2's fetch filled the card open for U3: %+v", p)
	}
}

// A failed fetch says so in a toast; the card stays open with what slk
// knew.
func TestProfileFetchFailureToastsAndKeepsTheCard(t *testing.T) {
	f := profileCardApp(t)
	f.profiles.err = errors.New("user_not_found")
	var toasts []string
	for _, msg := range drainCmds(f.a.openProfileCard("U2")) {
		_, cmd := f.a.Update(msg)
		for _, msg := range drainCmds(cmd) {
			if toast, ok := msg.(ToastMsg); ok {
				toasts = append(toasts, toast.Text)
			}
		}
	}
	if !reflect.DeepEqual(toasts, []string{"Failed to load profile"}) {
		t.Errorf("toasts %q, want [Failed to load profile]", toasts)
	}
	if f.a.mode != ModeProfileCard || !strings.Contains(screen(f.a), "@dana") {
		t.Errorf("after the failed fetch: mode %v, card:\n%s", f.a.mode, screen(f.a))
	}
}

// Without the workspace's domain a mention is no link, so a click on it
// does what it did.
func TestMentionWithoutDomainIsNoLink(t *testing.T) {
	f := profileCardApp(t)
	f.a.workspaceDomains = map[string]string{}
	f.a.setWorkspaceDomain()
	x, y := screenPos(t, f.a, "@dana")
	click(f.a, x+2, y)
	if f.a.mode == ModeProfileCard || !reflect.DeepEqual(f.threads, []string{"1.0"}) {
		t.Errorf("a click on an unlinked mention: mode %v, threads %v", f.a.mode, f.threads)
	}
}
