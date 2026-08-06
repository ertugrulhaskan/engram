package tui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// Shared dialog anatomy (design spec 07): an opaque BgPane panel framed in the
// dialog's semantic color, an icon+title header over a rule, body copy with the
// first entry in Fg and the rest in Dim, and the actions bottom-right on a Bg2
// footer band that bleeds flush to the border. Dialogs with custom rows
// (promote's choices, the secret findings, the resolve hunk) assemble the same
// pieces themselves; the rest go through dialog().

// dialogAction is one footer chip: its label ("y pull") and whether it is the
// primary action (semantic color, bold) rather than a cancel (Faint).
type dialogAction struct {
	label   string
	primary bool
}

// dialog assembles the full shared anatomy for a plain-copy dialog.
func (m Model) dialog(icon, title, color string, body []string, actions []dialogAction) string {
	cw := m.boxWidth()
	panel := m.panelBg()
	lines := m.dlgHeader(cw, icon, title, color)
	lines = append(lines, m.dlgBody(cw, body)...)
	lines = append(lines, padBG("", cw, panel))
	bleed := map[int]string{len(lines): m.theme().Bg2}
	lines = append(lines, m.dlgFooter(cw, color, actions))
	return m.frameLines(lines, cw, color, bleed)
}

// dlgHeader is the dialog top: the icon in the semantic color, the title in Fg,
// over an edge rule.
func (m Model) dlgHeader(cw int, icon, title, color string) []string {
	t := m.theme()
	panel := m.panelBg()
	head := onbg(color, panel).Bold(true).Render(" "+icon+" ") +
		onbg(t.Fg, panel).Bold(true).Render(clip(title, cw-4))
	return []string{padBG(head, cw, panel), m.ruleLine(cw)}
}

// dlgBody renders the spec's body rule — first entry Fg, the rest Dim — each
// entry word-wrapped to the dialog width.
func (m Model) dlgBody(cw int, entries []string) []string {
	t := m.theme()
	var lines []string
	for i, e := range entries {
		c := t.Dim
		if i == 0 {
			c = t.Fg
		}
		lines = append(lines, m.dlgText(cw, e, c)...)
	}
	return lines
}

// dlgText wraps one body entry and styles every produced line in the given
// color over the panel fill.
func (m Model) dlgText(cw int, s, color string) []string {
	panel := m.panelBg()
	var out []string
	for _, ln := range wrapPlain(s, cw-4) {
		out = append(out, padBG(onbg(color, panel).Render("  "+ln), cw, panel))
	}
	return out
}

// dlgFooter is the Bg2 action band, chips right-aligned. The caller bleeds
// this row so the band runs flush to the border (frameLines paints the side
// cells with the band's background instead of the border glyph).
func (m Model) dlgFooter(cw int, color string, actions []dialogAction) string {
	t := m.theme()
	right := ""
	for i, a := range actions {
		if i > 0 {
			right += onbg(t.Faint, t.Bg2).Render(" ")
		}
		if a.primary {
			right += onbg(color, t.Bg2).Bold(true).Render("[" + a.label + "]")
		} else {
			right += onbg(t.Faint, t.Bg2).Render("[" + a.label + "]")
		}
	}
	right += onbg(t.Faint, t.Bg2).Render(" ")
	return bandLine("", right, cw, t.Bg2)
}

// wrapPlain greedily word-wraps plain text to w display cells. A single word
// wider than the line clips rather than overflowing — an over-wide dialog row
// would break the frame (and lipgloss Width() would wrap, not truncate).
func wrapPlain(s string, w int) []string {
	if w < 1 {
		return []string{""}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	cur := ""
	for _, wd := range words {
		cand := wd
		if cur != "" {
			cand = cur + " " + wd
		}
		if runewidth.StringWidth(cand) <= w {
			cur = cand
			continue
		}
		if cur != "" {
			lines = append(lines, cur)
		}
		if runewidth.StringWidth(wd) > w {
			wd = clip(wd, w)
		}
		cur = wd
	}
	return append(lines, cur)
}
