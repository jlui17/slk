package appshortcuts

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/cellwidth"
	"github.com/gammons/slk/internal/ui/styles"
	"github.com/rivo/uniseg"
)

// Action is what a key in the form asks the App to do.
type Action int

const (
	ActionNone Action = iota
	// ActionSubmit: the input passed the form's checks; send State.
	ActionSubmit
	// ActionCancel: close the modal.
	ActionCancel
	// ActionOpenInBrowser: the form needs Slack; open the message there.
	ActionOpenInBrowser
)

const (
	textareaRows = 5
	// selectRows is the most options an open static_select shows at once.
	selectRows = 6
)

// field is one input block of the form.
type field struct {
	block   Block
	input   textarea.Model // plain_text_input
	options []Option       // radio_buttons, static_select
	cursor  int            // the option under the cursor
	chosen  int            // the selected option, -1 for none
	err     string
}

func (fl *field) kind() string { return fl.block.Element.Type }

// Form is a modal filled in the terminal.
type Form struct {
	view      View
	fields    []*field
	focus     int
	supported bool
	sending   bool
	footerErr string
}

// NewForm builds the form for v, honoring each input's initial value.
func NewForm(v View) *Form {
	f := &Form{view: v, supported: v.supported()}
	if !f.supported {
		return f
	}
	for _, b := range v.Blocks {
		if b.Type != "input" {
			continue
		}
		fl := &field{block: b, chosen: -1}
		el := b.Element
		switch el.Type {
		case "plain_text_input":
			fl.input = newTextarea(el)
		default:
			fl.options = el.Options
			if el.InitialOption != nil {
				fl.choose(el.InitialOption.Value)
			}
		}
		f.fields = append(f.fields, fl)
	}
	return f
}

func newTextarea(el *Element) textarea.Model {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	// No CharLimit: an over-long value is caught by the form's own
	// max_length check, which says so under the field.
	ta.CharLimit = 0
	rows := 1
	if el.Multiline {
		rows = textareaRows
	} else {
		ta.MaxHeight = 1
	}
	ta.SetHeight(rows)
	if el.Placeholder != nil {
		ta.Placeholder = el.Placeholder.Text
	}
	field := lipgloss.NewStyle().Background(styles.SurfaceDark).Foreground(styles.TextPrimary)
	s := ta.Styles()
	for _, st := range []*textarea.StyleState{&s.Focused, &s.Blurred} {
		st.Base, st.Text, st.CursorLine, st.EndOfBuffer, st.Prompt = field, field, field, field, field
		st.Placeholder = field.Foreground(styles.TextMuted)
	}
	// A steady cursor: the blink ticks would need routing back here.
	s.Cursor.Blink = false
	ta.SetStyles(s)
	ta.SetValue(el.InitialValue)
	ta.Blur()
	return ta
}

// Focus puts the cursor in the focused input, the first one of a new
// form. Call once the form shows.
func (f *Form) Focus() tea.Cmd { return f.setFocus(f.focus) }

func (f *Form) setFocus(i int) tea.Cmd {
	if len(f.fields) == 0 {
		return nil
	}
	if cur := f.fields[f.focus]; cur.kind() == "plain_text_input" {
		cur.input.Blur()
	}
	f.focus = (i + len(f.fields)) % len(f.fields)
	if fl := f.fields[f.focus]; fl.kind() == "plain_text_input" {
		return fl.input.Focus()
	}
	return nil
}

// Update is the form for v, the app's update of f's view: an input v
// still has, by block_id and action_id, keeps what was typed or chosen
// in it, focus stays on it, and a submit in flight stays in flight.
func (f *Form) Update(v View) *Form {
	type inputID struct{ blockID, actionID string }
	old := map[inputID]int{}
	for j, o := range f.fields {
		old[inputID{o.block.BlockID, o.block.Element.ActionID}] = j
	}
	nf := NewForm(v)
	nf.sending = f.sending
	for i, n := range nf.fields {
		j, ok := old[inputID{n.block.BlockID, n.block.Element.ActionID}]
		if !ok {
			continue
		}
		o := f.fields[j]
		if n.kind() != o.kind() {
			continue
		}
		if n.kind() == "plain_text_input" {
			n.input.SetValue(o.input.Value())
		} else if o.chosen >= 0 {
			n.choose(o.options[o.chosen].Value)
		}
		if j == f.focus {
			nf.focus = i
		}
	}
	return nf
}

// choose selects the option with value, and puts the cursor on it.
func (fl *field) choose(value string) {
	for j, o := range fl.options {
		if o.Value == value {
			fl.chosen, fl.cursor = j, j
		}
	}
}

func (f *Form) ViewID() string     { return f.view.ID }
func (f *Form) RootViewID() string { return f.view.RootViewID }

// HandleKey applies one key.
func (f *Form) HandleKey(msg tea.KeyMsg) (Action, tea.Cmd) {
	k := msg.String()
	if f.sending {
		return ActionNone, nil
	}
	if k == "esc" {
		return ActionCancel, nil
	}
	if !f.supported {
		if k == "o" {
			return ActionOpenInBrowser, nil
		}
		return ActionNone, nil
	}
	switch k {
	case "tab":
		return ActionNone, f.setFocus(f.focus + 1)
	case "shift+tab":
		return ActionNone, f.setFocus(f.focus - 1)
	case "ctrl+s":
		if f.view.SubmitText == "" || !f.validate() {
			return ActionNone, nil
		}
		return ActionSubmit, nil
	}
	if len(f.fields) == 0 {
		return ActionNone, nil
	}
	fl := f.fields[f.focus]
	if fl.kind() == "plain_text_input" {
		if k == "enter" && !fl.block.Element.Multiline {
			return ActionNone, nil
		}
		var cmd tea.Cmd
		fl.input, cmd = fl.input.Update(msg)
		return ActionNone, cmd
	}
	switch k {
	case "up", "k":
		if fl.cursor > 0 {
			fl.cursor--
		}
	case "down", "j":
		if fl.cursor < len(fl.options)-1 {
			fl.cursor++
		}
	case "space", " ", "enter":
		if len(fl.options) > 0 {
			fl.chosen = fl.cursor
		}
	}
	return ActionNone, nil
}

// Paste types pasted text into the focused text input.
func (f *Form) Paste(msg tea.PasteMsg) tea.Cmd {
	if !f.supported || f.sending || len(f.fields) == 0 {
		return nil
	}
	fl := f.fields[f.focus]
	if fl.kind() != "plain_text_input" {
		return nil
	}
	if !fl.block.Element.Multiline {
		msg.Content = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(msg.Content)
	}
	var cmd tea.Cmd
	fl.input, cmd = fl.input.Update(msg)
	return cmd
}

// validate runs Slack's own checks before anything is sent, and puts
// each failure under its field.
func (f *Form) validate() bool {
	ok := true
	for _, fl := range f.fields {
		fl.err = fl.check()
		if fl.err != "" {
			ok = false
		}
	}
	f.footerErr = ""
	return ok
}

func (fl *field) check() string {
	el := fl.block.Element
	if fl.kind() != "plain_text_input" {
		if fl.chosen < 0 && !fl.block.Optional {
			return "Required"
		}
		return ""
	}
	v := fl.input.Value()
	if strings.TrimSpace(v) == "" {
		if fl.block.Optional {
			return ""
		}
		return "Required"
	}
	n := utf8.RuneCountInString(v)
	if el.MinLength > 0 && n < el.MinLength {
		return "At least " + characters(el.MinLength)
	}
	if el.MaxLength > 0 && n > el.MaxLength {
		return "At most " + characters(el.MaxLength)
	}
	return ""
}

func characters(n int) string {
	if n == 1 {
		return "1 character"
	}
	return fmt.Sprintf("%d characters", n)
}

type textValue struct {
	Type  string  `json:"type"`
	Value *string `json:"value"`
}

type optionValue struct {
	Type           string  `json:"type"`
	SelectedOption *Option `json:"selected_option"`
}

// State is views.submit's state: every input's value under its block_id
// and action_id. An empty input is sent as null, as Slack hands it to
// the app.
func (f *Form) State() string {
	values := map[string]map[string]any{}
	for _, fl := range f.fields {
		var v any
		if fl.kind() == "plain_text_input" {
			tv := textValue{Type: fl.kind()}
			if s := fl.input.Value(); s != "" {
				tv.Value = &s
			}
			v = tv
		} else {
			ov := optionValue{Type: fl.kind()}
			if fl.chosen >= 0 {
				ov.SelectedOption = &fl.options[fl.chosen]
			}
			v = ov
		}
		id := fl.block.BlockID
		if values[id] == nil {
			values[id] = map[string]any{}
		}
		values[id][fl.block.Element.ActionID] = v
	}
	b, _ := json.Marshal(struct {
		Values map[string]map[string]any `json:"values"`
	}{values})
	return string(b)
}

// SetSending marks the submit in flight; every key waits for Slack's
// answer, esc included, so the answer is not dropped with the form.
func (f *Form) SetSending() { f.sending = true }

// SetSubmitError shows Slack's refusal of the submit: the app's message
// under each field it names, or the error code in the footer.
func (f *Form) SetSubmitError(code string, fieldErrors map[string]string) {
	f.sending = false
	f.footerErr = ""
	var unplaced []string
	for id, msg := range fieldErrors {
		placed := false
		for _, fl := range f.fields {
			if fl.block.BlockID == id {
				fl.err, placed = msg, true
			}
		}
		if !placed {
			unplaced = append(unplaced, msg)
		}
	}
	switch {
	case len(unplaced) > 0:
		f.footerErr = strings.Join(unplaced, " · ")
	case len(fieldErrors) == 0:
		f.footerErr = code
	}
}

var (
	bg      = func() lipgloss.Style { return lipgloss.NewStyle().Background(styles.Background) }
	plain   = func() lipgloss.Style { return bg().Foreground(styles.TextPrimary) }
	muted   = func() lipgloss.Style { return bg().Foreground(styles.TextMuted) }
	bold    = func() lipgloss.Style { return plain().Bold(true) }
	warning = func() lipgloss.Style { return bg().Foreground(styles.Warning) }
	cursor  = func() lipgloss.Style { return plain().Reverse(true) }
)

// wrap breaks unstyled s into lines of at most w cells as lipgloss
// counts them, at spaces where it can and inside a word too long for a
// line. messages.WordWrap breaks such a word with x/ansi, which counts
// a keycap emoji (1️⃣) as one cell where the box pads it as two.
func wrap(s string, w int) []string {
	w = max(w, 1)
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		line, lineW := "", 0
		for _, word := range strings.Fields(para) {
			wordW := lipgloss.Width(word)
			switch {
			case line == "":
			case lineW+1+wordW <= w:
				line, lineW = line+" "+word, lineW+1+wordW
				continue
			default:
				lines = append(lines, line)
			}
			line, lineW = "", 0
			for g := uniseg.NewGraphemes(word); g.Next(); {
				gW := lipgloss.Width(g.Str())
				if line != "" && lineW+gW > w {
					lines = append(lines, line)
					line, lineW = "", 0
				}
				line, lineW = line+g.Str(), lineW+gW
			}
		}
		lines = append(lines, line)
	}
	return lines
}

// clampStyled cuts a styled line to w cells as lipgloss counts them,
// keeping every escape sequence so the styles still close. The textarea
// measures a keycap emoji as one cell, so its rows can run wide.
func clampStyled(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	var b strings.Builder
	used, text := 0, ""
	flush := func() {
		for g := uniseg.NewGraphemes(text); g.Next(); {
			if used+lipgloss.Width(g.Str()) > w {
				used = w + 1 // nothing after a cut, not even a narrower grapheme
				break
			}
			used += lipgloss.Width(g.Str())
			b.WriteString(g.Str())
		}
		text = ""
	}
	var state byte
	for len(s) > 0 {
		seq, _, n, next := ansi.DecodeSequence(s, state, nil)
		state, s = next, s[n:]
		if strings.HasPrefix(seq, "\x1b") {
			flush()
			b.WriteString(seq)
			continue
		}
		text += seq
	}
	flush()
	return b.String()
}

func styled(st lipgloss.Style, lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = st.Render(l)
	}
	return out
}

// body draws the form's blocks at width w. focusTop and focusEnd bound
// the focused input's rows, for the caller's scroll window.
func (f *Form) body(w int) (lines []string, focusTop, focusEnd int) {
	if !f.supported {
		lines = append(lines, bold().Render("This form needs Slack"))
		lines = append(lines, styled(muted(), wrap("It has parts slk can't show. Open the message in the browser to run the shortcut there.", w))...)
		return lines, 0, 0
	}
	idx := -1 // the field of the input block at hand
	for i, b := range f.view.Blocks {
		if i > 0 {
			lines = append(lines, "")
		}
		switch b.Type {
		case "header":
			if b.Text != nil {
				lines = append(lines, styled(bold(), wrap(b.Text.Text, w))...)
			}
		case "section":
			if b.Text != nil {
				lines = append(lines, styled(plain(), wrap(b.Text.Text, w))...)
			}
			for _, t := range b.Fields {
				lines = append(lines, styled(plain(), wrap(t.Text, w))...)
			}
		case "context":
			var parts []string
			for _, e := range b.Elements {
				var text string
				if json.Unmarshal(e.Text, &text) == nil && text != "" {
					parts = append(parts, text)
				}
			}
			lines = append(lines, styled(muted(), wrap(strings.Join(parts, "  "), w))...)
		case "divider":
			lines = append(lines, muted().Render(strings.Repeat("─", w)))
		case "input":
			idx++
			if idx == f.focus {
				focusTop = len(lines)
			}
			lines = append(lines, f.fields[idx].render(w, idx == f.focus)...)
			if idx == f.focus {
				focusEnd = len(lines)
			}
		}
	}
	return lines, focusTop, focusEnd
}

// render draws an input: its label, its control, and its error.
// Controls sit two cells in, under the label; the focused field's label
// starts with ›.
func (fl *field) render(w int, focused bool) []string {
	marker := "  "
	if focused {
		marker = "› "
	}
	inner := w - 2
	if inner < 1 {
		inner = 1
	}
	label := ""
	if fl.block.Label != nil {
		label = fl.block.Label.Text
	}
	optional := ""
	if fl.block.Optional {
		optional = " optional"
	}
	head := plain().Render(marker) + bold().Render(cellwidth.Cut(label, max(inner-len(optional), 1))) + muted().Render(optional)
	lines := []string{head}
	indent := plain().Render("  ")
	switch fl.kind() {
	case "plain_text_input":
		fl.input.SetWidth(inner)
		for _, l := range strings.Split(fl.input.View(), "\n") {
			lines = append(lines, indent+clampStyled(l, inner))
		}
	case "radio_buttons":
		for j := range fl.options {
			lines = append(lines, indent+fl.optionRow(j, focused, inner))
		}
	case "static_select":
		if !focused {
			text := muted().Render("Choose an option ▾")
			if el := fl.block.Element; fl.chosen < 0 && el.Placeholder != nil {
				text = muted().Render(cellwidth.Cut(el.Placeholder.Text, inner-2) + " ▾")
			}
			if fl.chosen >= 0 {
				text = plain().Render(cellwidth.Cut(fl.options[fl.chosen].Text.Text, inner-2) + " ▾")
			}
			lines = append(lines, indent+text)
			break
		}
		start := fl.cursor - selectRows/2
		if start > len(fl.options)-selectRows {
			start = len(fl.options) - selectRows
		}
		if start < 0 {
			start = 0
		}
		for j := start; j < len(fl.options) && j < start+selectRows; j++ {
			lines = append(lines, indent+fl.optionRow(j, true, inner))
		}
	}
	if fl.err != "" {
		lines = append(lines, indent+warning().Render(cellwidth.Cut("! "+fl.err, inner)))
	}
	return lines
}

// optionRow is "( ) Positive" or "(•) Negative"; the row under the
// cursor of a focused field is drawn in reverse video.
func (fl *field) optionRow(j int, focused bool, w int) string {
	mark := "( ) "
	if j == fl.chosen {
		mark = "(•) "
	}
	row := cellwidth.Cut(mark+fl.options[j].Text.Text, w)
	if focused && j == fl.cursor {
		return cursor().Render(row)
	}
	return plain().Render(row)
}

// footer is the form's key hint, with any submit error above it.
func (f *Form) footer(w int) []string {
	var lines []string
	if f.footerErr != "" {
		lines = append(lines, styled(warning(), wrap("! "+f.footerErr, w))...)
	}
	var hint string
	switch {
	case !f.supported:
		hint = "o open in browser · esc " + f.view.CloseText
	case f.sending:
		hint = "Sending…"
	case f.view.SubmitText == "":
		hint = "tab next · esc " + f.view.CloseText
	default:
		hint = "tab next · ctrl+s " + f.view.SubmitText + " · esc " + f.view.CloseText
	}
	return append(lines, muted().Render(cellwidth.Cut(hint, w)))
}
