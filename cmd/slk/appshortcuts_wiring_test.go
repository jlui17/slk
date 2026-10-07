package main

import (
	"reflect"
	"testing"

	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/ui"
)

func TestOnView_TagsTheWorkspace(t *testing.T) {
	sender := &captureSender{}
	h := &rtmEventHandler{program: sender, workspaceID: "T1"}
	h.OnView(slackclient.ViewEvent{ClientToken: "web-1", View: []byte(`{"id":"V1"}`)})
	want := ui.AppViewMsg{TeamID: "T1", ClientToken: "web-1", View: []byte(`{"id":"V1"}`)}
	if len(sender.sent) != 1 || !reflect.DeepEqual(sender.sent[0], want) {
		t.Errorf("sent %#v, want %#v", sender.sent, want)
	}
}
