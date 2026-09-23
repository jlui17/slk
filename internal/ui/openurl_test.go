package ui

import (
	"errors"
	"testing"

	"github.com/gammons/slk/internal/core"
)

// openURLCmd hands the URL to the desktop service's opener, which is
// where cmd/slk's browserLauncher applies $BROWSER.
func TestOpenURLCmd_OpensThroughTheDesktopService(t *testing.T) {
	app := NewApp()
	var opened []string
	app.setDesktopForTest(func(d *core.DesktopServiceFuncs) {
		d.Open = func(target string) error {
			opened = append(opened, target)
			return nil
		}
	})

	if msg := app.openURLCmd("https://example.com/x")(); msg != nil {
		t.Fatalf("unexpected msg %#v", msg)
	}
	if len(opened) != 1 || opened[0] != "https://example.com/x" {
		t.Fatalf("opened %q, want the URL once", opened)
	}
}

func TestOpenURLCmd_BrowserLaunchFailureToasts(t *testing.T) {
	app := NewApp()
	app.setDesktopForTest(func(d *core.DesktopServiceFuncs) {
		d.Open = func(string) error { return errors.New("no browser") }
	})

	msg := app.openURLCmd("https://example.com/x")()
	toast, ok := msg.(ToastMsg)
	if !ok || toast.Text != "Failed to open link" {
		t.Fatalf("msg = %#v, want failure toast", msg)
	}
}
