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
		m.tabsRow(), m.controlsRow(), m.headerRule(), panes, m.bottomBar())

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
	case modePullConfirm:
		box = m.pullModal()
	case modeResolveConfirm:
		box = m.resolveModal()
	case modeReconcileConfirm:
		box = m.reconcileModal()
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
	// repaint. Normally the layout already yields exactly m.height-reservedRows
	// lines; this only bites on a very short terminal, where panesH is floored
	// above h-chromeRows.
	return clampFrameHeight(clampFrame(frame, m.width), m.height)
}

// clampFrameHeight drops any lines past the terminal's height budget so the
// frame can never be taller than the screen. It keeps at most h-reservedRows
// lines, leaving the terminal's final row unwritten (see chromeRows: writing
// the last cell scrolls the alt-screen on some terminals). Shorter frames pass
// through untouched.
func clampFrameHeight(frame string, h int) string {
	limit := h - reservedRows
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

// tabsRow is the first header row: source tabs on the left, app identity on the
// right, all on one shared Bg2 band across the full width.
func (m Model) tabsRow() string {
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
	left := t.bar(t.Faint).Render(" ")
	for i, tab := range tabs {
		if i > 0 {
			left += t.bar(t.Bg2).Render("  ")
		}
		if tab.kind == m.srcKind {
			left += onbg(t.Fg, t.Sel).Bold(true).Render("  "+tab.label+"  ") +
				onbg(t.Accent, t.Sel).Bold(true).Render(fmt.Sprintf("%d  ", tab.count))
		} else {
			left += t.bar(t.Fg).Bold(true).Render("  "+tab.label+"  ") +
				t.bar(t.Faint).Render(fmt.Sprintf("%d  ", tab.count))
		}
	}
	right := t.bar(t.Accent).Bold(true).Render("engram")
	if m.version != "" {
		// Clip: release versions are short ("v0.2.1"), but a dev build carries a
		// VCS pseudo-version long enough to crowd the header chrome.
		right += t.bar(t.Dim).Render(" " + clip(m.version, 20))
	}
	return m.barLine(left, right, t.Bg2)
}

// controlsRow is the second header row: list-shaping controls fixed to the
// left pane's width, with the palette affordance on the far right. The whole
// row shares the header's Bg2 surface across both panes.
func (m Model) controlsRow() string {
	t := m.theme()
	var left string
	if m.mode == modeFilter {
		left = padTo(m.search.View(), m.listW)
	} else {
		// Chips render bare — label in Dim, value in Fg/Accent — per review
		// feedback (the contained Sel blocks read too heavy next to the
		// keycaps). A chip's value reads accent when its shaping is active
		// (a non-default type filter or grouping); bold marks list focus.
		chip := func(label, val string, active bool) string {
			c := t.Fg
			if active {
				c = t.Accent
			}
			return onbg(t.Dim, t.Bg2).Render(" "+label+": ") +
				onbg(c, t.Bg2).Bold(m.focus == focusList).Render(val+" ")
		}
		var chips string
		switch m.srcKind {
		case srcMemories:
			typeScope := "all"
			if tf := typeCycle[m.typeIdx]; tf != "" {
				// typeLabel, not string(tf): the chip names the same filter the row
				// badges and the mark status line name, and it renders TypeUnknown as
				// "other" rather than leaking the internal "unknown".
				typeScope = typeLabel(tf)
			}
			group := "project"
			if m.groupBy == groupType {
				group = "type"
			}
			chips = chip("type", typeScope, typeScope != "all") + onbg(t.Faint, t.Bg2).Render(" ") +
				chip("group", group, group != "project")
		case srcPlans:
			chips = chip("group", "recency", false)
		default:
			chips = chip("group", "project", false)
		}
		// Search affordance as a keycap — "press / to search", the status bar's
		// key idiom — clipped against what the chips leave over. A committed
		// query replaces the word so a narrowed list keeps its visible reason.
		hint := "search"
		if q := strings.TrimSpace(m.search.Value()); q != "" {
			hint = "“" + q + "”"
		}
		avail := m.listW - lipgloss.Width(chips) - 6 // 1 gap + " / " keycap + 1 space each side
		var right string
		if avail >= 4 {
			right = onbg(t.Fg, t.Sel).Render(" / ") + onbg(t.Faint, t.Bg2).Render(" "+clip(hint, avail)+" ")
		}
		left = bandLine(chips, right, m.listW, t.Bg2)
	}
	right := t.bar(t.Accent).Render("^K") + t.bar(t.Faint).Render(" jump or run anything ")
	return m.barLine(left, right, t.Bg2)
}

// headerRule sits between the header block and the panes — both panes' top
// border on one shared row so the left and right sides align. Each side is
// its pane's focus underline: accent when that pane has focus, Edge
// otherwise; the ┬ connector joins them over the divider.
func (m Model) headerRule() string {
	t := m.theme()
	lc, rc := t.Edge, t.Edge
	if m.focus == focusList {
		lc = t.Accent
	} else {
		rc = t.Accent
	}
	return paintLine(fg(lc).Render(strings.Repeat("─", m.listW)), m.listW, t.Bg2) +
		paintLine(fg(t.Edge).Render("┬"), 1, t.Bg2) +
		paintLine(fg(rc).Render(strings.Repeat("─", m.previewW)), m.previewW, t.Bg2)
}

func (m Model) bottomBar() string {
	t := m.theme()
	var left string
	switch {
	case m.mode == modePalette:
		left = t.bar(t.Dim).Render(" ") + t.bar(t.Accent).Render("memories · plans · files · settings · @assistant · >team") +
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
	// Padding rows above and below give the footer air (its "padding-top/
	// bottom" in terminal cells); resize budgets panesH for all footerRows.
	pad := m.barLine("", "", t.Bg2)
	return pad + "\n" + m.barLine(left, right, t.Bg2) + "\n" + pad
}

// bottomRight is the status bar's right segment: theme switcher and help, so
// hints() can measure it when deciding how many key hints fit.
func (m Model) bottomRight(t Theme) string {
	return t.bar(t.Dim).Render("theme ") +
		t.bar(t.Accent).Bold(true).Render(t.Name) +
		t.bar(t.Dim).Render(" · 1–3 switch · ") +
		t.bar(t.Fg).Render("?") +
		t.bar(t.Dim).Render(" help ")
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
		if key, verb := offeredAction(it.Sync); key != "" {
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
	// The action keys are derived from the source's capabilities, so the bar
	// can only advertise what the handlers will actually do — the offer.go
	// rule: never advertise an action that can't run.
	var pairs [][2]string
	caps := m.caps()
	if caps.Edit {
		pairs = append(pairs, [2]string{"e", "edit"})
	}
	if caps.Create {
		pairs = append(pairs, [2]string{"n", "new"})
	}
	if caps.Delete {
		pairs = append(pairs, [2]string{"d", "delete"})
	}
	if m.srcKind == srcMemories {
		// Type and group are memories' data model (types, projects), not a
		// capability — the other sources have neither to filter or group by.
		pairs = append(pairs, [2]string{"t", "type"}, [2]string{"g", group})
		if m.driftOut {
			// Same verb as the banner chip — two wordings for one key is noise.
			pairs = append(pairs, [2]string{"R", "reconcile"})
		}
	}
	if readOnlyHint[m.srcKind] != "" {
		// A source that names an escape hatch for its denied edits advertises it
		// here. Keyed on the hint alone, not on !Edit, so a source wrongly
		// carrying both shows both keys — visible drift, not a masked one.
		pairs = append(pairs, [2]string{"@", "edit via assistant"})
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

// barLine lays out a full-width bar with a left and right segment over a
// filled background — bandLine at the terminal's width.
func (m Model) barLine(left, right, bg string) string {
	return bandLine(left, right, m.width, bg)
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
