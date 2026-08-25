package slacktest

// The synthetic workspace the default corpus describes: one channel,
// one DM with one other user. Entirely invented — never captured from
// real traffic.
const (
	TeamID    = "T0TEST"
	SelfID    = "U0SELF"
	PeerID    = "U0PEER"
	ChannelID = "C0GEN"
	IMID      = "D0PEER"

	teamDomain = "slacktest"
)

func defaultResponses() map[string]string {
	return map[string]string{
		"/api/auth.test":                      authTestBody,
		"/api/client.userBoot":                userBootBody,
		"/api/client.counts":                  countsBody,
		"/api/conversations.view":             conversationsViewBody,
		"/api/users.conversations":            usersConversationsBody,
		"/cache/" + TeamID + "/channels/info": channelsInfoBody,
		"/cache/" + TeamID + "/users/info":    usersInfoBody,
	}
}

const authTestBody = `{"ok":true,"url":"https://` + teamDomain + `.slack.com/","team":"Slacktest","user":"self","team_id":"` + TeamID + `","user_id":"` + SelfID + `"}`

const userBootBody = `{
  "ok": true,
  "self": {"id":"` + SelfID + `","name":"self","team_id":"` + TeamID + `","real_name":"Self Tester","updated":1700000000001,"profile":{"real_name":"Self Tester","display_name":"self"}},
  "team": {"id":"` + TeamID + `","name":"Slacktest","domain":"` + teamDomain + `","url":"https://` + teamDomain + `.slack.com/"},
  "channels": [{"id":"` + ChannelID + `","name":"general","name_normalized":"general","is_channel":true,"is_general":true,"created":1690000000,"updated":1700000000002,"context_team_id":"` + TeamID + `","topic":{"value":"synthetic corpus"},"purpose":{"value":"harness testing"}}],
  "ims": [{"id":"` + IMID + `","user":"` + PeerID + `","is_im":true,"is_open":true,"created":1690000001,"updated":1700000000003,"context_team_id":"` + TeamID + `"}],
  "is_open": ["` + ChannelID + `","` + IMID + `"],
  "starred": [],
  "subteams": {"self": []},
  "dnd": {"dnd_enabled":false,"next_dnd_start_ts":0,"next_dnd_end_ts":0,"snooze_enabled":false},
  "prefs": {"all_notifications_prefs":"{\"channels\":{\"` + ChannelID + `\":{\"muted\":false}}}"},
  "channels_priority": {"` + ChannelID + `": 0.5},
  "emoji_cache_ts": "17000000000000000"
}`

const countsBody = `{"ok":true,"channels":[{"id":"` + ChannelID + `","has_unreads":true,"mention_count":2,"last_read":"1700000001.000100"}],"mpims":[],"ims":[{"id":"` + IMID + `","has_unreads":false,"last_read":"1700000002.000200"}],"threads":{"has_unreads":false,"unread_count":0,"mention_count":0}}`

const conversationsViewBody = `{"ok":true,"channel":{"id":"` + ChannelID + `","name":"general","is_member":true,"last_read":"1700000001.000100"},"history":{"messages":[{"type":"message","ts":"1700000003.000100","user":"` + PeerID + `","text":"hello from the corpus"}],"has_more":false},"users":[{"id":"` + PeerID + `","name":"peer","team_id":"` + TeamID + `","updated":1700000000004,"profile":{"real_name":"Peer Person","display_name":"peer"}}],"emojis":{}}`

const usersConversationsBody = `{"ok":true,"channels":[{"id":"` + ChannelID + `","name":"general","is_channel":true,"is_member":true},{"id":"` + IMID + `","user":"` + PeerID + `","is_im":true}],"response_metadata":{"next_cursor":""}}`

const channelsInfoBody = `{"ok":true,"results":[{"id":"` + ChannelID + `","name":"general","updated":1700000000002,"is_channel":true,"context_team_id":"` + TeamID + `","topic":{"value":"synthetic corpus"}}],"member_channels":["` + ChannelID + `"]}`

const usersInfoBody = `{"ok":true,"results":[{"id":"` + PeerID + `","name":"peer","updated":1700000000004,"team_id":"` + TeamID + `","profile":{"display_name":"peer","real_name":"Peer Person","image_original":"https://` + teamDomain + `.slack.com/avatars/peer.png","is_custom_image":true}}]}`
