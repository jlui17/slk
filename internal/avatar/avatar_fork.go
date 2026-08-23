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

// kittyAvatarTarget picks the fetcher decode target for a kitty avatar
// source. The fetcher's Target is an exact resize, upscaling included,
// so a source the URL already declares at or under the bound (the
// _24/_32/_48/_72 variants users.info supplies) decodes native instead
// of being inflated 16x to the bound. Anything larger or of unknown
// size gets the bound.
func kittyAvatarTarget(avatarURL string) image.Point {
	if n, ok := trailingSizeMarker(avatarURL); ok && n <= kittyAvatarDecodeSize {
		return image.Point{}
	}
	return image.Pt(kittyAvatarDecodeSize, kittyAvatarDecodeSize)
}

// trailingSizeMarker parses the pixel size from a slack-edge avatar
// URL's `_<n>.<ext>` suffix.
func trailingSizeMarker(u string) (int, bool) {
	dot := strings.LastIndex(u, ".")
	if dot < 0 {
		return 0, false
	}
	us := strings.LastIndex(u[:dot], "_")
	if us < 0 || us+1 == dot {
		return 0, false
	}
	n := 0
	for _, r := range u[us+1 : dot] {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
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
