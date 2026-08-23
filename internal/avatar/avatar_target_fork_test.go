package avatar

import (
	"image"
	"testing"
)

func TestKittyAvatarTarget_SmallSizedVariantsDecodeNative(t *testing.T) {
	cases := []struct {
		url  string
		want image.Point
	}{
		{"https://avatars.slack-edge.com/2024/U1_abc_24.png", image.Point{}},
		{"https://avatars.slack-edge.com/2024/U1_abc_32.png", image.Point{}},
		{"https://avatars.slack-edge.com/2024/U1_abc_72.jpg", image.Point{}},
		{"https://avatars.slack-edge.com/2024/U1_abc_128.png", image.Point{}},
		{"https://avatars.slack-edge.com/2024/U1_abc_192.png", image.Pt(128, 128)},
		{"https://avatars.slack-edge.com/2024/U1_abc_1024.png", image.Pt(128, 128)},
		{"https://avatars.slack-edge.com/2024/U1_abc_original.png", image.Pt(128, 128)},
		{"https://secure.gravatar.com/avatar/abc.jpg", image.Pt(128, 128)},
	}
	for _, c := range cases {
		if got := kittyAvatarTarget(c.url); got != c.want {
			t.Errorf("kittyAvatarTarget(%q) = %v; want %v", c.url, got, c.want)
		}
	}
}
