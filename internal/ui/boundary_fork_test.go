package ui

// tuiForkExempt lists the fork files TestTUIReachesTheAppOnlyThroughCore
// lets import slack-go outside the Block Kit renderer, keyed by path
// relative to internal/ui.
var tuiForkExempt = map[string]map[string]bool{
	// Fixture builders shared by the blockkit and messages tests.
	"messages/blockkit/blockkittest/blockkittest.go": {slackGo: true},
}
