package messages

import (
	"reflect"
	"testing"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

func codeCopyable(language, code string) Copyable {
	return Copyable{Kind: CopyableCodeBlock, Text: code, CodeBlock: CodeBlock{Language: language, Code: code}}
}

func linkCopyable(url, label string) Copyable {
	return Copyable{Kind: CopyableLink, Text: url, Link: Link{URL: url, Label: label}}
}

func TestCopyables(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []Copyable
	}{
		{"none", "plain text with `inline` code", nil},
		{"one block", "```\nrun()\n```", []Copyable{codeCopyable("", "run()")}},
		{"one link", "see <https://a.example|the docs>", []Copyable{linkCopyable("https://a.example", "the docs")}},
		{
			"blocks and links keep message order",
			"<https://a.example> then\n```<go>\nfirst()\n```\nthen <https://b.example|b> and\n```second()```\n<https://c.example>",
			[]Copyable{
				linkCopyable("https://a.example", ""),
				codeCopyable("go", "first()"),
				linkCopyable("https://b.example", "b"),
				codeCopyable("", "second()"),
				linkCopyable("https://c.example", ""),
			},
		},
		{
			"a link inside a fence is not its own item",
			"```\ncurl <https://in.example/fence>\n```",
			[]Copyable{codeCopyable("", "curl <https://in.example/fence>")},
		},
		{
			"a repeated link is one item, the first",
			"<https://a.example|first> ```x``` <https://a.example|again>",
			[]Copyable{linkCopyable("https://a.example", "first"), codeCopyable("", "x")},
		},
		{
			"a link repeated inside a fence is still an item outside it",
			"```curl <https://a.example>```\n<https://a.example>",
			[]Copyable{codeCopyable("", "curl <https://a.example>"), linkCopyable("https://a.example", "")},
		},
		{
			"a link's clipboard text decodes &amp;, its Link keeps the wire URL",
			"<https://a.example/?x=1&amp;y=2>",
			[]Copyable{{Kind: CopyableLink, Text: "https://a.example/?x=1&y=2", Link: Link{URL: "https://a.example/?x=1&amp;y=2"}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Copyables(MessageItem{Text: tc.text}); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Copyables(%q) =\n%#v, want\n%#v", tc.text, got, tc.want)
			}
		})
	}
}

// ExtractLinks alone does match inside a fence: Copyables is what keeps
// those out.
func TestCopyables_ExtractLinksSeesInsideFences(t *testing.T) {
	if got := ExtractLinks("```\ncurl <https://in.example/fence>\n```"); len(got) != 1 {
		t.Errorf("ExtractLinks = %#v, want the fenced link", got)
	}
}

func TestCopyables_ReadsTheRichTextBody(t *testing.T) {
	msg := MessageItem{Text: "fallback without a fence or a link", Blocks: []blockkit.Block{
		blockkit.RichTextBlock{Elements: []slack.RichTextElement{
			&slack.RichTextPreformatted{
				Type:     slack.RTEPreformatted,
				Elements: []slack.RichTextSectionElement{&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "a < b"}},
			},
			&slack.RichTextSection{
				Type:     slack.RTESection,
				Elements: []slack.RichTextSectionElement{&slack.RichTextSectionLinkElement{Type: slack.RTSELink, URL: "https://a.example", Text: "docs"}},
			},
		}},
	}}
	want := []Copyable{codeCopyable("", "a < b"), linkCopyable("https://a.example", "docs")}
	if got := Copyables(msg); !reflect.DeepEqual(got, want) {
		t.Errorf("Copyables = %#v, want %#v (source %q)", got, want, MessageTextSource(msg))
	}
}
