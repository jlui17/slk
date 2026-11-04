package messages

import (
	"strings"
	"testing"
	"time"
)

// A selected continuation shows its long date and time after its first
// body row, as Slack shows any message's full time on hover.
func TestSelectedTimestamp_ContinuationShowsLong(t *testing.T) {
	SetNowFunc(func() time.Time { return time.Date(2026, 9, 29, 16, 0, 0, 0, time.Local) })
	t.Cleanup(func() { SetNowFunc(nil) })
	m, _ := groupedPane(t)
	m.SelectByIndex(1) // a-two, which continues a-one
	rows := paneRows(m)
	if row := rows[rowWith(t, rows, "a-two")]; row != " 3:01 a-two  Tue Sep 29, 3:01 PM" {
		t.Errorf("selected continuation row = %q", row)
	}
	m.SelectByIndex(0)
	rows = paneRows(m)
	if row := rows[rowWith(t, rows, "a-two")]; row != " 3:01 a-two" {
		t.Errorf("unselected continuation row = %q", row)
	}
}

// Where the long form does not fit after the text, the row stays as it
// is.
func TestSelectedTimestamp_ContinuationTooLongStaysShort(t *testing.T) {
	m, msgs := groupedPane(t)
	long := msgs[1]
	long.Text = strings.Repeat("x", 60)
	m.UpdateMessageInPlace(long)
	m.SelectByIndex(1)
	rows := strings.Split(m.View(60, 80), "\n")
	for _, r := range rows {
		if strings.Contains(r, "Sep 29") {
			t.Errorf("a long time that does not fit was drawn: %q", r)
		}
	}
}
