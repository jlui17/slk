// Package appshortcuts is the `.` overlay: the message shortcuts of the
// workspace's apps, and the modal form a shortcut opens, filled in the
// terminal.
package appshortcuts

import (
	"context"
	"encoding/json"
)

// Shortcut is one app's message shortcut.
type Shortcut struct {
	AppID    string
	AppName  string
	ActionID string
	Name     string
}

// SubmitResult is Slack's answer to a submit that reached it.
type SubmitResult struct {
	// Error is Slack's error code when it refused the submit.
	Error string
	// FieldErrors is the app's message for each block_id it rejected.
	FieldErrors map[string]string
}

// Service is the overlay's reach into Slack, per workspace.
type Service interface {
	List(ctx context.Context, teamID string) ([]Shortcut, error)
	// Run asks the app to run s on the message; the app's modal, if
	// any, comes back later on the websocket, echoing clientToken.
	Run(ctx context.Context, teamID string, s Shortcut, channelID, messageTS, clientToken string) error
	Close(ctx context.Context, teamID, viewID, rootViewID, clientToken string) error
	Submit(ctx context.Context, teamID, viewID, clientToken, state string) (SubmitResult, error)
}

// Text is a Block Kit text object.
type Text struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Emoji bool   `json:"emoji,omitempty"`
}

// Option is one choice of a radio_buttons or static_select element.
type Option struct {
	Text  Text   `json:"text"`
	Value string `json:"value"`
}

// Element is an input block's element. Only the fields of the element
// types the form draws are read.
type Element struct {
	Type          string          `json:"type"`
	ActionID      string          `json:"action_id"`
	Multiline     bool            `json:"multiline"`
	Placeholder   *Text           `json:"placeholder"`
	InitialValue  string          `json:"initial_value"`
	MinLength     int             `json:"min_length"`
	MaxLength     int             `json:"max_length"`
	Options       []Option        `json:"options"`
	InitialOption *Option         `json:"initial_option"`
	OptionGroups  json.RawMessage `json:"option_groups"`
}

// Block is one block of a modal: an input, or text the form only draws.
type Block struct {
	Type    string `json:"type"`
	BlockID string `json:"block_id"`
	// input
	Label    *Text    `json:"label"`
	Optional bool     `json:"optional"`
	Element  *Element `json:"element"`
	// section, header
	Text      *Text  `json:"text"`
	Fields    []Text `json:"fields"`
	Accessory *struct {
		Type string `json:"type"`
	} `json:"accessory"`
	// context: text objects and images; Text is a string in a text
	// object and an object in other blocks' elements.
	Elements []struct {
		Type string          `json:"type"`
		Text json.RawMessage `json:"text"`
	} `json:"elements"`
}

// View is a modal as Slack sends it in view_opened and view_updated.
type View struct {
	ID         string
	RootViewID string
	Title      string
	// SubmitText is the submit button's label; empty for a modal
	// without one.
	SubmitText string
	CloseText  string
	Blocks     []Block
}

// ParseView reads the view object of a view_opened or view_updated
// event.
func ParseView(raw []byte) (View, error) {
	var w struct {
		ID         string  `json:"id"`
		RootViewID string  `json:"root_view_id"`
		Title      *Text   `json:"title"`
		Submit     *Text   `json:"submit"`
		Close      *Text   `json:"close"`
		Blocks     []Block `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &w); err != nil {
		return View{}, err
	}
	v := View{ID: w.ID, RootViewID: w.RootViewID, Blocks: w.Blocks, CloseText: "Cancel"}
	if w.Title != nil {
		v.Title = w.Title.Text
	}
	if w.Submit != nil {
		v.SubmitText = w.Submit.Text
	}
	if w.Close != nil && w.Close.Text != "" {
		v.CloseText = w.Close.Text
	}
	return v, nil
}

// supported reports whether the form can draw every block of v and
// send what its inputs hold. Anything it can't draw would leave the app
// a form it can't trust, so such a view is filled in Slack instead.
func (v View) supported() bool {
	for _, b := range v.Blocks {
		switch b.Type {
		case "header", "divider", "context":
		case "section":
			if b.Accessory != nil && b.Accessory.Type != "image" {
				return false
			}
		case "input":
			if b.Element == nil {
				return false
			}
			switch b.Element.Type {
			case "plain_text_input", "radio_buttons":
			case "static_select":
				if len(b.Element.OptionGroups) > 0 && string(b.Element.OptionGroups) != "null" {
					return false
				}
			default:
				return false
			}
		default:
			return false
		}
	}
	return true
}
