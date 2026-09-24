package config

// Herdr configures the herdr agent-sidebar integration, which activates
// only when slk runs inside a herdr pane (HERDR_ENV/HERDR_PANE_ID set).
type Herdr struct {
	// Disabled turns off agent-thread reporting even inside a herdr pane.
	Disabled bool `toml:"disabled"`
	// OpenCommand launches the second slk instance when the O
	// keybinding opens a link in a new herdr tab: the tab's shell runs
	// `<open_command> '<permalink>'`. The tab's shell is a host shell
	// even when slk itself runs in a container, so this must be the
	// host-side launch command. Empty means "slk".
	OpenCommand string `toml:"open_command"`
	// TabNameModel enables model-generated tab labels: the Anthropic
	// model (e.g. "claude-haiku-4-5") asked to name the tab after the
	// open agent thread's root message, refining the deterministic
	// label. Empty means deterministic labels only. Needs an API key:
	// AnthropicAPIKey, or the ANTHROPIC_API_KEY env var.
	TabNameModel string `toml:"tab_name_model"`
	// AnthropicAPIKey is the key the tab_name_model calls use. Empty
	// falls back to the ANTHROPIC_API_KEY env var. A secret: never log it.
	AnthropicAPIKey string `toml:"anthropic_api_key"`
	// TabNameHints are freeform per-user lines handed to the :retitle
	// model as naming guidance (e.g. "task ids look like colony-123").
	TabNameHints []string `toml:"tab_name_hints"`
}
