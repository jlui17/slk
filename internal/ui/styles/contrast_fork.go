package styles

import (
	"image/color"
	"math"

	"charm.land/lipgloss/v2"
)

// luminance is c's WCAG 2 relative luminance.
func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		x := float64(v>>8) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// contrast is the WCAG 2 contrast ratio between a and b, from 1 to 21.
func contrast(a, b color.Color) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// textOn is the text color for a pill filled with bg: white, or the
// theme's Background when that contrasts more with bg (a light pill
// color, as in coffee). A tie keeps white.
func textOn(bg color.Color) color.Color {
	white := lipgloss.Color("#FFFFFF")
	if contrast(Background, bg) > contrast(white, bg) {
		return Background
	}
	return white
}
