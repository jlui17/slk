package bootstrap

import (
	"context"
	"sync"
)

// overlapPhases is everything Run does after userBoot, with the
// independent network calls overlapped instead of chained:
// client.counts, the channel open (conversations.view falling back to
// conversations.history) and the channels/info revalidation run
// concurrently; the users/info revalidation runs after the channel
// open completes. Exactly the same requests are made as when the
// chain was serial — only the timing differs.
//
// The users pass has to trail the open: the users it revalidates are
// the ones conversations.view returned, so running it any earlier
// would scope the request to the open-DM counterparties alone and
// leave every author in the opened channel stale. The channels pass
// has no such dependency — its id set is userBoot's channels + ims,
// already on out before this is called.
//
// Revalidation replaces slk's ~50-page users.list sweep: the official
// client issues zero users.list and zero conversations.list calls
// across all 8 captures; it sends {id: version} for what it holds and
// gets back only what moved. The id sets are deliberately SCOPED
// rather than "everything cached" — see revalidateChannels and
// revalidateUsers; everything else is left stale and revalidated when
// first needed. A fixed batch size over an unbounded id set emits a
// long run of identically-sized requests — 125 consecutive exactly-80s
// on a 10k-user workspace — which is a cleaner distributional
// signature than the client's own ragged 1-80 spread.
//
// Every degrade semantic is Run's own — see Run's "What is fatal".
// The only error returned is a nil dependency the open needs, checked
// before any goroutine starts so a wiring bug costs no requests.
//
// Result assembly is race-free by field ownership: the counts
// goroutine writes only Counts and CountsOK; the open-then-users
// goroutine writes only OpenedChannelID (set before the goroutines
// start) and the fields openChannel owns; the channels pass writes
// nothing to out and reads only Channels and IMs, assembled before
// any goroutine started.
func overlapPhases(ctx context.Context, deps Deps, out *Result, logf func(string, ...any)) error {
	if deps.OpenChannelID != "" {
		if err := checkOpenChannelDeps(deps); err != nil {
			return err
		}
		// Set from what was ASKED for, before the call, so that no
		// path can report a channel other than the one requested.
		out.OpenedChannelID = deps.OpenChannelID
	}

	// Nil revalidation dependencies are logged and skipped rather than
	// returned as errors, which is the opposite of what Run does for
	// Boot, Counts, View, History and Store-when-opening-a-channel.
	//
	// The rule is the same in both places — a wiring bug must not
	// panic — but the consequence differs. A missing Viewer means no
	// channel opens; a missing Revalidator means the cache stays as
	// stale as it already was, which is exactly the outcome of a
	// revalidation request that fails, and that outcome is documented
	// non-fatal. Refusing to boot over it would be strictly worse than
	// the failure it is reporting. The log line is the only signal, so
	// it says what was lost.
	canRevalidate := true
	switch {
	case deps.Revalidate == nil:
		logf("bootstrap: revalidation skipped: Deps.Revalidate is nil; the cache will render stale")
		canRevalidate = false
	case deps.Store == nil:
		logf("bootstrap: revalidation skipped: Deps.Store is nil; the cache will render stale")
		canRevalidate = false
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		// Unread state. Non-fatal: badges are cosmetic and a workspace
		// that boots without them beats one that does not boot.
		if counts, err := deps.Counts.Counts(ctx); err != nil {
			logf("bootstrap: counts: %v (continuing without unread state)", err)
		} else {
			out.Counts = counts
			out.CountsOK = true
		}
	}()
	go func() {
		defer wg.Done()
		if deps.OpenChannelID != "" {
			if err := openChannel(ctx, deps, out, logf); err != nil {
				// Non-fatal: both paths to the channel failed, and the
				// cost of that is one empty message pane. Failing the
				// boot instead would cost the whole workspace — see
				// Run's "What is fatal". Messages stays empty and
				// OpenedChannelID stays what was asked for, so the
				// caller opens the right conversation with no
				// scrollback rather than silently reopening a
				// different one.
				logf("bootstrap: opening %s: %v (continuing with an empty channel)", deps.OpenChannelID, err)
			}
		}
		if canRevalidate {
			revalidateUsers(ctx, deps, out, logf)
		}
	}()
	if canRevalidate {
		wg.Add(1)
		// Independent of the users pass, not one early-returning
		// sequence: a failed channels/info says nothing about
		// users/info, and losing the user pass to it would send slk
		// back to resolving every author one users.info call at a
		// time — the fan-out this phase exists to delete.
		go func() {
			defer wg.Done()
			revalidateChannels(ctx, deps, out, logf)
		}()
	}
	wg.Wait()
	return nil
}
