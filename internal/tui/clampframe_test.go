package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// scanBackground walks the SGR sequences in a line and reports whether any
// background color was set (seen) and whether one is still active at the end of
// the line (openEOL — such a line bleeds its background into the next terminal
// row). Foreground extended-color params (38;2;r;g;b / 38;5;n) are skipped
// positionally so an r/g/b of 40-48 isn't misread as a background code.
// (sgr is defined in dialog_test.go.)
func scanBackground(line string) (seen, openEOL bool) {
	bg := false
	for _, seq := range sgr.FindAllString(line, -1) {
		ps := strings.Split(strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b["), "m"), ";")
		for i := 0; i < len(ps); i++ {
			switch p := ps[i]; {
			case p == "0" || p == "" || p == "49":
				bg = false
			case p == "48":
				bg, seen = true, true
				if i+1 < len(ps) && ps[i+1] == "2" {
					i += 4
				} else if i+1 < len(ps) && ps[i+1] == "5" {
					i += 2
				}
			case len(p) == 2 && p[0] == '4' && p[1] >= '0' && p[1] <= '7':
				bg, seen = true, true
			case p == "38":
				if i+1 < len(ps) && ps[i+1] == "2" {
					i += 4
				} else if i+1 < len(ps) && ps[i+1] == "5" {
					i += 2
				}
			}
		}
	}
	return seen, bg
}

// TestClampFrameClosesOpenBackground guards the preview-bleed fix: a frame line
// that ends with an unclosed background (as glamour inline-code chips do at a
// wrapped line's end) must not leave that background open after clamping — else
// the padding and the next row inherit it as a gray ghost band.
func TestClampFrameClosesOpenBackground(t *testing.T) {
	const w = 24
	for _, tc := range []struct {
		name, line string
	}{
		// bg 236 opened via a 256-color code, never reset; content shorter than w (pad path).
		{"pad", "\x1b[48;5;236m\x1b[38;5;203m code-chip \x1b[39m"},
		// same but with a trailing truecolor bg left open.
		{"pad-truecolor", "\x1b[48;2;60;56;54mchip"},
		// content wider than w so the truncate path runs (must also stay closed).
		{"truncate", "plain plain \x1b[48;5;236mchip-that-runs-well-past-the-limit"},
	} {
		out := clampFrame(tc.line, w)
		if _, open := scanBackground(out); open {
			t.Errorf("%s: clampFrame left a background open at EOL (would bleed into the next row): %q",
				tc.name, strings.ReplaceAll(out, "\x1b", "\\e"))
		}
		if got := ansi.StringWidth(out); got != w {
			t.Errorf("%s: clamped width = %d, want %d", tc.name, got, w)
		}
	}
}
