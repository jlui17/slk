package config

import "os"

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
	// model (e.g. "claude-sonnet-5-5") asked to name the tab after the
	// open agent thread, at open and on :retitle, refining the
	// deterministic label. Empty means deterministic labels only. Needs
	// an API key: AnthropicAPIKey, or the ANTHROPIC_API_KEY env var.
	TabNameModel string `toml:"tab_name_model"`
	// TabNameEffort is the effort the tab-label calls run at: low,
	// medium, high, xhigh or max. Empty means low.
	TabNameEffort string `toml:"tab_name_effort"`
	// AgentStatusJudgeModel is the model the agent status judge asks whether the
	// agent in a thread is working, idle or blocked on the user. Empty
	// means tab_name_model. The judge runs only when tab_name_model is set.
	AgentStatusJudgeModel string `toml:"agent_status_judge_model"`
	// AgentStatusJudgeEffort is the effort the agent status judge runs at, with the
	// same values as tab_name_effort. Empty means low.
	AgentStatusJudgeEffort string `toml:"agent_status_judge_effort"`
	// AnthropicAPIKey is the key the tab_name_model calls use. Empty
	// falls back to the ANTHROPIC_API_KEY env var. A secret: never log it.
	AnthropicAPIKey string `toml:"anthropic_api_key"`
	// TabNameHints are freeform per-user lines handed to the tab-label
	// model as naming guidance (e.g. "task ids look like colony-123").
	TabNameHints []string `toml:"tab_name_hints"`
}

// ResolveAnthropicAPIKey is the key every model call uses:
// herdr.anthropic_api_key wins, the ANTHROPIC_API_KEY env var is the
// fallback.
func (h Herdr) ResolveAnthropicAPIKey() string {
	if h.AnthropicAPIKey != "" {
		return h.AnthropicAPIKey
	}
	return os.Getenv("ANTHROPIC_API_KEY")
}

// ResolveTabNameEffort is the effort the tab-label calls run at:
// herdr.tab_name_effort, or low when unset.
func (h Herdr) ResolveTabNameEffort() string {
	if h.TabNameEffort != "" {
		return h.TabNameEffort
	}
	return "low"
}

// ResolveAgentStatusJudgeModel is the model the agent status judge calls:
// herdr.agent_status_judge_model, or herdr.tab_name_model when unset.
func (h Herdr) ResolveAgentStatusJudgeModel() string {
	if h.AgentStatusJudgeModel != "" {
		return h.AgentStatusJudgeModel
	}
	return h.TabNameModel
}

// ResolveAgentStatusJudgeEffort is the effort the agent status judge runs at:
// herdr.agent_status_judge_effort, or low when unset.
func (h Herdr) ResolveAgentStatusJudgeEffort() string {
	if h.AgentStatusJudgeEffort != "" {
		return h.AgentStatusJudgeEffort
	}
	return "low"
}
