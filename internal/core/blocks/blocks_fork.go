package blocks

// TableBlock is the Slack `table` block. Slack draws the first row as the
// header.
type TableBlock struct {
	Rows [][]string
}

func (TableBlock) blockType() string { return "table" }

// ButtonFields are what a button carries besides its label, filled for
// Kind "button" only: the ids a block_actions payload sends back, and the
// URL of a link button, which Slack opens instead of calling the app.
// BlockID is the id of the block the button sits in.
type ButtonFields struct {
	ActionID string
	BlockID  string
	Value    string
	URL      string
}
