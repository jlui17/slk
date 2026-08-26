package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gammons/slk/internal/bootstrap"
	"github.com/gammons/slk/internal/slack/boot"
	"github.com/gammons/slk/internal/slack/edge"
)

// stallDeps satisfies every bootstrap network interface and never
// answers until its ctx dies — a wedged server, as bootstrap sees it.
type stallDeps struct{}

func (stallDeps) UserBoot(ctx context.Context) (*boot.Result, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (stallDeps) Counts(ctx context.Context) (bootstrap.Counts, error) {
	<-ctx.Done()
	return bootstrap.Counts{}, ctx.Err()
}

func (stallDeps) ConversationsView(ctx context.Context, channelID string) (*boot.ViewResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (stallDeps) HistoryWithVersions(ctx context.Context, channelID string, cached map[string]string) (bootstrap.History, error) {
	<-ctx.Done()
	return bootstrap.History{}, ctx.Err()
}

func (stallDeps) ChannelsInfo(ctx context.Context, teamID string, updatedIDs map[string]int64) (edge.ChannelsInfoResult, error) {
	<-ctx.Done()
	return edge.ChannelsInfoResult{}, ctx.Err()
}

func (stallDeps) UsersInfo(ctx context.Context, updatedIDs map[string]int64) ([]edge.User, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestBoundBootstrapDeps_CapsEveryNetworkCall drives each wrapped
// dependency against a never-answering fake with an unbounded caller
// ctx. Without the per-call cap every one of these calls blocks
// forever (the test itself would hang and time out).
func TestBoundBootstrapDeps_CapsEveryNetworkCall(t *testing.T) {
	deps := boundBootstrapDeps(bootstrap.Deps{
		Boot:       stallDeps{},
		Counts:     stallDeps{},
		View:       stallDeps{},
		History:    stallDeps{},
		Revalidate: stallDeps{},
	}, 20*time.Millisecond)

	calls := map[string]func(ctx context.Context) error{
		"UserBoot": func(ctx context.Context) error {
			_, err := deps.Boot.UserBoot(ctx)
			return err
		},
		"Counts": func(ctx context.Context) error {
			_, err := deps.Counts.Counts(ctx)
			return err
		},
		"ConversationsView": func(ctx context.Context) error {
			_, err := deps.View.ConversationsView(ctx, "C1")
			return err
		},
		"HistoryWithVersions": func(ctx context.Context) error {
			_, err := deps.History.HistoryWithVersions(ctx, "C1", nil)
			return err
		},
		"ChannelsInfo": func(ctx context.Context) error {
			_, err := deps.Revalidate.ChannelsInfo(ctx, "T1", map[string]int64{"C1": 0})
			return err
		},
		"UsersInfo": func(ctx context.Context) error {
			_, err := deps.Revalidate.UsersInfo(ctx, map[string]int64{"U1": 0})
			return err
		},
	}
	for name, call := range calls {
		err := call(context.Background())
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("%s against a stalled dep: err = %v, want the per-call deadline", name, err)
			continue
		}
		if !strings.Contains(err.Error(), "timed out after") {
			t.Errorf("%s error = %q; want the cap named in the chain", name, err)
		}
	}
}

type okBoot struct{ res *boot.Result }

func (b okBoot) UserBoot(ctx context.Context) (*boot.Result, error) { return b.res, nil }

func TestBoundBootstrapDeps_PassesResultsThrough(t *testing.T) {
	want := &boot.Result{}
	deps := boundBootstrapDeps(bootstrap.Deps{
		Boot:       okBoot{want},
		Counts:     stallDeps{},
		View:       stallDeps{},
		History:    stallDeps{},
		Revalidate: stallDeps{},
	}, time.Second)
	got, err := deps.Boot.UserBoot(context.Background())
	if err != nil || got != want {
		t.Errorf("UserBoot = (%p, %v), want the inner result (%p) untouched", got, err, want)
	}
}
