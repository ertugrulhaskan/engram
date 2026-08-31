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
	palPromote                    // > promote the selected memory
	palPull                       // > pull team memories
	palWithdraw                   // > withdraw the selected memory
	palResolve                    // > resolve a conflict
	palInit                       // > init the team store from a git URL (arg)
	palAlias                      // > alias the selected memory's remote-less project (arg)
)

// Palette sections, in display order. Rows are grouped section-contiguously and
// the renderer inserts a header line whenever the section changes — the flat
// palRows list (and so the cursor math) is untouched by sectioning.
const (
	palSecJump      = "Jump to"
	palSecSources   = "Sources"
	palSecTeam      = "Team"
	palSecAssistant = "Assistant"
)

// palItem is one command-palette candidate, rendered as a single-line row:
// section sigil + label on the left, the muted description right-aligned.
type palItem struct {
	glyph      string // section sigil (· / > @)
	glyphColor string // sigil color (hex); "" = accent
	label      string // primary text
	sub        string // muted description, right-aligned
	section    string // palSec* — groups rows under a rendered header
	action     palAction
	src        srcKind
	path       string
	provider   string // assistant provider key for palAssistant ("claude", "gemini", …); "" otherwise
	arg        string // for palInit: the git URL typed after ">init"

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
		case palPromote:
			m.mode = modeNormal
			return m.actionPromote()
		case palPull:
			m.mode = modeNormal
			return m.actionPull()
		case palWithdraw:
			m.mode = modeNormal
			return m.actionWithdraw()
		case palResolve:
			m.mode = modeNormal
			return m.actionResolve()
		case palInit:
			m.mode = modeNormal
			return m.actionInit(sel.arg)
		case palAlias:
			m.mode = modeNormal
			return m.actionAlias(sel.arg)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.palette, cmd = m.palette.Update(msg)
	m.palCursor, m.palTop = 0, 0
	m.rebuildPalette()
	return m, cmd
}

// palCommand is one of the top-level palette commands. Each matches its bare
// name or /slash form by prefix; alias keeps a retired spelling working (the
// "memory" command became "memories", whose prefix diverges at "memor<y|ies>").
type palCommand struct {
	name  string
	alias string
	item  palItem
}

func (m Model) paletteCommands() []palCommand {
	t := m.theme()
	sig := func(it palItem) palItem { it.glyph, it.section = "/", palSecSources; return it }
	return []palCommand{
		{"memories", "memory", sig(palItem{glyphColor: t.TProject, label: "memories", sub: "browse your Claude memories", action: palSwitch, src: srcMemories})},
		{"plans", "", sig(palItem{glyphColor: t.TReference, label: "plans", sub: "browse your plan-mode plans", action: palSwitch, src: srcPlans})},
		{"files", "", sig(palItem{glyphColor: t.TUser, label: "files", sub: "instruction files & MEMORY.md — read-only", action: palSwitch, src: srcFiles})},
		{"settings", "", sig(palItem{glyphColor: t.TFeedback, label: "settings", sub: "open the config file (theme, editor)", action: palSettings})},
	}
}

// matchCommands lists the source/settings commands whose name (or alias)
// starts with q; all of them for an empty q.
func (m Model) matchCommands(q string) []palItem {
	q = strings.ToLower(q)
	var rows []palItem
	for _, c := range m.paletteCommands() {
		if strings.HasPrefix(c.name, q) || (c.alias != "" && strings.HasPrefix(c.alias, q)) {
			rows = append(rows, c.item)
		}
	}
	return rows
}

// teamVerb is one team command reachable via ">" in the palette. Each acts on the
// selected memory (except init) through an actionX method, so the palette owns the
// team verbs that used to be single keys (p / P / w / c).
type teamVerb struct {
	name   string
	item   palItem
	argSub func(arg string) string // for a verb that takes an argument: the row's sub text once one is typed
}

func (m Model) teamVerbs() []teamVerb {
	t := m.theme()
	sig := func(it palItem) palItem { it.glyph, it.section = ">", palSecTeam; return it }
	return []teamVerb{
		{"promote", sig(palItem{glyphColor: t.TProject, label: "promote", sub: "share the selected memory", action: palPromote}), nil},
		{"pull", sig(palItem{glyphColor: t.TReference, label: "pull", sub: "bring team memories into their projects", action: palPull}), nil},
		{"resolve", sig(palItem{glyphColor: t.TFeedback, label: "resolve", sub: "merge a conflicting memory in $EDITOR", action: palResolve}), nil},
		{"withdraw", sig(palItem{glyphColor: t.TUser, label: "withdraw", sub: "take a promoted memory back", action: palWithdraw}), nil},
		{"init", sig(palItem{glyphColor: t.Accent, label: "init", sub: "set up the team store — >init <git-url>", action: palInit}),
			func(arg string) string { return "set up the team store from " + arg }},
		{"alias", sig(palItem{glyphColor: t.TReference, label: "alias", sub: "key a project that has no git remote — >alias <name>", action: palAlias}),
			func(arg string) string {
				if strings.TrimSpace(arg) == "-" {
					return "clear the selected memory's project alias"
				}
				return "key the selected memory's project as " + arg
			}},
	}
}

// matchTeamVerbs lists the team verbs whose name starts with q (all for an
// empty q) — the bare-query route; ">" queries keep their own arg-aware branch.
func (m Model) matchTeamVerbs(q string) []palItem {
	q = strings.ToLower(q)
	var rows []palItem
	for _, v := range m.teamVerbs() {
		if strings.HasPrefix(v.name, q) {
			rows = append(rows, v.item)
		}
	}
	return rows
}

// matchAssistants lists assistant providers whose key starts with q.
func (m Model) matchAssistants(q string) []palItem {
	t := m.theme()
	q = strings.ToLower(q)
	var rows []palItem
	for _, a := range installedAssistants() {
		if strings.HasPrefix(a.key, q) {
			rows = append(rows, palItem{
				glyph: "@", glyphColor: t.Accent, label: a.label, sub: a.sub,
				section: palSecAssistant, action: palAssistant, provider: a.key,
			})
		}
	}
	return rows
}

// palJumpCap bounds the Jump-to section so an empty query over a large corpus
// stays a palette, not a full listing (the sources themselves are the listing).
const palJumpCap = 30

// paletteJumpRows builds the Jump-to section: every memory and plan for an
// empty query (memories first, natural order), or fuzzy matches — over titles
// AND project names — sorted tightest-first for a typed one. Capped at limit.
func (m Model) paletteJumpRows(q string, limit int) []palItem {
	t := m.theme()
	type cand struct {
		it    palItem
		score int
	}
	var cands []cand
	for _, mm := range m.memories {
		score := 0
		if q != "" {
			sc, ok := fuzzyScore(q, mm.Title+" "+mm.Project.Name)
			if !ok {
				continue
			}
			score = sc
		}
		cands = append(cands, cand{palItem{
			glyph: "·", glyphColor: t.typeColor(mm.Type), label: mm.Title, sub: mm.Project.Name,
			section: palSecJump, action: palJump, src: srcMemories, path: mm.Path,
		}, score})
	}
	for _, p := range m.plans {
		score := 0
		if q != "" {
			sc, ok := fuzzyScore(q, p.Title)
			if !ok {
				continue
			}
			score = sc
		}
		cands = append(cands, cand{palItem{
			glyph: "·", glyphColor: t.TReference, label: p.Title, sub: "plan · " + humanizeSince(p.Modified),
			section: palSecJump, action: palJump, src: srcPlans, path: p.Path,
		}, score})
	}
	if q != "" {
		sort.SliceStable(cands, func(i, j int) bool {
			if cands[i].score != cands[j].score {
				return cands[i].score < cands[j].score // tighter match first
			}
			return cands[i].it.label < cands[j].it.label
		})
	}
	if len(cands) > limit {
		cands = cands[:limit]
	}
	rows := make([]palItem, 0, len(cands))
	for _, c := range cands {
		rows = append(rows, c.it)
	}
	return rows
}

// rebuildPalette recomputes candidates as one sectioned list: Jump to, then
// Sources, Team, Assistant. An empty query shows everything (jump capped);
// typing filters every section at once — fuzzy over item titles and project
// names, prefix over command/verb names. The old prefixes survive as section
// scopes: "/" narrows to Sources, ">" to Team (keeping its verb+argument
// parsing), "@" to Assistant. Nothing is reachable only via a prefix.
func (m *Model) rebuildPalette() {
	q := strings.TrimSpace(m.palette.Value())
	var rows []palItem

	switch {
	case strings.HasPrefix(q, "@"):
		rows = m.matchAssistants(strings.TrimSpace(q[1:]))

	case strings.HasPrefix(q, ">"):
		// ">verb [arg]" — filter the team verbs by the first word; init keeps the
		// rest as its git-URL argument.
		rest := strings.TrimSpace(q[1:])
		verb, arg := rest, ""
		if i := strings.IndexAny(rest, " \t"); i >= 0 {
			verb, arg = rest[:i], strings.TrimSpace(rest[i+1:])
		}
		verb = strings.ToLower(verb)
		for _, v := range m.teamVerbs() {
			if verb == "" || strings.HasPrefix(v.name, verb) {
				it := v.item
				if arg != "" && v.argSub != nil {
					it.arg, it.sub = arg, v.argSub(arg)
				}
				rows = append(rows, it)
			}
		}

	case strings.HasPrefix(q, "/"):
		rows = m.matchCommands(strings.TrimPrefix(q, "/"))

	default:
		rows = append(rows, m.paletteJumpRows(q, palJumpCap)...)
		rows = append(rows, m.matchCommands(q)...)
		rows = append(rows, m.matchTeamVerbs(q)...)
		rows = append(rows, m.matchAssistants(q)...)
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
// (each candidate is a single-line row; section headers add up to four more).
const palVisible = 10

// palVisibleRows is the candidate count the palette actually shows, reduced on
// short terminals so the rows plus chrome (header, rule, border, and up to four
// section header lines) still fit within the frame and the box never overflows.
func (m Model) palVisibleRows() int {
	n := m.panesH - 8 // header + rule + 2 border rows + ≤4 section headers
	if n > palVisible {
		n = palVisible
	}
	if n < 1 {
		n = 1
	}
	return n
}

// palLine renders one candidate as a single-line row: the section sigil and
// label on the left, the muted description right-aligned. The selected row is
// an accent bar with dark text. The label clips last; the description clips
// first and vanishes on very narrow boxes.
func (m Model) palLine(c palItem, cw int, panelBg, selBg string) string {
	t := m.theme()
	bg := panelBg
	pri, subc, gcol := t.Fg, t.Dim, c.glyphColor
	if gcol == "" {
		gcol = t.Accent
	}
	if selBg != "" { // bright bar, dark text
		bg = selBg
		pri, subc, gcol = t.Bg2, t.Bg2, t.Bg2
	}
	st := func(col string) lipgloss.Style { return fg(col).Background(lipgloss.Color(bg)) }
	fill := func(n int) string {
		if n <= 0 {
			return ""
		}
		return lipgloss.NewStyle().Background(lipgloss.Color(bg)).Render(spaces(n))
	}

	sw := runewidth.StringWidth(c.glyph)
	label := clip(c.label, cw-sw-4)
	lw := runewidth.StringWidth(label)
	desc := c.sub
	if maxD := cw - sw - lw - 5; runewidth.StringWidth(desc) > maxD {
		if maxD < 4 {
			desc = ""
		} else {
			desc = clip(desc, maxD)
		}
	}
	gap := cw - 3 - sw - lw - runewidth.StringWidth(desc) - 1
	line := fill(1) + st(gcol).Bold(true).Render(c.glyph) + fill(1) +
		st(pri).Bold(selBg != "").Render(label) + fill(gap) + st(subc).Render(desc) + fill(1)
	return padBG(line, cw, bg)
}

// paletteBox renders the command palette as a floating dialog: the input header
// above a sectioned single-line candidate list with a highlighted selection
// bar, on an opaque panel so it sits cleanly over the panes. Section headers
// are render-time lines inserted where the section changes — the cursor only
// ever addresses candidate rows.
func (m Model) paletteBox() string {
	t := m.theme()
	cw := m.boxWidth()
	panel := m.panelBg()
	pst := func(col string) lipgloss.Style { return fg(col).Background(lipgloss.Color(panel)) }

	// Pad the header with the input's own background (Sel) so the field reads as
	// a full-width box reaching the border; the right-side hint appears when the
	// box is wide enough to share the line with the input (see resize).
	left := fg(t.Accent).Background(lipgloss.Color(t.Sel)).Bold(true).Render("engram") +
		fg(t.Dim).Background(lipgloss.Color(t.Sel)).Render(":  ") + m.palette.View()
	right := ""
	if cw >= palHintMinWidth {
		right = fg(t.Faint).Background(lipgloss.Color(t.Sel)).Render(palHint + " ")
	}
	header := padBG(bandLine(left, right, cw, t.Sel), cw, t.Sel)
	lines := []string{header, m.ruleLine(cw)}
	bleed := map[int]string{} // selected row bleeds to the border (see frameLines)

	if len(m.palRows) == 0 {
		lines = append(lines, padBG(pst(t.Dim).Render("  no matches"), cw, panel))
	}
	prevSec := ""
	for i := 0; i < m.palVisibleRows(); i++ {
		idx := m.palTop + i
		if idx < 0 || idx >= len(m.palRows) {
			continue
		}
		c := m.palRows[idx]
		if c.section != "" && c.section != prevSec {
			lines = append(lines, padBG(pst(t.Faint).Render("  "+c.section), cw, panel))
			prevSec = c.section
		}
		selBg := ""
		if idx == m.palCursor {
			selBg = t.Accent
			bleed[len(lines)] = selBg
		}
		lines = append(lines, m.palLine(c, cw, panel, selBg))
	}
	return m.frameLines(lines, cw, t.Accent, bleed)
}

// palHint is the header's right-side nudge; it renders only when the box can
// fit it beside the input (palHintMinWidth, mirrored by resize's input budget).
const (
	palHint         = "type anything — prefixes optional"
	palHintMinWidth = 64
)

// openPalette opens the command palette seeded with a query — "" for the plain
// opener, "@" for the assistant key.
//
// The cursor and scroll offset reset HERE rather than in rebuildPalette, because
// reopening must start at the top while typing must not: updatePalette already
// zeroes them per keystroke, but neither opener did, so the palette reopened
// wherever it was left. Pressing @ after arrowing down twice highlighted the
// third assistant, and ↵ would have launched an AI session the user never chose;
// a leftover palTop scrolled the first rows out of the frame entirely.
func (m *Model) openPalette(q string) tea.Cmd {
	m.mode = modePalette
	m.palette.SetValue(q)
	// SetValue repositions the cursor only when the field was empty or the old
	// position now overruns (bubbles v0.20.0 setValueInternal), so a palette
	// left with its cursor at column 0 would put "@" *after* the caret and turn
	// the next keystrokes into "cla@". Place it explicitly.
	m.palette.CursorEnd()
	m.palCursor, m.palTop = 0, 0
	m.rebuildPalette()
	return m.palette.Focus()
}
