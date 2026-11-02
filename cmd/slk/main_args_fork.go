package main

import (
	"fmt"
	"os"
	"strings"
)

// exitOnUnknownFlag exits with status 2, the flag package's code for a
// usage error, when arg is a flag main's dispatch did not match, so an
// unknown flag fails at once instead of launching the TUI.
func exitOnUnknownFlag(arg string) {
	if !strings.HasPrefix(arg, "-") {
		return
	}
	fmt.Fprintf(os.Stderr, "slk: unknown flag %q\nRun 'slk --help' for usage.\n", arg)
	os.Exit(2)
}
