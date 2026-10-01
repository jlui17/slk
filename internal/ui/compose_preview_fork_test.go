package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/compose"
	"github.com/gammons/slk/internal/ui/imgrender"
)

const (
	pastePreviewTestWidth  = 60
	pastePreviewTestHeight = 40
)

func pastePNG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newPastePreviewApp(t *testing.T, proto imgpkg.Protocol) *App {
	t.Helper()
	app := NewApp()
	app.SetImageProtocol(proto)
	app.SetImageContext(imgrender.ImageContext{CellPixels: image.Pt(8, 16)})
	return app
}

func pastedImage(b []byte) compose.PendingAttachment {
	return compose.PendingAttachment{Filename: "paste.png", Bytes: b, Mime: "image/png", Size: int64(len(b))}
}

// previewRows returns the rows withPastePreview put above the plain
// compose view, after checking the plain view is still the suffix.
func previewRows(t *testing.T, app *App, contentHeight int) []string {
	t.Helper()
	plain := app.compose.View(pastePreviewTestWidth, true)
	got := app.withPastePreview(&app.compose, plain, pastePreviewTestWidth, contentHeight)
	if got == plain {
		return nil
	}
	if !strings.HasSuffix(got, "\n"+plain) {
		t.Fatalf("the plain compose view must stay the suffix, got:\n%q", got)
	}
	return strings.Split(strings.TrimSuffix(got, "\n"+plain), "\n")
}

func TestPastePreview_HalfBlockRowsAboveCompose(t *testing.T) {
	app := newPastePreviewApp(t, imgpkg.ProtoHalfBlock)
	app.compose.AddAttachment(pastedImage(pastePNG(t, 800, 600, color.RGBA{200, 30, 30, 255})))

	rows := previewRows(t, app, pastePreviewTestHeight)
	if len(rows) == 0 || len(rows) > 8 {
		t.Fatalf("expected 1..8 preview rows, got %d", len(rows))
	}
	for i, row := range rows {
		if w := lipgloss.Width(row); w != pastePreviewTestWidth {
			t.Errorf("row %d is %d cells wide, want %d", i, w, pastePreviewTestWidth)
		}
	}
	if !strings.Contains(rows[0], "▀") {
		t.Errorf("expected half-block cells, got %q", rows[0])
	}
	// 800x600 on an 8x16 cell is 100x37 cells: the 8-row cap binds,
	// and 8 rows of 4:3 is 21 columns.
	if got := strings.Count(rows[0], "▀"); len(rows) != 8 || got != 21 {
		t.Errorf("expected an 8-row, 21-column thumbnail, got %d rows x %d cols", len(rows), got)
	}
}

func TestPastePreview_NothingToPreview_ViewUnchanged(t *testing.T) {
	cases := map[string]func(*App){
		"no attachments": func(*App) {},
		"path only": func(a *App) {
			a.compose.AddAttachment(compose.PendingAttachment{Filename: "a.png", Path: "/tmp/a.png", Mime: "image/png", Size: 10})
		},
		"bytes that are not an image": func(a *App) {
			a.compose.AddAttachment(pastedImage([]byte("\x89PNG\r\n\x1a\nfake")))
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			app := newPastePreviewApp(t, imgpkg.ProtoHalfBlock)
			setup(app)
			if rows := previewRows(t, app, pastePreviewTestHeight); rows != nil {
				t.Fatalf("expected the compose view unchanged, got %d preview rows", len(rows))
			}
		})
	}
	t.Run("images off", func(t *testing.T) {
		app := newPastePreviewApp(t, imgpkg.ProtoOff)
		app.compose.AddAttachment(pastedImage(pastePNG(t, 80, 80, color.White)))
		if rows := previewRows(t, app, pastePreviewTestHeight); rows != nil {
			t.Fatalf("expected the compose view unchanged, got %d preview rows", len(rows))
		}
	})
}

func TestPastePreview_GoneOnceTheAttachmentIs(t *testing.T) {
	app := newPastePreviewApp(t, imgpkg.ProtoHalfBlock)
	app.compose.AddAttachment(pastedImage(pastePNG(t, 80, 80, color.White)))
	app.compose.AddAttachment(pastedImage(pastePNG(t, 80, 80, color.Black)))
	if rows := previewRows(t, app, pastePreviewTestHeight); rows == nil {
		t.Fatal("expected preview rows with two pasted images")
	}

	app.compose.RemoveLastAttachment()
	if rows := previewRows(t, app, pastePreviewTestHeight); rows == nil {
		t.Fatal("expected preview rows with one pasted image left")
	}
	app.compose.ClearAttachments()
	if rows := previewRows(t, app, pastePreviewTestHeight); rows != nil {
		t.Fatalf("expected no preview rows after the clear, got %d", len(rows))
	}
	if len(pastePreviewMemo) != 0 {
		t.Fatalf("the memo must let go of images no compose box holds, has %d", len(pastePreviewMemo))
	}
}

func TestPastePreview_SideBySide_StopsAtWidth(t *testing.T) {
	app := newPastePreviewApp(t, imgpkg.ProtoHalfBlock)
	// Each 256x128 image is exactly 32x8 cells; 60 columns fit one
	// thumbnail, a gap, and not a second.
	app.compose.AddAttachment(pastedImage(pastePNG(t, 256, 128, color.White)))
	app.compose.AddAttachment(pastedImage(pastePNG(t, 256, 128, color.Black)))
	rows := previewRows(t, app, pastePreviewTestHeight)
	if got := strings.Count(rows[0], "▀"); got != 32 {
		t.Fatalf("expected one 32-column thumbnail in 60 columns, got %d cells", got)
	}

	plain := app.compose.View(80, true)
	wide := app.withPastePreview(&app.compose, plain, 80, pastePreviewTestHeight)
	first := strings.SplitN(wide, "\n", 2)[0]
	if got := strings.Count(first, "▀"); got != 64 {
		t.Fatalf("expected two 32-column thumbnails in 80 columns, got %d cells", got)
	}
	if w := lipgloss.Width(first); w != 80 {
		t.Fatalf("row is %d cells wide, want 80", w)
	}
}

func TestPastePreview_TinyImageIsNotUpscaled(t *testing.T) {
	app := newPastePreviewApp(t, imgpkg.ProtoHalfBlock)
	app.compose.AddAttachment(pastedImage(pastePNG(t, 16, 16, color.White)))
	rows := previewRows(t, app, pastePreviewTestHeight)
	if len(rows) != 1 || strings.Count(rows[0], "▀") != 2 {
		t.Fatalf("a 16x16 image on an 8x16 cell is 2x1 cells, got %d rows: %q", len(rows), rows)
	}
}

func TestPastePreview_ShortPane_FewerRowsThenNone(t *testing.T) {
	app := newPastePreviewApp(t, imgpkg.ProtoHalfBlock)
	app.compose.AddAttachment(pastedImage(pastePNG(t, 800, 600, color.White)))
	composeHeight := lipgloss.Height(app.compose.View(pastePreviewTestWidth, true))

	roomFor4 := composeHeight + pastePreviewPaneReserve + 4
	if rows := previewRows(t, app, roomFor4); len(rows) != 4 {
		t.Fatalf("expected 4 preview rows when only 4 are spare, got %d", len(rows))
	}
	if rows := previewRows(t, app, roomFor4-3); rows != nil {
		t.Fatalf("expected no preview when only 1 row is spare, got %d", len(rows))
	}
}

func TestPastePreview_Kitty_SameSizeImagesGetTheirOwnID(t *testing.T) {
	t.Setenv("TMUX", "")
	var uploads bytes.Buffer
	prev := imgpkg.KittyOutput
	imgpkg.KittyOutput = &uploads
	t.Cleanup(func() { imgpkg.KittyOutput = prev })

	app := newPastePreviewApp(t, imgpkg.ProtoKitty)
	app.compose.AddAttachment(pastedImage(pastePNG(t, 128, 128, color.RGBA{10, 200, 10, 255})))
	app.compose.AddAttachment(pastedImage(pastePNG(t, 128, 128, color.RGBA{10, 10, 200, 255})))

	rows := previewRows(t, app, pastePreviewTestHeight)
	if len(rows) == 0 || !strings.ContainsRune(rows[0], imgpkg.PlaceholderRune) {
		t.Fatalf("expected kitty placeholder rows, got %q", rows)
	}
	for i, row := range rows {
		if w := lipgloss.Width(row); w != pastePreviewTestWidth {
			t.Errorf("row %d is %d cells wide, want %d", i, w, pastePreviewTestWidth)
		}
	}
	// The kitty image id is the placeholder cells' foreground colour.
	ids := map[string]bool{}
	for _, m := range regexp.MustCompile(`\x1b\[38;2;\d+;\d+;\d+m`).FindAllString(rows[0], -1) {
		ids[m] = true
	}
	if len(ids) != 2 {
		t.Fatalf("two different images of one size must get two kitty ids, got %v", ids)
	}
	if got := strings.Count(uploads.String(), "a=T"); got != 2 {
		t.Fatalf("expected 2 kitty uploads, got %d", got)
	}

	uploads.Reset()
	previewRows(t, app, pastePreviewTestHeight)
	if uploads.Len() != 0 {
		t.Fatal("a second frame must not upload again")
	}
}

func TestPastePreview_SecondFrameComesFromTheMemo(t *testing.T) {
	app := newPastePreviewApp(t, imgpkg.ProtoHalfBlock)
	app.compose.AddAttachment(pastedImage(pastePNG(t, 80, 80, color.White)))
	previewRows(t, app, pastePreviewTestHeight)
	if len(pastePreviewMemo) != 1 {
		t.Fatalf("expected one memo entry, got %d", len(pastePreviewMemo))
	}
	for k := range pastePreviewMemo {
		pastePreviewMemo[k] = pasteThumb{cols: 4, lines: []string{"MEMO"}}
	}
	rows := previewRows(t, app, pastePreviewTestHeight)
	if len(rows) != 1 || !strings.Contains(rows[0], "MEMO") {
		t.Fatalf("the second frame must reuse the memo, not decode again; got %q", rows)
	}
}

// The real Ctrl+V path: the pasted image shows above its chip in the
// app's frame.
func TestPastePreview_AfterCtrlV_ShowsInTheFrame(t *testing.T) {
	app := newAsyncPasteApp(t, pastePNG(t, 800, 600, color.RGBA{200, 30, 30, 255}), nil)
	app.SetImageProtocol(imgpkg.ProtoHalfBlock)
	app.SetImageContext(imgrender.ImageContext{CellPixels: image.Pt(8, 16)})
	_, _ = app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	halfBlockRows := func() (n, chipRow, lastHalfBlockRow int) {
		for i, line := range strings.Split(app.View().Content, "\n") {
			if strings.Contains(line, "▀") {
				n++
				lastHalfBlockRow = i
			}
			if strings.Contains(line, "📎") {
				chipRow = i
			}
		}
		return
	}
	if n, _, _ := halfBlockRows(); n != 0 {
		t.Fatalf("expected no half-block rows before the paste, got %d", n)
	}

	runSnapshot(t, app, app.handleInsertMode(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl}))

	n, chipRow, last := halfBlockRows()
	if n != 8 {
		t.Fatalf("expected 8 half-block rows after the paste, got %d", n)
	}
	if chipRow == 0 || last >= chipRow {
		t.Fatalf("the preview (last row %d) must sit above the chip (row %d)", last, chipRow)
	}
}
