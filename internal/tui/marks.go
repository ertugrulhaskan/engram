package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// listedMemoryPaths returns the path of every memory row in the list. Both
// shaping filters are already baked in — rebuildRows is what produced these
// rows, applying the type filter and the search query in turn — so reading the
// rows is what keeps this code's idea of "the list" identical to the one the
// filters produced.
//
// This is the whole filtered list, not the visible slice of it: render.go scrolls
// a window over m.rows, so a list of fifty matches has fifty rows here while
// twenty are on screen. That is the right set to act on — the filters are the
// selection, and where the fold happens to fall is not — but it is why the
// wording everywhere says "the list" rather than "in view".
//
// Headers and spacers are skipped: they are furniture, not memories.
func (m Model) listedMemoryPaths() []string {
	paths := make([]string, 0, len(m.rows))
	for _, r := range m.rows {
		if r.kind == rowMemory {
			paths = append(paths, r.item.Path)
		}
	}
	return paths
}

// toggleMarkList marks every memory in the list, or — when all of them are
// marked already — unmarks exactly those. It is `space` scaled up from one row
// to the whole list, which is what turns "promote a whole type" into two
// keystrokes: cycle `t` to the type, then `a`.
//
// Marks outside the current filters are never touched, so a set assembled under
// one filter survives switching to another. That is the useful behavior and also
// the dangerous one, which is why the status line names the running total
// whenever any mark sits outside the list it just acted on: a batch wider than
// what you are looking at must never be a surprise at the promote confirm.
func (m Model) toggleMarkList() (tea.Model, tea.Cmd) {
	listed := m.listedMemoryPaths()
	if len(listed) == 0 {
		return m, m.setStatus("nothing in the list to mark")
	}

	marked := 0
	for _, p := range listed {
		if m.marks[p] {
			marked++
		}
	}

	// Every listed row already marked: the only reading left for the key is
	// "undo that", and doing it here keeps `a` from being a dead keystroke.
	if marked == len(listed) {
		for _, p := range listed {
			delete(m.marks, p)
		}
		// Whatever survives sat outside the list, so it is all "elsewhere".
		elsewhere := len(m.marks)
		if elsewhere == 0 {
			m.marks = nil // collapses the mark column back to zero width
		}
		return m, m.setCancel(m.markScopeLine(len(listed), "unmarked", elsewhere))
	}

	if m.marks == nil {
		m.marks = map[string]bool{}
	}
	for _, p := range listed {
		m.marks[p] = true
	}
	// Report the set now marked in the list, not the delta: after a key that
	// means "mark them all", "marked 1" on a view where nine were already marked
	// describes the keystroke accurately and the resulting batch misleadingly —
	// and the batch is what the promote confirm will act on.
	return m, m.setStatus(m.markScopeLine(len(listed), "marked", len(m.marks)-len(listed)))
}

// markScopeLine describes what a mark-all keystroke just did. It names the type
// filter when one is active — that filter *is* the selection, so echoing it back
// is how the user confirms they marked the set they meant rather than whatever
// the list happened to hold — and appends the running total when elsewhere > 0,
// meaning marks survive outside the list this keystroke touched.
//
// elsewhere is passed in rather than derived from a count comparison: unmarking
// three while three others stay marked makes the two totals equal, and a test
// like `total != n` would go quiet in exactly the case the disclosure exists for.
//
// Call it after mutating the marks; the total it reports is the state as it now
// stands.
func (m Model) markScopeLine(n int, verb string, elsewhere int) string {
	unit, scope := "memories", " in the list"
	if n == 1 {
		unit = "memory"
	}
	if tf := typeCycle[m.typeIdx]; tf != "" {
		unit, scope = string(tf)+" "+unit, ""
	}
	line := fmt.Sprintf("%s %d %s%s", verb, n, unit, scope)
	if elsewhere > 0 {
		line += fmt.Sprintf(" · %d marked in all", len(m.marks))
	}
	return line
}
