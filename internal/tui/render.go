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
	badgeW := m.badgeColW() // widest type badge in view; computed once per render
	syncW := m.syncColW()   // sync-glyph column width (0 when nothing in view is shared)
	leftCols := badgeW
	if syncW > 0 {
		leftCols += syncW + 1
	}
	rightCol := m.rightColW(leftCols) // computed once per render, not per row
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
			lines = append(lines, m.memRow(m.rows[i].item, i == m.cursor, badgeW, syncW, rightCol))
		}
	}
	shown := m.shownCount()
	if shown == 0 && len(lines) > 0 {
		lines[0] = fg(t.Dim).Render("  no matches")
	}
	total := len(m.memories)
	switch m.srcKind {
	case srcPlans:
		total = len(m.plans)
	case srcFiles:
		total = len(m.docs)
	}
	body := lipgloss.NewStyle().Width(m.listW).Render(strings.Join(lines, "\n"))
	status := fg(t.Dim).Render(fmt.Sprintf(" %d of %d shown", shown, total))
	return lipgloss.JoinVertical(lipgloss.Left, body, lipgloss.NewStyle().Width(m.listW).Render(status))
}

// badgeColW sizes the badge bracket field from the widest badge in the current
// (filtered) list, so short badges (e.g. "[user]") don't leave a wide gap before the
// title. Capped at badgeWidth (the widest possible "[reference]"); 0 when no row has a badge.
func (m Model) badgeColW() int {
	w := 0
	for _, r := range m.rows {
		if r.kind == rowMemory && r.item.Badge != "" {
			if l := runewidth.StringWidth("[" + r.item.Badge + "]"); l > w {
				w = l
			}
		}
	}
	if w > badgeWidth {
		w = badgeWidth
	}
	return w
}

// syncColW is the width of the team-sync glyph column: the widest SyncBadge in
// view, measured with the same runewidth oracle memRow pads with, so the column
// can't drift. It collapses to 0 when nothing in view carries a sync badge, so
// the feature is invisible for anyone not using team sharing.
func (m Model) syncColW() int {
	w := 0
	for _, r := range m.rows {
		if r.kind == rowMemory && r.item.SyncBadge != "" {
			if l := runewidth.StringWidth(r.item.SyncBadge); l > w {
				w = l
			}
		}
	}
	return w
}

// rightColW sizes the right-aligned column from the widest Item.Right in view
// (project name when grouped by type, or the date for plans), collapsing to 0
// when nothing needs it or it would starve the title. leftCols is the in-view
// left column width (type badge + sync column) so the budget reflects the actual
// (not worst-case) left side.
func (m Model) rightColW(leftCols int) int {
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
	maxAllowed := m.listW - 2 - leftCols - 2 - 12
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

func (m Model) memRow(it Item, selected bool, badgeW, syncW, rightCol int) string {
	t := m.theme()

	badgeCol := 0
	if it.Badge != "" {
		badgeCol = badgeW + 1 // padded badge + trailing space
	}
	syncCol := 0
	if syncW > 0 {
		syncCol = syncW + 1 // reserved for every row so titles stay column-aligned
	}
	nameW := m.listW - 2 - badgeCol - syncCol - rightCol
	if rightCol > 0 {
		nameW-- // gap before the right column
	}
	if nameW < 4 {
		nameW = 4
	}

	// Selection: an accent chevron + bold accent title over a SelBg row highlight.
	// The highlight is safe from ghost-cell bleed because clampFrame now closes
	// every line's background (a glamour code chip could otherwise leave a bg open
	// and smear across rows) — the row fill itself was never the leak.
	bg := ""
	if selected {
		bg = t.SelBg
	}
	st := func(c string) lipgloss.Style {
		s := fg(c)
		if bg != "" {
			s = s.Background(lipgloss.Color(bg))
		}
		return s
	}

	indent := st(t.Faint).Render("  ")
	titleColor := t.Fg
	if selected {
		indent = st(t.Accent).Bold(true).Render("› ") // chevron, distinct from the header's ▌ bar
		titleColor = t.Accent
	}
	out := indent
	if it.Badge != "" {
		out += st(it.BadgeColor).Render(padRight("["+it.Badge+"]", badgeW)) + st(t.Fg).Render(" ")
	}
	if syncW > 0 {
		if it.SyncBadge != "" {
			out += st(it.SyncColor).Render(padRight(it.SyncBadge, syncW)) + st(t.Fg).Render(" ")
		} else {
			out += st(t.Fg).Render(padRight("", syncW) + " ") // blank column keeps titles aligned
		}
	}
	titleStyle := st(titleColor)
	if selected {
		titleStyle = titleStyle.Bold(true)
	}
	out += titleStyle.Render(padRight(it.Title, nameW))
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
	if _, c, word := syncBadge(m.syncStates[it.Path]); word != "" {
		tok := "team " + word // spelled out here where there's room, unlike the list glyph
		meta += fg(c).Bold(true).Render(tok) + " "
		used += runewidth.StringWidth(tok) + 1
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
	// Height+MaxHeight pin the pane to exactly panesH lines: a long preview can
	// never push the whole frame past the terminal height. An overflowing frame
	// scrolls the alt-screen, which desyncs Bubble Tea's line-diff renderer and
	// leaves ghost rows (a trailing highlight) until the next full repaint.
	return lipgloss.NewStyle().PaddingLeft(previewPad).Width(m.previewW).
		Height(m.panesH).MaxHeight(m.panesH).Render(block)
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

// panelBg is the fill behind the floating dialogs. It's empty ("") on purpose:
// the dialogs render as a rounded accent border over the terminal's own
// background, with no filled panel — the only way a terminal can show smooth
// rounded corners, since a filled cell squares the corner off (a terminal cell
// is one glyph + one fg + one bg, with no sub-cell clipping like CSS). lipgloss
// treats an empty color as "unset", so every shared fill helper (padBG,
// ruleLine, the per-modal background styles) goes transparent automatically.
// Selection and danger highlights pass their own color as selBg and are
// unaffected.
func (m Model) panelBg() string { return "" }

// padBG right-pads a (possibly styled) string to width w, filling the gap with
// the given background so every cell of a dialog row is opaque.
func padBG(s string, w int, bg string) string {
	gap := w - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	return s + lipgloss.NewStyle().Background(lipgloss.Color(bg)).Render(spaces(gap))
}

// frameLines wraps cw-wide content lines in a rounded accent border drawn by
// hand (not lipgloss's auto-border) so individual rows can bleed. The corners
// carry no background, so they render smoothly on the terminal's own background
// (a filled cell would square them off). A line index present in bleed has its
// two side cells painted with that background instead of the "│" glyph, so a
// full-width selection/danger highlight runs edge to edge — flush with the
// border, no dark gap — while every other row gets the thin accent border.
func (m Model) frameLines(lines []string, cw int, border string, bleed map[int]string) string {
	a := fg(border)
	out := make([]string, 0, len(lines)+2)
	out = append(out, a.Render("╭"+strings.Repeat("─", cw)+"╮"))
	for i, ln := range lines {
		if bg, ok := bleed[i]; ok {
			edge := lipgloss.NewStyle().Background(lipgloss.Color(bg)).Render(" ")
			out = append(out, edge+ln+edge)
		} else {
			bar := a.Render("│")
			out = append(out, bar+ln+bar)
		}
	}
	out = append(out, a.Render("╰"+strings.Repeat("─", cw)+"╯"))
	return strings.Join(out, "\n")
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
	}
	// Derive the bleed indices from where the target rows land (not hardcoded), so
	// they stay correct if the header/rule lines change — mirrors paletteBox.
	bleed := map[int]string{len(lines): t.Danger, len(lines) + 1: t.Danger}
	lines = append(lines, row[0], row[1], padBG("", cw, panel), padBG(hint, cw, panel))
	return m.frameLines(lines, cw, t.Danger, bleed)
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
		padBG(pst(t.Dim).Render(clip("  title for the new memory in this project", cw)), cw, panel),
		padBG("", cw, panel),
		padBG("  "+m.input.View(), cw, panel),
		padBG("", cw, panel),
		padBG(pst(t.Dim).Render("  press ")+pst(t.Accent).Bold(true).Render("↵")+pst(t.Dim).Render(" create     ")+
			pst(t.Fg).Bold(true).Render("esc")+pst(t.Dim).Render(" cancel"), cw, panel),
	}
	return m.frameLines(lines, cw, t.Accent, nil)
}
