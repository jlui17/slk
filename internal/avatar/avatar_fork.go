package avatar

import (
	"image"
	"strings"
)

// kittyAvatarDecodeSize bounds the fetcher-side decode of kitty avatar
// sources, which is retained for the process lifetime in the fetcher's
// decoded memo and the kitty registry. The renderer stretches the
// source across AvatarCols x AvatarRows cells, at most ~20x40 device
// pixels per cell on hidpi terminals (~80x80 px total); 128 keeps
// >=1.5x headroom over that, and becomes wrong only if cells exceed
// ~32x64 device pixels. Unbounded, an _original source retains
// 0.4-4 MB per distinct author.
const kittyAvatarDecodeSize = 128

func kittyAvatarTarget() image.Point {
	return image.Pt(kittyAvatarDecodeSize, kittyAvatarDecodeSize)
}

// sizedAvatarURL rewrites a slack-edge avatar URL ending in
// _original.<ext> to the _192 variant, a size Slack serves for every
// workspace avatar and that stays sharp above kittyAvatarTarget. Any
// other URL passes through unchanged.
func sizedAvatarURL(u string) string {
	if !strings.HasPrefix(u, "https://avatars.slack-edge.com/") {
		return u
	}
	i := strings.LastIndex(u, "_original.")
	if i < 0 {
		return u
	}
	ext := u[i+len("_original."):]
	if ext == "" || strings.ContainsAny(ext, "./?#") {
		return u
	}
	return u[:i] + "_192." + ext
}
