package messages

import (
	"reflect"
	"slices"
	"testing"

	"github.com/gammons/slk/internal/ui/messages/blockkit"
	"github.com/gammons/slk/internal/ui/messages/blockkit/blockkittest"
)

func TestMessageLinks(t *testing.T) {
	msg := MessageItem{
		Text: "see <https://a.example|a> and <https://b.example>",
		Blocks: []blockkit.Block{
			blockkit.SectionBlock{Text: "<https://c.example|c>", Fields: []string{"<https://a.example|a again>"}},
			blockkit.TableBlock{Rows: [][]string{
				{"Task", "Notes"},
				{"<https://d.example|row  1>\n*(Q4)*", "<https://e.example|1>, <https://f.example|2>"},
			}},
			blockkit.ContextBlock{Elements: []blockkit.ContextElement{{Text: "<https://g.example>"}}},
		},
	}
	want := []Link{
		{URL: "https://a.example", Label: "a"},
		{URL: "https://b.example"},
		{URL: "https://c.example", Label: "c"},
		{URL: "https://d.example", Label: "row  1"},
		{URL: "https://e.example", Label: "1", Context: "<https://d.example|row  1>\n*(Q4)*"},
		{URL: "https://f.example", Label: "2", Context: "<https://d.example|row  1>\n*(Q4)*"},
		{URL: "https://g.example"},
	}
	if got := MessageLinks(msg); !reflect.DeepEqual(got, want) {
		t.Errorf("MessageLinks:\n got %#v\nwant %#v", got, want)
	}

	var copied []string
	for _, c := range Copyables(msg) {
		copied = append(copied, c.Link.URL)
	}
	var offered []string
	for _, l := range want {
		offered = append(offered, l.URL)
	}
	if !reflect.DeepEqual(copied, offered) {
		t.Errorf("c offers %v, o offers %v", copied, offered)
	}
}

// A link in both the text and a table cell stays one row, at its place
// in the text, and learns its table row from the cell. The text spells
// & in a URL as &amp;, a rich_text cell as &.
func TestMessageLinks_TextLinkAlsoInTable(t *testing.T) {
	msg := MessageItem{
		Text: "see <https://a.example/p1?thread_ts=1.0&amp;cid=C1> and <https://b.example|the plan>",
		Blocks: []blockkit.Block{blockkit.TableBlock{Rows: [][]string{
			{"Task", "Notes"},
			{"row 1", "<https://a.example/p1?thread_ts=1.0&cid=C1|1>, <https://b.example|2>"},
		}}},
	}
	want := []Link{
		{URL: "https://a.example/p1?thread_ts=1.0&amp;cid=C1", Label: "1", Context: "row 1"},
		{URL: "https://b.example", Label: "the plan", Context: "row 1"},
	}
	if got := MessageLinks(msg); !reflect.DeepEqual(got, want) {
		t.Errorf("MessageLinks:\n got %#v\nwant %#v", got, want)
	}
	var copied []Link
	for _, c := range Copyables(msg) {
		copied = append(copied, c.Link)
	}
	if !reflect.DeepEqual(copied, want) {
		t.Errorf("Copyables links:\n got %#v\nwant %#v", copied, want)
	}
}

// The text of a bot post flattens its body: the list items' links are
// bare and a labeled link is literal markdown. The body's rich_text
// block gives each link of the text its list item and its link text,
// and adds no row; a second rich_text block adds its links, with theirs.
func TestMessageLinks_BodyListItems(t *testing.T) {
	msg := MessageItem{
		Text: "Waiting: 1. <@U1> - <https://x.example/m1> 2. <@U2> - <https://x.example/m2> See [the summary](<https://x.example/s?a=1&amp;b=2>).",
		Blocks: blockkittest.FromJSON(`[
			{"type":"rich_text","elements":[
				{"type":"rich_text_section","elements":[{"type":"text","text":"Waiting:\n"}]},
				{"type":"rich_text_list","style":"ordered","indent":0,"offset":0,"elements":[
					{"type":"rich_text_section","elements":[{"type":"user","user_id":"U1"},{"type":"text","text":" - "},{"type":"message_mention","message_ts":"1.0","channel_id":"C1","url":"https://x.example/m1"}]},
					{"type":"rich_text_section","elements":[{"type":"user","user_id":"U2"},{"type":"text","text":" - "},{"type":"message_mention","message_ts":"2.0","channel_id":"C1","url":"https://x.example/m2"}]}]},
				{"type":"rich_text_section","elements":[{"type":"text","text":"See "},{"type":"link","url":"https://x.example/s?a=1&b=2","text":"the summary"},{"type":"text","text":"."},
					{"type":"link","url":"https://x.example/not-in-text","text":"drawn only"}]}]},
			{"type":"rich_text","elements":[
				{"type":"rich_text_list","style":"bullet","indent":0,"offset":0,"elements":[
					{"type":"rich_text_section","elements":[{"type":"text","text":"follow-up: "},{"type":"link","url":"https://x.example/f","text":"notes"}]}]}]}]`),
	}
	want := []Link{
		{URL: "https://x.example/m1", Context: "1. <@U1>"},
		{URL: "https://x.example/m2", Context: "2. <@U2>"},
		{URL: "https://x.example/s?a=1&amp;b=2", Label: "the summary"},
		{URL: "https://x.example/not-in-text", Label: "drawn only"},
		{URL: "https://x.example/f", Label: "notes", Context: "follow-up"},
	}
	if got := MessageLinks(msg); !reflect.DeepEqual(got, want) {
		t.Errorf("MessageLinks:\n got %#v\nwant %#v", got, want)
	}
}

// A bot that posts blocks with a short text fallback: o offers every
// link the body draws, after the links of the text, and so the links c
// offers.
func TestMessageLinks_BodyLinksMissingFromTheText(t *testing.T) {
	msg := MessageItem{
		Text: "3 reviews waiting, see <https://x.example/board>",
		Blocks: blockkittest.FromJSON(`[{"type":"rich_text","elements":[
			{"type":"rich_text_list","style":"ordered","indent":0,"offset":0,"elements":[
				{"type":"rich_text_section","elements":[{"type":"user","user_id":"U1"},{"type":"text","text":" - "},{"type":"message_mention","message_ts":"1.0","channel_id":"C1","url":"https://x.example/m1?thread_ts=1.0&cid=C1"}]},
				{"type":"rich_text_section","elements":[{"type":"user","user_id":"U2"},{"type":"text","text":" - "},{"type":"message_mention","message_ts":"2.0","channel_id":"C1","url":"https://x.example/m2"}]}]},
			{"type":"rich_text_section","elements":[{"type":"text","text":"All of them: "},{"type":"link","url":"https://x.example/board","text":"the board"}]}]}]`),
	}
	want := []Link{
		{URL: "https://x.example/board", Label: "the board"},
		{URL: "https://x.example/m1?thread_ts=1.0&cid=C1", Context: "1. <@U1>"},
		{URL: "https://x.example/m2", Context: "2. <@U2>"},
	}
	got := MessageLinks(msg)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MessageLinks:\n got %#v\nwant %#v", got, want)
	}
	var offered, copied []string
	for _, l := range got {
		offered = append(offered, linkKey(l.URL))
	}
	for _, c := range Copyables(msg) {
		copied = append(copied, c.Text)
	}
	slices.Sort(offered)
	slices.Sort(copied)
	if !slices.Equal(offered, copied) {
		t.Errorf("o offers %v, c offers %v", offered, copied)
	}
}

// A URL in two list items is one row, named by the first.
func TestMessageLinks_URLInTwoListItems(t *testing.T) {
	msg := MessageItem{
		Text: "1. first - <https://x.example/a> 2. second - <https://x.example/a>",
		Blocks: blockkittest.FromJSON(`[{"type":"rich_text","elements":[
			{"type":"rich_text_list","style":"ordered","indent":0,"offset":0,"elements":[
				{"type":"rich_text_section","elements":[{"type":"text","text":"first - "},{"type":"link","url":"https://x.example/a"}]},
				{"type":"rich_text_section","elements":[{"type":"text","text":"second - "},{"type":"link","url":"https://x.example/a","text":"again"}]}]}]}]`),
	}
	want := []Link{{URL: "https://x.example/a", Label: "again", Context: "1. first"}}
	if got := MessageLinks(msg); !reflect.DeepEqual(got, want) {
		t.Errorf("MessageLinks:\n got %#v\nwant %#v", got, want)
	}
}
