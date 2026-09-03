package tui

import (
	"strings"
	"time"

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
	docs     []memory.DocFile // read-only instruction files + MEMORY.md indexes
	srcKind  srcKind          // which source is being browsed
	cursors  [srcCount]int    // remembered cursor (row index) per source

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

	// Store-timestamp cache for the sync strip's "store advanced …" stamp:
	// engram id -> last store commit time, fetched lazily for the selected row
	// (storeTimeCmd) and cleared on reload. storeTimeAsked marks ids already
	// requested this cycle — including failures, so a broken lookup is asked
	// once and the stamp is simply omitted, never fabricated or retried hot.
	storeTimes     map[string]time.Time
	storeTimeAsked map[string]bool

	themeIdx       int
	editorOverride string            // optional editor command from config; "" = use env/host
	aliases        map[string]string // config projectAliases: memory dir → alias, for projects with no git remote
	// installedAsst is the assistant registry filtered to the CLIs on $PATH,
	// resolved once at startup. Resolving it per call put four exec.LookPath
	// sweeps of $PATH on the event loop for every keystroke in the palette,
	// since the unprefixed query matches assistants too.
	installedAsst  []assistant
	typeIdx        int
	groupBy        groupMode
	focus          focus
	mode           mode
	status         string
	statusKind     statusKind      // severity of the current status, picks its color
	statusSeq      int             // generation, so an old auto-dismiss timer can't clear a newer status
	fsSig          string          // last-seen filesystem fingerprint; "" until the first poll baselines it
	driftDir       string          // memory dir the drift flag was computed for (cache key)
	driftOut       bool            // selected project's MEMORY.md is out of sync with its files
	driftUnindexed []string        // memory files on disk with no MEMORY.md bullet (added without an index line)
	driftDangling  []string        // MEMORY.md bullets whose .md file is gone (deleted/renamed without updating the index)
	driftDismissed map[string]bool // memory dirs whose banner was esc-dismissed; session-only, lazily initialized
	driftErr       error           // the drift check itself failed — "couldn't check" is not "in sync", and R must not claim it is

	// pull confirm (modePullConfirm)
	pullPlan team.PullResult // the accounting shown pre-confirm; y applies this same walk

	// resolve confirm (modeResolveConfirm)
	resolvePath string // the conflicted memory
	resolveTmp  string // merge temp file BeginConflictResolve wrote (removed on cancel)
	// The two sides BeginConflictResolve merged, kept rather than discarded once
	// the rows are built: the row budget comes from the frame, so a resize while
	// the confirm is open has to re-diff them (setResolveDiff).
	resolveYours  []string
	resolveTheirs []string
	resolveIdent  bool            // the sides' shared content is byte-identical (only the anchor differs)
	resolveRows   []diffRow       // inline diff of the two sides, shown before $EDITOR opens
	resolveSame   resolveSameness // when the diff shows nothing, why

	width, height           int
	listW, previewW, panesH int // layout, recomputed in resize (sole writer)
	ready                   bool

	// Batch promote: paths of memories marked with space. Empty (the common case)
	// means promote acts on the cursor row, exactly as it always has — marking is
	// an opt-in mode, not a state every promote has to reason about.
	marks map[string]bool

	// Batch promote (marks). batchItems is the marked set resolved for promotion;
	// scanFlagged is the subset the secret scan objected to, walked one memory at
	// a time so each gets its own decision. Everything is decided before the first
	// write, so the accepted memories still land as one commit.
	batchItems   []batchItem     // marked memories, each carrying its own project key
	scanFlagged  []flaggedMemory // memories the scan flagged, awaiting a per-memory call
	scanIdx      int             // which flagged memory the modal is asking about
	scanAccepted []batchItem     // clean or overridden, waiting for the single commit
	scanSkipped  int             // flagged memories the user declined
	scanOverrode int             // flagged memories the user included anyway

	// promote scope picker (modePromoteScope)
	promotePath  string           // memory file being promoted
	promoteTitle string           // its title, for the modal header
	promoteKey   string           // resolved project key, or "" when the project has no key
	promoteState team.RemoteState // why, when it has none: no remote (>alias would help), gone, or git couldn't say
	// promoteAliasable is false where >alias would itself refuse — the home
	// folder — which reads as RemoteNone like any remoteless project, so the
	// state alone can't tell the dialog whether to offer the hint.
	promoteAliasable bool
	promoteCursor    int // 0 = this project, 1 = global

	// withdraw confirm (modeWithdrawConfirm)
	withdrawPath  string           // memory being withdrawn
	withdrawTitle string           // its title, for the modal header
	withdrawOwner team.OwnerStatus // owner-guard inputs, so the modal can say the check was skipped

	// secret-scan guard on promote
	scanAction      string            // config policy: block | block-strict | warn | off
	scanPII         bool              // also flag PII when scanning
	secretFindings  []secrets.Finding // findings that blocked the pending promote (modeSecretWarn)
	secretTitle     string            // the flagged memory's title (batch walk); "" for a single promote
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
	themeIdx := resolveThemeIdx(cfg.Theme)
	t := themes[themeIdx]
	// The poll's scanRoots fallback starts from the config already read here,
	// rather than from nil until the first tick lands (see seedScanRoots).
	seedScanRoots(cfg.ScanRoots)

	se := textinput.New()
	se.Prompt = "/ "
	se.PromptStyle = fgb(t.Accent)
	se.Cursor.Style = fg(t.Accent)
	se.CharLimit = 64

	pal := textinput.New()
	pal.Prompt = "" // the box header renders the "engram:" label
	pal.Placeholder = "type to jump or run…"
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
		aliases:        cleanAliases(cfg.ProjectAliases),
		installedAsst:  installedAssistants(),
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
	panel := lipgloss.Color(t.Sel)
	m.search.PromptStyle = fgb(t.Accent)
	m.search.Cursor.Style = fg(t.Accent)
	// Explicit text colors: the filter input sits on the header band (painted
	// t.Bg2 by sourceStrip), so terminal-default foreground text could be
	// invisible on it (white on Paperback's near-white).
	m.search.TextStyle = fg(t.Fg)
	m.search.PlaceholderStyle = fg(t.Faint)
	m.palette.PlaceholderStyle = fg(t.Dim).Background(panel)
	m.palette.TextStyle = fg(t.Fg).Background(panel)
	m.palette.Cursor.Style = fg(t.Accent).Background(panel)
	m.input.PromptStyle = fgb(t.Accent).Background(panel)
	m.input.TextStyle = fg(t.Fg).Background(panel)
	m.input.Cursor.Style = fg(t.Accent).Background(panel)
}

// applyTheme switches the active theme by index for this session — restyles
// inputs and re-renders — and persists nothing; setTheme is the persisting
// variant.
// The /settings reload applies only — the file it just read is the source of
// truth there, and saving would rewrite the file the user just edited.
func (m *Model) applyTheme(idx int) bool {
	if idx < 0 || idx >= len(themes) {
		return false
	}
	m.themeIdx = idx
	m.styleInputs()
	// Order matters: rebuild the glamour renderer for the new theme BEFORE
	// clearing the cache and rebuilding rows — rebuildRows ends in syncPreview,
	// which re-renders the selected body and re-fills the cache. With the old
	// renderer still in place it would cache a stale-theme render, and the
	// preview only caught up on the next theme press.
	m.buildRenderer()
	m.previewCache = nil // glamour style changed
	m.rebuildRows()
	return true
}

// setTheme applies the theme and persists it through config.Update, which
// keeps unrelated settings and refuses to write over a file that did not
// parse; that refusal is the one thing worth a status here.
func (m *Model) setTheme(idx int) tea.Cmd {
	if !m.applyTheme(idx) {
		return nil
	}
	err := config.Update(func(c *config.Config) error {
		c.Theme = m.theme().Key // only the theme: Update keeps every other setting as the file has it
		return nil
	})
	if err != nil {
		return m.setDanger("theme not saved — " + configErr(err))
	}
	return nil
}

func (m Model) Init() tea.Cmd { return pollCmd() }
