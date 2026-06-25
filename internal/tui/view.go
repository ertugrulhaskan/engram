package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// --- view ---

func (m Model) View() string {
	if !m.ready {
		return "loading…"
	}
	panes := lipgloss.JoinHorizontal(lipgloss.Top, m.listPane(), m.dividerCol(), m.previewPane())
	frame := lipgloss.JoinVertical(lipgloss.Left,
		m.topBar(), m.subRow(), panes, m.bottomRule(), m.bottomBar())

	// Modal dialogs float over the frame (VS Code-style) instead of replacing a
	// pane, so the list and preview stay visible behind them.
	box, top := "", false
	switch m.mode {
	case modeConfirm:
		box = m.confirmModal()
	case modeNew:
		box = m.newModal()
	case modePalette:
		box, top = m.paletteBox(), true
	case modeHelp:
		box = m.helpModal()
	}
	if box != "" {
		frame = m.overlay(frame, box, top)
	}

	// Make every line exactly m.width: pad short lines (so an overlaid dialog
	// never leaves stale cells to its right) and clip overflow (glamour margins
	// on very narrow terminals). Height is already correct via the reserved row.
	return clampFrame(frame, m.width)
}

// clampFrame makes every line of frame exactly w cells — padding short lines and
// truncating long ones — so no rendered row leaves stale terminal cells to its
// right or overflows the width. It measures with the same ansi helpers the
// overlay uses, so a styled (background-filled) row is sized consistently.
func clampFrame(frame string, w int) string {
	lines := strings.Split(frame, "\n")
	for i, ln := range lines {
		switch lw := ansi.StringWidth(ln); {
		case lw < w:
			ln += spaces(w - lw)
		case lw > w:
			ln = ansi.Truncate(ln, w, "")
		}
		lines[i] = ln
	}
	return strings.Join(lines, "\n")
}

// overlay floats box over frame, horizontally centered. A "top" box (the
// palette) sits near the top like VS Code's quick-open; everything else is
// centered vertically.
func (m Model) overlay(frame, box string, top bool) string {
	bw, bh := lipgloss.Width(box), lipgloss.Height(box)
	x := (m.width - bw) / 2
	y := 2
	if !top {
		y = (lipgloss.Height(frame) - bh) / 2
	}
	if y < 1 {
		y = 1
	}
	return placeOverlay(x, y, box, frame)
}

func (m Model) topBar() string {
	t := m.theme()
	brand := t.bar(t.Accent).Bold(true).Render(" engram ")

	var info, right string
	if m.srcKind == srcPlans {
		info = fmt.Sprintf("%d plans", len(m.plans))
		right = t.bar(t.Dim).Render("source ") + t.bar(t.Accent).Bold(true).Render("plans") + t.bar(t.Dim).Render(" ")
	} else if m.srcKind == srcFiles {
		info = fmt.Sprintf("%d files · read-only", len(m.docs))
		right = t.bar(t.Dim).Render("source ") + t.bar(t.Accent).Bold(true).Render("files") + t.bar(t.Dim).Render(" ")
	} else {
		seen := map[string]struct{}{}
		for _, mm := range m.memories {
			seen[mm.Project.Name] = struct{}{}
		}
		info = fmt.Sprintf("%d memories · %d projects", len(m.memories), len(seen))
		scope := "project"
		if m.groupBy == groupType {
			scope = "type"
		}
		typeScope := "all"
		if tf := typeCycle[m.typeIdx]; tf != "" {
			typeScope = string(tf)
		}
		right = t.bar(t.Dim).Render("grouped by ") + t.bar(t.Accent).Bold(true).Render(scope) +
			t.bar(t.Dim).Render("   type ") + t.bar(t.Accent).Bold(true).Render(typeScope) + t.bar(t.Dim).Render(" ")
	}
	if q := strings.TrimSpace(m.search.Value()); q != "" && m.mode != modeFilter {
		info += " · “" + q + "”" // echo the active filter so a narrowed list has a visible reason
	}
	left := brand + t.bar(t.Dim).Render(" "+info+" ")
	if m.driftOut {
		left += dangerStyle().Render(" " + driftSummary(m.driftUnindexed, m.driftDangling) + " ")
	}
	return m.barLine(left, right, t.BarBg)
}

// subRow is the line under the top bar: a focus underline per pane, or the
// search input over the list when filtering.
func (m Model) subRow() string {
	t := m.theme()
	var left string
	if m.mode == modeFilter {
		left = padTo(m.search.View(), m.listW)
	} else {
		c := t.Border
		if m.focus == focusList {
			c = t.Accent
		}
		left = fg(c).Render(strings.Repeat("─", m.listW))
	}
	rc := t.Border
	if m.focus == focusPreview {
		rc = t.Accent
	}
	right := fg(rc).Render(strings.Repeat("─", m.previewW))
	return left + fg(t.Border).Render("┬") + right
}

func (m Model) bottomRule() string {
	t := m.theme()
	return fg(t.Border).Render(strings.Repeat("─", m.listW)) +
		fg(t.Border).Render("┴") +
		fg(t.Border).Render(strings.Repeat("─", m.previewW))
}

func (m Model) bottomBar() string {
	t := m.theme()
	var left string
	switch {
	case m.mode == modePalette:
		left = t.bar(t.Dim).Render(" ") + t.bar(t.Accent).Render("memory · plans · files · settings · @claude") +
			t.bar(t.Dim).Render(" · type to jump · ") + t.bar(t.Accent).Render("↑↓") + t.bar(t.Dim).Render(" · ") +
			t.bar(t.Accent).Render("↵") + t.bar(t.Dim).Render(" · ") +
			t.bar(t.Accent).Render("esc") + t.bar(t.Dim).Render(" close ")
	case m.mode == modeFilter:
		left = t.bar(t.Dim).Render(" type to filter  ") + t.bar(t.Accent).Render("↵") +
			t.bar(t.Dim).Render(" apply   ") + t.bar(t.Accent).Render("esc") + t.bar(t.Dim).Render(" clear ")
	case m.status != "":
		left = m.statusStyle(t).Render(" " + m.status + " ")
	default:
		left = m.hints(t)
	}
	right := t.bar(t.Dim).Render("theme ") + t.bar(t.Accent).Bold(true).Render(t.Name) +
		t.bar(t.Dim).Render(" · 1–5 to switch ")
	return m.barLine(left, right, t.BarBg)
}

// statusStyle picks the footer color for the current status by its kind: danger
// and cancel get their semantic backgrounds, everything else the bar default.
func (m Model) statusStyle(t Theme) lipgloss.Style {
	switch m.statusKind {
	case statusDanger:
		return dangerStyle()
	case statusCancel:
		return cancelStyle()
	default:
		return t.bar(t.Fg)
	}
}

// driftSummary names the cause of MEMORY.md drift so the warning is actionable:
// files added on disk without an index line, entries left dangling by a deleted
// or renamed file, or both.
func driftSummary(unindexed, dangling int) string {
	switch {
	case unindexed > 0 && dangling > 0:
		return fmt.Sprintf("⚠ index out of sync · %d file(s) added without an index line · %d entry(ies) for a deleted/renamed file", unindexed, dangling)
	case unindexed > 0:
		return fmt.Sprintf("⚠ index out of sync · %d file(s) added without a MEMORY.md index line", unindexed)
	default:
		return fmt.Sprintf("⚠ index out of sync · %d .md file(s) deleted/renamed without updating MEMORY.md", dangling)
	}
}

func (m Model) hints(t Theme) string {
	var pairs [][2]string
	if m.srcKind == srcPlans {
		pairs = [][2]string{
			{"↑↓/jk", "move"}, {"/", "filter"}, {"⇥", "focus"}, {"d", "delete"},
		}
	} else if m.srcKind == srcFiles {
		pairs = [][2]string{
			{"↑↓/jk", "move"}, {"/", "filter"}, {"⇥", "focus"}, {"@", "edit via Claude"},
		}
	} else {
		pairs = [][2]string{
			{"↑↓/jk", "move"}, {"/", "filter"}, {"⇥", "focus"}, {"e", "edit"},
			{"n", "new"}, {"d", "delete"}, {"t", "type"}, {"g", "group"},
		}
		if m.driftOut {
			pairs = append(pairs, [2]string{"R", "fix index"})
		}
	}
	pairs = append(pairs, [2]string{"^P", "palette"})
	pairs = append(pairs, [2]string{"?", "help"})
	pairs = append(pairs, [2]string{"q", "quit"})
	render := func(ps [][2]string) string {
		out := t.bar(t.Dim).Render(" ")
		for _, p := range ps {
			out += t.bar(t.Fg).Render(p[0]) + t.bar(t.Dim).Render(" "+p[1]+"  ")
		}
		return out
	}
	out := render(pairs)
	avail := m.width - lipgloss.Width(t.bar(t.Dim).Render("theme "+t.Name+" · 1–5 to switch ")) - 1
	for lipgloss.Width(out) > avail && len(pairs) > 1 {
		pairs = pairs[:len(pairs)-1]
		out = render(pairs)
	}
	return out
}

// barLine lays out a bar with a left and right segment over a filled background.
func (m Model) barLine(left, right, bg string) string {
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	mid := lipgloss.NewStyle().Background(lipgloss.Color(bg)).Render(spaces(gap))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, mid, right)
}

func (m Model) dividerCol() string {
	line := fg(m.theme().Border).Render("│")
	lines := make([]string, m.panesH)
	for i := range lines {
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}
