package ui

import (
	"context"
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/appshortcuts"
)

// appViewWait is how long the overlay waits for a shortcut's modal.
const appViewWait = 10 * time.Second

// AppViewMsg is a view_opened or view_updated event: an app's modal for
// this user, in workspace TeamID. View is the view object as Slack sent
// it.
type AppViewMsg struct {
	TeamID      string
	ClientToken string
	Updated     bool
	View        []byte
}

type appShortcutsListedMsg struct {
	teamID    string
	shortcuts []appshortcuts.Shortcut
	err       error
}

type appShortcutRunFailedMsg struct {
	token string
	err   error
}

type appShortcutTimeoutMsg struct{ token string }

type appShortcutSubmittedMsg struct {
	viewID string
	result appshortcuts.SubmitResult
	err    error
}

// appShortcutsState is the `.` overlay and what it acts on.
type appShortcutsState struct {
	svc   appshortcuts.Service
	model appshortcuts.Model
	// lists holds each workspace's shortcuts, loaded on the first `.`
	// and kept for the session.
	lists map[string][]appshortcuts.Shortcut

	// The message the overlay was opened on.
	teamID, channelID, messageTS string
	running                      appshortcuts.Shortcut
	// token is the client token of the latest run, the one the menu
	// waits on while it is Waiting.
	token string
	// abandoned holds the latest runs the overlay gave up on (timeout,
	// esc, failure), so a modal that still arrives for one is closed in
	// Slack.
	abandoned []abandonedRun
	// quitPrompt is set while ctrl+c's quit prompt sits over the
	// overlay; cancelling it hands the keys back to the overlay.
	quitPrompt bool

	// after is tea.Tick; tests replace it.
	after func(time.Duration, func(time.Time) tea.Msg) tea.Cmd
}

type abandonedRun struct{ token, teamID string }

// maxAbandoned is how many abandoned runs are remembered; the oldest
// goes first.
const maxAbandoned = 8

// SetAppShortcutService wires the `.` overlay to Slack.
func (a *App) SetAppShortcutService(s appshortcuts.Service) { a.appShortcuts.svc = s }

// clientToken is a fresh token in the web client's form, web-<unix ms>.
func (a *App) clientToken() string { return fmt.Sprintf("web-%d", a.now().UnixMilli()) }

// openAppShortcuts is the `.` key: the message shortcuts of the
// workspace's apps, for the selected message in the messages pane or
// the thread panel.
func (a *App) openAppShortcuts() tea.Cmd {
	s := &a.appShortcuts
	if s.svc == nil {
		return nil
	}
	channelID, ts, _, _, _, ok := a.selectedMessageContext()
	if !ok || channelID == "" || ts == "" {
		return nil
	}
	s.teamID, s.channelID, s.messageTS = a.activeTeamID, channelID, ts
	a.SetMode(ModeAppShortcuts)
	if list, ok := s.lists[s.teamID]; ok {
		s.model.OpenMenu(list)
		return nil
	}
	s.model.OpenLoading()
	svc, teamID := s.svc, s.teamID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		list, err := svc.List(ctx, teamID)
		return appShortcutsListedMsg{teamID: teamID, shortcuts: list, err: err}
	}
}

// closeAppShortcuts closes the overlay. A quit prompt open over it keeps
// the keys, and hands them to normal mode when cancelled.
func (a *App) closeAppShortcuts() {
	s := &a.appShortcuts
	if s.model.Waiting() {
		s.abandoned = append(s.abandoned, abandonedRun{s.token, s.teamID})
		s.abandoned = slices.Delete(s.abandoned, 0, max(len(s.abandoned)-maxAbandoned, 0))
	}
	s.model.Close()
	s.quitPrompt = false
	if a.mode == ModeAppShortcuts {
		a.SetMode(ModeNormal)
	}
}

// dismissAppShortcuts closes the overlay, and in Slack the modal open in
// it: esc, o on a form slk can't fill, and a workspace switch under it.
func (a *App) dismissAppShortcuts() tea.Cmd {
	s := &a.appShortcuts
	var closeView tea.Cmd
	if f := s.model.Form(); f != nil {
		closeView = a.closeAppViewCmd(s.teamID, f.ViewID(), f.RootViewID())
	}
	a.closeAppShortcuts()
	return closeView
}

// noteQuitPrompt records, as ctrl+c's quit prompt opens, whether it
// opens over the overlay.
func (a *App) noteQuitPrompt() { a.appShortcuts.quitPrompt = a.mode == ModeAppShortcuts }

// modeAfterConfirm is the mode a closed confirm prompt hands the keys
// to: the overlay's when the quit prompt sat over it, else normal mode.
func (a *App) modeAfterConfirm() Mode {
	if a.appShortcuts.quitPrompt {
		a.appShortcuts.quitPrompt = false
		return ModeAppShortcuts
	}
	return ModeNormal
}

// appShortcutsOverlay draws the overlay over screen when it is open.
func (a *App) appShortcutsOverlay(screen string) string {
	return a.appShortcuts.model.ViewOverlay(a.width, a.height, screen)
}

// handleAppShortcutsMode owns every key while the overlay is open, so
// nothing typed into the form reaches the rest of slk.
func handleAppShortcutsMode(a *App, msg tea.KeyMsg) tea.Cmd {
	s := &a.appShortcuts
	if f := s.model.Form(); f != nil {
		action, cmd := f.HandleKey(msg)
		switch action {
		case appshortcuts.ActionSubmit:
			return a.submitAppForm(f)
		case appshortcuts.ActionCancel:
			return a.dismissAppShortcuts()
		case appshortcuts.ActionOpenInBrowser:
			return tea.Batch(a.dismissAppShortcuts(), a.openMessageInBrowser(s.channelID, s.messageTS))
		}
		return cmd
	}
	k := normalizeFinderKey(msg)
	if k == "esc" {
		a.closeAppShortcuts()
		return nil
	}
	if !s.model.InMenu() {
		return nil
	}
	sc, ok := s.model.MenuKey(k, a.height)
	if !ok {
		return nil
	}
	s.running, s.token = sc, a.clientToken()
	s.model.Wait(sc.AppName)
	svc, teamID, channelID, ts, token := s.svc, s.teamID, s.channelID, s.messageTS, s.token
	after := s.after
	if after == nil {
		after = tea.Tick
	}
	return tea.Batch(
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := svc.Run(ctx, teamID, sc, channelID, ts, token); err != nil {
				return appShortcutRunFailedMsg{token: token, err: err}
			}
			return nil
		},
		after(appViewWait, func(time.Time) tea.Msg { return appShortcutTimeoutMsg{token: token} }),
	)
}

func (a *App) submitAppForm(f *appshortcuts.Form) tea.Cmd {
	f.SetSending()
	svc, teamID, viewID, token, state := a.appShortcuts.svc, a.appShortcuts.teamID, f.ViewID(), a.clientToken(), f.State()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		res, err := svc.Submit(ctx, teamID, viewID, token, state)
		return appShortcutSubmittedMsg{viewID: viewID, result: res, err: err}
	}
}

// closeAppViewCmd tells Slack the modal was dismissed. Its error is
// only logged: the form is gone either way.
func (a *App) closeAppViewCmd(teamID, viewID, rootID string) tea.Cmd {
	svc, token := a.appShortcuts.svc, a.clientToken()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := svc.Close(ctx, teamID, viewID, rootID, token); err != nil {
			debuglog.General("views.close %s: %v", viewID, err)
		}
		return nil
	}
}

// openMessageInBrowser opens the message's permalink in the browser,
// for a form slk can't fill.
func (a *App) openMessageInBrowser(channelID, ts string) tea.Cmd {
	messageSvc := a.messageSvc
	if messageSvc == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		url, err := messageSvc.Permalink(ctx, ids.ChannelID(channelID), ids.MessageTS(ts))
		if err != nil || url == "" {
			return ToastMsg{Text: "Could not get the message's link"}
		}
		return a.openURLCmd(url)()
	}
}

// reduceAppShortcuts handles the overlay's async results, and keeps the
// mouse and pastes away from the panes behind it while it is open.
var reduceAppShortcuts reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	s := &a.appShortcuts
	switch m := msg.(type) {
	case tea.MouseMsg:
		return nil, s.model.IsVisible()

	case tea.PasteMsg:
		f := s.model.Form()
		if f == nil || a.mode != ModeAppShortcuts {
			// No form, or the quit prompt holds the keys over it.
			return nil, s.model.IsVisible()
		}
		return f.Paste(m), true

	case appShortcutsListedMsg:
		if m.err != nil {
			debuglog.General("apps.actions.list %s: %v", m.teamID, m.err)
			if s.model.Loading() && s.teamID == m.teamID {
				s.model.SetLoadFailed()
			}
			return nil, true
		}
		if s.lists == nil {
			s.lists = map[string][]appshortcuts.Shortcut{}
		}
		s.lists[m.teamID] = m.shortcuts
		if s.model.Listing() && s.teamID == m.teamID {
			s.model.SetShortcuts(m.shortcuts)
		}
		return nil, true

	case appShortcutRunFailedMsg:
		if m.token == s.token && s.model.Waiting() {
			text := s.running.Name + " failed: " + m.err.Error()
			a.closeAppShortcuts()
			return func() tea.Msg { return ToastMsg{Text: text} }, true
		}
		return nil, true

	case appShortcutTimeoutMsg:
		if m.token == s.token && s.model.Waiting() {
			app := s.running.AppName
			a.closeAppShortcuts()
			return func() tea.Msg { return ToastMsg{Text: app + " did not open a form"} }, true
		}
		return nil, true

	case AppViewMsg:
		return a.applyAppView(m), true

	case appShortcutSubmittedMsg:
		f := s.model.Form()
		if f == nil || f.ViewID() != m.viewID {
			return nil, true
		}
		switch {
		case m.err != nil:
			f.SetSubmitError(m.err.Error(), nil)
		case m.result.Error != "" || len(m.result.FieldErrors) > 0:
			f.SetSubmitError(m.result.Error, m.result.FieldErrors)
		default:
			app := s.running.AppName
			a.closeAppShortcuts()
			return func() tea.Msg { return ToastMsg{Text: "Sent to " + app} }, true
		}
		return nil, true
	}
	return nil, false
}

// applyAppView opens the modal the waiting run asked for, matched by
// its client token, or refreshes the open form when its app updates it.
func (a *App) applyAppView(m AppViewMsg) tea.Cmd {
	s := &a.appShortcuts
	v, err := appshortcuts.ParseView(m.View)
	if m.Updated {
		f := s.model.Form()
		if err != nil || f == nil || m.TeamID != s.teamID || v.ID != f.ViewID() {
			return nil
		}
		nf := f.Update(v)
		s.model.OpenForm(nf)
		return nf.Focus()
	}
	if i := slices.Index(s.abandoned, abandonedRun{m.ClientToken, m.TeamID}); i >= 0 {
		s.abandoned = slices.Delete(s.abandoned, i, i+1)
		if err != nil {
			return nil
		}
		return a.closeAppViewCmd(m.TeamID, v.ID, v.RootViewID)
	}
	if !s.model.Waiting() || m.ClientToken != s.token || m.TeamID != s.teamID {
		return nil
	}
	if err != nil {
		debuglog.General("view_opened: %v", err)
		app := s.running.AppName
		a.closeAppShortcuts()
		return func() tea.Msg { return ToastMsg{Text: "Could not read the form from " + app} }
	}
	f := appshortcuts.NewForm(v)
	s.model.OpenForm(f)
	return f.Focus()
}
