package main

import (
	"os"
	"time"

	"github.com/gammons/slk/internal/debuglog"
	imgpkg "github.com/gammons/slk/internal/image"
	"golang.org/x/term"
)

// withRawTerminal puts stdin in raw mode, runs fn, and restores on the
// way out (deferred, so a panicking probe cannot leave the terminal
// raw). Returns false without running fn when raw mode can't be
// entered — the caller learned nothing. probeName labels the debug log
// lines. Every interactive escape probe goes through here so the
// enter/restore handling can't drift between probes.
func withRawTerminal(probeName string, fn func()) bool {
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		debuglog.ImgRender("%s skipped: cannot enter raw mode: %v", probeName, err)
		return false
	}
	defer func() {
		if rerr := term.Restore(int(os.Stdin.Fd()), state); rerr != nil {
			debuglog.ImgRender("term restore after %s: %v", probeName, rerr)
		}
	}()
	fn()
	return true
}

// probeKittySupport interrogates the real terminal for working kitty
// graphics: PNG transmit first, then raw RGBA when the terminal
// answered the PNG with an explicit rejection — a graphics
// implementation without a PNG decoder (herdr's embedded
// libghostty-vt) rejects f=100 but decodes raw pixels natively. An
// RGBA-only ack flips the renderer to raw uploads for the whole
// session (imgpkg.SetKittyUploadRGBA). probed is false when raw mode
// couldn't be entered and nothing was learned; ok is meaningful only
// when probed. Must run before bubbletea takes over stdin.
func probeKittySupport() (ok, probed bool) {
	probed = withRawTerminal("kitty probe", func() {
		var rejected bool
		ok, rejected = imgpkg.ProbeKittyGraphics(os.Stdout, os.Stdin, 200*time.Millisecond)
		if ok || !rejected {
			return
		}
		ok, _ = imgpkg.ProbeKittyRGBA(os.Stdout, os.Stdin, 200*time.Millisecond)
		if ok {
			imgpkg.SetKittyUploadRGBA()
		}
	})
	return ok, probed
}

// resolveImageCapabilities settles the image rendering protocol and the
// cell pixel metrics by combining env detection with the interactive
// escape probes below. cfgProto is the configured image_protocol value.
// Must run before bubbletea takes over the terminal.
func resolveImageCapabilities(cfgProto string) (proto imgpkg.Protocol, pxW, pxH int) {
	proto = imgpkg.Detect(imgpkg.CaptureEnv(), cfgProto)
	debuglog.ImgRender("image protocol detect: cfg=%q result=%s", cfgProto, proto)

	// noMuxTTY gates every optional escape probe below: never inside
	// tmux/zellij (tmux protocol policy lives in Detect's
	// client_termname path; zellij swallows the escapes, so each probe
	// would burn its full timeout for nothing), and only when stdin is
	// really the terminal.
	noMuxTTY := os.Getenv("TMUX") == "" && os.Getenv("ZELLIJ") == "" &&
		term.IsTerminal(int(os.Stdin.Fd()))
	// upgradeProbeAllowed is the single statement of when a 200ms
	// upgrade probe (kitty or sixel) may be spent from halfblock: auto
	// mode only — an explicit image_protocol stays what it says — on a
	// probeable terminal.
	upgradeProbeAllowed := imgpkg.IsAutoProtocol(cfgProto) && noMuxTTY

	// Optional: run kitty version probe if detected as kitty AND stdin is a TTY.
	// Must happen BEFORE bubbletea takes over the terminal.
	// kittyProbed records that the terminal already answered (or timed
	// out on) the kitty probe sequence this startup, so the upgrade
	// block below never re-runs the identical probes against a terminal
	// that just failed them — that would double the startup stall and
	// give a slow terminal's late round-one reply a second window to be
	// misread.
	kittyProbed := false
	if proto == imgpkg.ProtoKitty && term.IsTerminal(int(os.Stdin.Fd())) {
		ok, probed := probeKittySupport()
		kittyProbed = probed
		if probed && !ok {
			debuglog.ImgRender("kitty probe failed, downgrading to halfblock")
			proto = imgpkg.ProtoHalfBlock
		}
	}

	// Kitty upgrade probe: Detect only recognizes kitty-capable
	// terminals it can name from the env, so a mux that strips the
	// outer terminal's identity but passes graphics escapes through
	// verbatim (a herdr pane in front of Ghostty: TERM=xterm-256color,
	// no TERM_PROGRAM) lands on halfblock even though kitty graphics
	// would work. Ask the terminal directly — the probe upload gets an
	// explicit ;OK reply, so a hit is authoritative. Runs before the
	// sixel probe because kitty is the preferred protocol (sharper
	// scaling, and emoji-as-images requires it). A terminal with no
	// kitty support ignores the probe, costing the 200ms timeout once
	// at startup.
	if proto == imgpkg.ProtoHalfBlock && upgradeProbeAllowed && !kittyProbed {
		if ok, probed := probeKittySupport(); probed && ok {
			// A transmit ack alone doesn't prove the unicode-placeholder
			// placement the renderer depends on: WezTerm and iTerm2 ack
			// uploads but print the placeholder codepoints as literal
			// glyphs, and Detect's TERM_PROGRAM routing for them is
			// useless here — this path only runs when that identity was
			// stripped. Ask XTVERSION; a terminal naming itself as one
			// of those stays halfblock so the sixel probe below picks
			// the protocol that actually renders. No reply is no veto.
			var name string
			withRawTerminal("xtversion probe", func() {
				name = imgpkg.ProbeTerminalVersion(os.Stdout, os.Stdin, 200*time.Millisecond)
			})
			if imgpkg.LacksUnicodePlaceholders(name) {
				debuglog.ImgRender("kitty upgrade vetoed: %q acks transmits but lacks unicode placeholders", name)
			} else {
				debuglog.ImgRender("kitty upgrade probe succeeded, upgrading halfblock to kitty")
				proto = imgpkg.ProtoKitty
			}
		}
	}

	// Sixel capability probe (issue #116). Detect only knows the handful
	// of terminals it can name from $TERM / $TERM_PROGRAM, so plenty of
	// sixel-capable terminals (xterm -ti vt340, DomTerm, toyterm, foot
	// behind a generic TERM, …) land on halfblock and render the blocky
	// mosaic even though real pixels would work. Ask the terminal
	// directly via DA1 before settling for halfblock.
	//
	// Shares upgradeProbeAllowed with the kitty upgrade probe above —
	// one policy for when an upgrade probe may be spent.
	if proto == imgpkg.ProtoHalfBlock && upgradeProbeAllowed {
		withRawTerminal("sixel probe", func() {
			if imgpkg.ProbeSixel(os.Stdout, os.Stdin, 200*time.Millisecond) {
				debuglog.ImgRender("sixel probe succeeded, upgrading halfblock to sixel")
				proto = imgpkg.ProtoSixel
			}
		})
	}
	debuglog.ImgRender("image protocol: %s", proto)

	// Cell pixel metrics for sizing decisions.
	var measured bool
	pxW, pxH, measured = imgpkg.CellPixels(int(os.Stdout.Fd()))
	// A pty that strips TIOCGWINSZ pixel fields (docker -it, some muxes)
	// leaves the 8x16 fallback, and kitty/sixel then encode a raster
	// 2-4x smaller than a hidpi cell box — the terminal stretches it
	// back up and images render soft. Ask the terminal itself via
	// XTWINOPS (CSI 16t) before settling; halfblock needs no pixel
	// metrics, so don't spend the probe there, and noMuxTTY keeps it
	// out of tmux/zellij like every other escape probe (older tmux
	// ignores 16t, so the probe would stall its full timeout on every
	// startup and a late reply would leak into bubbletea as keys).
	if !measured && (proto == imgpkg.ProtoKitty || proto == imgpkg.ProtoSixel) &&
		noMuxTTY {
		withRawTerminal("cellpx probe", func() {
			if w, h, ok := imgpkg.ProbeCellPixels(os.Stdout, os.Stdin, 200*time.Millisecond); ok {
				pxW, pxH = w, h
			}
		})
	}
	debuglog.ImgRender("cell pixels: %dx%d", pxW, pxH)

	return proto, pxW, pxH
}
