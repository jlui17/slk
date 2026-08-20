package config

// Herdr configures the herdr agent-sidebar integration, which activates
// only when slk runs inside a herdr pane (HERDR_ENV/HERDR_PANE_ID set).
type Herdr struct {
	// Disabled turns off agent-thread reporting even inside a herdr pane.
	Disabled bool `toml:"disabled"`
}
