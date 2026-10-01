package ui

import (
	"bytes"
	"image"
	"image/color"
	"reflect"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/compose"
	"github.com/gammons/slk/internal/ui/imgrender"
	"github.com/gammons/slk/internal/ui/messages"
)

// pasteFocusApp is a sized App in insert mode with pasted images in a
// compose box, and a record of every upload it starts.
type pasteFocusApp struct {
	*App
	box     *compose.Model
	uploads int
}

func namedPaste(name string, b []byte) compose.PendingAttachment {
	att := pastedImage(b)
	att.Filename = name
	return att
}

// newPasteFocusApp types "hello" into the channel compose box, or the
// thread's when thread is set, and attaches one 8x4-cell image per
// name.
func newPasteFocusApp(t *testing.T, thread bool, names ...string) *pasteFocusApp {
	t.Helper()
	p := &pasteFocusApp{}
	p.App = newHarnessApp(t, withHarnessSize(160, 30), withApp(func(a *App) {
		a.SetImageProtocol(imgpkg.ProtoHalfBlock)
		a.SetImageContext(imgrender.ImageContext{CellPixels: image.Pt(8, 16)})
		a.activeChannelID = "C1"
		wireUploader(a, func(string, string, string, []compose.PendingAttachment) tea.Cmd {
			p.uploads++
			return nil
		})
		a.focusedPanel = PanelMessages
		p.box = &a.compose
		if thread {
			a.threadPanel.SetThread(messages.MessageItem{TS: "P1"}, nil, "C1", "P1")
			a.threadVisible = true
			a.focusedPanel = PanelThread
			p.box = &a.threadCompose
		}
		a.SetMode(ModeInsert)
		_ = p.box.Focus()
		p.box.SetValue("hello")
		for i, name := range names {
			p.box.AddAttachment(namedPaste(name, pastePNG(t, 64, 64, color.RGBA{uint8(40 * i), 90, 200, 255})))
		}
	}))
	return p
}

// press sends each key through Update and draws a frame after it, the
// way the runtime does.
func (p *pasteFocusApp) press(keys ...tea.KeyMsg) {
	for _, k := range keys {
		_, _ = p.Update(k)
		_ = p.View()
	}
}

func (p *pasteFocusApp) attachmentNames() []string {
	var names []string
	for _, att := range p.box.Attachments() {
		names = append(names, att.Filename)
	}
	return names
}

// Enter must never send the message while a thumbnail has the focus.
func TestPasteThumbFocus_EnterViewsTheImageAndSendsNothing(t *testing.T) {
	for name, thread := range map[string]bool{"channel": false, "thread": true} {
		t.Run(name, func(t *testing.T) {
			p := newPasteFocusApp(t, thread, "a.png")

			p.press(keyCode(tea.KeyUp), keyCode(tea.KeyEnter))

			if p.uploads != 0 {
				t.Fatalf("Enter on a thumbnail started %d uploads, want none", p.uploads)
			}
			if got := p.attachmentNames(); !reflect.DeepEqual(got, []string{"a.png"}) {
				t.Fatalf("attachments = %v, want [a.png]", got)
			}
			if got := p.box.Value(); got != "hello" {
				t.Fatalf("text = %q, want it unchanged", got)
			}
			if p.box.Uploading() || p.mode != ModeInsert {
				t.Fatalf("uploading=%v mode=%v, want an idle box still in insert mode", p.box.Uploading(), p.mode)
			}
			if !p.preview.Active() {
				t.Fatal("Enter on a thumbnail must open the full-screen preview")
			}
		})
	}
}

const (
	pasteThumbTextHint  = "↑ select"
	pasteThumbFocusHint = "enter view · ⌫ remove · esc back"
)

func TestPasteThumbFocus_UpOnTheFirstRowSelectsTheLastThumbnail(t *testing.T) {
	for name, thread := range map[string]bool{"channel": false, "thread": true} {
		t.Run(name, func(t *testing.T) {
			p := newPasteFocusApp(t, thread, "a.png", "b.png")
			before := ansi.Strip(p.View().Content)
			if strings.Contains(before, "▸") || !strings.Contains(before, pasteThumbTextHint) {
				t.Fatalf("before Up: want %q and no marker, got:\n%s", pasteThumbTextHint, before)
			}

			p.press(keyCode(tea.KeyUp))

			if got := p.selectedPasteThumb(p.box); got != 1 {
				t.Fatalf("selected thumbnail = %d, want 1 (the last)", got)
			}
			after := ansi.Strip(p.View().Content)
			// The side-by-side thread pane is too narrow for the
			// longer hint, which is then left out.
			if !strings.Contains(after, "▸") || strings.Contains(after, pasteThumbFocusHint) == thread || strings.Contains(after, pasteThumbTextHint) {
				t.Fatalf("after Up: want the marker and %q, got:\n%s", pasteThumbFocusHint, after)
			}
			if a, b := strings.Count(before, "\n"), strings.Count(after, "\n"); a != b {
				t.Fatalf("selecting changed the frame from %d to %d rows", a, b)
			}
		})
	}
}

// With no image attached Up is upstream's: on the first line it jumps
// to the start of the text.
func TestPasteThumbFocus_UpWithNoImageJumpsToTheStartOfTheText(t *testing.T) {
	p := newPasteFocusApp(t, false)
	p.box.AddAttachment(compose.PendingAttachment{Filename: "notes.txt", Path: "/tmp/notes.txt", Size: 5})
	_ = p.View()

	p.press(keyCode(tea.KeyUp), keyPress('X'))

	if got := p.selectedPasteThumb(p.box); got != -1 {
		t.Fatalf("selected thumbnail = %d, want none", got)
	}
	if got := p.box.Value(); got != "Xhello" {
		t.Fatalf("text = %q, want the typed rune at the start", got)
	}
}

func TestPasteThumbFocus_UpElsewhereKeepsItsMeaning(t *testing.T) {
	t.Run("second line: the cursor moves up", func(t *testing.T) {
		p := newPasteFocusApp(t, false, "a.png")
		p.box.SetValue("one\ntwo")
		p.press(keyCode(tea.KeyUp), keyPress('X'))
		if got := p.selectedPasteThumb(p.box); got != -1 {
			t.Fatalf("selected thumbnail = %d, want none", got)
		}
		if got := p.box.Value(); got != "oneX\ntwo" {
			t.Fatalf("text = %q, want the typed rune on the first line", got)
		}
	})
	t.Run("wrapped row of the first line: jumps to the start", func(t *testing.T) {
		p := newPasteFocusApp(t, false, "a.png")
		p.box.SetValue(strings.Repeat("word ", 60))
		_ = p.View()
		p.press(keyCode(tea.KeyUp))
		if got := p.selectedPasteThumb(p.box); got != -1 {
			t.Fatalf("selected thumbnail = %d, want none", got)
		}
		p.press(keyCode(tea.KeyUp))
		if got := p.selectedPasteThumb(p.box); got != 0 {
			t.Fatalf("a second Up, now on the first row, selected %d, want 0", got)
		}
	})
	t.Run("picker open: the picker gets it", func(t *testing.T) {
		p := newPasteFocusApp(t, false, "a.png")
		p.box.SetValue("")
		p.press(keyPress('@'))
		if !p.box.IsMentionActive() {
			t.Fatal("setup: expected the mention picker open")
		}
		p.press(keyCode(tea.KeyUp))
		if got := p.selectedPasteThumb(p.box); got != -1 || !p.box.IsMentionActive() {
			t.Fatalf("selected = %d, picker open = %v; want none and open", got, p.box.IsMentionActive())
		}
	})
	t.Run("uploading", func(t *testing.T) {
		p := newPasteFocusApp(t, false, "a.png")
		p.box.SetUploading(true)
		p.press(keyCode(tea.KeyUp))
		if got := p.selectedPasteThumb(p.box); got != -1 {
			t.Fatalf("selected thumbnail = %d, want none", got)
		}
	})
}

func TestPasteThumbFocus_LeftAndRightClampAtTheEnds(t *testing.T) {
	p := newPasteFocusApp(t, false, "a.png", "b.png", "c.png")
	steps := []struct {
		key  tea.KeyMsg
		want int
	}{
		{keyCode(tea.KeyUp), 2},
		{keyCode(tea.KeyRight), 2},
		{keyPress('l'), 2},
		{keyCode(tea.KeyLeft), 1},
		{keyPress('h'), 0},
		{keyCode(tea.KeyLeft), 0},
		{keyPress('l'), 1},
	}
	for i, s := range steps {
		p.press(s.key)
		if got := p.selectedPasteThumb(p.box); got != s.want {
			t.Fatalf("step %d (%s): selected = %d, want %d", i, s.key, got, s.want)
		}
	}
}

func TestPasteThumbFocus_RemoveKeysRemoveTheSelectedImage(t *testing.T) {
	removeKeys := map[string]tea.KeyMsg{
		"backspace": keyCode(tea.KeyBackspace),
		"delete":    keyCode(tea.KeyDelete),
		"x":         keyPress('x'),
	}
	for name, removeKey := range removeKeys {
		for boxName, thread := range map[string]bool{"channel": false, "thread": true} {
			t.Run(name+" "+boxName, func(t *testing.T) {
				p := newPasteFocusApp(t, thread, "a.png", "b.png")

				p.press(keyCode(tea.KeyUp), keyCode(tea.KeyLeft), removeKey)
				if got := p.attachmentNames(); !reflect.DeepEqual(got, []string{"b.png"}) {
					t.Fatalf("attachments = %v, want [b.png]", got)
				}
				if got := p.selectedPasteThumb(p.box); got != 0 {
					t.Fatalf("selected = %d, want the remaining thumbnail", got)
				}

				p.press(removeKey)
				if got := p.attachmentNames(); got != nil {
					t.Fatalf("attachments = %v, want none", got)
				}
				if got := p.selectedPasteThumb(p.box); got != -1 {
					t.Fatalf("selected = %d, want the focus back on the text", got)
				}
				p.press(keyPress('!'))
				if got := p.box.Value(); got != "hello!" {
					t.Fatalf("text = %q, want it kept and typed into again", got)
				}
				if p.uploads != 0 || p.mode != ModeInsert {
					t.Fatalf("uploads=%d mode=%v, want nothing sent and insert mode", p.uploads, p.mode)
				}
			})
		}
	}
}

// Removing from the middle selects the thumbnail before it, and the
// selection keeps naming the right attachment.
func TestPasteThumbFocus_RemoveInTheMiddleSelectsThePrevious(t *testing.T) {
	p := newPasteFocusApp(t, false, "a.png", "b.png", "c.png")
	p.press(keyCode(tea.KeyUp), keyCode(tea.KeyLeft), keyCode(tea.KeyBackspace))
	if got := p.attachmentNames(); !reflect.DeepEqual(got, []string{"a.png", "c.png"}) {
		t.Fatalf("attachments = %v, want [a.png c.png]", got)
	}
	if got := p.selectedPasteThumb(p.box); got != 0 {
		t.Fatalf("selected = %d, want 0", got)
	}
}

// The selection counts drawn thumbnails, which skip attachments with
// no picture.
func TestPasteThumbFocus_RemoveSkipsAttachmentsWithoutAThumbnail(t *testing.T) {
	p := newPasteFocusApp(t, false)
	p.box.AddAttachment(compose.PendingAttachment{Filename: "notes.txt", Path: "/tmp/notes.txt", Size: 5})
	p.box.AddAttachment(namedPaste("a.png", pastePNG(t, 64, 64, color.White)))
	_ = p.View()

	p.press(keyCode(tea.KeyUp), keyCode(tea.KeyBackspace))

	if got := p.attachmentNames(); !reflect.DeepEqual(got, []string{"notes.txt"}) {
		t.Fatalf("attachments = %v, want [notes.txt]", got)
	}
}

func TestPasteThumbFocus_OtherKeysDoNothing(t *testing.T) {
	p := newPasteFocusApp(t, false, "a.png")
	p.press(keyCode(tea.KeyUp))
	p.press(keyPress('z'), keyPress('q'), keyPress(' '), keyCode(tea.KeyUp), keyCode(tea.KeyTab),
		tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl},
		tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift},
		tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt})

	if got := p.box.Value(); got != "hello" {
		t.Fatalf("text = %q, want it unchanged", got)
	}
	if got := p.attachmentNames(); !reflect.DeepEqual(got, []string{"a.png"}) {
		t.Fatalf("attachments = %v, want [a.png]", got)
	}
	if got := p.selectedPasteThumb(p.box); got != 0 || p.uploads != 0 || p.preview.Active() || p.mode != ModeInsert {
		t.Fatalf("selected=%d uploads=%d preview=%v mode=%v, want the thumbnail still selected and nothing else",
			got, p.uploads, p.preview.Active(), p.mode)
	}
}

func TestPasteThumbFocus_DownAndEscReturnToTheText(t *testing.T) {
	for name, back := range map[string]tea.KeyMsg{"down": keyCode(tea.KeyDown), "esc": keyCode(tea.KeyEscape)} {
		t.Run(name, func(t *testing.T) {
			p := newPasteFocusApp(t, false, "a.png")
			// Cursor between "hel" and "lo".
			p.press(keyCode(tea.KeyLeft), keyCode(tea.KeyLeft), keyCode(tea.KeyUp))
			if got := p.selectedPasteThumb(p.box); got != 0 {
				t.Fatalf("setup: selected = %d, want 0", got)
			}

			p.press(back)

			if got := p.selectedPasteThumb(p.box); got != -1 || p.mode != ModeInsert {
				t.Fatalf("selected=%d mode=%v, want the text focused in insert mode", got, p.mode)
			}
			p.press(keyPress('X'))
			if got := p.box.Value(); got != "helXlo" {
				t.Fatalf("text = %q, want the cursor where it was", got)
			}
			// A second Esc is insert mode's own.
			p.press(keyCode(tea.KeyEscape))
			if p.mode != ModeNormal {
				t.Fatalf("mode = %v after Esc on the text, want normal", p.mode)
			}
		})
	}
}

func TestPasteThumbFocus_PreviewClosesBackOntoTheSameThumbnail(t *testing.T) {
	p := newPasteFocusApp(t, false, "a.png", "b.png")
	p.press(keyCode(tea.KeyUp), keyCode(tea.KeyLeft), keyCode(tea.KeyEnter))
	if !p.preview.Active() {
		t.Fatal("expected the preview open")
	}
	if frame := ansi.Strip(p.View().Content); !strings.Contains(frame, "a.png  •  64x64") || strings.Contains(frame, "(1/") {
		t.Fatalf("expected a.png alone in the preview, got:\n%s", frame)
	}

	// The preview's keys that act on a message, its siblings or its
	// file have nothing to act on.
	for _, k := range []tea.KeyMsg{keyPress('h'), keyPress('l'), keyCode(tea.KeyLeft), keyCode(tea.KeyRight),
		keyPress('o'), keyPress('d'), keyPress('y'), keyPress('r')} {
		_, cmd := p.Update(k)
		_ = p.View()
		if msgs := drainCmds(cmd); len(msgs) != 0 || !p.preview.Active() {
			t.Fatalf("%s in the preview gave %v (preview open: %v), want nothing", k, msgs, p.preview.Active())
		}
	}

	p.press(keyCode(tea.KeyEscape))
	if p.preview.Active() || p.mode != ModeInsert {
		t.Fatalf("preview=%v mode=%v, want it closed and insert mode", p.preview.Active(), p.mode)
	}
	if got := p.selectedPasteThumb(p.box); got != 0 {
		t.Fatalf("selected = %d after the preview closed, want 0 still", got)
	}

	// Enter in the preview is "open in the system viewer": with no
	// file on disk it only closes.
	p.press(keyCode(tea.KeyEnter))
	_, cmd := p.Update(keyCode(tea.KeyEnter))
	if msgs := drainCmds(cmd); len(msgs) != 0 || p.preview.Active() {
		t.Fatalf("Enter in the preview gave %v (preview open: %v), want it closed and nothing else", msgs, p.preview.Active())
	}
	if got := p.attachmentNames(); !reflect.DeepEqual(got, []string{"a.png", "b.png"}) || p.uploads != 0 || p.box.Value() != "hello" {
		t.Fatalf("attachments=%v uploads=%d text=%q, want nothing sent or changed", got, p.uploads, p.box.Value())
	}
}

// The focus is only ever what the current state supports.
func TestPasteThumbFocus_ClearsItselfWhenItNoLongerApplies(t *testing.T) {
	cases := map[string]func(*pasteFocusApp){
		"the attachments are gone": func(p *pasteFocusApp) { p.box.ClearAttachments() },
		"insert mode was left":     func(p *pasteFocusApp) { p.SetMode(ModeNormal) },
		"the pane is too short":    func(p *pasteFocusApp) { _, _ = p.Update(tea.WindowSizeMsg{Width: 160, Height: 12}) },
		"the other box is focused": func(p *pasteFocusApp) { p.threadVisible = false },
		"images were turned off":   func(p *pasteFocusApp) { p.SetImageProtocol(imgpkg.ProtoOff) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			p := newPasteFocusApp(t, true, "a.png")
			p.press(keyCode(tea.KeyUp))
			if got := p.selectedPasteThumb(p.box); got != 0 {
				t.Fatalf("setup: selected = %d, want 0", got)
			}

			change(p)
			_ = p.View()

			if p.pasteThumbFocus.box != nil {
				t.Fatal("the thumbnail focus must clear itself")
			}
			// Back in the original state, nothing is selected.
			p.threadVisible = true
			p.SetMode(ModeInsert)
			if got := p.selectedPasteThumb(p.box); got != -1 {
				t.Fatalf("selected = %d, want none", got)
			}
		})
	}
}

// focusRows draws the preview rows of p's box at width, like
// previewRows.
func (p *pasteFocusApp) focusRows(t *testing.T, width int) []string {
	t.Helper()
	plain := p.box.View(width, true)
	got := p.withPastePreview(p.box, plain, width, pastePreviewTestHeight)
	return strings.Split(strings.TrimSuffix(got, "\n"+plain), "\n")
}

func TestPasteThumbFocus_RowsKeepTheirSize(t *testing.T) {
	p := newPasteFocusApp(t, false, "a.png", "b.png")
	const width = 70
	textRows := p.focusRows(t, width)
	p.press(keyCode(tea.KeyUp), keyCode(tea.KeyLeft))
	thumbRows := p.focusRows(t, width)

	if len(textRows) != 4 || len(thumbRows) != 4 {
		t.Fatalf("a 64x64 image is 4 rows; got %d rows unselected, %d selected", len(textRows), len(thumbRows))
	}
	for name, rows := range map[string][]string{"text focused": textRows, "thumbnail focused": thumbRows} {
		for i, row := range rows {
			if w := lipgloss.Width(row); w != width {
				t.Errorf("%s: row %d is %d cells wide, want %d", name, i, w, width)
			}
		}
	}
	// One gutter column before each 8-column thumbnail; the marker
	// fills the first thumbnail's gutter, top to bottom.
	wantGutter := []string{"▸", "│", "│", "│"}
	for i, row := range thumbRows {
		plain := ansi.Strip(row)
		if !strings.HasPrefix(plain, wantGutter[i]+strings.Repeat("▀", 8)+" "+strings.Repeat("▀", 8)) {
			t.Errorf("row %d = %q, want gutter %q, a thumbnail, a blank gutter, a thumbnail", i, plain, wantGutter[i])
		}
		if text := ansi.Strip(textRows[i]); !strings.HasPrefix(text, " "+strings.Repeat("▀", 8)+" "+strings.Repeat("▀", 8)) {
			t.Errorf("row %d = %q, want blank gutters while the text has the focus", i, text)
		}
	}
	if got := ansi.Strip(thumbRows[3]); !strings.HasPrefix(got[strings.LastIndex(got, "▀")+len("▀"):], "  "+pasteThumbFocusHint+" ") {
		t.Errorf("bottom row = %q, want two spaces then %q", got, pasteThumbFocusHint)
	}
	if got := ansi.Strip(textRows[3]); !strings.Contains(got, "▀  "+pasteThumbTextHint+" ") {
		t.Errorf("bottom row = %q, want two spaces then %q", got, pasteThumbTextHint)
	}
	for i := 0; i < 3; i++ {
		if strings.Contains(ansi.Strip(thumbRows[i]), "enter") {
			t.Errorf("row %d carries the hint, want it on the bottom row only", i)
		}
	}
}

func TestPasteThumbFocus_HintIsDroppedWhenItDoesNotFit(t *testing.T) {
	p := newPasteFocusApp(t, false, "a.png")
	p.press(keyCode(tea.KeyUp))
	// 1 + 8 columns of thumbnail, 2 of gap, and the hint.
	fits := 1 + 8 + 2 + lipgloss.Width(pasteThumbFocusHint)

	for _, width := range []int{fits, fits - 1} {
		rows := p.focusRows(t, width)
		for i, row := range rows {
			if w := lipgloss.Width(row); w != width {
				t.Errorf("width %d: row %d is %d cells wide", width, i, w)
			}
		}
		if got, want := strings.Contains(ansi.Strip(rows[len(rows)-1]), pasteThumbFocusHint), width == fits; got != want {
			t.Errorf("width %d: hint drawn = %v, want %v", width, got, want)
		}
		if !strings.HasPrefix(ansi.Strip(rows[0]), "▸") {
			t.Errorf("width %d: the marker must stay when the hint goes", width)
		}
	}
}

// Kitty placeholder cells carry the image id in their foreground: the
// selection must leave them byte for byte as they were.
func TestPasteThumbFocus_Kitty_SelectionLeavesThePlaceholderCellsAlone(t *testing.T) {
	t.Setenv("TMUX", "")
	prev := imgpkg.KittyOutput
	imgpkg.KittyOutput = &bytes.Buffer{}
	t.Cleanup(func() { imgpkg.KittyOutput = prev })

	p := newPasteFocusApp(t, false, "a.png", "b.png")
	p.SetImageProtocol(imgpkg.ProtoKitty)
	_ = p.View()
	placeholders := regexp.MustCompile(`\x1b\[38;2;\d+;\d+;\d+m` + string(imgpkg.PlaceholderRune) + `[^\x1b]*\x1b\[39m`)

	textRows := p.focusRows(t, 70)
	p.press(keyCode(tea.KeyUp))
	thumbRows := p.focusRows(t, 70)

	if len(textRows) != len(thumbRows) {
		t.Fatalf("selecting changed the row count from %d to %d", len(textRows), len(thumbRows))
	}
	for i := range textRows {
		before, after := placeholders.FindAllString(textRows[i], -1), placeholders.FindAllString(thumbRows[i], -1)
		if len(before) != 2 || !reflect.DeepEqual(before, after) {
			t.Errorf("row %d: placeholder cells changed with the selection:\n%q\n%q", i, before, after)
		}
		if textRows[i] == thumbRows[i] {
			t.Errorf("row %d: expected the gutter to differ", i)
		}
		if w := lipgloss.Width(thumbRows[i]); w != 70 {
			t.Errorf("row %d is %d cells wide, want 70", i, w)
		}
	}
	if !strings.Contains(thumbRows[0], "▸") {
		t.Errorf("expected the marker before the selected thumbnail, got %q", thumbRows[0])
	}
}
