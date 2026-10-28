# Shared by every demo tape via `Source`. Not rendered on its own.
#
# Sixel: slk draws the demo's images (the hero's meme, the #design chart)
# as sixel, and VHS captures them. That needs the VHS from
# charmbracelet/vhs#783, which `nix develop` provides (see flake.nix).
# `Set Sixel true` MUST stay above every `Env` line: VHS silently ignores
# a `Set` that comes after an action, and `Env` counts as one.
Set Sixel true
Set Shell "bash"
Set FontSize 18
Set Width 1600
Set Height 900
Set Padding 12
Set Framerate 30
Set TypingSpeed 90ms
Env COLORTERM "truecolor"
Env SLK_DEMO_IMAGES "sixel"
# ttyd reports no pixel size, so tell slk its cells are 11x23 px: xterm.js
# draws FontSize 18 cells at about 11.65x23.4. Rounded down so images never
# spill past the cells slk reserves. Recompute if FontSize changes.
Env COLORTERM_CELL_WIDTH "11"
Env COLORTERM_CELL_HEIGHT "23"
