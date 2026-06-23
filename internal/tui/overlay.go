package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// placeOverlay composites fg on top of bg starting at cell (x, y), preserving
// the ANSI styling of the surrounding background. Each overlaid row is rebuilt
// as [bg left of the box] + [box line] + [bg right of the box]; the right cut
// re-emits the background's active SGR so its colors survive the splice. This is
// what lets the command palette float over the panes like a VS Code dialog
// instead of replacing them.
func placeOverlay(x, y int, fg, bg string) string {
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	fgLines := strings.Split(fg, "\n")
	bgLines := strings.Split(bg, "\n")

	var b strings.Builder
	for i, bgLine := range bgLines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if i < y || i >= y+len(fgLines) {
			b.WriteString(bgLine)
			continue
		}
		fgLine := fgLines[i-y]
		fgW := ansi.StringWidth(fgLine)

		left := ansi.Truncate(bgLine, x, "")
		if lw := ansi.StringWidth(left); lw < x {
			left += strings.Repeat(" ", x-lw)
		}
		b.WriteString(left)
		b.WriteString(fgLine)
		b.WriteString(cutLeft(bgLine, x+fgW))
	}
	return b.String()
}

// cutLeft drops the first cutWidth display cells from s, keeping the remainder's
// ANSI styling intact by re-emitting the SGR run active at the cut point. A
// double-width rune straddling the boundary is replaced by spaces for its kept
// cells so columns stay aligned.
func cutLeft(s string, cutWidth int) string {
	if cutWidth <= 0 {
		return s
	}
	var (
		pos    int
		inEsc  bool
		seq    strings.Builder // escape sequence currently being read
		active strings.Builder // SGR runs active since the last reset
		out    strings.Builder
		open   bool // started emitting kept content?
	)
	for _, r := range s {
		switch {
		case r == 0x1b:
			inEsc = true
			seq.Reset()
			seq.WriteRune(r)
			continue
		case inEsc:
			seq.WriteRune(r)
			// A CSI sequence ends at the first byte in 0x40–0x7e AFTER the '['
			// introducer. '[' itself (0x5b) is in that range but must NOT be
			// treated as the terminator, or the SGR parameters get counted as
			// visible width and the splice width math breaks.
			if r != '[' && r >= 0x40 && r <= 0x7e {
				inEsc = false
				e := seq.String()
				switch {
				case open:
					out.WriteString(e)
				case e == "\x1b[0m" || e == "\x1b[m":
					active.Reset()
				default:
					active.WriteString(e)
				}
			}
			continue
		}
		w := runewidth.RuneWidth(r)
		if pos+w <= cutWidth { // fully before the cut: drop
			pos += w
			continue
		}
		if !open {
			out.WriteString(active.String())
			open = true
		}
		if pos < cutWidth { // wide rune straddles the boundary
			out.WriteString(strings.Repeat(" ", pos+w-cutWidth))
		} else {
			out.WriteRune(r)
		}
		pos += w
	}
	return out.String()
}
