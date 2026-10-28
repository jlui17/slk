package chrome

// Dialog is what every dialog and composer popup provides: whether it
// is open, and a way to close it.
type Dialog interface {
	IsVisible() bool
	Close()
}

// Boxed is a dialog drawn as a centered box. BoxSize is the box's outer
// size for a terminal of the given size; the App's click router centers
// it the way the dimmed overlay does, to tell a click inside the box
// from one outside it.
type Boxed interface {
	BoxSize(termWidth, termHeight int) (int, int)
}

// Clickable is a Boxed dialog with a selectable list. ClickRow moves the
// selection to the row at box-local localY (0 is the top border) and
// reports whether a row was there.
type Clickable interface {
	Boxed
	ClickRow(termWidth, termHeight, localY int) bool
}
