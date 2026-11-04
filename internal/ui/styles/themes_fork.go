package styles

// Fork-added built-in themes.
func init() {
	// Coffee: warm and dark. Every text color is at least 4.5:1 on the
	// background it sits on (TestCoffeeTextContrast in
	// roles_fork_test.go). Color that carries meaning is blue against
	// orange: links, mentions and online are blue; the new-messages
	// line and errors are orange. The sidebar is a shade darker than
	// the palette's #26211B so the channels panel stands apart from the
	// messages (TestChannelsPanelContrast).
	builtinThemes["coffee"] = struct {
		Name   string
		Colors ThemeColors
	}{"Coffee", ThemeColors{
		Primary: "#93C2EE", Accent: "#E8AB55", Warning: "#E8AB55", Error: "#D97B57",
		Background: "#2F2922", Surface: "#3A332B", SurfaceDark: "#28231D",
		Text: "#F6F0E6", TextMuted: "#A89E8E", Border: "#75695B",
		SidebarBackground: "#211C17", RailBackground: "#110E0C",
		ThemeRoles: ThemeRoles{
			Name: "#F6F0E6", TextBody: "#D2C8B6", Time: "#BDB3A3",
			NewMessages: "#D97B57", Online: "#58AAF1",
			FocusBorder: "#977A4F",
			Code:        "#D6C3A0", CodeBackground: "#4A4036",
		},
	}}
}
