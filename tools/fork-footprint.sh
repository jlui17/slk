#!/bin/sh
# Prints the fork's churn on upstream-owned files: every file that exists
# upstream and that this fork has modified, with its diff size. Files the
# fork added are excluded; they can't conflict with an upstream merge.
# Run `git fetch upstream` first. Extra args narrow the diff, e.g.:
#   tools/fork-footprint.sh -- internal/ui
set -e
base=$(git merge-base HEAD upstream/main)
git diff --diff-filter=M --stat "$base" HEAD "$@"
