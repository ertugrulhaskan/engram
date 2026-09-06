package tui

// The inline diff the resolve confirm shows: what `>resolve` is about to hand
// $EDITOR, rendered as a line diff of the two sides instead of raw conflict
// markers. Pure computation over two line slices — no IO, no files — so the
// version fetching stays in internal/team (BeginConflictResolve hands back the
// two sides it merged) and only the presentation lives here.
//
// The view is unified (one column, ± gutters), not two columns side by side:
// boxWidth caps a dialog at 68 cells, so two columns would leave ~32 each,
// which wraps ordinary memory prose to shreds. A unified diff reads at full
// dialog width and still shows both sides.

// resolveSameness is why the resolve confirm has no diff to show.
type resolveSameness int

const (
	resolveDiffers   resolveSameness = iota // there is a diff; the rows hold it
	resolveIdentical                        // byte-identical shared content: only the sync anchor differs
	resolveInvisible                        // the bytes differ, but only where contentLines normalizes: CRLF, or a trailing newline
	resolveNoRoom                           // there is a diff, but the frame can't seat even a minimal preview
)

// diffOp classifies one row of a rendered diff.
type diffOp int

const (
	diffEqual  diffOp = iota // in both copies
	diffYours                // only in your local copy
	diffTheirs               // only in the team store's copy
	diffElide                // stands for a run of unchanged lines the view skipped
	diffMore                 // stands for the rest of the diff, changes included, past the row cap
)

// diffRow is one line of the rendered diff. n counts the lines a diffElide or
// diffMore stands for and is 0 otherwise.
type diffRow struct {
	op   diffOp
	text string
	n    int
}

// maxDiffCells bounds the LCS table. Memories are small, and the common
// prefix/suffix trim below usually leaves a handful of lines to align, so this
// only bites on a pathological pair — where the honest answer is "these two
// don't line up", not a multi-second stall on the UI thread.
const maxDiffCells = 250_000

// diffLines computes a line-level diff of yours against theirs.
func diffLines(yours, theirs []string) []diffRow {
	p := 0
	for p < len(yours) && p < len(theirs) && yours[p] == theirs[p] {
		p++
	}
	s := 0
	for s < len(yours)-p && s < len(theirs)-p && yours[len(yours)-1-s] == theirs[len(theirs)-1-s] {
		s++
	}
	out := runRows(yours[:p], diffEqual)
	out = append(out, alignRows(yours[p:len(yours)-s], theirs[p:len(theirs)-s])...)
	return append(out, runRows(yours[len(yours)-s:], diffEqual)...)
}

// alignRows diffs two runs that share no prefix or suffix, by longest common
// subsequence — the standard alignment, and O(n·m), which is why the caller
// trims first and why maxDiffCells exists.
func alignRows(a, b []string) []diffRow {
	switch {
	case len(a) == 0 && len(b) == 0:
		return nil
	case len(a) == 0:
		return runRows(b, diffTheirs)
	case len(b) == 0:
		return runRows(a, diffYours)
	case len(a)*len(b) > maxDiffCells:
		// Too large to align line by line: show each side whole, which is
		// true (every line differs somewhere) if less precise.
		return append(runRows(a, diffYours), runRows(b, diffTheirs)...)
	}
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				lcs[i][j] = lcs[i+1][j+1] + 1
			case lcs[i+1][j] >= lcs[i][j+1]:
				lcs[i][j] = lcs[i+1][j]
			default:
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var out []diffRow
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			out = append(out, diffRow{op: diffEqual, text: a[i]})
			i, j = i+1, j+1
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, diffRow{op: diffYours, text: a[i]})
			i++
		default:
			out = append(out, diffRow{op: diffTheirs, text: b[j]})
			j++
		}
	}
	out = append(out, runRows(a[i:], diffYours)...)
	return append(out, runRows(b[j:], diffTheirs)...)
}

// runRows tags a whole run of lines with one op.
func runRows(lines []string, op diffOp) []diffRow {
	out := make([]diffRow, 0, len(lines))
	for _, ln := range lines {
		out = append(out, diffRow{op: op, text: ln})
	}
	return out
}

// collapseDiff keeps ctx unchanged lines either side of every change and
// replaces each longer run with a single diffElide naming how many it stands
// for — so the dialog shows the changes, not the whole memory.
func collapseDiff(rows []diffRow, ctx int) []diffRow {
	keep := make([]bool, len(rows))
	for i, r := range rows {
		if r.op == diffEqual {
			continue
		}
		for k := i - ctx; k <= i+ctx; k++ {
			if k >= 0 && k < len(rows) {
				keep[k] = true
			}
		}
	}
	var out []diffRow
	skipped := 0
	// end is one past the skipped run, so rows[end-1] is its last (and, at
	// skipped == 1, its only) line.
	flush := func(end int) {
		switch {
		case skipped == 0:
		case skipped == 1:
			// A run of one costs the same screen row either way, so an elision
			// standing for it buys no space and shows strictly less than the
			// line it replaced. Keep the line.
			out = append(out, rows[end-1])
		default:
			out = append(out, diffRow{op: diffElide, n: skipped})
		}
		skipped = 0
	}
	for i, r := range rows {
		if !keep[i] {
			skipped++
			continue
		}
		flush(i)
		out = append(out, r)
	}
	flush(len(rows))
	return out
}

// capDiff trims the view to max rows, replacing the tail with a diffMore that
// counts the *lines* dropped (an elision already standing for a run keeps its
// own count), so a long conflict can't grow the dialog past the terminal.
//
// diffMore, not diffElide: the tail is whatever was left, changes included, so
// calling it "unchanged" would tell the user the hidden remainder agrees when
// it may be the entire other side.
func capDiff(rows []diffRow, max int) []diffRow {
	if max < 1 || len(rows) <= max {
		return rows
	}
	dropped := 0
	for _, r := range rows[max-1:] {
		if r.op == diffElide || r.op == diffMore {
			dropped += r.n
			continue
		}
		dropped++
	}
	return append(append([]diffRow{}, rows[:max-1]...), diffRow{op: diffMore, n: dropped})
}

// alignDiff is the frame-independent half of the resolve confirm's diff: the LCS
// alignment and the context collapse, neither of which depends on how many rows
// the dialog has. changed reports whether the shared content differs at all — a
// resolve is reachable on an anchor difference alone, and "no differences" is a
// real answer worth saying rather than an empty box.
//
// It is split from the capping because the confirm re-measures on every
// tea.WindowSizeMsg, and a drag-resize is a stream of them — re-running the
// O(n*m) table for two 500-line memories is a 250k-cell allocation per event, on
// the event loop, to change one integer. setResolveSides runs this once per
// conflict and setResolveDiff re-runs only capDiff.
func alignDiff(yours, theirs []string, ctx int) (rows []diffRow, changed bool) {
	full := diffLines(yours, theirs)
	for _, r := range full {
		if r.op == diffYours || r.op == diffTheirs {
			changed = true
			break
		}
	}
	if !changed {
		return nil, false
	}
	return collapseDiff(full, ctx), true
}
