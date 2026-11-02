package chrome

import "unicode/utf8"

// EditFilter is key typed into a dialog's filter text: backspace drops
// the last rune, space adds a space, and a key that is one rune adds
// it. ok is false for every other key, which the dialog handles itself.
func EditFilter(filter, key string) (edited string, ok bool) {
	switch {
	case key == "backspace":
		r := []rune(filter)
		return string(r[:max(len(r)-1, 0)]), true
	case key == "space":
		return filter + " ", true
	case utf8.RuneCountInString(key) == 1:
		return filter + key, true
	}
	return filter, false
}
