package main

import (
	"context"
	"testing"

	"github.com/gammons/slk/internal/core/blocks"
	slackclient "github.com/gammons/slk/internal/slack"
)

type fakeActioner struct {
	got slackclient.AttachmentActionRequest
}

func (f *fakeActioner) AttachmentAction(_ context.Context, req slackclient.AttachmentActionRequest) error {
	f.got = req
	return nil
}

func TestPressAttachmentActionMapsFields(t *testing.T) {
	f := &fakeActioner{}
	act := blocks.LegacyAction{ID: "2", Name: "ignore", Text: "Dismiss", Type: "button", Value: "ignore"}
	if err := pressAttachmentAction(context.Background(), f, "C0EXAMPLE01", "1.000100", 1, "cb", "B01", act, true); err != nil {
		t.Fatal(err)
	}
	want := slackclient.AttachmentActionRequest{
		ChannelID: "C0EXAMPLE01", MessageTS: "1.000100", AttachmentID: 1,
		CallbackID: "cb", ServiceID: "B01", Ephemeral: true, Action: act,
	}
	if f.got != want {
		t.Errorf("request = %+v, want %+v", f.got, want)
	}
}
