package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/team"
)

// batchItem is one marked memory resolved for promotion: where it lives, what to
// call it in a dialog, its own project key (empty when that project has no git
// remote), and — once the scope is picked — the placement it will be written to.
type batchItem struct {
	path      string
	title     string
	key       string // the memory's own project key; "" when the project has no remote
	placement string // resolved at the scope step: key or "global"
}

// actionPromoteBatch opens the scope picker for the marked set. The marked
// memories are collected from the full memory list rather than the visible rows:
// a filter typed after marking hides rows, and silently dropping those marks
// would promote less than the user marked without ever saying so.
func (m Model) actionPromoteBatch() (tea.Model, tea.Cmd) {
	// Marked memories cluster into far fewer projects than there are marks —
	// `t` then `a` marks a whole type, which is typically one project's worth
	// several times over. projectKey shells out to git, and this runs on the
	// Bubble Tea event loop, so resolving per memory would stall the UI for one
	// process per mark. Resolve per project instead and reuse it — per memory
	// dir, the project's stable identity, since two memory dirs can decode to
	// one best-effort project dir and would otherwise share one key.
	keys := make(map[string]string, 8)
	items := make([]batchItem, 0, len(m.marks))
	for _, mm := range m.memories {
		if !m.marks[mm.Path] {
			continue
		}
		key, seen := keys[mm.Project.MemoryDir]
		if !seen {
			key = m.projectKey(mm.Project.Dir, mm.Project.MemoryDir) // "" when the project has neither a remote nor an alias
			keys[mm.Project.MemoryDir] = key
		}
		items = append(items, batchItem{path: mm.Path, title: mm.Title, key: key})
	}
	if len(items) == 0 {
		return m, m.setStatus("nothing marked is promotable")
	}
	m.batchItems = items
	// The scope picker serves both modes and tells them apart by whether a batch
	// is set, so the two must never both be live: entering the batch clears the
	// single-memory state, exactly as the single path clears the batch.
	m.promotePath, m.promoteTitle, m.promoteKey = "", "", ""
	m.promoteCursor = 0
	if !anyKeyed(items) {
		m.promoteCursor = 1 // only "global" is available
	}
	m.mode = modePromoteScope
	return m, nil
}

// anyKeyed reports whether at least one item has a project of its own to go to.
func anyKeyed(items []batchItem) bool {
	for _, b := range items {
		if b.key != "" {
			return true
		}
	}
	return false
}

// distinctKeys counts the separate project keys in the set, and how many items
// have none. A batch can span several projects — "their own projects" means each
// memory goes to its own, which is only honest if the dialog says how many that is.
func distinctKeys(items []batchItem) (keys, remoteless int) {
	seen := map[string]bool{}
	for _, b := range items {
		if b.key == "" {
			remoteless++
			continue
		}
		if !seen[b.key] {
			seen[b.key] = true
			keys++
		}
	}
	return keys, remoteless
}

// resolvePlacements stamps each item with the placement the picked scope implies.
// Under "their own projects" a memory whose project has no remote still falls to
// global — there is nowhere else for it to go, and the dialog said so.
func resolvePlacements(items []batchItem, ownProjects bool) []batchItem {
	out := make([]batchItem, len(items))
	for i, b := range items {
		b.placement = "global"
		if ownProjects && b.key != "" {
			b.placement = b.key
		}
		out[i] = b
	}
	return out
}

// promoteItems converts the accepted set into the team package's batch input.
func promoteItems(items []batchItem) []team.PromoteItem {
	out := make([]team.PromoteItem, 0, len(items))
	for _, b := range items {
		out = append(out, team.PromoteItem{Path: b.path, Placement: b.placement})
	}
	return out
}

// promoteBatchCmd runs team.PromoteBatch off the UI thread. count and skipped
// travel with the result so the toast can report the whole act, not just its
// success — a batch where two of five were declined is not "promoted".
func (m Model) promoteBatchCmd(items []batchItem, skipped, overrode int) tea.Cmd {
	payload := promoteItems(items)
	n := len(payload)
	return func() tea.Msg {
		pushed, err := team.PromoteBatch(payload)
		return promoteFinishedMsg{pushed: pushed, count: n, skipped: skipped, overrode: overrode, err: err}
	}
}

// batchScopeSub describes what "their own projects" will actually do, so the
// choice is made against the real spread of the marked set rather than a guess.
func batchScopeSub(items []batchItem) string {
	keys, remoteless := distinctKeys(items)
	s := pluralLine(keys, "1 project key", "%d project keys")
	if remoteless > 0 {
		s += " · " + pluralLine(remoteless, "1 without a key → global", "%d without a key → global")
	}
	return s
}

// batchHeader names the batch in a dialog header. It plurals for the same reason
// the mechanics line below it does: marking one memory and pressing P is an
// ordinary thing to do, and "promote 1 memories" is the first thing that batch
// shows you.
func batchHeader(n int) string {
	return pluralLine(n, "promote 1 memory — pick a scope", "promote %d memories — pick a scope")
}

// promoteNoun words the result toast for however many memories actually went.
// A batch that promoted three is not "promoted" — the count is the outcome.
func promoteNoun(msg promoteFinishedMsg) string {
	if msg.count > 1 {
		return fmt.Sprintf("promoted %d memories", msg.count)
	}
	return "promoted"
}

// overrodeSuffix reports memories promoted past a secret-scan finding. Including
// a flagged memory is a real decision, and a result line that stayed silent about
// it would read as a clean promote.
func overrodeSuffix(overrode int) string {
	if overrode == 0 {
		return ""
	}
	return " · " + pluralLine(overrode, "1 overridden", "%d overridden")
}

// skippedSuffix reports memories the user declined during the scan walk, so a
// partial batch never reads as a complete one.
func skippedSuffix(skipped int) string {
	if skipped == 0 {
		return ""
	}
	return " · " + pluralLine(skipped, "1 skipped", "%d skipped")
}
