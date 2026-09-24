package tablabel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// New must put the key it is handed on the wire whatever the environment
// holds: the SDK's default client reads ANTHROPIC_API_KEY on its own.
func TestNewSendsTheGivenAPIKey(t *testing.T) {
	for _, envKey := range []string{"", "from-env"} {
		t.Run("env="+envKey, func(t *testing.T) {
			var gotKey string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotKey = r.Header.Get("X-Api-Key")
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-haiku-4-5","content":[{"type":"text","text":"label"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
			}))
			defer srv.Close()
			t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
			t.Setenv("ANTHROPIC_API_KEY", envKey)

			if _, err := New("claude-haiku-4-5", "from-config").Label(context.Background(), "root"); err != nil {
				t.Fatalf("Label: %v", err)
			}
			if gotKey != "from-config" {
				t.Errorf("X-Api-Key = %q, want %q", gotKey, "from-config")
			}
		})
	}
}
