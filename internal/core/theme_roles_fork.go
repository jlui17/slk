package core

// ThemeRoles are the theme colors for one job each. Every one is
// optional: an empty role takes the color slk used for that job before
// the role existed (styles.applyRoles), so a theme that sets none of
// them renders as it always did. A built-in theme, a theme file and the
// [theme] overrides in config all accept them.
type ThemeRoles struct {
	Name           string `toml:"name"`            // author names; else primary
	TextBody       string `toml:"text_body"`       // message body text; else text
	Code           string `toml:"code"`            // inline and block code text; else warning
	CodeBackground string `toml:"code_background"` // inline code and code block fill; else surface
	FocusBorder    string `toml:"focus_border"`    // the focused pane's border; else primary
	NewMessages    string `toml:"new_messages"`    // the new-messages line; else error
	Online         string `toml:"online"`          // online presence and Connected; else accent
	Time           string `toml:"time"`            // the time margin; else text_muted
}
