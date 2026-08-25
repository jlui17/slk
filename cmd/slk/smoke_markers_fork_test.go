package main

import (
	"os"
	"strings"
	"testing"
)

// tools/smoke.sh greps the debug log for these literals; nothing else
// ties them to the debuglog format strings that emit them, so a reword
// would leave smoke asserting against strings that never appear
// (vacuously green). A marker moving is fine — update smoke.sh and the
// source file named here together.
func TestSmokeMarkersStillEmitted(t *testing.T) {
	for _, tc := range []struct{ file, marker string }{
		{"main.go", "shutdown API request tally"},
		{"main.go", "failed to connect"},
		{"reconnect_sync.go", "reconnect-sync"},
	} {
		src, err := os.ReadFile(tc.file)
		if err != nil {
			t.Fatalf("reading %s: %v", tc.file, err)
		}
		if !strings.Contains(string(src), tc.marker) {
			t.Errorf("%s no longer contains %q — update tools/smoke.sh's greps with it", tc.file, tc.marker)
		}
	}
}
