package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/appshortcuts"
	"github.com/gammons/slk/internal/ui/messages"
)

type fakeShortcutRun struct {
	teamID, channelID, messageTS, token string
	shortcut                            appshortcuts.Shortcut
}

type fakeShortcutService struct {
	shortcuts []appshortcuts.Shortcut
	lists     int
	runs      []fakeShortcutRun
	runErr    error
	closes    [][4]string // teamID, viewID, rootViewID, token
	submits   [][4]string // teamID, viewID, token, state
	submitRes appshortcuts.SubmitResult
}

func (f *fakeShortcutService) List(_ context.Context, teamID string) ([]appshortcuts.Shortcut, error) {
	f.lists++
	return f.shortcuts, nil
}

func (f *fakeShortcutService) Run(_ context.Context, teamID string, s appshortcuts.Shortcut, channelID, messageTS, token string) error {
	f.runs = append(f.runs, fakeShortcutRun{teamID, channelID, messageTS, token, s})
	return f.runErr
}

func (f *fakeShortcutService) Close(_ context.Context, teamID, viewID, rootViewID, token string) error {
	f.closes = append(f.closes, [4]string{teamID, viewID, rootViewID, token})
	return nil
}

func (f *fakeShortcutService) Submit(_ context.Context, teamID, viewID, token, state string) (appshortcuts.SubmitResult, error) {
	f.submits = append(f.submits, [4]string{teamID, viewID, token, state})
	return f.submitRes, nil
}

var colonyAnnotate = appshortcuts.Shortcut{AppID: "A00000APP01", AppName: "Colony", ActionID: "10000000000001", Name: "Annotate"}

// shortcutApp is an App on a message in C1 of T1, wired to a fake
// service, with the clock at web-1700000000000 and the 10 s wait
// recorded instead of slept.
func shortcutApp(t *testing.T) (*App, *fakeShortcutService, *[]time.Duration) {
	t.Helper()
	a := newTestApp(t, withActiveChannel("C1"), withActiveTeam("T1"), withMessages(
		messages.MessageItem{TS: "1700000000.000100", UserName: "alice", Text: "please annotate"},
	))
	a.focusedPanel = PanelMessages
	svc := &fakeShortcutService{shortcuts: []appshortcuts.Shortcut{colonyAnnotate}}
	a.SetAppShortcutService(svc)
	a.SetNowFunc(func() time.Time { return time.UnixMilli(1700000000000) })
	var waits []time.Duration
	a.appShortcuts.after = func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
		waits = append(waits, d)
		return nil
	}
	return a, svc, &waits
}

// send runs msg through Update and then every command it returns,
// feeding each resulting message back in, and returns the toasts.
func send(a *App, msg tea.Msg) []string {
	var toasts []string
	queue := []tea.Msg{msg}
	for len(queue) > 0 {
		m := queue[0]
		queue = queue[1:]
		if tm, ok := m.(ToastMsg); ok {
			toasts = append(toasts, tm.Text)
			continue
		}
		_, cmd := a.Update(m)
		queue = append(queue, runCmds(cmd)...)
	}
	return toasts
}

func runCmds(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch m := cmd().(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range m {
			out = append(out, runCmds(c)...)
		}
		return out
	default:
		return []tea.Msg{m}
	}
}

func shortcutKey(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "ctrl+s":
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: []rune(k)[0], Text: k}
}

func typeKeys(a *App, s string) {
	for _, r := range s {
		send(a, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// capturedViewOpened is the view_opened event captured from Colony's
// Annotate shortcut, ids replaced; its client_token is web-1700000000000.
func capturedViewOpened(t *testing.T, token string) AppViewMsg {
	t.Helper()
	raw, err := os.ReadFile("../slack/testdata/ws_view_opened.json")
	if err != nil {
		t.Fatal(err)
	}
	var evt struct {
		View json.RawMessage `json:"view"`
	}
	if err := json.Unmarshal(raw, &evt); err != nil {
		t.Fatal(err)
	}
	return AppViewMsg{TeamID: "T1", ClientToken: token, View: evt.View}
}

func overlayText(a *App) string { return ansi.Strip(a.appShortcutsOverlay("")) }

func TestAppShortcuts_MenuLoadsOnceAndRuns(t *testing.T) {
	a, svc, waits := shortcutApp(t)

	send(a, shortcutKey("."))
	if a.mode != ModeAppShortcuts || !strings.Contains(overlayText(a), "Annotate") || !strings.Contains(overlayText(a), "Colony") {
		t.Fatalf("mode %v, overlay:\n%s", a.mode, overlayText(a))
	}
	send(a, shortcutKey("esc"))
	send(a, shortcutKey("."))
	if svc.lists != 1 {
		t.Errorf("apps.actions.list called %d times, want once per workspace", svc.lists)
	}

	send(a, shortcutKey("enter"))
	want := fakeShortcutRun{"T1", "C1", "1700000000.000100", "web-1700000000000", colonyAnnotate}
	if len(svc.runs) != 1 || svc.runs[0] != want {
		t.Fatalf("runs = %+v, want %+v", svc.runs, want)
	}
	if len(*waits) != 1 || (*waits)[0] != 10*time.Second {
		t.Errorf("waits = %v, want one of 10s", *waits)
	}
	if !strings.Contains(overlayText(a), "Waiting for Colony…") {
		t.Errorf("overlay:\n%s", overlayText(a))
	}

	// Another run's modal is not this one.
	send(a, capturedViewOpened(t, "web-1"))
	if a.appShortcuts.model.Form() != nil {
		t.Fatal("a view_opened with another client token opened the form")
	}
	send(a, capturedViewOpened(t, "web-1700000000000"))
	if a.appShortcuts.model.Form() == nil || !strings.Contains(overlayText(a), "Annotate Claude's work") {
		t.Fatalf("form not open:\n%s", overlayText(a))
	}
}

func TestAppShortcuts_LoadingAndEmpty(t *testing.T) {
	a, svc, _ := shortcutApp(t)
	svc.shortcuts = nil
	// Run the open key alone, without its List command, to see the
	// loading state.
	a.Update(shortcutKey("."))
	if !strings.Contains(overlayText(a), "Loading shortcuts…") {
		t.Errorf("overlay:\n%s", overlayText(a))
	}
	send(a, appShortcutsListedMsg{teamID: "T1"})
	if !strings.Contains(overlayText(a), "No app shortcuts in this workspace") {
		t.Errorf("overlay:\n%s", overlayText(a))
	}
}

func TestAppShortcuts_ThreadReply(t *testing.T) {
	a, svc, _ := shortcutApp(t)
	parent := messages.MessageItem{TS: "1700000000.000100", Text: "parent"}
	reply := messages.MessageItem{TS: "1700000000.000200", Text: "reply", ThreadTS: "1700000000.000100"}
	a.threadPanel.SetThread(parent, []messages.MessageItem{reply}, "C9", parent.TS)
	a.threadVisible = true
	a.focusedPanel = PanelThread
	send(a, shortcutKey("."))
	send(a, shortcutKey("enter"))
	if len(svc.runs) != 1 || svc.runs[0].channelID != "C9" || svc.runs[0].messageTS != a.threadPanel.SelectedReply().TS {
		t.Errorf("runs = %+v, want the selected reply in C9", svc.runs)
	}
}

func TestAppShortcuts_Timeout(t *testing.T) {
	a, _, _ := shortcutApp(t)
	var fire tea.Cmd
	a.appShortcuts.after = func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
		fire = func() tea.Msg { return fn(time.Time{}) }
		return nil
	}
	send(a, shortcutKey("."))
	send(a, shortcutKey("enter"))
	toasts := send(a, fire())
	if a.appShortcuts.model.IsVisible() || a.mode != ModeNormal {
		t.Errorf("overlay still open after the wait, mode %v", a.mode)
	}
	if len(toasts) != 1 || toasts[0] != "Colony did not open a form" {
		t.Errorf("toasts = %q", toasts)
	}
	// The modal arriving late opens nothing.
	send(a, capturedViewOpened(t, "web-1700000000000"))
	if a.appShortcuts.model.IsVisible() {
		t.Error("a late view_opened reopened the overlay")
	}
}

func TestAppShortcuts_TimeoutAfterFormIsNoop(t *testing.T) {
	a, _, _ := shortcutApp(t)
	send(a, shortcutKey("."))
	send(a, shortcutKey("enter"))
	send(a, capturedViewOpened(t, "web-1700000000000"))
	send(a, appShortcutTimeoutMsg{token: "web-1700000000000"})
	if a.appShortcuts.model.Form() == nil {
		t.Error("the stale timeout closed the open form")
	}
}

func TestAppShortcuts_RunFails(t *testing.T) {
	a, svc, _ := shortcutApp(t)
	svc.runErr = errString("not_in_channel")
	send(a, shortcutKey("."))
	toasts := send(a, shortcutKey("enter"))
	if a.appShortcuts.model.IsVisible() || len(toasts) != 1 || toasts[0] != "Annotate failed: not_in_channel" {
		t.Errorf("visible %v toasts %q", a.appShortcuts.model.IsVisible(), toasts)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

// openAnnotateForm brings up the captured form.
func openAnnotateForm(t *testing.T) (*App, *fakeShortcutService) {
	a, svc, _ := shortcutApp(t)
	send(a, shortcutKey("."))
	send(a, shortcutKey("enter"))
	send(a, capturedViewOpened(t, "web-1700000000000"))
	return a, svc
}

func TestAppShortcuts_FormKeysStayInForm(t *testing.T) {
	a, svc := openAnnotateForm(t)
	// Keys that mean something to slk elsewhere: q, j, i, ., :, ?
	typeKeys(a, "qji.:?")
	send(a, shortcutKey("enter"))
	typeKeys(a, "ok")
	if a.mode != ModeAppShortcuts || a.threadVisible {
		t.Fatalf("mode %v", a.mode)
	}
	send(a, shortcutKey("tab"))
	send(a, shortcutKey("j"))
	send(a, shortcutKey("space"))
	a.SetNowFunc(func() time.Time { return time.UnixMilli(1700000005000) })
	toasts := send(a, shortcutKey("ctrl+s"))

	if len(svc.submits) != 1 {
		t.Fatalf("submits = %v", svc.submits)
	}
	got := svc.submits[0]
	want := [4]string{"T1", "V00000VIEW1", "web-1700000005000",
		`{"values":{"note":{"text":{"type":"plain_text_input","value":"qji.:?\nok"}},"sign":{"pick":{"type":"radio_buttons","selected_option":{"text":{"type":"plain_text","text":"Negative","emoji":true},"value":"negative"}}}}}`}
	if got != want {
		t.Errorf("submit\n got %v\nwant %v", got, want)
	}
	if a.appShortcuts.model.IsVisible() || a.mode != ModeNormal || len(toasts) != 1 || toasts[0] != "Sent to Colony" {
		t.Errorf("after ok: visible %v mode %v toasts %q", a.appShortcuts.model.IsVisible(), a.mode, toasts)
	}
}

func TestAppShortcuts_SubmitRefusedKeepsForm(t *testing.T) {
	a, svc := openAnnotateForm(t)
	svc.submitRes = appshortcuts.SubmitResult{FieldErrors: map[string]string{"note": "Say more"}}
	typeKeys(a, "x")
	send(a, shortcutKey("tab"))
	send(a, shortcutKey("space"))
	send(a, shortcutKey("ctrl+s"))
	if a.appShortcuts.model.Form() == nil || !strings.Contains(overlayText(a), "! Say more") {
		t.Fatalf("overlay:\n%s", overlayText(a))
	}
	svc.submitRes = appshortcuts.SubmitResult{Error: "view_not_found"}
	send(a, shortcutKey("ctrl+s"))
	if !strings.Contains(overlayText(a), "! view_not_found") {
		t.Errorf("overlay:\n%s", overlayText(a))
	}
}

func TestAppShortcuts_InvalidFormSendsNothing(t *testing.T) {
	a, svc := openAnnotateForm(t)
	send(a, shortcutKey("ctrl+s"))
	if len(svc.submits) != 0 || strings.Count(overlayText(a), "! Required") != 2 {
		t.Errorf("submits %v overlay:\n%s", svc.submits, overlayText(a))
	}
}

func TestAppShortcuts_EscClosesView(t *testing.T) {
	a, svc := openAnnotateForm(t)
	a.SetNowFunc(func() time.Time { return time.UnixMilli(1700000009000) })
	send(a, shortcutKey("esc"))
	want := [4]string{"T1", "V00000VIEW1", "V00000VIEW1", "web-1700000009000"}
	if len(svc.closes) != 1 || svc.closes[0] != want {
		t.Errorf("closes = %v, want %v", svc.closes, want)
	}
	if a.appShortcuts.model.IsVisible() || a.mode != ModeNormal {
		t.Error("form still open")
	}
}

func TestAppShortcuts_ViewUpdatedReplacesForm(t *testing.T) {
	a, _ := openAnnotateForm(t)
	send(a, AppViewMsg{TeamID: "T1", Updated: true, View: []byte(`{"id":"V00000VIEW1","title":{"type":"plain_text","text":"Thanks"},"blocks":[]}`)})
	if !strings.Contains(overlayText(a), "Thanks") {
		t.Errorf("overlay:\n%s", overlayText(a))
	}
	send(a, AppViewMsg{TeamID: "T1", Updated: true, View: []byte(`{"id":"V-other","title":{"type":"plain_text","text":"Other"},"blocks":[]}`)})
	if strings.Contains(overlayText(a), "Other") {
		t.Error("another view's update replaced the form")
	}
}

func TestAppShortcuts_MouseAndPasteStayInOverlay(t *testing.T) {
	a, _ := openAnnotateForm(t)
	send(a, tea.MouseClickMsg{X: 1, Y: 1, Button: tea.MouseLeft})
	send(a, tea.PasteMsg{Content: "pasted"})
	if a.appShortcuts.model.Form() == nil || !strings.Contains(a.appShortcuts.model.Form().State(), `"value":"pasted"`) {
		t.Errorf("state %s", a.appShortcuts.model.Form().State())
	}
}

// ctrl+c's quit prompt over the form, cancelled, hands the keys back.
func TestAppShortcuts_QuitPromptCancelReturnsToForm(t *testing.T) {
	a, _ := openAnnotateForm(t)
	send(a, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if a.mode != ModeConfirm {
		t.Fatalf("mode %v, want the quit prompt", a.mode)
	}
	send(a, shortcutKey("esc"))
	typeKeys(a, "q")
	if a.mode != ModeAppShortcuts || !strings.Contains(a.appShortcuts.model.Form().State(), `"value":"q"`) {
		t.Errorf("mode %v state %s", a.mode, a.appShortcuts.model.Form().State())
	}
}

// The overlay closing under ctrl+c's quit prompt (the wait ran out)
// leaves the keys with the prompt, and cancelling it lands in normal
// mode.
func TestAppShortcuts_CloseUnderQuitPrompt(t *testing.T) {
	a, _, _ := shortcutApp(t)
	send(a, shortcutKey("."))
	send(a, shortcutKey("enter"))
	send(a, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	send(a, appShortcutTimeoutMsg{token: "web-1700000000000"})
	if a.appShortcuts.model.IsVisible() || a.mode != ModeConfirm {
		t.Fatalf("visible %v mode %v, want the quit prompt alone", a.appShortcuts.model.IsVisible(), a.mode)
	}
	send(a, shortcutKey("esc"))
	if a.mode != ModeNormal {
		t.Errorf("mode %v after cancel, want normal", a.mode)
	}
}

// Only the latest abandoned runs are remembered: a modal that arrives
// for an older one is left alone.
func TestAppShortcuts_AbandonedRunsAreCapped(t *testing.T) {
	a, svc, _ := shortcutApp(t)
	token := func(i int) string { return fmt.Sprintf("web-%d", 1700000000000+i) }
	for i := range maxAbandoned + 1 {
		a.SetNowFunc(func() time.Time { return time.UnixMilli(1700000000000 + int64(i)) })
		send(a, shortcutKey("."))
		send(a, shortcutKey("enter"))
		send(a, shortcutKey("esc"))
	}
	send(a, capturedViewOpened(t, token(0)))
	if len(svc.closes) != 0 {
		t.Errorf("closes = %v, want the oldest run forgotten", svc.closes)
	}
	send(a, capturedViewOpened(t, token(maxAbandoned)))
	if len(svc.closes) != 1 {
		t.Errorf("closes = %v, want the latest run's modal closed", svc.closes)
	}
}

// A paste under ctrl+c's quit prompt does not reach the form below it.
func TestAppShortcuts_PasteUnderQuitPromptSkipsForm(t *testing.T) {
	a, _ := openAnnotateForm(t)
	send(a, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	send(a, tea.PasteMsg{Content: "pasted"})
	if strings.Contains(a.appShortcuts.model.Form().State(), "pasted") {
		t.Errorf("the paste reached the form: %s", a.appShortcuts.model.Form().State())
	}
}

// A list that fails while a second request for the same workspace is
// pending does not keep the menu on the failure once the second lands.
func TestAppShortcuts_ListSuccessAfterFailure(t *testing.T) {
	a, _, _ := shortcutApp(t)
	a.Update(shortcutKey("."))
	send(a, appShortcutsListedMsg{teamID: "T1", err: errString("ratelimited")})
	send(a, appShortcutsListedMsg{teamID: "T1", shortcuts: []appshortcuts.Shortcut{colonyAnnotate}})
	if !a.appShortcuts.model.InMenu() || !strings.Contains(overlayText(a), "Annotate") {
		t.Errorf("overlay:\n%s", overlayText(a))
	}
}

// A modal that arrives after the overlay gave up on it is closed in
// Slack, not left open there.
func TestAppShortcuts_LateViewIsClosed(t *testing.T) {
	for name, giveUp := range map[string]func(a *App){
		"timeout": func(a *App) { send(a, appShortcutTimeoutMsg{token: "web-1700000000000"}) },
		"esc":     func(a *App) { send(a, shortcutKey("esc")) },
	} {
		t.Run(name, func(t *testing.T) {
			a, svc, _ := shortcutApp(t)
			send(a, shortcutKey("."))
			send(a, shortcutKey("enter"))
			giveUp(a)
			a.SetNowFunc(func() time.Time { return time.UnixMilli(1700000003000) })
			send(a, capturedViewOpened(t, "web-1700000000000"))
			want := [4]string{"T1", "V00000VIEW1", "V00000VIEW1", "web-1700000003000"}
			if len(svc.closes) != 1 || svc.closes[0] != want {
				t.Errorf("closes = %v, want %v", svc.closes, want)
			}
			if a.appShortcuts.model.IsVisible() {
				t.Error("the late modal opened the overlay")
			}
			// Only once.
			send(a, capturedViewOpened(t, "web-1700000000000"))
			if len(svc.closes) != 1 {
				t.Errorf("closes = %v", svc.closes)
			}
		})
	}
}

// ctrl+s, then the app updates the view before Slack answers, then
// ctrl+s again: one submit, and the typed note survives the update.
func TestAppShortcuts_ViewUpdatedDuringSubmit(t *testing.T) {
	a, svc := openAnnotateForm(t)
	typeKeys(a, "note")
	send(a, shortcutKey("tab"))
	send(a, shortcutKey("space"))
	_, submit := a.Update(shortcutKey("ctrl+s"))
	upd := capturedViewOpened(t, "")
	upd.Updated = true
	send(a, upd)
	if f := a.appShortcuts.model.Form(); f == nil || !strings.Contains(f.State(), `"value":"note"`) {
		t.Fatal("the update dropped the typed note")
	}
	_, again := a.Update(shortcutKey("ctrl+s"))
	runCmds(submit)
	runCmds(again)
	if len(svc.submits) != 1 {
		t.Errorf("submits = %d, want 1", len(svc.submits))
	}
	if !strings.Contains(svc.submits[0][3], `"value":"note"`) {
		t.Errorf("state %s", svc.submits[0][3])
	}
}

// A workspace switch that isn't a key closes the overlay and the open
// modal, and the next key acts in the new workspace.
func TestAppShortcuts_WorkspaceSwitchClosesForm(t *testing.T) {
	a, svc := openAnnotateForm(t)
	send(a, WorkspaceSwitchedMsg{TeamID: "T2"})
	if a.appShortcuts.model.IsVisible() || a.mode != ModeNormal {
		t.Fatalf("visible %v mode %v", a.appShortcuts.model.IsVisible(), a.mode)
	}
	if len(svc.closes) != 1 || svc.closes[0][0] != "T1" || svc.closes[0][1] != "V00000VIEW1" {
		t.Errorf("closes = %v", svc.closes)
	}
	send(a, shortcutKey("j"))
	if a.mode != ModeNormal {
		t.Errorf("mode %v after j, want normal", a.mode)
	}
}
