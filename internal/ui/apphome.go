package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/slackurl"
	"github.com/gammons/slk/internal/ui/apphome"
	"github.com/gammons/slk/internal/ui/appshortcuts"
	"github.com/gammons/slk/internal/ui/overlay"
	"github.com/gammons/slk/internal/ui/styles"
)

// appHomeWait is how long o or O on a button waits for the app's
// redraw to add the link it opens.
const appHomeWait = 10 * time.Second

type appHomeLoadedMsg struct {
	teamID, channelID string
	home              apphome.Home
	err               error
}

// appHomeSpinnerMsg carries the visit whose loading line it turns, so a
// re-entry's spinner is the only one.
type appHomeSpinnerMsg struct{ visit int }

// appHomeClickFailedMsg and appHomeWaitTimeoutMsg carry the visit the
// click was made in, as a wait id counts from 1 again on every visit.
type appHomeClickFailedMsg struct {
	visit, waitID int
	err           error
}

type appHomeWaitTimeoutMsg struct{ visit, waitID int }

// warningToastMsg shows Text in the toast slot in the warning color.
type warningToastMsg struct{ Text string }

// appHomeState is the app DM open in the messages pane: its Home tab,
// shown as ViewAppHome, and its Messages tab, the DM as slk shows any
// DM, with the tab row under the header.
type appHomeState struct {
	svc apphome.Service
	// teamID and channelID are the open app DM; "" for none. One is open
	// while its Home loads, and stays open once it has loaded only when
	// the app has a Home tab.
	teamID, channelID string
	appID             string
	appTeam           string
	model             apphome.Model
	spinner           int
	// noHome holds the app DMs known to have no Home tab, for the
	// session, so they open as a plain DM without reading the Home.
	noHome map[string]bool
	// visit counts the opens of an app DM, for the session.
	visit int
	// clicks holds the latest clicks sent from a Home tab, kept across
	// visits, so a modal an app opens in answer to one opens in slk.
	clicks []homeClick

	// panel is the bordered Home pane of the last frame.
	panel appHomePanelCache

	// after is tea.Tick; tests replace it.
	after func(time.Duration, func(time.Time) tea.Msg) tea.Cmd
}

// appHomePanelCache keeps the bordered pane while the Home's render and
// the pane's size, focus and theme are unchanged. An unchanged render is
// the string Render cached, so comparing it is cheap.
type appHomePanelCache struct {
	key appHomePanelKey
	out string
}

type appHomePanelKey struct {
	inner                 string
	width, border, height int
	focused               bool
	theme                 int64
}

// homeClick is a click sent from an app's Home tab. A view_opened that
// echoes its client token is the modal the app opened in answer, as for
// an app shortcut's run (the echo is not yet seen live for a Home click).
// visit and waitID name the wait of an o or O press, which the modal
// ends.
type homeClick struct {
	token, teamID, channelID, appName string
	visit, waitID                     int
}

// maxHomeClicks is how many clicks are remembered; the oldest goes
// first.
const maxHomeClicks = 8

// SetAppHomeService wires App Home to Slack.
func (a *App) SetAppHomeService(s apphome.Service) { a.appHome.svc = s }

func (s *appHomeState) tick(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
	if s.after != nil {
		return s.after(d, fn)
	}
	return tea.Tick(d, fn)
}

// appHomeChannelSelected opens an app DM's Home tab when the channel
// switch lands on one, and closes App Home for any other channel. A
// permalink into the DM opens its Messages tab instead.
func (a *App) appHomeChannelSelected(m ChannelSelectedMsg) tea.Cmd {
	s := &a.appHome
	if a.activeChannelID != m.ID {
		return nil // the switch was refused (an upload in flight)
	}
	a.closeAppHome()
	if m.Type != "app" || s.svc == nil || s.noHome[m.ID] {
		return nil
	}
	s.teamID, s.channelID, s.model = a.activeTeamID, m.ID, apphome.Open(m.Name)
	s.visit++
	linkNav := a.pendingLinkNav != nil && a.pendingLinkNav.channelID == m.ID
	var spin tea.Cmd
	if !linkNav {
		a.view = ViewAppHome
		spin = s.spinnerTick()
	}
	svc, teamID, channelID := s.svc, s.teamID, s.channelID
	return tea.Batch(spin, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		home, err := svc.Load(ctx, teamID, channelID)
		return appHomeLoadedMsg{teamID: teamID, channelID: channelID, home: home, err: err}
	})
}

// closeAppHome leaves the app DM: the Home tab, if shown, gives way to
// the messages pane, and the tab row goes.
func (a *App) closeAppHome() {
	s := &a.appHome
	if s.channelID == "" {
		return
	}
	if a.view == ViewAppHome {
		a.view = ViewChannels
	}
	a.messagepane.SetHeaderRow(nil)
	*s = appHomeState{svc: s.svc, noHome: s.noHome, visit: s.visit, clicks: s.clicks, after: s.after}
}

func (s *appHomeState) spinnerTick() tea.Cmd {
	visit := s.visit
	return s.tick(100*time.Millisecond, func(time.Time) tea.Msg { return appHomeSpinnerMsg{visit} })
}

func (s *appHomeState) isOpen(teamID, channelID string) bool {
	return s.channelID != "" && s.teamID == teamID && s.channelID == channelID
}

// showMessagesTab swaps the Home tab for the DM.
func (a *App) showMessagesTab() {
	if a.view == ViewAppHome {
		a.view = ViewChannels
	}
	a.appHome.model.CloseBox()
}

// showHomeTab swaps the DM for the Home tab, and tells the app, as
// Slack does on every open of the tab.
func (a *App) showHomeTab() tea.Cmd {
	a.view = ViewAppHome
	return a.appHomeOpenedCmd()
}

func (a *App) appHomeOpenedCmd() tea.Cmd {
	s := &a.appHome
	svc, teamID, channelID, appTeam := s.svc, s.teamID, s.channelID, s.appTeam
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := svc.Opened(ctx, teamID, channelID, appTeam); err != nil {
			debuglog.General("apps.home.dispatchOpenEvent %s: %v", channelID, err)
		}
		return nil
	}
}

// appHomeKey handles a key for App Home: m between the tabs of an app
// DM, and on the Home tab the keys that drive it. Keys that act on a
// selected message or the compose box are swallowed while the Home tab
// is shown, from the sidebar too, as the DM they would act on is hidden.
// The thread panel keeps its keys: it is on screen.
func (a *App) appHomeKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	s := &a.appHome
	if s.channelID == "" || s.channelID != a.activeChannelID {
		return nil, false
	}
	if a.focusedPanel == PanelSidebar {
		return nil, a.view == ViewAppHome && a.homeSwallows(msg)
	}
	if a.focusedPanel != PanelMessages {
		return nil, false
	}
	if key.Matches(msg, a.keys.AppHomeTab) && !s.model.Loading() && !s.model.BoxOpen() {
		switch a.view {
		case ViewAppHome:
			if !s.model.HomeOnly() {
				a.showMessagesTab()
			}
			return nil, true
		case ViewChannels:
			return a.showHomeTab(), true
		}
	}
	if a.view != ViewAppHome {
		return nil, false
	}
	k := normalizeFinderKey(msg)
	switch k {
	case "j", "k", "up", "down", "enter", "o", "O", "esc":
	default:
		if s.model.BoxOpen() || a.appHomePage(msg) || a.homeSwallows(msg) {
			return nil, true
		}
		return nil, false
	}
	if s.model.Loading() {
		if k == "esc" {
			a.focusSidebar()
		}
		return nil, true
	}
	return a.runAppHomeIntent(s.model.Key(k)), true
}

// appHomePage scrolls the Home tab for the paging keys, and reports
// whether msg was one. Top's binding (g) has no handler in normal mode,
// so on the Home a single g goes to the top.
func (a *App) appHomePage(msg tea.KeyMsg) bool {
	m, k := &a.appHome.model, a.keys
	switch {
	case key.Matches(msg, k.Top):
		m.GoToTop()
	case key.Matches(msg, k.Bottom):
		m.GoToBottom()
	case key.Matches(msg, k.PageUp):
		m.Scroll(-a.pageSize())
	case key.Matches(msg, k.PageDown):
		m.Scroll(a.pageSize())
	case key.Matches(msg, k.HalfPageUp):
		m.Scroll(-a.halfPageSize())
	case key.Matches(msg, k.HalfPageDown):
		m.Scroll(a.halfPageSize())
	default:
		return false
	}
	return true
}

// homeSwallows reports whether msg is a key that acts on a selected
// message, its links or its search, or types into the compose box, none
// of which the Home tab has.
func (a *App) homeSwallows(msg tea.KeyMsg) bool {
	k := a.keys
	for _, b := range []key.Binding{k.InsertMode, k.Edit, k.Delete, k.CopyMessage, k.CopyPermalink, k.CopyFromMessage,
		k.OpenPreview, k.OpenLink, k.OpenLinkTab, k.DownloadFile, k.MarkUnread, k.Reaction, k.ReactionNav, k.ListReactions,
		k.SaveThread, k.ToggleAttachmentFold, k.MessageActions, k.ZoomThread, k.SearchMode, k.SearchNext, k.SearchPrev} {
		if key.Matches(msg, b) {
			return true
		}
	}
	return false
}

func (a *App) focusSidebar() {
	if a.sidebarVisible {
		a.focusedPanel = PanelSidebar
	}
}

func (a *App) runAppHomeIntent(in apphome.Intent) tea.Cmd {
	s := &a.appHome
	switch in.Kind {
	case apphome.IntentToSidebar:
		a.focusSidebar()
	case apphome.IntentNeedsSlack:
		return func() tea.Msg { return ToastMsg{Text: "This control needs Slack"} }
	case apphome.IntentClick:
		cmds := []tea.Cmd{a.appHomeClickCmd(in)}
		if in.URL != "" {
			cmds = append(cmds, a.openAppHomeLink(in.URL, in.Open == apphome.OpenInTab))
		}
		if in.WaitID != 0 {
			visit, id := s.visit, in.WaitID
			cmds = append(cmds, s.tick(appHomeWait, func(time.Time) tea.Msg {
				return appHomeWaitTimeoutMsg{visit: visit, waitID: id}
			}))
		}
		return tea.Batch(cmds...)
	}
	return nil
}

// openAppHomeLink opens a link from the Home tab as o and O open one:
// here (a permalink navigates in slk), or in a new herdr tab, which
// outside herdr is the browser.
func (a *App) openAppHomeLink(url string, inTab bool) tea.Cmd {
	switch {
	case inTab && a.herdrTabOpener == nil:
		return a.browserOpener(url)
	case inTab:
		return a.routeLink(url, true)
	}
	return a.routeHomeLink(url)
}

// routeHomeLink opens a link from the Home tab here, as routeLink does.
// A permalink into this app's DM first shows its Messages tab, where the
// message is selected, unless the app has none.
func (a *App) routeHomeLink(url string) tea.Cmd {
	pl, ok := slackurl.Parse(url)
	if ok && string(pl.ChannelID) == a.appHome.channelID && pl.Subdomain == a.activeWorkspaceDomain() && !a.appHome.model.HomeOnly() {
		a.showMessagesTab()
	}
	return a.routeLink(url, false)
}

func (a *App) appHomeClickCmd(in apphome.Intent) tea.Cmd {
	s := &a.appHome
	v := s.model.View()
	click := apphome.Click{
		ViewID: v.ID, BotID: v.BotID, AppID: v.AppID, TeamID: v.TeamID,
		Action: in.Action, State: in.State, ClientToken: a.clientToken(),
	}
	s.clicks = append(s.clicks, homeClick{click.ClientToken, s.teamID, s.channelID, s.model.AppName(), s.visit, in.WaitID})
	s.clicks = slices.Delete(s.clicks, 0, max(len(s.clicks)-maxHomeClicks, 0))
	svc, teamID, visit, waitID := s.svc, s.teamID, s.visit, in.WaitID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := svc.Click(ctx, teamID, click); err != nil {
			return appHomeClickFailedMsg{visit: visit, waitID: waitID, err: err}
		}
		return nil
	}
}

// appHomeWheel scrolls the Home tab under the mouse wheel.
func (a *App) appHomeWheel(up bool, lines int) {
	if up {
		lines = -lines
	}
	a.appHome.model.Scroll(lines)
}

// appHomeClick handles a click at pane-local (x, y) of the Home tab: a
// link opens, else the focus moves to the element on that row.
func (a *App) appHomeClick(x, y int) tea.Cmd {
	if url := a.appHome.model.ClickAt(x, y); url != "" {
		return a.routeHomeLink(url)
	}
	return nil
}

// renderAppHomePanel draws the Home tab in the messages region.
func (a *App) renderAppHomePanel(msgWidth, msgBorder, contentHeight int, msgFocused bool) string {
	border := styles.UnfocusedBorder.Width(msgWidth)
	if msgFocused {
		border = styles.FocusedBorder.Width(msgWidth)
	}
	s := &a.appHome
	a.layout.SetMsgHeight(apphome.BodyHeight(contentHeight - 2)) // what PgDn and ctrl+d page by
	theme := styles.Version()
	inner := s.model.Render(apphome.Params{
		Width:        msgWidth - 2,
		Height:       contentHeight - 2,
		PaneFocused:  msgFocused,
		InHerdr:      a.herdrTabOpener != nil,
		Spinner:      string(styles.SpinnerChars[s.spinner%len(styles.SpinnerChars)]),
		ThemeVersion: theme,
		Ctx:          a.messagepane.HomeBlockKitContext(),
		// The pane's version moves on a resolved name, new channel names
		// or a warm emoji, which change what Ctx draws; its other moves
		// (the hidden DM's own changes) cost a relayout of the Home.
		CtxVersion: a.messagepane.Version(),
	})
	key := appHomePanelKey{inner, msgWidth, msgBorder, contentHeight, msgFocused, theme}
	if s.panel.out == "" || s.panel.key != key {
		s.panel = appHomePanelCache{key, exactSize(border.Render(inner), msgWidth+msgBorder, contentHeight)}
	}
	return s.panel.out
}

// appHomeOverlay draws the Home tab's confirm box over screen.
func (a *App) appHomeOverlay(screen string) string {
	if a.view != ViewAppHome {
		return screen
	}
	box := a.appHome.model.ConfirmBox(a.width)
	if box == "" {
		return screen
	}
	lines := strings.Split(overlay.DimmedOverlay(a.width, a.height, screen, box, 0.5), "\n")
	return strings.Join(lines[:min(len(lines), a.height)], "\n")
}

var reduceAppHome reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	s := &a.appHome
	switch m := msg.(type) {
	case tea.MouseClickMsg:
		// An open select or confirm box keeps clicks from the panes
		// under it; the wheel still scrolls the Home, and a modal over
		// it (ctrl+c's quit prompt) still takes its clicks.
		return nil, a.view == ViewAppHome && s.model.BoxOpen() && !a.mode.IsModalOverlay()

	case appHomeLoadedMsg:
		if !s.isOpen(m.teamID, m.channelID) {
			return nil, true
		}
		return a.applyAppHome(m), true

	case appHomeSpinnerMsg:
		if s.channelID == "" || s.visit != m.visit || !s.model.Loading() {
			return nil, true
		}
		s.spinner++
		return s.spinnerTick(), true

	case appHomeClickFailedMsg:
		if s.channelID == "" || s.visit != m.visit {
			return nil, true
		}
		s.model.EndWait(m.waitID)
		text := s.model.AppName() + " failed: " + m.err.Error()
		return func() tea.Msg { return ToastMsg{Text: text} }, true

	case appHomeWaitTimeoutMsg:
		if s.channelID == "" || s.visit != m.visit {
			return nil, true
		}
		if s.model.WaitTimedOut(m.waitID) {
			text := "! " + s.model.AppName() + " did not answer"
			return func() tea.Msg { return warningToastMsg{Text: text} }, true
		}
		return nil, true

	case warningToastMsg:
		a.statusbar.SetWarningToast(m.Text)
		return copiedClearAfter(3 * time.Second), true

	case AppViewMsg:
		return a.applyAppHomeView(m)
	}
	return nil, false
}

// applyAppHome shows the loaded Home tab, or the DM when the app has
// none or it could not be read.
func (a *App) applyAppHome(m appHomeLoadedMsg) tea.Cmd {
	s := &a.appHome
	if m.err != nil {
		debuglog.General("conversations.info return_app_home %s: %v", m.channelID, m.err)
		app := s.model.AppName()
		a.closeAppHome()
		return func() tea.Msg { return ToastMsg{Text: "Could not load " + app + "'s Home"} }
	}
	if !m.home.Enabled {
		if s.noHome == nil {
			s.noHome = map[string]bool{}
		}
		s.noHome[m.channelID] = true
		a.closeAppHome()
		return nil
	}
	s.appID, s.appTeam = m.home.AppID, m.home.TeamID
	if s.appTeam == "" {
		s.appTeam = s.teamID
	}
	var v *apphome.View
	if m.home.View != nil {
		var err error
		if v, err = apphome.ParseView(m.home.View); err != nil {
			debuglog.General("home view %s: %v", m.channelID, err)
		}
	}
	s.model.SetHomeOnly(!m.home.MessagesTab)
	s.model.SetView(v)
	if m.home.MessagesTab {
		a.messagepane.SetHeaderRow(func(w int) string { return apphome.TabRow(w, false) })
	}
	if a.view == ViewAppHome {
		return a.appHomeOpenedCmd()
	}
	return nil
}

// applyAppHomeView takes a Home view the open app published or redrew,
// and opens the link o or O on the Home tab was waiting for, or says
// the redraw added several. A modal opened in answer to a Home click
// opens in the app shortcuts' form; other modals are left to them.
func (a *App) applyAppHomeView(m AppViewMsg) (tea.Cmd, bool) {
	var head struct {
		Type  string `json:"type"`
		AppID string `json:"app_id"`
	}
	if json.Unmarshal(m.View, &head) != nil {
		return nil, false
	}
	if head.Type != "home" {
		return a.openHomeModal(m)
	}
	s := &a.appHome
	if s.channelID == "" || m.TeamID != s.teamID || head.AppID != s.appID || s.model.Loading() {
		return nil, true
	}
	v, err := apphome.ParseView(m.View)
	if err != nil {
		debuglog.General("home view_updated: %v", err)
		return nil, true
	}
	added, open := s.model.Update(v)
	switch {
	case len(added) > 1:
		text := fmt.Sprintf("! %s added %d links: pick one on the Home", s.model.AppName(), len(added))
		return func() tea.Msg { return warningToastMsg{Text: text} }, true
	case len(added) == 1 && s.channelID == a.activeChannelID:
		return a.openAppHomeLink(added[0], open == apphome.OpenInTab), true
	}
	return nil, true
}

// openHomeModal opens a modal an app opened in answer to a click on its
// Home tab, in the app shortcuts' form, as Slack shows it over the Home;
// the form's submit, close and "needs Slack" work as for a shortcut's.
// One that arrives while slk is in another mode or overlay is closed in
// Slack instead, so it takes no keys from what is open. Any other
// view_opened is not one, and is left to the app shortcuts.
func (a *App) openHomeModal(m AppViewMsg) (tea.Cmd, bool) {
	s := &a.appHome
	i := slices.IndexFunc(s.clicks, func(c homeClick) bool { return c.token == m.ClientToken && c.teamID == m.TeamID })
	if m.Updated || i < 0 {
		return nil, false
	}
	c := s.clicks[i]
	s.clicks = slices.Delete(s.clicks, i, i+1)
	if s.channelID != "" && s.visit == c.visit {
		s.model.EndWait(c.waitID)
	}
	v, err := appshortcuts.ParseView(m.View)
	if err != nil {
		debuglog.General("view_opened from %s's Home: %v", c.appName, err)
		return func() tea.Msg { return ToastMsg{Text: "Could not read the form from " + c.appName} }, true
	}
	sc := &a.appShortcuts
	if sc.model.IsVisible() || a.mode != ModeNormal {
		return a.closeAppViewCmd(c.teamID, v.ID, v.RootViewID), true
	}
	sc.teamID, sc.channelID, sc.messageTS = c.teamID, c.channelID, ""
	sc.running, sc.token = appshortcuts.Shortcut{AppName: c.appName}, c.token
	f := appshortcuts.NewForm(v)
	sc.model.OpenForm(f)
	a.SetMode(ModeAppShortcuts)
	return f.Focus(), true
}
