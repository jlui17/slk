package ui

// PaneStateRecorder persists the pane's currently open state so a
// relaunch can restore it: teamID is the workspace owning channelID,
// threadTS the thread panel's thread (empty when no thread panel is
// open). The implementation writes storage.
type PaneStateRecorder func(teamID, channelID, threadTS string)

// SetPaneStateRecorder installs the pane-state recorder. Unset,
// reporting is inert.
func (a *App) SetPaneStateRecorder(rec PaneStateRecorder) {
	a.paneStateRecorder = rec
}

// reportPaneState publishes the pane's open state through the recorder.
// Called from the three places open state changes: channel select,
// setThreadPanel, and CloseThread. The workspace is a.activeTeamID
// read HERE, not resolved by the recorder: during a workspace switch
// the router already points at the new workspace while CloseThread
// still reports the old workspace's channel, and only activeTeamID
// (reassigned after CloseThread in reduceWorkspaceSwitched) pairs them
// correctly. Empty IDs (no channel open yet at boot) are skipped so
// they can't clobber a persisted state.
func (a *App) reportPaneState(channelID, threadTS string) {
	if a.paneStateRecorder == nil || channelID == "" || a.activeTeamID == "" {
		return
	}
	next := paneReport{teamID: a.activeTeamID, channelID: channelID, threadTS: threadTS}
	if a.lastPaneReport == next {
		return
	}
	a.lastPaneReport = next
	a.paneStateRecorder(next.teamID, next.channelID, next.threadTS)
}

// paneReport is reportPaneState's dedupe key. The workspace is part of
// it: a Slack Connect shared channel keeps its channel ID across
// workspaces, so (channelID, threadTS) alone would swallow a report
// after a workspace switch that lands on the same shared channel.
type paneReport struct {
	teamID    string
	channelID string
	threadTS  string
}
