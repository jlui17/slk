---
name: try-it-tab
description: Use after a change to slk that Justin judges by looking or pressing keys (a picker, a key binding, a render, a mode, a toast) once its tests pass and before asking for the merge word. Also when he asks for a test thread, a test session, or somewhere to try a change. Brings up the branch's slk in its own herdr tab, on a real message that exercises the change, for him to drive.
---

# A try-it tab for a UI or UX change

Tests show that the code does what we agreed. They do not show that it
feels right. Justin decides that with his hands, so the change comes to him
as a running slk, already on a message where the change shows, without him
asking for it. Bring it up when the tests pass, then tell him it is there.

## Bring it up

From a session inside herdr, in this session's own space:

```bash
out=$(herdr tab create --workspace "$HERDR_WORKSPACE_ID" --label 'try <feature>' --no-focus)
tab=$(jq -r .result.tab.tab_id <<<"$out"); pane=$(jq -r .result.root_pane.pane_id <<<"$out")
herdr pane run "$pane" "cd <worktree> && SLK_ROLE=agent tools/run-docker.sh '<permalink>'"
```

Record the tab id, and the container name from
`docker ps --filter label=slk.role=agent` once it is up. Outside herdr, hand
him the `cd … && tools/run-docker.sh '<permalink>'` line to run himself.

- **`SLK_ROLE=agent` is not optional.** A fresh herdr shell has no
  `CLAUDECODE`, so without it `run-docker.sh` takes the user role: his state
  volume, and a build to the `bin/slk-linux` his live sessions execute.
- **The permalink boots slk onto the test message.** Find a real message
  that exercises the change with a read-only search (slk's search, or the
  Slack MCP search tools). Never post a test message: it goes out under his
  name. If no message fits, say so and ask.
- **`--no-focus`, and send no keys.** Focus is his: a focus move lands
  whatever he is typing in a live slk, and `i` then Enter sends a Slack
  message as him. slk also serves a frozen frame while its tab is unviewed
  (`internal/ui/viewgate_fork.go`), so `herdr pane read` shows a stale boot
  frame, not the truth. The tab cannot be checked from outside; that is why
  this is his check and not a worker's.
- **What runs the branch and what does not.** The try-it tab runs the
  worktree's build. Anything it spawns through `open_command` (the `O` tabs)
  is a fresh herdr shell running the main checkout's user-role slk.

## Tell him

One short message: the tab label, the message to go to (sender and first
words, in case the cursor is not on it), the keys to press, and what he
should see for each. Add that `Q` then `y` quits, because plain `q` only
closes the thread view.

## Take it down

When he says he is done, or the change merges: `herdr tab close <tab id>`
and stop the recorded container, both by the ids recorded above. Tabs he
opened from inside the try-it tab are his.
