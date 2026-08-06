package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// The paint layer: the redesign fills every cell with a theme surface color
// (list = Bg, preview/dialogs = BgPane, bars = Bg2) so all three themes —
// including the light Paperback — render identically regardless of the
// terminal's own colors. lipgloss styles close every span with a full SGR
// reset, which would punch holes back to the terminal background; paintLine
// re-asserts the surface color after each reset instead.

// bgSeq returns the raw SGR sequence that sets background color c under the
// active color profile, or "" when the profile strips color (tests, dumb
// terminals) — callers fall back to plain padding so output stays byte-clean.
func bgSeq(c string) string {
	if c == "" {
		return ""
	}
	col := lipgloss.ColorProfile().Color(c)
	if col == nil {
		return ""
	}
	seq := col.Sequence(true)
	if seq == "" {
		return ""
	}
	return "\x1b[" + seq + "m"
}

// paintLine makes line exactly w display cells over background bg: opens the
// background up front, re-asserts it after every SGR reset, and pads the tail
// with bg-filled cells (a reset first, so a span left open by glamour can't
// tint the padding). Segments that set their own background (the selected
// row's Sel, sync pills, code chips) win as usual — their explicit bg comes
// after any reset. Under a color-stripping profile it degrades to plain
// clamp-and-pad.
func paintLine(line string, w int, bg string) string {
	seq := bgSeq(bg)
	if seq == "" {
		if lw := ansi.StringWidth(line); lw > w {
			return ansi.Truncate(line, w, "")
		} else {
			return line + spaces(w-lw)
		}
	}
	line = strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+seq)
	line = strings.ReplaceAll(line, "\x1b[m", "\x1b[m"+seq)
	line = seq + line
	if lw := ansi.StringWidth(line); lw > w {
		return ansi.Truncate(line, w, "") + ansi.ResetStyle
	} else {
		return line + ansi.ResetStyle + seq + spaces(w-lw) + ansi.ResetStyle
	}
}

// paintBlock paints every line of a multi-line block to exactly w cells over bg.
func paintBlock(block string, w int, bg string) string {
	lines := strings.Split(block, "\n")
	for i, ln := range lines {
		lines[i] = paintLine(ln, w, bg)
	}
	return strings.Join(lines, "\n")
}

// --- scrim ---

// sgrSeq matches one SGR escape sequence.
var sgrSeq = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// blendHex mixes hex color c toward base by amount (0 keeps c, 1 becomes
// base), returning c unchanged when either fails to parse as #rrggbb.
func blendHex(c, base string, amount float64) string {
	cr, cg, cb, ok1 := hexRGB(c)
	br, bg2, bb, ok2 := hexRGB(base)
	if !ok1 || !ok2 {
		return c
	}
	mix := func(a, b int) int { return int(float64(a)*(1-amount) + float64(b)*amount + 0.5) }
	return fmt.Sprintf("#%02x%02x%02x", mix(cr, br), mix(cg, bg2), mix(cb, bb))
}

func hexRGB(c string) (r, g, b int, ok bool) {
	if len(c) != 7 || c[0] != '#' {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(c[1:], 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(v >> 16), int(v >> 8 & 0xff), int(v & 0xff), true
}

// scrimAmount is how far the backdrop's colors slide toward the surface color
// while a dialog floats — the terminal approximation of the prototype's
// alpha scrim. Hues stay recognizable, contrast recedes.
const scrimAmount = 0.55

// dimFrame is the scrim: every truecolor foreground and background in the
// already-painted frame is blended toward the theme surface, so the page
// visibly recedes behind a floating dialog while keeping its hues. Bare
// resets get the blended default foreground appended, so unstyled text (most
// glamour body copy) dims too. Only truecolor output is rewritten — on
// downsampled or stripped profiles the frame passes through, and the opaque
// dialog panel alone separates overlay from page.
func (m Model) dimFrame(frame string) string {
	if lipgloss.ColorProfile() != termenv.TrueColor {
		return frame
	}
	t := m.theme()
	baseR, baseG, baseB, ok := hexRGB(t.Bg)
	if !ok {
		return frame
	}
	mix := func(a, b int) int { return int(float64(a)*(1-scrimAmount) + float64(b)*scrimAmount + 0.5) }
	blendParams := func(kind string, r, g, b int) string {
		return fmt.Sprintf("%s;2;%d;%d;%d", kind, mix(r, baseR), mix(g, baseG), mix(b, baseB))
	}
	defFgR, defFgG, defFgB, _ := hexRGB(t.Fg)
	defFg := blendParams("38", defFgR, defFgG, defFgB)

	return sgrSeq.ReplaceAllStringFunc(frame, func(seq string) string {
		ps := strings.Split(strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b["), "m"), ";")
		out := make([]string, 0, len(ps)+1)
		for i := 0; i < len(ps); i++ {
			p := ps[i]
			// A truecolor fg/bg run: 38;2;r;g;b or 48;2;r;g;b → blend toward Bg.
			if (p == "38" || p == "48") && i+4 < len(ps) && ps[i+1] == "2" {
				r, _ := strconv.Atoi(ps[i+2])
				g, _ := strconv.Atoi(ps[i+3])
				b, _ := strconv.Atoi(ps[i+4])
				out = append(out, blendParams(p, r, g, b))
				i += 4
				continue
			}
			out = append(out, p)
			// A reset restores the terminal default fg; follow it with the blended
			// default so unstyled backdrop text dims like everything else.
			if p == "0" || p == "" {
				out = append(out, defFg)
			}
		}
		return "\x1b[" + strings.Join(out, ";") + "m"
	})
}
