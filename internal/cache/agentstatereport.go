package cache

import (
	"fmt"
	"time"
)

// maxAgentStateReports caps the agent_state_reports log: it exists to
// study recent wrong statuses, not to keep history.
const maxAgentStateReports = 5000

// AgentStateReport is one agent state slk showed for a tracked thread and
// the rule that decided it. MessageTS joins to messages.ts; the message
// text itself is deliberately not stored. The Judge* fields,
// MessageTextHash and Error are set only when the model judge answered
// for this message.
type AgentStateReport struct {
	WorkspaceID     string
	ChannelID       string
	ThreadTS        string
	MessageTS       string
	FromAgent       bool
	State           string
	Source          string
	JudgeReply      string
	JudgeModel      string
	JudgePromptHash string
	MessageTextHash string
	Error           string
}

// InsertAgentStateReport appends r and trims the log to its newest
// maxAgentStateReports rows.
func (db *DB) InsertAgentStateReport(r AgentStateReport) error {
	_, err := db.conn.Exec(`
		INSERT INTO agent_state_reports (reported_at, workspace_id, channel_id, thread_ts,
			message_ts, from_agent, state, source,
			judge_reply, judge_model, judge_prompt_hash, message_text_hash, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		time.Now().Unix(), r.WorkspaceID, r.ChannelID, r.ThreadTS,
		r.MessageTS, r.FromAgent, r.State, r.Source,
		r.JudgeReply, r.JudgeModel, r.JudgePromptHash, r.MessageTextHash, r.Error,
	)
	if err != nil {
		return fmt.Errorf("inserting agent state report: %w", err)
	}
	_, err = db.conn.Exec(`
		DELETE FROM agent_state_reports
		WHERE id <= (SELECT MAX(id) FROM agent_state_reports) - ?`,
		maxAgentStateReports,
	)
	if err != nil {
		return fmt.Errorf("trimming agent state reports: %w", err)
	}
	return nil
}
