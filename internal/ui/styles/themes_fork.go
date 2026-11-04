package styles

// Fork-added built-in themes.
func init() {
	// Coffee: warm and dark. Every text color is at least 4.5:1 on every
	// background it is drawn on, the selected-row tints and the code fill
	// included (TestCoffeeTextContrast in roles_fork_test.go). Color that
	// carries meaning is blue against orange: links, mentions and online
	// are blue; the new-messages line and errors are orange. Code is set
	// apart by a cool slate fill against the warm base, at body-text
	// lightness, not by a brighter or greener text color. The sidebar is
	// a shade darker than the palette's #26211B so the channels panel
	// stands apart from the messages (TestChannelsPanelContrast).
	builtinThemes["coffee"] = struct {
		Name   string
		Colors ThemeColors
	}{"Coffee", ThemeColors{
		Primary: "#93C2EE", Accent: "#E8AB55", Warning: "#E8AB55", Error: "#E4A187",
		Background: "#2F2922", Surface: "#3A332B", SurfaceDark: "#28231D",
		Text: "#F6F0E6", TextMuted: "#BCAF98", Border: "#857561",
		SidebarBackground: "#211C17", RailBackground: "#110E0C",
		ThemeRoles: ThemeRoles{
			Name: "#E9C285", TextBody: "#D2C8B6", Time: "#BFAE95",
			NewMessages: "#D97B57", Online: "#58AAF1",
			FocusBorder: "#B88A4E",
			Code:        "#BCCCD6", CodeBackground: "#363E48",
		},
	}}
}
