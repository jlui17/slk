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
