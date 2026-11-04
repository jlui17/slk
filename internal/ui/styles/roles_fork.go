package styles

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/gammons/slk/internal/core"
)

// ThemeRoles lets ThemeColors carry the optional role colors.
type ThemeRoles = core.ThemeRoles

// The role colors. Apply resolves each one; before the first Apply they
// hold the fallbacks of the package defaults.
var (
	NameColor        color.Color = Primary
	TextBody         color.Color = TextPrimary
	CodeText         color.Color = Warning
	CodeBackground   color.Color = Surface
	FocusBorderColor color.Color = Primary
	NewMessages      color.Color = Error
	Online           color.Color = Accent
	TimeColor        color.Color = TextMuted
)

// applyRoles resolves the role colors after Apply has set the base
// colors: a config override wins, then the theme's own role, then the
// base color slk used for that job before the role existed.
func applyRoles(theme, overrides ThemeRoles) {
	pick := func(override, own string, fallback color.Color) color.Color {
		switch {
		case override != "":
			return lipgloss.Color(override)
		case own != "":
			return lipgloss.Color(own)
		}
		return fallback
	}
	NameColor = pick(overrides.Name, theme.Name, Primary)
	TextBody = pick(overrides.TextBody, theme.TextBody, TextPrimary)
	CodeText = pick(overrides.Code, theme.Code, Warning)
	CodeBackground = pick(overrides.CodeBackground, theme.CodeBackground, Surface)
	FocusBorderColor = pick(overrides.FocusBorder, theme.FocusBorder, Primary)
	NewMessages = pick(overrides.NewMessages, theme.NewMessages, Error)
	Online = pick(overrides.Online, theme.Online, Accent)
	TimeColor = pick(overrides.Time, theme.Time, TextMuted)
}
