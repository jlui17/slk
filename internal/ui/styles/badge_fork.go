package styles

import "charm.land/lipgloss/v2"

// InAppBadge is the style of the [slk] mark on a link slk opens itself,
// in the link picker and the pins and bookmarks dialog.
func InAppBadge() lipgloss.Style {
	return lipgloss.NewStyle().Background(Background).Foreground(Accent)
}
