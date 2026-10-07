package messages

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestLinkAt_FindsTheLinkUnderAColumn(t *testing.T) {
	const url = "https://example.com/a"
	styled := "\x1b[1m" + "▌" + "\x1b[0m"
	tests := []struct {
		name  string
		line  string
		cases map[int]string
	}{
		{
			// "▌ see docs now": the label "docs" sits on columns 6..9.
			name: "ascii",
			line: styled + " see " + osc8Hyperlink(url, linkStyle().Render("docs")) + " now",
			cases: map[int]string{
				-1: "", 0: "", 5: "", 6: url, 9: url, 10: "", 99: "",
			},
		},
		{
			// "▌日本 docs": 日 and 本 are two columns each, so the label
			// sits on columns 6..9, as in the ascii line.
			name: "wide characters before the link",
			line: styled + "日本 " + osc8Hyperlink(url, "docs"),
			cases: map[int]string{
				2: "", 5: "", 6: url, 9: url, 10: "",
			},
		},
		{
			// 👩‍💻 is one cluster two columns wide even with a style
			// escape inside it, so the label sits on columns 3..6.
			name: "cluster split by a style escape",
			line: "👩\x1b[1m\u200d💻\x1b[0m " + osc8Hyperlink(url, "docs"),
			cases: map[int]string{
				2: "", 3: url, 6: url, 7: "",
			},
		},
		{
			name: "emoji before the link, BEL terminators",
			line: "👍 \x1b]8;;" + url + "\adocs\x1b]8;;\a",
			cases: map[int]string{
				2: "", 3: url, 6: url, 7: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for col, want := range tt.cases {
				if got := LinkAt(tt.line, col); got != want {
					t.Errorf("col %d: LinkAt = %q, want %q (line %q)", col, got, want, ansi.Strip(tt.line))
				}
			}
		})
	}
}

// A long link that wraps in the pane resolves from a click on either of
// its rows.
func TestLinkURLAt_WrappedLinkResolvesOnBothRows(t *testing.T) {
	url := "https://example.com/" + strings.Repeat("segment/", 12)
	m := New([]MessageItem{{
		TS: "1.0", UserID: "U1", UserName: "alice", Timestamp: "1:00 PM",
		Text: "see <" + url + ">",
	}}, "general")
	plain := ansi.Strip(m.View(20, 40))
	rows := strings.Split(plain, "\n")
	var linkRows []int
	for y, row := range rows {
		if strings.Contains(row, "segment/") {
			linkRows = append(linkRows, y)
		}
	}
	if len(linkRows) < 2 {
		t.Fatalf("want the link wrapped over 2+ rows, got rows %v:\n%s", linkRows, plain)
	}
	for _, y := range linkRows {
		x := strings.Index(rows[y], "segment/")
		x = ansi.StringWidth(rows[y][:x])
		if got := m.LinkURLAt(y, x); got != url {
			t.Errorf("row %d col %d: LinkURLAt = %q, want %q", y, x, got, url)
		}
	}
}
