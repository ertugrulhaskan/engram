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
		m.topBar(), m.sourceStrip(), m.subRow(), panes, m.bottomRule(), m.bottomBar())

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
	case modePromoteScope:
		box = m.scopeModal()
	case modeSecretWarn:
		box = m.secretModal()
	case modeWithdrawConfirm:
		box = m.withdrawModal()
	}
	if box != "" {
		// Scrim: dim the page toward the surface color before compositing, so
		// the dialog visibly floats (truecolor only; a no-op elsewhere).
		frame = m.overlay(m.dimFrame(frame), box, top)
	}

	// Make every line exactly m.width: pad short lines (so an overlaid dialog
	// never leaves stale cells to its right) and clip overflow (glamour margins
	// on very narrow terminals). Then clamp the line count so the frame is never
	// taller than the screen — a too-tall frame scrolls the alt-screen and
	// desyncs the line-diff renderer, leaving ghost rows until the next full
	// repaint. Normally the layout already yields exactly m.height-1 lines; this
	// only bites on a very short terminal, where panesH is floored above h-5.
	return clampFrameHeight(clampFrame(frame, m.width), m.height)
}

// clampFrameHeight drops any lines past the terminal's height budget so the
// frame can never be taller than the screen. It keeps at most h-1 lines, leaving
// the final terminal row unwritten (see resize: writing the last cell scrolls
// the alt-screen on some terminals). Shorter frames pass through untouched.
func clampFrameHeight(frame string, h int) string {
	limit := h - 1
	if limit < 1 {
		limit = 1
	}
	lines := strings.Split(frame, "\n")
	if len(lines) <= limit {
		return frame
	}
	return strings.Join(lines[:limit], "\n")
}

// clampFrame makes every line of frame exactly w cells — padding short lines and
// truncating long ones — so no rendered row leaves stale terminal cells to its
// right or overflows the width. It measures with the same ansi helpers the
// overlay uses, so a styled (background-filled) row is sized consistently.
func clampFrame(frame string, w int) string {
	lines := strings.Split(frame, "\n")
	for i, ln := range lines {
		// A glamour inline-code chip (`code`) can end a wrapped preview line with
		// its background still open; every clamped line must therefore end with a
		// reset, or the padding spaces — and the first cells of the next row —
		// inherit that background and render as a gray ghost band / block.
		switch lw := ansi.StringWidth(ln); {
		case lw > w:
			ln = ansi.Truncate(ln, w, "") + ansi.ResetStyle // reset after (Truncate only keeps existing codes, doesn't synthesize a close)
		default:
			ln += ansi.ResetStyle + spaces(w-lw) // reset before padding so the padding stays clean
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

// topBar is the title bar: brand + version on the left, the theme switcher on
// the right (moved here from the bottom bar when the source strip landed). The
// drift pill stays here until the Phase 8 banner replaces it.
func (m Model) topBar() string {
	t := m.theme()
	left := t.bar(t.Accent).Bold(true).Render(" engram ")
	if m.version != "" {
		// Clip: release versions are short ("v0.2.1"), but a dev build carries a
		// VCS pseudo-version long enough to crowd out the rest of the bar.
		left += t.bar(t.Dim).Render(clip(m.version, 20) + " ")
	}
	if m.driftOut {
		left += t.danger().Render(" " + driftSummary(m.driftUnindexed, m.driftDangling) + " ")
	}
	right := t.bar(t.Dim).Render("theme ") + t.bar(t.Accent).Bold(true).Render(t.Name) +
		t.bar(t.Dim).Render(" · 1–3 to switch ")
	return m.barLine(left, right, t.Bg2)
}

// sourceStrip is the persistent source tab row under the title bar: every
// source with its live count, so switching is a visible affordance rather than
// palette trivia. The active tab reads in Fg bold + underline with the count in
// Accent; inactive tabs are Faint. The right side echoes the list-shaping state
// (group/type for memories, plus the committed search for any source) or, when
// there is nothing to echo, points at the palette.
func (m Model) sourceStrip() string {
	t := m.theme()
	tabs := []struct {
		kind  srcKind
		label string
		count int
	}{
		{srcMemories, "memories", len(m.memories)},
		{srcPlans, "plans", len(m.plans)},
		{srcFiles, "files", len(m.docs)},
	}
	left := onbg(t.Faint, t.Bg).Render(" ")
	for i, tab := range tabs {
		if i > 0 {
			left += onbg(t.Faint, t.Bg).Render("  ·  ")
		}
		if tab.kind == m.srcKind {
			left += onbg(t.Fg, t.Bg).Bold(true).Underline(true).Render(tab.label) +
				onbg(t.Fg, t.Bg).Underline(true).Render(" ") +
				onbg(t.Accent, t.Bg).Underline(true).Render(fmt.Sprintf("%d", tab.count))
		} else {
			left += onbg(t.Faint, t.Bg).Render(fmt.Sprintf("%s %d", tab.label, tab.count))
		}
	}

	var right string
	echo := ""
	if q := strings.TrimSpace(m.search.Value()); q != "" && m.mode != modeFilter {
		echo = "“" + q + "”" // echo the committed filter so a narrowed list has a visible reason
	}
	if m.srcKind == srcMemories {
		scope := "project"
		if m.groupBy == groupType {
			scope = "type"
		}
		typeScope := "all"
		if tf := typeCycle[m.typeIdx]; tf != "" {
			typeScope = string(tf)
		}
		right = onbg(t.Dim, t.Bg).Render("group ") + onbg(t.Accent, t.Bg).Bold(true).Render(scope) +
			onbg(t.Dim, t.Bg).Render(" · type ") + onbg(t.Accent, t.Bg).Bold(true).Render(typeScope)
		if echo != "" {
			right += onbg(t.Dim, t.Bg).Render(" · " + echo)
		}
		right += onbg(t.Dim, t.Bg).Render(" ")
	} else if echo != "" {
		right = onbg(t.Dim, t.Bg).Render(echo + " ")
	} else {
		right = onbg(t.Accent, t.Bg).Render("^K") + onbg(t.Faint, t.Bg).Render(" jump or run ")
	}
	return m.barLine(left, right, t.Bg)
}

// subRow is the line under the top bar: a focus underline per pane, or the
// search input over the list when filtering.
func (m Model) subRow() string {
	t := m.theme()
	var left string
	if m.mode == modeFilter {
		left = padTo(m.search.View(), m.listW)
	} else {
		c := t.Edge
		if m.focus == focusList {
			c = t.Accent
		}
		left = fg(c).Render(strings.Repeat("─", m.listW))
	}
	rc := t.Edge
	if m.focus == focusPreview {
		rc = t.Accent
	}
	right := fg(rc).Render(strings.Repeat("─", m.previewW))
	// Paint each side with its pane's surface so the rule row belongs to the
	// panes below it; the ┬ connector sits on the preview side of the divide.
	return paintLine(left, m.listW, t.Bg) +
		paintLine(fg(t.Edge).Render("┬"), 1, t.BgPane) +
		paintLine(right, m.previewW, t.BgPane)
}

func (m Model) bottomRule() string {
	t := m.theme()
	return paintLine(fg(t.Edge).Render(strings.Repeat("─", m.listW)), m.listW, t.Bg) +
		paintLine(fg(t.Edge).Render("┴"), 1, t.BgPane) +
		paintLine(fg(t.Edge).Render(strings.Repeat("─", m.previewW)), m.previewW, t.BgPane)
}

func (m Model) bottomBar() string {
	t := m.theme()
	var left string
	switch {
	case m.mode == modePalette:
		left = t.bar(t.Dim).Render(" ") + t.bar(t.Accent).Render("memories · plans · files · settings · @claude · >team") +
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
	right := m.bottomRight(t)
	return m.barLine(left, right, t.Bg2)
}

// bottomRight is the status bar's right segment (`? help`) — its own method so
// hints() can measure it when deciding how many key hints fit.
func (m Model) bottomRight(t Theme) string {
	return t.bar(t.Fg).Render("?") + t.bar(t.Dim).Render(" help ")
}

// statusStyle picks the footer color for the current status by its kind: danger
// and cancel get their semantic backgrounds, everything else the bar default.
func (m Model) statusStyle(t Theme) lipgloss.Style {
	switch m.statusKind {
	case statusDanger:
		return t.danger()
	case statusCancel:
		return t.cancel()
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

// hints is the contextual left side of the status bar. For the selected row it
// leads with the one offered team action (offeredAction, in the row's sync-state
// color) so `p` never appears when there is nothing to pull; the always-available
// keys follow. Keys render as keycaps — the key on a subtle Sel block over the
// bar — per the prototype. Toggle hints name the target state, not the current
// one ("g group by type" while grouped by project).
func (m Model) hints(t Theme) string {
	keycap := func(key, fgc string) string {
		return fg(fgc).Background(lipgloss.Color(t.Sel)).Render(" " + key + " ")
	}
	// The offered action sits outside the truncation loop: on a narrow terminal
	// the generic keys shed first and the contextual one survives longest.
	offered := ""
	if it, ok := m.selected(); ok {
		if key, verb := offeredAction(it.Sync, it.Scope); key != "" {
			_, c := t.syncBadge(it.Sync)
			offered = fgb(c).Background(lipgloss.Color(t.Sel)).Render(" "+key+" ") +
				t.bar(c).Render(" "+verb+" ")
		}
	}

	// Action keys only, per the prototype — navigation (move / filter / focus /
	// source cycling) lives in the `?` help, where all of it is listed.
	group := "group by type"
	if m.groupBy == groupType {
		group = "group by project"
	}
	var pairs [][2]string
	if m.srcKind == srcPlans {
		pairs = [][2]string{{"d", "delete"}}
	} else if m.srcKind == srcFiles {
		pairs = [][2]string{{"@", "edit via Claude"}}
	} else {
		pairs = [][2]string{
			{"e", "edit"}, {"n", "new"}, {"d", "delete"}, {"t", "type"}, {"g", group},
		}
		if m.driftOut {
			pairs = append(pairs, [2]string{"R", "fix index"})
		}
	}
	pairs = append(pairs, [2]string{"^P", "palette"})
	pairs = append(pairs, [2]string{"q", "quit"})
	render := func(ps [][2]string) string {
		out := t.bar(t.Dim).Render(" ") + offered
		for _, p := range ps {
			out += keycap(p[0], t.Fg) + t.bar(t.Dim).Render(" "+p[1]+" ")
		}
		return out
	}
	out := render(pairs)
	avail := m.width - lipgloss.Width(m.bottomRight(t)) - 1
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
	t := m.theme()
	line := paintLine(fg(t.Edge).Render("│"), 1, t.BgPane)
	lines := make([]string, m.panesH)
	for i := range lines {
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}
