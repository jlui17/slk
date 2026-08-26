package main

import (
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/slackfmt"
	"github.com/gammons/slk/internal/ui"
	"github.com/gammons/slk/internal/ui/channelfinder"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/sidebar"
)

// cacheFirstLoader paints one workspace from sqlite before any network
// call, and carries the cache-only fallbacks the ChannelService
// closures need while router.Active() is still nil. teamID holds the
// provisionally claimed workspace ("" until claimed); it is written by
// the SendCachedWorkspace goroutine and read from the Update loop.
type cacheFirstLoader struct {
	db       *cache.DB
	cfg      config.Config
	tsFormat string
	router   *workspaceRouter
	teamID   atomic.Value
}

func newCacheFirstLoader(db *cache.DB, cfg config.Config, tsFormat string, router *workspaceRouter) *cacheFirstLoader {
	l := &cacheFirstLoader{db: db, cfg: cfg, tsFormat: tsFormat, router: router}
	l.teamID.Store("")
	return l
}

// SendCachedWorkspace mirrors the active-workspace claim the real boot
// makes: defaultTeamID when set (default_workspace, a startup link, or
// the pane restore forced it), else the first configured workspace
// whose cache holds channels. At most one WorkspaceCachedMsg is sent
// per run, and none for an empty cache — a first run keeps the
// connecting overlay.
func (l *cacheFirstLoader) SendCachedWorkspace(tokens []config.OrderedToken, defaultTeamID string, paneRestore *cache.PaneState, send func(tea.Msg)) {
	for _, ot := range tokens {
		if defaultTeamID != "" && ot.Token.TeamID != defaultTeamID {
			continue
		}
		msg, ok := l.cachedWorkspaceMsg(ot.Token.TeamID, ot.Token.TeamName, paneRestore)
		if !ok {
			if defaultTeamID != "" {
				return
			}
			continue
		}
		l.teamID.Store(msg.TeamID)
		send(msg)
		return
	}
}

func (l *cacheFirstLoader) cachedWorkspaceMsg(teamID, teamName string, paneRestore *cache.PaneState) (ui.WorkspaceCachedMsg, bool) {
	rows, err := l.db.ListChannels(teamID, false)
	if err != nil || len(rows) == 0 {
		return ui.WorkspaceCachedMsg{}, false
	}
	handleNames := map[string]string{}
	if users, err := l.db.ListUsers(teamID); err == nil {
		for _, u := range users {
			if u.Name != "" {
				handleNames[u.Name] = u.BestName()
			}
		}
	}
	visits, err := l.db.GetChannelVisits(teamID)
	if err != nil {
		visits = nil
	}
	var items []sidebar.ChannelItem
	var finderItems []channelfinder.Item
	for _, ch := range rows {
		if !ch.IsMember {
			continue
		}
		name := ch.Name
		if ch.Type == "group_dm" {
			name = slackfmt.FormatMPDMName(name, func(h string) string { return handleNames[h] })
		}
		// DM rows cache no display name (it lives on the peer's user
		// record, which the channel row doesn't reference); they join
		// the sidebar when the real list lands.
		if name == "" {
			continue
		}
		section, channelOrder := l.cfg.MatchSectionAndOrder(teamID, ch.Name)
		item := sidebar.ChannelItem{
			ID:           ch.ID,
			Name:         name,
			Type:         ch.Type,
			Section:      section,
			ChannelOrder: channelOrder,
			IsStarred:    ch.IsStarred,
		}
		if section != "" {
			item.SectionOrder = l.cfg.SectionOrder(teamID, section)
		}
		items = append(items, item)
		finderItems = append(finderItems, channelfinder.Item{
			ID:          ch.ID,
			Name:        name,
			Type:        ch.Type,
			Joined:      true,
			LastVisited: visits[ch.ID],
		})
	}
	if len(items) == 0 {
		return ui.WorkspaceCachedMsg{}, false
	}
	return ui.WorkspaceCachedMsg{
		TeamID:        teamID,
		TeamName:      teamName,
		Theme:         l.cfg.ResolveTheme(teamID),
		SidebarWidth:  l.cfg.ResolveWidth(teamID),
		Channels:      items,
		FinderItems:   finderItems,
		LastChannelID: restoredChannelFor(paneRestore, teamID, visits),
	}, true
}

func (l *cacheFirstLoader) claimedTeam() string {
	return l.teamID.Load().(string)
}

// readCache is the ChannelService.ReadCache fallback for the window
// before any workspace connects: selfUserID and the live user-name
// store don't exist yet, so reactions render un-highlighted and names
// resolve from the cached users table.
func (l *cacheFirstLoader) readCache(channelID string) []messages.MessageItem {
	if l.claimedTeam() == "" {
		return nil
	}
	return loadCachedMessages(l.db, "", channelID, nil, l.tsFormat, l.router)
}

// readState is the read-state reader fallback for the same window.
func (l *cacheFirstLoader) readState() map[string]cache.ReadState {
	team := l.claimedTeam()
	if team == "" {
		return nil
	}
	state, err := l.db.GetWorkspaceReadState(team)
	if err != nil {
		return nil
	}
	return state
}
