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

// groupMode is the dimension the list is grouped by.
type groupMode int

const (
	groupProject groupMode = iota
	groupType
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

var (
	headerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)
	selTitle     = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)
	normTitle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	descText     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	confirmStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
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

// typeBadge returns a fixed-width bracketed label, e.g. "[project]  ".
func typeBadge(t memory.Type) string {
	label := string(t)
	if label == "" || t == memory.TypeUnknown {
		label = "other"
	}
	return fmt.Sprintf("%-11s", "["+label+"]")
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

// item adapts memory.Memory to the bubbles/list.Item interface.
type item struct {
	mem          memory.Memory
	firstInGroup bool   // first item of its group (gets a header)
	groupCount   int    // group size (set only on the first item)
	groupLabel   string // header text for the group
}

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
	l := list.New(buildGroupedItems(mems, groupProject), newGroupDelegate(), 0, 0)
	l.Title = "engram — memories · by project"
	l.SetShowHelp(true)

	ti := textinput.New()
	ti.Prompt = "New memory title: "
	ti.CharLimit = 120
	ti.Width = 50

	return Model{
		list:     l,
		input:    ti,
		memories: mems,
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
		contentHeight := msg.Height - 3 // 2 borders + 1 footer line
		if contentHeight < 3 {
			contentHeight = 3
		}

		m.list.SetSize(listWidth, contentHeight)
		if !m.ready {
			m.viewport = viewport.New(previewWidth, contentHeight)
			m.ready = true
		} else {
			m.viewport.Width = previewWidth
			m.viewport.Height = contentHeight
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
	left := paneStyle(m.focus == focusList).Render(m.list.View())
	right := paneStyle(m.focus == focusPreview).Render(m.viewport.View())
	panes := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	return lipgloss.JoinVertical(lipgloss.Left, panes, m.footerView())
}

func (m Model) footerView() string {
	w := m.width
	if w <= 0 {
		w = 200
	}
	switch m.mode {
	case modeNewInput:
		return m.input.View()
	case modeConfirmDelete:
		title := ""
		if it, ok := m.list.SelectedItem().(item); ok {
			title = it.mem.Title
		}
		return confirmStyle.Render(truncateStr("Delete \""+title+"\"?  (y/n)", w))
	default:
		text := "n new · e edit · d delete · t type · g group · / search · tab focus · q quit"
		if m.status != "" {
			text = m.status
		}
		return helpStyle.Render(truncateStr(text, w))
	}
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
	m.list.SetItems(buildGroupedItems(sub, m.groupBy))

	title := "engram — memories · "
	if m.groupBy == groupType {
		title += "by type"
	} else {
		title += "by project"
	}
	if tf != "" {
		title += " [" + string(tf) + "]"
	}
	m.list.Title = title
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

// updatePreview renders the selected memory into the viewport.
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

// --- grouping helpers ---

func groupKeyOf(mm memory.Memory, by groupMode) string {
	if by == groupType {
		return string(mm.Type)
	}
	return mm.Project.Name
}

func groupLabelOf(mm memory.Memory, by groupMode) string {
	if by == groupType {
		return typeLabel(mm.Type)
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

// buildGroupedItems wraps memories as list items, marking the first of each
// group and recording the group size on it (memories must already be sorted by
// the same grouping key).
func buildGroupedItems(mems []memory.Memory, by groupMode) []list.Item {
	items := make([]list.Item, len(mems))
	for i, mm := range mems {
		first := i == 0 || groupKeyOf(mems[i-1], by) != groupKeyOf(mm, by)
		items[i] = item{mem: mm, firstInGroup: first, groupLabel: groupLabelOf(mm, by)}
	}
	for i := 0; i < len(mems); {
		j := i
		for j < len(mems) && groupKeyOf(mems[j], by) == groupKeyOf(mems[i], by) {
			j++
		}
		if it, ok := items[i].(item); ok {
			it.groupCount = j - i
			items[i] = it
		}
		i = j
	}
	return items
}

func paneStyle(focused bool) lipgloss.Style {
	s := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	if focused {
		return s.BorderForeground(lipgloss.Color("62"))
	}
	return s.BorderForeground(lipgloss.Color("240"))
}

// --- list item delegate ---

// groupDelegate fully renders each row: an optional colored group header, a
// colored type badge + title with a selection cursor, and a dimmed description.
// Every item is a uniform 3 lines tall (header line is blank when not the first
// of a group), which bubbles/list requires for correct pagination.
type groupDelegate struct {
	list.DefaultDelegate
}

func newGroupDelegate() groupDelegate {
	d := list.NewDefaultDelegate()
	d.SetSpacing(0)
	return groupDelegate{d}
}

func (d groupDelegate) Height() int  { return 3 }
func (d groupDelegate) Spacing() int { return 0 }

func (d groupDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	it, ok := listItem.(item)
	if !ok {
		fmt.Fprint(w, "\n\n")
		return
	}

	width := m.Width()
	if width <= 0 {
		width = 40
	}

	// Line 1: group header (or blank, to keep the row height uniform).
	header := ""
	if it.firstInGroup {
		label := "▌ " + it.groupLabel
		if it.groupCount > 0 {
			label += fmt.Sprintf(" (%d)", it.groupCount)
		}
		header = headerStyle.Render(truncateStr(label, width))
	}

	// Line 2: cursor + colored type badge + title.
	cursor := "  "
	titleStyle := normTitle
	if index == m.Index() {
		cursor = cursorStyle.Render("❯ ")
		titleStyle = selTitle
	}
	badge := lipgloss.NewStyle().Foreground(typeColor(it.mem.Type)).Bold(true).
		Render(typeBadge(it.mem.Type))
	titleBudget := width - 15 // cursor(2) + badge(11) + space(1) + safety(1)
	if titleBudget < 1 {
		titleBudget = 1
	}
	titleLine := cursor + badge + " " + titleStyle.Render(truncateStr(it.mem.Title, titleBudget))

	// Line 3: indented, dimmed description.
	descBudget := width - 5
	if descBudget < 1 {
		descBudget = 1
	}
	descLine := "    " + descText.Render(truncateStr(it.mem.Description, descBudget))

	fmt.Fprintf(w, "%s\n%s\n%s", header, titleLine, descLine)
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
