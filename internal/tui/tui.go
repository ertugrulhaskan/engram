// Package tui implements engram's Bubble Tea terminal UI. It contains no file
// logic; it consumes parsed memories from the memory package.
package tui

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/ertughaskan/engram/internal/memory"
)

type focus int

const (
	focusList focus = iota
	focusPreview
)

type mode int

const (
	modeNormal mode = iota
	modeFilter
	modeNew
	modeConfirm
)

type groupMode int

const (
	groupProject groupMode = iota
	groupType
)

const (
	badgeWidth  = 11 // width of the widest "[reference]" badge field
	previewPad  = 2  // left margin between the divider and preview content
	maxReadCols = 88 // cap the prose line length on wide terminals for readability
)

// typeCycle is the order the `t` key steps through. "" means "all types".
var typeCycle = []memory.Type{
	"",
	memory.TypeUser,
	memory.TypeFeedback,
	memory.TypeProject,
	memory.TypeReference,
	memory.TypeUnknown,
}

// rowKind distinguishes the three kinds of display rows in the list.
type rowKind int

const (
	rowMemory rowKind = iota
	rowHeader
	rowSpacer
)

type row struct {
	kind  rowKind
	mem   memory.Memory
	label string // header label
	color string // header color (hex)
	count int    // header group size
}

// Model is the root Bubble Tea model.
type Model struct {
	memories []memory.Memory // full set, unfiltered
	rows     []row           // computed display rows (headers + memories + spacers)
	cursor   int             // index into rows; always points at a rowMemory
	top      int             // first visible row index (scroll offset)

	viewport     viewport.Model
	search       textinput.Model
	input        textinput.Model
	renderer     *glamour.TermRenderer
	previewCache map[string]string // rendered body keyed by path; cleared on resize/theme/reload

	themeIdx  int
	typeIdx   int
	groupBy   groupMode
	focus     focus
	mode      mode
	status    string
	statusSeq int // generation, so an old auto-dismiss timer can't clear a newer status

	width, height           int
	listW, previewW, panesH int // layout, recomputed in resize (sole writer)
	ready                   bool
}

// New builds the initial model from a set of memories.
func New(mems []memory.Memory) Model {
	t := themes[0]

	se := textinput.New()
	se.Prompt = "/ "
	se.PromptStyle = fgb(t.Accent)
	se.Cursor.Style = fg(t.Accent)
	se.CharLimit = 64

	ti := textinput.New()
	ti.Prompt = "› "
	ti.PromptStyle = fgb(t.Accent)
	ti.Cursor.Style = fg(t.Accent)
	ti.CharLimit = 120

	m := Model{
		memories: mems,
		search:   se,
		input:    ti,
		focus:    focusList,
		mode:     modeNormal,
		groupBy:  groupProject,
	}
	m.rebuildRows()
	return m
}

func (m Model) theme() Theme { return themes[m.themeIdx] }

func (m Model) Init() tea.Cmd { return nil }

// clearStatusMsg auto-dismisses a transient status after its timer elapses.
type clearStatusMsg struct{ seq int }

// setStatus shows a transient footer message and returns a command that clears
// it after a short delay (unless a newer status replaces it first).
func (m *Model) setStatus(s string) tea.Cmd {
	m.status = s
	m.statusSeq++
	seq := m.statusSeq
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return clearStatusMsg{seq: seq}
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil

	case editorFinishedMsg:
		if msg.err != nil {
			return m, m.setStatus("editor error: " + msg.err.Error())
		}
		return m, reloadCmd()

	case reloadMsg:
		if msg.err != nil {
			return m, m.setStatus("reload failed: " + msg.err.Error())
		}
		m.memories = msg.mems
		m.previewCache = nil
		m.rebuildRows()
		return m, nil

	case clearStatusMsg:
		if msg.seq == m.statusSeq {
			m.status = ""
		}
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case modeFilter:
			return m.updateFilter(msg)
		case modeNew:
			return m.updateNew(msg)
		case modeConfirm:
			return m.updateConfirm(msg)
		default:
			return m.updateNormal(msg)
		}
	}
	return m, nil
}

func (m *Model) resize(w, h int) {
	m.width, m.height = w, h

	// Split into list | divider(1) | preview so the three always sum to width
	// (no horizontal overflow even on narrow terminals).
	m.listW = w * 2 / 5
	if m.listW < 20 {
		m.listW = 20
	}
	if m.listW > w-2 { // keep previewW >= 1
		m.listW = w - 2
	}
	if m.listW < 1 {
		m.listW = 1
	}
	m.previewW = w - m.listW - 1
	if m.previewW < 1 {
		m.previewW = 1
	}

	// Chrome is 4 lines (top bar, sub row, bottom rule, bottom bar) and we leave
	// the terminal's final row unwritten — filling the very last cell makes some
	// terminals scroll the alt-screen buffer on each repaint, which shows up as
	// blank scrollback with the UI pinned to the bottom. That single reservation
	// is the whole scroll fix; no force-clear or frame clamp needed.
	m.panesH = h - 5
	if m.panesH < 6 {
		m.panesH = 6
	}
	m.search.Width = m.listW - 4
	if m.search.Width < 1 {
		m.search.Width = 1
	}
	m.input.Width = m.previewW
	m.previewCache = nil // width changed — rendered bodies must re-wrap

	vpH := m.panesH - 4 // preview meta header is 4 lines
	if vpH < 1 {
		vpH = 1
	}
	innerW := m.previewW - previewPad
	if innerW < 10 {
		innerW = 10
	}
	if !m.ready {
		m.viewport = viewport.New(innerW, vpH)
		m.ready = true
	} else {
		m.viewport.Width = innerW
		m.viewport.Height = vpH
	}
	m.buildRenderer()
	m.ensureVisible()
	m.syncPreview()
}

func (m *Model) buildRenderer() {
	if m.previewW <= 0 {
		return
	}
	wrap := m.previewW - previewPad
	if wrap > maxReadCols {
		wrap = maxReadCols
	}
	if wrap < 1 {
		wrap = 1
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(m.theme().Glamour),
		glamour.WithWordWrap(wrap),
	)
	if err == nil {
		m.renderer = r
	}
}

// --- normal-mode keys ---

func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Any normal-mode key clears a lingering status (e.g. "deleted"), so the
	// footer reverts to the key hints — the status line is a transient toast.
	m.status = ""
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "1", "2", "3", "4", "5":
		idx := int(msg.String()[0] - '1')
		if idx < len(themes) {
			m.themeIdx = idx
			m.search.PromptStyle = fgb(m.theme().Accent)
			m.input.PromptStyle = fgb(m.theme().Accent)
			m.previewCache = nil // glamour style changed
			m.rebuildRows()
			m.buildRenderer()
			m.syncPreview()
		}
		return m, nil
	case "tab":
		if m.focus == focusList {
			m.focus = focusPreview
		} else {
			m.focus = focusList
		}
		return m, nil
	case "up", "k":
		if m.focus == focusPreview {
			m.viewport.LineUp(1)
		} else {
			m.move(-1)
		}
		return m, nil
	case "down", "j":
		if m.focus == focusPreview {
			m.viewport.LineDown(1)
		} else {
			m.move(1)
		}
		return m, nil
	case "pgup":
		if m.focus == focusPreview {
			m.viewport.HalfViewUp()
		} else {
			m.page(-1)
		}
		return m, nil
	case "pgdown":
		if m.focus == focusPreview {
			m.viewport.HalfViewDown()
		} else {
			m.page(1)
		}
		return m, nil
	case "g":
		if m.groupBy == groupProject {
			m.groupBy = groupType
		} else {
			m.groupBy = groupProject
		}
		m.rebuildRows()
		return m, nil
	case "t":
		m.typeIdx = (m.typeIdx + 1) % len(typeCycle)
		m.rebuildRows()
		return m, nil
	case "/":
		m.mode = modeFilter
		m.focus = focusList
		return m, m.search.Focus()
	case "esc":
		if m.search.Value() != "" {
			m.search.SetValue("")
			m.rebuildRows()
		}
		return m, nil
	case "e":
		if mm, ok := m.selected(); ok {
			return m, editCmd(mm.Path)
		}
		return m, nil
	case "n":
		m.mode = modeNew
		m.input.SetValue("")
		return m, m.input.Focus()
	case "d":
		if _, ok := m.selected(); ok {
			m.mode = modeConfirm
		}
		return m, nil
	}
	return m, nil
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.search.SetValue("")
		m.search.Blur()
		m.rebuildRows()
		return m, nil
	case "enter":
		m.mode = modeNormal
		m.search.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	m.rebuildRows()
	return m, cmd
}

func (m Model) updateNew(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeNormal
		m.input.Blur()
		return m, m.setStatus("cancelled")
	case "enter":
		title := strings.TrimSpace(m.input.Value())
		m.mode = modeNormal
		m.input.Blur()
		if title == "" {
			return m, m.setStatus("cancelled")
		}
		dir := m.currentMemDir()
		if dir == "" {
			return m, m.setStatus("no project to add to")
		}
		path, err := memory.Create(dir, title)
		if err != nil {
			return m, m.setStatus("create failed: " + err.Error())
		}
		return m, editCmd(path)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.mode = modeNormal
		if mm, ok := m.selected(); ok {
			if err := memory.Delete(mm.Path); err != nil {
				return m, m.setStatus("delete failed: " + err.Error())
			}
			return m, tea.Batch(m.setStatus("deleted “"+clip(mm.Title, 40)+"”"), reloadCmd())
		}
		return m, nil
	default:
		m.mode = modeNormal
		return m, m.setStatus("cancelled")
	}
}

// --- list model ---

// rebuildRows recomputes the display rows from memories using the active type
// filter, search query, and grouping, then fixes up the cursor and scroll.
func (m *Model) rebuildRows() {
	tf := typeCycle[m.typeIdx]
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))

	var sub []memory.Memory
	for _, mm := range m.memories {
		if tf != "" && mm.Type != tf {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(mm.Title+" "+mm.Description+" "+mm.Project.Name), q) {
			continue
		}
		sub = append(sub, mm)
	}
	sortForGroup(sub, m.groupBy)

	counts := map[string]int{}
	for _, mm := range sub {
		counts[groupKeyOf(mm, m.groupBy)]++
	}

	var rows []row
	prevKey := "\x00sentinel"
	gIdx := -1
	for _, mm := range sub {
		key := groupKeyOf(mm, m.groupBy)
		if key != prevKey {
			gIdx++
			if len(rows) > 0 {
				rows = append(rows, row{kind: rowSpacer})
			}
			label, color := mm.Project.Name, m.theme().groupColor(gIdx)
			if m.groupBy == groupType {
				label, color = typeLabel(mm.Type), m.theme().typeColor(mm.Type)
			}
			rows = append(rows, row{kind: rowHeader, label: label, color: color, count: counts[key]})
			prevKey = key
		}
		rows = append(rows, row{kind: rowMemory, mem: mm})
	}

	m.rows = rows

	if m.cursor >= len(rows) || m.cursor < 0 || rows[clampIdx(m.cursor, len(rows))].kind != rowMemory {
		m.cursor = m.firstMemRow()
	}
	m.ensureVisible()
	m.syncPreview()
}

func (m *Model) firstMemRow() int {
	for i, r := range m.rows {
		if r.kind == rowMemory {
			return i
		}
	}
	return 0
}

// move steps the cursor by delta, skipping header and spacer rows.
func (m *Model) move(delta int) {
	i := m.cursor
	for {
		j := i + delta
		if j < 0 || j >= len(m.rows) {
			return
		}
		i = j
		if m.rows[i].kind == rowMemory {
			m.cursor = i
			m.ensureVisible()
			m.syncPreview()
			return
		}
	}
}

// page jumps the cursor about one screen in dir (-1 up, +1 down), snapping to
// the nearest memory row.
func (m *Model) page(dir int) {
	if len(m.rows) == 0 {
		return
	}
	h := m.listRows()
	if h < 1 {
		h = 1
	}
	target := m.cursor + dir*h
	if target < 0 {
		target = 0
	}
	if target > len(m.rows)-1 {
		target = len(m.rows) - 1
	}
	j := target
	for j >= 0 && j < len(m.rows) && m.rows[j].kind != rowMemory { // prefer dir
		j += dir
	}
	if j < 0 || j >= len(m.rows) { // fall back to the opposite direction
		for j = target; j >= 0 && j < len(m.rows) && m.rows[j].kind != rowMemory; j -= dir {
		}
	}
	if j >= 0 && j < len(m.rows) && m.rows[j].kind == rowMemory {
		m.cursor = j
		m.ensureVisible()
		m.syncPreview()
	}
}

// shownCount is the number of memory rows currently displayed (post-filter).
func (m Model) shownCount() int {
	n := 0
	for _, r := range m.rows {
		if r.kind == rowMemory {
			n++
		}
	}
	return n
}

func (m *Model) ensureVisible() {
	h := m.listRows()
	if h < 1 {
		return
	}
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+h {
		m.top = m.cursor - h + 1
	}
	// Pull the group header above the cursor into view when it fits.
	if m.cursor > 0 && m.rows[m.cursor-1].kind == rowHeader && m.cursor-1 < m.top {
		m.top = m.cursor - 1
	}
	if m.top < 0 {
		m.top = 0
	}
}

func (m Model) listRows() int { return m.panesH - 1 } // last line is the status

func (m Model) selected() (memory.Memory, bool) {
	if m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].kind == rowMemory {
		return m.rows[m.cursor].mem, true
	}
	return memory.Memory{}, false
}

func (m Model) currentMemDir() string {
	if mm, ok := m.selected(); ok {
		return mm.Project.MemoryDir
	}
	if len(m.memories) > 0 {
		return m.memories[0].Project.MemoryDir
	}
	return ""
}

func (m *Model) syncPreview() {
	if !m.ready {
		return
	}
	mm, ok := m.selected()
	if !ok {
		m.viewport.SetContent("")
		return
	}
	if m.previewCache == nil {
		m.previewCache = map[string]string{}
	}
	if cached, ok := m.previewCache[mm.Path]; ok {
		m.viewport.SetContent(cached)
		m.viewport.GotoTop()
		return
	}
	// Decide the empty-body fallback before stripping, so a body that is only a
	// heading renders as empty rather than falling back to the raw frontmatter.
	body := mm.Body
	if body == "" {
		body = mm.Raw
	}
	body = stripFirstHeading(body)
	rendered := body
	if m.renderer != nil {
		if out, err := m.renderer.Render(body); err == nil {
			rendered = out
		}
	}
	// Glamour pads its output with leading/trailing blank lines; trim them so
	// the viewport only scrolls over real content.
	rendered = trimBlankLines(rendered)
	m.previewCache[mm.Path] = rendered
	m.viewport.SetContent(rendered)
	m.viewport.GotoTop()
}

// trimBlankLines drops leading and trailing all-whitespace lines.
func trimBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// --- view ---

func (m Model) View() string {
	if !m.ready {
		return "loading…"
	}
	var panes string
	if m.mode == modeConfirm {
		panes = m.modalArea(m.confirmModal())
	} else if m.mode == modeNew {
		panes = m.modalArea(m.newModal())
	} else {
		panes = lipgloss.JoinHorizontal(lipgloss.Top, m.listPane(), m.dividerCol(), m.previewPane())
	}
	frame := lipgloss.JoinVertical(lipgloss.Left,
		m.topBar(), m.subRow(), panes, m.bottomRule(), m.bottomBar())
	// Vertical fit is handled by reserving the last row in resize. Horizontally,
	// the bars and glamour's margins can still exceed the width on very narrow
	// terminals, so clip width only (height is already correct — no MaxHeight).
	return lipgloss.NewStyle().MaxWidth(m.width).Render(frame)
}

func (m Model) topBar() string {
	t := m.theme()
	brand := t.bar(t.Accent).Bold(true).Render(" engram ")

	seen := map[string]struct{}{}
	for _, mm := range m.memories {
		seen[mm.Project.Name] = struct{}{}
	}
	info := fmt.Sprintf("%d memories · %d projects", len(m.memories), len(seen))
	if q := strings.TrimSpace(m.search.Value()); q != "" && m.mode != modeFilter {
		info += " · “" + q + "”" // echo the active filter so a narrowed list has a visible reason
	}
	left := brand + t.bar(t.Dim).Render(" "+info+" ")

	scope := "project"
	if m.groupBy == groupType {
		scope = "type"
	}
	typeScope := "all"
	if tf := typeCycle[m.typeIdx]; tf != "" {
		typeScope = string(tf)
	}
	right := t.bar(t.Dim).Render("grouped by ") + t.bar(t.Accent).Bold(true).Render(scope) +
		t.bar(t.Dim).Render("   type ") + t.bar(t.Accent).Bold(true).Render(typeScope) + t.bar(t.Dim).Render(" ")

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
	case m.mode == modeFilter:
		left = t.bar(t.Dim).Render(" type to filter  ") + t.bar(t.Accent).Render("↵") +
			t.bar(t.Dim).Render(" apply   ") + t.bar(t.Accent).Render("esc") + t.bar(t.Dim).Render(" clear ")
	case m.status != "":
		left = t.bar(t.Fg).Render(" " + m.status + " ")
	default:
		left = m.hints(t)
	}
	right := t.bar(t.Dim).Render("theme ") + t.bar(t.Accent).Bold(true).Render(t.Name) +
		t.bar(t.Dim).Render(" · 1–5 to switch ")
	return m.barLine(left, right, t.BarBg)
}

func (m Model) hints(t Theme) string {
	pairs := [][2]string{
		{"↑↓/jk", "move"}, {"/", "filter"}, {"⇥", "focus"}, {"e", "edit"},
		{"n", "new"}, {"d", "delete"}, {"t", "type"}, {"g", "group"}, {"q", "quit"},
	}
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
			lines = append(lines, m.memRow(m.rows[i].mem, i == m.cursor, rightCol))
		}
	}
	shown := m.shownCount()
	if shown == 0 && len(lines) > 0 {
		lines[0] = fg(t.Dim).Render("  no matches")
	}
	body := lipgloss.NewStyle().Width(m.listW).Render(strings.Join(lines, "\n"))
	status := fg(t.Dim).Render(fmt.Sprintf(" %d of %d shown", shown, len(m.memories)))
	return lipgloss.JoinVertical(lipgloss.Left, body, lipgloss.NewStyle().Width(m.listW).Render(status))
}

// rightColW sizes the right-aligned project column (only shown when grouped by
// type), adapting to the longest project name and collapsing to 0 when it would
// leave too little room for the title.
func (m Model) rightColW() int {
	if m.groupBy != groupType {
		return 0
	}
	maxp := 0
	for _, r := range m.rows {
		if r.kind == rowMemory {
			if l := len([]rune(r.mem.Project.Name)); l > maxp {
				maxp = l
			}
		}
	}
	w := maxp + 2 // "· name"
	maxAllowed := m.listW - 2 - badgeWidth - 2 - 12
	if maxAllowed < 6 {
		return 0
	}
	if w > maxAllowed {
		w = maxAllowed
	}
	if w < 6 {
		return 0
	}
	return w
}

func (m Model) headerRow(r row) string {
	t := m.theme()
	suffix := fmt.Sprintf(" (%d)", r.count)
	label := clip(r.label, m.listW-2-runewidth.StringWidth(suffix))
	return fg(r.color).Render("▌ ") + fgb(r.color).Render(label) + fg(t.Dim).Render(suffix)
}

func (m Model) memRow(mm memory.Memory, selected bool, rightCol int) string {
	t := m.theme()
	badge := padRight("["+typeName(mm.Type)+"]", badgeWidth)

	rightText := ""
	if rightCol > 0 {
		rightText = "· " + mm.Project.Name
	}
	nameW := m.listW - 2 - badgeWidth - 1 - rightCol
	if rightCol > 0 {
		nameW--
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
	out := indent + st(t.typeColor(mm.Type)).Render(badge) + st(t.Fg).Render(" ") + st(titleColor).Render(padRight(mm.Title, nameW))
	if rightCol > 0 {
		out += st(t.Fg).Render(" ") + st(t.Dim).Render(padLeft(rightText, rightCol))
	}
	return out
}

func (m Model) previewPane() string {
	t := m.theme()
	innerW := m.previewW - previewPad
	mm, ok := m.selected()
	if !ok {
		return lipgloss.NewStyle().Width(m.previewW).Height(m.panesH).Render(fg(t.Dim).Render("  no memory selected"))
	}
	badgeStr := "[" + typeName(mm.Type) + "]"
	rest := " " + mm.Project.Name + " · edited " + humanizeSince(mm.Modified)
	rest = clip(rest, innerW-runewidth.StringWidth(badgeStr))
	meta := fg(t.typeColor(mm.Type)).Bold(true).Render(badgeStr) + fg(t.Dim).Render(rest)
	title := m.renderTitle(mm.Title, innerW)
	block := lipgloss.JoinVertical(lipgloss.Left, meta, "", title, "", m.viewport.View())
	return lipgloss.NewStyle().PaddingLeft(previewPad).Render(block)
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

// modalArea centers a modal box within the panes region.
func (m Model) modalArea(box string) string {
	return lipgloss.Place(m.width, m.panesH, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) confirmModal() string {
	t := m.theme()
	title := ""
	if mm, ok := m.selected(); ok {
		title = mm.Title
	}
	body := lipgloss.JoinVertical(lipgloss.Left,
		fg(t.Fg).Render("Delete this memory?"),
		fgb(t.Danger).Render(clip(title, 44)),
		"",
		fg(t.Dim).Render("press ")+fgb(t.Danger).Render("y")+fg(t.Dim).Render(" to delete  ·  ")+
			fgb(t.Fg).Render("n")+fg(t.Dim).Render(" to cancel"),
	)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(t.Danger)).
		Padding(1, 3).Render(body)
}

func (m Model) newModal() string {
	t := m.theme()
	body := lipgloss.JoinVertical(lipgloss.Left,
		fgb(t.Accent).Render("New memory"),
		fg(t.Dim).Render("title for the new memory in this project"),
		"",
		m.input.View(),
		"",
		fg(t.Dim).Render("press ")+fgb(t.Accent).Render("↵")+fg(t.Dim).Render(" create  ·  ")+
			fgb(t.Fg).Render("esc")+fg(t.Dim).Render(" cancel"),
	)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(t.Accent)).
		Padding(1, 3).Render(body)
}

// --- grouping helpers ---

func groupKeyOf(mm memory.Memory, by groupMode) string {
	if by == groupType {
		return string(mm.Type)
	}
	return mm.Project.Name
}

func sortForGroup(mems []memory.Memory, by groupMode) {
	sort.SliceStable(mems, func(i, j int) bool {
		ki, kj := groupKeyOf(mems[i], by), groupKeyOf(mems[j], by)
		if ki != kj {
			return ki < kj
		}
		// Within a project group, cluster by type before falling back to title.
		if by == groupProject {
			if ri, rj := typeRank(mems[i].Type), typeRank(mems[j].Type); ri != rj {
				return ri < rj
			}
		}
		return mems[i].Title < mems[j].Title
	})
}

// typeRank is the within-group ordering of memory types: project, feedback,
// user, reference, then everything else.
func typeRank(t memory.Type) int {
	switch t {
	case memory.TypeProject:
		return 0
	case memory.TypeFeedback:
		return 1
	case memory.TypeUser:
		return 2
	case memory.TypeReference:
		return 3
	default:
		return 4
	}
}

func typeLabel(t memory.Type) string {
	switch t {
	case memory.TypeUser:
		return "user"
	case memory.TypeFeedback:
		return "feedback"
	case memory.TypeProject:
		return "project"
	case memory.TypeReference:
		return "reference"
	default:
		return "other"
	}
}

// typeName is the badge label for a type.
func typeName(t memory.Type) string {
	if t == memory.TypeUnknown || t == "" {
		return "other"
	}
	return string(t)
}

// --- text helpers ---

func fg(c string) lipgloss.Style  { return lipgloss.NewStyle().Foreground(lipgloss.Color(c)) }
func fgb(c string) lipgloss.Style { return fg(c).Bold(true) }

func humanizeSince(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 28*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	default:
		return t.Format("Jan 2, 2006")
	}
}

// stripFirstHeading removes a leading "# ..." line (and a following blank) so
// the preview's own title isn't duplicated by the rendered body.
func stripFirstHeading(body string) string {
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		s := strings.TrimSpace(ln)
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "# ") {
			rest := lines[i+1:]
			for len(rest) > 0 && strings.TrimSpace(rest[0]) == "" {
				rest = rest[1:]
			}
			return strings.Join(rest, "\n")
		}
		break
	}
	return body
}

func clampIdx(i, n int) int {
	if i < 0 {
		return 0
	}
	if i >= n {
		if n == 0 {
			return 0
		}
		return n - 1
	}
	return i
}

// clip truncates s to at most w display columns (measuring wide runes
// correctly), appending an ellipsis when it had to cut.
func clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= w {
		return s
	}
	return runewidth.Truncate(s, w, "…")
}

// padRight clips s to w display columns then right-pads to exactly w.
func padRight(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = clip(s, w)
	return s + spaces(w-runewidth.StringWidth(s))
}

// padLeft clips s to w display columns then left-pads to exactly w.
func padLeft(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = clip(s, w)
	return spaces(w-runewidth.StringWidth(s)) + s
}

// padTo right-pads a possibly-styled string to width w (display columns).
func padTo(s string, w int) string {
	gap := w - lipgloss.Width(s)
	if gap < 0 {
		return s
	}
	return s + spaces(gap)
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

// --- editing ---

type editorFinishedMsg struct{ err error }

func editCmd(path string) tea.Cmd {
	parts := resolveEditor()
	args := append([]string{}, parts[1:]...)
	args = append(args, path)
	c := exec.Command(parts[0], args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{err: err}
	})
}

// resolveEditor picks the command (and any args) used to open a memory for
// editing. It honors $VISUAL and $EDITOR first (the Unix convention), then opens
// in the host editor when engram is run inside one (e.g. VS Code's integrated
// terminal — using --wait so the edit completes before the list reloads), then
// a terminal editor, falling back to vi.
func resolveEditor() []string {
	if v := strings.TrimSpace(os.Getenv("VISUAL")); v != "" {
		return strings.Fields(v)
	}
	if v := strings.TrimSpace(os.Getenv("EDITOR")); v != "" {
		return strings.Fields(v)
	}
	if os.Getenv("TERM_PROGRAM") == "vscode" {
		if c := firstInPath("code", "code-insiders", "cursor", "codium"); c != "" {
			return []string{c, "--wait"}
		}
	}
	if c := firstInPath("nvim", "vim", "nano", "vi"); c != "" {
		return []string{c}
	}
	return []string{"vi"}
}

// firstInPath returns the first of names found on $PATH, or "".
func firstInPath(names ...string) string {
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

// --- reloading after a mutation ---

type reloadMsg struct {
	mems []memory.Memory
	err  error
}

func reloadCmd() tea.Cmd {
	return func() tea.Msg {
		mems, err := memory.Discover("")
		return reloadMsg{mems: mems, err: err}
	}
}
