package image

import "strings"

// LacksUnicodePlaceholders reports whether an XTVERSION reply names a
// terminal known to ack kitty graphics transmits without rendering the
// unicode-placeholder placement slk's kitty renderer depends on — the
// same terminals Detect routes to sixel when TERM_PROGRAM survives
// (see the iTerm.app / WezTerm case there). An empty or unrecognized
// reply reports false: no identity is no veto.
func LacksUnicodePlaceholders(xtversion string) bool {
	v := strings.ToLower(xtversion)
	return strings.Contains(v, "wezterm") || strings.Contains(v, "iterm")
}
