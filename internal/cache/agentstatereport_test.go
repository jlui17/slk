package cache

import "testing"

func TestInsertAgentStateReportRoundtrip(t *testing.T) {
	db := newPaneStateTestDB(t)

	want := AgentStateReport{
		WorkspaceID: "T1", ChannelID: "C1", ThreadTS: "100.0", MessageTS: "101.0",
		FromAgent: true, State: "blocked", Source: "judge",
		JudgeReply: "u", JudgeModel: "claude-haiku-4-5", JudgePromptHash: "abc123",
		MessageTextHash: "def456", Error: "",
	}
	if err := db.InsertAgentStateReport(want); err != nil {
		t.Fatal(err)
	}

	var got AgentStateReport
	var reportedAt int64
	err := db.conn.QueryRow(`
		SELECT reported_at, workspace_id, channel_id, thread_ts, message_ts, from_agent, state, source,
			judge_reply, judge_model, judge_prompt_hash, message_text_hash, error
		FROM agent_state_reports`).Scan(&reportedAt, &got.WorkspaceID, &got.ChannelID, &got.ThreadTS,
		&got.MessageTS, &got.FromAgent, &got.State, &got.Source,
		&got.JudgeReply, &got.JudgeModel, &got.JudgePromptHash, &got.MessageTextHash, &got.Error)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if reportedAt == 0 {
		t.Error("reported_at was not stamped")
	}
}

func TestInsertAgentStateReportTrimsToCap(t *testing.T) {
	db := newPaneStateTestDB(t)
	insert := func() {
		t.Helper()
		if err := db.InsertAgentStateReport(AgentStateReport{State: "idle", Source: "no_message"}); err != nil {
			t.Fatal(err)
		}
	}

	insert() // id 1
	// Jump the AUTOINCREMENT counter instead of inserting the cap's worth
	// of rows: the next ids are cap and cap+1.
	if _, err := db.conn.Exec(`UPDATE sqlite_sequence SET seq = ? WHERE name = 'agent_state_reports'`, maxAgentStateReports-1); err != nil {
		t.Fatal(err)
	}
	insert() // id cap: id 1 is exactly cap-1 behind, still kept
	if ids := agentStateReportIDs(t, db); len(ids) != 2 || ids[0] != 1 {
		t.Fatalf("ids before crossing the cap = %v, want [1 %d]", ids, maxAgentStateReports)
	}
	insert() // id cap+1: id 1 falls off
	if ids := agentStateReportIDs(t, db); len(ids) != 2 || ids[0] != maxAgentStateReports || ids[1] != maxAgentStateReports+1 {
		t.Errorf("ids after crossing the cap = %v, want [%d %d]", ids, maxAgentStateReports, maxAgentStateReports+1)
	}
}

func agentStateReportIDs(t *testing.T, db *DB) []int {
	t.Helper()
	rows, err := db.conn.Query(`SELECT id FROM agent_state_reports ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}
