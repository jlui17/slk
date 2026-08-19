// internal/ui/view_preview.go
//
// Image-preview overlay panel renderer for App.View (Phase 6i).
//
// When the full-screen image preview is active, the messages
// region and thread region are skipped (see renderMessagesRegion
// and the thread visibility gate). The preview panel takes their
// place in the panels list: a single overlay spanning the
// combined (msgWidth + msgBorder + threadWidth + threadBorder)
// when both messages and thread are visible, just the messages
// width when the thread is hidden, or the thread's own width
// when the thread is zoomed over the messages region.
//
// The rail and sidebar still render normally above the preview
// so the user can see context (which workspace is active, which
// channel the preview was opened from).
//
// Caller is responsible for the visibility gate; this helper
// assumes preview is active.
package ui

// renderPreviewPanel returns the exact-sized preview overlay
// panel string. Width is whatever the messages region currently
// spans (see the file header for the three cases).
func (a *App) renderPreviewPanel(frame panelLayoutFrame) string {
	overlayW := frame.MsgWidth + frame.MsgBorder
	switch {
	case frame.ThreadFullscreen:
		// A zoomed thread already spans the whole region, and
		// MsgWidth is the suppressed pane's would-be width, so the
		// sum would overshoot.
		overlayW = frame.ThreadWidth + frame.ThreadBorder
	case a.threadVisible && frame.ThreadWidth > 0:
		overlayW += frame.ThreadWidth + frame.ThreadBorder
	}
	overlayContent := a.preview.Overlay().View(overlayW, frame.ContentHeight, a.imgProtocol)
	return exactSize(overlayContent, overlayW, frame.ContentHeight)
}
