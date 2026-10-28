package confirmprompt

import "charm.land/bubbles/v2/key"

// KeyMap is the prompt's key bindings. Confirm and Cancel are the yes/no
// preset's buttons. Press, Next and Prev act only in a Choice that does not
// cancel on other keys; Close always closes without pressing.
type KeyMap struct {
	Confirm key.Binding
	Cancel  key.Binding
	Press   key.Binding
	Close   key.Binding
	Next    key.Binding
	Prev    key.Binding
}

// DefaultKeyMap returns the default bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Confirm: key.NewBinding(key.WithKeys("y", "Y", "enter"), key.WithHelp("y", "confirm")),
		Cancel:  key.NewBinding(key.WithKeys("n", "N", "esc"), key.WithHelp("n/Esc", "cancel")),
		Press:   key.NewBinding(key.WithKeys("enter")),
		Close:   key.NewBinding(key.WithKeys("esc")),
		Next:    key.NewBinding(key.WithKeys("l", "right", "tab")),
		Prev:    key.NewBinding(key.WithKeys("h", "left", "shift+tab")),
	}
}
