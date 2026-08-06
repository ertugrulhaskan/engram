package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/ertugrulhaskan/engram/internal/team"
)

func (m Model) listPane() string {
	t := m.theme()
	h := m.listRows() - m.bannerRows()               // the drift banner takes the first row from the scroll window
	badgeW := m.badgeColW()                          // widest type badge in view; computed once per render
	syncW := m.syncColW()                            // right-aligned sync-pill width (0 when nothing in view is shared)
	scopeW := m.scopeColW()                          // scope chip width (0 when nothing in view is shared)
	rightCol := m.rightColW(badgeW + scopeW + syncW) // Right column budget accounts for the left/right furniture
	// On a narrow pane the fixed furniture can crowd out the title and push the sync
	// pill past the edge (clampFrame would then cut it mid-glyph). Shed the least-
	// critical columns first — the muted scope chip, then the Right column — so the
	// color-coded sync pill and a readable title always fit.
	scopeW, rightCol = m.fitFurniture(badgeW, scopeW, syncW, rightCol)
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
			lines = append(lines, m.memRow(m.rows[i].item, i == m.cursor, badgeW, scopeW, syncW, rightCol))
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
	// Paint every list line to exactly listW over the theme surface — the
	// selected row's Sel fill carries its own background on top of it
	// (paintLine leaves explicit backgrounds alone).
	body := paintBlock(strings.Join(lines, "\n"), m.listW, t.Bg)
	status := paintLine(fg(t.Dim).Render(fmt.Sprintf(" %d of %d shown", shown, total)), m.listW, t.Bg)
	if it, ok := m.driftBannerItem(); ok {
		return lipgloss.JoinVertical(lipgloss.Left, m.driftBanner(it), body, status)
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, status)
}

// driftBannerItem reports whether the drift banner shows, and for which item.
// It shows only in the memories source (R is dead elsewhere — plans have no
// index, files are read-only) for the selected project, until esc-dismissed.
func (m Model) driftBannerItem() (Item, bool) {
	if m.srcKind != srcMemories || !m.driftOut {
		return Item{}, false
	}
	it, ok := m.selected()
	if !ok || it.MemDir == "" || it.MemDir != m.driftDir || m.driftDismissed[it.MemDir] {
		return Item{}, false
	}
	return it, true
}

// bannerRows is how many list-pane rows the drift banner occupies (0 or 1) —
// the single source the scroll window (listPane, ensureVisible, page) shrinks
// by. On a degenerate short pane the list keeps its rows and the banner yields.
func (m Model) bannerRows() int {
	if _, ok := m.driftBannerItem(); ok && m.listRows() >= 2 {
		return 1
	}
	return 0
}

// driftBanner renders the WarnBg warning row that sits above the list: named
// project, named cause, one action. The row must hold exactly w cells (an
// over-wide line would wrap and grow the row), so what doesn't fit sheds in a
// fixed order — the project name first (the group header below repeats it),
// then the chip (R stays offered in the status bar), and only then does the
// cause itself clip. Shedding is monotonic: nothing returns at narrower widths.
func (m Model) driftBanner(it Item) string {
	t := m.theme()
	w := m.listW
	cause := driftCause(len(m.driftUnindexed), len(m.driftDangling))
	full := it.Context + ": " + cause
	avail := w - 3 - 1 // " △ " lead and a trailing cell
	chipW := runewidth.StringWidth("[R reconcile]") + 2
	chipTxt := "[R reconcile]"
	clipped := full
	switch {
	case runewidth.StringWidth(full)+chipW <= avail:
	case runewidth.StringWidth(cause)+chipW <= avail:
		clipped = cause
	case runewidth.StringWidth(cause) <= avail:
		clipped, chipTxt = cause, ""
	default:
		clipped, chipTxt = clip(cause, avail), ""
	}
	// Tint the index's name inside the already-clipped sentence — styling after
	// clipping keeps the width math exact (each cause names MEMORY.md once).
	sentR := onbg(t.Fg, t.WarnBg).Render(clipped)
	if i := strings.Index(clipped, "MEMORY.md"); i >= 0 {
		sentR = onbg(t.Fg, t.WarnBg).Render(clipped[:i]) +
			onbg(t.Warn, t.WarnBg).Render("MEMORY.md") +
			onbg(t.Fg, t.WarnBg).Render(clipped[i+len("MEMORY.md"):])
	}
	left := onbg(t.Warn, t.WarnBg).Render(" △ ") + sentR
	right := ""
	if chipTxt != "" {
		right = onbg(t.Warn, t.WarnBg).Bold(true).Render(chipTxt) +
			onbg(t.Warn, t.WarnBg).Render(" ")
	}
	return paintLine(bandLine(left, right, w, t.WarnBg), w, t.WarnBg)
}

// driftCause names what is out of sync between a project's memory files and
// its MEMORY.md index — both directions when both apply, never a generic
// reconcile prompt.
func driftCause(unindexed, dangling int) string {
	plural := func(n int, one, many string) string {
		if n == 1 {
			return one
		}
		return many
	}
	switch {
	case unindexed > 0 && dangling > 0:
		return fmt.Sprintf("%d %s no line in MEMORY.md · %d %s no file",
			unindexed, plural(unindexed, "file has", "files have"),
			dangling, plural(dangling, "line has", "lines have"))
	case unindexed > 0:
		return fmt.Sprintf("%d %s no line in MEMORY.md",
			unindexed, plural(unindexed, "file has", "files have"))
	default:
		return fmt.Sprintf("%d MEMORY.md %s no file",
			dangling, plural(dangling, "line has", "lines have"))
	}
}

// badgeColW sizes the type-badge field from the widest badge in the current
// (filtered) list, so short badges (e.g. "user") don't leave a wide gap before
// the title. The badge renders as a bare colored word per the prototype.
// Capped at badgeWidth (the widest possible "reference"); 0 when no row has one.
func (m Model) badgeColW() int {
	w := 0
	for _, r := range m.rows {
		if r.kind == rowMemory && r.item.Badge != "" {
			if l := runewidth.StringWidth(r.item.Badge); l > w {
				w = l
			}
		}
	}
	if w > badgeWidth {
		w = badgeWidth
	}
	return w
}

// syncColW is the width of the right-aligned team-sync pill column: the widest
// bracketed state word ("[conflict]") in view. Measured with the same runewidth
// oracle memRow pads with, so the column can't drift. Collapses to 0 when
// nothing in view carries a sync state, so the feature is invisible for anyone
// not using team sharing.
func (m Model) syncColW() int {
	w := 0
	for _, r := range m.rows {
		if r.kind == rowMemory && r.item.SyncBadge != "" {
			if l := runewidth.StringWidth("[" + r.item.SyncBadge + "]"); l > w {
				w = l
			}
		}
	}
	return w
}

// scopeColW sizes the muted scope chip ("global" / "project") from the widest
// scope label in view. Like syncColW it collapses to 0 when nothing shared is in
// view, so it's invisible outside team sharing. It's dim text, not a filled pill,
// so it stays secondary to the color-coded sync state next to it.
func (m Model) scopeColW() int {
	w := 0
	for _, r := range m.rows {
		if r.kind == rowMemory {
			if l := runewidth.StringWidth(r.item.Scope); l > w {
				w = l
			}
		}
	}
	return w
}

// minTitleW is the least title width a memory row will hold before the renderer
// starts shedding right-side furniture to make room.
const minTitleW = 8

// fitFurniture sheds the least-critical right-side columns — the muted scope chip
// first, then the Right column — until a row's fixed furniture leaves at least
// minTitleW cells for the title. The furniture sum mirrors memRow's nameW math
// (indent + padded badge + each column + its leading gap), so the sync pill is
// never pushed past the pane edge and cut by clampFrame.
func (m Model) fitFurniture(badgeW, scopeW, syncW, rightCol int) (int, int) {
	furniture := func() int {
		w := 2 // indent
		if badgeW > 0 {
			w += badgeW + 1
		}
		if rightCol > 0 {
			w += rightCol + 1
		}
		if scopeW > 0 {
			w += scopeW + 1
		}
		if syncW > 0 {
			w += syncW + 1
		}
		return w
	}
	for m.listW-furniture() < minTitleW {
		switch {
		case scopeW > 0:
			scopeW = 0
		case rightCol > 0:
			rightCol = 0
		default:
			return scopeW, rightCol // badge + pill alone already fill the pane; memRow floors the title
		}
	}
	return scopeW, rightCol
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

func (m Model) memRow(it Item, selected bool, badgeW, scopeW, syncW, rightCol int) string {
	t := m.theme()

	badgeCol := 0
	if it.Badge != "" {
		badgeCol = badgeW + 1 // padded badge + trailing space
	}
	nameW := m.listW - 2 - badgeCol - rightCol - scopeW - syncW
	if rightCol > 0 {
		nameW-- // gap before the right column
	}
	if scopeW > 0 {
		nameW-- // gap before the scope chip
	}
	if syncW > 0 {
		nameW-- // gap before the sync pill
	}
	if nameW < 4 {
		nameW = 4
	}

	// Selection: an accent chevron + bold accent title over a Sel row highlight.
	// The highlight is safe from ghost-cell bleed because clampFrame now closes
	// every line's background (a glamour code chip could otherwise leave a bg open
	// and smear across rows) — the row fill itself was never the leak.
	bg := ""
	if selected {
		bg = t.Sel
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
		out += st(it.BadgeColor).Render(padRight(it.Badge, badgeW)) + st(t.Fg).Render(" ")
	}
	titleStyle := st(titleColor)
	if selected {
		titleStyle = titleStyle.Bold(true)
	}
	out += titleStyle.Render(padRight(it.Title, nameW))
	if rightCol > 0 {
		out += st(t.Fg).Render(" ") + st(t.Dim).Render(padLeft(it.Right, rightCol))
	}
	if scopeW > 0 {
		out += st(t.Fg).Render(" ")
		if it.Scope != "" {
			c := it.ScopeColor
			if c == "" {
				c = t.Dim
			}
			out += st(c).Render(padLeft(it.Scope, scopeW)) // teal global / blue project, right-aligned against the pill
		} else {
			out += st(t.Fg).Render(padRight("", scopeW)) // blank, keeps the pill edge aligned
		}
	}
	if syncW > 0 {
		out += st(t.Fg).Render(" ")
		if it.SyncBadge != "" {
			// An outlined pill — the bracketed word in the state color — bold on
			// the selected row so its state reads first. (The spec dims unselected
			// pills to 75% opacity; bold-vs-regular is the terminal stand-in.)
			pill := st(it.SyncColor)
			if selected {
				pill = pill.Bold(true)
			}
			out += pill.Render(padLeft("["+it.SyncBadge+"]", syncW))
		} else {
			out += st(t.Fg).Render(padRight("", syncW)) // blank, keeps the right edge aligned
		}
	}
	return out
}

// shortPath is the preview meta's compact location — the prototype's
// "engram/memory/tui-layering.md" shape: project name, the file's parent dir
// base, and the filename, derived from the real path (never assembled from
// assumptions about the layout).
func shortPath(it Item) string {
	if it.Path == "" {
		return ""
	}
	base := filepath.Base(it.Path)
	if it.Context == "" {
		return base
	}
	if dir := filepath.Base(filepath.Dir(it.Path)); dir != "." && dir != string(filepath.Separator) {
		return it.Context + "/" + dir + "/" + base
	}
	return it.Context + "/" + base
}

func (m Model) previewPane() string {
	t := m.theme()
	innerW := m.previewW - previewPad
	it, ok := m.selected()
	if !ok {
		empty := lipgloss.NewStyle().Width(m.previewW).Height(m.panesH).Render(fg(t.Dim).Render("  nothing selected"))
		return paintBlock(empty, m.previewW, t.BgPane)
	}
	// Header per the prototype: the title first, the meta line right under it —
	// badge word, shortened path, scope pill. The sync state itself is NOT in
	// the meta: the sync strip directly below states it in a full sentence for
	// every stateful memory. The edited stamp stays only where no strip follows
	// (personal rows), so the time is never lost.
	meta, used := "", 0
	if it.Badge != "" {
		meta = fg(it.BadgeColor).Bold(true).Render(it.Badge) + " "
		used = runewidth.StringWidth(it.Badge) + 1
	}
	scopePill, scopeUsed := "", 0
	if it.Scope != "" {
		sc := it.ScopeColor
		if sc == "" {
			sc = t.Dim
		}
		pill := "[" + it.Scope + "]"
		scopePill = " " + fg(sc).Bold(true).Render(pill)
		scopeUsed = runewidth.StringWidth(pill) + 1
	}
	rest := shortPath(it)
	if m.stripRows(it) == 0 {
		stamp := "edited " + humanizeSince(it.Modified)
		if rest == "" {
			rest = stamp
		} else {
			rest += " · " + stamp
		}
	}
	meta += fg(t.Dim).Render(clip(rest, innerW-used-scopeUsed)) + scopePill
	title := m.renderTitle(it.Title, innerW)
	// The header and body are padded blocks; the sync strip band between them
	// renders full-bleed (its own Bg2 rows span the pane edge to edge), so it
	// reads as a strip, not indented text. Total header rows: 3, or 6 with the
	// band — syncPreview shrinks the viewport to match (stripRows).
	head := lipgloss.NewStyle().PaddingLeft(previewPad).Width(m.previewW).
		Render(lipgloss.JoinVertical(lipgloss.Left, title, meta, ""))
	parts := []string{head}
	if band := m.syncStrip(it); len(band) > 0 {
		parts = append(parts, strings.Join(band, "\n"), "")
	}
	parts = append(parts, lipgloss.NewStyle().PaddingLeft(previewPad).Width(m.previewW).
		Render(m.viewport.View()))
	block := lipgloss.JoinVertical(lipgloss.Left, parts...)
	// Width(previewW) so every preview line fills the pane — otherwise the joined
	// frame has ragged line widths and a floated dialog leaves stale cells.
	// Height+MaxHeight pin the pane to exactly panesH lines: a long preview can
	// never push the whole frame past the terminal height. An overflowing frame
	// scrolls the alt-screen, which desyncs Bubble Tea's line-diff renderer and
	// leaves ghost rows (a trailing highlight) until the next full repaint.
	rendered := lipgloss.NewStyle().Width(m.previewW).
		Height(m.panesH).MaxHeight(m.panesH).Render(block)
	// Paint the whole pane BgPane; glamour's own fills (code blocks, chips)
	// sit on top, and the reset-reassert in paintLine closes their bleed.
	return paintBlock(rendered, m.previewW, t.BgPane)
}

// syncStrip is the preview's sync band for a memory with team state: a Bg2 band
// under the title carrying the spec's plain sentence with the offered action as
// a chip, then the direction gauge (`you ▬▬▬ ← ▬▬▬ team` — the side that moved
// is filled in the state color, conflict fills both) with the honest timestamp.
// Timestamps degrade by omission: "store advanced" appears only once git has
// answered (storeTimes); missing and unknown state their facts instead.
func (m Model) syncStrip(it Item) []string {
	if m.stripRows(it) == 0 {
		return nil
	}
	t := m.theme()
	w := m.previewW
	_, c := t.syncBadge(it.Sync)

	// Row 1 budget: pads + dot + sentence + ≥1 gap + chip. Over-wide content
	// must never leave here — lipgloss's Width() wraps long lines, which would
	// grow the band and push the body down. The sentence clips first; the chip
	// is dropped only when a pane is too narrow to share the row at all.
	chipTxt := ""
	if key, verb := offeredAction(it.Sync, it.Scope); key != "" {
		chipTxt = "[" + key + " " + verb + "]"
	}
	sentAvail := w - 2*previewPad - 2 - runewidth.StringWidth(chipTxt) - 1
	if chipTxt != "" && sentAvail < 12 {
		chipTxt = ""
		sentAvail = w - 2*previewPad - 2
	}
	if sentAvail < 1 {
		sentAvail = 1
	}
	left1 := onbg(c, t.Bg2).Render(spaces(previewPad)+"● ") +
		onbg(t.Fg, t.Bg2).Render(clip(stateSentence(it.Sync), sentAvail))
	right1 := ""
	if chipTxt != "" {
		right1 = onbg(c, t.Bg2).Bold(true).Render(chipTxt) +
			onbg(t.Dim, t.Bg2).Render(spaces(previewPad))
	}

	youC, teamC := t.Edge, t.Edge
	switch it.Sync {
	case team.StateIncoming, team.StateMissing:
		teamC = c
	case team.StateLocalAhead:
		youC = c
	case team.StateDiverged:
		youC, teamC = c, c
	}
	left2 := onbg(t.Dim, t.Bg2).Render(spaces(previewPad)+"you ") +
		onbg(youC, t.Bg2).Render("▬▬▬") +
		onbg(c, t.Bg2).Render(" "+gaugeGlyph(it.Sync)+" ") +
		onbg(teamC, t.Bg2).Render("▬▬▬") +
		onbg(t.Dim, t.Bg2).Render(" team")
	right2 := ""
	// The stamp clips to what the gauge leaves over, and vanishes rather than
	// crowding it on very narrow panes.
	if stamp := clip(m.stripStamp(it), w-lipgloss.Width(left2)-previewPad-1); stamp != "" && runewidth.StringWidth(stamp) >= 4 {
		right2 = onbg(t.Dim, t.Bg2).Render(stamp + spaces(previewPad))
	}

	return []string{bandLine(left1, right1, w, t.Bg2), bandLine(left2, right2, w, t.Bg2)}
}

// stripStamp is the sync strip's timestamp, honest per state: local knowledge
// covers synced/ahead ("edited") and conflict ("diverged"); behind needs the
// store side's last commit time and is omitted until git has answered; missing
// and unknown have no time to claim, so they state the fact instead.
func (m Model) stripStamp(it Item) string {
	switch it.Sync {
	case team.StateSynced, team.StateLocalAhead:
		return "edited " + humanizeSince(it.Modified)
	case team.StateDiverged:
		return "diverged " + humanizeSince(it.Modified)
	case team.StateIncoming:
		if ts, ok := m.storeTimes[it.SyncID]; ok {
			return "store advanced " + humanizeSince(ts)
		}
		return "" // not fetched (or the lookup failed) — never guessed
	case team.StateMissing:
		return "not in store"
	case team.StateDiffers:
		return "no anchor"
	default:
		return ""
	}
}

// bandLine lays a left and right segment over a bg-filled row of exactly w
// cells — barLine's shape, but for a pane-width band rather than the full bar.
func bandLine(left, right string, w int, bg string) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	mid := lipgloss.NewStyle().Background(lipgloss.Color(bg)).Render(spaces(gap))
	return left + mid + right
}

// renderTitle styles the preview title in the accent color, with `code` spans
// shown as inline chips.
func (m Model) renderTitle(title string, w int) string {
	t := m.theme()
	title = clip(title, w)
	var b strings.Builder
	for i, part := range strings.Split(title, "`") {
		if i%2 == 1 {
			b.WriteString(fg(t.Fg).Background(lipgloss.Color(t.Sel)).Render(part))
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

// panelBg is the fill behind the floating dialogs: the theme's pane surface,
// so dialogs read as opaque panels over the (scrim-dimmed) page, per the
// design spec. Every shared fill helper (padBG, ruleLine, the per-modal
// background styles) threads this value, so the panel is opaque everywhere.
// The rounded corner glyphs render over the fill and therefore read
// near-square — a terminal cell can't clip sub-cell corners; accepted
// deviation from the prototype's border radius.
func (m Model) panelBg() string { return m.theme().BgPane }

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
	// Border rows carry the panel fill too — an unpainted corner or side cell
	// would punch a hole to whatever the (dimmed) page renders beneath the box.
	if p := m.panelBg(); p != "" {
		a = a.Background(lipgloss.Color(p))
	}
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

// confirmModal is the delete confirmation, in the shared dialog anatomy: it
// names the target, its path, and — for memories — the index consequence.
func (m Model) confirmModal() string {
	t := m.theme()
	it, _ := m.selected()
	kind := "memory"
	body := []string{it.Title, it.Path}
	if it.Kind == "plan" {
		kind = "plan"
	} else {
		body = append(body, "Its line in MEMORY.md is removed too.")
	}
	return m.dialog("✕", "delete this "+kind+"?", t.Danger,
		body, []dialogAction{{"n cancel", false}, {"y delete", true}})
}

// newModal is the new-memory title prompt: shared anatomy around a live input.
func (m Model) newModal() string {
	t := m.theme()
	cw := m.boxWidth()
	panel := m.panelBg()
	lines := m.dlgHeader(cw, "+", "new memory", t.Accent)
	lines = append(lines, m.dlgText(cw, "title for the new memory in this project", t.Dim)...)
	lines = append(lines,
		padBG("", cw, panel),
		padBG("  "+m.input.View(), cw, panel),
		padBG("", cw, panel),
	)
	bleed := map[int]string{len(lines): t.Bg2}
	lines = append(lines, m.dlgFooter(cw, t.Accent, []dialogAction{{"esc cancel", false}, {"↵ create", true}}))
	return m.frameLines(lines, cw, t.Accent, bleed)
}
