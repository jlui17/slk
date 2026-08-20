package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// $BROWSER overrides the per-OS launcher: openURLCmd must invoke it
// with the URL as the only argument. This is how tools/run-docker.sh
// bridges link opens out of the container, where no browser exists.
func TestOpenURLCmd_BrowserEnvOverride(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	script := filepath.Join(dir, "browser")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$1\" > "+out+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BROWSER", script)

	if msg := openURLCmd("https://example.com/x")(); msg != nil {
		t.Fatalf("unexpected msg %#v", msg)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := os.ReadFile(out)
		if err == nil {
			if string(got) != "https://example.com/x" {
				t.Fatalf("browser invoked with %q", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("browser script never ran")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A multi-word $BROWSER ("open -a Firefox") is a command plus flags,
// not a binary whose name contains spaces: the URL must land after
// the flags.
func TestOpenURLCmd_BrowserEnvMultiWord(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	script := filepath.Join(dir, "browser")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s %s' \"$1\" \"$2\" > "+out+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BROWSER", script+" --new-tab")

	if msg := openURLCmd("https://example.com/x")(); msg != nil {
		t.Fatalf("unexpected msg %#v", msg)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := os.ReadFile(out)
		if err == nil {
			if string(got) != "--new-tab https://example.com/x" {
				t.Fatalf("browser invoked with %q", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("browser script never ran")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestOpenURLCmd_BrowserLaunchFailureToasts(t *testing.T) {
	t.Setenv("BROWSER", "/nonexistent/browser")
	msg := openURLCmd("https://example.com/x")()
	toast, ok := msg.(ToastMsg)
	if !ok || toast.Text != "Failed to open link" {
		t.Fatalf("msg = %#v, want failure toast", msg)
	}
}
