package tui

import (
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/plan"
	"github.com/ertugrulhaskan/engram/internal/team"
)

// pollInterval is how often engram re-scans the filesystem for external changes.
const pollInterval = 2 * time.Second

// pollResultMsg carries the latest filesystem fingerprint from a poll tick.
type pollResultMsg struct {
	sig string
	err error
}

// lastGoodRoots remembers the scanRoots of the last config that parsed. A tick
// that finds a broken file then keeps scanning the roots the session was already
// using, instead of degrading to none: config.Load answers an unparseable file
// with a zero Config, so the poll would have dropped every scanRoots-discovered
// instruction file out of /files two seconds after the editor handler told the
// user the settings had *not* been applied. Guarded by a mutex because pollCmd
// and reloadCmd run in command goroutines.
var lastGoodRoots struct {
	sync.Mutex
	roots []string
}

// seedScanRoots primes the fallback with the config the caller has already read,
// so it is never empty. Without it the cache is nil until the first poll tick
// lands, and a config broken inside that window would drop every
// scanRoots-discovered file out of /files — the exact degradation the fallback
// exists to prevent, just in a narrower window.
func seedScanRoots(roots []string) {
	lastGoodRoots.Lock()
	defer lastGoodRoots.Unlock()
	lastGoodRoots.roots = roots
}

// currentScanRoots re-reads the configured scan roots, falling back to the last
// ones that parsed. The per-tick read is deliberate — a tiny file beside a ~1ms
// scan — and it is what makes a scanRoot added in /settings take effect live
// rather than at the next restart.
func currentScanRoots() []string {
	cfg, err := config.Read()
	lastGoodRoots.Lock()
	defer lastGoodRoots.Unlock()
	if err == nil {
		lastGoodRoots.roots = cfg.ScanRoots
	}
	return lastGoodRoots.roots
}

// combinedSig fingerprints both sources into one string, so a change in either
// tree flips it. One baseline (m.fsSig) covers both — no second baseline (that
// would risk a reload loop).
func combinedSig() (string, error) {
	ms, err := memory.Signature("")
	ps, _ := plan.Signature("")
	ds, _ := memory.DocsSignature("", currentScanRoots()) // CLAUDE.md edits aren't under the memory tree
	return ms + "|" + ps + "|" + ds, err
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

// --- reloading after a mutation ---

type reloadMsg struct {
	mems  []memory.Memory
	plans []plan.Plan
	docs  []memory.DocFile
	sync  map[string]team.SyncState
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
		docs, _ := memory.DiscoverDocs("", currentScanRoots()) // best-effort; don't fail the reload over docs
		syncStates, _ := team.SyncStates(mems)                 // best-effort; empty when no team store
		// Capture the signature alongside the data so the reload updates the
		// poll baseline atomically (no reload -> sig-changed -> reload loop).
		sig, _ := combinedSig()
		return reloadMsg{mems: mems, plans: plans, docs: docs, sync: syncStates, sig: sig}
	}
}
