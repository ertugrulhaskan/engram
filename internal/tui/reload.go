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

// pollResultMsg carries the latest filesystem fingerprint from a poll tick,
// and the settings that tick re-read.
//
// parsed says whether the config could be read at all, and the handler adopts
// the settings only when it is true. It is a field rather than a nil check on
// the map, because the two states are not distinguishable from the map:
// clearing the last alias with `>alias -` leaves `projectAliases` absent (the
// field is `omitempty`), so a perfectly good config parses to a nil map — and a
// nil check would read that as "couldn't parse, keep what we have" and pin the
// alias the user just deleted, forever, in every other session.
//
// gen is the m.settingsGen this tick was launched with. The read happens at the
// top of the tick and the message arrives after a full filesystem scan, so a
// `>alias` handled in between would be overwritten by the pre-write snapshot —
// the status line saying "keyed by alias/acme" while the map no longer holds
// it, and the very next promote placing into global/. Every in-session settings
// write bumps the counter, so a tick that straddles one is discarded.
type pollResultMsg struct {
	sig     string
	cfg     config.Config
	aliases map[string]string
	dropped []string
	parsed  bool
	gen     uint64
	err     error
}

// lastGood remembers the last config that parsed. A tick that finds a broken
// file then keeps using the settings the session already had, instead of
// degrading to none: a failed read yields a zero Config, so the poll would have
// dropped every scanRoots-discovered instruction file out of /files two seconds
// after the editor handler told the user the settings had *not* been applied.
// Guarded by a mutex because pollCmd and reloadCmd run in command goroutines.
var lastGood struct {
	sync.Mutex
	cfg config.Config
}

// seedConfig primes the fallback with the config the caller has already read,
// so it is never empty. Without it the cache is zero until the first poll tick
// lands, and a config broken inside that window would drop every
// scanRoots-discovered file out of /files — the exact degradation the fallback
// exists to prevent, just in a narrower window.
func seedConfig(cfg config.Config) {
	lastGood.Lock()
	defer lastGood.Unlock()
	lastGood.cfg = cfg
}

// currentConfig re-reads the settings a running session must not go stale on,
// falling back to the last config that parsed. The per-tick read is deliberate
// — a tiny file beside a ~1ms scan — and it is what makes a setting changed in
// /settings, or by a second engram, take effect live rather than at the next
// restart.
// ok reports whether *this* read succeeded, so a caller can tell a setting that
// is genuinely absent from one it merely failed to see.
func currentConfig() (cfg config.Config, ok bool) {
	cfg, err := config.Read()
	lastGood.Lock()
	defer lastGood.Unlock()
	if err == nil {
		lastGood.cfg = cfg
		return cfg, true
	}
	return lastGood.cfg, false
}

// currentScanRoots is currentConfig's scanRoots, for the callers that need only
// those and are happy with the last-good fallback either way.
func currentScanRoots() []string {
	cfg, _ := currentConfig()
	return cfg.ScanRoots
}

// combinedSig fingerprints both sources into one string, so a change in either
// tree flips it. One baseline (m.fsSig) covers both — no second baseline (that
// would risk a reload loop). roots comes from the caller so a tick reads the
// config once rather than once per consumer.
func combinedSig(roots []string) (string, error) {
	ms, err := memory.Signature("")
	ps, _ := plan.Signature("")
	ds, _ := memory.DocsSignature("", roots) // CLAUDE.md edits aren't under the memory tree
	return ms + "|" + ps + "|" + ds, err
}

// pollCmd schedules the next filesystem scan. The closure runs in the command
// goroutine, so the scan never blocks the event loop. It is the only thing that
// re-arms the poll loop (started once in Init, re-armed once per pollResultMsg).
//
// It carries the whole config, not just the roots. Everything a running session
// holds from that file goes stale the same way, and it used to refresh only on
// the way back from /settings — so a second engram window, or a hand edit,
// left this one deciding off values it could no longer see. projectAliases is
// the sharpest case, because it decides *placement*: >promote and >pull would
// put a keyed project's memories in global/ while the scope dialog said "this
// project has no key to promote under". secretScanAction is the next, because
// it decides whether credentials are caught before a push. Refreshing the roots
// and nothing else was the asymmetry; they all come off one read now.
//
// The theme is deliberately not among them: a running session's colours
// changing under a poll tick is a visual surprise, and `1`-`3` and /settings are
// both explicit. That is a decision, not an omission.
func pollCmd(gen uint64) tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg {
		cfg, parsed := currentConfig()
		sig, err := combinedSig(cfg.ScanRoots)
		var aliases, dropped = map[string]string(nil), []string(nil)
		if parsed {
			// CleanAliases validates here, where the map is read, exactly as the
			// /settings path does — a hand-edited config cannot smuggle an
			// unsafe name in through the poll. It answers nil with an empty map,
			// which is the authoritative "no aliases" this tick.
			aliases, dropped = team.CleanAliases(cfg.ProjectAliases)
		}
		return pollResultMsg{sig: sig, cfg: cfg, aliases: aliases, dropped: dropped, parsed: parsed, gen: gen, err: err}
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
		roots := currentScanRoots()
		docs, _ := memory.DiscoverDocs("", roots) // best-effort; don't fail the reload over docs
		syncStates, _ := team.SyncStates(mems)    // best-effort; empty when no team store
		// Capture the signature alongside the data so the reload updates the
		// poll baseline atomically (no reload -> sig-changed -> reload loop).
		sig, _ := combinedSig(roots)
		return reloadMsg{mems: mems, plans: plans, docs: docs, sync: syncStates, sig: sig}
	}
}
