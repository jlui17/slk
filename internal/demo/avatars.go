package demo

import (
	"fmt"
	"hash/fnv"
	"image/color"
	"strings"
	"unicode"
)

// Avatar cell footprint. These mirror avatar.AvatarCols/AvatarRows, which
// this package cannot import (internal/avatar fetches over the network);
// avatars_test.go pins them equal.
const (
	avatarCols = 4
	avatarRows = 2
)

var avatarPalette = []color.RGBA{
	{0xe0, 0x6c, 0x75, 0xff}, {0xd1, 0x9a, 0x66, 0xff}, {0x98, 0xc3, 0x79, 0xff}, {0x56, 0xb6, 0xc2, 0xff},
	{0x61, 0xaf, 0xef, 0xff}, {0xc6, 0x78, 0xdd, 0xff}, {0xbe, 0x50, 0x46, 0xff}, {0x2b, 0x8a, 0x6e, 0xff},
}

// renderAvatar draws a user's initials on a colour picked from their ID:
// a full-colour top row carrying the initials, and a half-block row
// beneath it that finishes the tile. It is SGR text rather than an image
// because a 4x2-cell half-block image is only 4x4 pixels, too small to
// hold letters.
func renderAvatar(userID, name string) string {
	h := fnv.New32a()
	h.Write([]byte(userID))
	c := avatarPalette[h.Sum32()%uint32(len(avatarPalette))]
	top := fmt.Sprintf("\x1b[1;38;2;255;255;255;48;2;%d;%d;%dm %s \x1b[0m", c.R, c.G, c.B, initials(name))
	bottom := fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m", c.R, c.G, c.B, strings.Repeat("▀", avatarCols))
	return top + "\n" + bottom
}

// initials returns exactly two cells: the first letters of the first two
// words, or the first two letters of a single word.
func initials(name string) string {
	words := strings.Fields(name)
	var rs []rune
	switch len(words) {
	case 0:
		return "??"
	case 1:
		rs = []rune(words[0])
	default:
		rs = []rune{[]rune(words[0])[0], []rune(words[1])[0]}
	}
	if len(rs) > 2 {
		rs = rs[:2]
	}
	for len(rs) < 2 {
		rs = append(rs, ' ')
	}
	for i := range rs {
		rs[i] = unicode.ToUpper(rs[i])
	}
	return string(rs)
}
