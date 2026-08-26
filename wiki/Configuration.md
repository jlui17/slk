# Configuration

Config lives at `~/.config/slk/config.toml`.

## Full example

```toml
[general]
default_workspace = "work"      # the slug, not the team ID
use_slack_sections = true       # use real Slack sidebar sections (default).
                                # set false to use [sections.*] globs instead.

[appearance]
theme = "dracula"
timestamp_format = "3:04 PM"
image_protocol = "auto"   # auto | kitty | sixel | halfblock | off
max_image_rows = 20       # cap inline image height in terminal rows
colored_usernames = false # color each user's name by ID hash (Slack-style)

[animations]
enabled = true
smooth_scrolling = true
typing_indicators = true

[notifications]
enabled = true
on_mention = true
on_dm = true
on_keyword = ["deploy", "incident"]
quiet_hours = "22:00-08:00"   # planned

# notify_command (optional): run INSTEAD of the built-in OS notification for any
# message that would notify (DM / mention / keyword). One slk runs it even when
# several are open (see below). Executed via `sh -c` with $SLK_TITLE and
# $SLK_BODY set, so you can route notifications through your own tooling
# (terminal-notifier, a multiplexer's notifier, mako, ...). Values are
# passed via the environment, so message text can't inject shell syntax.
# notify_command = 'terminal-notifier -title "$SLK_TITLE" -message "$SLK_BODY"'

# status_command (optional): run on every unread-state change (a message arrives
# or a channel is read) so an external surface can mirror slk's unread state.
# Only one slk runs it when several are open (see below).
# Because it fires on reads too, it can clear a status as well as set one.
# Runs are serialized and coalesced: states never run concurrently or out of
# order, and under a burst intermediate states may be skipped — the newest
# state always runs last, so the surface converges on the current state.
# Executed via `sh -c` with:
#   $SLK_UNREAD        unread channels in the active workspace (mute-filtered)
#   $SLK_OTHER_UNREAD  unread count across other workspaces
#   $SLK_WORKSPACE     active workspace name
#   $SLK_TITLE         the window-title string, e.g. "slk SW (3) +1"
# status_command = 'my-statusbar --slack-unread "$SLK_UNREAD"'

# With more than one slk running, only one of them runs these hooks. The first
# instance with something to report takes a lock on ~/.local/share/slk/notify.lock
# and holds it until it quits; another instance then takes over. Without it every
# notification would fire once per running slk, and each would drive the same
# status surface. Where the lock cannot be taken at all — a filesystem with no
# locking — every instance runs them again, on the grounds that duplicate
# notifications beat silence.

# Both hooks require a POSIX `sh` on $PATH and are unavailable on Windows
# (the built-in OS notification still works there). Hook failures are silent
# in the UI; run slk with SLK_DEBUG=1 and check slk-debug.log ([notify] lines)
# to diagnose a misbehaving command.

# Muted channels and DMs never notify — including on mentions and keywords —
# matching Slack. (This is a behavior change: previously a mention or keyword
# in a muted channel would still notify.)

[cache]
message_retention_days = 30
max_db_size_mb = 500
max_image_cache_mb = 200

# When slk runs inside a herdr pane, it mirrors an agent thread (a thread
# whose root message @-mentions a bot, e.g. Claude) onto herdr's agent
# sidebar: idle/working state, the bot's name, and a channel+snippet title.
# Opening one starts the mirroring, and it keeps going after you navigate
# away, close the thread panel, or switch to another workspace — that is
# where replies you haven't read arrive. Only opening a different agent
# thread switches it.
#
# The working state combines two signals: the assistant's live composing
# status ("is thinking…"), and the thread's content — the row reads as
# working while the newest message is a human's the bot hasn't reacted to
# (any emoji counts as its ack), or a bot todo-list post (the ones ending
# with a "todos as of HH:MM UTC" stamp).
#
# While Slack considers the thread unread, the row shows the unread reply
# count and herdr's unseen "done" indicator, the same blue dot it shows for
# an agent that finished while you were elsewhere. Reading the thread
# clears the count. Two limits come from herdr owning that indicator: it
# hides the dot while you are looking at the pane, and reading the thread
# on another device clears the count but cannot clear a dot already shown —
# focusing the tab does that.
#
# Running inside herdr also tells slk whether you are actually looking at
# the pane (its tab being the focused space's active tab), which is what
# read state keys off: a thread panel left open in a herdr tab you are not
# viewing does not count as read.
#
# slk also names the surrounding herdr tab after the thread's root message
# ("fix the ingest retries"; a task id anywhere in the message is hoisted
# to the front, "[colony-562] fix the flow viewer") — but only over a
# default tab label or one slk set itself; a label you typed is never
# overwritten. Labels slk set stay renameable everywhere they occur: the
# channel-name label the O keybinding gives a new tab counts (the opener
# claims it for the slk instance it spawns), and ownership survives herdr
# restarts (slk keeps its own record of the labels it set, since herdr
# keeps labels but forgets pane metadata on restart). All of the above
# needs no configuration — it activates from herdr's own pane environment
# (HERDR_ENV / HERDR_PANE_ID) and is inert everywhere else.
#
# Optionally, tab_name_model refines that label with a model-generated one:
# the named Anthropic model reads the thread's root message and writes a
# short task name, which replaces the truncated snippet a moment after the
# deterministic rename ("[colony-562] fix flow viewer rendering" instead of
# "[colony-562] the flow viewer renders sta…").
# The task-id prefix and the never-overwrite-your-labels rule still apply,
# and any failure (no key, timeout) leaves the deterministic label in
# place. Costs one small API call per opened agent thread; needs
# ANTHROPIC_API_KEY in slk's environment. The label derives from the root
# message once, at thread open; :retitle re-derives it later from the
# thread's recent messages (see Keybindings).
[herdr]
disabled = false   # set true to opt out of agent-sidebar reporting
tab_name_model = ""   # e.g. "claude-haiku-4-5"; empty disables (default)

# The O keybinding opens a Slack permalink in a new herdr tab running a
# second slk instance. The tab's shell runs `<open_command> '<permalink>'`;
# it is a host shell even when slk itself runs in a container (e.g. via
# tools/run-docker.sh), so point this at the host-side launch command.
open_command = "slk"   # default

# Relaunching slk reopens the workspace, channel, and thread that were open
# when it last ran (state is saved as you navigate, so a hard kill loses
# nothing). Inside herdr the state is kept per pane, so each pane restores
# its own view; outside herdr all instances share one slot. A permalink
# argument (slk <link>) overrides the restore for that launch.
[restore]
disabled = false   # set true to always start at the most-recently-visited channel

# Glob-based channel sections — only consulted when use_slack_sections
# is false (globally or per-workspace), or when Slack's section API is
# unreachable. Otherwise slk reads the user's actual Slack sections.
[sections.Alerts]
channels = ["alerts", "ops", "*-alerts"]
order = 1

# Channels can carry an optional ":<N>" suffix to pin their order
# within the section. Lower numbers sort higher. Entries without a
# suffix fall after annotated ones, in the order they appear.
# This syntax is only honored when use_slack_sections = false;
# in Slack-native mode, channel order comes from Slack.
[sections.Engineering]
channels = ["eng-general:1", "eng-alerts:2", "eng-*"]
order = 2

# Per-workspace settings: keyed by a slug you choose at --add-workspace
# time. team_id ties the slug to the underlying Slack workspace.
[workspaces.work]
team_id = "T01ABCDEF"
order   = 1                     # rail position; 1-based, used by 1-9 keys
theme   = "dracula"             # overrides [appearance].theme
use_slack_sections = false      # this workspace uses [sections.*] globs;
                                # other workspaces still use Slack sections

[workspaces.work.sections.Alerts]
channels = ["alerts", "*-alerts"]
order = 1

[workspaces.work.sections.Engineering]
channels = ["eng-*", "deploys"]
order = 2

# A second workspace with no per-workspace sections — falls back to
# the global [sections.*] above.
[workspaces.side]
team_id = "T02XYZ"
order   = 2

# Inline color overrides on top of the active theme
[theme]
primary = "#4A9EFF"
accent = "#50C878"
background = "#1A1A2E"
text = "#E0E0E0"
```

## Section resolution

When `use_slack_sections = true` (the default) and Slack's section endpoint
is reachable, slk reads the user's actual sidebar sections — names, emoji,
linked-list order, and channel membership — directly from Slack and keeps
them live via WebSocket events. Any `[sections.*]` or
`[workspaces.<slug>.sections.*]` blocks in `config.toml` are ignored in this
mode (a one-line info note is emitted to the debug log on first connect so
the shadowing isn't silent). Set `use_slack_sections = false` globally, or
per-workspace, to opt into glob-based sections instead.

Per-workspace `[workspaces.<slug>.sections.*]` blocks fully replace the
global `[sections.*]` for that workspace. Workspaces that define no
sections of their own fall back to the global table.

### Ordering channels within a section

Each entry in a section's `channels` list may carry an optional `:<N>`
suffix where `N` is a non-negative integer. Channels matched by an
annotated pattern sort ahead of channels matched by un-annotated
patterns; among annotated channels, lower `N` wins; un-annotated
channels keep the order Slack returned them in.

```toml
[sections.Engineering]
channels = ["eng-general:1", "eng-alerts:2", "eng-*"]
order = 2
```

In the example above, `#eng-general` is pinned to the top of the
Engineering section, followed by `#eng-alerts`, followed by every
other `eng-*` channel in Slack-API order.

This syntax is only honored when `use_slack_sections = false` (or
when Slack's section endpoint is unreachable and slk falls back to
glob mode). In Slack-native mode, channel order within a section
comes from Slack and the `:<N>` suffix is ignored along with the
rest of the `[sections.*]` block.

### Limitations of Slack-native sections

Slack-native sections are read-only — section editing still happens in the official client; slk
reflects the results. The `stars` section type (Slack's "Starred" feature) is rendered
when non-empty, with the header `Starred`. Sections of type `slack_connect`,
`salesforce_records`, and `agents` are hidden. Sections with more than 10 channels may be returned
only partially by Slack's API on initial load; the missing channels
temporarily fall into the catch-all bucket and migrate into their correct
section as WebSocket events fire or the workspace reconnects. A debug-log
warning identifies which sections were truncated.

## Workspace order

The `order` field controls workspace position in the rail and the mapping
for the `1`–`9` digit keys. Positive values sort ascending (lowest first);
workspaces without an `order` (or with `order = 0`) sort after explicitly
ordered ones, alphabetically by slug. Tokens on disk that have no
`[workspaces.<slug>]` block at all sort last, alphabetically by team ID.
The order is stable across runs. Previously the rail order depended on
which workspace's WebSocket connected first; it is now deterministic
regardless of network timing, even without an explicit `order` set.

Legacy configs that key the block by raw team ID
(`[workspaces.T01ABCDEF]`) keep working unchanged.

## Terminal-palette themes (`ANSI Dark`, `ANSI Light`)

Two built-in themes use ANSI 16 color codes exclusively rather than
fixed RGB values. They inherit the user's terminal color palette, so
changing your terminal colorscheme (light/dark, solarized,
accessibility palettes, etc.) immediately changes slk's UI colors to
match.

```toml
[appearance]
theme = "ANSI Dark"   # or "ANSI Light"
```

Pick the variant whose background matches your terminal's background.

**Trade-off:** selection-row highlights and compose-input tints are
still computed as RGB approximations, so the tint regions of those
elements use truecolor rather than your palette. The rest of the UI
honors the palette.

## Custom themes

Drop `.toml` files into `~/.config/slk/themes/`:

```toml
name = "My Theme"

[colors]
primary      = "#BD93F9"
accent       = "#50FA7B"
warning      = "#FFB86C"
error        = "#FF5555"
background   = "#282A36"
surface      = "#343746"
surface_dark = "#21222C"
text         = "#F8F8F2"
text_muted   = "#6272A4"
border       = "#44475A"

# Optional sidebar/rail overrides — lets you have a darker sidebar with a
# lighter message pane (Slack's default look). Fall back to
# background/text/text_muted/surface_dark when omitted.
sidebar_background = "#19171D"
sidebar_text       = "#D1D2D3"
sidebar_text_muted = "#9A9B9E"
rail_background    = "#19171D"
```

Every built-in theme now sets a channels-panel (sidebar) background that is
perceptibly distinct from the message pane. When writing a custom theme,
set `sidebar_background` to a clearly darker (or, on near-black themes, a
slightly lighter) shade than `background` for the same effect.

Switch themes live with `Ctrl+y`.

## Data paths (XDG)

| Path | Contents |
|---|---|
| `~/.config/slk/` | Configuration, custom themes |
| `~/.local/share/slk/` | SQLite cache, tokens |
| `~/.cache/slk/` | Avatars, image cache |

Two lock files sit alongside those, both empty. `~/.config/slk/config.toml.lock`
serializes edits to `config.toml` between instances: a save that cannot take it
within a moment writes anyway, so a stuck instance costs at most one overwritten
setting — except `slk --add-workspace`, which stops with an error rather than
risk a config that will not parse. `~/.local/share/slk/notify.lock` picks the
instance that runs the notification hooks. Deleting either while slk is closed
is harmless; they come back on their own.
