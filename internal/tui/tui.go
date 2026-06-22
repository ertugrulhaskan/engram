// Package tui implements engram's Bubble Tea terminal UI. It contains no file
// logic; it consumes parsed memories from the memory package.
package tui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

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
	modeNewInput
	modeConfirmDelete
)

// groupMode is the dimension the list is sorted/clustered by.
type groupMode int

const (
	groupProject groupMode = iota
	groupType
)

// Layout constants.
const (
	metaHeaderHeight = 4  // preview card: title, meta, path, rule
	listHeaderHeight = 2  // list: column labels + rule
	typeColW         = 4  // "TYPE" tag column
	rightColW        = 12 // MODIFIED / PROJECT column
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

// Palette. A single cyan accent for engram's chrome (brand, selection, active
// state) over the terminal's own near-black background — no painted backdrop.
// The type colors below stay reserved for memory categories.
var (
	cAccent = lipgloss.Color("45")  // engram cyan — brand & selection
	cInk    = lipgloss.Color("16")  // near-black, for text on the accent
	cBright = lipgloss.Color("231") // brightest text
	cText   = lipgloss.Color("252") // primary text
	cDim    = lipgloss.Color("245") // secondary text
	cFaint  = lipgloss.Color("240") // tertiary text & rules
	cBarBg  = lipgloss.Color("236") // top/bottom bar background
	cChip   = lipgloss.Color("245") // keycap chip background
	cBorder = lipgloss.Color("238") // unfocused pane border
	cDanger = lipgloss.Color("203") // destructive confirm
)

var (
	brandStyle   = lipgloss.NewStyle().Foreground(cInk).Background(cAccent).Bold(true).Padding(0, 1)
	barStyle     = lipgloss.NewStyle().Foreground(cDim).Background(cBarBg)
	bcStyle      = lipgloss.NewStyle().Foreground(cDim).Background(cBarBg)
	statStyle    = lipgloss.NewStyle().Foreground(cText).Background(cBarBg).Bold(true).Padding(0, 1)
	colHeadStyle = lipgloss.NewStyle().Foreground(cDim).Bold(true)
	ruleStyle    = lipgloss.NewStyle().Foreground(cFaint)
	normTitle    = lipgloss.NewStyle().Foreground(cText)
	dimStyle     = lipgloss.NewStyle().Foreground(cDim)
	selStyle     = lipgloss.NewStyle().Foreground(cInk).Background(cAccent).Bold(true)
	metaTitle    = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	metaInfo     = lipgloss.NewStyle().Foreground(cDim)
	metaPath     = lipgloss.NewStyle().Foreground(cFaint).Italic(true)
	keycapStyle  = lipgloss.NewStyle().Foreground(cInk).Background(cChip).Bold(true)
	keyLabel     = lipgloss.NewStyle().Foreground(cDim).Background(cBarBg)
	confirmStyle = lipgloss.NewStyle().Foreground(cDanger).Bold(true).Padding(0, 1)
)

// typeColor maps a memory type to a distinct, readable color.
func typeColor(t memory.Type) lipgloss.Color {
	switch t {
	case memory.TypeUser:
		return lipgloss.Color("75") // blue
	case memory.TypeFeedback:
		return lipgloss.Color("214") // orange
	case memory.TypeProject:
		return lipgloss.Color("42") // green
	case memory.TypeReference:
		return lipgloss.Color("141") // purple
	default:
		return lipgloss.Color("244") // gray
	}
}

func typeLabel(t memory.Type) string {
	switch t {
	case memory.TypeUser:
		return "User"
	case memory.TypeFeedback:
		return "Feedback"
	case memory.TypeProject:
		return "Project"
	case memory.TypeReference:
		return "Reference"
	default:
		return "Other"
	}
}

// typeTag is the fixed-width column code for a memory type.
func typeTag(t memory.Type) string {
	switch t {
	case memory.TypeUser:
		return "USER"
	case memory.TypeFeedback:
		return "FEED"
	case memory.TypeProject:
		return "PROJ"
	case memory.TypeReference:
		return "REF "
	default:
		return "MISC"
	}
}

// item adapts memory.Memory to the bubbles/list.Item interface.
type item struct{ mem memory.Memory }

func (i item) Title() string       { return i.mem.Title }
func (i item) Description() string { return i.mem.Description }
func (i item) FilterValue() string {
	return i.mem.Title + " " + i.mem.Description + " " + i.mem.Project.Name
}

// Model is the root Bubble Tea model.
type Model struct {
	list     list.Model
	viewport viewport.Model
	input    textinput.Model
	renderer *glamour.TermRenderer
	memories []memory.Memory // full set, unfiltered
	home     string          // cached home dir for path prettifying
	typeIdx  int             // index into typeCycle
	groupBy  groupMode
	focus    focus
	mode     mode
	status   string
	width    int
	ready    bool
}

// New builds the initial model from a set of memories.
func New(mems []memory.Memory) Model {
	sorted := append([]memory.Memory(nil), mems...)
	sortForGroup(sorted, groupProject)

	l := list.New(wrapItems(sorted), newRowDelegate(groupProject), 0, 0)
	// engram draws its own chrome (top bar, footer, search, column header), so
	// the list's own title/filter/status/pagination are silenced.
	l.SetShowTitle(false)
	l.SetShowFilter(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.FilterInput.PromptStyle = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	l.FilterInput.Prompt = "search ❯ "

	ti := textinput.New()
	ti.Prompt = "new memory ❯ "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	ti.CharLimit = 120
	ti.Width = 50

	home, _ := os.UserHomeDir()

	return Model{
		list:     l,
		input:    ti,
		memories: mems,
		home:     home,
		focus:    focusList,
		mode:     modeNormal,
		groupBy:  groupProject,
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		listWidth := msg.Width * 2 / 5
		if listWidth < 24 {
			listWidth = 24
		}
		previewWidth := msg.Width - listWidth - 4 // 4 = both panes' borders
		if previewWidth < 20 {
			previewWidth = 20
		}
		// height − top bar(1) − footer(1) − pane top/bottom borders(2).
		contentHeight := msg.Height - 4
		if contentHeight < 6 {
			contentHeight = 6
		}
		listHeight := contentHeight - listHeaderHeight
		if listHeight < 1 {
			listHeight = 1
		}
		viewportHeight := contentHeight - metaHeaderHeight
		if viewportHeight < 1 {
			viewportHeight = 1
		}

		m.list.SetSize(listWidth, listHeight)
		if !m.ready {
			m.viewport = viewport.New(previewWidth, viewportHeight)
			m.ready = true
		} else {
			m.viewport.Width = previewWidth
			m.viewport.Height = viewportHeight
		}
		if w := msg.Width - 20; w > 10 {
			m.input.Width = w
		}
		if r, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(previewWidth-2),
		); err == nil {
			m.renderer = r
		}
		m.updatePreview()
		return m, nil

	case editorFinishedMsg:
		return m, reloadCmd()

	case reloadMsg:
		idx := m.list.Index()
		m.memories = msg.mems
		m.applyFilter()
		if items := m.list.Items(); idx >= 0 && idx < len(items) {
			m.list.Select(idx)
		}
		m.updatePreview()
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case modeNewInput:
			return m.updateNewInput(msg)
		case modeConfirmDelete:
			return m.updateConfirm(msg)
		}
		if handled, nm, cmd := m.handleNormalKey(msg); handled {
			return nm, cmd
		}
	}

	// Default routing.
	if m.mode == modeNewInput {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	if m.focus == focusPreview {
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	prevIndex := m.list.Index()
	m.list, cmd = m.list.Update(msg)
	if m.list.Index() != prevIndex {
		m.updatePreview()
	}
	return m, cmd
}

// handleNormalKey processes keys in normal mode. It returns handled=false to let
// the key fall through to the focused pane (list navigation, filtering, etc.).
func (m Model) handleNormalKey(msg tea.KeyMsg) (bool, tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return true, m, tea.Quit
	}
	if m.list.FilterState() == list.Filtering {
		return false, m, nil
	}

	switch msg.String() {
	case "q":
		return true, m, tea.Quit
	case "tab":
		if m.focus == focusList {
			m.focus = focusPreview
		} else {
			m.focus = focusList
		}
		return true, m, nil
	case "e":
		if it, ok := m.list.SelectedItem().(item); ok {
			return true, m, editCmd(it.mem.Path)
		}
		return true, m, nil
	case "n":
		m.mode = modeNewInput
		m.status = ""
		m.input.SetValue("")
		return true, m, m.input.Focus()
	case "d":
		if _, ok := m.list.SelectedItem().(item); ok {
			m.mode = modeConfirmDelete
		}
		return true, m, nil
	case "t":
		m.typeIdx = (m.typeIdx + 1) % len(typeCycle)
		m.applyFilter()
		return true, m, nil
	case "g":
		if m.groupBy == groupProject {
			m.groupBy = groupType
		} else {
			m.groupBy = groupProject
		}
		m.applyFilter()
		return true, m, nil
	}
	return false, m, nil
}

func (m Model) updateNewInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeNormal
		m.input.Blur()
		m.status = "cancelled"
		return m, nil
	case "enter":
		title := strings.TrimSpace(m.input.Value())
		m.mode = modeNormal
		m.input.Blur()
		if title == "" {
			m.status = "cancelled"
			return m, nil
		}
		memDir := m.currentMemDir()
		if memDir == "" {
			m.status = "no project to add to"
			return m, nil
		}
		path, err := memory.Create(memDir, title)
		if err != nil {
			m.status = "create failed: " + err.Error()
			return m, nil
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
		if it, ok := m.list.SelectedItem().(item); ok {
			if err := memory.Delete(it.mem.Path); err != nil {
				m.status = "delete failed: " + err.Error()
				return m, nil
			}
			m.status = "deleted"
			return m, reloadCmd()
		}
		return m, nil
	default:
		m.mode = modeNormal
		m.status = "cancelled"
		return m, nil
	}
}

func (m Model) View() string {
	if !m.ready {
		return "loading…"
	}
	left := paneStyle(m.focus == focusList).Render(
		lipgloss.JoinVertical(lipgloss.Left, m.listHeaderView(), m.list.View()),
	)
	right := paneStyle(m.focus == focusPreview).Render(
		lipgloss.JoinVertical(lipgloss.Left, m.previewHeaderView(), m.viewport.View()),
	)
	panes := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	return lipgloss.JoinVertical(lipgloss.Left, m.headerView(), panes, m.footerView())
}

// headerView renders the top bar: an accent ENGRAM chip, a breadcrumb of the
// active scope, and a right-aligned counts chip. Content degrades gracefully on
// narrow terminals.
func (m Model) headerView() string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	brand := brandStyle.Render("ENGRAM")

	nMem := len(m.memories)
	seen := map[string]struct{}{}
	for _, mm := range m.memories {
		seen[mm.Project.Name] = struct{}{}
	}

	scope := "BY PROJECT"
	if m.groupBy == groupType {
		scope = "BY TYPE"
	}
	typeScope := "ALL TYPES"
	if tf := typeCycle[m.typeIdx]; tf != "" {
		typeScope = strings.ToUpper(typeLabel(tf))
	}
	query := ""
	if q := strings.TrimSpace(m.list.FilterInput.Value()); q != "" && m.list.FilterState() == list.FilterApplied {
		query = " · “" + q + "”"
	}

	bcFull := scope + " · " + typeScope + query
	bcShort := scope + query
	statFull := fmt.Sprintf("%d MEMORIES · %d PROJECTS", nMem, len(seen))
	statShort := fmt.Sprintf("%d MEM", nMem)

	for _, c := range []struct{ bc, st string }{
		{bcFull, statFull}, {bcFull, statShort}, {bcShort, statShort}, {"", statShort},
	} {
		left := lipgloss.JoinHorizontal(lipgloss.Top, brand, bcStyle.Render("  "+c.bc))
		right := statStyle.Render(c.st)
		gap := w - lipgloss.Width(left) - lipgloss.Width(right)
		if gap >= 0 {
			return lipgloss.JoinHorizontal(lipgloss.Top, left, barStyle.Render(spaces(gap)), right)
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, brand, barStyle.Render(spaces(max(0, w-lipgloss.Width(brand)))))
}

// listHeaderView renders the table column header above the scrolling rows.
func (m Model) listHeaderView() string {
	w := m.list.Width()
	if w <= 0 {
		w = 40
	}
	nameW := w - typeColW - rightColW - 2
	if nameW < 4 {
		nameW = 4
	}
	rightLabel := "MODIFIED"
	if m.groupBy == groupType {
		rightLabel = "PROJECT"
	}
	head := colHeadStyle.Render(fit("TYPE", typeColW)) + " " +
		colHeadStyle.Render(fit("NAME", nameW)) + " " +
		colHeadStyle.Render(fitRight(rightLabel, rightColW))
	rule := ruleStyle.Render(strings.Repeat("─", w))
	return lipgloss.JoinVertical(lipgloss.Left, head, rule)
}

// previewHeaderView is the fixed metadata card above the scrolling preview body.
func (m Model) previewHeaderView() string {
	w := m.viewport.Width
	if w <= 0 {
		w = 40
	}
	it, ok := m.list.SelectedItem().(item)
	if !ok {
		return metaInfo.Render("no memory selected") + strings.Repeat("\n", metaHeaderHeight-1)
	}
	title := metaTitle.Render(truncateStr(it.mem.Title, w))
	tag := lipgloss.NewStyle().Foreground(typeColor(it.mem.Type)).Bold(true).Render(typeTag(it.mem.Type))
	info := tag + "  " + metaInfo.Render(truncateStr(it.mem.Project.Name+"  ·  "+fmtDate(it.mem.Modified), w-6))
	path := metaPath.Render(truncateLeft(m.prettyPath(it.mem.Path), w))
	rule := ruleStyle.Render(strings.Repeat("─", w))
	return lipgloss.JoinVertical(lipgloss.Left, title, info, path, rule)
}

func (m Model) footerView() string {
	w := m.width
	if w <= 0 {
		w = 200
	}
	if m.list.FilterState() == list.Filtering {
		return barFill(m.list.FilterInput.View(), w)
	}
	switch m.mode {
	case modeNewInput:
		return barFill(m.input.View(), w)
	case modeConfirmDelete:
		title := ""
		if it, ok := m.list.SelectedItem().(item); ok {
			title = it.mem.Title
		}
		return confirmStyle.Render(truncateStr("delete “"+title+"” ?  y / n", w-2))
	default:
		if m.status != "" {
			return barFill(barStyle.Render(" "+m.status+" "), w)
		}
		return m.hintsView(w)
	}
}

// hintsView renders the function-key bar, dropping trailing hints to fit.
func (m Model) hintsView(w int) string {
	hints := [][2]string{
		{"↑↓", "NAV"}, {"⇥", "FOCUS"}, {"/", "FIND"}, {"n", "NEW"}, {"e", "EDIT"},
		{"d", "DEL"}, {"t", "TYPE"}, {"g", "GROUP"}, {"q", "QUIT"},
	}
	render := func(hs [][2]string) string {
		parts := make([]string, 0, len(hs)*2)
		for _, h := range hs {
			parts = append(parts,
				keycapStyle.Render(" "+h[0]+" "),
				keyLabel.Render(" "+h[1]+"  "),
			)
		}
		return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	}
	out := render(hints)
	for lipgloss.Width(out) > w && len(hints) > 1 {
		hints = hints[:len(hints)-1]
		out = render(hints)
	}
	return barFill(out, w)
}

// barFill pads a rendered bar segment with the bar background out to width w.
func barFill(seg string, w int) string {
	gap := w - lipgloss.Width(seg)
	if gap < 0 {
		gap = 0
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, seg, barStyle.Render(spaces(gap)))
}

// applyFilter re-sorts and rebuilds the list from m.memories using the active
// type filter and grouping mode.
func (m *Model) applyFilter() {
	tf := typeCycle[m.typeIdx]
	var sub []memory.Memory
	for _, mm := range m.memories {
		if tf == "" || mm.Type == tf {
			sub = append(sub, mm)
		}
	}
	sortForGroup(sub, m.groupBy)
	m.list.SetDelegate(newRowDelegate(m.groupBy))
	m.list.SetItems(wrapItems(sub))
	m.updatePreview()
}

func (m Model) currentMemDir() string {
	if it, ok := m.list.SelectedItem().(item); ok {
		return it.mem.Project.MemoryDir
	}
	if len(m.memories) > 0 {
		return m.memories[0].Project.MemoryDir
	}
	return ""
}

// updatePreview renders the selected memory body into the viewport.
func (m *Model) updatePreview() {
	if !m.ready {
		return
	}
	it, ok := m.list.SelectedItem().(item)
	if !ok {
		m.viewport.SetContent("")
		return
	}
	content := it.mem.Body
	if content == "" {
		content = it.mem.Raw
	}
	if m.renderer != nil {
		if out, err := m.renderer.Render(content); err == nil {
			m.viewport.SetContent(out)
			m.viewport.GotoTop()
			return
		}
	}
	m.viewport.SetContent(content)
	m.viewport.GotoTop()
}

func (m Model) prettyPath(p string) string {
	if m.home != "" && strings.HasPrefix(p, m.home) {
		return "~" + p[len(m.home):]
	}
	return p
}

// --- grouping / items ---

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
		return mems[i].Title < mems[j].Title
	})
}

func wrapItems(mems []memory.Memory) []list.Item {
	items := make([]list.Item, len(mems))
	for i, mm := range mems {
		items[i] = item{mem: mm}
	}
	return items
}

func paneStyle(focused bool) lipgloss.Style {
	s := lipgloss.NewStyle().Border(lipgloss.NormalBorder())
	if focused {
		return s.BorderForeground(cAccent)
	}
	return s.BorderForeground(cBorder)
}

// --- list item delegate ---

// rowDelegate renders each memory as one tabular line: a colored type tag, the
// name, and a right-aligned column that shows the modified date (when clustered
// by project) or the project (when clustered by type). The selected row is a
// solid accent bar. Rows are a uniform single line, as bubbles/list requires.
type rowDelegate struct {
	list.DefaultDelegate
	groupBy groupMode
}

func newRowDelegate(by groupMode) rowDelegate {
	d := list.NewDefaultDelegate()
	d.SetSpacing(0)
	return rowDelegate{DefaultDelegate: d, groupBy: by}
}

func (d rowDelegate) Height() int  { return 1 }
func (d rowDelegate) Spacing() int { return 0 }

func (d rowDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	it, ok := listItem.(item)
	if !ok {
		fmt.Fprint(w, "")
		return
	}
	width := m.Width()
	if width <= 0 {
		width = 40
	}
	nameW := width - typeColW - rightColW - 2
	if nameW < 4 {
		nameW = 4
	}

	tag := typeTag(it.mem.Type)
	name := fit(it.mem.Title, nameW)
	rightRaw := fmtDate(it.mem.Modified)
	if d.groupBy == groupType {
		rightRaw = it.mem.Project.Name
	}
	right := fitRight(rightRaw, rightColW)

	if index == m.Index() {
		// Selected: one solid accent bar, monochrome on the highlight.
		fmt.Fprint(w, selStyle.Width(width).Render(tag+" "+name+" "+right))
		return
	}
	tagStyle := lipgloss.NewStyle().Foreground(typeColor(it.mem.Type)).Bold(true)
	fmt.Fprint(w, tagStyle.Render(tag)+" "+normTitle.Render(name)+" "+dimStyle.Render(right))
}

// --- formatting helpers ---

// fmtDate renders a modification time as a compact ISO date, or "—" if unset.
func fmtDate(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02")
}

func truncateStr(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

func truncateLeft(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return "…" + string(r[len(r)-(n-1):])
}

// fit truncates or right-pads s to exactly w display columns.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) > w {
		if w == 1 {
			return "…"
		}
		return string(r[:w-1]) + "…"
	}
	return s + spaces(w-len(r))
}

// fitRight truncates or left-pads s to exactly w display columns.
func fitRight(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) > w {
		if w == 1 {
			return "…"
		}
		return string(r[:w-1]) + "…"
	}
	return spaces(w-len(r)) + s
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// --- editing ---

type editorFinishedMsg struct{ err error }

func editCmd(path string) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	c := exec.Command(editor, path)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{err: err}
	})
}

// --- reloading after a mutation ---

type reloadMsg struct{ mems []memory.Memory }

func reloadCmd() tea.Cmd {
	return func() tea.Msg {
		mems, err := memory.Discover("")
		if err != nil {
			return reloadMsg{}
		}
		return reloadMsg{mems: mems}
	}
}
