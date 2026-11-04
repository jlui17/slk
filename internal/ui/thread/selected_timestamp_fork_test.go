package thread

import "testing"

// A selected continuation reply shows its long date and time after its
// first body row; unselected, only its margin time.
func TestThreadSelectedTimestamp_ContinuationShowsLong(t *testing.T) {
	m, _, _ := groupedThread(t)
	m.SelectByIndex(1) // b-two, which continues b-one
	rows := threadRows(m)
	if row := rows[threadRowWith(t, rows, "b-two")]; row != " 3:02 b-two  Tue Sep 29, 3:02 PM" {
		t.Errorf("selected continuation row = %q", row)
	}
	m.SelectByIndex(0)
	rows = threadRows(m)
	if row := rows[threadRowWith(t, rows, "b-two")]; row != " 3:02 b-two" {
		t.Errorf("unselected continuation row = %q", row)
	}
}
