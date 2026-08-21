package config

// Restore configures pane-state restore: relaunching slk reopens the
// workspace, channel, and thread that were open when the process last
// ran (keyed per herdr pane, one shared slot outside herdr).
type Restore struct {
	// Disabled turns off both the recording and the restore-at-boot.
	Disabled bool `toml:"disabled"`
}
