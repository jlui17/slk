package apphome

import (
	"encoding/json"
	"math"
	"strings"
)

// OpenMode is what follows a click on a button that doesn't link: the
// link the app's redraw adds is opened here, in a new herdr tab, or
// not at all.
type OpenMode int

const (
	OpenNone OpenMode = iota
	OpenHere
	OpenInTab
)

// IntentKind is what a key asks the host to do.
type IntentKind int

const (
	IntentNone IntentKind = iota
	// IntentClick sends Action to the app; with URL set, the host also
	// opens URL as Open says.
	IntentClick
	// IntentNeedsSlack: the element is one slk can't act on.
	IntentNeedsSlack
	// IntentToSidebar moves focus to the sidebar.
	IntentToSidebar
)

// Intent is the outcome of a key.
type Intent struct {
	Kind   IntentKind
	Action json.RawMessage
	State  json.RawMessage
	URL    string
	Open   OpenMode
	// WaitID is set when the click (o or O) waits for the app's redraw;
	// the host reports a timeout with WaitTimedOut(WaitID).
	WaitID int
}

type stop struct {
	block int
	// elem is the element's index in an actions block, -1 for a
	// section's accessory.
	elem int
}

// stopKey names a stop across redraws.
type stopKey struct{ blockID, actionID string }

type boxKind int

const (
	boxNone boxKind = iota
	boxSelect
	boxConfirm
)

type wait struct {
	id   int
	key  stopKey
	open OpenMode
	// before is the view the click was made on: the link the redraw
	// adds is the one it lacks.
	before *View
}

// Model is one app's Home tab.
type Model struct {
	appName string
	// homeOnly is an app with no Messages tab.
	homeOnly bool
	loading  bool
	view     *View
	stops    []stop

	focus int // index into stops, -1 for none
	top   int // first body line shown
	// snap asks the next render to scroll the focused stop into view.
	snap bool

	box          boxKind
	selectCursor int
	confirm      *Confirm
	// held is the click a confirm box holds until it is confirmed.
	held Intent

	wait   *wait
	waitID int

	version int
	cache   renderCache
}

// Open starts loading appName's Home.
func Open(appName string) Model {
	return Model{appName: appName, loading: true, focus: -1}
}

// SetHomeOnly marks an app with no Messages tab.
func (m *Model) SetHomeOnly(on bool) {
	m.homeOnly = on
	m.dirty()
}

func (m *Model) AppName() string { return m.appName }
func (m *Model) HomeOnly() bool  { return m.homeOnly }
func (m *Model) Loading() bool   { return m.loading }
func (m *Model) View() *View     { return m.view }

// BoxOpen reports whether a select's options or a confirm box is open.
func (m *Model) BoxOpen() bool { return m.box != boxNone }

func (m *Model) dirty() { m.version++ }

// CloseBox closes an open select or confirm box.
func (m *Model) CloseBox() {
	m.box, m.confirm, m.held = boxNone, nil, Intent{}
	m.dirty()
}

// SetView shows v, nil for an app that has published no Home, with the
// focus on its first stop.
func (m *Model) SetView(v *View) {
	*m = Model{appName: m.appName, homeOnly: m.homeOnly, view: v, focus: -1, snap: true, waitID: m.waitID, version: m.version + 1}
	m.stops = stopsOf(v)
	if len(m.stops) > 0 {
		m.focus = 0
	}
}

// Update takes the app's redraw of its Home. The focus stays on the
// same element when the redraw of the open view still has it (see
// find), else at the same position; a view with another id is shown as
// SetView shows it. While o or O waits, the link buttons the redraw
// adds to the view o or O was pressed on are returned with what o or O
// was to do: one is the link to open, more than one leaves the choice to
// the user. Either ends the wait; none keeps waiting, on a new view too.
func (m *Model) Update(v *View) (added []string, open OpenMode) {
	if m.view == nil || v.ID != m.view.ID {
		w := m.wait
		m.SetView(v)
		m.wait = w
	} else {
		m.redraw(v)
	}
	if m.wait == nil {
		return nil, OpenNone
	}
	added = m.wait.before.newLinkURLs(v)
	if len(added) == 0 {
		return nil, OpenNone
	}
	open = m.wait.open
	m.wait = nil
	return added, open
}

// redraw shows v in place of the open view it redraws, keeping the
// focus on its element.
func (m *Model) redraw(v *View) {
	var key stopKey
	hadFocus := m.focus >= 0 && m.focus < len(m.stops)
	if hadFocus {
		key = m.keyOf(m.stops[m.focus])
	}
	m.view, m.stops = v, stopsOf(v)
	m.box, m.confirm = boxNone, nil
	switch {
	case len(m.stops) == 0:
		m.focus = -1
	case hadFocus:
		m.focus = m.find(key)
	default:
		m.focus = 0
	}
	m.snap = true
	m.dirty()
}

// find is the stop key names in the redrawn view (see stopOf), else
// the focus's position.
func (m *Model) find(key stopKey) int {
	if i := m.stopOf(key); i >= 0 {
		return i
	}
	return min(m.focus, len(m.stops)-1)
}

// stopOf is the stop key names: the one with the same block_id and
// action_id, else the one stop with its action_id (an app's generated
// block_ids change on every update); -1 for none.
func (m *Model) stopOf(key stopKey) int {
	byAction, n := -1, 0
	for i, s := range m.stops {
		k := m.keyOf(s)
		if k == key {
			return i
		}
		if k.actionID == key.actionID {
			byAction, n = i, n+1
		}
	}
	if n == 1 {
		return byAction
	}
	return -1
}

// WaitTimedOut ends the wait id, and reports whether it was still on.
func (m *Model) WaitTimedOut(id int) bool {
	if m.wait == nil || m.wait.id != id {
		return false
	}
	m.wait = nil
	m.dirty()
	return true
}

// EndWait ends the wait id: its click failed, or the app answered it
// with a modal.
func (m *Model) EndWait(id int) {
	if m.wait != nil && m.wait.id == id {
		m.wait = nil
		m.dirty()
	}
}

func stopsOf(v *View) []stop {
	if v == nil {
		return nil
	}
	var out []stop
	for i, b := range v.blocks {
		if b.accessory != nil {
			out = append(out, stop{block: i, elem: -1})
		}
		for j := range b.elements {
			out = append(out, stop{block: i, elem: j})
		}
	}
	return out
}

func (m *Model) element(s stop) *element {
	b := &m.view.blocks[s.block]
	if s.elem < 0 {
		return b.accessory
	}
	return &b.elements[s.elem]
}

func (m *Model) keyOf(s stop) stopKey {
	return stopKey{m.view.blocks[s.block].id, m.element(s).ActionID}
}

func (m *Model) focused() (stop, *element, bool) {
	if m.view == nil || m.focus < 0 || m.focus >= len(m.stops) {
		return stop{}, nil, false
	}
	s := m.stops[m.focus]
	return s, m.element(s), true
}

// Scroll moves the body n lines, down for n > 0. The body's lines don't
// change, so the render keeps them and only reslices.
func (m *Model) Scroll(n int) { m.top += n }

// GoToTop focuses the first stop and scrolls to the top.
func (m *Model) GoToTop() {
	m.top = 0
	if len(m.stops) > 0 {
		m.focus = 0
	}
	m.dirty()
}

// GoToBottom focuses the last stop and scrolls to the end.
func (m *Model) GoToBottom() {
	m.top = math.MaxInt32 // the render clamps it
	m.snap = false
	if len(m.stops) > 0 {
		m.focus = len(m.stops) - 1
	}
	m.dirty()
}

// Key handles a key, named as normalizeFinderKey names it.
func (m *Model) Key(k string) Intent {
	switch m.box {
	case boxConfirm:
		return m.confirmKey(k)
	case boxSelect:
		return m.selectKey(k)
	}
	switch k {
	case "j", "down":
		m.move(1)
		return Intent{}
	case "k", "up":
		m.move(-1)
		return Intent{}
	case "esc":
		if m.wait != nil {
			m.wait = nil
			m.dirty()
			return Intent{}
		}
		return Intent{Kind: IntentToSidebar}
	case "enter":
		return m.press(OpenNone)
	case "o":
		return m.press(OpenHere)
	case "O":
		return m.press(OpenInTab)
	}
	return Intent{}
}

// move steps the focus; past the first or last stop it scrolls instead,
// so the blocks above the first stop and below the last can be read.
func (m *Model) move(d int) {
	next := m.focus + d
	if len(m.stops) == 0 || next < 0 || next >= len(m.stops) {
		m.Scroll(d)
		return
	}
	m.focus = next
	m.snap = true
	m.dirty()
}

// press is enter (OpenNone, a plain click), o or O on the focused stop.
func (m *Model) press(open OpenMode) Intent {
	s, e, ok := m.focused()
	if !ok {
		return Intent{}
	}
	isEnter := open == OpenNone
	if !e.supported() {
		if isEnter {
			return Intent{Kind: IntentNeedsSlack}
		}
		return Intent{}
	}
	if e.Type == "static_select" {
		if !isEnter {
			return Intent{}
		}
		m.box, m.selectCursor = boxSelect, max(m.chosenIndex(s, e), 0)
		m.dirty()
		return Intent{}
	}
	in := Intent{Kind: IntentClick, Action: m.action(s, e, nil), State: m.stateJSON(), Open: open}
	if e.isLinkButton() {
		in.URL = e.URL
	} else if !isEnter {
		m.waitID++
		in.WaitID = m.waitID
	}
	if e.Confirm != nil {
		m.box, m.confirm, m.held = boxConfirm, e.Confirm, in
		m.dirty()
		return Intent{}
	}
	return m.send(in, s)
}

// send starts the wait of o or O on a button that doesn't link.
func (m *Model) send(in Intent, s stop) Intent {
	if in.WaitID != 0 {
		m.wait = &wait{id: in.WaitID, key: m.keyOf(s), open: in.Open, before: m.view}
		m.dirty()
	}
	return in
}

func (m *Model) confirmKey(k string) Intent {
	switch k {
	case "enter":
		in := m.held
		m.CloseBox()
		if s, _, ok := m.focused(); ok {
			return m.send(in, s)
		}
	case "esc":
		m.CloseBox()
	}
	return Intent{}
}

func (m *Model) selectKey(k string) Intent {
	s, e, ok := m.focused()
	if !ok {
		m.box = boxNone
		return Intent{}
	}
	switch k {
	case "j", "down":
		m.selectCursor = min(m.selectCursor+1, len(e.Options)-1)
	case "k", "up":
		m.selectCursor = max(m.selectCursor-1, 0)
	case "esc":
		m.box = boxNone
	case "enter":
		m.box = boxNone
		if m.selectCursor == m.chosenIndex(s, e) {
			break
		}
		opt := e.optionsRaw[m.selectCursor]
		m.setValue(s, e, opt)
		m.dirty()
		return Intent{Kind: IntentClick, Action: m.action(s, e, opt), State: m.stateJSON()}
	}
	m.dirty()
	return Intent{}
}

// chosenIndex is the option e shows: the view's state, else its
// initial option; -1 for none.
func (m *Model) chosenIndex(s stop, e *element) int {
	value := ""
	if raw, ok := m.view.values[m.view.blocks[s.block].id][e.ActionID]; ok {
		var st struct {
			Selected *option `json:"selected_option"`
		}
		if json.Unmarshal(raw, &st) == nil && st.Selected != nil {
			value = st.Selected.Value
		}
	} else if e.Initial != nil {
		value = e.Initial.Value
	}
	for i, o := range e.Options {
		if value != "" && o.Value == value {
			return i
		}
	}
	return -1
}

func (m *Model) setValue(s stop, e *element, opt json.RawMessage) {
	blockID := m.view.blocks[s.block].id
	if m.view.values[blockID] == nil {
		m.view.values[blockID] = map[string]json.RawMessage{}
	}
	v, _ := json.Marshal(map[string]json.RawMessage{
		"type":            json.RawMessage(`"static_select"`),
		"selected_option": opt,
	})
	m.view.values[blockID][e.ActionID] = v
}

// action is e as the view has it, with its block's block_id, and
// selected for a choice in a select.
func (m *Model) action(s stop, e *element, selected json.RawMessage) json.RawMessage {
	fields := map[string]json.RawMessage{}
	_ = json.Unmarshal(e.raw, &fields)
	id, _ := json.Marshal(m.view.blocks[s.block].id)
	fields["block_id"] = id
	if selected != nil {
		fields["selected_option"] = selected
	}
	out, _ := json.Marshal(fields)
	return out
}

func (m *Model) stateJSON() json.RawMessage {
	out, _ := json.Marshal(map[string]any{"values": m.view.values})
	return out
}

// Hint is the key hint under the Home tab.
func (m *Model) Hint(inHerdr bool) string {
	switch {
	case m.box == boxConfirm:
		return "enter " + m.confirm.ConfirmText() + "  esc " + m.confirm.DenyText()
	case m.box == boxSelect:
		return "enter choose  esc close"
	}
	hint := m.stopHint(inHerdr)
	if m.wait != nil {
		hint = append([]string{"esc stop waiting"}, hint...)
	}
	if !m.homeOnly {
		hint = append(hint, "m messages")
	}
	return strings.Join(hint, "  ")
}

// stopHint is the keys of the focused stop.
func (m *Model) stopHint(inHerdr bool) []string {
	_, e, ok := m.focused()
	switch {
	case !ok || !e.supported():
		return nil
	case e.Type == "static_select":
		return []string{"enter choose"}
	case e.isLinkButton():
		tab := "O open in browser"
		if inHerdr {
			tab = "O herdr tab"
		}
		return []string{"o open", tab}
	}
	verb := strings.ToLower(e.Text.String())
	tab := "O " + verb + " and open in browser"
	if inHerdr {
		tab = "O " + verb + " in herdr tab"
	}
	return []string{"o " + verb + " and open", tab, "enter " + verb}
}
