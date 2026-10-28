package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeBrowser writes an executable that records its first two
// arguments, and returns its path and a func that waits for the record.
func fakeBrowser(t *testing.T) (script string, invokedWith func() string) {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	script = filepath.Join(dir, "browser")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s|%s' \"$1\" \"$2\" > "+out+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return script, func() string {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for {
			if got, err := os.ReadFile(out); err == nil {
				return string(got)
			}
			if time.Now().After(deadline) {
				t.Fatal("browser script never ran")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func fallbackMustNotRun(t *testing.T) func(string) error {
	return func(target string) error {
		t.Errorf("fallback launched %q; $BROWSER should have", target)
		return nil
	}
}

// $BROWSER overrides the per-OS launcher: it must be invoked with the
// URL as the only argument. This is how tools/run-docker.sh bridges
// link opens out of the container, where no browser exists.
func TestBrowserLauncher_BrowserEnvOverride(t *testing.T) {
	script, invokedWith := fakeBrowser(t)
	t.Setenv("BROWSER", script)

	if err := browserLauncher(fallbackMustNotRun(t))("https://example.com/x"); err != nil {
		t.Fatal(err)
	}
	if got := invokedWith(); got != "https://example.com/x|" {
		t.Fatalf("browser invoked with %q", got)
	}
}

// A multi-word $BROWSER ("open -a Firefox") is a command plus flags,
// not a binary whose name contains spaces: the URL must land after
// the flags.
func TestBrowserLauncher_BrowserEnvMultiWord(t *testing.T) {
	script, invokedWith := fakeBrowser(t)
	t.Setenv("BROWSER", script+" --new-tab")

	if err := browserLauncher(fallbackMustNotRun(t))("https://example.com/x"); err != nil {
		t.Fatal(err)
	}
	if got := invokedWith(); got != "--new-tab|https://example.com/x" {
		t.Fatalf("browser invoked with %q", got)
	}
}

// The error is what the TUI's openURLCmd turns into its failure toast.
func TestBrowserLauncher_BrowserLaunchFailureReturnsError(t *testing.T) {
	t.Setenv("BROWSER", "/nonexistent/browser")
	if err := browserLauncher(fallbackMustNotRun(t))("https://example.com/x"); err == nil {
		t.Fatal("err = nil, want the failed launch")
	}
}

// The browser child is reaped once it exits. In docker slk runs as
// PID 1, so an unwaited child stays a zombie for the life of the
// session, one per link opened. /proc shows the state: a zombie's
// stat reads "Z", a reaped child's entry is gone.
func TestBrowserLauncher_ReapsTheBrowserProcess(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	script := filepath.Join(dir, "browser")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho $$ > "+pidFile+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BROWSER", script)
	if err := browserLauncher(fallbackMustNotRun(t))("https://example.com/x"); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var stat string
	for time.Now().Before(deadline) {
		if pid, err := os.ReadFile(pidFile); err == nil && len(pid) > 0 {
			b, err := os.ReadFile("/proc/" + strings.TrimSpace(string(pid)) + "/stat")
			if os.IsNotExist(err) {
				return // reaped
			}
			stat = string(b)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("browser process never reaped; /proc stat %q", stat)
}

func TestBrowserLauncher_FallsBackToTheOSLauncher(t *testing.T) {
	script, _ := fakeBrowser(t)
	tests := []struct {
		name, browser, target string
	}{
		{"URL without $BROWSER", "", "https://example.com/x"},
		{"file path despite $BROWSER", script, "/tmp/slk-files/report.pdf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BROWSER", tt.browser)
			var launched []string
			fallback := func(target string) error {
				launched = append(launched, target)
				return nil
			}
			if err := browserLauncher(fallback)(tt.target); err != nil {
				t.Fatal(err)
			}
			if len(launched) != 1 || launched[0] != tt.target {
				t.Fatalf("fallback launched %q, want [%q]", launched, tt.target)
			}
		})
	}
}

func TestIsURL(t *testing.T) {
	tests := map[string]bool{
		"https://example.com/x":         true,
		"http://example.com":            true,
		"mailto:someone@example.com":    true,
		"/home/me/Downloads/report.pdf": false,
		"downloads/report.pdf":          false,
		"downloads/10:30 notes.txt":     false,
		`C:\Users\me\report.pdf`:        false,
	}
	for target, want := range tests {
		if got := isURL(target); got != want {
			t.Errorf("isURL(%q) = %v, want %v", target, got, want)
		}
	}
}
