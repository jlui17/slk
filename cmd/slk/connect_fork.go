package main

import (
	"context"
	"sync"

	"github.com/gammons/slk/internal/bootstrap"
	"github.com/gammons/slk/internal/service"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/slack-go/slack"
)

// bootChains holds the results of connectWorkspace's three independent
// boot request chains.
type bootChains struct {
	res *bootstrap.Result
	err error

	// sectionStore is nil unless useSlackSections was set.
	sectionStore    *service.SectionStore
	sectionStoreErr error

	channels    []slack.Channel
	channelsErr error
}

// runBootChains runs bootstrap.Run, users.conversations and, when
// useSlackSections is set, the section-store bootstrap concurrently and
// returns once all have finished. connectWorkspace consumes the results
// in upstream's serial order: the channel-item build reads the section
// store, and GetChannels' fallback reads res. Exactly the same requests
// as the serial order made — only the timing differs (docs/fork.md
// records why the call pattern itself is load-bearing).
func runBootChains(ctx context.Context, client *slackclient.Client, deps bootstrap.Deps, useSlackSections bool) bootChains {
	var b bootChains
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		b.res, b.err = bootstrap.Run(ctx, boundBootstrapDeps(deps, bootCallTimeout))
	}()
	go func() {
		defer wg.Done()
		b.channels, b.channelsErr = client.GetChannels(ctx)
	}()
	if useSlackSections {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.sectionStore = service.NewSectionStore()
			sctx, scancel := bootCtx(ctx)
			defer scancel()
			b.sectionStoreErr = b.sectionStore.Bootstrap(sctx, client)
		}()
	}
	wg.Wait()
	return b
}
