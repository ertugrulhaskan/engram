package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

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

// combinedSig fingerprints both sources into one string, so a change in either
// tree flips it. One baseline (m.fsSig) covers both — no second baseline (that
// would risk a reload loop).
func combinedSig() (string, error) {
	ms, err := memory.Signature("")
	ps, _ := plan.Signature("")
	ds, _ := memory.DocsSignature("") // CLAUDE.md edits aren't under the memory tree
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
		docs, _ := memory.DiscoverDocs("") // best-effort; don't fail the reload over docs
		sync, _ := team.SyncStates(mems)   // best-effort; empty when no team store
		// Capture the signature alongside the data so the reload updates the
		// poll baseline atomically (no reload -> sig-changed -> reload loop).
		sig, _ := combinedSig()
		return reloadMsg{mems: mems, plans: plans, docs: docs, sync: sync, sig: sig}
	}
}
