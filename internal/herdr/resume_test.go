package herdr

import (
	"reflect"
	"testing"
	"time"
)

func TestResumeArgv(t *testing.T) {
	link := "https://acme.slack.com/archives/C1/p1700000000000100?thread_ts=1700000000.000100&cid=C1"
	cases := []struct {
		name    string
		command []string
		link    string
		want    []string
	}{
		{"plain command name runs as is", []string{"slk"}, link, []string{"slk", link}},
		{"a path runs through env", []string{"/main/tools/run-docker.sh"}, link, []string{"env", "/main/tools/run-docker.sh", link}},
		{"an env launcher stays as it is", []string{"env", "SLK_ROLE=user", "/main/tools/run-docker.sh"}, link, []string{"env", "SLK_ROLE=user", "/main/tools/run-docker.sh", link}},
		{"no link: the launcher alone", []string{"slk"}, "", []string{"slk"}},
		{"no command: nothing to resume", nil, link, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResumeArgv(c.command, c.link); !reflect.DeepEqual(got, c.want) {
				t.Errorf("ResumeArgv = %q, want %q", got, c.want)
			}
		})
	}
}

// argvOf reads a request's resume_argv as strings; nil when absent.
func argvOf(t *testing.T, params map[string]any) []string {
	t.Helper()
	raw, ok := params["resume_argv"]
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		t.Fatalf("resume_argv = %v, want an array", raw)
	}
	out := make([]string, len(list))
	for i, v := range list {
		out[i] = v.(string)
	}
	return out
}

// TestReportResumeHoldsPane: with no agent thread, the resume command
// rides a report that holds the pane as an idle "slk" row titled with
// the open channel; herdr refuses a resume command from a reporter
// that does not hold the pane.
func TestReportResumeHoldsPane(t *testing.T) {
	sock := testSocketPath(t)
	_, rec := startServer(t, "unix", sock)
	r := newClient("unix", sock, "pane-1", "tab-1")
	argv := []string{"env", "SLK_ROLE=user", "/main/tools/run-docker.sh"}

	r.ReportResume(argv, "#general")
	lines := waitLines(t, rec, 2)

	method, params := decode(t, lines[0])
	if method != "pane.report_agent" {
		t.Fatalf("first method = %q, want pane.report_agent", method)
	}
	want := map[string]any{"pane_id": "pane-1", "source": "slk", "agent": "slk", "state": "idle"}
	for k, v := range want {
		if params[k] != v {
			t.Errorf("report_agent params[%q] = %v, want %v", k, params[k], v)
		}
	}
	if got := argvOf(t, params); !reflect.DeepEqual(got, argv) {
		t.Errorf("report_agent resume_argv = %q, want %q", got, argv)
	}
	method, params = decode(t, lines[1])
	if method != "pane.report_metadata" {
		t.Fatalf("second method = %q, want pane.report_metadata", method)
	}
	if params["display_agent"] != "slk" || params["title"] != "#general" {
		t.Errorf("report_metadata = %v, want display_agent slk, title #general", params)
	}
}

// TestReportResumeUnderAgentThread: once an agent thread holds the pane,
// a new resume command leaves its row alone (a session report, no state
// or metadata) and keeps the agent thread's agent id.
func TestReportResumeUnderAgentThread(t *testing.T) {
	sock := testSocketPath(t)
	_, rec := startServer(t, "unix", sock)
	r := newClient("unix", sock, "pane-1", "tab-1")
	argv := []string{"slk", "https://acme.slack.com/archives/C1/p1700000000000100?thread_ts=1700000000.000100&cid=C1"}

	r.Report("slack-claude", "@Claude", "#general thread", "idle", "")
	waitLines(t, rec, 2)
	r.ReportResume(argv, "#general")
	lines := waitLines(t, rec, 3)

	method, params := decode(t, lines[2])
	if method != "pane.report_agent_session" {
		t.Fatalf("method = %q, want pane.report_agent_session", method)
	}
	if params["agent"] != "slack-claude" || params["source"] != "slk" || params["pane_id"] != "pane-1" {
		t.Errorf("report_agent_session params = %v", params)
	}
	if got := argvOf(t, params); !reflect.DeepEqual(got, argv) {
		t.Errorf("resume_argv = %q, want %q", got, argv)
	}
	time.Sleep(50 * time.Millisecond)
	if n := len(rec.snapshot()); n != 3 {
		t.Errorf("sent %d requests, want 3: the row must not be re-reported", n)
	}
}

// TestReportsCarryResume: herdr drops a stored resume command when a
// report names a different agent, so every agent report after
// ReportResume carries it, and none does before.
func TestReportsCarryResume(t *testing.T) {
	sock := testSocketPath(t)
	_, rec := startServer(t, "unix", sock)
	r := newClient("unix", sock, "pane-1", "tab-1")
	argv := []string{"slk"}

	r.Report("slack-claude", "@Claude", "t", "idle", "")
	lines := waitLines(t, rec, 2)
	if _, params := decode(t, lines[0]); argvOf(t, params) != nil {
		t.Errorf("report before ReportResume carries resume_argv %v", params["resume_argv"])
	}

	r.ReportResume(argv, "#general")
	waitLines(t, rec, 3)
	r.Report("slack-codex", "@Codex", "t", "working", "")
	waitLines(t, rec, 5)
	r.ReportUnread("slack-codex", "@Codex", "t", "")
	lines = waitLines(t, rec, 8)
	for _, i := range []int{3, 5, 6} {
		method, params := decode(t, lines[i])
		if method != "pane.report_agent" {
			t.Fatalf("line %d method = %q, want pane.report_agent", i, method)
		}
		if got := argvOf(t, params); !reflect.DeepEqual(got, argv) {
			t.Errorf("line %d resume_argv = %q, want %q", i, got, argv)
		}
	}
}

// TestReleaseAfterResumeHold: a clean quit releases the "slk" hold, which
// clears the resume command, so a quit pane does not come back.
func TestReleaseAfterResumeHold(t *testing.T) {
	sock := testSocketPath(t)
	_, rec := startServer(t, "unix", sock)
	r := newClient("unix", sock, "pane-1", "tab-1")

	r.ReportResume([]string{"slk"}, "#general")
	waitLines(t, rec, 2)
	r.Close(time.Second)
	lines := waitLines(t, rec, 3)

	method, params := decode(t, lines[2])
	if method != "pane.release_agent" || params["agent"] != "slk" {
		t.Errorf("last request = %s %v, want pane.release_agent for agent slk", method, params)
	}
}
