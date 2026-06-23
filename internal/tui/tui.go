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
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/plan"
)

// palAction is what selecting a command-palette candidate does.
type palAction int

const (
	palSwitch   palAction = iota // switch source (src)
	palJump                      // switch source and select path
	palSettings                  // open the settings dialog
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
}

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
	modePalette
)

// srcKind selects which collection is being browsed.
type srcKind int

const (
	srcMemories srcKind = iota
	srcPlans
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
	item  Item
	label string // header label
	color string // header color (hex)
	count int    // header group size
}

// Item is the source-agnostic unit the list and preview render. Each source
// (memories, plans) maps its records into Items; presentation (badge text,
// colors, group labels) is resolved by the mapper so the renderers stay generic.
type Item struct {
	Title    string
	Body     string // markdown for the preview
	Raw      string // fallback when Body is empty
	Path     string // identity: selection, edit, delete
	Modified time.Time

	Badge      string // bracket label, e.g. "user"/"project"; "" = no badge column
	BadgeColor string // hex
	GroupKey   string // "" = flat (no group headers)
	GroupLabel string // header text for the first row of a group
	GroupColor string // header color (hex)
	Right      string // right-aligned column text (project when grouped by type, or date)
	Context    string // preview meta context (project name, or "plan")
	MemDir     string // memory dir for new/index/drift; "" for plans
	Kind       string // "memory" | "plan" — palette tag + feature gating
}

// Model is the root Bubble Tea model.
type Model struct {
	memories []memory.Memory // full memory set, unfiltered
	plans    []plan.Plan     // full plan set
	srcKind  srcKind         // which source is being browsed
	cursors  [2]int          // remembered cursor (row index) per source

	rows   []row // computed display rows (headers + items + spacers)
	cursor int   // index into rows; always points at a rowMemory
	top    int   // first visible row index (scroll offset)

	viewport     viewport.Model
	search       textinput.Model
	palette      textinput.Model // command palette input (Ctrl+P)
	palRows      []palItem       // palette candidates
	palCursor    int             // selected palette candidate
	palTop       int             // first visible palette candidate (scroll)
	input        textinput.Model // new-memory title
	renderer     *glamour.TermRenderer
	previewCache map[string]string // rendered body keyed by path; cleared on resize/theme/reload

	themeIdx       int
	editorOverride string // optional editor command from config; "" = use env/host
	typeIdx        int
	groupBy        groupMode
	focus          focus
	mode           mode
	status         string
	statusSeq      int    // generation, so an old auto-dismiss timer can't clear a newer status
	fsSig          string // last-seen filesystem fingerprint; "" until the first poll baselines it
	driftDir       string // memory dir the drift flag was computed for (cache key)
	driftOut       bool   // selected project's MEMORY.md is out of sync with its files

	width, height           int
	listW, previewW, panesH int // layout, recomputed in resize (sole writer)
	ready                   bool
}

// New builds the initial model from the discovered memories and plans, applying
// persisted settings (theme, editor override).
func New(mems []memory.Memory, plans []plan.Plan, cfg config.Config) Model {
	themeIdx := 0
	for i, th := range themes {
		if th.Name == cfg.Theme {
			themeIdx = i
			break
		}
	}
	t := themes[themeIdx]

	se := textinput.New()
	se.Prompt = "/ "
	se.PromptStyle = fgb(t.Accent)
	se.Cursor.Style = fg(t.Accent)
	se.CharLimit = 64

	pal := textinput.New()
	pal.Prompt = "" // the box header renders the "engram:" label
	pal.Placeholder = "Search memories, plans, settings…"
	pal.CharLimit = 64

	ti := textinput.New()
	ti.Prompt = "› "
	ti.CharLimit = 120
	ti.Width = 44 // bound the visible field so dialogs stay dialog-sized

	m := Model{
		memories:       mems,
		plans:          plans,
		themeIdx:       themeIdx,
		editorOverride: strings.TrimSpace(cfg.Editor),
		search:         se,
		palette:        pal,
		input:          ti,
		focus:          focusList,
		mode:           modeNormal,
		groupBy:        groupProject,
	}
	m.styleInputs()
	m.rebuildRows()
	return m
}

func (m Model) theme() Theme { return themes[m.themeIdx] }

// styleInputs (re)applies theme colors to the text inputs. The palette and
// new-memory inputs live inside opaque dialogs, so their text/placeholder/cursor
// carry the panel background; the filter input sits on the normal surface.
func (m *Model) styleInputs() {
	t := m.theme()
	panel := lipgloss.Color(t.SelBg)
	m.search.PromptStyle = fgb(t.Accent)
	m.search.Cursor.Style = fg(t.Accent)
	m.palette.PlaceholderStyle = fg(t.Dim).Background(panel)
	m.palette.TextStyle = fg(t.Fg).Background(panel)
	m.palette.Cursor.Style = fg(t.Accent).Background(panel)
	m.input.PromptStyle = fgb(t.Accent).Background(panel)
	m.input.TextStyle = fg(t.Fg).Background(panel)
	m.input.Cursor.Style = fg(t.Accent).Background(panel)
}

// setTheme switches the active theme by index, restyles inputs, re-renders, and
// persists the choice (best-effort) so it survives restarts.
func (m *Model) setTheme(idx int) {
	if idx < 0 || idx >= len(themes) {
		return
	}
	m.themeIdx = idx
	m.styleInputs()
	m.previewCache = nil // glamour style changed
	m.rebuildRows()
	m.buildRenderer()
	m.syncPreview()
	_ = config.Save(config.Config{Theme: m.theme().Name, Editor: m.editorOverride})
}

func (m Model) Init() tea.Cmd { return pollCmd() }

// pollInterval is how often engram re-scans the filesystem for external changes.
const pollInterval = 2 * time.Second

// pollResultMsg carries the latest filesystem fingerprint from a poll tick.
type pollResultMsg struct {
	sig string
	err error
}

// combinedSig fingerprints both sources into one string, so a change in either
// tree flips it. One baseline (m.fsSig) covers both — no second baseline (that
// would risk a reload loop).
func combinedSig() (string, error) {
	ms, err := memory.Signature("")
	ps, _ := plan.Signature("")
	return ms + "|" + ps, err
}

// pollCmd schedules the next filesystem scan. The closure runs in the command
// goroutine, so the scan never blocks the event loop. It is the only thing that
// re-arms the poll loop (started once in Init, re-armed once per pollResultMsg).
func pollCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg {
		sig, err := combinedSig()
		return pollResultMsg{sig: sig, err: err}
	})
}

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
		// Editing the config file (via /settings): re-read it and apply theme +
		// editor, rather than treating it as a memory.
		if cp, _ := config.Path(); msg.path != "" && msg.path == cp {
			cfg := config.Load()
			m.editorOverride = strings.TrimSpace(cfg.Editor)
			for i, th := range themes {
				if th.Name == cfg.Theme {
					m.setTheme(i)
					break
				}
			}
			return m, m.setStatus("settings updated")
		}
		// Keep the folder's MEMORY.md index in sync with the new/edited file
		// (best-effort; the reload reflects the files regardless).
		if msg.path != "" {
			_ = memory.UpsertIndexForPath(msg.path)
		}
		return m, reloadCmd()

	case reloadMsg:
		if msg.err != nil {
			return m, m.setStatus("reload failed: " + msg.err.Error())
		}
		// Capture the selection before rebuilding (rebuildRows re-clamps the
		// cursor by index), then restore it by path so a background reload
		// doesn't make the selection jump.
		prevPath := ""
		if mm, ok := m.selected(); ok {
			prevPath = mm.Path
		}
		m.memories = msg.mems
		m.plans = msg.plans
		m.fsSig = msg.sig
		m.previewCache = nil
		m.driftDir = "" // index may have changed — recompute on next syncPreview
		m.rebuildRows()
		if prevPath != "" {
			m.selectByPath(prevPath)
		}
		return m, nil

	case clearStatusMsg:
		if msg.seq == m.statusSeq {
			m.status = ""
		}
		return m, nil

	case pollResultMsg:
		// The poll loop re-arms here and nowhere else.
		switch {
		case msg.err != nil:
			// Transient FS error — ignore so the footer doesn't churn.
		case m.fsSig == "":
			m.fsSig = msg.sig // first poll: adopt the baseline, don't reload
		case msg.sig != m.fsSig && m.mode != modeNew && m.mode != modeConfirm && m.mode != modePalette:
			// Changed on disk and no modal is open → reload. Don't update fsSig
			// here; reloadMsg sets it atomically with the new memories.
			return m, tea.Batch(reloadCmd(), pollCmd())
		}
		return m, pollCmd()

	case tea.KeyMsg:
		switch m.mode {
		case modeFilter:
			return m.updateFilter(msg)
		case modeNew:
			return m.updateNew(msg)
		case modeConfirm:
			return m.updateConfirm(msg)
		case modePalette:
			return m.updatePalette(msg)
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
		m.setTheme(idx)
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
		if m.srcKind != srcMemories {
			return m, nil
		}
		if m.groupBy == groupProject {
			m.groupBy = groupType
		} else {
			m.groupBy = groupProject
		}
		m.rebuildRows()
		return m, nil
	case "t":
		if m.srcKind != srcMemories {
			return m, nil
		}
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
		if m.srcKind != srcMemories { // plans are view + delete only
			return m, nil
		}
		if mm, ok := m.selected(); ok {
			return m, m.editCmd(mm.Path)
		}
		return m, nil
	case "n":
		if m.srcKind != srcMemories {
			return m, nil
		}
		m.mode = modeNew
		m.input.SetValue("")
		if w := m.boxWidth() - 4; w > 8 {
			m.input.Width = w
		}
		return m, m.input.Focus()
	case "d":
		if _, ok := m.selected(); ok {
			m.mode = modeConfirm
		}
		return m, nil
	case "ctrl+p":
		m.mode = modePalette
		m.palette.SetValue("")
		m.rebuildPalette()
		return m, m.palette.Focus()
	case "R":
		// Rebuild the current project's MEMORY.md index: drop dangling bullets,
		// add unindexed files, preserve order. Fixes drift from external moves.
		if m.srcKind != srcMemories {
			return m, nil
		}
		dir := m.currentMemDir()
		if dir == "" {
			return m, nil
		}
		added, removed, err := memory.ReconcileIndex(dir)
		if err != nil {
			return m, m.setStatus("index rebuild failed: " + err.Error())
		}
		m.driftDir = "" // force a fresh drift check after the rebuild
		return m, tea.Batch(m.setStatus(fmt.Sprintf("index rebuilt  +%d  −%d", added, removed)), reloadCmd())
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
		return m, m.editCmd(path)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.mode = modeNormal
		if it, ok := m.selected(); ok {
			var err error
			if it.Kind == "plan" {
				err = plan.Delete(it.Path)
			} else {
				err = memory.Delete(it.Path)
			}
			if err != nil {
				return m, m.setStatus("delete failed: " + err.Error())
			}
			if it.Kind == "memory" {
				_ = memory.RemoveIndexForPath(it.Path) // drop its MEMORY.md bullet too
			}
			return m, tea.Batch(m.setStatus("deleted “"+clip(it.Title, 40)+"”"), reloadCmd())
		}
		return m, nil
	default:
		m.mode = modeNormal
		return m, m.setStatus("cancelled")
	}
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
		{"settings", palItem{glyph: "◈", glyphColor: t.TFeedback, label: "settings", sub: "Open the config file (theme, editor)", right: "/settings", action: palSettings}},
	}
}

// rebuildPalette recomputes candidates. Commands (memory/plans/settings) match a
// bare or /slashed prefix; the remaining text fuzzy-jumps across item titles in
// both sources, so a query can surface a command and matching items together.
func (m *Model) rebuildPalette() {
	t := m.theme()
	q := strings.TrimSpace(m.palette.Value())
	cmdq := strings.ToLower(strings.TrimPrefix(q, "/"))
	var rows []palItem

	for _, c := range m.paletteCommands() {
		if q == "" || strings.HasPrefix(c.name, cmdq) {
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

// openSettingsFile ensures the config file exists, then opens it in the editor
// so the user can edit theme/editor as JSON. Settings reload when the editor
// closes (see editorFinishedMsg).
func (m *Model) openSettingsFile() tea.Cmd {
	m.mode = modeNormal
	p, err := config.Path()
	if err != nil {
		return m.setStatus("settings: " + err.Error())
	}
	if _, statErr := os.Stat(p); os.IsNotExist(statErr) {
		_ = config.Save(config.Config{Theme: m.theme().Name, Editor: m.editorOverride})
	}
	return m.editCmd(p)
}

// switchSource changes the active source, remembering the per-source cursor and
// clearing the (per-source) filter.
func (m *Model) switchSource(k srcKind) {
	if k == m.srcKind {
		return
	}
	m.cursors[m.srcKind] = m.cursor
	m.srcKind = k
	m.search.SetValue("")
	m.cursor = m.cursors[k]
	m.rebuildRows() // clamps the remembered cursor if it's now out of range
}

// fuzzyScore reports whether all runes of query appear in order in s (case-
// insensitive), scoring by match span so tighter matches rank higher.
func fuzzyScore(query, s string) (int, bool) {
	q := []rune(strings.ToLower(query))
	if len(q) == 0 {
		return 0, true
	}
	text := []rune(strings.ToLower(s))
	qi, first, last := 0, -1, -1
	for ti := 0; ti < len(text) && qi < len(q); ti++ {
		if text[ti] == q[qi] {
			if first < 0 {
				first = ti
			}
			last = ti
			qi++
		}
	}
	if qi < len(q) {
		return 0, false
	}
	return last - first, true
}

// --- list model ---

// rebuildRows recomputes display rows from the active source: it maps records to
// Items (the source applies its own grouping/ordering), applies the generic
// search filter, then builds header/spacer/item rows by GroupKey.
func (m *Model) rebuildRows() {
	items := m.activeItems()

	if q := strings.ToLower(strings.TrimSpace(m.search.Value())); q != "" {
		var f []Item
		for _, it := range items {
			if strings.Contains(strings.ToLower(it.Title+" "+it.Context+" "+it.Body), q) {
				f = append(f, it)
			}
		}
		items = f
	}

	counts := map[string]int{}
	for _, it := range items {
		counts[it.GroupKey]++
	}

	var rows []row
	prevKey := "\x00sentinel"
	for _, it := range items {
		if it.GroupKey != "" && it.GroupKey != prevKey {
			if len(rows) > 0 {
				rows = append(rows, row{kind: rowSpacer})
			}
			rows = append(rows, row{kind: rowHeader, label: it.GroupLabel, color: it.GroupColor, count: counts[it.GroupKey]})
		}
		prevKey = it.GroupKey
		rows = append(rows, row{kind: rowMemory, item: it})
	}

	m.rows = rows
	if m.cursor >= len(rows) || m.cursor < 0 || rows[clampIdx(m.cursor, len(rows))].kind != rowMemory {
		m.cursor = m.firstMemRow()
	}
	m.ensureVisible()
	m.syncPreview()
}

// activeItems returns the Items for the source currently being browsed.
func (m Model) activeItems() []Item {
	if m.srcKind == srcPlans {
		return m.planItems()
	}
	return m.memoryItems()
}

// memoryItems maps memories into Items, applying the active type filter and
// grouping (project or type) and resolving badge/group colors from the theme.
func (m Model) memoryItems() []Item {
	t := m.theme()
	tf := typeCycle[m.typeIdx]
	byType := m.groupBy == groupType

	var sub []memory.Memory
	for _, mm := range m.memories {
		if tf == "" || mm.Type == tf {
			sub = append(sub, mm)
		}
	}
	sortForGroup(sub, m.groupBy)

	items := make([]Item, 0, len(sub))
	colorFor := t.groupColorer()
	for _, mm := range sub {
		key := groupKeyOf(mm, m.groupBy)
		label, color := mm.Project.Name, colorFor(key)
		right := ""
		if byType {
			label, color = typeLabel(mm.Type), t.typeColor(mm.Type)
			right = "· " + mm.Project.Name
		}
		items = append(items, Item{
			Title: mm.Title, Body: mm.Body, Raw: mm.Raw, Path: mm.Path, Modified: mm.Modified,
			Badge: typeName(mm.Type), BadgeColor: t.typeColor(mm.Type),
			GroupKey: key, GroupLabel: label, GroupColor: color,
			Right: right, Context: mm.Project.Name, MemDir: mm.Project.MemoryDir, Kind: "memory",
		})
	}
	return items
}

// planItems maps plans into Items grouped by recency (Today / This week /
// Older), mirroring how memories group by project: colored group headers with
// counts and the modified date in the right column. Plans have no
// badge/type/project. Sorting newest-first here makes the buckets contiguous
// regardless of the order plans arrive in (the same guarantee memoryItems gets
// from sortForGroup).
func (m Model) planItems() []Item {
	t := m.theme()
	plans := append([]plan.Plan(nil), m.plans...)
	sort.SliceStable(plans, func(i, j int) bool { return plans[i].Modified.After(plans[j].Modified) })

	items := make([]Item, 0, len(plans))
	colorFor := t.groupColorer()
	for _, p := range plans {
		key, label := recencyBucket(p.Modified)
		items = append(items, Item{
			Title: p.Title, Body: p.Body, Raw: p.Body, Path: p.Path, Modified: p.Modified,
			GroupKey: key, GroupLabel: label, GroupColor: colorFor(key),
			Right: humanizeSince(p.Modified), Context: "plan", Kind: "plan",
		})
	}
	return items
}

// recencyBucket buckets a plan by how recently it was modified into three
// coarse spans: Today (<24h), This week (<7d), and Older. The right-column date
// (humanizeSince) is finer-grained, so a row can read e.g. "3w ago" under the
// "Older" header — consistent, just more precise than the bucket.
func recencyBucket(mod time.Time) (key, label string) {
	if mod.IsZero() {
		return "older", "Older"
	}
	switch d := time.Since(mod); {
	case d < 24*time.Hour:
		return "today", "Today"
	case d < 7*24*time.Hour:
		return "week", "This week"
	default:
		return "older", "Older"
	}
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

func (m Model) selected() (Item, bool) {
	if m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].kind == rowMemory {
		return m.rows[m.cursor].item, true
	}
	return Item{}, false
}

// selectByPath moves the cursor to the item with the given path, if it's still
// present. If not found (e.g. it was deleted) the clamped cursor is left as-is.
func (m *Model) selectByPath(path string) {
	for i, r := range m.rows {
		if r.kind == rowMemory && r.item.Path == path {
			m.cursor = i
			m.ensureVisible()
			m.syncPreview()
			return
		}
	}
}

func (m Model) currentMemDir() string {
	if it, ok := m.selected(); ok && it.MemDir != "" {
		return it.MemDir
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
	it, ok := m.selected()
	if !ok {
		m.viewport.SetContent("")
		return
	}
	// Index drift applies to memories only; refresh the flag when the selected
	// project changes (cached by memory dir so it only touches disk on a switch).
	if it.Kind == "memory" {
		if it.MemDir != m.driftDir {
			m.driftDir = it.MemDir
			un, dang, err := memory.IndexDrift(it.MemDir)
			m.driftOut = err == nil && (len(un) > 0 || len(dang) > 0)
		}
	} else {
		m.driftOut = false
	}
	if m.previewCache == nil {
		m.previewCache = map[string]string{}
	}
	if cached, ok := m.previewCache[it.Path]; ok {
		m.viewport.SetContent(cached)
		m.viewport.GotoTop()
		return
	}
	// Decide the empty-body fallback before stripping, so a body that is only a
	// heading renders as empty rather than falling back to the raw frontmatter.
	body := it.Body
	if body == "" {
		body = it.Raw
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
	m.previewCache[it.Path] = rendered
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
		left += t.bar(t.Danger).Bold(true).Render("⚠ index out of sync ")
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
		left = t.bar(t.Dim).Render(" ") + t.bar(t.Accent).Render("memory · plans · settings") +
			t.bar(t.Dim).Render(" · type to jump · ") + t.bar(t.Accent).Render("↑↓") + t.bar(t.Dim).Render(" · ") +
			t.bar(t.Accent).Render("↵") + t.bar(t.Dim).Render(" · ") +
			t.bar(t.Accent).Render("esc") + t.bar(t.Dim).Render(" close ")
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
	var pairs [][2]string
	if m.srcKind == srcPlans {
		pairs = [][2]string{
			{"↑↓/jk", "move"}, {"/", "filter"}, {"⇥", "focus"}, {"d", "delete"},
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
			lines = append(lines, m.memRow(m.rows[i].item, i == m.cursor, rightCol))
		}
	}
	shown := m.shownCount()
	if shown == 0 && len(lines) > 0 {
		lines[0] = fg(t.Dim).Render("  no matches")
	}
	total := len(m.memories)
	if m.srcKind == srcPlans {
		total = len(m.plans)
	}
	body := lipgloss.NewStyle().Width(m.listW).Render(strings.Join(lines, "\n"))
	status := fg(t.Dim).Render(fmt.Sprintf(" %d of %d shown", shown, total))
	return lipgloss.JoinVertical(lipgloss.Left, body, lipgloss.NewStyle().Width(m.listW).Render(status))
}

// rightColW sizes the right-aligned column from the widest Item.Right in view
// (project name when grouped by type, or the date for plans), collapsing to 0
// when nothing needs it or it would starve the title.
func (m Model) rightColW() int {
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
	maxAllowed := m.listW - 2 - badgeWidth - 2 - 12
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

func (m Model) memRow(it Item, selected bool, rightCol int) string {
	t := m.theme()

	badgeCol := 0
	if it.Badge != "" {
		badgeCol = badgeWidth + 1 // padded badge + trailing space
	}
	nameW := m.listW - 2 - badgeCol - rightCol
	if rightCol > 0 {
		nameW-- // gap before the right column
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
	out := indent
	if it.Badge != "" {
		out += st(it.BadgeColor).Render(padRight("["+it.Badge+"]", badgeWidth)) + st(t.Fg).Render(" ")
	}
	out += st(titleColor).Render(padRight(it.Title, nameW))
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
	rest := "edited " + humanizeSince(it.Modified)
	if it.Context != "" {
		rest = it.Context + " · " + rest
	}
	meta += fg(t.Dim).Render(clip(rest, innerW-used))
	title := m.renderTitle(it.Title, innerW)
	block := lipgloss.JoinVertical(lipgloss.Left, meta, "", title, "", m.viewport.View())
	// Width(previewW) so every preview line fills the pane — otherwise the joined
	// frame has ragged line widths and a floated dialog leaves stale cells.
	return lipgloss.NewStyle().PaddingLeft(previewPad).Width(m.previewW).Render(block)
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

// panelBg is the opaque background shared by the floating dialogs — a shade
// lighter than the terminal so the box clearly reads as a raised surface.
func (m Model) panelBg() string { return m.theme().SelBg }

// padBG right-pads a (possibly styled) string to width w, filling the gap with
// the given background so every cell of a dialog row is opaque.
func padBG(s string, w int, bg string) string {
	gap := w - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	return s + lipgloss.NewStyle().Background(lipgloss.Color(bg)).Render(spaces(gap))
}

// dialogFrame wraps opaque content in a rounded border whose cells share the
// panel background, so the whole box is a solid surface over the panes.
func (m Model) dialogFrame(content, border string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(border)).
		BorderBackground(lipgloss.Color(m.panelBg())).
		Render(content)
}

// ruleLine is a horizontal rule that carries the panel background.
func (m Model) ruleLine(cw int) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme().Faint)).
		Background(lipgloss.Color(m.panelBg())).Render(strings.Repeat("─", cw))
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
		row[0], row[1],
		padBG("", cw, panel),
		padBG(hint, cw, panel),
	}
	return m.dialogFrame(strings.Join(lines, "\n"), t.Danger)
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
		padBG(pst(t.Dim).Render("  title for the new memory in this project"), cw, panel),
		padBG("", cw, panel),
		padBG("  "+m.input.View(), cw, panel),
		padBG("", cw, panel),
		padBG(pst(t.Dim).Render("  press ")+pst(t.Accent).Bold(true).Render("↵")+pst(t.Dim).Render(" create     ")+
			pst(t.Fg).Bold(true).Render("esc")+pst(t.Dim).Render(" cancel"), cw, panel),
	}
	return m.dialogFrame(strings.Join(lines, "\n"), t.Accent)
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

type editorFinishedMsg struct {
	path string
	err  error
}

func (m Model) editCmd(path string) tea.Cmd {
	parts := m.resolveEditor()
	args := append([]string{}, parts[1:]...)
	args = append(args, path)
	c := exec.Command(parts[0], args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{path: path, err: err}
	})
}

// resolveEditor picks the command (and any args) used to open a file for editing.
// It honors the config editor override first, then $VISUAL and $EDITOR (the Unix
// convention), then the host editor when engram runs inside one (e.g. VS Code's
// integrated terminal — using --wait so the edit completes before reload), then
// a terminal editor, falling back to vi.
func (m Model) resolveEditor() []string {
	if v := strings.TrimSpace(m.editorOverride); v != "" {
		return strings.Fields(v)
	}
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
	mems  []memory.Memory
	plans []plan.Plan
	sig   string
	err   error
}

func reloadCmd() tea.Cmd {
	return func() tea.Msg {
		mems, err := memory.Discover("")
		if err != nil {
			return reloadMsg{err: err}
		}
		plans, err := plan.Discover("")
		if err != nil {
			return reloadMsg{err: err} // keep the current state rather than blanking plans
		}
		// Capture the signature alongside the data so the reload updates the
		// poll baseline atomically (no reload -> sig-changed -> reload loop).
		sig, _ := combinedSig()
		return reloadMsg{mems: mems, plans: plans, sig: sig}
	}
}
