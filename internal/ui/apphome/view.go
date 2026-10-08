// Package apphome is an app's Home tab: the blocks the app published,
// drawn with slk's Block Kit renderer, with a focus that moves between
// the elements a user can press.
package apphome

import (
	"cmp"
	"encoding/json"
	"fmt"

	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// View is a Home view an app published, as conversations.info or a
// view_updated event carries it.
type View struct {
	ID    string
	BotID string
	AppID string
	// TeamID is the team the app is installed in.
	TeamID string
	blocks []block
	// values is the view's input state: block_id -> action_id -> value.
	values map[string]map[string]json.RawMessage
}

type block struct {
	id     string
	typ    string
	parsed blockkit.Block
	// accessory is a section's interactive accessory, nil for none or
	// an image.
	accessory *element
	// elements are an actions block's elements.
	elements []element
}

// element is an interactive element of a block, read from its JSON.
type element struct {
	raw         json.RawMessage
	Type        string   `json:"type"`
	ActionID    string   `json:"action_id"`
	URL         string   `json:"url"`
	Style       string   `json:"style"`
	Text        *text    `json:"text"`
	Placeholder *text    `json:"placeholder"`
	Confirm     *Confirm `json:"confirm"`
	Options     []option `json:"options"`
	Initial     *option  `json:"initial_option"`
	optionsRaw  []json.RawMessage
}

type text struct {
	Text string `json:"text"`
}

type option struct {
	Text  text   `json:"text"`
	Value string `json:"value"`
}

// Confirm is a button's confirm dialog.
type Confirm struct {
	Title   text `json:"title"`
	Text    text `json:"text"`
	Confirm text `json:"confirm"`
	Deny    text `json:"deny"`
}

func (c Confirm) TitleText() string   { return c.Title.Text }
func (c Confirm) BodyText() string    { return c.Text.Text }
func (c Confirm) ConfirmText() string { return cmp.Or(c.Confirm.Text, "Yes") }
func (c Confirm) DenyText() string    { return cmp.Or(c.Deny.Text, "Cancel") }

func (t *text) String() string {
	if t == nil {
		return ""
	}
	return t.Text
}

// isLinkButton reports whether e is a button that opens a URL.
func (e *element) isLinkButton() bool { return e.Type == "button" && e.URL != "" }

// supported reports whether slk can act on e: a button, or a select of
// plain options (not option groups).
func (e *element) supported() bool {
	return e.Type == "button" || e.Type == "static_select" && len(e.Options) > 0
}

// ParseView reads a Home view object.
func ParseView(raw []byte) (*View, error) {
	var v struct {
		ID          string            `json:"id"`
		BotID       string            `json:"bot_id"`
		AppID       string            `json:"app_id"`
		TeamID      string            `json:"team_id"`
		InstalledIn string            `json:"app_installed_team_id"`
		Blocks      []json.RawMessage `json:"blocks"`
		State       struct {
			Values map[string]map[string]json.RawMessage `json:"values"`
		} `json:"state"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("parsing home view: %w", err)
	}
	out := &View{ID: v.ID, BotID: v.BotID, AppID: v.AppID, TeamID: cmp.Or(v.InstalledIn, v.TeamID), values: v.State.Values}
	if out.values == nil {
		out.values = map[string]map[string]json.RawMessage{}
	}
	for _, rb := range v.Blocks {
		out.blocks = append(out.blocks, parseBlock(rb))
	}
	return out, nil
}

func parseBlock(raw json.RawMessage) block {
	var head struct {
		Type      string            `json:"type"`
		BlockID   string            `json:"block_id"`
		Accessory json.RawMessage   `json:"accessory"`
		Elements  []json.RawMessage `json:"elements"`
	}
	_ = json.Unmarshal(raw, &head)
	b := block{id: head.BlockID, typ: head.Type, parsed: blockkit.ParseJSON(raw, head.Type)}
	switch head.Type {
	case "section":
		if e, ok := parseElement(head.Accessory); ok && e.Type != "image" {
			b.accessory = &e
		}
	case "actions":
		for _, re := range head.Elements {
			if e, ok := parseElement(re); ok {
				b.elements = append(b.elements, e)
			}
		}
	}
	return b
}

func parseElement(raw json.RawMessage) (element, bool) {
	if len(raw) == 0 {
		return element{}, false
	}
	var e element
	if err := json.Unmarshal(raw, &e); err != nil || e.Type == "" {
		return element{}, false
	}
	e.raw = raw
	var opts struct {
		Options []json.RawMessage `json:"options"`
	}
	_ = json.Unmarshal(raw, &opts)
	e.optionsRaw = opts.Options
	return e, true
}

// linkButtonURLs is the URLs the view's link buttons open, in block
// order.
func (v *View) linkButtonURLs() []string {
	var out []string
	for _, b := range v.blocks {
		if b.accessory != nil && b.accessory.isLinkButton() {
			out = append(out, b.accessory.URL)
		}
		for i := range b.elements {
			if b.elements[i].isLinkButton() {
				out = append(out, b.elements[i].URL)
			}
		}
	}
	return out
}

// newLinkURLs is the URLs of next's link buttons that v lacks, in
// block order, each once.
func (v *View) newLinkURLs(next *View) []string {
	seen := map[string]bool{}
	for _, u := range v.linkButtonURLs() {
		seen[u] = true
	}
	var out []string
	for _, u := range next.linkButtonURLs() {
		if !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}
