package main

import "github.com/gammons/slk/internal/usernames"

// newUserNameStore is upstream's constructor for its engine-side name
// store, kept for upstream's tests: the fork's workspaces hold an
// internal/usernames.Store instead (see docs/fork.md).
func newUserNameStore(seed map[string]string) *usernames.Store { return usernames.FromMap(seed) }
