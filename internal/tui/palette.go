package tui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// palAction is what selecting a command-palette candidate does.
type palAction int

const (
	palSwitch    palAction = iota // switch source (src)
	palJump                       // switch source and select path
	palSettings                   // open the settings dialog
	palAssistant                  // launch an AI assistant session (@Claude …)
	palPrefix                     // seed a prefix ("/" or "@") into the input to reveal its menu
)

// palItem is one command-palette candidate, rendered as a two-line row (primary
// label + muted subtitle) with a right-aligned pill, à la Warp's palette.
type palItem struct {
	glyph      string // leading icon
	glyphColor string // icon color (hex)
	label      string // primary line
	sub        string // secondary muted line
	right      string // right-aligned pill (slash form, or item type)
	rightColor string // pill color (hex); "" = dim
	action     palAction
	src        srcKind
	path       string
	provider   string // assistant provider for palAssistant ("claude"); "" otherwise
	prefix     string // for palPrefix: text seeded into the input ("/" or "@")
}

// --- command palette ---

func (m Model) updatePalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+p":
		m.mode = modeNormal
		m.palette.Blur()
		return m, nil
	case "up", "ctrl+k":
		if m.palCursor > 0 {
			m.palCursor--
		}
		if m.palCursor < m.palTop {
			m.palTop = m.palCursor
		}
		return m, nil
	case "down", "ctrl+j":
		if m.palCursor < len(m.palRows)-1 {
			m.palCursor++
		}
		if vis := m.palVisibleRows(); m.palCursor >= m.palTop+vis {
			m.palTop = m.palCursor - vis + 1
		}
		return m, nil
	case "enter":
		if m.palCursor < 0 || m.palCursor >= len(m.palRows) {
			m.mode = modeNormal
			m.palette.Blur()
			return m, nil
		}
		sel := m.palRows[m.palCursor]
		m.palette.Blur()
		switch sel.action {
		case palSwitch:
			m.mode = modeNormal
			m.switchSource(sel.src)
		case palJump:
			m.mode = modeNormal
			m.switchSource(sel.src)
			// Clear any in-list filter so the chosen item isn't hidden from the
			// rows (switchSource only clears it when the source actually changes).
			if m.search.Value() != "" {
				m.search.SetValue("")
				m.rebuildRows()
			}
			m.selectByPath(sel.path)
		case palSettings:
			return m, m.openSettingsFile()
		case palAssistant:
			m.mode = modeNormal
			return m, m.assistantCmd(sel.provider)
		case palPrefix:
			// A guide row ("/" or "@") seeds its prefix so the next keystrokes
			// filter that menu — equivalent to typing the prefix by hand. The
			// input was blurred above; re-focus so typing continues.
			m.palette.SetValue(sel.prefix)
			m.palette.CursorEnd()
			m.palCursor, m.palTop = 0, 0
			m.rebuildPalette()
			return m, m.palette.Focus()
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.palette, cmd = m.palette.Update(msg)
	m.palCursor, m.palTop = 0, 0
	m.rebuildPalette()
	return m, cmd
}

// palCommand is one of the three top-level palette commands. Each matches its
// bare name or /slash form (so "mem", "/mem", "memory" all hit memory).
type palCommand struct {
	name string
	item palItem
}

func (m Model) paletteCommands() []palCommand {
	t := m.theme()
	return []palCommand{
		{"memory", palItem{glyph: "◆", glyphColor: t.TProject, label: "memory", sub: "Browse your Claude memories", right: "/memory", action: palSwitch, src: srcMemories}},
		{"plans", palItem{glyph: "▣", glyphColor: t.TReference, label: "plans", sub: "Browse your plan-mode plans", right: "/plans", action: palSwitch, src: srcPlans}},
		{"files", palItem{glyph: "▤", glyphColor: t.TUser, label: "files", sub: "CLAUDE.md & MEMORY.md (read-only; edit via @Claude)", right: "/files", action: palSwitch, src: srcFiles}},
		{"settings", palItem{glyph: "◈", glyphColor: t.TFeedback, label: "settings", sub: "Open the config file (theme, editor)", right: "/settings", action: palSettings}},
	}
}

// palProvider is one AI assistant reachable via "@" in the palette. Today only
// Claude Code exists; the registry keeps adding another (Phase 3) a one-line change.
type palProvider struct {
	key   string // matched against the text after "@"
	label string // primary line, e.g. "@Claude"
	sub   string // secondary muted line
}

func (m Model) assistantProviders() []palProvider {
	return []palProvider{
		{key: "claude", label: "@Claude", sub: "Fix & edit memories/plans with Claude Code"},
	}
}

// rebuildPalette recomputes candidates. A leading "@" is assistant-only (mirrors
// how a leading "/" is command-only); otherwise commands (memory/plans/settings)
// match a bare or /slashed prefix and the remaining text fuzzy-jumps across item
// titles in both sources, so a query can surface a command and matches together.
func (m *Model) rebuildPalette() {
	t := m.theme()
	q := strings.TrimSpace(m.palette.Value())
	cmdq := strings.ToLower(strings.TrimPrefix(q, "/"))
	var rows []palItem

	// Empty palette is a guide: two rows pointing at the two entry points ("/"
	// for commands, "@" for the assistant). Each seeds its prefix on Enter.
	if q == "" {
		rows = []palItem{
			{glyph: "/", glyphColor: t.Accent, label: "commands",
				sub:   "Browse memories, plans, files & settings",
				right: "type /", rightColor: t.Dim, action: palPrefix, prefix: "/"},
			{glyph: "@", glyphColor: t.Accent, label: "assistant",
				sub:   "Fix & edit memories/plans with Claude Code",
				right: "type @", rightColor: t.Dim, action: palPrefix, prefix: "@"},
		}
		m.palRows = rows
		if m.palCursor >= len(rows) || m.palCursor < 0 {
			m.palCursor = 0
		}
		if m.palTop > m.palCursor {
			m.palTop = m.palCursor
		}
		return
	}

	if strings.HasPrefix(q, "@") {
		pq := strings.ToLower(strings.TrimSpace(q[1:]))
		for _, p := range m.assistantProviders() {
			if pq == "" || strings.HasPrefix(p.key, pq) {
				rows = append(rows, palItem{
					glyph: "✦", glyphColor: t.Accent,
					label: p.label, sub: p.sub,
					right: "assistant", rightColor: t.Accent,
					action: palAssistant, provider: p.key,
				})
			}
		}
		m.palRows = rows
		if m.palCursor >= len(rows) || m.palCursor < 0 {
			m.palCursor = 0
		}
		if m.palTop > m.palCursor {
			m.palTop = m.palCursor
		}
		return
	}

	for _, c := range m.paletteCommands() {
		if strings.HasPrefix(c.name, cmdq) {
			rows = append(rows, c.item)
		}
	}

	// A bare word also fuzzy-jumps to items (a leading "/" means command-only).
	if q != "" && !strings.HasPrefix(q, "/") {
		type cand struct {
			it    palItem
			score int
		}
		var cands []cand
		for _, mm := range m.memories {
			if sc, ok := fuzzyScore(q, mm.Title); ok {
				it := palItem{glyph: "•", glyphColor: t.typeColor(mm.Type), label: mm.Title,
					sub: "memory · " + mm.Project.Name, right: string(mm.Type), rightColor: t.typeColor(mm.Type),
					action: palJump, src: srcMemories, path: mm.Path}
				cands = append(cands, cand{it, sc})
			}
		}
		for _, p := range m.plans {
			if sc, ok := fuzzyScore(q, p.Title); ok {
				it := palItem{glyph: "•", glyphColor: t.TReference, label: p.Title,
					sub: "plan · " + humanizeSince(p.Modified), right: "plan", rightColor: t.TReference,
					action: palJump, src: srcPlans, path: p.Path}
				cands = append(cands, cand{it, sc})
			}
		}
		sort.SliceStable(cands, func(i, j int) bool {
			if cands[i].score != cands[j].score {
				return cands[i].score < cands[j].score // tighter match first
			}
			return cands[i].it.label < cands[j].it.label
		})
		for _, c := range cands {
			rows = append(rows, c.it)
		}
	}

	m.palRows = rows
	if m.palCursor >= len(rows) || m.palCursor < 0 {
		m.palCursor = 0
	}
	if m.palTop > m.palCursor {
		m.palTop = m.palCursor
	}
}

// palVisible caps how many palette candidates the floating dialog shows at once
// (each candidate is a two-line row).
const palVisible = 6

// palVisibleRows is the candidate count the palette actually shows, reduced on
// short terminals so the two-line rows plus chrome (header, rule, border) still
// fit within the frame and the box never overflows past the bottom.
func (m Model) palVisibleRows() int {
	n := (m.panesH - 2) / 2 // ~4 chrome lines; 2 lines per candidate
	if n > palVisible {
		n = palVisible
	}
	if n < 1 {
		n = 1
	}
	return n
}

// palRow renders one candidate as a two-line Warp-style row (icon + label + pill
// over a muted subtitle). Every cell carries panelBg; the selected row is filled
// with selBg and dark text instead, like the highlighted entry in the screenshot.
func (m Model) palRow(c palItem, cw int, panelBg, selBg string) []string {
	t := m.theme()
	sel := selBg != ""
	bg := panelBg
	pri, subc, rc, gcol := t.Fg, t.Dim, c.rightColor, c.glyphColor
	if rc == "" {
		rc = t.Dim
	}
	if gcol == "" {
		gcol = t.Accent
	}
	if sel { // bright bar, dark text
		bg = selBg
		pri, subc, rc, gcol = t.BarBg, t.BarBg, t.BarBg, t.BarBg
	}
	st := func(col string) lipgloss.Style {
		return fg(col).Background(lipgloss.Color(bg))
	}
	fill := func(n int) string {
		if n <= 0 {
			return ""
		}
		return lipgloss.NewStyle().Background(lipgloss.Color(bg)).Render(spaces(n))
	}

	gw := runewidth.StringWidth(c.glyph)
	rightW := runewidth.StringWidth(c.right)
	before := 1 + gw + 2 // leading space + glyph + two spaces
	labelMax := cw - before - rightW - 2
	if labelMax < 4 {
		labelMax = 4
	}
	label := clip(c.label, labelMax)
	gap := cw - before - runewidth.StringWidth(label) - rightW - 1
	if gap < 1 {
		gap = 1
	}
	line1 := fill(1) + st(gcol).Bold(true).Render(c.glyph) + fill(2) +
		st(pri).Bold(true).Render(label) + fill(gap)
	if rightW > 0 {
		line1 += st(rc).Render(c.right)
	}
	line1 = padBG(line1+fill(1), cw, bg)

	line2 := padBG(fill(5)+st(subc).Render(clip(c.sub, cw-6)), cw, bg)
	return []string{line1, line2}
}

// paletteBox renders the command palette as a floating Warp-style dialog: a
// search header above two-line candidate rows with a highlighted selection bar,
// on an opaque panel so it sits cleanly over the panes.
func (m Model) paletteBox() string {
	t := m.theme()
	cw := m.boxWidth()
	panel := m.panelBg()
	pst := func(col string) lipgloss.Style { return fg(col).Background(lipgloss.Color(panel)) }

	header := padBG(pst(t.Accent).Bold(true).Render("engram")+pst(t.Dim).Render(":  ")+m.palette.View(), cw, panel)
	lines := []string{header, m.ruleLine(cw)}

	if len(m.palRows) == 0 {
		lines = append(lines, padBG(pst(t.Dim).Render("  no matches"), cw, panel))
	}
	for i := 0; i < m.palVisibleRows(); i++ {
		idx := m.palTop + i
		if idx < 0 || idx >= len(m.palRows) {
			continue
		}
		selBg := ""
		if idx == m.palCursor {
			selBg = t.Accent
		}
		lines = append(lines, m.palRow(m.palRows[idx], cw, panel, selBg)...)
	}
	return m.dialogFrame(strings.Join(lines, "\n"), t.Accent)
}
