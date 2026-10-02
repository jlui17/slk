package blockkit

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/slack-go/slack"
)

func richTextFromJSON(t *testing.T, elementsJSON string) RichTextBlock {
	t.Helper()
	var blocks slack.Blocks
	if err := json.Unmarshal([]byte(`[{"type":"rich_text","elements":`+elementsJSON+`}]`), &blocks); err != nil {
		t.Fatal(err)
	}
	return Parse(blocks)[0].(RichTextBlock)
}

func TestRichTextLinks(t *testing.T) {
	const mention = `{"type":"message_mention","message_ts":"1.0","channel_id":"C1","url":"https://x.example/m1"}`
	link := func(url, label string) string {
		return `{"type":"link","url":"` + url + `","text":"` + label + `"}`
	}
	txt := func(s string) string { b, _ := json.Marshal(s); return `{"type":"text","text":` + string(b) + `}` }
	item := func(elements string) string { return `{"type":"rich_text_section","elements":[` + elements + `]}` }
	list := func(style string, indent, offset int, items string) string {
		b, _ := json.Marshal(map[string]any{"type": "rich_text_list", "style": style, "indent": indent, "offset": offset})
		return string(b[:len(b)-1]) + `,"elements":[` + items + `]}`
	}

	rt := richTextFromJSON(t, `[`+
		item(txt("intro ")+","+link("https://x.example/p", "the plan")+","+txt(" and ")+","+link("https://x.example/bare", "https://x.example/bare"))+","+
		list("ordered", 0, 4,
			item(`{"type":"user","user_id":"U1"},`+txt(" - ")+","+mention)+","+
				item(link("https://x.example/only", "only a link"))+","+
				item(txt("compare ")+","+link("https://x.example/a", "before")+","+txt(" with ")+","+link("https://x.example/b", "after")+","+txt(": ")+`,{"type":"emoji","name":"tada"}`))+","+
		list("bullet", 1, 0,
			item(txt("nested, ")+","+link("https://x.example/n", "notes"))+","+
				item(link("https://x.example/bullet-only", "a bullet that is only a link")))+
		`]`)

	want := []RichTextLink{
		{URL: "https://x.example/p", Text: "the plan"},
		{URL: "https://x.example/bare"},
		{URL: "https://x.example/m1", ListItem: "5. <@U1>"},
		{URL: "https://x.example/only", Text: "only a link", ListItem: "6."},
		{URL: "https://x.example/a", Text: "before", ListItem: "7. compare  with : :tada:"},
		{URL: "https://x.example/b", Text: "after", ListItem: "7. compare  with : :tada:"},
		{URL: "https://x.example/n", Text: "notes", ListItem: "nested"},
		{URL: "https://x.example/bullet-only", Text: "a bullet that is only a link"},
	}
	if got := RichTextLinks(rt); !reflect.DeepEqual(got, want) {
		t.Errorf("RichTextLinks:\n got %#v\nwant %#v", got, want)
	}
}

// Separators go only from an edge of the item where a link was removed,
// with a bracket the link leaves unmatched there; an edge that is the
// item's own text stays as typed. A bracket pair the link leaves empty
// goes too.
func TestRichTextLinks_ListItemEdges(t *testing.T) {
	const link = `{"type":"link","url":"https://x.example/a","text":"a"}`
	txt := func(s string) string { b, _ := json.Marshal(s); return `{"type":"text","text":` + string(b) + `}` }
	for _, tt := range []struct{ elements, want string }{
		{txt("-5% latency (") + "," + link, "1. -5% latency"},
		{txt("see (") + "," + link + "," + txt(")"), "1. see"},
		{txt("see (") + "," + link + "," + txt(") for more"), "1. see  for more"},
		{txt(":tada: literal ") + "," + link, "1. :tada: literal"},
		{`{"type":"user","user_id":"U1"},` + txt(" - ") + "," + link, "1. <@U1>"},
		{link + "," + txt(" - the plan"), "1. the plan"},
		{link + "," + txt("] - the plan:"), "1. the plan:"},
		{`{"type":"user","user_id":"U1"},` + txt(" ") + `,{"type":"emoji","name":"tada"},` + txt(" ") + "," + link, "1. <@U1> :tada:"},
		{txt("call foo() - ") + "," + link, "1. call foo()"},
	} {
		rt := richTextFromJSON(t, `[{"type":"rich_text_list","style":"ordered","indent":0,"offset":0,"elements":[{"type":"rich_text_section","elements":[`+tt.elements+`]}]}]`)
		if got := RichTextLinks(rt)[0].ListItem; got != tt.want {
			t.Errorf("item %s:\n got %q\nwant %q", tt.elements, got, tt.want)
		}
	}
}

// A link in a quote is walked like one in a paragraph: no list item.
func TestRichTextLinks_Quote(t *testing.T) {
	rt := richTextFromJSON(t, `[{"type":"rich_text_quote","elements":[
		{"type":"text","text":"as said in "},{"type":"link","url":"https://x.example/q","text":"the thread"}]}]`)
	want := []RichTextLink{{URL: "https://x.example/q", Text: "the thread"}}
	if got := RichTextLinks(rt); !reflect.DeepEqual(got, want) {
		t.Errorf("RichTextLinks:\n got %#v\nwant %#v", got, want)
	}
}
