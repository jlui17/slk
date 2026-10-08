package slackclient

import (
	"context"
	"fmt"
	"io"

	"github.com/slack-go/slack"
)

// UploadFileUnshared uploads one file through the first two steps of
// Slack's V2 external-upload flow (getUploadURLExternal, then the PUT)
// and returns it without sharing it anywhere. Pass the summaries of
// every file meant for one message to ShareFiles.
func (c *Client) UploadFileUnshared(ctx context.Context, filename string, r io.Reader, size int64) (slack.FileSummary, error) {
	u, err := c.api.GetUploadURLExternalContext(ctx, slack.GetUploadURLExternalParameters{
		FileName: filename,
		FileSize: int(size),
	})
	if err != nil {
		return slack.FileSummary{}, fmt.Errorf("getting upload URL: %w", err)
	}
	if err := c.api.UploadToURL(ctx, slack.UploadToURLParameters{
		UploadURL: u.UploadURL,
		Reader:    r,
		Filename:  filename,
	}); err != nil {
		return slack.FileSummary{}, fmt.Errorf("uploading: %w", err)
	}
	return slack.FileSummary{ID: u.FileID}, nil
}

// ShareFiles posts files uploaded by UploadFileUnshared to a channel
// (and thread, when threadTS is non-empty) as one message, with caption
// as its text, in a single completeUploadExternal call: Slack groups the
// files completed in one call into one message, as the Slack app sends
// them.
func (c *Client) ShareFiles(ctx context.Context, channelID, threadTS string, files []slack.FileSummary, caption string) error {
	resp, err := c.api.CompleteUploadExternalContext(ctx, slack.CompleteUploadExternalParameters{
		Files:           files,
		Channel:         channelID,
		InitialComment:  caption,
		ThreadTimestamp: threadTS,
	})
	if err != nil {
		return fmt.Errorf("sharing files: %w", err)
	}
	if len(resp.Files) != len(files) {
		return fmt.Errorf("sharing files: Slack shared %d of %d", len(resp.Files), len(files))
	}
	return nil
}
