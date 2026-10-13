---
name: slk-herdr
description: Use when managing Justin's live slk sessions (the Slack TUI) in herdr panes, from any repo. Restarting, refreshing or reloading them, for example after a fix lands so the panes run the new build. Relaunching slk in panes that sit at a shell prompt after a herdr restart. Finding which panes or tabs show a given thread, channel or kind of message (such as every "!review-open" thread). Moving slk panes or tabs into another herdr space. Opening a new slk session on a Slack permalink. Reading what an slk pane shows, or judging whether it is connected.
---

# Managing slk sessions in herdr

Justin runs many slk sessions at once, one per herdr pane, each parked on
the channel or thread it is working on. Each session is a docker container
started by `tools/run-docker.sh` in the slk checkout. This skill is how to
find, restart, relaunch and move those sessions without losing their state
or touching anything else on the machine.

## The slk checkout, and which build the sessions run

Every command here runs a file from the slk checkout:
`~/src/personal/slk` on Justin's machines. If it is not there, ask Justin
where it is.

The sessions run whatever the checkout's working tree holds. Every
user-role launch rebuilds `bin/slk-linux` from the tree when any `.go`
file is newer than the binary. So a refresh while a feature branch is
checked out puts the live sessions on that branch's build. Before a
refresh, check `git -C ~/src/personal/slk branch --show-current`, and tell
Justin when it is not `main`.

## What may be stopped or killed

This machine also runs Justin's own processes. The slk repo's CLAUDE.md
holds the full rule; when this skill runs from another repo that file is
not loaded, so these are the parts that apply here:

- A user session (container label `slk.role=user`) is stopped only by
  `tools/refresh-sessions.sh`, which reads each container's name from its
  own pane's process. That is the restart Justin asked for. Never stop
  one by a name pattern or a label sweep.
- An agent session (label `slk.role=agent`) is stopped only when this
  session created it, by the container name recorded at launch.
- Never print a pane's process argv or env. `herdr pane process-info`
  returns the full `docker run` argv; parse it inside a script and print
  only pane ids and container names, as `tools/refresh-sessions.sh` does.

## User role versus agent role

`run-docker.sh` picks its role from the environment. When `CLAUDECODE` is
set (any Claude Code shell, including yours), it runs as the agent role:
a separate state volume, `bin/slk-linux-agent`, and `slk-agent-*`
containers. That keeps an agent's own test slk away from Justin's.

Justin's sessions, their saved pane states and their config live in the
user role. To read or act on them from an agent shell, set the role
explicitly: `SLK_ROLE=user ~/src/personal/slk/tools/run-docker.sh herdr …`.
Without it, `herdr pane states` and `herdr reboot` read the agent volume
and report the wrong panes.

A command that `herdr pane run` types into one of Justin's panes runs in
his shell, which has no `CLAUDECODE`, so it gets the user role on its own.

A user-role call rebuilds `bin/slk-linux` when the sources are newer, and
every live user container executes that bind-mounted file, so a rebuild
under a running session can crash it. While user sessions are live, gate
every bare `SLK_ROLE=user … run-docker.sh` call on nothing being newer
than the binary, so the call runs only when it will not rebuild:

```bash
cd ~/src/personal/slk && [ -z "$(find cmd internal -name '*.go' -newer bin/slk-linux -print -quit)" ] \
  && SLK_ROLE=user tools/run-docker.sh herdr pane states
```

When the gate fails, run the refresh first, which stops every session
before it builds.

## Finding the sessions and what each one shows

- **Live panes and tab labels:** `herdr api snapshot` lists every
  workspace (a herdr "space"), tab and pane, with labels and `focused`.
  `herdr workspace list` maps a space's name to its id.
- **Which panes run slk:** an slk pane's `terminal_title` in the
  snapshot starts with `slk`, and its screen's last line is slk's status
  bar. `tools/refresh-sessions.sh` finds them by their process instead,
  for the restart.
- **What each pane has open:**
  `SLK_ROLE=user ~/src/personal/slk/tools/run-docker.sh herdr pane states`
  (behind the rebuild gate above) prints one tab-separated line per saved
  pane state: pane id, team, channel, thread ts (empty when no thread is
  open), last update. It also lists panes that no longer exist; join it
  with the snapshot's live pane ids.
- **A moved pane** has no line under its current id: its state stays
  under the id the pane had when slk started (see moving, below). The
  move's result names that id as `previous_pane_id`; without it, match
  the thread on the pane's screen against the lines of panes that no
  longer exist.
- **What a thread is about:** read the thread's first message with the
  Slack MCP `slack_read_thread` (channel id, thread ts, `limit` 1). When
  no Slack tools are loaded, read the pane's screen: an open thread's
  first message sits at the top of the thread panel. A session tracking
  an agent thread also reports a `title` on its pane in the snapshot,
  which starts with the channel and the root's first words; that is a
  quick hint, and the pane state is the record.

## Reading a pane's screen

`herdr pane read <pane_id> --source visible` returns the screen text; the
last line is slk's status bar (mode, channel, `● Connected`).

A pane in a tab that is not on screen keeps showing its last frame and
redraws only when the frame would turn wrong by itself: the connection
state changes, the pane resizes, or a load or sync is in flight. The
status bar's connection state is therefore current, but newer messages
may be missing from the screen. Read the pane state or Slack for content,
and the screen for connected or not.

## Refreshing every live session

Run `~/src/personal/slk/tools/refresh-sessions.sh`. It is the whole
runbook: it finds the user-role slk panes, stops all their containers,
relaunches the first pane alone (which rebuilds the binary when the
sources changed), waits for it to draw, then relaunches the rest. Check
afterwards that each pane's last line reads `● Connected`.

Why it is shaped this way:

- **It quits through docker, not keystrokes.** slk is PID 1 in its
  container, so `docker stop` delivers SIGTERM, which slk handles: it
  exits cleanly and releases its herdr sidebar entry. Scripting "press Q,
  confirm" against a pane whose screen may be a stale frame is fragile.
- **Every session stops before the first relaunch,** so the rebuild never
  replaces the binary under a running container.
- **A restart keeps each session's place.** slk saves each pane's channel
  and thread (its pane state) and restores it when slk starts again in
  the same pane, so the refresh relaunches in place, not in new tabs.
- **A draft stops the whole refresh.** When a pane's composer does not
  show its empty placeholder, the script exits before stopping anything
  and names the pane. Skipping only that pane would leave it running the
  binary the next relaunch rebuilds. Ask Justin to send or clear the
  draft; never clear it yourself. Then rerun.

When the first pane never draws (the script's wait times out), the build
is failing: `herdr pane read <pane_id> --source visible` shows the compile
error. Fix the build before rerunning.

## Relaunching slk in idle panes

After a herdr restart, or when panes that ran slk now sit at a shell
prompt, `herdr reboot` relaunches slk in each pane that has a saved pane
state and nothing in the foreground. It skips panes that already run slk
or something else.

```bash
SLK_ROLE=user ~/src/personal/slk/tools/run-docker.sh herdr reboot --dry-run
SLK_ROLE=user ~/src/personal/slk/tools/run-docker.sh herdr reboot
```

Run the dry run first, and run both behind the rebuild gate above,
because reboot launches every pane at once. The dry run prints `launch`, `live`
or `stale` per saved pane state; `stale` is a pane that no longer exists.

## Moving a session to another space

```bash
herdr pane move <pane_id> --new-tab --workspace <workspace_id> --label '<tab label>' --no-focus
```

- `--label` names the new tab. `--tab-label` is accepted only with
  `--new-workspace`; with `--new-tab`, herdr rejects it and moves nothing.
- To put the pane beside another pane instead of in its own tab:
  `--tab <tab_id> --split right|down`.
- The pane gets a new id in the new space (the result's
  `move_result.pane.pane_id`), and herdr closes the tab the move left
  empty. Keep the tab's label by passing it to `--label`.
- The running slk follows the move and keeps working. Its pane state stays
  saved under the id the pane had when slk started. So the next time slk
  restarts in the moved pane (a refresh, for example), it opens fresh,
  without its thread. Tell Justin this when you move a session.

## Opening a new session on a thread

slk launched with a Slack permalink opens that message, or its thread:

```bash
herdr tab create --workspace <workspace_id> --label '<tab label>' --no-focus
herdr pane run <new_pane_id> "$HOME/src/personal/slk/tools/run-docker.sh '<permalink>'"
```

Inside slk, `O` on a message opens its link in a new herdr tab running
slk, and `alt+o` opens it in a new pane split beside slk.
