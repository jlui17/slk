# Demo mode for recording slk

## Goal

Produce polished, reproducible recordings of slk (a hero GIF for the README
plus short feature clips) without exposing any real Slack data. Today the
README has one static screenshot (`docs/assets/screenshot.png`) and slk cannot
run without a real, connected workspace.

Demo mode is a **recording tool**, not a user feature. It is launched by a
hidden flag, is not documented for users, and its fake data only needs to
hold up along the paths the recording scripts take.

### Success criteria

- `slk --demo` starts instantly, with no config, tokens, SQLite cache, network
  or config writes, and shows two believable fake workspaces.
- Each of these moments can be recorded: vim-style navigation, the fuzzy
  channel finder, opening a thread, live activity (typing, a new message
  arriving, a reaction appearing), composing and sending, adding a reaction,
  the theme switcher, switching workspaces, and window splits.
- `make demo-gifs` regenerates every GIF from checked-in VHS tapes.

## Recordings

- **Hero GIF** (~20–25s, replaces the README screenshot): navigation, the
  channel finder, live activity, and a window split.
- **Feature clips**, one short tape each: themes, threads, reactions,
  workspaces, compose. All clips use the same fake data.

Tapes live at `docs/assets/demo/<scenario>.tape`. Each renders
`docs/assets/demo/<scenario>.gif`. VHS draws through ttyd/xterm.js. Stock VHS
does not capture xterm.js's sixel layer, so the first recordings used slk's
half-block image path. They now use sixel, recorded with the VHS from
charmbracelet/vhs#783 (pinned in `flake.nix` until it is released):
`settings.tape` sets `SLK_DEMO_IMAGES=sixel` and the cell size in pixels,
which ttyd does not report.

## Approach

Fake the services in `internal/core/ports.go` (the only way `internal/ui`
reaches the outside world) and feed the App the same messages `cmd/slk` sends.
Two alternatives were rejected:

- A fake Slack server (HTTP, browser-protocol WebSocket, edge API, token
  minting) would test everything end to end, but it is several times the work
  and breaks whenever slk starts calling a new endpoint.
- A pre-filled SQLite cache with an offline start supports no live events,
  sending fails, and slk still tries to connect.

## Architecture

```
cmd/slk/
  main.go        the existing argument switch gains `--demo [scenario]`,
                 which is not listed in --help
  demo.go        runDemo(scenario): builds the App, installs the demo
                 services, starts tea.Program, starts the director
internal/demo/
  world.go       World: a mutex-guarded in-memory store of workspaces,
                 users, channels, messages, threads, reactions, unread state
  fixtures.go    the content, as plain Go literals
  services.go    builds core.*Service values over a World, using the
                 existing core.New…Service adapters and *Funcs structs
  avatars.go     generated avatars and the one generated inline image
  director.go    rule engine that sends tea.Msgs in response to user actions
  scenarios.go   the named rule sets: hero, themes, threads, reactions,
                 workspaces, compose
```

Dependencies: `internal/demo` may import `internal/core`, `internal/ui` (for
message types and sidebar/workspace item types), `internal/image` (half-block
rendering only) and bubbletea. It must not import `internal/slack`,
`internal/cache`, `internal/config`, or anything that touches the network or
filesystem. A test enforces this, in the same way as
`internal/ui/boundary_test.go`.

### Shared startup with `run()`

`run()` in `cmd/slk/main.go` performs about 30 `app.Set…` calls, mixed in with
token, SQLite, probe and connection code. UI setup that both `run()` and
`runDemo()` need (theme items and overrides, emoji and image context, the
timestamp formatter, the help footer) is moved into a shared helper, in its own
commit and with no change in behaviour. That commit happens before any demo
code, so `runDemo()` does not copy it.

`runDemo()` skips: config loading and every config write, tokens and re-minting,
SQLite, the kitty and sixel probes, WebSocket connections, the status reporter
and notifications. It uses the half-block image protocol unless
`SLK_DEMO_IMAGES=sixel`, and a fixed timestamp format.

### Services

Each fake reads and writes the `World` and returns the same `Msg` type as the
real implementation. The implementation plan must confirm each returned type
against its reducer.

| Port | Demo behaviour |
|---|---|
| ChannelService | `Fetch` returns the channel's messages. `FetchOlder` returns an empty page. `FetchAround` returns the window around the ts. `ReadCache` returns nil. `MarkRead` clears unread state. `Lookup` reads World. `SearchRemote` returns nil. Remaining methods do nothing. |
| ThreadService | `Fetch` returns replies. `SendReply` adds to World. `ListFetch` returns the threads the user is involved in. `Mark` clears thread unread. `ThreadLastRead` reads World. |
| MessageService | `Send`, `Edit` and `Delete` change World and return the usual result msgs. `MarkUnread` updates World. `Forward` and `Permalink` return a "not available in demo" error. |
| ReactionService | `Add` and `Remove` change World. `LoadFrecent` returns a fixed list. `RecordFrecent` does nothing. |
| SearchService | `SearchChannel` does a substring search over World. `SearchWorkspace` returns empty results. |
| UnreadService | Computed from World. |
| WorkspaceService | `Switch` returns a `WorkspaceSwitchedMsg` built from World. |
| AvatarService | Returns the generated half-block avatar for the user. |
| ImageFetcher | Serves the demo's two images (the chart and the hero's meme) from memory. |
| SettingsService | Does nothing, so switching theme in the demo never writes the user's config. |
| PresenceService | Does nothing. |
| ActivityService, FileService, EditorService, DesktopService | Do nothing, or return a "not available in demo" error, which the App shows as its normal toast. None touch the network or disk. |

## Content

All timestamps are relative to process start, so date separators read "Today"
and "Yesterday".

**Lumen Labs**, a small software startup. It is the active workspace at start,
with its own dark theme.
- Channels: `#general`, `#engineering`, `#deploys`, `#design`, `#random`,
  `#incidents`. DMs with three people and one group DM.
- About 10 people. The user is "Alex Rivera".
- Starts with several unread channels and one @mention badge.
- Content chosen to show off rendering:
  - a Go code block in `#engineering`
  - a deploy thread with about 6 replies in `#deploys`
  - a deploy bot message using a legacy attachment with a coloured stripe
  - reaction pills and emoji shortcodes
  - the generated chart image in `#design`
  - a day separator between yesterday and today
  - edited and thread-reply markers

**Driftwood OSS**, an open-source community, with a light theme (Catppuccin
Latte) so that switching workspace also shows per-workspace themes.
- Channels: `#announcements`, `#contributors`, `#help`, plus one DM.
- About 6 people.
- Starts with unread messages, so its rail badge is lit.

**Avatars** are generated in code: the user's initials on a colour derived
from their ID, drawn with the half-block renderer. There are no image files
and no licensing questions. **Inline images:** a bar chart generated in code
with `image/draw`, in `#design`; and, added after the first recordings, KC
Green's "This is fine" panels, which Priya posts live in the hero. That one is
an embedded JPEG used without a licence, at the maintainer's choice; see
`internal/demo/assets/NOTICE.md`.

## Director

The fake services report user actions to the director: channel opened,
thread opened, message sent (with channel and thread), reaction added,
workspace switched. A scenario is a list of rules:

```
when <action matcher>, [once], after <delay>: send <sequence of timed msgs>
```

Sequences use the same messages as `cmd/slk`'s WebSocket handler
(`UserTypingMsg`, `NewMessageMsg`, `ReactionAddedMsg`, `ReadStateChangedMsg`,
and so on), and the director records each change in World so that later
fetches stay consistent. Rules triggered by user actions keep recordings in
step with their tape even when rendering speed varies. The one exception is a
single start-time rule in `hero`, which sends a new message to Driftwood OSS so
its rail badge lights up.

`hero` rules:
1. First time `#engineering` is opened: after 1.5s Priya starts typing for
   about 2s, then her message arrives with the "This is fine" image, then
   about 1s later Sam adds a 😂 reaction to it.
2. The user sends a message anywhere: after about 1s a teammate starts typing,
   then replies.
3. Once, a few seconds after start: a new message arrives in Driftwood OSS.

The other scenarios reuse the same mechanism with rules suited to each clip.
The director takes an injectable clock and a `send func(tea.Msg)`. It stops
when its context is cancelled, which `runDemo()` does when the program exits.

## Failure handling

- An unknown scenario prints the valid names and exits with a non-zero status.
- Actions the demo does not support show the App's normal error toast. They
  never fall through to a real network or disk operation.
- The director stops on shutdown with no stray goroutines. Tests run it under
  `-race`.

## Testing

Plain `testing.T`, white-box.

- **Fixtures:** every message author exists in its workspace, every thread
  parent has replies, every channel belongs to a workspace, IDs are unique,
  and every scenario references a channel or user that exists.
- **Services:** sending adds a message, reactions toggle, unread state clears
  when a channel is marked read, a workspace switch returns that workspace's
  channels, and the unsupported methods return errors rather than panicking.
- **Director:** with a fake clock and a recording `send`, each `hero` rule
  fires its messages in order and with the right delays, `once` rules fire
  only once, and cancellation stops pending sequences.
- **Imports:** `internal/demo` does not import the forbidden packages.
- **Smoke test (`cmd/slk`):** build the demo App, deliver the workspace-ready
  messages, render one frame at 120×40, and check that the workspace name, a
  channel name and a message body appear. This catches demo mode breaking when
  a new service is added to the App. The function that builds the demo App
  takes no config, token store or cache handle, so it cannot reach them;
  the no-real-data guarantee comes from that signature, not from a runtime
  check.
- GIFs are not generated in CI, because VHS needs ttyd and Chromium.
  `make demo-gifs` is run by hand.

## Tooling

- `flake.nix`: add `vhs` to the dev shell.
- `Makefile`: `demo-gifs` builds slk and runs `vhs` on every tape in
  `docs/assets/demo/`.
- README: replace the screenshot with `docs/assets/demo/hero.gif`.

## Out of scope

- Documenting demo mode for users.
- A fake Slack server, or exercising `cmd/slk`'s fetch and cache pipeline.
- Kitty graphics in recordings (VHS captures sixel only).
- Workspace-wide search, the Activity view and file upload inside the demo.
- Generating GIFs in CI.
