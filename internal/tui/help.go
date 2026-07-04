package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// helpGroups is the keybinding reference shown by the `?` overlay, in display
// order, split into logical groups. A blank line is drawn between groups so the
// list has vertical rhythm and stays scannable. It's the full set; the
// bottom-bar hints show only the context-relevant subset, so this overlay is the
// one place that lists everything.
var helpGroups = [][]struct{ key, desc string }{
	{
		{"↑ ↓ · j k", "move selection"},
		{"pgup pgdn", "page up / down"},
		{"⇥ tab", "switch list ⇄ preview"},
	},
	{
		{"/", "filter"},
		{"e", "edit in $EDITOR"},
		{"n", "new memory"},
		{"d", "delete"},
		{"R", "rebuild MEMORY.md index"},
	},
	{
		{"t", "cycle type filter"},
		{"g", "group by project ⇄ type"},
		{"1–5", "switch theme"},
	},
	{
		{"^P", "palette · / sources · @Claude · > team"},
		{">", "team: promote · pull · resolve · withdraw · init"},
		{"?", "this help"},
		{"q · ^C", "quit"},
	},
}

// helpModal renders the keybinding cheat-sheet as a floating dialog, in the same
// opaque style as the palette and confirm modals, with an about footer carrying
// the version. Dismissed by any key (see updateHelp).
func (m Model) helpModal() string {
	t := m.theme()
	cw := m.boxWidth()
	panel := m.panelBg()
	pst := func(c string) lipgloss.Style { return fg(c).Background(lipgloss.Color(panel)) }

	lines := []string{
		padBG(pst(t.Accent).Bold(true).Render(" Keybindings"), cw, panel),
		m.ruleLine(cw),
	}
	keyCol := helpKeyCol()
	for gi, group := range helpGroups {
		if gi > 0 {
			lines = append(lines, padBG("", cw, panel)) // breathing room between groups
		}
		for _, r := range group {
			lines = append(lines, m.helpRow(r.key, r.desc, cw, keyCol))
		}
	}
	v := m.version
	if v == "" {
		v = "dev"
	}
	about := "engram " + v + " · engram.im · MIT"
	lines = append(lines,
		m.ruleLine(cw),
		padBG(pst(t.Dim).Render("  "+clip(about, cw-2)), cw, panel),
	)
	return m.frameLines(lines, cw, t.Accent, nil)
}

// helpKeyCol is the key-column width, derived from the widest key in helpGroups
// plus a fixed gap, so descriptions stay aligned and nothing truncates as the
// table grows (rather than a hand-maintained magic width).
func helpKeyCol() int {
	w := 0
	for _, g := range helpGroups {
		for _, r := range g {
			if x := runewidth.StringWidth(r.key); x > w {
				w = x
			}
		}
	}
	return w + 5 // gap between the key column and the description
}

// helpRow lays out one key→description line: the key in the accent color in a
// keyCol-wide column, then the dim description, all carrying the panel
// background. The description is clipped so the row never exceeds cw.
func (m Model) helpRow(key, desc string, cw, keyCol int) string {
	t := m.theme()
	panel := m.panelBg()
	if w := runewidth.StringWidth(key); w > keyCol {
		key = runewidth.Truncate(key, keyCol, "")
	}
	gap := keyCol - runewidth.StringWidth(key) // ≥ 0
	avail := cw - 2 - keyCol - 1               // 2 indent + key column + 1 separating space
	if avail < 4 {
		avail = 4
	}
	bg := lipgloss.Color(panel)
	line := fg(t.Accent).Background(bg).Render("  "+key) +
		fg(t.Dim).Background(bg).Render(spaces(gap+1)+clip(desc, avail))
	return padBG(line, cw, panel)
}
