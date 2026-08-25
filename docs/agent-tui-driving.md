# Driving the slk TUI from an agent

How an agent session runs and observes the real TUI without a human at the
keyboard, using herdr panes as the pty. Everything in "What works" was
verified end to end on 2026-08-25; the input gap at the bottom is real and
open.

An agent's first choice should not be this file: unit tests
(`tools/go.sh test ./...`), `tools/smoke.sh`, and the frame goldens cover
most verification needs headlessly. Drive the live TUI only when the
question is genuinely interactive.

## What works

Launch in a pane (always the agent sandbox — see
`docs/developing-on-santa-hosts.md` § Agent isolation):

```sh
herdr pane split --pane <own-pane> --direction right --ratio 0.5
# take pane_id from the JSON, then:
herdr pane run <pane> 'SLK_ROLE=agent <checkout>/tools/run-docker.sh'
```

`run-docker.sh` prints `container: slk-agent-<pid>` on start — record it;
that name is the only thing teardown is allowed to kill.

Wait and read:

```sh
herdr pane wait-output --match 'NORMAL' --timeout 60 --source visible <pane>
herdr pane read --source visible <pane>        # live screen
```

`--source recent` (the default) can lag the live screen; use `visible` for
assertions. `--ansi` keeps styling when colors are the thing under test.

Teardown: `docker kill <recorded container name>`, then
`herdr pane close <pane>`. Never select containers by pattern or label for
killing; the label (`slk.role=agent`) is a guardrail for what you must NOT
touch (anything without it), not a kill list.

## Two traps

**The viewgate freezes unviewed frames.** When slk runs with the herdr
integration and its tab is not the focused workspace's active tab, `View()`
serves a memoized frame while state keeps advancing
(`internal/ui/viewgate_fork.go`). A screen read of an unviewed pane can be
stale by design. `run-docker.sh` inherits `HERDR_*` from the pane, so a
pane-launched slk always has the integration on.

**Key injection does not reach slk (open).** `herdr pane send-text` and
`send-keys` provably deliver bytes to the pane and through `docker -it`
(verified with `cat` both at the shell and inside a container), but slk
processed none of the injected keys — `Q` (quit immediately) left the
process running, so this is not just the viewgate hiding a render.
`send-keys` emits kitty-protocol CSI-u sequences (`ctrl+c` arrives as
`ESC [99;5u` in a plain shell), and the suspected cause is the kitty
keyboard negotiation between Bubble Tea v2 and herdr's vterm; not yet
diagnosed. Until this is fixed, interactive key-driving is not a usable
verification channel — assert through `SLK_DEBUG` logs, `tools/smoke.sh`,
and frame goldens instead.
