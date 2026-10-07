package appshortcuts

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// capturedView is the view object of the view_opened event captured
// from Colony's Annotate shortcut, with its ids replaced.
func capturedView(t *testing.T) View {
	t.Helper()
	raw, err := os.ReadFile("../../slack/testdata/ws_view_opened.json")
	if err != nil {
		t.Fatal(err)
	}
	var evt struct {
		ClientToken string          `json:"client_token"`
		View        json.RawMessage `json:"view"`
	}
	if err := json.Unmarshal(raw, &evt); err != nil {
		t.Fatal(err)
	}
	v, err := ParseView(evt.View)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func viewFrom(t *testing.T, raw string) View {
	t.Helper()
	v, err := ParseView([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func press(f *Form, keys ...string) Action {
	var last Action
	for _, k := range keys {
		last, _ = f.HandleKey(keyMsg(k))
	}
	return last
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "ctrl+s":
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	}
	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: k}
}

func typeText(f *Form, s string) {
	for _, r := range s {
		f.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func TestParseView_Captured(t *testing.T) {
	v := capturedView(t)
	if v.ID != "V00000VIEW1" || v.RootViewID != "V00000VIEW1" {
		t.Errorf("ids = %q %q", v.ID, v.RootViewID)
	}
	if v.Title != "Annotate Claude's work" || v.SubmitText != "Submit" || v.CloseText != "Cancel" {
		t.Errorf("title %q submit %q close %q", v.Title, v.SubmitText, v.CloseText)
	}
	if len(v.Blocks) != 2 {
		t.Fatalf("got %d blocks", len(v.Blocks))
	}
	note, sign := v.Blocks[0], v.Blocks[1]
	if note.BlockID != "note" || note.Element.Type != "plain_text_input" || !note.Element.Multiline || note.Element.MinLength != 1 {
		t.Errorf("note block = %+v / %+v", note, note.Element)
	}
	if sign.BlockID != "sign" || sign.Element.Type != "radio_buttons" || len(sign.Element.Options) != 2 {
		t.Errorf("sign block = %+v", sign)
	}
	if !v.supported() {
		t.Error("the captured view should be fillable in slk")
	}
}

// The whole Annotate flow on the captured form: type a two-line note,
// tab to the radio, pick Negative, submit.
func TestForm_CapturedFlow(t *testing.T) {
	f := NewForm(capturedView(t))
	f.Focus()
	typeText(f, "first")
	press(f, "enter")
	typeText(f, "second")
	press(f, "tab", "j", "space")
	if got := press(f, "ctrl+s"); got != ActionSubmit {
		t.Fatalf("ctrl+s = %v, want submit", got)
	}
	want := `{"values":{"note":{"text":{"type":"plain_text_input","value":"first\nsecond"}},"sign":{"pick":{"type":"radio_buttons","selected_option":{"text":{"type":"plain_text","text":"Negative","emoji":true},"value":"negative"}}}}}`
	if got := f.State(); got != want {
		t.Errorf("state\n got %s\nwant %s", got, want)
	}
}

func TestForm_StateEncoding(t *testing.T) {
	v := viewFrom(t, `{"id":"V1","title":{"type":"plain_text","text":"T"},"submit":{"type":"plain_text","text":"Send"},"blocks":[
		{"type":"input","block_id":"b1","label":{"type":"plain_text","text":"Name"},"optional":true,
		 "element":{"type":"plain_text_input","action_id":"name","initial_value":"Ada"}},
		{"type":"input","block_id":"b2","label":{"type":"plain_text","text":"Kind"},"optional":true,
		 "element":{"type":"radio_buttons","action_id":"kind","options":[
		  {"text":{"type":"plain_text","text":"A"},"value":"a"},{"text":{"type":"plain_text","text":"B"},"value":"b"}]}},
		{"type":"input","block_id":"b3","label":{"type":"plain_text","text":"Team"},"optional":true,
		 "element":{"type":"static_select","action_id":"team","initial_option":{"text":{"type":"plain_text","text":"Blue","emoji":true},"value":"blue"},"options":[
		  {"text":{"type":"plain_text","text":"Orange","emoji":true},"value":"orange"},{"text":{"type":"plain_text","text":"Blue","emoji":true},"value":"blue"}]}},
		{"type":"input","block_id":"b4","label":{"type":"plain_text","text":"Notes"},"optional":true,
		 "element":{"type":"plain_text_input","action_id":"notes"}}]}`)
	f := NewForm(v)
	want := `{"values":{"b1":{"name":{"type":"plain_text_input","value":"Ada"}},"b2":{"kind":{"type":"radio_buttons","selected_option":null}},"b3":{"team":{"type":"static_select","selected_option":{"text":{"type":"plain_text","text":"Blue","emoji":true},"value":"blue"}}},"b4":{"notes":{"type":"plain_text_input","value":null}}}}`
	if got := f.State(); got != want {
		t.Errorf("state\n got %s\nwant %s", got, want)
	}

	// Change the select: focus it, move up to Orange, choose it.
	f.Focus()
	press(f, "tab", "tab", "k", "enter")
	if !strings.Contains(f.State(), `"team":{"type":"static_select","selected_option":{"text":{"type":"plain_text","text":"Orange","emoji":true},"value":"orange"}}`) {
		t.Errorf("after choosing Orange: %s", f.State())
	}
}

func TestForm_Validation(t *testing.T) {
	v := viewFrom(t, `{"id":"V1","title":{"type":"plain_text","text":"T"},"submit":{"type":"plain_text","text":"Send"},"blocks":[
		{"type":"input","block_id":"note","label":{"type":"plain_text","text":"Note"},
		 "element":{"type":"plain_text_input","action_id":"t","min_length":5,"max_length":8}},
		{"type":"input","block_id":"sign","label":{"type":"plain_text","text":"Label"},
		 "element":{"type":"radio_buttons","action_id":"p","options":[{"text":{"type":"plain_text","text":"Yes"},"value":"y"}]}}]}`)
	f := NewForm(v)
	f.Focus()
	if got := press(f, "ctrl+s"); got != ActionNone {
		t.Fatalf("empty required form submitted: %v", got)
	}
	if f.fields[0].err != "Required" || f.fields[1].err != "Required" {
		t.Errorf("errors = %q, %q; want Required twice", f.fields[0].err, f.fields[1].err)
	}
	box := ansi.Strip(Model{phase: phaseForm, form: f}.box(100, 40))
	if strings.Count(box, "! Required") != 2 {
		t.Errorf("the form should show Required under both fields:\n%s", box)
	}

	typeText(f, "abc")
	press(f, "tab", "space")
	press(f, "ctrl+s")
	if f.fields[0].err != "At least 5 characters" || f.fields[1].err != "" {
		t.Errorf("errors = %q, %q", f.fields[0].err, f.fields[1].err)
	}
	press(f, "tab")
	typeText(f, "defghi")
	press(f, "ctrl+s")
	if f.fields[0].err != "At most 8 characters" {
		t.Errorf("error = %q", f.fields[0].err)
	}
	f.fields[0].input.SetValue("abcdef")
	if got := press(f, "ctrl+s"); got != ActionSubmit {
		t.Errorf("valid form: %v, errors %q", got, f.fields[0].err)
	}
}

func TestForm_Keys(t *testing.T) {
	f := NewForm(capturedView(t))
	f.Focus()
	press(f, "shift+tab")
	if f.focus != 1 {
		t.Errorf("shift+tab from the first field: focus %d, want the last", f.focus)
	}
	press(f, "tab")
	if f.focus != 0 {
		t.Errorf("tab from the last field: focus %d, want the first", f.focus)
	}
	typeText(f, "j k")
	if got := f.fields[0].input.Value(); got != "j k" {
		t.Errorf("typed into the note: %q", got)
	}
	f.Paste(tea.PasteMsg{Content: "!"})
	if got := f.fields[0].input.Value(); got != "j k!" {
		t.Errorf("after paste: %q", got)
	}
	press(f, "tab", "space")
	if f.fields[1].chosen != 0 {
		t.Errorf("space on Positive: chosen %d", f.fields[1].chosen)
	}
	press(f, "down", "enter")
	if f.fields[1].chosen != 1 {
		t.Errorf("enter on Negative: chosen %d", f.fields[1].chosen)
	}
	box := ansi.Strip(Model{phase: phaseForm, form: f}.box(100, 40))
	for _, want := range []string{"( ) Positive", "(•) Negative", "tab next · ctrl+s Submit · esc Cancel", "Annotate Claude's work"} {
		if !strings.Contains(box, want) {
			t.Errorf("form missing %q:\n%s", want, box)
		}
	}
	if got := press(f, "esc"); got != ActionCancel {
		t.Errorf("esc = %v, want cancel", got)
	}
}

func TestForm_SingleLineIgnoresEnter(t *testing.T) {
	f := NewForm(viewFrom(t, `{"id":"V1","title":{"type":"plain_text","text":"T"},"blocks":[
		{"type":"input","block_id":"b","label":{"type":"plain_text","text":"Name"},
		 "element":{"type":"plain_text_input","action_id":"n"}}]}`))
	f.Focus()
	typeText(f, "a")
	press(f, "enter")
	typeText(f, "b")
	if got := f.fields[0].input.Value(); got != "ab" {
		t.Errorf("value %q, want ab", got)
	}
}

func TestForm_SubmitErrors(t *testing.T) {
	f := NewForm(capturedView(t))
	f.SetSending()
	f.SetSubmitError("validation_failed", map[string]string{"note": "Say more", "gone": "Elsewhere"})
	if f.sending || f.fields[0].err != "Say more" || f.footerErr != "Elsewhere" {
		t.Errorf("sending %v note %q footer %q", f.sending, f.fields[0].err, f.footerErr)
	}
	f.SetSubmitError("view_not_found", nil)
	if f.footerErr != "view_not_found" {
		t.Errorf("footer %q", f.footerErr)
	}
}

// While a submit is in flight every key waits, esc included, so Slack's
// answer is not dropped with the form.
func TestForm_SendingIgnoresEsc(t *testing.T) {
	f := NewForm(capturedView(t))
	f.SetSending()
	if got := press(f, "esc"); got != ActionNone {
		t.Errorf("esc while sending = %v, want ActionNone", got)
	}
}

func TestForm_UnsupportedNeedsSlack(t *testing.T) {
	for name, block := range map[string]string{
		"actions block":    `{"type":"actions","elements":[{"type":"button","text":{"type":"plain_text","text":"Go"}}]}`,
		"datepicker input": `{"type":"input","block_id":"d","label":{"type":"plain_text","text":"When"},"element":{"type":"datepicker","action_id":"d"}}`,
		"button accessory": `{"type":"section","text":{"type":"mrkdwn","text":"hi"},"accessory":{"type":"button"}}`,
		"option groups":    `{"type":"input","block_id":"s","label":{"type":"plain_text","text":"S"},"element":{"type":"static_select","action_id":"s","option_groups":[{"label":{"type":"plain_text","text":"G"},"options":[]}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := NewForm(viewFrom(t, `{"id":"V1","title":{"type":"plain_text","text":"T"},"submit":{"type":"plain_text","text":"Send"},"blocks":[`+block+`]}`))
			if f.supported {
				t.Fatal("form should need Slack")
			}
			if got := press(f, "ctrl+s"); got != ActionNone {
				t.Errorf("ctrl+s = %v, want nothing sent", got)
			}
			if got := press(f, "o"); got != ActionOpenInBrowser {
				t.Errorf("o = %v, want open in browser", got)
			}
			box := ansi.Strip(Model{phase: phaseForm, form: f}.box(100, 40))
			if !strings.Contains(box, "This form needs Slack") || !strings.Contains(box, "o open in browser · esc Cancel") || strings.Contains(box, "ctrl+s") {
				t.Errorf("box:\n%s", box)
			}
		})
	}
}

func TestForm_DisplayBlocks(t *testing.T) {
	f := NewForm(viewFrom(t, `{"id":"V1","title":{"type":"plain_text","text":"T"},"blocks":[
		{"type":"header","text":{"type":"plain_text","text":"Heads up"}},
		{"type":"section","text":{"type":"mrkdwn","text":"Some *text*"},"accessory":{"type":"image"}},
		{"type":"divider"},
		{"type":"context","elements":[{"type":"image"},{"type":"mrkdwn","text":"small print"}]}]}`))
	if !f.supported {
		t.Fatal("display-only blocks should be supported")
	}
	box := ansi.Strip(Model{phase: phaseForm, form: f}.box(60, 40))
	for _, want := range []string{"Heads up", "Some *text*", "────", "small print", "tab next · esc Cancel"} {
		if !strings.Contains(box, want) {
			t.Errorf("missing %q:\n%s", want, box)
		}
	}
}

func TestForm_TinyTerminalDoesNotPanic(t *testing.T) {
	m := Model{phase: phaseForm, form: NewForm(capturedView(t))}
	m.form.Focus()
	for _, size := range [][2]int{{1, 1}, {8, 3}, {20, 6}, {200, 60}} {
		_ = m.ViewOverlay(size[0], size[1], "")
	}
}

// A form taller than the terminal keeps the focused field in view.
func TestForm_ScrollsToFocus(t *testing.T) {
	m := Model{phase: phaseForm, form: NewForm(capturedView(t))}
	m.form.Focus()
	m.form.HandleKey(keyMsg("tab"))
	box := ansi.Strip(m.box(80, 12))
	if !strings.Contains(box, "Negative") || !strings.Contains(box, "tab next") {
		t.Errorf("focused radio or footer scrolled away:\n%s", box)
	}
	if h := strings.Count(box, "\n") + 1; h > 12 {
		t.Errorf("box is %d rows, terminal 12", h)
	}
}

// The focused text input draws its caret (a reverse-video cell), and a
// blurred one doesn't.
func TestForm_CaretInFocusedInput(t *testing.T) {
	f := NewForm(capturedView(t))
	f.Focus()
	typeText(f, "hi")
	if !strings.Contains(f.fields[0].input.View(), "\x1b[7") {
		t.Errorf("no caret in the focused note: %q", f.fields[0].input.View())
	}
	press(f, "tab")
	if strings.Contains(f.fields[0].input.View(), "\x1b[7") {
		t.Error("caret still drawn in the blurred note")
	}
}

// Keycaps typed into a text input, or in a section's text, don't push
// any row past the box.
func TestForm_KeycapsKeepBoxWidth(t *testing.T) {
	f := NewForm(viewFrom(t, `{"id":"V1","title":{"type":"plain_text","text":"1️⃣ T"},"submit":{"type":"plain_text","text":"Send"},"blocks":[
		{"type":"section","text":{"type":"mrkdwn","text":"1️⃣ #️⃣ 1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣1️⃣ end"}},
		{"type":"input","block_id":"n","label":{"type":"plain_text","text":"1️⃣ Note"},"element":{"type":"plain_text_input","action_id":"n","multiline":true}},
		{"type":"input","block_id":"r","label":{"type":"plain_text","text":"Pick"},"element":{"type":"radio_buttons","action_id":"r","options":[{"text":{"type":"plain_text","text":"#️⃣ one #️⃣ two #️⃣ three #️⃣ four #️⃣ five #️⃣ six #️⃣ seven"},"value":"1"}]}}]}`))
	f.Focus()
	typeText(f, strings.Repeat("1️⃣", 40))
	f.fields[0].err = "1️⃣ " + strings.Repeat("x", 80)
	m := Model{phase: phaseForm, form: f}
	for _, w := range []int{20, 40, 60, 100} {
		for i, line := range strings.Split(m.box(w, 60), "\n") {
			if got := lipgloss.Width(line); got != boxWidth(w) {
				t.Fatalf("terminal %d: row %d is %d cells, box %d: %q", w, i, got, boxWidth(w), ansi.Strip(line))
			}
		}
	}
}

func TestForm_PasteIntoSingleLineFlattens(t *testing.T) {
	f := NewForm(viewFrom(t, `{"id":"V1","title":{"type":"plain_text","text":"T"},"blocks":[
		{"type":"input","block_id":"b","label":{"type":"plain_text","text":"Name"},
		 "element":{"type":"plain_text_input","action_id":"n"}}]}`))
	f.Focus()
	f.Paste(tea.PasteMsg{Content: "one\ntwo\r\nthree  four"})
	if got := f.fields[0].input.Value(); got != "one two three  four" {
		t.Errorf("value %q", got)
	}
}

// An updated view keeps what was typed and chosen in the inputs it
// still has, and a submit in flight stays in flight.
func TestForm_UpdateKeepsInput(t *testing.T) {
	old := NewForm(capturedView(t))
	old.Focus()
	typeText(old, "kept")
	press(old, "tab", "j", "space")
	old.SetSending()
	v := capturedView(t)
	v.Title = "Updated"
	nf := old.Update(v)
	if got := nf.fields[0].input.Value(); got != "kept" {
		t.Errorf("note %q", got)
	}
	if nf.fields[1].chosen != 1 || nf.focus != 1 {
		t.Errorf("chosen %d focus %d", nf.fields[1].chosen, nf.focus)
	}
	if got, _ := nf.HandleKey(keyMsg("ctrl+s")); got != ActionNone {
		t.Errorf("ctrl+s during the first submit = %v", got)
	}
}
