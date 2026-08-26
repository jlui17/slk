package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gammons/slk/internal/bootstrap"
	"github.com/gammons/slk/internal/slack/boot"
	"github.com/gammons/slk/internal/slack/edge"
)

// bootCallTimeout bounds each network call on the paint-blocking boot
// path. The API client sets no http.Client.Timeout — the same client
// serves long file downloads that must not be capped — so without a
// per-call ctx bound a server that accepts the connection and never
// answers wedges the boot forever (see the shouldReload comment in
// main.go). 15s matches shouldReloadTimeout and MintToken's bound
// rather than inventing a third number for the same job.
const bootCallTimeout = 15 * time.Second

func bootCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, bootCallTimeout)
}

// boundBootstrapDeps wraps every network dependency in deps with a
// per-call timeout of d, so a wedged call errors and bootstrap.Run
// degrades by its documented non-fatal semantics instead of hanging
// the workspace behind the loading overlay.
func boundBootstrapDeps(deps bootstrap.Deps, d time.Duration) bootstrap.Deps {
	deps.Boot = boundedBoot{deps.Boot, d}
	deps.Counts = boundedCounts{deps.Counts, d}
	deps.View = boundedView{deps.View, d}
	deps.History = boundedHistory{deps.History, d}
	deps.Revalidate = boundedRevalidate{deps.Revalidate, d}
	return deps
}

// capErr names the cap in the error chain, so the log line a timeout
// produces says "timed out after 15s" rather than a bare "context
// deadline exceeded" with no owner.
func capErr(err error, d time.Duration) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("timed out after %s: %w", d, err)
	}
	return err
}

type boundedBoot struct {
	inner bootstrap.UserBooter
	d     time.Duration
}

func (b boundedBoot) UserBoot(ctx context.Context) (*boot.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, b.d)
	defer cancel()
	res, err := b.inner.UserBoot(ctx)
	return res, capErr(err, b.d)
}

type boundedCounts struct {
	inner bootstrap.CountsFetcher
	d     time.Duration
}

func (b boundedCounts) Counts(ctx context.Context) (bootstrap.Counts, error) {
	ctx, cancel := context.WithTimeout(ctx, b.d)
	defer cancel()
	res, err := b.inner.Counts(ctx)
	return res, capErr(err, b.d)
}

type boundedView struct {
	inner bootstrap.Viewer
	d     time.Duration
}

func (b boundedView) ConversationsView(ctx context.Context, channelID string) (*boot.ViewResult, error) {
	ctx, cancel := context.WithTimeout(ctx, b.d)
	defer cancel()
	res, err := b.inner.ConversationsView(ctx, channelID)
	return res, capErr(err, b.d)
}

type boundedHistory struct {
	inner bootstrap.Historian
	d     time.Duration
}

func (b boundedHistory) HistoryWithVersions(ctx context.Context, channelID string, cached map[string]string) (bootstrap.History, error) {
	ctx, cancel := context.WithTimeout(ctx, b.d)
	defer cancel()
	res, err := b.inner.HistoryWithVersions(ctx, channelID, cached)
	return res, capErr(err, b.d)
}

type boundedRevalidate struct {
	inner bootstrap.Revalidator
	d     time.Duration
}

func (b boundedRevalidate) ChannelsInfo(ctx context.Context, teamID string, updatedIDs map[string]int64) (edge.ChannelsInfoResult, error) {
	ctx, cancel := context.WithTimeout(ctx, b.d)
	defer cancel()
	res, err := b.inner.ChannelsInfo(ctx, teamID, updatedIDs)
	return res, capErr(err, b.d)
}

func (b boundedRevalidate) UsersInfo(ctx context.Context, updatedIDs map[string]int64) ([]edge.User, error) {
	ctx, cancel := context.WithTimeout(ctx, b.d)
	defer cancel()
	res, err := b.inner.UsersInfo(ctx, updatedIDs)
	return res, capErr(err, b.d)
}
