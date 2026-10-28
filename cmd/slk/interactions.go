package main

import (
	"context"

	"github.com/gammons/slk/internal/core/blocks"
	"github.com/gammons/slk/internal/ids"
	slackclient "github.com/gammons/slk/internal/slack"
)

// attachmentActioner is the part of *slackclient.Client a button press
// needs.
type attachmentActioner interface {
	AttachmentAction(ctx context.Context, req slackclient.AttachmentActionRequest) error
}

// pressAttachmentAction presses a legacy attachment button through client.
func pressAttachmentAction(ctx context.Context, client attachmentActioner, channelID ids.ChannelID,
	messageTS ids.MessageTS, attachmentID int, callbackID, serviceID string, action blocks.LegacyAction, ephemeral bool) error {
	return client.AttachmentAction(ctx, slackclient.AttachmentActionRequest{
		ChannelID:    string(channelID),
		MessageTS:    string(messageTS),
		AttachmentID: attachmentID,
		CallbackID:   callbackID,
		ServiceID:    serviceID,
		Ephemeral:    ephemeral,
		Action:       action,
	})
}
