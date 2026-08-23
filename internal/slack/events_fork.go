package slackclient

// wsAssistantStatusEvent represents an ai_assistant_status event. The
// payload also carries a status_type field (banner/typing) whose values
// vary; it is deliberately not parsed — only whether status is empty
// matters.
type wsAssistantStatusEvent struct {
	Type      string `json:"type"`
	Channel   string `json:"channel_id"`
	ThreadTS  string `json:"thread_ts"`
	BotUserID string `json:"bot_user_id"`
	Status    string `json:"status"`
}
