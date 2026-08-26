---
name: refresh-sessions
description: Use when Justin asks to restart, refresh, bounce, or reload his slk sessions — "restart my slk sessions", "refresh the panes", "get my sessions on the new build" — typically after a fix lands on main so the live TUIs pick up the new binary. Also when a change was just merged and he asks whether/how his running sessions get it. Runs tools/refresh-sessions.sh, which restarts every live user-role slk pane in the current herdr session.
---

# Refreshing live slk sessions

`tools/refresh-sessions.sh` is the whole runbook: discover the user-role slk
panes, stop their containers, relaunch each pane. Run it; don't hand-roll the
steps. What follows is the judgment the script encodes, so a failure or an
edge case can be handled without rediscovering it.

## Why the flow is shaped this way

- **The TUI can't be driven from outside.** `herdr pane send-text` /
  `send-keys` never reach slk in these docker-attached panes (verified: keys
  land nowhere, no mode change), so "press Q, confirm" is not scriptable.
  `docker stop` is the quit path: slk is PID 1 in its container, bubbletea
  handles the SIGTERM, and main's defers release the herdr sidebar entry.
- **Stopping the user's containers here is compatible with the never-kill
  rule** (CLAUDE.md, Running Go on Santa-managed hosts): the ask *is* the
  restart, and identity is captured per pane at run time — the container
  name is read out of that pane's own foreground argv, never matched by
  name pattern or label sweep.
- **All sessions stop before the first relaunch.** The first
  `run-docker.sh` rebuilds `bin/slk-linux` when sources are newer, and every
  user container executes that same bind-mounted file — a rebuild under a
  live container can crash it. The script serializes: stop all, relaunch
  one, wait for it to paint, then the rest.
- **Restart is lossless.** slk persists each pane's last channel/thread
  (`pane_state`, keyed by pane id in the shared state volume), so
  relaunching in the same pane restores exactly where the session was.
  That's why the refresh relaunches in place instead of opening new tabs.
- **The draft guard aborts, it doesn't skip.** A pane whose composer isn't
  showing the empty-input placeholder may hold an unsent draft; skipping
  just that pane would leave it executing the binary the next relaunch
  rebuilds. Clear or send the draft, rerun.

## Handling the edges

- **Zero sessions found**: nothing is running. A dead pane has no process
  to discover; relaunch it by hand with
  `herdr pane run <pane_id> tools/run-docker.sh` — `pane_state` restores it.
- **`wait-output` timeout on the first pane**: the build is failing. Read
  the pane (`herdr pane read <pane_id> --source visible`) for the compile
  error; don't rerun until it builds.
- **Never print a discovered pane's process argv.** run-docker.sh forwards
  `ANTHROPIC_API_KEY` by name only (`-e ANTHROPIC_API_KEY`, docker reads
  the value from its own environment) precisely so argv stays
  secret-free; the script still confines argv parsing to python and prints
  only pane ids and container names, so a regression to inline
  interpolation can't leak into a transcript via `herdr pane process-info`.
- Agent-role sessions (`slk-agent-*`) are out of scope by construction:
  discovery matches the `slk.role=user` label in the pane's argv.
