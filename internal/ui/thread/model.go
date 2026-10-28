package thread

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/gammons/slk/internal/debuglog"
	emojiutil "github.com/gammons/slk/internal/emoji"

	"github.com/gammons/slk/internal/bubbles/entrylist"
	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/imgrender"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
	"github.com/gammons/slk/internal/ui/peerstatus"
	"github.com/gammons/slk/internal/ui/styles"
	"github.com/gammons/slk/internal/usergroups"
	"github.com/gammons/slk/internal/usernames"
)

var thickLeftBorder = lipgloss.Border{Left: "▌"}

// viewEntry is the thread pane's payload for one reply, index-aligned
// with m.replies (and with m.replyEntries): the parts the list substrate
// does not need. m.cache == nil means the reply cache was discarded and
// the next View rebuilds it.
type viewEntry struct {
	// shortSrc is the selected reply's content with the short timestamp,
	// set only when the selected variant carries a long one;
	// linesSelectedShort is built from it on demand (selectedLines) and
	// shown while a text selection exists.
	shortSrc           string
	linesSelectedShort []string

	// flushes are per-frame kitty image uploads for the reply's inline
	// images, invoked when the frame's layout is rebuilt. v1 scope: sixel
	// renders its placeholder only, so no sixel rows are captured.
	flushes []func(io.Writer) error

	// codeBlockCopyLabels locates the copy label of each fenced block in
	// the body, in Lines cells. CodeBlockAt reads them. See
	// codeblocks_fork.go.
	codeBlockCopyLabels []messages.CodeBlockCopyLabel

	// attachmentFoldRows are the rows of Lines that hold a card's fold
	// row. See attachmentfold_fork.go.
	attachmentFoldRows []int
}

// reactionEntryHit is one reaction-pill hit-rect relative to a rendered
// reply, carrying the emoji name the click toggles.
type reactionEntryHit struct {
	rowStartInEntry int
	rowEndInEntry   int // exclusive
	colStart        int
	colEnd          int // exclusive
	emoji           string
}

// Model represents the thread panel UI component.
// It displays a parent message and its replies with cursor navigation.
// parentSelected is the sentinel value of Model.selected meaning the
// cursor sits on the parent message (the row above reply index 0).
const parentSelected = -1

type Model struct {
	parent    messages.MessageItem
	replies   []messages.MessageItem
	channelID string
	threadTS  string
	selected  int
	// pendingSelectTS, when non-empty, is a one-shot cursor pin: the
	// first SetThread of the current thread whose content contains
	// this ts selects it, then the pin disarms. Armed by a permalink
	// open (SetPendingSelectTS) and by a same-thread reload whose
	// content lost the cursor's message (restoreSelection); both live
	// in select.go.
	pendingSelectTS   string
	focused           bool
	coloredUsernames  bool
	avatarFn          messages.AvatarFunc
	userNames         *usernames.Store
	userStatuses      map[string]peerstatus.Status
	channelNames      map[string]string
	reactionNavActive bool
	reactionNavIndex  int
	// currentUserID is the authenticated user; UpdateReaction uses it to
	// set HasReacted only for the current user's own reactions.
	currentUserID string

	// Reply cache: the payload and the list entry of every reply,
	// rebuilt when the replies or the width change.
	cache         []viewEntry
	replyEntries  []entrylist.Entry
	cacheWidth    int
	cacheReplyLen int

	// list is the shared substrate: entries (the parent, the replies and
	// the decoration rows between them), scroll, the visible window, text
	// selection, hit translation and the scrollbar. Configured for the
	// thread pane's behaviour in New.
	list *entrylist.Model

	// layoutValid says the previous frame's layout still describes the
	// screen: a width change then keeps the scroll position (U5). Cleared
	// by every invalidation, by SetFocused, SetUnreadBoundary and ClickAt
	// (the events that cleared the old view cache).
	layoutValid bool

	// flushKey is the layout the reply flushes were last written for;
	// kitty uploads are re-emitted only when it changes, as when the old
	// view cache was rebuilt.
	flushKey struct {
		selected, width, height int
		hasSelection            bool
	}

	// chromeCache holds the rendered "header + separator" prefix that
	// View() prepends to the reply area. Rebuilt only when its inputs
	// (width, replyCount, userNames, channelNames, breadcrumb) change.
	chromeCache         string
	chromeCacheValid    bool
	chromeWidth         int
	chromeReplyCount    int
	chromeUserNamesV    uint64 // value of userNamesV at build time
	chromeChannelNamesV uint64 // version of the channelNames map at build time

	// Breadcrumb inputs (SetBreadcrumb), and the values the chrome
	// cache was last built from.
	crumbChannel       string
	crumbType          string
	chromeCrumbChannel string
	chromeCrumbType    string
	chromeAuthor       string

	// userNamesV / channelNamesV are bumped every time SetUserNames /
	// SetChannelNames replaces the map. Used by chromeCache (and any other
	// cache that depends on these maps) to detect changes without hashing.
	userNamesV    uint64
	channelNamesV uint64
	userGroups    map[string]string

	workspaceDomain string // fork: profilelink_fork.go

	// bodyCopyLabels is render scratch: the copy labels of the body
	// renderThreadMessage drew last, until View takes them for the entry.
	bodyCopyLabels []messages.CodeBlockCopyLabel

	// attachmentFoldRows is render scratch like bodyCopyLabels.
	// expandedAttachments holds the TS of each message whose cards show
	// in full; SetThread leaves it alone. parentEntry carries the
	// parent's labels and fold rows, which the reply cache does not. See
	// attachmentfold_fork.go.
	attachmentFoldRows  []int
	expandedAttachments map[string]bool
	parentEntry         viewEntry

	// replyIDToIdx maps reply TS -> reply index; rebuilt with the reply
	// cache. HasReply reads it.
	replyIDToIdx map[string]int
	// chromeHeight is the number of rows the thread's chrome (header +
	// separator) occupies at the top of View()'s output. Click and drag
	// handlers subtract it from pane-local rows.
	chromeHeight int

	// unreadBoundaryTS is the Slack timestamp the user has already read up
	// to in this thread. Replies whose TS > unreadBoundaryTS are considered
	// new; a "── new ──" landmark is inserted between the last read reply
	// and the first new one. Empty string disables the landmark. Set via
	// SetUnreadBoundary, typically with the parent channel's last_read_ts
	// at the moment the thread is opened.
	unreadBoundaryTS string

	// version increments on every state change that could alter View() output.
	version int64

	// imgRenderer is the inline-image rendering pipeline. Configured at
	// startup via Model.SetImageContext (mirrors messages.Model). v1
	// renders images inline (kitty + halfblock fully supported; sixel
	// renders placeholder-only) but does not support click-to-preview
	// from a thread reply.
	imgRenderer *imgrender.Renderer

	// emojiCtx bundles the emoji-image rendering dependencies. Threaded
	// through RenderSlackMarkdownWith for reply body text and consulted
	// directly when building reply reaction pills. Configured at startup
	// via Model.SetEmojiContext (mirrors messages.Model).
	emojiCtx EmojiContext
}

// EmojiContext bundles the emoji-image rendering dependencies for the
// thread pane. Mirrors messages.EmojiContext in shape and purpose —
// see internal/ui/messages/model.go for the architectural rationale.
type EmojiContext struct {
	PlaceCtx emojiutil.PlaceContext
	Cells    int               // 1 or 2; 0 falls back to 2
	Customs  map[string]string // workspace custom emoji map; nil = empty
}

// New creates an empty thread panel.
func New() *Model {
	return &Model{
		imgRenderer: imgrender.NewRenderer(),
		list:        newThreadList(),
	}
}

// newThreadList configures the substrate for the thread pane (spec §5):
// the offset clamps eagerly, as bubbles/viewport did (U6); every drag
// step bumps Version (U4); a width change anchors only on replies, so a
// first visible line inside the parent keeps its raw offset (#254) and
// "at the bottom" is measured with the previous frame's height (U5). The
// cursor does not follow the scroll (U1), a shrinking pane is not pinned
// (U2) and a press on a separator anchors nothing (8c): the defaults.
func newThreadList() *entrylist.Model {
	return entrylist.New(
		entrylist.WithStyles(messages.ThemeListStyles()),
		entrylist.WithEagerClamp(),
		entrylist.WithDirtyOnDrag(),
		entrylist.WithReanchor(entrylist.ReanchorPolicy{AnchorRefsOnly: true}),
	)
}

// dropCache discards the reply cache. Until the next View, lookups see
// only the parent, whose entry stays current.
func (m *Model) dropCache() {
	m.cache = nil
	m.replyEntries = nil
	m.list.KeepLookups(1)
}

// SetImageContext configures the inline-image rendering pipeline for
// the thread panel. Triggers a cache rebuild so existing replies
// re-render with the new context. Mirrors messages.Model.SetImageContext.
func (m *Model) SetImageContext(ctx imgrender.ImageContext) {
	if m.imgRenderer == nil {
		m.imgRenderer = imgrender.NewRenderer()
	}
	m.imgRenderer.SetContext(ctx)
	m.InvalidateCache()
}

// SetEmojiContext configures emoji-image rendering for thread replies.
// Should be called once at startup (cmd/slk/main.go) and again after
// CustomEmojisLoadedMsg arrives (via SetEmojiCustoms). Mirrors
// messages.Model.SetEmojiContext.
func (m *Model) SetEmojiContext(ctx EmojiContext) {
	if ctx.Cells != 1 && ctx.Cells != 2 {
		ctx.Cells = 2
	}
	m.emojiCtx = ctx
	m.InvalidateCache()
}

// SetEmojiCustoms updates only the customs map on the active emoji
// context, leaving PlaceCtx and Cells untouched. Invalidates the cache
// so the new map is consulted on the next View(). Mirrors
// messages.Model.SetEmojiCustoms.
func (m *Model) SetEmojiCustoms(customs map[string]string) {
	m.emojiCtx.Customs = customs
	m.InvalidateCache()
}

// HandleEmojiImageReady is invoked by the host (App.Update) when an
// emoji.EmojiImageReadyMsg lands. The same emoji can appear anywhere
// in the open thread (body text or reaction pills), so v1 performs a
// wholesale render-cache invalidation; the next View() rebuilds with
// the now-warm emoji placement. Mirrors messages.Model.HandleEmojiImageReady.
func (m *Model) HandleEmojiImageReady(url string) {
	debuglog.ImgFetch("thread.HandleEmojiImageReady: url=%s wholesale_invalidate", url)
	m.InvalidateCache()
}

// Version returns a counter that increments any time the View() output could
// change. Used by App's panel-output cache.
func (m *Model) Version() int64 { return m.version + m.list.Version() }

// ChromeHeight returns the number of rows the breadcrumb chrome occupies
// at the top of the pane, as of the last View. The app-level mouse
// handler subtracts it from a pane-local row before HitTestReaction.
func (m *Model) ChromeHeight() int { return m.chromeHeight }

func (m *Model) dirty() { m.version++ }

// InvalidateCache forces the render cache to be rebuilt on next View().
// Call this after theme changes or style updates.
func (m *Model) InvalidateCache() {
	m.dropCache()
	m.layoutValid = false
	m.chromeCacheValid = false
	m.dirty()
}

// HandleAvatarReady invalidates the thread render cache so the next
// View() picks up the now-rendered avatar for userID. Mirrors
// messages.Model.HandleAvatarReady. Empty UserID is a no-op.
//
// Workspace-coarse: we don't try to scope by channel/thread identity
// because (a) avatar arrivals are rare and (b) the thread panel only
// renders if it's visible, so an invalidation while hidden is a noop
// at next View() time anyway.
func (m *Model) HandleAvatarReady(userID string) {
	if userID == "" {
		return
	}
	m.InvalidateCache()
}

// SetThread populates the thread panel with a parent message and replies.
// The cursor starts at the bottom (newest reply). When the channel/thread
// identity changes (i.e. the user is opening a different thread, not just
// receiving a refresh of the current one), the unread boundary is cleared
// so a fresh boundary can be set by the caller via SetUnreadBoundary.
// A refresh of the current thread instead keeps the user's cursor and
// scroll where they were — see restoreSelection in select.go.
func (m *Model) SetThread(parent messages.MessageItem, replies []messages.MessageItem, channelID, threadTS string) {
	prev := m.snapshotSelection(channelID, threadTS)
	if channelID != m.channelID || threadTS != m.threadTS {
		m.unreadBoundaryTS = ""
	}
	m.ClearSelection()
	m.parent = parent
	m.replies = replies
	m.channelID = channelID
	m.threadTS = threadTS
	// Per the doc comment, the cursor starts at the bottom (newest reply)
	// so a long thread opens scrolled to the latest activity rather than
	// jammed up at the parent message. A reply-less thread selects the
	// parent (parentSelected) so message ops still have a target.
	if len(replies) > 0 {
		m.selected = len(replies) - 1
	} else {
		m.selected = parentSelected
	}
	// Force the next View() to re-snap the viewport to the new selection.
	// Without this, opening a thread whose newest-reply index matches the
	// previously-viewed thread's snapped selection skips the snap and leaves
	// the viewport scrolled to the top. Mirrors messages.Model.SetMessages.
	m.list.ResetSnap()
	m.restoreSelection(prev)
	m.InvalidateCache()
}

// SetUnreadBoundary sets the timestamp the user has already read up to in
// this thread. A "── new ──" landmark is rendered between the last reply
// with TS <= boundary and the first reply with TS > boundary. Pass "" to
// clear the boundary. Typically called by the App right after SetThread,
// using the thread's own last-read cursor from thread_subscriptions.
func (m *Model) SetUnreadBoundary(ts string) {
	if m.unreadBoundaryTS == ts {
		return
	}
	m.unreadBoundaryTS = ts
	m.layoutValid = false
	m.dirty()
}

// SetBreadcrumb sets the channel the header breadcrumb names.
// channelType picks the glyph (see messages.ChannelGlyph); an empty
// channelName omits the channel segment.
func (m *Model) SetBreadcrumb(channelName, channelType string) {
	if m.crumbChannel == channelName && m.crumbType == channelType {
		return
	}
	m.crumbChannel, m.crumbType = channelName, channelType
	m.dirty()
}

// breadcrumbAuthor is the parent's display name: the parent's own
// UserName, else the userNames entry for its UserID.
func (m *Model) breadcrumbAuthor() string {
	if m.parent.UserName != "" {
		return m.parent.UserName
	}
	name, _ := m.userNames.Get(m.parent.UserID)
	return name
}

// UnreadBoundaryTS returns the current unread-boundary ts. Used by tests.
func (m *Model) UnreadBoundaryTS() string {
	return m.unreadBoundaryTS
}

// AddReply appends a reply to the thread and scrolls to the bottom.
// We always advance `selected` to the new last index so the incoming
// reply is visible regardless of where the user had scrolled.
func (m *Model) AddReply(msg messages.MessageItem) {
	// Idempotent on TS -- same race-defense rationale as
	// messages.Model.AppendMessage: optimistic add (HTTP response) and
	// WS echo can arrive in either order, and a caller-side dedup map
	// can lose the race if the echo lands first.
	if msg.TS != "" {
		for i := len(m.replies) - 1; i >= 0; i-- {
			if m.replies[i].TS == msg.TS {
				return
			}
		}
	}
	m.replies = append(m.replies, msg)
	m.InvalidateCache()
	m.selected = len(m.replies) - 1
}

// SwapLocalSentReply replaces an optimistic placeholder identified
// by localTS (an internal "local:..." id assigned when the user
// pressed Enter on a thread reply, before chat.postMessage returned
// the real Slack TS) with the authoritative msg. Returns true if a
// row matching localTS was found and swapped, false otherwise.
// Mirrors messages.Model.SwapLocalSent.
func (m *Model) SwapLocalSentReply(localTS string, msg messages.MessageItem) bool {
	if localTS == "" {
		return false
	}
	for i := len(m.replies) - 1; i >= 0; i-- {
		if m.replies[i].TS == localTS {
			m.replies[i] = msg
			m.InvalidateCache()
			return true
		}
	}
	return false
}

// RemoveLocalSentReply removes an optimistic placeholder reply
// identified by localTS. Used when the chat.postMessage HTTP call
// fails and we want to roll back the instant-display add. Returns
// true if a row was removed.
func (m *Model) RemoveLocalSentReply(localTS string) bool {
	if localTS == "" {
		return false
	}
	for i := len(m.replies) - 1; i >= 0; i-- {
		if m.replies[i].TS == localTS {
			m.replies = append(m.replies[:i], m.replies[i+1:]...)
			m.InvalidateCache()
			if m.selected >= len(m.replies) && len(m.replies) > 0 {
				m.selected = len(m.replies) - 1
			}
			return true
		}
	}
	return false
}

// UpsertSelfSentReply is the optimistic-add variant of AddReply for
// thread replies the user just sent themselves. If a reply with the
// same TS already exists (e.g. a WS echo arrived faster than the
// HTTP response and AddReply stored its version first), this method
// REPLACES that entry's contents with msg. Otherwise it appends.
//
// Mirrors messages.Model.UpsertSelfSent — see that method for the
// motivating bug (Slack's WS-echo Text may flatten paragraph breaks
// for rich_text_block messages).
func (m *Model) UpsertSelfSentReply(msg messages.MessageItem) {
	if msg.TS != "" {
		for i := len(m.replies) - 1; i >= 0; i-- {
			if m.replies[i].TS == msg.TS {
				m.replies[i] = msg
				m.InvalidateCache()
				return
			}
		}
	}
	m.replies = append(m.replies, msg)
	m.InvalidateCache()
	m.selected = len(m.replies) - 1
}

// Clear resets all thread state.
func (m *Model) Clear() {
	m.ClearSelection()
	m.parent = messages.MessageItem{}
	m.replies = nil
	m.channelID = ""
	m.threadTS = ""
	m.selected = parentSelected
	m.pendingSelectTS = ""
	m.InvalidateCache()
}

// ThreadTS returns the thread timestamp.
func (m *Model) ThreadTS() string {
	return m.threadTS
}

// ChannelID returns the channel ID this thread belongs to.
func (m *Model) ChannelID() string {
	return m.channelID
}

// IsEmpty returns true if no thread is loaded.
func (m *Model) IsEmpty() bool {
	return m.threadTS == ""
}

// HasReply returns true when the open thread contains a reply with the
// given TS. App.Update uses this to decide whether to invalidate the
// thread cache on ImageReadyMsg.
//
// Note: replyIDToIdx is built lazily during View() (see the cache-build
// path), so HasReply may return false for replies whose cache hasn't
// been built yet. That's acceptable for v1 — when the user opens a
// thread, View() runs at least once before any image bytes arrive.
// Returning false when the index is nil is the safe default; the cache
// is rebuilt on the next frame anyway.
func (m *Model) HasReply(ts string) bool {
	if m.replyIDToIdx == nil {
		return false
	}
	_, ok := m.replyIDToIdx[ts]
	return ok
}

// HandleImageFailed clears the in-flight bit for key on the thread's
// renderer and marks it as permanently failed for this session.
// App.Update calls this when an ImageFailedMsg lands so the thread's
// renderer state stays in sync with the messages-pane renderer's.
func (m *Model) HandleImageFailed(key string) {
	if m.imgRenderer == nil {
		debuglog.ImgFetch("thread.HandleImageFailed: thread_ts=%q key=%s SKIP (no renderer)",
			m.threadTS, key)
		return
	}
	tracked := m.imgRenderer.MarkFailed(key)
	debuglog.ImgFetch("thread.HandleImageFailed: thread_ts=%q key=%s was_in_flight=%v",
		m.threadTS, key, tracked)
}

// ReplyCount returns the number of replies.
func (m *Model) ReplyCount() int {
	return len(m.replies)
}

// ParentMsg returns the parent message.
func (m *Model) ParentMsg() messages.MessageItem {
	return m.parent
}

// Replies returns the slice of currently-loaded thread replies. Used by
// the App for cross-pane lookups (e.g. resolving an attachment for the
// preview overlay when the click landed inside the thread panel).
func (m *Model) Replies() []messages.MessageItem {
	return m.replies
}

// UserNames returns the user-ID-to-display-name map used for rendering mentions.
func (m *Model) UserNames() map[string]string { return m.userNames.Current() }

// ChannelNames returns the channel-ID-to-name map used for rendering channel mentions.
func (m *Model) ChannelNames() map[string]string { return m.channelNames }

// UpdateMessageInPlace finds a reply by TS and replaces its text,
// marking it edited. Returns true if found.
func (m *Model) UpdateMessageInPlace(ts, newText string) bool {
	for i, r := range m.replies {
		if r.TS == ts {
			m.replies[i].Text = newText
			m.replies[i].IsEdited = true
			m.InvalidateCache()
			return true
		}
	}
	return false
}

// RemoveMessageByTS removes a reply by TS, adjusting the selected
// index so it remains valid. Returns true if found.
func (m *Model) RemoveMessageByTS(ts string) bool {
	for i, r := range m.replies {
		if r.TS == ts {
			m.replies = append(m.replies[:i], m.replies[i+1:]...)
			if i <= m.selected && m.selected > 0 {
				m.selected--
			}
			if m.selected >= len(m.replies) {
				if len(m.replies) == 0 {
					m.selected = parentSelected
				} else {
					m.selected = len(m.replies) - 1
				}
			}
			m.InvalidateCache()
			return true
		}
	}
	return false
}

// UpdateParentInPlace updates the thread parent's text and marks it
// edited if its TS matches. Returns true if updated.
func (m *Model) UpdateParentInPlace(ts, newText string) bool {
	if m.parent.TS != ts {
		return false
	}
	m.parent.Text = newText
	m.parent.IsEdited = true
	m.InvalidateCache()
	return true
}

// SetFocused sets whether the thread panel has focus. When the value flips
// the layout is invalidated so the selected reply's "▌" border
// re-renders with the appropriate color (Accent when focused, TextMuted when
// not — see styles.SelectionBorderColor).
func (m *Model) SetFocused(focused bool) {
	if m.focused != focused {
		m.focused = focused
		m.layoutValid = false
		m.dirty()
		debuglog.Perf("thread.SetFocused flip focused=%v replies=%d layout=invalidated",
			focused, len(m.replies))
	}
}

// Focused returns whether the thread panel has focus.
func (m *Model) Focused() bool {
	return m.focused
}

// SetAvatarFunc sets the avatar rendering function.
func (m *Model) SetAvatarFunc(fn messages.AvatarFunc) {
	m.avatarFn = fn
}

// SetUserNames sets the user-name store for mention resolution.
// Bumps userNamesV unconditionally so chromeCache (and any other cache
// keyed by this version counter) sees the change via a simple `!=` check.
func (m *Model) SetUserNames(names *usernames.Store) {
	m.userNames = names
	m.userNamesV++
	m.InvalidateCache()
}

// SetColoredUsernames enables or disables deterministic per-user coloring
// of usernames. Invalidates the render cache so existing rows re-render
// with the new coloring.
func (m *Model) SetColoredUsernames(enabled bool) {
	m.coloredUsernames = enabled
	m.InvalidateCache()
}

// SetUserGroups sets the workspace-scoped usergroup ID -> handle map used
// to resolve bare <!subteam^SID> mentions in the parent and replies.
// No-op when the new map matches the current one -- App.SetUserGroups
// fires on every workspace switch, and busting the render cache for an
// identical set is pure waste.
func (m *Model) SetUserGroups(groups map[string]string) {
	if usergroups.Equal(m.userGroups, groups) {
		return
	}
	m.userGroups = usergroups.Copy(groups)
	m.InvalidateCache()
}

// SetCurrentUser records the authenticated user ID so UpdateReaction can
// flag the current user's own reactions (HasReacted) correctly.
func (m *Model) SetCurrentUser(userID string) {
	m.currentUserID = userID
}

// SetUserStatuses replaces the user ID -> custom status map; see
// messages.Model.SetUserStatuses.
func (m *Model) SetUserStatuses(statuses map[string]peerstatus.Status) {
	m.userStatuses = make(map[string]peerstatus.Status, len(statuses))
	for id, st := range statuses {
		m.userStatuses[id] = st
	}
	m.invalidateAuthors()
}

// PatchUserStatus records one user's status; see
// messages.Model.PatchUserStatus.
func (m *Model) PatchUserStatus(userID string, st peerstatus.Status) {
	if userID == "" || m.userStatuses[userID] == st {
		return
	}
	if m.userStatuses == nil {
		m.userStatuses = map[string]peerstatus.Status{}
	}
	m.userStatuses[userID] = st
	if m.hasAuthor(userID) {
		m.invalidateAuthors()
	}
}

// ExpireStatuses drops statuses whose deadline has passed and reports
// whether an author name rendered in this pane changed.
func (m *Model) ExpireStatuses(now time.Time) bool {
	changed := false
	for uid, st := range m.userStatuses {
		if st.Expired(now) {
			m.userStatuses[uid] = st.Clear(now)
			if m.hasAuthor(uid) {
				changed = true
			}
		}
	}
	if changed {
		m.invalidateAuthors()
	}
	return changed
}

func (m *Model) hasAuthor(userID string) bool {
	if m.parent.UserID == userID {
		return true
	}
	for i := range m.replies {
		if m.replies[i].UserID == userID {
			return true
		}
	}
	return false
}

// invalidateAuthors forces every cached message header to re-render.
// userNamesV is bumped because the chrome cache renders the parent
// message's header too.
func (m *Model) invalidateAuthors() {
	m.userNamesV++
	m.dropCache()
	m.layoutValid = false
}

// PatchUserName updates the user-name store (used for @mention
// rendering) and overwrites the UserName field on the parent message
// and every cached reply authored by userID. Always invalidates the
// render cache so mentions of <@userID> in other authors' text
// re-resolve.
//
// Mirrors messages.Model.PatchUserName, including why it always
// applies: the map is normally the App's, shared by every pane and
// already holding the name by the time this pane is patched. Used by
// the async user-resolution path: history fetchers stash
// MessageItem.UserName = m.UserID for unknown authors. When the
// resolution returns asynchronously, the App calls PatchUserName to
// replace the placeholders live without re-fetching the thread.
func (m *Model) PatchUserName(userID, displayName string) {
	if userID == "" {
		return
	}
	if m.userNames == nil {
		m.userNames = usernames.NewStore()
	}
	m.userNames.Set(userID, displayName)
	// The render cache stores rows with their mentions already resolved
	// (RenderSlackMarkdown consults userNames at render time), so any
	// cached row that mentioned <@userID> is now stale. Mirror
	// SetUserNames's behavior: invalidate unconditionally on map change.
	// Bump userNamesV so chromeCache (which renders the parent message
	// and consults userNames for its mentions) also rebuilds.
	m.userNamesV++
	m.dropCache()
	m.layoutValid = false
	if m.parent.UserID == userID && m.parent.UserName != displayName {
		m.parent.UserName = displayName
	}
	for i := range m.replies {
		if m.replies[i].UserID == userID && m.replies[i].UserName != displayName {
			m.replies[i].UserName = displayName
		}
	}
	m.dirty()
}

// SetChannelNames sets the channel ID -> name map used to resolve bare
// <#CHANNELID> mentions in thread replies. Bumps channelNamesV
// unconditionally; see SetUserNames for the rationale.
func (m *Model) SetChannelNames(names map[string]string) {
	m.channelNames = names
	m.channelNamesV++
	m.InvalidateCache()
}

// SelectedReply returns the currently selected reply, or nil if none.
func (m *Model) SelectedReply() *messages.MessageItem {
	// The parent is a selectable row like any reply — returning it here
	// makes every SelectedReply-driven message op (react, permalink,
	// yank, open links) work on the thread's first message too.
	if m.selected == parentSelected && m.parent.TS != "" {
		return &m.parent
	}
	if m.selected < 0 || m.selected >= len(m.replies) {
		return nil
	}
	return &m.replies[m.selected]
}

// SelectByIndex moves the selection cursor to i (an index into Replies()).
// No-op if i is out of range. Used by tests that need a deterministic
// selection state.
func (m *Model) SelectByIndex(i int) {
	if i < 0 || i >= len(m.replies) {
		return
	}
	if m.selected != i {
		m.selected = i
		m.InvalidateCache()
	}
}

// MoveUp moves the selection cursor up one reply.
// ScrollUp scrolls the thread viewport up by n lines without changing the
// selected reply. Marks the current selection as already-snapped so View()
// won't pull the viewport back to keep selection visible on the next render.
func (m *Model) ScrollUp(n int) {
	m.list.ScrollUp(n, m.selected)
}

// ScrollDown scrolls the thread viewport down by n lines without changing the
// selected reply. Marks the current selection as already-snapped so View()
// won't pull the viewport back on the next render.
func (m *Model) ScrollDown(n int) {
	m.list.ScrollDown(n, m.selected)
}

// ViewportAtTop reports whether the thread viewport is scrolled to the top.
// Used by the app layer to detect a wheel-up / PageUp that hit the top of the
// reply stream (cosmetic only -- no thread backfill exists today).
func (m *Model) ViewportAtTop() bool {
	return m.list.YOffset() == 0
}

func (m *Model) MoveUp() {
	if m.reactionNavActive {
		m.ExitReactionNav()
	}
	// The parent row (parentSelected, index -1) sits above the first
	// reply, so the cursor can move one step past index 0.
	if m.selected > parentSelected {
		m.selected--
		m.dirty()
	}
}

// MoveDown moves the selection cursor down one reply.
func (m *Model) MoveDown() {
	if m.reactionNavActive {
		m.ExitReactionNav()
	}
	if m.selected < len(m.replies)-1 {
		m.selected++
		m.dirty()
	}
}

func (m *Model) IsAtBottom() bool {
	return m.selected >= len(m.replies)-1
}

// GoToTop moves the selection to the parent, the topmost row.
func (m *Model) GoToTop() {
	if m.selected != parentSelected {
		m.selected = parentSelected
		m.dirty()
	}
}

// GoToBottom moves the selection to the last reply.
func (m *Model) GoToBottom() {
	if len(m.replies) > 0 && m.selected != len(m.replies)-1 {
		m.selected = len(m.replies) - 1
		m.dirty()
	}
}

// EnterReactionNav activates reaction navigation on the selected reply.
func (m *Model) EnterReactionNav() {
	if reply := m.SelectedReply(); reply != nil && len(reply.Reactions) > 0 {
		m.reactionNavActive = true
		m.reactionNavIndex = 0
		m.InvalidateCache()
	}
}

// ExitReactionNav deactivates reaction navigation.
func (m *Model) ExitReactionNav() {
	m.reactionNavActive = false
	m.reactionNavIndex = 0
	m.InvalidateCache()
}

// ReactionNavActive returns whether reaction navigation is active.
func (m *Model) ReactionNavActive() bool {
	return m.reactionNavActive
}

// ReactionNavLeft moves the reaction cursor left with wrapping.
func (m *Model) ReactionNavLeft() {
	reply := m.SelectedReply()
	if reply == nil {
		return
	}
	total := len(reply.Reactions) + 1
	m.reactionNavIndex = (m.reactionNavIndex - 1 + total) % total
	m.InvalidateCache()
}

// ReactionNavRight moves the reaction cursor right with wrapping.
func (m *Model) ReactionNavRight() {
	reply := m.SelectedReply()
	if reply == nil {
		return
	}
	total := len(reply.Reactions) + 1
	m.reactionNavIndex = (m.reactionNavIndex + 1) % total
	m.InvalidateCache()
}

// SelectedReaction returns the currently highlighted reaction emoji name,
// or isPlus=true if the "+" button is highlighted.
func (m *Model) SelectedReaction() (emojiName string, isPlus bool) {
	reply := m.SelectedReply()
	if reply == nil {
		return "", false
	}
	if m.reactionNavIndex >= len(reply.Reactions) {
		return "", true
	}
	return reply.Reactions[m.reactionNavIndex].Emoji, false
}

// ClampReactionNav ensures the reaction nav index is within bounds.
func (m *Model) ClampReactionNav() {
	reply := m.SelectedReply()
	if reply == nil || len(reply.Reactions) == 0 {
		m.ExitReactionNav()
		return
	}
	total := len(reply.Reactions) + 1
	if m.reactionNavIndex >= total {
		m.reactionNavIndex = total - 1
	}
	m.InvalidateCache()
}

// UpdateReaction updates the reaction state for a specific message in the thread.
func (m *Model) UpdateReaction(messageTS, emojiName, userID string, remove bool) {
	var msg *messages.MessageItem
	if m.parent.TS == messageTS {
		msg = &m.parent
	} else {
		for i := range m.replies {
			if m.replies[i].TS == messageTS {
				msg = &m.replies[i]
				break
			}
		}
	}
	if msg == nil {
		return
	}

	m.updateMessageReaction(msg, emojiName, userID, remove)
	m.InvalidateCache()
	if m.reactionNavActive {
		m.ClampReactionNav()
	}
}

// updateMessageReaction applies one idempotent reaction event to msg.
func (m *Model) updateMessageReaction(msg *messages.MessageItem, emojiName, userID string, remove bool) {
	if remove {
		for j, r := range msg.Reactions {
			if r.Emoji != emojiName {
				continue
			}
			newIDs := messages.RemoveUserID(r.UserIDs, userID)
			if len(newIDs) == len(r.UserIDs) && r.Count <= len(r.UserIDs) {
				return
			}
			r.UserIDs = newIDs
			r.Count--
			if userID == m.currentUserID {
				r.HasReacted = false
			}
			if r.Count <= 0 {
				msg.Reactions = append(msg.Reactions[:j], msg.Reactions[j+1:]...)
			} else {
				msg.Reactions[j] = r
			}
			return
		}
		return
	}

	for j, r := range msg.Reactions {
		if r.Emoji != emojiName {
			continue
		}
		newIDs := messages.AppendUserID(r.UserIDs, userID)
		if len(newIDs) != len(r.UserIDs) {
			r.UserIDs = newIDs
			r.Count++
		}
		if userID == m.currentUserID {
			r.HasReacted = true
		}
		msg.Reactions[j] = r
		return
	}
	msg.Reactions = append(msg.Reactions, messages.ReactionItem{
		Emoji:      emojiName,
		Count:      1,
		HasReacted: userID == m.currentUserID,
		UserIDs:    messages.AppendUserID(nil, userID),
	})
}

// ClickAt handles a mouse click at the given y-coordinate (the pane-local
// y returned by App.panelAt — measured from the panel's top border, so
// y=0..chromeHeight-1 sits inside the chrome (header + separator) and
// y=chromeHeight onward is the scrolling parent+replies content). Clicks
// in the chrome are ignored. A click below the content lands on the last
// line (the row is clamped), so it selects the last reply; a click on a
// separator, a divider or the rule under the parent selects nothing.
func (m *Model) ClickAt(y int) {
	if y < m.chromeHeight {
		return
	}
	i, within, ok := m.list.EntryAt(m.list.AbsLine(y - m.chromeHeight))
	if !ok {
		return
	}
	e := m.list.Entries()[i]
	if !e.Selectable || within >= len(e.Plain) {
		return
	}
	if m.selected != e.Ref {
		m.selected = e.Ref
		m.layoutValid = false
		m.dirty()
	}
}

// BeginSelectionAt anchors a new selection at the given pane-local
// coordinates (App.panelAt's coordinate system). Chrome rows are ignored;
// a press on a separator or divider anchors nothing (thread does not snap
// to the nearest message).
func (m *Model) BeginSelectionAt(viewportY, x int) {
	if viewportY < m.chromeHeight {
		return
	}
	m.list.BeginSelection(m.list.AbsLine(viewportY-m.chromeHeight), x)
}

// ExtendSelectionAt updates the End anchor of the active selection.
// No-op if BeginSelectionAt was never called or the coordinates fall
// on a non-entry row (inter-reply separator).
func (m *Model) ExtendSelectionAt(viewportY, x int) {
	m.list.ExtendSelection(m.list.AbsLine(viewportY-m.chromeHeight), x)
}

// EndSelection finalizes the drag, returning the plain-text contents
// of the selection. Returns ok=false when the selection is empty
// (a click without drag).
func (m *Model) EndSelection() (string, bool) { return m.list.EndSelection() }

// ClearSelection removes the current selection, if any.
func (m *Model) ClearSelection() { m.list.ClearSelection() }

// HasSelection reports whether a selection is currently active or
// pinned-on-screen post-drag.
func (m *Model) HasSelection() bool { return m.list.HasSelection() }

// SelectionText extracts the plain-text contents of the current
// selection.
func (m *Model) SelectionText() string { return m.list.SelectionText() }

// selectedLines returns the rows to draw for the cursor's entry i. For a
// reply: the long-timestamp variant, or the short one while a text
// selection exists (the drag overlay splices the short-form plain text
// into the row by column). The parent's rows are re-rendered every frame
// for its current state, so they are drawn as they are.
//
// The short variant is memoised in m.cache.
func (m *Model) selectedLines(i int) []string {
	e := &m.list.Entries()[i]
	if e.Ref < 0 || e.Ref >= len(m.cache) {
		return e.Lines
	}
	p := &m.cache[e.Ref]
	if !m.list.HasSelection() || p.shortSrc == "" {
		return e.Selected
	}
	if p.linesSelectedShort == nil {
		short := m.selectedVariant(p.shortSrc, m.cacheWidth, m.borderSelectStyle())
		if len(short) != len(e.Selected) {
			return e.Selected // unreachable: variants share a height
		}
		p.linesSelectedShort = short
	}
	return p.linesSelectedShort
}

// borderSelectStyle is the thick, tinted left border of the selected reply.
func (m *Model) borderSelectStyle() lipgloss.Style {
	return lipgloss.NewStyle().BorderStyle(thickLeftBorder).BorderLeft(true).
		BorderForeground(styles.SelectionBorderColor(m.focused)).
		BorderBackground(styles.SelectionTintColor(m.focused)).
		Background(styles.SelectionTintColor(m.focused))
}

// selectedVariant renders content r as the selected reply: the selection
// tint repainted over inner spans, filled to the row width, then the
// selection border. Split on "\n". Mirrors messages.Model.selectedVariant.
func (m *Model) selectedVariant(r string, width int, borderSelect lipgloss.Style) []string {
	fill := lipgloss.NewStyle().Background(styles.SelectionTintColor(m.focused)).Width(width - 1)
	tinted := messages.ReapplyBgAfterResets(messages.RepaintBgToSelectionTint(r, m.focused), messages.SelectionTintBgANSI(m.focused))
	return strings.Split(borderSelect.Render(fill.Render(tinted)), "\n")
}

// ScrollHintForDrag returns -1 if the cursor is within 1 row of the top
// edge of the scrolling content, +1 if within 1 row of the bottom, else 0.
// The incoming viewportY is pane-local; a cursor on the chrome counts as
// the top content row, so an upward drag keeps auto-scrolling.
func (m *Model) ScrollHintForDrag(viewportY int) int {
	return m.list.ScrollHint(viewportY - m.chromeHeight)
}

// View renders the thread panel content without a border.
// The parent App is responsible for adding the border.
func (m *Model) View(height, width int) string {
	if m.IsEmpty() {
		return lipgloss.NewStyle().
			Width(width).
			Height(height).
			Background(styles.Background).
			Foreground(styles.TextMuted).
			Render("No thread selected")
	}

	// Chrome: header + separator + parent message + separator. Cached
	// because the parent's full markdown pipeline (RenderSlackMarkdown +
	// WordWrap + reactions + attachments) is expensive and identical
	// frame-to-frame on a plain j/k. Mirrors internal/ui/messages's
	// chromeCache.
	//
	// Invariant: any mutation of m.parent.TS, m.parent.Text, m.userNames
	// (which bumps userNamesV), m.channelNames (which bumps channelNamesV),
	// or len(m.replies) MUST be paired with InvalidateCache() or a bump of
	// the relevant version counter — otherwise the chrome will go stale.
	// Today SetThread, AddReply, UpdateMessageInPlace, UpdateParentInPlace,
	// SetUserNames, and SetChannelNames all honor this. If you add a new
	// visual element to the chrome (e.g. parent reactions, parent edited
	// marker, attachments rendered above the separator), either add its
	// state to the predicate below or invalidate on its mutation path.
	// Adding to the predicate is preferred when the state changes
	// infrequently — chrome is rebuilt rarely, so the comparison cost is
	// negligible and invalidation discipline is easier to get wrong.
	// An ephemeral reply is not a reply: the header counts the rest.
	chromeReplyCount := 0
	for _, r := range m.replies {
		if !r.Ephemeral {
			chromeReplyCount++
		}
	}
	// Chrome no longer contains the parent message -- the parent is the
	// first entry of the list so it scrolls with the replies (a long
	// parent must not pin and block the reply area). Chrome is only the
	// header line + a single border separator; everything else lives in
	// the list, so parent identity no longer participates in the
	// chrome cache key.
	author := m.breadcrumbAuthor()
	if !m.chromeCacheValid ||
		m.chromeWidth != width ||
		m.chromeReplyCount != chromeReplyCount ||
		m.chromeUserNamesV != m.userNamesV ||
		m.chromeChannelNamesV != m.channelNamesV ||
		m.chromeCrumbChannel != m.crumbChannel ||
		m.chromeCrumbType != m.crumbType ||
		m.chromeAuthor != author {
		header := renderBreadcrumb(width, m.crumbChannel, m.crumbType, author, chromeReplyCount)
		separator := lipgloss.NewStyle().
			Width(width).
			Background(styles.Background).
			Foreground(styles.Border).
			Render(strings.Repeat("-", width))
		m.chromeCache = header + "\n" + separator
		m.chromeHeight = lipgloss.Height(m.chromeCache)
		m.chromeCacheValid = true
		m.chromeWidth = width
		m.chromeReplyCount = chromeReplyCount
		m.chromeUserNamesV = m.userNamesV
		m.chromeChannelNamesV = m.channelNamesV
		m.chromeCrumbChannel = m.crumbChannel
		m.chromeCrumbType = m.crumbType
		m.chromeAuthor = author
	}
	chrome := m.chromeCache
	chromeHeight := m.chromeHeight

	// The reply area is everything under the chrome.
	replyAreaHeight := max(height-chromeHeight, 1)
	m.list.SetStyles(messages.ThemeListStyles())
	m.list.SetHeight(replyAreaHeight, m.selected)

	// The parent is rendered every frame for its current state (selected
	// or not, focused or not) and is entry 0 of the list: its rows, then
	// the rule under it. The rule is not selectable text (Plain stops
	// before it). The parent renders on every View, so its image uploads
	// go out here rather than with the replies'. We discard the parent's
	// reaction hits in v1, as before.
	parentIsSelected := m.selected == parentSelected
	parentContent, parentFlushes, _, parentHeader, parentHeaderBudget := m.renderThreadMessage(m.parent, width, m.userNames.Current(), m.channelNames, parentIsSelected)
	for _, fl := range parentFlushes {
		if fl != nil {
			_ = fl(imgpkg.KittyOutput)
		}
	}
	m.parentEntry = viewEntry{
		codeBlockCopyLabels: m.takeBodyCopyLabels(),
		attachmentFoldRows:  m.takeAttachmentFoldRows(),
	}
	// Replies only get a day divider when their day differs from the
	// parent's, so the parent header carries its own date; otherwise a
	// thread whose first replies share the parent's day shows no date.
	// Before the plain mirror, so copy and drag highlight match what is
	// drawn. Skipped when the header would not fit, and for a
	// summary-built parent that has no time yet.
	if !parentIsSelected && m.parent.Timestamp != "" {
		if date := messages.DateFromTS(m.parent.TS); date != "" {
			dated := messages.FormatShortDate(date) + ", " + m.parent.Timestamp
			parentContent = messages.ReplaceHeaderTimestamp(parentContent, parentHeader, m.parent.Timestamp, dated, parentHeaderBudget)
		}
	}
	parentPlain := messages.PlainLines(parentContent)
	// Selection border for the parent row mirrors the per-reply
	// cache-build treatment (borderSelect / borderInvis below): thick
	// left border + tint when the cursor sits on the parent, invisible
	// border otherwise so the parent's width matches the reply rows.
	if parentIsSelected {
		// After the plain mirror, so it keeps the short form. Skipped
		// while a text selection exists; see selectedLines.
		if !m.list.HasSelection() {
			parentContent = messages.SelectedHeader(parentContent, parentHeader, m.parent.TS, m.parent.Timestamp, parentHeaderBudget)
		}
		parentContent = lipgloss.NewStyle().BorderStyle(thickLeftBorder).BorderLeft(true).
			BorderForeground(styles.SelectionBorderColor(m.focused)).
			BorderBackground(styles.SelectionTintColor(m.focused)).
			Background(styles.SelectionTintColor(m.focused)).
			Render(parentContent)
	} else {
		parentContent = lipgloss.NewStyle().BorderStyle(thickLeftBorder).BorderLeft(true).
			BorderForeground(styles.Background).BorderBackground(styles.Background).
			Render(parentContent)
	}
	// Pad every parent row to the pane width with the theme background.
	// The outer wrapper used to do this (the parent, unlike a reply, is
	// not width-filled when rendered); this is the same per-row padding,
	// applied where it is needed.
	parentContent = lipgloss.NewStyle().Width(width).Background(styles.Background).Render(parentContent)
	parentSeparator := lipgloss.NewStyle().
		Width(width).
		Background(styles.Background).
		Foreground(styles.Border).
		Render(strings.Repeat("─", width))
	parent := entrylist.Entry{
		ID:         m.parent.TS,
		Ref:        parentSelected,
		Selectable: true,
		Lines:      append(strings.Split(parentContent, "\n"), parentSeparator),
		ColOffset:  1,
	}
	if m.parent.TS != "" {
		// A parent without a ts cannot be clicked or text-selected.
		parent.Plain = parentPlain
	}

	if len(m.replies) == 0 {
		// "No replies yet" under the parent. It goes through the list so
		// a parent taller than the pane still scrolls; the scrollbar
		// reflects the parent's own height. No snap, no hits, no
		// selection overlay: as before (plan decision P1 for the totals).
		emptyHeight := max(replyAreaHeight-len(parent.Lines), 1)
		empty := lipgloss.NewStyle().
			Width(width).
			Height(emptyHeight).
			Background(styles.Background).
			Foreground(styles.TextMuted).
			Render("No replies yet")
		m.list.SetEntries([]entrylist.Entry{parent, {Ref: -1, Lines: strings.Split(empty, "\n")}}, width, false)
		rows := m.list.Compose(m.list.Static(), "", "", false)
		return fitHeight(chrome+"\n"+strings.Join(rows, "\n"), height)
	}

	// A width change with the previous layout intact keeps the scroll
	// position (SetEntries, per the reanchor policy). Captured before the
	// reply-cache rebuild.
	keepPosition := m.layoutValid

	// Rebuild the reply cache if the replies or the width changed.
	//
	// The predicate intentionally excludes m.selected, m.focused,
	// m.reactionNavActive/Index and the theme version. The cache holds
	// renderThreadMessage output, which depends on those fields, so any
	// state that can change rendered output for a given (reply, width)
	// MUST call InvalidateCache() on its mutation path. EnterReactionNav /
	// ExitReactionNav / ReactionNavLeft / ReactionNavRight all do;
	// MoveUp/MoveDown call ExitReactionNav before mutating selected.
	// (SetFocused does not: plan decision P2, a known stale-border bug.)
	if m.cache == nil || m.cacheWidth != width || m.cacheReplyLen != len(m.replies) {
		var perfReason string
		var perfStart time.Time
		if debuglog.Enabled() {
			switch {
			case m.cache == nil:
				perfReason = "cache-nil"
			case m.cacheWidth != width:
				perfReason = fmt.Sprintf("width-changed %d->%d", m.cacheWidth, width)
			default:
				perfReason = fmt.Sprintf("len-changed %d->%d", m.cacheReplyLen, len(m.replies))
			}
			perfStart = time.Now()
		}
		m.cache = m.cache[:0]
		m.replyEntries = m.replyEntries[:0]
		if m.replyIDToIdx == nil {
			m.replyIDToIdx = make(map[string]int, len(m.replies))
		} else {
			clear(m.replyIDToIdx)
		}
		// Pre-build border styles ONCE (don't allocate per reply): the
		// thick left border ▌ is applied at cache-build time so View()
		// picks a slice (normal vs selected) instead of running lipgloss
		// for every visible reply on every j/k.
		borderFill := lipgloss.NewStyle().Background(styles.Background)
		borderInvis := lipgloss.NewStyle().BorderStyle(thickLeftBorder).BorderLeft(true).
			BorderForeground(styles.Background).BorderBackground(styles.Background)
		borderSelect := m.borderSelectStyle()
		for i, reply := range m.replies {
			// renderThreadMessage's last arg ("isSelected") drives reaction-
			// nav pill highlighting, so it MUST be i == m.selected.
			rendered, attachFlushes, reactHits, header, headerBudget := m.renderThreadMessage(reply, width, m.userNames.Current(), m.channelNames, i == m.selected)
			// Selected variant only; Lines / Plain keep the short form.
			renderedSel := messages.SelectedHeader(rendered, header, reply.TS, reply.Timestamp, headerBudget)
			// Two filled variants, so the trailing whitespace of every
			// wrapped line carries the right background and the tint
			// reaches the end of the row. Plain mirrors the untinted
			// filledNormal, so clipboard text never carries the tint.
			filledNormal := borderFill.Width(width - 1).Render(rendered)
			linesN := strings.Split(borderInvis.Render(filledNormal), "\n")
			linesS := m.selectedVariant(renderedSel, width, borderSelect)
			// The short-timestamp selected variant is built on demand
			// from shortSrc (see selectedLines).
			var shortSrc string
			if renderedSel != rendered {
				shortSrc = rendered
			}
			hits := make([]entrylist.Hit, 0, len(reactHits))
			for _, h := range reactHits {
				hits = append(hits, entrylist.Hit{
					RowStart: h.rowStartInEntry, RowEnd: h.rowEndInEntry,
					ColStart: h.colStart, ColEnd: h.colEnd,
					Kind: entrylist.HitReaction, Key: h.emoji,
				})
			}
			m.replyEntries = append(m.replyEntries, entrylist.Entry{
				ID:         reply.TS,
				Ref:        i,
				Selectable: true,
				Lines:      linesN,
				Selected:   linesS,
				Plain:      messages.PlainLines(filledNormal),
				ColOffset:  1,
				Hits:       hits,
			})
			m.cache = append(m.cache, viewEntry{shortSrc: shortSrc, flushes: attachFlushes,
				codeBlockCopyLabels: m.takeBodyCopyLabels(),     // fork: codeblocks_fork.go
				attachmentFoldRows:  m.takeAttachmentFoldRows(), // fork: attachmentfold_fork.go
			})
			m.replyIDToIdx[reply.TS] = i
		}
		m.cacheWidth = width
		m.cacheReplyLen = len(m.replies)
		m.flushKey.selected = math.MinInt // re-emit the kitty uploads
		if debuglog.Enabled() {
			debuglog.Perf("thread.buildCache N=%d width=%d took=%s reason=%s",
				len(m.replies), width, time.Since(perfStart), perfReason)
		}
	}

	// The list: the parent, then for each reply its day divider (when
	// its local day differs from the previous one, seeded with the
	// parent's), the "── new ──" landmark before the first reply past
	// the unread boundary, the reply, and a full-width rule between
	// replies. Dividers, landmark and rules are decorations: not
	// selectable, not text-selectable.
	ruleStyle := lipgloss.NewStyle().
		Width(width).
		Background(styles.Background).
		Foreground(styles.Border)
	replySeparator := ruleStyle.Render(strings.Repeat("─", width))
	var newLandmark string
	if m.unreadBoundaryTS != "" {
		newLandmark = lipgloss.NewStyle().
			Width(width).
			Background(styles.Background).
			Foreground(styles.Error).
			Bold(true).
			Align(lipgloss.Center).
			Render("── new ──")
	}
	dateSeparatorStyle := lipgloss.NewStyle().
		Width(width).
		Background(styles.Background).
		Foreground(styles.TextMuted).
		Bold(true).
		Align(lipgloss.Center)
	decoration := func(row string) entrylist.Entry { return entrylist.Entry{Ref: -1, Lines: []string{row}} }
	entries := make([]entrylist.Entry, 0, 2*len(m.replyEntries)+2)
	entries = append(entries, parent)
	lastDate := messages.DateFromTS(m.parent.TS)
	landmarkInserted := false
	for i, e := range m.replyEntries {
		if replyDate := messages.DateFromTS(m.replies[i].TS); replyDate != "" && replyDate != lastDate {
			entries = append(entries, decoration(dateSeparatorStyle.Render("── "+messages.FormatDateSeparator(replyDate)+" ──")))
			lastDate = replyDate
		}
		if !landmarkInserted && newLandmark != "" && m.replies[i].TS > m.unreadBoundaryTS {
			entries = append(entries, decoration(newLandmark))
			landmarkInserted = true
		}
		entries = append(entries, e)
		if i < len(m.replyEntries)-1 {
			entries = append(entries, decoration(replySeparator))
		}
	}
	m.list.SetEntries(entries, width, keepPosition)

	// Kitty uploads for every reply (the thread cache is small, so no
	// visibility scoping), written straight to the terminal output: APC
	// sequences embedded in line content get mangled by the renderer.
	// Re-emitted only when the layout changes, as before.
	key := m.flushKey
	key.selected, key.width, key.height, key.hasSelection = m.selected, width, replyAreaHeight, m.list.HasSelection()
	if key != m.flushKey || !m.layoutValid {
		var kittyFlushBuf bytes.Buffer
		for _, p := range m.cache {
			for _, fl := range p.flushes {
				if fl != nil {
					_ = fl(&kittyFlushBuf)
				}
			}
		}
		if kittyFlushBuf.Len() > 0 {
			_, _ = imgpkg.KittyOutput.Write(kittyFlushBuf.Bytes())
		}
		m.flushKey = key
	}

	// Snap to the cursor if it moved, slice the window, translate the
	// reply hits (content-relative), overlay the selection, draw the
	// scrollbar.
	frame := m.list.Layout(m.selected, m.selectedLines)
	rows := m.list.Compose(frame, "", "", true)
	m.layoutValid = true
	return fitHeight(chrome+"\n"+strings.Join(rows, "\n"), height)
}

// HitTestReaction returns the (reply index, reaction emoji name) at
// (row, col) within the thread's content area, or ok=false when no
// reaction pill covers that cell. Coordinate frame: row 0 is the first
// row under the chrome, as for messages.Model.HitTestReaction (the
// app-level mouse handler subtracts ChromeHeight). Hits come from the
// most recent View().
func (m *Model) HitTestReaction(row, col int) (replyIdx int, emoji string, ok bool) {
	h, ok := m.list.HitTest(entrylist.HitReaction, row, col)
	if !ok {
		return 0, "", false
	}
	return h.Ref, h.Key, true
}

// fitHeight keeps the first height rows of s, as the old outer
// MaxHeight(height) did when a floored reply area (height < chrome + 1)
// made the pane one row too tall.
func fitHeight(s string, height int) string {
	if height <= 0 {
		return s
	}
	if rows := strings.Split(s, "\n"); len(rows) > height {
		return strings.Join(rows[:height], "\n")
	}
	return s
}

// renderThreadMessage renders a single message for the thread panel.
// Returns the content string, any per-frame kitty flush callbacks for
// inline image attachments, and the per-pill hit rects for the
// rendered reactions (in coordinates relative to the rendered content
// AFTER buildCache wraps it with the thick left border in column 0).
// The flushes are consumed by View() when the entry is visible
// (mirroring messages.Model). v1: per-block Hit and SixelRows from
// imgrender are discarded — click-to-preview from a thread reply and
// inline sixel emission are out of scope.
// blockkitContext wires the thread pane's host dependencies into a
// blockkit.Context. Mirrors messages.Model.blockkitContext so Block
// Kit blocks and legacy attachments render identically in the thread
// panel and the main message pane.
func (m *Model) blockkitContext(msg messages.MessageItem, userNames, channelNames map[string]string, emojiFlushes *[]func(io.Writer) error) blockkit.Context {
	var imgCtx imgrender.ImageContext
	if m.imgRenderer != nil {
		imgCtx = m.imgRenderer.Context()
	}
	send := imgCtx.SendMsg
	return blockkit.Context{
		Protocol:    imgCtx.Protocol,
		Fetcher:     imgCtx.Fetcher,
		KittyRender: imgCtx.KittyRender,
		CellPixels:  imgCtx.CellPixels,
		MaxRows:     imgCtx.MaxRows,
		MaxCols:     imgCtx.MaxCols,
		UserNames:   userNames,
		MessageTS:   msg.TS,
		Channel:     m.channelID,
		Card:        messages.CardContext(m.expandedAttachments[msg.TS], channelNames),
		RenderTextForWidth: func(s string, un map[string]string, width int) string {
			return messages.RenderSlackMarkdownWith(s, messages.RenderSlackMarkdownOpts{
				UserNames:    un,
				ChannelNames: channelNames,
				UserGroups:   m.userGroups,
				PlaceCtx:     m.emojiCtx.PlaceCtx,
				EmojiCells:   m.emojiCtx.Cells,
				Customs:      m.emojiCtx.Customs,
				EmojiFlushes: emojiFlushes,
				Width:        width,

				WorkspaceDomain: m.workspaceDomain,
			})
		},
		WrapText: messages.WordWrap,
		SendMsg: func(v any) {
			if send != nil {
				send(v)
			}
		},
	}
}

func (m *Model) renderThreadMessage(msg messages.MessageItem, width int, userNames map[string]string, channelNames map[string]string, isSelected bool) (string, []func(io.Writer) error, []reactionEntryHit, string, int) {
	line := styles.Username(msg.UserID, m.coloredUsernames).Render(msg.UserName) + messages.AuthorStatusSuffix(m.userStatuses, msg.UserID, time.Now()) + lipgloss.NewStyle().Background(styles.Background).Render("  ") + styles.Timestamp.Render(msg.Timestamp)

	// Label rows above the author line; mirrors the messages pane.
	labelRows, labelRowCount := "", 0
	if msg.Ephemeral {
		labelRows, labelRowCount = messages.EphemeralLabel()+"\n", 1
	}

	contentWidth := width - 4
	if contentWidth < 20 {
		contentWidth = 20
	}

	// flushes aggregates per-frame side effects (kitty image upload
	// escapes) from body-text emoji, reaction-pill emoji, and inline
	// image attachments. Walked by the View() loop for visible entries.
	// Mirrors the messages-pane named-return `flushes` slice.
	var flushes []func(io.Writer) error
	bodyOpts := messages.RenderSlackMarkdownOpts{
		UserNames:    userNames,
		ChannelNames: channelNames,
		UserGroups:   m.userGroups,
		PlaceCtx:     m.emojiCtx.PlaceCtx,
		EmojiCells:   m.emojiCtx.Cells,
		Customs:      m.emojiCtx.Customs,
		EmojiFlushes: &flushes,
		Width:        contentWidth,

		CodeBlockCopyLabels: m.newBodyCopyLabels(),
		PermalinkChips:      messages.PermalinkChipsOf(msg, channelNames),
		WorkspaceDomain:     m.workspaceDomain,
	}
	// Match the main pane: content-bearing blocks suppress the fallback
	// text and its row. See messages.BlocksCarryBody.
	hasBody := !messages.BlocksCarryBody(msg)
	bodySrc := messages.MessageTextSource(msg)
	if !hasBody {
		bodySrc = ""
	}
	text := styles.MessageText.Render(messages.WordWrap(messages.RenderSlackMarkdownWith(bodySrc, bodyOpts), contentWidth))
	bodyRow, bodyRows := "", 0
	if hasBody {
		bodyRow, bodyRows = "\n"+text, lipgloss.Height(text)
	}

	// Block Kit blocks + legacy attachments render between the body
	// text and file attachments, mirroring the main message pane's
	// renderMessagePlain. Image flushes are aggregated into the shared
	// `flushes` slice; per-image Hit/SixelRows are discarded (v1 thread
	// rendering, consistent with the file-attachment path below).
	bkCtx := m.blockkitContext(msg, userNames, channelNames, &flushes)
	var bkLines []string
	bkInteractive := false
	if len(msg.Blocks) > 0 {
		res := blockkit.RenderMessageBlocks(msg.Blocks, bkCtx, contentWidth)
		bkLines = append(bkLines, res.Lines...)
		flushes = append(flushes, res.Flushes...)
		bkInteractive = bkInteractive || res.Interactive
	}
	if len(msg.LegacyAttachments) > 0 {
		res := blockkit.RenderLegacy(msg.LegacyAttachments, bkCtx, contentWidth)
		m.attachmentFoldRows = messages.OffsetAttachmentFoldRows(res.FoldRows, 1+bodyRows+len(bkLines))
		bkLines = append(bkLines, res.Lines...)
		flushes = append(flushes, res.Flushes...)
		bkInteractive = bkInteractive || res.Interactive
	}
	if bkInteractive {
		bkLines = append(bkLines, styles.Timestamp.Render(messages.InteractHint(msg))) // fork: messages/links_fork.go
	}
	bkBlock := ""
	bkLineCount := len(bkLines)
	if bkLineCount > 0 {
		// Same background treatment the message pane applies; see the
		// comment at the matching site in messages/model.go. Block Kit
		// lines have no outer background-providing style, so without
		// this the run after the gutter's closing reset draws on the
		// terminal default instead of the theme's.
		bkBlock = "\n" + messages.WithBackground(bkLines, messages.BgANSI())
	}

	var reactionLine string
	// pillSpecs captures one entry per real (non-"+") reaction pill in
	// rendering order, with line index and column extents relative to
	// the reaction-line block. Translated to absolute reactionEntryHit
	// rects below (once we know the row offset of the reaction block).
	type pillSpec struct {
		lineIdx  int
		colStart int
		colEnd   int
		emoji    string
	}
	var pillSpecs []pillSpec
	reactionLineCount := 0
	if len(msg.Reactions) > 0 {
		var pills []string
		var pillEmojis []string
		// Image-aware emoji-as-image path is active only when the
		// process-global ImageMode is on AND a fetcher has been
		// installed via SetEmojiContext. Otherwise the legacy
		// glyph/shortcode-text branch renders. Mirrors the
		// messages-pane reaction-pill loop.
		imageOK := emojiutil.ImageModeActive() && m.emojiCtx.PlaceCtx.Fetcher != nil
		pillCells := m.emojiCtx.Cells
		if pillCells <= 0 {
			pillCells = 2
		}
		for i, r := range msg.Reactions {
			// Image path: pass r.Emoji directly (including any skin-tone
			// suffix) — URLForShortcode composes the per-tone CDN URL
			// natively. Stripping skin tone was a workaround for
			// glyph-rendering width inconsistencies that no longer apply
			// at the kitty-image cell footprint. Mirror of the
			// messages-pane reaction-pill loop.
			//
			// Legacy path: still strip the suffix because the glyph
			// renderer suffers the original width-inconsistency problem
			// (multi-codepoint ZWJ / skin-tone sequences render at
			// terminal-dependent widths and break border alignment).
			var emojiStr string
			var placedFlush func(io.Writer) error
			if imageOK {
				if url, ok := emojiutil.URLForShortcode(r.Emoji, m.emojiCtx.Customs); ok {
					if placement, flush, ok := emojiutil.Place(m.emojiCtx.PlaceCtx, url, pillCells); ok {
						emojiStr = placement
						placedFlush = flush
					}
				}
			}
			if emojiStr == "" {
				// Legacy fallback path (image mode off, no URL, or
				// Place returned false). Strip skin tone here only —
				// the glyph renderer still needs the workaround.
				legacyName := emojiutil.StripSkinTone(r.Emoji)
				resolved := emojiutil.Sprint(":" + legacyName + ":")
				if emojiutil.ShouldRenderUnicode(resolved) {
					emojiStr = resolved
				} else {
					emojiStr = ":" + legacyName + ":"
				}
			}
			var style lipgloss.Style
			if isSelected && m.reactionNavActive && i == m.reactionNavIndex {
				style = styles.ReactionPillSelected
			} else if r.HasReacted {
				style = styles.ReactionPillOwn
			} else {
				style = styles.ReactionPillOther
			}
			pillText := messages.ReactionPillText(emojiStr, r.Count, style, placedFlush != nil)
			pills = append(pills, style.Render(pillText))
			pillEmojis = append(pillEmojis, r.Emoji)
			// Image emoji in reaction pills land in the same per-frame
			// flush list as body text and inline image attachments.
			if placedFlush != nil {
				flushes = append(flushes, placedFlush)
			}
		}
		if isSelected && m.reactionNavActive {
			plusStyle := styles.ReactionPillPlus
			if m.reactionNavIndex >= len(msg.Reactions) {
				plusStyle = styles.ReactionPillSelected
			}
			pills = append(pills, plusStyle.Render("+"))
			pillEmojis = append(pillEmojis, "")
		}
		bgSpace := lipgloss.NewStyle().Background(styles.Background).Render(" ")
		var reactionLines []string
		currentLine := ""
		lineIdx := 0
		for i, pill := range pills {
			candidate := currentLine
			sepWidth := 0
			if i > 0 {
				candidate += bgSpace
				sepWidth = 1
			}
			candidate += pill
			pillW := emojiutil.Width(pill)
			if emojiutil.Width(candidate) > contentWidth && currentLine != "" {
				reactionLines = append(reactionLines, currentLine)
				lineIdx++
				currentLine = pill
				if pillEmojis[i] != "" {
					pillSpecs = append(pillSpecs, pillSpec{
						lineIdx:  lineIdx,
						colStart: 0,
						colEnd:   pillW,
						emoji:    pillEmojis[i],
					})
				}
			} else {
				colStart := emojiutil.Width(currentLine) + sepWidth
				if pillEmojis[i] != "" {
					pillSpecs = append(pillSpecs, pillSpec{
						lineIdx:  lineIdx,
						colStart: colStart,
						colEnd:   colStart + pillW,
						emoji:    pillEmojis[i],
					})
				}
				currentLine = candidate
			}
		}
		if currentLine != "" {
			reactionLines = append(reactionLines, currentLine)
		}
		reactionLine = "\n" + strings.Join(reactionLines, "\n")
		reactionLineCount = len(reactionLines)
	}

	// Attachments: per-attachment inline render via imgrender.Renderer.
	// v1: flushes are aggregated into the unified `flushes` slice for
	// the cache layer to invoke during View(); the per-block Hit and
	// SixelRows are discarded (click-to-preview from threads and inline
	// sixel emission are out of scope for v1; sixel images render their
	// res.Lines placeholder/sentinel only).
	var attachmentLines string
	attachmentLineCount := 0
	if len(msg.Attachments) > 0 {
		if m.imgRenderer == nil {
			m.imgRenderer = imgrender.NewRenderer()
		}
		blocks := make([]string, 0, len(msg.Attachments))
		for attIdx, att := range msg.Attachments {
			imgThumbs := make([]imgrender.ThumbSpec, len(att.Thumbs))
			for i, t := range att.Thumbs {
				imgThumbs[i] = imgrender.ThumbSpec{URL: t.URL, W: t.W, H: t.H}
			}
			res := m.imgRenderer.RenderBlock(imgrender.Block{
				Kind:   att.Kind,
				FileID: att.FileID,
				Name:   att.Name,
				URL:    att.URL,
				Thumbs: imgThumbs,
			}, m.channelID, msg.TS, contentWidth, 0 /* baseRow */, attIdx, 0 /* contentColBase */)
			blocks = append(blocks, strings.Join(res.Lines, "\n"))
			flushes = append(flushes, res.Flushes...)
			attachmentLineCount += len(res.Lines)
		}
		attachmentLines = "\n" + strings.Join(blocks, "\n")
	}

	// Translate per-pill specs into reactionEntryHit rects. Row layout
	// of the reply content (pre-border, pre-tint):
	//   rows [0 .. labelRowCount): label rows (ephemeral)
	//   next row: username + timestamp line
	//   rows [labelRowCount+1 .. +textRows): wrapped body text
	//   rows [.. +bkLineCount): block kit + legacy attachment content
	//   rows [.. +attachmentLineCount): file attachments
	//   rows [reactionRowBase ..): reaction lines
	// buildCache wraps the content with a thick left border in col 0
	// (so contentColBase = 1); it adds no rows.
	var reactionHits []reactionEntryHit
	if len(pillSpecs) > 0 && reactionLineCount > 0 {
		const contentColBase = 1 // thick left border occupies col 0 of linesNormal
		reactionRowBase := labelRowCount + 1 + bodyRows + bkLineCount + attachmentLineCount
		for _, ps := range pillSpecs {
			row := reactionRowBase + ps.lineIdx
			reactionHits = append(reactionHits, reactionEntryHit{
				rowStartInEntry: row,
				rowEndInEntry:   row + 1,
				colStart:        contentColBase + ps.colStart,
				colEnd:          contentColBase + ps.colEnd,
				emoji:           ps.emoji,
			})
		}
	}

	// Header width budget for messages.SelectedHeader: the body's wrap
	// width, capped at the columns the row really has (width-1 after the
	// thick left border) because contentWidth is floored at 20.
	return labelRows + line + bodyRow + bkBlock + attachmentLines + reactionLine, flushes, reactionHits, line, min(contentWidth, width-1)
}
