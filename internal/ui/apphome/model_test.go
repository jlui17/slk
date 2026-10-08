package apphome

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// colonyView is Colony's captured Home: In Progress with one Open review
// link button, the sort select, two Start review rows, then Review the
// next 3 (with a confirm) and Next.
func colonyView(t *testing.T) *View {
	t.Helper()
	raw, err := os.ReadFile("../../slack/testdata/conversations_info_app_home.json")
	if err != nil {
		t.Fatal(err)
	}
	var resp struct {
		HomeView json.RawMessage `json:"home_view"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	v, err := ParseView(resp.HomeView)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func opened(t *testing.T) *Model {
	m := Open("Colony")
	m.SetView(colonyView(t))
	return &m
}

func (m *Model) focusedAction() string {
	_, e, ok := m.focused()
	if !ok {
		return ""
	}
	return e.ActionID
}

func actionField(t *testing.T, raw json.RawMessage, field string) string {
	t.Helper()
	var f map[string]json.RawMessage
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("action %s: %v", raw, err)
	}
	return string(f[field])
}

func TestParseView_Colony(t *testing.T) {
	v := colonyView(t)
	if v.ID != "V00000HOME1" || v.BotID != "B00000BOT01" || v.AppID != "A00000APP01" || v.TeamID != "T00000TEAM1" {
		t.Errorf("view = %+v", v)
	}
	m := opened(t)
	var got []string
	for range m.stops {
		got = append(got, m.focusedAction())
		m.Key("j")
	}
	want := []string{"open_review", "sort_order", "start_review", "start_review", "start_next_3", "next_page"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("stops = %v, want %v", got, want)
	}
}

func TestKey_MovesBetweenStops(t *testing.T) {
	m := opened(t)
	m.Key("j")
	m.Key("down")
	if m.focusedAction() != "start_review" || m.focus != 2 {
		t.Fatalf("focus = %d %s", m.focus, m.focusedAction())
	}
	m.Key("k")
	if m.focusedAction() != "sort_order" {
		t.Errorf("after k: %s", m.focusedAction())
	}
	for range 10 {
		m.Key("k")
	}
	if m.focus != 0 {
		t.Errorf("k past the first stop moved the focus to %d", m.focus)
	}
}

// A redraw keeps the focus on the element with the same block_id and
// action_id, wherever it moved; when it is gone, on the same position.
func TestUpdate_KeepsFocus(t *testing.T) {
	m := opened(t)
	m.Key("j")
	m.Key("j")
	m.Key("j") // the second Start review
	before := m.view.blocks[m.stops[m.focus].block].id

	// The first Start review row moves up into In Progress: the focused
	// row is now one stop earlier... no: it stays the 4th stop but its
	// block moved. Drop the first start_review row from the redraw.
	next := colonyView(t)
	for i, b := range next.blocks {
		if b.id == "start_review/C0BCG30UGEP/1790723734.808739" {
			next.blocks = append(next.blocks[:i], next.blocks[i+1:]...)
			break
		}
	}
	m.Update(next)
	if got := m.view.blocks[m.stops[m.focus].block].id; got != before {
		t.Errorf("focus on %s after redraw, want %s", got, before)
	}

	// Now drop the focused row itself: the focus stays at its position.
	pos := m.focus
	next2 := colonyView(t)
	var kept []block
	for _, b := range next2.blocks {
		if !strings.HasPrefix(b.id, "start_review/") {
			kept = append(kept, b)
		}
	}
	next2.blocks = kept
	m.Update(next2)
	if m.focus != pos {
		t.Errorf("focus = %d, want position %d", m.focus, pos)
	}
}

// Page 2 of Colony's Home adds Previous before Next, and the actions
// block's generated block_id changes: the focus on Next stays on Next,
// found by its action_id, where the position would land on Previous.
func TestUpdate_KeepsFocusByActionID(t *testing.T) {
	m := opened(t)
	for m.focusedAction() != "next_page" {
		m.Key("j")
	}
	page2 := colonyView(t)
	for i, b := range page2.blocks {
		if b.id != "iepno" {
			continue
		}
		prev, label := b.elements[1], *b.elements[1].Text
		label.Text = "Previous"
		prev.ActionID, prev.Text = "prev_page", &label
		b.id = "xK2q9"
		b.elements = []element{b.elements[0], prev, b.elements[1]}
		page2.blocks[i] = b
	}
	m.Update(page2)
	if got := m.focusedAction(); got != "next_page" {
		t.Errorf("focus on %s after the page 2 redraw, want next_page", got)
	}
}

func TestEnter_LinkButtonClicksAndOpens(t *testing.T) {
	m := opened(t)
	in := m.Key("enter")
	if in.Kind != IntentClick || in.URL != "https://example.slack.com/archives/C0C6ZMJJT1U/p1791343906925689" || in.Open == OpenInTab || in.WaitID != 0 {
		t.Fatalf("intent = %+v", in)
	}
	if got := actionField(t, in.Action, "block_id"); got != `"open_review/C0BS6HBB3R6/1788218799.815259"` {
		t.Errorf("block_id = %s", got)
	}
	if in := m.Key("O"); in.Open != OpenInTab || in.URL == "" {
		t.Errorf("O = %+v, want the link in a herdr tab", in)
	}
}

// o on Start review clicks it, shows Waiting in its place, and opens the
// one link button the app's redraw adds.
func TestO_NonLinkButtonOpensTheNewLink(t *testing.T) {
	m := opened(t)
	m.Key("j")
	m.Key("j") // first Start review
	in := m.Key("o")
	if in.Kind != IntentClick || in.URL != "" || in.WaitID == 0 {
		t.Fatalf("intent = %+v", in)
	}
	if got := actionField(t, in.Action, "action_id"); got != `"start_review"` {
		t.Errorf("action_id = %s", got)
	}
	if hint := m.Hint(true); hint != "esc stop waiting  o start review and open  O start review in herdr tab  enter start review  m messages" {
		t.Errorf("hint = %q", hint)
	}
	body := ansi.Strip(m.Render(Params{Width: 120, Height: 60, Spinner: "⠋"}))
	if !strings.Contains(body, "Waiting for Colony…") {
		t.Errorf("no Waiting label:\n%s", body)
	}

	// A redraw without a new link keeps waiting.
	if added, _ := m.Update(colonyView(t)); added != nil {
		t.Fatalf("opened %q on a redraw with no new link", added)
	}
	// The redraw that adds the review's Open review button opens it.
	next := colonyView(t)
	newRow := next.blocks[3]
	acc := *newRow.accessory
	acc.URL = "https://example.slack.com/archives/C0C6ZMJJT1U/p1791999999000001"
	newRow.accessory = &acc
	newRow.id = "open_review/new"
	next.blocks = append(next.blocks[:4], append([]block{newRow}, next.blocks[4:]...)...)
	added, open := m.Update(next)
	if len(added) != 1 || added[0] != acc.URL || open != OpenHere {
		t.Errorf("Update = %q %v, want the new link here", added, open)
	}
	if m.WaitTimedOut(in.WaitID) {
		t.Error("still waiting after the link opened")
	}
}

func TestO_NoAnswerTimesOut(t *testing.T) {
	m := opened(t)
	m.Key("j")
	m.Key("j")
	in := m.Key("O")
	if !m.WaitTimedOut(in.WaitID) {
		t.Error("WaitTimedOut found no wait")
	}
	if strings.HasPrefix(m.Hint(true), "esc stop waiting") {
		t.Error("still waiting after the timeout")
	}
}

// enter on a button that doesn't link is a plain click, as in Slack: the
// click goes out and the Home stays as it was, with no wait.
func TestEnter_PlainButtonOnlyClicks(t *testing.T) {
	m := opened(t)
	m.Key("j")
	m.Key("j") // first Start review
	in := m.Key("enter")
	if in.Kind != IntentClick || in.URL != "" || in.WaitID != 0 {
		t.Fatalf("intent = %+v", in)
	}
	if strings.Contains(ansi.Strip(m.Render(Params{Width: 120, Height: 60})), "Waiting for") || strings.HasPrefix(m.Hint(true), "esc stop waiting") {
		t.Error("enter started a wait")
	}
	m.Key("j")
	if m.focusedAction() != "start_review" || m.focus != 3 {
		t.Errorf("j after enter: focus %d %s", m.focus, m.focusedAction())
	}
}

// While o waits for the redraw, the keys keep working and the pressed
// button keeps its Waiting label; esc stops the wait, and a second esc
// goes to the sidebar.
func TestO_WaitKeepsTheKeys(t *testing.T) {
	m := opened(t)
	m.Key("j")
	m.Key("j") // first Start review
	m.Key("o")
	m.Key("j")
	if m.focus != 3 {
		t.Fatalf("j during the wait: focus %d", m.focus)
	}
	body := ansi.Strip(m.Render(Params{Width: 120, Height: 60}))
	if strings.Count(body, "Waiting for Colony…") != 1 || strings.Count(body, "[ Start review ]") != 1 {
		t.Errorf("the pressed button lost its Waiting label:\n%s", body)
	}
	if in := m.Key("esc"); in.Kind != IntentNone {
		t.Fatalf("first esc = %+v, want the wait stopped", in)
	}
	if strings.Contains(ansi.Strip(m.Render(Params{Width: 120, Height: 60})), "Waiting for") {
		t.Error("esc left the wait on")
	}
	if in := m.Key("esc"); in.Kind != IntentToSidebar {
		t.Errorf("second esc = %+v, want the sidebar", in)
	}
}

func TestConfirm_HoldsTheClickUntilConfirmed(t *testing.T) {
	m := opened(t)
	for m.focusedAction() != "start_next_3" {
		m.Key("j")
	}
	if in := m.Key("enter"); in.Kind != IntentNone {
		t.Fatalf("enter sent %+v before the confirm", in)
	}
	box := ansi.Strip(m.ConfirmBox(120))
	for _, want := range []string{"Start 3 reviews?", "The judge will tag you three times over the next half hour.", "enter Start esc Cancel"} {
		if !strings.Contains(strings.Join(strings.Fields(box), " "), want) {
			t.Errorf("confirm box lacks %q:\n%s", want, box)
		}
	}
	if hint := m.Hint(true); hint != "enter Start  esc Cancel" {
		t.Errorf("hint = %q", hint)
	}
	if in := m.Key("x"); in.Kind != IntentNone || !m.BoxOpen() {
		t.Fatal("another key closed the box")
	}
	in := m.Key("enter")
	if in.Kind != IntentClick || actionField(t, in.Action, "block_id") != `"iepno"` {
		t.Fatalf("confirmed intent = %+v", in)
	}

	m.Key("enter")
	if in := m.Key("esc"); in.Kind != IntentNone || m.BoxOpen() {
		t.Errorf("esc: %+v, open=%v", in, m.BoxOpen())
	}
}

// Confirming a link button's box redraws the pane: the hint goes back
// to the button's.
func TestConfirm_LinkButtonRedrawsAfterConfirming(t *testing.T) {
	m := opened(t)
	var confirm *Confirm
	for _, b := range m.view.blocks {
		for _, e := range b.elements {
			if e.Confirm != nil {
				confirm = e.Confirm
			}
		}
	}
	m.element(m.stops[0]).Confirm = confirm // Open review
	m.Key("enter")
	p := Params{Width: 120, Height: 30}
	if !strings.Contains(ansi.Strip(m.Render(p)), "enter Start  esc Cancel") {
		t.Fatal("no confirm hint")
	}
	if in := m.Key("enter"); in.Kind != IntentClick || in.URL == "" {
		t.Fatalf("confirmed intent = %+v", in)
	}
	if out := ansi.Strip(m.Render(p)); !strings.Contains(out, "o open  O open in browser  m messages") {
		t.Errorf("stale hint after confirming:\n%s", out)
	}
}

func TestSelect_ChoosingSendsTheOption(t *testing.T) {
	m := opened(t)
	m.Key("j") // sort_order
	if in := m.Key("enter"); in.Kind != IntentNone || !m.BoxOpen() {
		t.Fatalf("enter on the select: %+v", in)
	}
	body := ansi.Strip(m.Render(Params{Width: 120, Height: 60}))
	if !strings.Contains(body, "(•) Newest first") || !strings.Contains(body, "( ) Oldest first") {
		t.Errorf("options not drawn:\n%s", body)
	}
	if in := m.Key("enter"); in.Kind != IntentNone {
		t.Fatalf("choosing the chosen option sent %+v", in)
	}
	m.Key("enter")
	m.Key("j")
	in := m.Key("enter")
	if in.Kind != IntentClick {
		t.Fatalf("intent = %+v", in)
	}
	if got := actionField(t, in.Action, "selected_option"); !strings.Contains(got, `"value":"oldest"`) {
		t.Errorf("selected_option = %s", got)
	}
	if got := actionField(t, in.Action, "block_id"); got != `"WbJB6"` {
		t.Errorf("block_id = %s", got)
	}
	if !strings.Contains(string(in.State), `"WbJB6":{"sort_order":{"selected_option":{"text":{"type":"plain_text","text":"Oldest first","emoji":true},"value":"oldest"},"type":"static_select"}}`) {
		t.Errorf("state = %s", in.State)
	}
	if body := ansi.Strip(m.Render(Params{Width: 120, Height: 60})); !strings.Contains(body, "Oldest first ▾") {
		t.Errorf("select does not show the choice:\n%s", body)
	}
}

func TestRender_Pane(t *testing.T) {
	m := opened(t)
	out := m.Render(Params{Width: 120, Height: 30, PaneFocused: true, InHerdr: true, Ctx: blockkit.Context{}})
	lines := strings.Split(ansi.Strip(out), "\n")
	if len(lines) != 30 {
		t.Fatalf("%d lines, want 30", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 120 {
			t.Errorf("line %d is %d wide: %q", i, w, l)
		}
	}
	if !strings.HasPrefix(lines[0], " ▣ Colony") || !strings.HasPrefix(lines[1], " Home   Messages") {
		t.Errorf("header = %q / %q", lines[0], lines[1])
	}
	if !strings.Contains(lines[29], "o open  O herdr tab  m messages") {
		t.Errorf("hint = %q", lines[29])
	}
	if !strings.Contains(ansi.Strip(out), "▌") || !strings.Contains(ansi.Strip(out), "[ Open review ]") {
		t.Errorf("no focus bar or button:\n%s", ansi.Strip(out))
	}

	loading := Open("Colony")
	if got := ansi.Strip(loading.Render(Params{Width: 60, Height: 20, Spinner: "⠋"})); !strings.Contains(got, " ⠋ Loading Colony's Home…") {
		t.Errorf("loading pane:\n%s", got)
	}
}

// slackCtx renders text as the messages pane does.
func slackCtx() blockkit.Context {
	return blockkit.Context{
		RenderTextForWidth: func(s string, un map[string]string, w int) string {
			return messages.RenderSlackMarkdownWith(s, messages.RenderSlackMarkdownOpts{UserNames: un, Width: w, WorkspaceDomain: "example"})
		},
		WrapText: messages.WordWrap,
	}
}

// A click on a link in a section's text returns it; a click elsewhere on
// a stop's row moves the focus there.
func TestClickAt_LinkOpensElseFocuses(t *testing.T) {
	m := opened(t)
	out := ansi.Strip(m.Render(Params{Width: 140, Height: 60, Ctx: slackCtx()}))
	rows := strings.Split(out, "\n")
	y, x := -1, -1
	for i, r := range rows {
		if c := strings.Index(r, "open message"); c >= 0 {
			y, x = i, ansi.StringWidth(r[:c])
			break
		}
	}
	if y < 0 {
		t.Fatalf("no open message link drawn:\n%s", out)
	}
	if url := m.ClickAt(x+2, y); !strings.HasPrefix(url, "https://example.slack.com/archives/C0BS6HBB3R6/p1788218799815259") {
		t.Errorf("ClickAt(open message) = %q", url)
	}
	for i, r := range rows {
		if strings.Contains(r, "[ Review the next 3 ]") {
			y = i
		}
	}
	if url := m.ClickAt(5, y); url != "" || m.focusedAction() != "start_next_3" {
		t.Errorf("click on the actions row: url %q, focus %s", url, m.focusedAction())
	}
}

// A redraw with no new link that regenerates the pressed button's
// block_id keeps its Waiting label while the wait goes on.
func TestO_WaitingLabelFollowsARegeneratedBlockID(t *testing.T) {
	m := opened(t)
	for m.focusedAction() != "next_page" {
		m.Key("j")
	}
	m.Key("o")
	next := colonyView(t)
	for i := range next.blocks {
		if next.blocks[i].id == "iepno" {
			next.blocks[i].id = "xK2q9"
		}
	}
	m.Update(next)
	body := ansi.Strip(m.Render(Params{Width: 120, Height: 60}))
	if !strings.Contains(body, "Waiting for Colony…") || strings.Contains(body, "[ Next ]") {
		t.Errorf("the Waiting label left Next:\n%s", body)
	}
}

// A click on the blank row or the key hint under the body, on a Home
// taller than the pane, does nothing: the body lines below the fold are
// not on those rows. Each pane height puts other body lines there.
func TestClickAt_RowsUnderTheBodyDoNothing(t *testing.T) {
	for h := chromeRows + 1; h < 40; h++ {
		m := opened(t)
		m.Render(Params{Width: 120, Height: h, Ctx: slackCtx()})
		focus := m.focusedAction()
		for _, y := range []int{h - 2, h - 1} {
			if url := m.ClickAt(5, y); url != "" || m.focusedAction() != focus {
				t.Errorf("height %d, click on row %d: url %q, focus %s, want %s", h, y, url, m.focusedAction(), focus)
			}
		}
	}
}

// A redraw with a new view id (the app publishing its Home again rather
// than updating it) keeps o's wait: one with no new link keeps waiting,
// and the link the next one adds, against the view o was pressed on,
// opens.
func TestO_RedrawWithANewViewIDKeepsTheWait(t *testing.T) {
	m := opened(t)
	m.Key("j")
	m.Key("j") // first Start review
	in := m.Key("o")

	same := colonyView(t)
	same.ID = "V00000HOME2"
	if added, _ := m.Update(same); added != nil {
		t.Fatalf("opened %q on a new view with no new link", added)
	}
	if !strings.Contains(ansi.Strip(m.Render(Params{Width: 120, Height: 60})), "Waiting for Colony…") {
		t.Error("the wait ended on a new view with no new link")
	}

	next := colonyView(t)
	next.ID = "V00000HOME3"
	newRow := next.blocks[3]
	acc := *newRow.accessory
	acc.URL = "https://example.slack.com/archives/C0C6ZMJJT1U/p1791999999000001"
	newRow.accessory = &acc
	newRow.id = "open_review/new"
	next.blocks = append(next.blocks[:4], append([]block{newRow}, next.blocks[4:]...)...)
	added, open := m.Update(next)
	if len(added) != 1 || added[0] != acc.URL || open != OpenHere {
		t.Errorf("Update = %q %v, want the new link here", added, open)
	}
	if m.WaitTimedOut(in.WaitID) {
		t.Error("still waiting after the link opened")
	}
}
