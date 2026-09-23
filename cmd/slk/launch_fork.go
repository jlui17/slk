package main

import (
	"net/url"
	"os"
	"os/exec"
	"strings"
)

// browserLauncher returns the desktop service's opener: URLs go to
// $BROWSER when set (the conventional override; also how
// tools/run-docker.sh bridges container link-opens to the host
// browser), everything else to fallback. File paths (the image viewer,
// a downloaded attachment) stay on fallback because $BROWSER names a
// browser, not a file handler, and the docker bridge's host can't see
// container paths.
func browserLauncher(fallback func(target string) error) func(target string) error {
	// Word-split: $BROWSER conventionally carries flags
	// ("open -a Firefox", "firefox --new-tab"), and a multi-word
	// value used whole as argv[0] would fail every launch.
	argv := strings.Fields(os.Getenv("BROWSER"))
	if len(argv) == 0 {
		return fallback
	}
	return func(target string) error {
		if !isURL(target) {
			return fallback(target)
		}
		return exec.Command(argv[0], append(argv[1:], target)...).Start()
	}
}

// isURL reports whether target is a URL rather than a file path. A
// one-letter scheme is a Windows drive letter ("C:\...").
func isURL(target string) bool {
	u, err := url.Parse(target)
	return err == nil && len(u.Scheme) > 1
}
