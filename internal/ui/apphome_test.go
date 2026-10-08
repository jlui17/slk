package ui

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/apphome"
	"github.com/gammons/slk/internal/ui/appshortcuts"
	"github.com/gammons/slk/internal/ui/messages"
)

type fakeAppHomeService struct {
	home   apphome.Home
	loads  int
	opened int
	clicks []apphome.Click
}

func (f *fakeAppHomeService) Load(context.Context, string, string) (apphome.Home, error) {
	f.loads++
	return f.home, nil
}

func (f *fakeAppHomeService) Opened(context.Context, string, string, string) error {
	f.opened++
	return nil
}

func (f *fakeAppHomeService) Click(_ context.Context, _ string, c apphome.Click) error {
	f.clicks = append(f.clicks, c)
	return nil
}

// colonyHomeView is Colony's captured Home view object.
func colonyHomeView(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../slack/testdata/conversations_info_app_home.json")
	if err != nil {
		t.Fatal(err)
	}
	var resp struct {
		HomeView json.RawMessage `json:"home_view"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	return resp.HomeView
}

// appHomeApp is an App in T1 (example.slack.com) whose Colony DM has
// Colony's Home, with the clock at web-1700000000000, the 10 s wait
// recorded instead of slept, and links recorded instead of opened.
func appHomeApp(t *testing.T) (*App, *fakeAppHomeService, *[]string) {
	t.Helper()
	a := newTestApp(t, withActiveTeam("T1"), withChannelService(core.ChannelServiceFuncs{
		ReadCache: func(ids.ChannelID) []messages.MessageItem {
			return []messages.MessageItem{{TS: "1700000000.000100", UserName: "Colony", Text: "Your annotation was sent."}}
		},
		Lookup: func(id ids.ChannelID) (string, string, bool) {
			return "annotation-review", "channel", id == "C0C6ZMJJT1U"
		},
	}))
	a.workspaceDomains["T1"] = "example"
	a.SetNowFunc(func() time.Time { return time.UnixMilli(1700000000000) })
	svc := &fakeAppHomeService{home: apphome.Home{AppID: "A00000APP01", TeamID: "T00000TEAM1", Enabled: true, MessagesTab: true, View: colonyHomeView(t)}}
	a.SetAppHomeService(svc)
	a.appHome.after = func(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }
	var opened []string
	a.browserOpener = func(url string) tea.Cmd {
		opened = append(opened, "browser "+url)
		return nil
	}
	return a, svc, &opened
}

func selectColony(a *App) []string {
	return send(a, ChannelSelectedMsg{ID: "D1", Name: "Colony", Type: "app"})
}

func homeKey(a *App, k string) []string { return send(a, shortcutKey(k)) }

func TestAppHome_AppRowOpensItsHomeTab(t *testing.T) {
	a, svc, _ := appHomeApp(t)
	selectColony(a)
	if a.view != ViewAppHome || svc.loads != 1 || svc.opened != 1 {
		t.Fatalf("view %v, loads %d, opened %d", a.view, svc.loads, svc.opened)
	}
	out := ansi.Strip(a.View().Content)
	for _, want := range []string{"▣ Colony", "Home   Messages", "[ Open review ]", "Needs a review · 89", "o open  O open in browser  m messages"} {
		if !strings.Contains(out, want) {
			t.Errorf("Home tab lacks %q:\n%s", want, out)
		}
	}

	// m shows the DM with the tab row; m again, the Home tab, which
	// tells the app again.
	homeKey(a, "m")
	out = ansi.Strip(a.View().Content)
	if a.view != ViewChannels || !strings.Contains(out, "Your annotation was sent.") || !strings.Contains(out, "m home") {
		t.Fatalf("Messages tab: view %v\n%s", a.view, out)
	}
	homeKey(a, "m")
	if a.view != ViewAppHome || svc.opened != 2 {
		t.Errorf("back on Home: view %v, opened %d", a.view, svc.opened)
	}

	// Another channel closes App Home and its tab row.
	send(a, ChannelSelectedMsg{ID: "C2", Name: "eng", Type: "channel"})
	if a.view != ViewChannels || a.appHome.channelID != "" || strings.Contains(ansi.Strip(a.View().Content), "m home") {
		t.Errorf("after leaving: view %v, open %q", a.view, a.appHome.channelID)
	}
}

func TestAppHome_AppWithoutHomeOpensItsMessages(t *testing.T) {
	a, svc, _ := appHomeApp(t)
	svc.home = apphome.Home{AppID: "A2", Enabled: false}
	selectColony(a)
	if a.view != ViewChannels || strings.Contains(ansi.Strip(a.View().Content), "Home   Messages") {
		t.Fatalf("view %v", a.view)
	}
	homeKey(a, "m") // no Home tab to go to
	if a.view != ViewChannels {
		t.Errorf("m opened a Home the app lacks")
	}
	// Known now: the next open goes straight to the DM.
	a.Update(ChannelSelectedMsg{ID: "D1", Name: "Colony", Type: "app"})
	if a.view != ViewChannels {
		t.Errorf("second open showed the Home tab's loading pass")
	}
}

// O on Start review clicks it with the web client's fields, then opens
// the review the redraw adds in a herdr tab.
func TestAppHome_ONonLinkButtonOpensTheNewLinkInAHerdrTab(t *testing.T) {
	a, svc, opened := appHomeApp(t)
	var tabs []string
	a.herdrTabOpener = func(url, label string, focus bool) error {
		tabs = append(tabs, label+" "+url)
		return nil
	}
	selectColony(a)
	homeKey(a, "j")
	homeKey(a, "j") // first Start review
	homeKey(a, "O")
	if len(svc.clicks) != 1 {
		t.Fatalf("%d clicks", len(svc.clicks))
	}
	c := svc.clicks[0]
	if c.ViewID != "V00000HOME1" || c.BotID != "B00000BOT01" || c.AppID != "A00000APP01" || c.TeamID != "T00000TEAM1" || c.ClientToken != "web-1700000000000" {
		t.Errorf("click = %+v", c)
	}
	if !strings.Contains(string(c.Action), `"action_id":"start_review"`) || !strings.Contains(string(c.Action), `"block_id":"start_review/C0BCG30UGEP/1790723734.808739"`) {
		t.Errorf("action = %s", c.Action)
	}
	if !strings.HasPrefix(string(c.State), `{"values":{"WbJB6":`) {
		t.Errorf("state = %s", c.State)
	}
	if out := ansi.Strip(a.View().Content); !strings.Contains(out, "Waiting for Colony…") || !strings.Contains(out, "esc stop waiting") {
		t.Errorf("no wait shown:\n%s", out)
	}

	newURL := "https://example.slack.com/archives/C0C6ZMJJT1U/p1791999999000001"
	redraw := strings.Replace(string(colonyHomeView(t)), "https://example.slack.com/archives/C0C6ZMJJT1U/p1791343906925689", newURL, 1)
	send(a, AppViewMsg{TeamID: "T1", Updated: true, View: []byte(redraw)})
	if len(tabs) != 1 || tabs[0] != "annotation-review "+newURL || len(*opened) != 0 {
		t.Errorf("tabs %v, browser %v", tabs, *opened)
	}
}

// enter on Start review sends the click and nothing else: no wait, no
// timer, and the Home keeps its keys.
func TestAppHome_EnterOnAButtonOnlyClicks(t *testing.T) {
	a, svc, _ := appHomeApp(t)
	ticks := 0
	a.appHome.after = func(time.Duration, func(time.Time) tea.Msg) tea.Cmd {
		ticks++
		return nil
	}
	selectColony(a)
	homeKey(a, "j")
	homeKey(a, "j") // first Start review
	ticks = 0
	homeKey(a, "enter")
	if len(svc.clicks) != 1 || ticks != 0 {
		t.Fatalf("%d clicks, %d timers", len(svc.clicks), ticks)
	}
	if out := ansi.Strip(a.View().Content); strings.Contains(out, "Waiting for Colony") {
		t.Errorf("enter shows a wait:\n%s", out)
	}
	homeKey(a, "m")
	if a.view != ViewChannels {
		t.Errorf("m after enter: view %v", a.view)
	}
}

// A redraw that adds two links after o ends the wait with a warning,
// and opens neither.
func TestAppHome_ORedrawWithSeveralLinksWarns(t *testing.T) {
	a, _, opened := appHomeApp(t)
	selectColony(a)
	homeKey(a, "j")
	homeKey(a, "j") // first Start review
	homeKey(a, "o")

	var view map[string]any
	if err := json.Unmarshal(colonyHomeView(t), &view); err != nil {
		t.Fatal(err)
	}
	blocks := view["blocks"].([]any)
	for i, b := range blocks {
		if b.(map[string]any)["block_id"] != "open_review/C0BS6HBB3R6/1788218799.815259" {
			continue
		}
		raw, _ := json.Marshal(b)
		var second map[string]any
		_ = json.Unmarshal(raw, &second)
		b.(map[string]any)["accessory"].(map[string]any)["url"] = "https://example.slack.com/archives/C0C6ZMJJT1U/p1791999999000001"
		second["block_id"] = "open_review/second"
		second["accessory"].(map[string]any)["url"] = "https://example.slack.com/archives/C0C6ZMJJT1U/p1791999999000002"
		blocks = append(blocks[:i+1], append([]any{second}, blocks[i+1:]...)...)
		break
	}
	view["blocks"] = blocks
	redraw, _ := json.Marshal(view)
	_, cmd := a.Update(AppViewMsg{TeamID: "T1", Updated: true, View: redraw})
	for _, m := range runCmds(cmd) {
		a.Update(m) // the toast, without running the tick that clears it
	}
	out := ansi.Strip(a.View().Content)
	if !strings.Contains(out, "! Colony added 2 links: pick one on the Home") || strings.Contains(out, "Waiting for Colony") || len(*opened) != 0 {
		t.Errorf("opened %v after the redraw:\n%s", *opened, out)
	}
}

func TestAppHome_NoAnswerWarns(t *testing.T) {
	a, _, _ := appHomeApp(t)
	selectColony(a)
	homeKey(a, "j")
	homeKey(a, "j")
	homeKey(a, "o")
	_, cmd := a.Update(appHomeWaitTimeoutMsg{visit: a.appHome.visit, waitID: 1})
	for _, m := range runCmds(cmd) {
		a.Update(m) // the toast, without running the tick that clears it
	}
	if out := ansi.Strip(a.View().Content); !strings.Contains(out, "! Colony did not answer") || strings.Contains(out, "Waiting for Colony") {
		t.Errorf("after the wait ran out:\n%s", out)
	}
}

// A timeout or click failure from an earlier visit to the Home can't
// end a wait started on a later one, though both count wait ids from 1.
func TestAppHome_EarlierVisitCantEndTheWait(t *testing.T) {
	a, _, _ := appHomeApp(t)
	startWait := func() {
		selectColony(a)
		homeKey(a, "j")
		homeKey(a, "j")
		homeKey(a, "o")
	}
	startWait()
	old := a.appHome.visit
	send(a, ChannelSelectedMsg{ID: "C2", Name: "eng", Type: "channel"})
	startWait()
	a.Update(appHomeWaitTimeoutMsg{visit: old, waitID: 1})
	a.Update(appHomeClickFailedMsg{visit: old, waitID: 1, err: errors.New("boom")})
	if out := ansi.Strip(a.View().Content); !strings.Contains(out, "Waiting for Colony…") {
		t.Errorf("an earlier visit's results ended the wait:\n%s", out)
	}
}

func TestAppHome_ConfirmThenSelect(t *testing.T) {
	a, svc, _ := appHomeApp(t)
	selectColony(a)
	for range 4 {
		homeKey(a, "j") // Review the next 3
	}
	homeKey(a, "enter")
	out := ansi.Strip(a.View().Content)
	if len(svc.clicks) != 0 || !strings.Contains(out, "Start 3 reviews?") || !strings.Contains(out, "enter Start  esc Cancel") {
		t.Fatalf("confirm box: %d clicks\n%s", len(svc.clicks), out)
	}
	homeKey(a, "esc")
	if len(svc.clicks) != 0 || a.appHome.model.BoxOpen() {
		t.Fatalf("esc: %d clicks, box open %v", len(svc.clicks), a.appHome.model.BoxOpen())
	}

	for range 3 {
		homeKey(a, "k") // the sort select
	}
	homeKey(a, "enter")
	homeKey(a, "j")
	homeKey(a, "enter")
	if len(svc.clicks) != 1 || !strings.Contains(string(svc.clicks[0].Action), `"value":"oldest"`) {
		t.Fatalf("clicks = %+v", svc.clicks)
	}
}

func TestAppHome_EscFocusesTheSidebar(t *testing.T) {
	a, _, _ := appHomeApp(t)
	selectColony(a)
	homeKey(a, "esc")
	if a.focusedPanel != PanelSidebar {
		t.Errorf("focus = %v", a.focusedPanel)
	}
}

// A permalink into the app DM opens its Messages tab, where the linked
// message is.
func TestAppHome_PermalinkOpensMessages(t *testing.T) {
	a, _, _ := appHomeApp(t)
	a.pendingLinkNav = &pendingLinkNav{channelID: "D1", messageTS: "1700000000.000100"}
	a.Update(ChannelSelectedMsg{ID: "D1", Name: "Colony", Type: "app"})
	if a.view != ViewChannels {
		t.Errorf("view = %v", a.view)
	}
}

// A switch to another workspace leaves App Home and its tab row.
func TestAppHome_WorkspaceSwitchCloses(t *testing.T) {
	a, _, _ := appHomeApp(t)
	selectColony(a)
	homeKey(a, "m")
	a.Update(WorkspaceSwitchedMsg{TeamID: "T2", TeamName: "Other"})
	if a.view != ViewChannels || a.appHome.channelID != "" || strings.Contains(ansi.Strip(a.View().Content), "m home") {
		t.Errorf("after the switch: view %v, open %q", a.view, a.appHome.channelID)
	}
}

// An app with a Home tab it has not published yet (the Claude app before
// it answers the open event) shows an empty Home, and the view it then
// publishes on the socket.
func TestAppHome_FirstPublishArrivesOnTheSocket(t *testing.T) {
	a, svc, _ := appHomeApp(t)
	svc.home.View = nil
	selectColony(a)
	out := ansi.Strip(a.View().Content)
	if a.view != ViewAppHome || strings.Contains(out, "Annotations") || !strings.Contains(out, "m messages") || svc.opened != 1 {
		t.Fatalf("empty Home: view %v, opened %d\n%s", a.view, svc.opened, out)
	}
	send(a, AppViewMsg{TeamID: "T1", View: colonyHomeView(t)})
	if out := ansi.Strip(a.View().Content); !strings.Contains(out, "Annotations") || !strings.Contains(out, "[ Open review ]") {
		t.Errorf("published view not shown:\n%s", out)
	}
}

// A modal the app opens in answer to a Home click (a view_opened echoing
// the click's client_token) opens in the app shortcuts' form, whose
// submit and esc work as for a shortcut's; a view_opened echoing no Home
// click opens nothing.
func TestAppHome_ModalFromAHomeClickOpensTheForm(t *testing.T) {
	a, homeSvc, _ := appHomeApp(t)
	formSvc := &fakeShortcutService{}
	a.SetAppShortcutService(formSvc)
	selectColony(a)
	homeKey(a, "j")
	homeKey(a, "j")
	homeKey(a, "enter") // Start review, client token web-1700000000000
	if len(homeSvc.clicks) != 1 {
		t.Fatalf("%d clicks", len(homeSvc.clicks))
	}

	send(a, capturedViewOpened(t, "web-1699999999999"))
	if a.appShortcuts.model.IsVisible() {
		t.Fatal("a view_opened for no Home click opened a form")
	}

	send(a, capturedViewOpened(t, "web-1700000000000"))
	if a.mode != ModeAppShortcuts || a.appShortcuts.model.Form() == nil {
		t.Fatalf("mode %v, overlay:\n%s", a.mode, overlayText(a))
	}
	typeKeys(a, "ok")
	send(a, shortcutKey("tab"))
	send(a, shortcutKey("space"))
	toasts := send(a, shortcutKey("ctrl+s"))
	if len(formSvc.submits) != 1 || formSvc.submits[0][0] != "T1" || formSvc.submits[0][1] != "V00000VIEW1" {
		t.Fatalf("submits = %v", formSvc.submits)
	}
	if a.appShortcuts.model.IsVisible() || a.mode != ModeNormal || len(toasts) != 1 || toasts[0] != "Sent to Colony" {
		t.Errorf("after the submit: visible %v mode %v toasts %q", a.appShortcuts.model.IsVisible(), a.mode, toasts)
	}

	// The same token again is no second modal: each click opens one.
	send(a, capturedViewOpened(t, "web-1700000000000"))
	if a.appShortcuts.model.IsVisible() {
		t.Error("a second view_opened for the same click opened a form")
	}
}

// esc on a Home modal closes it in Slack, and o on one slk can't fill
// opens the app's DM in Slack's web client, as the modal has no message.
func TestAppHome_ModalFromAHomeClickEscAndNeedsSlack(t *testing.T) {
	a, _, _ := appHomeApp(t)
	formSvc := &fakeShortcutService{}
	a.SetAppShortcutService(formSvc)
	var browser []string
	a.setDesktopForTest(func(d *core.DesktopServiceFuncs) {
		d.Open = func(url string) error {
			browser = append(browser, url)
			return nil
		}
	})
	selectColony(a)
	homeKey(a, "j")
	homeKey(a, "j")
	homeKey(a, "enter")
	send(a, capturedViewOpened(t, "web-1700000000000"))
	send(a, shortcutKey("esc"))
	if len(formSvc.closes) != 1 || formSvc.closes[0][0] != "T1" || formSvc.closes[0][1] != "V00000VIEW1" || a.mode != ModeNormal {
		t.Fatalf("closes %v, mode %v", formSvc.closes, a.mode)
	}

	homeKey(a, "enter")
	send(a, AppViewMsg{TeamID: "T1", ClientToken: "web-1700000000000", View: []byte(`{"id":"V2","type":"modal","title":{"type":"plain_text","text":"Pick"},"blocks":[{"type":"input","block_id":"d","label":{"type":"plain_text","text":"When"},"element":{"type":"datepicker","action_id":"d"}}]}`)})
	if !strings.Contains(overlayText(a), "This form needs Slack") {
		t.Fatalf("overlay:\n%s", overlayText(a))
	}
	send(a, shortcutKey("o"))
	if len(browser) != 1 || browser[0] != "https://app.slack.com/client/T1/D1" {
		t.Errorf("browser = %v", browser)
	}
}

// The paging keys scroll the Home tab, not the DM under it, and fetch no
// DM history: PgDn and ctrl+d move down, PgUp and ctrl+u back, G goes to
// the last button and g to the first.
func TestAppHome_PagingKeysScrollTheHome(t *testing.T) {
	a, _, _ := appHomeApp(t)
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	selectColony(a)
	top := ansi.Strip(a.View().Content)
	if !strings.Contains(top, "Annotations") || strings.Contains(top, "[ Next ]") {
		t.Fatalf("the Home fits the pane, nothing to page:\n%s", top)
	}
	for _, k := range []tea.KeyPressMsg{{Code: tea.KeyPgDown}, {Code: 'd', Mod: tea.ModCtrl}} {
		a.Update(k)
		if out := ansi.Strip(a.View().Content); out == top {
			t.Errorf("%s did not scroll the Home", k)
		}
		a.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
		a.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
		if out := ansi.Strip(a.View().Content); out != top {
			t.Errorf("PgUp and ctrl+u after %s did not scroll back:\n%s", k, out)
		}
	}
	homeKey(a, "G")
	if out := ansi.Strip(a.View().Content); !strings.Contains(out, "[ Next ]") || a.appHome.model.Hint(false) != "o next and open  O next and open in browser  enter next  m messages" {
		t.Errorf("G: hint %q\n%s", a.appHome.model.Hint(false), out)
	}
	homeKey(a, "g")
	if out := ansi.Strip(a.View().Content); out != top {
		t.Errorf("g did not go back to the top:\n%s", out)
	}
	if a.fetchingOlder["D1"] || a.view != ViewAppHome {
		t.Errorf("fetching DM history %v, view %v", a.fetchingOlder["D1"], a.view)
	}
}

// An app with only a Home tab (messages_tab_enabled false) shows no
// Messages tab, and m stays on the Home, as in Slack.
func TestAppHome_HomeOnlyAppHasNoMessagesTab(t *testing.T) {
	a, svc, _ := appHomeApp(t)
	svc.home.MessagesTab = false
	selectColony(a)
	out := ansi.Strip(a.View().Content)
	if strings.Contains(out, "Messages") || strings.Contains(out, "m messages") || !strings.Contains(out, "o open  O open in browser") {
		t.Fatalf("Home-only app shows a Messages tab:\n%s", out)
	}
	homeKey(a, "m")
	if a.view != ViewAppHome {
		t.Errorf("m left the Home: view %v", a.view)
	}
}

// A Home modal that arrives while a shortcut's form is open, or while
// another mode has the keys (the channel finder), is closed in Slack and
// leaves what is open alone.
func TestAppHome_LateModalIsClosedWhenSomethingElseIsOpen(t *testing.T) {
	a, _, _ := appHomeApp(t)
	formSvc := &fakeShortcutService{}
	a.SetAppShortcutService(formSvc)
	selectColony(a)
	homeKey(a, "j")
	homeKey(a, "j")
	homeKey(a, "enter")
	homeKey(a, "enter") // two clicks, both web-1700000000000

	a.SetMode(ModeChannelFinder)
	send(a, capturedViewOpened(t, "web-1700000000000"))
	if a.mode != ModeChannelFinder || a.appShortcuts.model.IsVisible() || len(formSvc.closes) != 1 || formSvc.closes[0][1] != "V00000VIEW1" {
		t.Fatalf("under the finder: mode %v, visible %v, closes %v", a.mode, a.appShortcuts.model.IsVisible(), formSvc.closes)
	}

	a.SetMode(ModeNormal)
	other := appshortcuts.NewForm(appshortcuts.View{ID: "V00000SHORT"})
	a.appShortcuts.model.OpenForm(other)
	a.SetMode(ModeAppShortcuts)
	send(a, capturedViewOpened(t, "web-1700000000000"))
	if a.appShortcuts.model.Form() != other || len(formSvc.closes) != 2 || formSvc.closes[1][1] != "V00000VIEW1" {
		t.Errorf("over a shortcut's form: form replaced %v, closes %v", a.appShortcuts.model.Form() != other, formSvc.closes)
	}
}

// A modal that answers o ends the Home's wait: no Waiting label, and the
// 10 s timeout that follows says nothing under the open form.
func TestAppHome_ModalAnsweringOEndsTheWait(t *testing.T) {
	a, _, _ := appHomeApp(t)
	a.SetAppShortcutService(&fakeShortcutService{})
	selectColony(a)
	homeKey(a, "j")
	homeKey(a, "j")
	homeKey(a, "o")
	send(a, capturedViewOpened(t, "web-1700000000000"))
	if a.appShortcuts.model.Form() == nil {
		t.Fatal("no form")
	}
	_, cmd := a.Update(appHomeWaitTimeoutMsg{visit: a.appHome.visit, waitID: 1})
	if cmd != nil || strings.Contains(ansi.Strip(a.View().Content), "Waiting for Colony") {
		t.Errorf("the wait outlived the modal: timeout cmd %v", cmd != nil)
	}
}

// A Home-only app opened through a permalink shows its DM with no
// "Home   Messages" row, as it has no Messages tab.
func TestAppHome_HomeOnlyAppPermalinkHasNoTabRow(t *testing.T) {
	a, svc, _ := appHomeApp(t)
	svc.home.MessagesTab = false
	a.pendingLinkNav = &pendingLinkNav{channelID: "D1", messageTS: "1700000000.000100"}
	send(a, ChannelSelectedMsg{ID: "D1", Name: "Colony", Type: "app"})
	out := ansi.Strip(a.View().Content)
	if a.view != ViewChannels || strings.Contains(out, "Home   Messages") || strings.Contains(out, "m home") {
		t.Errorf("view %v:\n%s", a.view, out)
	}
}

// With the sidebar focused beside the Home tab, i and / do nothing to
// the DM under it: no compose box, no search over its messages.
func TestAppHome_SidebarFocusKeysDontReachTheDM(t *testing.T) {
	a, _, _ := appHomeApp(t)
	selectColony(a)
	homeKey(a, "esc")
	if a.focusedPanel != PanelSidebar {
		t.Fatalf("focus = %v", a.focusedPanel)
	}
	for _, k := range []string{"i", "/"} {
		homeKey(a, k)
		if a.mode != ModeNormal || a.focusedPanel != PanelSidebar || a.view != ViewAppHome {
			t.Errorf("%s: mode %v, focus %v, view %v", k, a.mode, a.focusedPanel, a.view)
		}
	}
}

// A permalink on the Home into the app's own DM shows the Messages tab
// with the message selected; for a Home-only app it routes as any
// permalink does, with the Home kept.
func TestAppHome_PermalinkIntoTheDMShowsMessages(t *testing.T) {
	for _, homeOnly := range []bool{false, true} {
		a, svc, _ := appHomeApp(t)
		a.setChannelLookupFuncForTest(func(id ids.ChannelID) (string, string, bool) {
			return "Colony", "app", id == "D1"
		})
		svc.home.MessagesTab = !homeOnly
		svc.home.View = []byte(strings.Replace(string(colonyHomeView(t)),
			"https://example.slack.com/archives/C0C6ZMJJT1U/p1791343906925689",
			"https://example.slack.com/archives/D1/p1700000000000100", 1))
		selectColony(a)
		homeKey(a, "o") // Open review, the first stop
		msg, sel := a.messagepane.SelectedMessage()
		switch {
		case homeOnly && a.view != ViewAppHome:
			t.Errorf("Home-only: view %v", a.view)
		case !homeOnly && (a.view != ViewChannels || !sel || msg.TS != "1700000000.000100"):
			t.Errorf("view %v, selected %v %q", a.view, sel, msg.TS)
		}
	}
}

// PgDn pages the Home by its own pane's height, one line of context
// kept, whatever height the messages pane last had.
func TestAppHome_PageDownUsesTheHomePaneHeight(t *testing.T) {
	a, _, _ := appHomeApp(t)
	selectColony(a)
	a.layout.SetMsgHeight(100) // a taller pane drawn earlier
	body := func() []string {
		lines := strings.Split(ansi.Strip(a.renderAppHomePanel(80, 0, 12, true)), "\n")
		return lines[4 : len(lines)-3] // the border, header, tabs and blank; the blank, hint and border
	}
	before := body()
	a.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	if after := body(); after[0] != before[len(before)-1] {
		t.Errorf("PgDn: first row %q, want the last row before it %q", after[0], before[len(before)-1])
	}
}

// A name that resolves after the Home is drawn replaces the raw mention
// on the next frame, as it does in the messages pane.
func TestAppHome_ResolvedNameRedrawsTheHome(t *testing.T) {
	a, svc, _ := appHomeApp(t)
	svc.home.View = []byte(`{"id":"V1","type":"home","blocks":[{"type":"section","block_id":"s","text":{"type":"mrkdwn","text":"Reviewer: <@U0000ALICE>"}}]}`)
	selectColony(a)
	if out := ansi.Strip(a.View().Content); !strings.Contains(out, "Reviewer: @U0000ALICE") {
		t.Fatalf("before the name resolves:\n%s", out)
	}
	send(a, UserResolvedMsg{TeamID: "T1", UserID: "U0000ALICE", DisplayName: "alice"})
	if out := ansi.Strip(a.View().Content); !strings.Contains(out, "Reviewer: @alice") {
		t.Errorf("after the name resolved:\n%s", out)
	}
}

// Leaving an app DM and coming back while its Home loads runs one
// spinner, not the earlier visit's tick chain beside the new one.
func TestAppHome_ReentryRunsOneSpinner(t *testing.T) {
	a, _, _ := appHomeApp(t)
	var ticks []func(time.Time) tea.Msg
	a.appHome.after = func(_ time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
		ticks = append(ticks, fn)
		return nil
	}
	a.Update(ChannelSelectedMsg{ID: "D1", Name: "Colony", Type: "app"}) // the load is never run
	first := ticks[len(ticks)-1]
	a.Update(ChannelSelectedMsg{ID: "C2", Name: "eng", Type: "channel"})
	a.Update(ChannelSelectedMsg{ID: "D1", Name: "Colony", Type: "app"})
	n := len(ticks)
	a.Update(first(time.Now()))
	if len(ticks) != n {
		t.Errorf("the earlier visit's spinner tick scheduled another: %d ticks, want %d", len(ticks), n)
	}
}
