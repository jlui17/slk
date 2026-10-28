# Phase 3 measurement baseline

Date: 2026-10-11
Commit: 30c1c61a (branch refactor/phase3/pr0-characterize, based on #304's head)
Toolchain: go version go1.26.5 linux/amd64; golangci-lint v2.14.0

## Test-file references to unexported members (spec Appendix A.1)

```
== messages.Model: 159 refs, 24 distinct members
   cache                  43
   yOffset                16
   version                15
   messages               12
   chromeHeight           10
   messageIDToEntryIdx    8
   lastReactionHits       6
   buildCache             6
   staleEntries           6
   lastViewHeight         6
   selRange               4
   selected               4
   selectedEndLine        3
   searchTerms            3
   selectedStartLine      3
   hasSnapped             2
   cacheMoreBelow         2
   totalLines             2
   lastHits               2
   entryOffsets           2
   renderLoadingOlderHint 1
   resolveAnchor          1
   hasSelection           1
   cacheWidth             1
== thread.Model: 137 refs, 20 distinct members
   chromeHeight           21
   replies                18
   selected               18
   vp                     16
   entryOffsets           10
   parent                 9
   cache                  7
   renderThreadMessage    7
   selRange               6
   userNamesV             5
   lastReactionHits       5
   version                3
   viewContent            2
   hasSelection           2
   hasSnapped             2
   totalLines             2
   parentEntry            1
   snappedSelection       1
   lastViewHeight         1
   userNames              1
== ui.App: 3972 refs, 191 distinct members
   messagepane            307
   threadPanel            166
   winModels              92
```

## Shared lines and file sizes

```
shared(>=16): 491 of 986
  3795 internal/ui/messages/model.go
  2314 internal/ui/thread/model.go
  4343 internal/ui/app.go
 10452 total
```

Benchmarks: see bench.txt (count=10, -benchmem).
