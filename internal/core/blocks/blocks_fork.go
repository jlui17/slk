package blocks

// TableBlock is the Slack `table` block. Slack draws the first row as the
// header.
type TableBlock struct {
	Rows [][]string
}

func (TableBlock) blockType() string { return "table" }

// ButtonFields are what a button carries besides its label, filled for
// Kind "button" only: the URL of a link button, which Slack opens instead
// of calling the app.
type ButtonFields struct {
	URL string
}
