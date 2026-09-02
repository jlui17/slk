package blockkit

// TableBlock is the Slack `table` block. Slack draws the first row as the
// header.
type TableBlock struct {
	Rows [][]string
}

func (TableBlock) blockType() string { return "table" }
