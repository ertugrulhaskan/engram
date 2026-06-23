package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

func (m Model) listPane() string {
	t := m.theme()
	h := m.listRows()
	rightCol := m.rightColW() // computed once per render, not per row
	lines := make([]string, 0, h)
	for i := m.top; i < m.top+h; i++ {
		switch {
		case i < 0 || i >= len(m.rows):
			lines = append(lines, "")
		case m.rows[i].kind == rowSpacer:
			lines = append(lines, "")
		case m.rows[i].kind == rowHeader:
			lines = append(lines, m.headerRow(m.rows[i]))
		default:
			lines = append(lines, m.memRow(m.rows[i].item, i == m.cursor, rightCol))
		}
	}
	shown := m.shownCount()
	if shown == 0 && len(lines) > 0 {
		lines[0] = fg(t.Dim).Render("  no matches")
	}
	total := len(m.memories)
	if m.srcKind == srcPlans {
		total = len(m.plans)
	}
	body := lipgloss.NewStyle().Width(m.listW).Render(strings.Join(lines, "\n"))
	status := fg(t.Dim).Render(fmt.Sprintf(" %d of %d shown", shown, total))
	return lipgloss.JoinVertical(lipgloss.Left, body, lipgloss.NewStyle().Width(m.listW).Render(status))
}

// rightColW sizes the right-aligned column from the widest Item.Right in view
// (project name when grouped by type, or the date for plans), collapsing to 0
// when nothing needs it or it would starve the title.
func (m Model) rightColW() int {
	maxr := 0
	for _, r := range m.rows {
		if r.kind == rowMemory {
			if l := runewidth.StringWidth(r.item.Right); l > maxr {
				maxr = l
			}
		}
	}
	if maxr == 0 {
		return 0
	}
	maxAllowed := m.listW - 2 - badgeWidth - 2 - 12
	if maxAllowed < 6 {
		return 0
	}
	if maxr > maxAllowed {
		maxr = maxAllowed
	}
	if maxr < 4 {
		return 0
	}
	return maxr
}

func (m Model) headerRow(r row) string {
	t := m.theme()
	suffix := fmt.Sprintf(" (%d)", r.count)
	label := clip(r.label, m.listW-2-runewidth.StringWidth(suffix))
	return fg(r.color).Render("▌ ") + fgb(r.color).Render(label) + fg(t.Dim).Render(suffix)
}

func (m Model) memRow(it Item, selected bool, rightCol int) string {
	t := m.theme()

	badgeCol := 0
	if it.Badge != "" {
		badgeCol = badgeWidth + 1 // padded badge + trailing space
	}
	nameW := m.listW - 2 - badgeCol - rightCol
	if rightCol > 0 {
		nameW-- // gap before the right column
	}
	if nameW < 4 {
		nameW = 4
	}

	bg := ""
	titleColor := t.Fg
	if selected {
		bg, titleColor = t.SelBg, t.SelFg
	}
	st := func(c string) lipgloss.Style {
		s := fg(c)
		if bg != "" {
			s = s.Background(lipgloss.Color(bg))
		}
		return s
	}

	indent := st(t.Faint).Render("  ")
	if selected {
		indent = st(t.Accent).Bold(true).Render("› ")
	}
	out := indent
	if it.Badge != "" {
		out += st(it.BadgeColor).Render(padRight("["+it.Badge+"]", badgeWidth)) + st(t.Fg).Render(" ")
	}
	out += st(titleColor).Render(padRight(it.Title, nameW))
	if rightCol > 0 {
		out += st(t.Fg).Render(" ") + st(t.Dim).Render(padLeft(it.Right, rightCol))
	}
	return out
}

func (m Model) previewPane() string {
	t := m.theme()
	innerW := m.previewW - previewPad
	it, ok := m.selected()
	if !ok {
		return lipgloss.NewStyle().Width(m.previewW).Height(m.panesH).Render(fg(t.Dim).Render("  nothing selected"))
	}
	meta, used := "", 0
	if it.Badge != "" {
		b := "[" + it.Badge + "]"
		meta = fg(it.BadgeColor).Bold(true).Render(b) + " "
		used = runewidth.StringWidth(b) + 1
	}
	rest := "edited " + humanizeSince(it.Modified)
	if it.Context != "" {
		rest = it.Context + " · " + rest
	}
	meta += fg(t.Dim).Render(clip(rest, innerW-used))
	title := m.renderTitle(it.Title, innerW)
	block := lipgloss.JoinVertical(lipgloss.Left, meta, "", title, "", m.viewport.View())
	// Width(previewW) so every preview line fills the pane — otherwise the joined
	// frame has ragged line widths and a floated dialog leaves stale cells.
	return lipgloss.NewStyle().PaddingLeft(previewPad).Width(m.previewW).Render(block)
}

// renderTitle styles the preview title in the accent color, with `code` spans
// shown as inline chips.
func (m Model) renderTitle(title string, w int) string {
	t := m.theme()
	title = clip(title, w)
	var b strings.Builder
	for i, part := range strings.Split(title, "`") {
		if i%2 == 1 {
			b.WriteString(fg(t.Fg).Background(lipgloss.Color(t.SelBg)).Render(part))
		} else {
			b.WriteString(fgb(t.Accent).Render(part))
		}
	}
	return b.String()
}

// boxWidth is the inner content width shared by the floating dialogs, sized to
// the terminal but capped so the box reads like a dialog, not a full pane.
func (m Model) boxWidth() int {
	w := m.width - 12
	if w > 68 {
		w = 68
	}
	if w < 30 {
		w = 30
	}
	return w
}

// panelBg is the opaque background shared by the floating dialogs — a shade
// lighter than the terminal so the box clearly reads as a raised surface.
func (m Model) panelBg() string { return m.theme().SelBg }

// padBG right-pads a (possibly styled) string to width w, filling the gap with
// the given background so every cell of a dialog row is opaque.
func padBG(s string, w int, bg string) string {
	gap := w - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	return s + lipgloss.NewStyle().Background(lipgloss.Color(bg)).Render(spaces(gap))
}

// dialogFrame wraps opaque content in a rounded border whose cells share the
// panel background, so the whole box is a solid surface over the panes.
func (m Model) dialogFrame(content, border string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(border)).
		BorderBackground(lipgloss.Color(m.panelBg())).
		Render(content)
}

// ruleLine is a horizontal rule that carries the panel background.
func (m Model) ruleLine(cw int) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme().Faint)).
		Background(lipgloss.Color(m.panelBg())).Render(strings.Repeat("─", cw))
}

// confirmModal is the delete confirmation, styled like the palette dialog: a
// header, the target shown as a highlighted (danger) row, and the y/n actions.
func (m Model) confirmModal() string {
	t := m.theme()
	cw := m.boxWidth()
	panel := m.panelBg()
	pst := func(col string) lipgloss.Style { return fg(col).Background(lipgloss.Color(panel)) }
	it, _ := m.selected()
	kind := "memory"
	if it.Kind == "plan" {
		kind = "plan"
	}
	row := m.palRow(palItem{glyph: "✕", label: it.Title, sub: kind + " · this cannot be undone"}, cw, panel, t.Danger)
	hint := pst(t.Dim).Render(" press ") + pst(t.Danger).Bold(true).Render("y") + pst(t.Dim).Render(" delete     ") +
		pst(t.Fg).Bold(true).Render("n") + pst(t.Dim).Render(" / ") + pst(t.Fg).Bold(true).Render("esc") + pst(t.Dim).Render(" cancel")
	lines := []string{
		padBG(pst(t.Danger).Bold(true).Render(" Delete "+kind+"?"), cw, panel),
		m.ruleLine(cw),
		row[0], row[1],
		padBG("", cw, panel),
		padBG(hint, cw, panel),
	}
	return m.dialogFrame(strings.Join(lines, "\n"), t.Danger)
}

// newModal is the new-memory title prompt, in the same opaque dialog style.
func (m Model) newModal() string {
	t := m.theme()
	cw := m.boxWidth()
	panel := m.panelBg()
	pst := func(col string) lipgloss.Style { return fg(col).Background(lipgloss.Color(panel)) }
	lines := []string{
		padBG(pst(t.Accent).Bold(true).Render(" New memory"), cw, panel),
		m.ruleLine(cw),
		padBG(pst(t.Dim).Render("  title for the new memory in this project"), cw, panel),
		padBG("", cw, panel),
		padBG("  "+m.input.View(), cw, panel),
		padBG("", cw, panel),
		padBG(pst(t.Dim).Render("  press ")+pst(t.Accent).Bold(true).Render("↵")+pst(t.Dim).Render(" create     ")+
			pst(t.Fg).Bold(true).Render("esc")+pst(t.Dim).Render(" cancel"), cw, panel),
	}
	return m.dialogFrame(strings.Join(lines, "\n"), t.Accent)
}
