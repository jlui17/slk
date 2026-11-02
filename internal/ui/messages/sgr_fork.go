package messages

import "strings"

// writeSubstitutedSGRParams writes the SGR parameter list params to b
// with every background token equal to from replaced by to. A token is
// an extended-colour group (38;5;N, 38;2;R;G;B, 48;5;N, 48;2;R;G;B),
// so a "40" argument of "38;5" is never taken for a background, or a
// single parameter, a background one when it is a basic 16-colour
// background. It walks params in place rather than splitting and
// joining it: the selected variant of every message runs every SGR of
// the message through here on each cache build.
func writeSubstitutedSGRParams(b *strings.Builder, params, from, to string) {
	for i := 0; ; {
		end := sgrFieldEnd(params, i)
		field := params[i:end]
		isBg := isBasicBgParam(field)
		if field == "38" || field == "48" {
			if groupEnd, ok := sgrColorGroupEnd(params, end); ok {
				end, isBg = groupEnd, field == "48"
			}
		}
		if tok := params[i:end]; isBg && tok == from {
			b.WriteString(to)
		} else {
			b.WriteString(tok)
		}
		if end == len(params) {
			return
		}
		b.WriteByte(';')
		i = end + 1
	}
}

// sgrColorGroupEnd returns the end of the extended-colour arguments
// that follow a "38" or "48" field ending at end: "5;N" or "2;R;G;B".
// ok is false when the next field is neither or the group is cut short.
func sgrColorGroupEnd(params string, end int) (int, bool) {
	if end == len(params) {
		return 0, false
	}
	modeEnd := sgrFieldEnd(params, end+1)
	var args int
	switch params[end+1 : modeEnd] {
	case "5":
		args = 1
	case "2":
		args = 3
	default:
		return 0, false
	}
	end = modeEnd
	for range args {
		if end == len(params) {
			return 0, false
		}
		end = sgrFieldEnd(params, end+1)
	}
	return end, true
}

// sgrFieldEnd returns the index of the ';' that ends the field starting
// at i, or len(params) for the last field.
func sgrFieldEnd(params string, i int) int {
	if k := strings.IndexByte(params[i:], ';'); k >= 0 {
		return i + k
	}
	return len(params)
}
