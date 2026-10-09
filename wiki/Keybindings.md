# Keybindings

| Key | Mode | Action |
|---|---|---|
| `j` / `k` | Normal | Move down/up in channel list or messages |
| `h` / `l` | Normal | Switch focus between panels |
| `Tab` / `Shift+Tab` | Normal | Cycle focus |
| `Enter` | Normal (sidebar) | Open selected channel, or toggle a section header |
| `Space` | Normal (sidebar) | Toggle the selected section header (collapse/expand) |
| `Enter` | Normal (message) | Open thread |
| `i` | Normal | Enter insert mode |
| `Esc` | Insert / Command | Return to normal mode |
| `Enter` | Insert | Send message |
| `Shift+Enter` | Insert | Newline |
| `Ctrl+V` | Insert | Smart paste — image / file path / text (use `Ctrl+V`, not the terminal's `Ctrl+Shift+V`) |
| `Ctrl+U` | Insert | Delete to start of line (what kitty sends for `Cmd+Backspace`) |
| `Ctrl+E` | Insert | Move to end of line (what kitty sends for `Cmd+Right`) |
| `Ctrl+X` | Insert | Edit the draft in `$VISUAL` / `$EDITOR` (or `compose.editor`) |
| `Ctrl+O` | Insert (thread) | Toggle "also send to channel" for the next thread reply |
| `Alt+Enter` | Insert (thread) | Send thread reply and broadcast to channel (one-shot) |
| `Ctrl+U` / `Ctrl+D` | Normal | Half-page up / down |
| `Up` | Insert | Previous line; on the first line, jump to start of message |
| `Down` | Insert | Next line; on the last line, jump to end of message |
| `gg` / `G` | Normal | Jump to top / bottom |
| `/` | Normal | Search in channel (vim-style; searches cached history of the current channel) |
| `n` / `N` | Normal | Next / previous search match (wraps) |
| `a` / `A` | Normal | Jump to next / previous unread channel (wraps) |
| `Esc` | Normal (search active) | Clear active search |
| `Ctrl+f` | Any | Search workspace (Slack server-side; supports modifiers like `from:@user`, `in:#channel`, `before:YYYY-MM-DD`) |
| `Ctrl+b` | Any | Toggle sidebar |
| `Ctrl+]` | Any | Toggle thread panel |
| `t` | Normal (thread open) | Zoom the thread over the whole message area (press again to restore) |
| `Ctrl+t` / `Ctrl+p` | Any | Fuzzy channel finder |
| `:ws` | Normal | Workspace picker |
| `1`–`9` | Normal | Jump to workspace N |
| `r` | Normal (message) | Open reaction picker |
| `R` | Normal (message) | Quick-toggle existing reactions |
| `E` | Normal (message) | Edit your own message |
| `D` | Normal (message) | Delete your own message (with confirmation) |
| `U` | Normal (message) | Mark selected message and everything newer as unread |
| `S` | Normal (thread) | Save thread to markdown file (`~/.local/share/slk/exports/` or `$XDG_DATA_HOME/slk/exports/`) |
| `y` | Normal (message) | Copy message text |
| `Y` / `C` | Normal (message) | Copy message permalink |
| `c` | Normal (message) | Copy a fenced code block or a link from the message. A block copies as the code only, without the fence or language tag; a link copies as the URL only, without its label. Several blocks or links open a picker that lists them in message order, the links of a table and of the other blocks below the body after those of the body; a link inside a block is not listed on its own. Clicking the `copy` label on a block's top border copies that block |
| `o` | Normal (message) | Open link in message (Slack permalinks for the active workspace navigate in-app; other links open in the browser; multiple links open a picker). The links of a table and of the other blocks below the body are offered after those of the body; links in cards are not. A picker row starts with the list item or the table row its link sits in, as `<item or first cell> · <link text>`: `7. @dana` for a linked message in the seventh item of a list, with names shown as in the message |
| `O` | Normal (message) | Open a Slack permalink from the message in a new herdr tab running a second slk; only slk-openable links are offered. With several, a picker opens: `Enter` opens the cursor row in a focused tab, or `Space` marks rows (`a` marks or clears the rows shown) and `Enter` opens every marked link in its own background tab, in list order (requires running inside herdr; otherwise behaves exactly like `o`) |
| `/` | Link, copy and file picker | Filter the rows. While the filter is being typed every key is filter text (`q`, `a` and `Space` type characters): typed text keeps the rows that contain it, whatever the case, `Up`/`Down` or `Ctrl+p`/`Ctrl+n` move, `Enter` opens, `Tab` gives the keys back to the list with the filter kept (so `a` marks only the rows it shows), `Esc` clears the filter and a second `Esc` closes the picker. The filter reads a link's row name and link text, the channel of a Slack permalink, and the URL of a row that shows its URL; it does not read dates or message previews. A file row or a code-block row is filtered on its whole text. A picker with more rows than fit scrolls with the cursor and shows its position (`7–18 of 46`); under a filter the title counts the matches (`4 matches`, `1–14 of 15 matches`) |
| `z` | Normal (message) | Expand or collapse the message's long cards (a linked Slack message, a bot card). A card with six or more lines of text (image rows do not count) starts collapsed to three, under a `▸ N more lines · z to expand` row; `z` toggles every card of the selected message, in the messages pane and the thread pane. Clicking that row toggles too. Does nothing on a message with no long card |
| `.` | Normal (message) | Run one of the workspace's app shortcuts on the message (an app's "message shortcut", such as an Annotate action): a menu lists them, and the form the app opens is filled in slk. In the form, `tab` / `shift+tab` move between fields, `Enter` adds a line in a multi-line field, `j` / `k` or the arrows and `Space` / `Enter` pick an option, `ctrl+s` submits and `esc` cancels. A form with parts slk can't draw (date pickers, buttons, multi-selects, ...) offers `o` to open the message in the browser instead |
| `Enter` | Normal (sidebar, on an app `▣`) | Open the app the way a channel opens: its Home tab when it has one (the blocks the app published, under a `Home   Messages` tab row), else its messages. An app you starred in Slack sits in Starred |
| `m` | Normal (app open) | Switch between the app's Home and Messages tabs; an app with only a Home tab shows no Messages tab, and `m` does nothing |
| `j` / `k` | App Home | Move between the parts you can press: a row with a button (the `▌` bar marks all its lines), and each button or menu of a row of controls. Past the first or last one they scroll |
| `Enter` | App Home | Press it, as a click does in Slack: a link button opens its link (a Slack permalink navigates in slk); any other button tells the app, after its confirm box if it has one (`Enter` confirms, `Esc` cancels), and a form the app opens in answer is filled as an app shortcut's form is (see `.`; `o` on one slk can't fill opens the app in the browser); a menu opens its options (`j` / `k` move, `Enter` chooses, `Esc` closes). A part slk can't use says it needs Slack |
| `o` / `O` | App Home | On a link button, open its link here / in a new herdr tab (outside herdr, in the browser). On any other button, press it, show `Waiting for <App>…` in its place, and open the one link the app's redraw adds, here / in a new herdr tab; with none after 10 s, `! <App> did not answer`, and with several, `! <App> added N links: pick one on the Home`. The other keys keep working while it waits |
| `PgUp` / `PgDn`, `Ctrl+u` / `Ctrl+d`, `g` / `G` | App Home | Scroll the Home a page / half a page up or down; `g` goes to the top and its first button, `G` to the end and its last |
| `Esc` | App Home | Close a menu or confirm box, or stop waiting for the app's redraw after `o` / `O`; else move to the sidebar |
| Click | App Home | A link in the text opens; a click on a row you can press moves there |
| `d` | Normal (message) | Download file attachment (multiple files open a picker) |
| `v` | Normal (message) | Open full-screen image preview |
| `Esc` / `q` | Preview | Close preview |
| `Enter` | Preview | Open in system image viewer |
| `h` / `←` | Preview | Previous image (when message has multiple) |
| `l` / `→` | Preview | Next image (when message has multiple) |
| Click | Any (on image) | Open full-screen preview |
| Click | Normal (on an `@person` mention) | Open the person's profile card: name, display name and title, Active or Away, their local time, their status. In the card, `m` opens the DM with them, `o` opens their profile in the browser, `Esc`, `q` or a click outside closes it |
| `:agent` | Normal (thread open) | Mark the open thread as an agent thread, for one whose root neither mentions nor was written by a bot: slk takes the first bot mentioned in or writing the root, then the replies, and tracks the thread in the herdr sidebar and tab as it would a detected one, now and whenever the thread opens again. On a marked thread, removes the mark; the thread stays tracked until another agent thread opens |
| `:retitle` | Normal (thread open) | Re-derive the herdr tab label from the open thread, agent thread or not: `tab_name_model` reads the whole thread and judges the task id and names the work the thread is about (`tab_name_hints` steers the naming). It renames the tab even when something else set the current label; the automatic label never does. On a thread that isn't the tracked agent thread it only names the tab. The answer lands even after the thread is closed or another is opened; when `:retitle` runs again before an answer comes back, only the newest one lands |
| `Ctrl+r` / `:reload` | Normal | Reload: force every workspace's websocket to reconnect and catch up, and refetch the open thread panel (the Slack app's `Cmd+R` analog) |
| `Ctrl+y` | Any | Switch theme |
| `Ctrl+s` | Any | Set status (Active / Away / DND snooze) |
| `q` | Normal | Quit (with confirmation) |
| `Q` | Normal | Quit immediately |
| `Ctrl+c` | Any | Quit (with confirmation) |

Custom keybinding overrides are on the roadmap — see [[Tradeoffs and Non-Goals|Tradeoffs-and-Non-Goals]].
