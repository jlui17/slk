package herdr

import (
	"fmt"
	"strings"
)

// paneAgent is the agent id slk holds its pane under while no agent
// thread is tracked. herdr stores a resume command only for a pane a
// reporter holds through pane.report_agent, so without this every pane
// that never opened an agent thread would come back as a bare shell.
const paneAgent = "slk"

// holdPaneAtStartup makes ReportResume hold the pane under paneAgent
// (an idle "slk" sidebar row) until an agent thread takes it over.
// Set to false to attach the command only once an agent thread
// holds the pane, leaving the sidebar as before but resuming only the
// panes that opened one.
const holdPaneAtStartup = true

type reportAgentSessionParams struct {
	PaneID     string   `json:"pane_id"`
	Source     string   `json:"source"`
	Agent      string   `json:"agent"`
	Seq        int64    `json:"seq"`
	ResumeArgv []string `json:"resume_argv"`
}

// ResumeArgv is the resume command herdr accepts for a launcher command
// and the link the relaunched slk opens: herdr rejects a command whose
// first word is a path, so a launcher given by path runs through env.
// An empty link leaves it off; nil without a command.
func ResumeArgv(command []string, link string) []string {
	if len(command) == 0 {
		return nil
	}
	var argv []string
	if strings.Contains(command[0], "/") {
		argv = append(argv, "env")
	}
	argv = append(argv, command...)
	if link != "" {
		argv = append(argv, link)
	}
	return argv
}

// ReportResume sets the command herdr runs in this pane after its server
// restarts, for slk to come back where it is now. Every later report
// carries it. While an agent thread holds the pane it rides a
// pane.report_agent_session, which leaves the row as it is; otherwise
// slk holds the pane as an idle "slk" row titled title (see
// holdPaneAtStartup). Release (Close) clears it, so a quit pane does not
// come back.
func (r *Client) ReportResume(argv []string, title string) {
	if r == nil || len(argv) == 0 {
		return
	}
	r.mu.Lock()
	r.resumeArgv = argv
	if r.agent == "" && holdPaneAtStartup {
		r.agent = paneAgent
	}
	agent := r.agent
	paneID := r.paneID
	r.mu.Unlock()
	switch agent {
	case "":
		return
	case paneAgent:
		seq := nextSeq()
		metaSeq := nextSeq()
		r.send(
			request{
				ID:     fmt.Sprintf("slk:%d", seq),
				Method: "pane.report_agent",
				Params: reportAgentParams{
					PaneID:     paneID,
					Source:     source,
					Agent:      paneAgent,
					State:      "idle",
					Seq:        seq,
					ResumeArgv: argv,
				},
			},
			request{
				ID:     fmt.Sprintf("slk:%d", metaSeq),
				Method: "pane.report_metadata",
				Params: reportMetadataParams{
					PaneID:       paneID,
					Source:       source,
					DisplayAgent: paneAgent,
					Title:        title,
					Seq:          metaSeq,
				},
			},
		)
	default:
		seq := nextSeq()
		r.send(request{
			ID:     fmt.Sprintf("slk:%d", seq),
			Method: "pane.report_agent_session",
			Params: reportAgentSessionParams{
				PaneID:     paneID,
				Source:     source,
				Agent:      agent,
				Seq:        seq,
				ResumeArgv: argv,
			},
		})
	}
}
