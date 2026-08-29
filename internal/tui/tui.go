// Package tui implements engram's Bubble Tea terminal UI. It contains no file
// logic; it consumes parsed memories from the memory package.
package tui

import (
	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/plan"
	"github.com/ertugrulhaskan/engram/internal/source"
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
	modePalette
	modeHelp
	modePromoteScope     // picking team scope (this project / global) for a promote
	modeSecretWarn       // a promote was blocked because the memory looks like it holds a secret
	modeWithdrawConfirm  // confirming a withdraw (removing a shared memory from the team store)
	modePullConfirm      // showing a pull's full accounting (team.PullPlan) before anything moves
	modeResolveConfirm   // showing the first conflict hunk before handing the merge file to $EDITOR
	modeReconcileConfirm // naming the drifted files before rebuilding MEMORY.md
)

// srcKind selects which collection is being browsed.
type srcKind int

const (
	srcMemories srcKind = iota
	srcPlans
	srcFiles // read-only instruction files (CLAUDE.md, AGENTS.md, …) + MEMORY.md

	srcCount // number of sources; keeps shift+tab cycling and per-source state in step
)

type groupMode int

const (
	groupProject groupMode = iota
	groupType
)

const (
	badgeWidth  = 9  // width of the widest bare "reference" badge word
	previewPad  = 2  // preview margin, applied on both sides (content width is previewW-2*previewPad)
	maxReadCols = 88 // cap the prose line length on wide terminals for readability
)

// The frame's row budget, in one place: View() joins headerRows + panesH +
// footerRows, and resize() sizes panesH from the same numbers, so adding or
// removing a chrome row can't leave the two disagreeing.
const (
	headerRows = 3 // tabs row, controls row, header rule
	footerRows = 3 // padding, status bar, padding
	// reservedRows keeps the terminal's final row unwritten. Filling the very
	// last cell makes some terminals scroll the alt-screen buffer on every
	// repaint, which desyncs Bubble Tea's line-diff renderer and shows up as
	// blank scrollback with ghost rows until a resize (fixed in d11da02 — do
	// not reclaim this row).
	reservedRows = 1
	chromeRows   = headerRows + footerRows + reservedRows
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

// srcCaps wires each source to the capability set its data package declares
// (ENGR-12): the memory package for memories and the read-only files source,
// the plan package for plans. Key handlers ask caps() — the one capability
// gate — rather than comparing srcKind, so a source's row here plus its
// readOnlyHint entry below are its whole policy (TestCapsMatrix pins both),
// and a source without a row gets the zero Caps, which grants nothing.
// Checks that remain on srcKind (type filter, grouping, reconcile, the drift
// banner) are about what memories *are*, not what the user may do.
var srcCaps = [srcCount]source.Caps{
	srcMemories: memory.Caps,
	srcPlans:    plan.Caps,
	srcFiles:    memory.DocsCaps,
}

// readOnlyHint is what a capability-denied e/n/d answers, per source: the
// files source names its escape hatch, everywhere else the denial is silent —
// the key is absent from the controls row too, so there is nothing to explain.
var readOnlyHint = [srcCount]string{
	srcFiles: "read-only — edit with @Claude (ctrl+p, then @)",
}

// caps returns the current source's capability set.
func (m Model) caps() source.Caps { return srcCaps[m.srcKind] }
