package blockkit

import (
	"strings"
	"testing"
)

// Slack sends an ordered list with bullets under its items as one
// rich_text_list per run of same-indent items, and none of the later
// ordered lists carries an offset. Shape as seen 2026-10 in a bot reply;
// the text is made up.
func TestRichTextToMrkdwn_OrderedListWithNestedBullets(t *testing.T) {
	rt := Parse(loadFixture(t, "ordered_list_with_nested_bullets.json").Blocks)[0].(RichTextBlock)
	got := RichTextToMrkdwn(rt)
	_, buildOrder, _ := strings.Cut(got, "Build order, with a check per step:\n")
	buildOrder, _, _ = strings.Cut(buildOrder, "\n\n")
	want := strings.Join([]string{
		"1. Beds and soil, as one trip to the yard plus a second trip for compost.",
		"   ◦ Check 1: the beds stand level.",
		"   ◦ Check 2: the soil fills every bed to the top edge, and nothing spills onto the gravel path between them.",
		"   ◦ Check 3: water drains in a minute.",
		"2. Seedlings that wilted go back on the windowsill, so they recover instead of `compost_bin` taking them.",
		"   ◦ Check 1: the seedlings stand up.",
		"   ◦ Check 2: the leaves turn green again.",
		"3. A fence, so `rabbits` and `deer` stay out. Only if you want it.",
	}, "\n")
	if buildOrder != want {
		t.Errorf("build order list:\n got:\n%s\nwant:\n%s", buildOrder, want)
	}
	if !strings.Contains(got, "Picks:\n• Three beds, not four.\n• Seeds come") {
		t.Errorf("the top-level bullet list changed:\n%s", got)
	}
}

// Slack's markers by indent: bullets cycle •, ◦, ▪ and ordered lists
// cycle 1., a., i.; each level's marker starts where its parent item's
// text starts.
func TestRichTextToMrkdwn_NestedMarkersCycleByIndent(t *testing.T) {
	item := func(text string) string {
		return `{"type":"rich_text_section","elements":[{"type":"text","text":"` + text + `"}]}`
	}
	list := func(style string, indent string, items ...string) string {
		return `{"type":"rich_text_list","style":"` + style + `","indent":` + indent + `,"elements":[` + strings.Join(items, ",") + `]}`
	}
	cases := map[string]struct {
		elements string
		want     []string
	}{
		"bullets": {
			"[" + strings.Join([]string{
				list("bullet", "0", item("zero")),
				list("bullet", "1", item("one")),
				list("bullet", "2", item("two")),
				list("bullet", "3", item("three")),
			}, ",") + "]",
			[]string{"• zero", "  ◦ one", "    ▪ two", "      • three"},
		},
		"ordered": {
			"[" + strings.Join([]string{
				list("ordered", "0", item("zero")),
				list("ordered", "1", item("one"), item("one again")),
				list("ordered", "2", item("two"), item("two"), item("two"), item("two")),
				list("ordered", "3", item("three")),
				list("ordered", "1", item("one more")),
				list("ordered", "0", item("zero again")),
				list("ordered", "1", item("under the second")),
			}, ",") + "]",
			[]string{
				"1. zero",
				"   a. one",
				"   b. one again",
				"      i. two",
				"      ii. two",
				"      iii. two",
				"      iv. two",
				"          1. three",
				"   c. one more",
				"2. zero again",
				"   a. under the second",
			},
		},
		"a paragraph restarts the numbering": {
			"[" + strings.Join([]string{
				list("ordered", "0", item("first")),
				item("between"),
				list("ordered", "0", item("first again")),
			}, ",") + "]",
			[]string{"1. first", "between", "1. first again"},
		},
		"a nested list with no parent": {
			"[" + list("bullet", "2", item("deep")) + "]",
			[]string{"    ▪ deep"},
		},
	}
	for name, tc := range cases {
		got := RichTextToMrkdwn(richTextElementsFromJSON(t, tc.elements))
		if want := strings.Join(tc.want, "\n"); got != want {
			t.Errorf("%s:\n got:\n%s\nwant:\n%s", name, got, want)
		}
	}
}
