package main

import (
	"fmt"

	slackclient "github.com/gammons/slk/internal/slack"
)

// Client is the Slack client of the connected workspace teamID, for a
// call an overlay makes in the workspace it was opened in.
func (r *workspaceRouter) Client(teamID string) (*slackclient.Client, error) {
	wctx := r.ByID(teamID)
	if wctx == nil || wctx.Client == nil {
		return nil, fmt.Errorf("workspace %s is not connected", teamID)
	}
	return wctx.Client, nil
}
