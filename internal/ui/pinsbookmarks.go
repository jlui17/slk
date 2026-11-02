package ui

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/pinsbookmarks"
	"github.com/gammons/slk/internal/ui/styles"
)

// Pins and bookmarks, the App's side of the p and B dialog. pins.list
// allows about 20 calls a minute across every slk window, too few to
// call on every channel switch, so the count line under a channel's
// name takes its pin count from the history read that opens the channel
// (PinCountMsg), or from a one-message history read when none is
// coming, and from the pin events after it; pins.list runs only when
// the dialog opens. bookmarks.list allows more: a channel's bookmarks
// are read when it opens and kept for five minutes. Slack sends no
// bookmark events, so the dialog reads both lists every time it opens.

// PinsChangedMsg is a pin added to (Added) or removed from ChannelID,
// in the workspace TeamID.
type PinsChangedMsg struct {
	TeamID, ChannelID string
	Added             bool
}

// PinCountMsg is how many pins ChannelID has, in the workspace TeamID,
// as conversations.history said in a read that began at Started.
type PinCountMsg struct {
	TeamID, ChannelID string
	Count             int
	Started           time.Time
}

// pinsReadDelay is how long a channel must stay on screen before its
// lists are read, so a quick walk through channels reads none of the
// ones it passes.
var pinsReadDelay = 300 * time.Millisecond

// bookmarksFresh is how long a channel's bookmarks are kept before a
// reopen reads them again.
const bookmarksFresh = 5 * time.Minute

// pinsReadDueMsg is the moment a channel opened pinsReadDelay ago reads
// its lists, if it is still on screen. history is a history read on its
// way, which brings the pin count.
type pinsReadDueMsg struct {
	key     pinsKey
	history bool
}

type pinsBookmarksLoadedMsg struct {
	key       pinsKey
	list      pinsbookmarks.Section
	seq       int
	pins      []pinsbookmarks.Pin
	bookmarks []pinsbookmarks.Bookmark
	err       error
}

type pinsKey struct{ teamID, channelID string }

// channelLists is what slk knows of one channel's pins and bookmarks.
type channelLists struct {
	// pinCount is how many pins the channel has, -1 until a history
	// read or pins.list says.
	pinCount int
	// pinEvent is when the latest pin event came: a history read that
	// began before it says nothing new.
	pinEvent  time.Time
	pins      []pinsbookmarks.Pin
	bookmarks []pinsbookmarks.Bookmark
	// lists is the state of each list's read, by section.
	lists [2]listRead
}

// listRead is the state of reading one list.
type listRead struct {
	loaded, loading bool
	err             error
	// readAt is when the list was last read.
	readAt time.Time
	// stale is a change since the read in flight began: it reads again
	// when it lands, if the dialog shows it.
	stale bool
	// seq names the latest read; an earlier one's answer is dropped.
	seq int
}

type pinsBookmarksState struct {
	svc   pinsbookmarks.Service
	model pinsbookmarks.Model
	lists map[pinsKey]*channelLists
	// open is the channel the dialog shows.
	open pinsKey
}

// SetPinsBookmarksService wires the pins and bookmarks dialog to Slack.
func (a *App) SetPinsBookmarksService(s pinsbookmarks.Service) { a.pinsBookmarks.svc = s }

func init() {
	commands["pins"] = func(a *App, _ commandArgs) tea.Cmd { return a.openPinsBookmarks(pinsbookmarks.Pins) }
	commands["bookmarks"] = func(a *App, _ commandArgs) tea.Cmd { return a.openPinsBookmarks(pinsbookmarks.Bookmarks) }
}

func (a *App) activePinsKey() pinsKey { return pinsKey{a.activeTeamID, a.activeChannelID} }

// channelLists is key's lists, made when there are none.
func (s *pinsBookmarksState) channelLists(key pinsKey) *channelLists {
	if s.lists == nil {
		s.lists = map[pinsKey]*channelLists{}
	}
	l := s.lists[key]
	if l == nil {
		l = &channelLists{pinCount: -1}
		s.lists[key] = l
	}
	return l
}

// pinsBookmarksChannelSelected is a channel select: history reports
// whether a history read of the channel is on its way.
func (a *App) pinsBookmarksChannelSelected(m ChannelSelectedMsg, history bool) tea.Cmd {
	if a.activeChannelID != m.ID {
		return nil // the switch was refused (an upload in flight)
	}
	return a.pinsChannelOpened(history)
}

// pinsChannelOpened draws the count line of the channel now on screen
// and, if it stays on screen pinsReadDelay, reads what it lacks. A
// channel select calls it, and focusing a split window, which reads no
// history.
func (a *App) pinsChannelOpened(history bool) tea.Cmd {
	key := a.activePinsKey()
	a.setPinsCountRow(key)
	if a.pinsBookmarks.svc == nil || key.channelID == "" {
		return nil
	}
	return tea.Tick(pinsReadDelay, func(time.Time) tea.Msg { return pinsReadDueMsg{key, history} })
}

// pinsReadDue reads the bookmarks of the channel still on screen when
// it has none, or they failed or are older than bookmarksFresh, and its
// pin count when it has none and no history read brings it.
func (a *App) pinsReadDue(m pinsReadDueMsg) tea.Cmd {
	if m.key != a.activePinsKey() {
		return nil // the channel was left before its read was due
	}
	l := a.pinsBookmarks.channelLists(m.key)
	var cmds []tea.Cmd
	if r := l.lists[pinsbookmarks.Bookmarks]; !r.loading && (!r.loaded || r.err != nil || a.now().Sub(r.readAt) > bookmarksFresh) {
		cmds = append(cmds, a.readList(m.key, pinsbookmarks.Bookmarks))
	}
	if l.pinCount < 0 && !m.history {
		cmds = append(cmds, a.readPinCount(m.key))
	}
	return tea.Batch(cmds...)
}

// readPinCount reads key's pin count from a one-message history read.
// A failure leaves the count unknown.
func (a *App) readPinCount(key pinsKey) tea.Cmd {
	svc := a.pinsBookmarks.svc
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		started := time.Now()
		n, err := svc.PinCount(ctx, key.teamID, key.channelID)
		if err != nil {
			return nil
		}
		return PinCountMsg{TeamID: key.teamID, ChannelID: key.channelID, Count: n, Started: started}
	}
}

// readList reads one of key's lists.
func (a *App) readList(key pinsKey, list pinsbookmarks.Section) tea.Cmd {
	s := &a.pinsBookmarks
	r := &s.channelLists(key).lists[list]
	r.seq++
	r.loading, r.err, r.stale = true, nil, false
	svc, seq := s.svc, r.seq
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		msg := pinsBookmarksLoadedMsg{key: key, list: list, seq: seq}
		if list == pinsbookmarks.Pins {
			msg.pins, msg.err = svc.Pins(ctx, key.teamID, key.channelID)
		} else {
			msg.bookmarks, msg.err = svc.Bookmarks(ctx, key.teamID, key.channelID)
		}
		return msg
	}
}

var reducePinsBookmarks reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case pinsReadDueMsg:
		return a.pinsReadDue(m), true
	case pinsBookmarksLoadedMsg:
		return a.listLoaded(m), true
	case PinsChangedMsg:
		return a.pinsChanged(m), true
	case PinCountMsg:
		a.pinCountRead(m)
		return nil, true
	}
	return nil, false
}

// listLoaded takes a list's answer. Pins that changed while they were
// read load once more if the dialog shows them.
func (a *App) listLoaded(m pinsBookmarksLoadedMsg) tea.Cmd {
	l := a.pinsBookmarks.lists[m.key]
	if l == nil || l.lists[m.list].seq != m.seq {
		return nil
	}
	r := &l.lists[m.list]
	r.loading = false
	switch {
	case m.err != nil:
		r.err = m.err
	case m.list == pinsbookmarks.Pins:
		r.loaded, r.readAt, l.pins, l.pinCount = true, a.now(), m.pins, len(m.pins)
	default:
		r.loaded, r.readAt, l.bookmarks = true, a.now(), m.bookmarks
	}
	a.showPinsBookmarks(m.key)
	if r.stale && a.dialogShows(m.key) {
		return a.readList(m.key, m.list)
	}
	return nil
}

// pinsChanged counts a pin event into the channel's pin count and marks
// a pins read in flight stale. The dialog, when it shows them, reads
// them again: at once, or when the read in flight lands, so a burst of
// events costs one more read.
func (a *App) pinsChanged(m PinsChangedMsg) tea.Cmd {
	key := pinsKey{m.TeamID, m.ChannelID}
	l := a.pinsBookmarks.lists[key]
	if l == nil {
		return nil // never opened: its history read will count its pins
	}
	l.pinEvent = time.Now()
	switch {
	case l.pinCount < 0:
	case m.Added:
		l.pinCount++
	default:
		l.pinCount = max(l.pinCount-1, 0)
	}
	r := &l.lists[pinsbookmarks.Pins]
	r.stale = true
	a.setPinsCountRow(key)
	if a.dialogShows(key) && !r.loading {
		return a.readList(key, pinsbookmarks.Pins)
	}
	return nil
}

// pinCountRead takes the pin count of a history read, unless a pin
// event came after the read began.
func (a *App) pinCountRead(m PinCountMsg) {
	key := pinsKey{m.TeamID, m.ChannelID}
	l := a.pinsBookmarks.channelLists(key)
	if m.Started.Before(l.pinEvent) {
		return
	}
	l.pinCount = m.Count
	a.setPinsCountRow(key)
}

func (a *App) dialogShows(key pinsKey) bool {
	s := &a.pinsBookmarks
	return s.model.IsVisible() && s.open == key
}

// showPinsBookmarks redraws what shows key's lists: its count lines and
// the dialog, when it is open on key.
func (a *App) showPinsBookmarks(key pinsKey) {
	a.setPinsCountRow(key)
	if a.dialogShows(key) {
		a.fillPinsBookmarks()
	}
}

// openPinsBookmarks is p, B, :pins and :bookmarks: the dialog over the
// channel on screen, on section. Slack sends no bookmark events, so it
// reads both lists, at once, whatever it read before.
func (a *App) openPinsBookmarks(section pinsbookmarks.Section) tea.Cmd {
	if a.view != ViewChannels || a.activeChannelID == "" {
		return toastWithClear(a, "Open a channel to see its pins and bookmarks", 3*time.Second)
	}
	s := &a.pinsBookmarks
	key := a.activePinsKey()
	l := s.channelLists(key)
	var cmds []tea.Cmd
	for list, r := range l.lists {
		if s.svc != nil && !r.loading {
			cmds = append(cmds, a.readList(key, pinsbookmarks.Section(list)))
		}
	}
	name, chType, _ := a.channels.Lookup(ids.ChannelID(a.activeChannelID))
	s.open = key
	s.model.Open(channelDisplayName(name, chType), section)
	a.fillPinsBookmarks()
	a.SetMode(ModePinsBookmarks)
	a.statusbar.SetMode(&s.model)
	return tea.Batch(cmds...)
}

// retryPinsBookmarks is r: it reads again the lists that failed.
func (a *App) retryPinsBookmarks() tea.Cmd {
	s := &a.pinsBookmarks
	var cmds []tea.Cmd
	for list, r := range s.channelLists(s.open).lists {
		if r.err != nil && !r.loading {
			cmds = append(cmds, a.readList(s.open, pinsbookmarks.Section(list)))
		}
	}
	a.showPinsBookmarks(s.open)
	return tea.Batch(cmds...)
}

// fillPinsBookmarks puts the open channel's lists, or where their
// reading is, in the dialog.
func (a *App) fillPinsBookmarks() {
	s := &a.pinsBookmarks
	l := s.channelLists(s.open)
	for list, r := range l.lists {
		section := pinsbookmarks.Section(list)
		switch {
		case r.loading:
			s.model.SetLoading(section)
		case r.err != nil:
			s.model.SetError(section, pinsBookmarksErrText(r.err))
		case section == pinsbookmarks.Pins:
			s.model.SetPins(a.pinRows(l.pins))
		default:
			s.model.SetBookmarks(a.bookmarkRows(l.bookmarks))
		}
	}
	a.statusbar.SetMode(&s.model)
}

func pinsBookmarksErrText(err error) string {
	var rl *core.RateLimitedError
	if !errors.As(err, &rl) {
		return "Loading failed: " + err.Error() + ". Press r to retry."
	}
	wait := "a few seconds"
	if secs := int(rl.RetryAfter.Round(time.Second) / time.Second); secs > 0 {
		wait = plural(secs, "second")
	}
	return "Slack is limiting requests right now. Wait " + wait + ", then press r to retry."
}

func (a *App) pinRows(pins []pinsbookmarks.Pin) []pinsbookmarks.PinRow {
	rows := make([]pinsbookmarks.PinRow, len(pins))
	for i, p := range pins {
		author := a.userNameFor(p.UserID)
		if p.UserID == "" {
			author = p.Username // a bot's message
		}
		row := pinsbookmarks.PinRow{
			Author:   author,
			Text:     a.flattenMrkdwn(p.Text),
			InThread: p.ThreadTS != "" && p.ThreadTS != p.TS,
		}
		if p.TS != "" {
			row.Day = messages.FormatShortDate(messages.DateFromTS(p.TS))
		}
		if row.Text == "" && p.FileName != "" {
			row.Text, row.File = p.FileName, true
		}
		rows[i] = row
	}
	return rows
}

func (a *App) bookmarkRows(bookmarks []pinsbookmarks.Bookmark) []pinsbookmarks.BookmarkRow {
	rows := make([]pinsbookmarks.BookmarkRow, len(bookmarks))
	for i, b := range bookmarks {
		title := b.Title
		if title == "" {
			title = b.Link
		}
		rows[i] = pinsbookmarks.BookmarkRow{
			Title:  title,
			Site:   linkSite(b.Link),
			Folder: b.Folder,
			InApp:  a.opensInSlk(b.Link),
		}
	}
	return rows
}

// linkSite is a link as a bookmark row shows it: its host and path.
func linkSite(link string) string {
	u, err := url.Parse(link)
	if err != nil || u.Host == "" {
		return link
	}
	if len(u.Path) > 1 {
		return u.Host + u.Path
	}
	return u.Host
}

// openPinOrBookmark opens what the dialog chose.
func (a *App) openPinOrBookmark(act pinsbookmarks.Action) tea.Cmd {
	s := &a.pinsBookmarks
	l := s.lists[s.open]
	target := HerdrNone
	switch act.Kind {
	case pinsbookmarks.OpenTab:
		target = HerdrTab
	case pinsbookmarks.OpenPane:
		target = HerdrPane
	}
	if act.Section == pinsbookmarks.Pins {
		return a.openPin(s.open, l.pins[act.Index], target)
	}
	link := l.bookmarks[act.Index].Link
	return func() tea.Msg { return OpenLinkMsg{URL: link, InHerdr: target} }
}

// openPin opens a pin as its permalink opens: a thread reply in its
// thread, a pinned file in the browser, and any pin in a herdr tab or
// pane for O and alt+o. A top-level message, and a pin without a
// permalink, jump straight to the message instead (applyLocation, as a
// mark does): routeLink opens the thread of a message that has replies,
// and Slack takes a pinned thread parent to the message, not its
// thread.
func (a *App) openPin(key pinsKey, p pinsbookmarks.Pin, target HerdrTarget) tea.Cmd {
	reply := p.ThreadTS != "" && p.ThreadTS != p.TS
	if link := p.Permalink; link != "" && (reply || p.TS == "" || a.herdrOpener(target) != nil) {
		return func() tea.Msg { return OpenLinkMsg{URL: link, InHerdr: target} }
	}
	loc := Location{TeamID: ids.TeamID(key.teamID), ChannelID: ids.ChannelID(key.channelID), MessageTS: ids.MessageTS(p.TS)}
	if reply {
		loc.ThreadTS = ids.ThreadTS(p.ThreadTS)
	}
	cmd, _ := a.applyLocation(loc, false)
	return cmd
}

// pinsBookmarksKey is p and B in normal mode.
func (a *App) pinsBookmarksKey(msg tea.KeyMsg) tea.Cmd {
	if key.Matches(msg, a.keys.Bookmarks) {
		return a.openPinsBookmarks(pinsbookmarks.Bookmarks)
	}
	return a.openPinsBookmarks(pinsbookmarks.Pins)
}

// leavePinsBookmarks closes the dialog when its mode gives way to
// another, as a global intercept (ctrl+c) makes it.
func (a *App) leavePinsBookmarks(mode Mode) {
	if a.mode == ModePinsBookmarks && mode != ModePinsBookmarks {
		a.pinsBookmarks.model.Close()
	}
}

// closePinsBookmarks is a click outside the dialog: it closes, a filter
// typed or not.
func (a *App) closePinsBookmarks() tea.Cmd {
	a.SetMode(ModeNormal)
	return nil
}

func (a *App) pinsBookmarksOverlay(screen string) string {
	return a.pinsBookmarks.model.ViewOverlay(a.width, a.height, screen)
}

// setPinsCountRow sets the count line under the channel's name in every
// window that shows key's channel.
func (a *App) setPinsCountRow(key pinsKey) {
	if key.teamID != a.activeTeamID {
		return
	}
	row := a.pinsCountRow(key)
	if key.channelID == a.activeChannelID {
		a.messagepane.SetHeaderRow(messages.HeaderCounts, row)
	}
	if a.wins == nil {
		return
	}
	for _, id := range a.wins.Leaves() {
		if ch, ok := a.wins.Channel(id); ok && ch.ID == key.channelID && a.winModels[id] != nil {
			a.winModels[id].SetHeaderRow(messages.HeaderCounts, row)
		}
	}
}

// pinsCountRow is key's count line: how many pins and bookmarks it
// has, and which list failed to load. None until there is something to
// show, so the pane does not jump while they load. Styled when drawn,
// so a theme switch recolors it.
func (a *App) pinsCountRow(key pinsKey) func(int) string {
	l := a.pinsBookmarks.lists[key]
	if l == nil {
		return nil
	}
	pins, bookmarks, failed := l.pinCount, len(l.bookmarks), l.lists[pinsbookmarks.Bookmarks].err != nil
	if pins <= 0 && bookmarks == 0 && !failed {
		return nil
	}
	return func(width int) string {
		on := func() lipgloss.Style { return lipgloss.NewStyle().Background(styles.Background) }
		muted, text := on().Foreground(styles.TextMuted), on().Foreground(styles.TextPrimary)
		var counts, keys []string
		if pins > 0 {
			counts = append(counts, text.Render("📌 ")+muted.Render(plural(pins, "pin")))
			keys = append(keys, "p pins")
		}
		switch {
		case failed:
			counts = append(counts, on().Foreground(styles.Warning).Render("! Couldn't load bookmarks")+
				muted.Render("   B to see why and retry"))
		case bookmarks > 0:
			counts = append(counts, text.Render("🔖 ")+muted.Render(plural(bookmarks, "bookmark")))
			keys = append(keys, "B bookmarks")
		}
		row := on().Render(" ") + strings.Join(counts, muted.Render("   "))
		right := muted.Render(strings.Join(keys, "  "))
		if lipgloss.Width(row)+lipgloss.Width(right)+2 <= width {
			row += on().Render(strings.Repeat(" ", width-lipgloss.Width(row)-lipgloss.Width(right)-1)) + right
		}
		row = ansi.Truncate(row, width, "…")
		return row + on().Render(strings.Repeat(" ", max(width-lipgloss.Width(row), 0)))
	}
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
