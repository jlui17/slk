package ui

// tuiForkExempt lists the fork files TestTUIReachesTheAppOnlyThroughCore
// lets import a banned package, keyed by path relative to internal/ui.
// Each entry is fork code that predates upstream's boundary rule; the
// rule-abiding home for the first two is cmd/slk, behind
// core.DesktopService.
var tuiForkExempt = map[string]map[string]bool{
	// $BROWSER launch for openURLCmd.
	"app_fork.go": {"os/exec": true},
	// HTTP reader for the docker host clipboard bridge.
	"clipboard_remote.go": {"net/http": true},
	// Fixture builders shared by the blockkit and messages tests.
	"messages/blockkit/blockkittest/blockkittest.go": {slackGo: true},
}
