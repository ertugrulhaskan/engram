package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/plan"
	"github.com/ertugrulhaskan/engram/internal/secrets"
	"github.com/ertugrulhaskan/engram/internal/team"
)

// Model is the root Bubble Tea model.
type Model struct {
	memories []memory.Memory  // full memory set, unfiltered
	plans    []plan.Plan      // full plan set
	docs     []memory.DocFile // read-only CLAUDE.md / MEMORY.md files
	srcKind  srcKind          // which source is being browsed
	cursors  [3]int           // remembered cursor (row index) per source

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
	previewCache map[string]string         // rendered body keyed by path; cleared on resize/theme/reload
	syncStates   map[string]team.SyncState // memory path -> team sync state; recomputed on load/reload

	themeIdx       int
	editorOverride string // optional editor command from config; "" = use env/host
	typeIdx        int
	groupBy        groupMode
	focus          focus
	mode           mode
	status         string
	statusKind     statusKind // severity of the current status, picks its color
	statusSeq      int        // generation, so an old auto-dismiss timer can't clear a newer status
	fsSig          string     // last-seen filesystem fingerprint; "" until the first poll baselines it
	driftDir       string     // memory dir the drift flag was computed for (cache key)
	driftOut       bool       // selected project's MEMORY.md is out of sync with its files
	driftUnindexed int        // memory files on disk with no MEMORY.md bullet (added without an index line)
	driftDangling  int        // MEMORY.md bullets whose .md file is gone (deleted/renamed without updating the index)

	width, height           int
	listW, previewW, panesH int // layout, recomputed in resize (sole writer)
	ready                   bool

	// promote scope picker (modePromoteScope)
	promotePath   string // memory file being promoted
	promoteTitle  string // its title, for the modal header
	promoteKey    string // resolved project key, or "" when the project has no remote
	promoteCursor int    // 0 = this project, 1 = global

	// withdraw confirm (modeWithdrawConfirm)
	withdrawPath  string // memory being withdrawn
	withdrawTitle string // its title, for the modal header

	// secret-scan guard on promote
	scanAction      string            // config policy: block | block-strict | warn | off
	scanPII         bool              // also flag PII when scanning
	secretFindings  []secrets.Finding // findings that blocked the pending promote (modeSecretWarn)
	secretPath      string            // the scanned memory path to promote if the user overrides
	secretPlacement string            // placement to promote to if the user overrides

	version string // release version for the help/about footer; "" → "dev"
}

// WithVersion sets the version string shown in the `?` help overlay's about
// footer. It's optional (the TUI runs fine without it), so it's a chainable
// setter rather than a New() parameter — keeps the many New() call sites simple.
func (m Model) WithVersion(v string) Model {
	m.version = v
	return m
}

// New builds the initial model from the discovered memories, plans, and
// read-only docs, applying persisted settings (theme, editor override).
func New(mems []memory.Memory, plans []plan.Plan, docs []memory.DocFile, cfg config.Config) Model {
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
	pal.Placeholder = "Type / for commands, @ for Claude…"
	pal.CharLimit = 64

	ti := textinput.New()
	ti.Prompt = "› "
	ti.CharLimit = 120
	ti.Width = 44 // bound the visible field so dialogs stay dialog-sized

	m := Model{
		memories:       mems,
		plans:          plans,
		docs:           docs,
		themeIdx:       themeIdx,
		editorOverride: strings.TrimSpace(cfg.Editor),
		search:         se,
		palette:        pal,
		input:          ti,
		focus:          focusList,
		mode:           modeNormal,
		groupBy:        groupProject,
		scanAction:     cfg.ScanAction(),
		scanPII:        cfg.ScanPII(),
	}
	m.styleInputs()
	m.syncStates, _ = team.SyncStates(mems) // best-effort; empty when no team store
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
