package herdr

import (
	"encoding/json"
	"errors"
	"fmt"
)

type paneSplitParams struct {
	Direction    string `json:"direction"`
	TargetPaneID string `json:"target_pane_id"`
	Focus        bool   `json:"focus"`
}

type paneCloseParams struct {
	PaneID string `json:"pane_id"`
}

// OpenPane splits splitPaneID, slk's own pane when empty, putting a new
// pane to its right in the same tab, runs `<openCommand> '<rawURL>'` in
// the new pane's shell, and returns the new pane's id. focus moves
// herdr's focus to the new pane; false leaves it where it is. The new
// pane shares slk's tab, so it claims no tab label. Blocking, like
// OpenTab; call it off the UI goroutine.
func (r *Reporter) OpenPane(splitPaneID, openCommand, rawURL string, focus bool) (string, error) {
	if splitPaneID == "" {
		splitPaneID = r.identity().PaneID
	}
	created, err := r.roundTrip("pane.split", paneSplitParams{
		Direction:    "right",
		TargetPaneID: splitPaneID,
		Focus:        focus,
	})
	if err != nil {
		return "", fmt.Errorf("pane.split: %w", err)
	}
	var parsed struct {
		Pane Pane `json:"pane"`
	}
	if err := json.Unmarshal(created, &parsed); err != nil {
		return "", fmt.Errorf("pane.split: %w", err)
	}
	paneID := parsed.Pane.PaneID
	if paneID == "" {
		return "", errors.New("pane.split: no pane in response")
	}
	if err := r.sendOpenCommand(paneID, openCommand, rawURL, func() {
		_, _ = r.roundTrip("pane.close", paneCloseParams{PaneID: paneID})
	}); err != nil {
		return "", err
	}
	return paneID, nil
}
