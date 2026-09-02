package blockkit

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A legacy attachment has no host-rendered body, so a rich_text block
// nested in it is drawn by Render itself.
func TestRenderLegacyDrawsNestedRichText(t *testing.T) {
	r := RenderLegacy([]LegacyAttachment{{Blocks: []Block{richTextBlockOf("unfurl body")}}}, makeCtx(), 80)
	if !strings.Contains(ansi.Strip(strings.Join(r.Lines, "\n")), "unfurl body") {
		t.Errorf("nested rich_text missing from legacy attachment render: %q", r.Lines)
	}
}
