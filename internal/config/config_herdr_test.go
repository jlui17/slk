package config

import "testing"

func TestResolveAnthropicAPIKey(t *testing.T) {
	rows := []struct {
		name, config, env, want string
	}{
		{"config only", "from-config", "", "from-config"},
		{"env only", "", "from-env", "from-env"},
		{"config wins over env", "from-config", "from-env", "from-config"},
		{"neither", "", "", ""},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Setenv("ANTHROPIC_API_KEY", row.env)
			got := Herdr{AnthropicAPIKey: row.config}.ResolveAnthropicAPIKey()
			if got != row.want {
				t.Errorf("ResolveAnthropicAPIKey = %q, want %q", got, row.want)
			}
		})
	}
}

func TestResolveTabNameEffort(t *testing.T) {
	if got := (Herdr{}).ResolveTabNameEffort(); got != "low" {
		t.Errorf("unset: ResolveTabNameEffort = %q, want low", got)
	}
	if got := (Herdr{TabNameEffort: "high"}).ResolveTabNameEffort(); got != "high" {
		t.Errorf("set: ResolveTabNameEffort = %q, want high", got)
	}
}

func TestResolveAgentStatusJudge(t *testing.T) {
	unset := Herdr{TabNameModel: "claude-haiku-5-5", TabNameEffort: "high"}
	if got := unset.ResolveAgentStatusJudgeModel(); got != "claude-haiku-5-5" {
		t.Errorf("unset: ResolveAgentStatusJudgeModel = %q, want tab_name_model", got)
	}
	if got := unset.ResolveAgentStatusJudgeEffort(); got != "low" {
		t.Errorf("unset: ResolveAgentStatusJudgeEffort = %q, want low, not tab_name_effort", got)
	}
	set := Herdr{TabNameModel: "claude-haiku-5-5", AgentStatusJudgeModel: "claude-sonnet-5-5", AgentStatusJudgeEffort: "medium"}
	if got := set.ResolveAgentStatusJudgeModel(); got != "claude-sonnet-5-5" {
		t.Errorf("set: ResolveAgentStatusJudgeModel = %q, want claude-sonnet-5-5", got)
	}
	if got := set.ResolveAgentStatusJudgeEffort(); got != "medium" {
		t.Errorf("set: ResolveAgentStatusJudgeEffort = %q, want medium", got)
	}
}
