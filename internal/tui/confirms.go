package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/team"
)

// The three pre-action confirms (design spec 07): a pull shows its full
// accounting before anything moves, a resolve shows the first conflict hunk
// before $EDITOR opens, and a reconcile names the files it will index. Each
// zero-work case skips its dialog and says so in the status bar instead.

// --- pull confirm (modePullConfirm) ---

// pullPlanMsg carries the accounting team.PullPlan computed in the background.
type pullPlanMsg struct {
	res team.PullResult
	err error
}

// applyPullPlan routes a finished plan: errors and zero-work plans go straight
// to the status bar; a plan with real work opens the accounting dialog.
func (m Model) applyPullPlan(msg pullPlanMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeNormal {
		// The plan landed while another dialog was open — never stomp it. The
		// plan is discarded; pressing p again replans against a fresh fetch.
		return m, nil
	}
	if msg.err != nil {
		return m, m.setDanger("pull: " + msg.err.Error())
	}
	r := msg.res
	if r.Placed+r.Updated+r.Removed == 0 {
		parts := []string{"nothing to pull"}
		if r.Conflicts > 0 {
			parts = append(parts, fmt.Sprintf("%d in conflict — resolve with r", r.Conflicts))
		}
		if r.Ahead > 0 {
			parts = append(parts, fmt.Sprintf("%d ahead (yours to promote)", r.Ahead))
		}
		if r.UpToDate > 0 {
			parts = append(parts, fmt.Sprintf("%d up to date", r.UpToDate))
		}
		if r.Skipped > 0 {
			parts = append(parts, fmt.Sprintf("%d skipped", r.Skipped))
		}
		s := strings.Join(parts, " · ")
		if r.Conflicts > 0 {
			return m, m.setDanger(s)
		}
		return m, m.setStatus(s)
	}
	m.pullPlan = r
	m.mode = modePullConfirm
	return m, nil
}

func (m Model) updatePullConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		m.mode = modeNormal
		return m, tea.Batch(m.setStatus("pulling…"), m.applyPullCmd())
	case "esc", "n", "ctrl+c":
		m.mode = modeNormal
		return m, m.setCancel("pull cancelled — nothing moved")
	}
	return m, nil
}

// pullModal shows the plan's full accounting, one line per non-zero bucket.
func (m Model) pullModal() string {
	t := m.theme()
	r := m.pullPlan
	var body []string
	if r.Updated > 0 {
		body = append(body, pluralLine(r.Updated,
			"1 project memory fast-forwards cleanly.",
			"%d project memories fast-forward cleanly."))
	}
	if r.Placed > 0 {
		body = append(body, pluralLine(r.Placed,
			"1 new team memory arrives.",
			"%d new team memories arrive."))
	}
	if r.Removed > 0 {
		body = append(body, pluralLine(r.Removed,
			"1 withdrawn upstream — removed here.",
			"%d withdrawn upstream — removed here."))
	}
	if r.Ahead > 0 {
		body = append(body, pluralLine(r.Ahead,
			"1 has local edits — left alone.",
			"%d have local edits — left alone."))
	}
	if r.Conflicts > 0 {
		body = append(body, pluralLine(r.Conflicts,
			"1 diverged — flagged as a conflict, not overwritten.",
			"%d diverged — flagged as conflicts, not overwritten."))
	}
	if r.UpToDate > 0 {
		body = append(body, pluralLine(r.UpToDate,
			"1 already up to date.",
			"%d already up to date."))
	}
	body = append(body, "Global memories are skipped; take those with resolve.")
	return m.dialog("←", "pull from the team store", t.Info,
		body, []dialogAction{{"esc cancel", false}, {"y pull", true}})
}

// pluralLine picks the singular or plural sentence for a count.
func pluralLine(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return fmt.Sprintf(many, n)
}

// --- resolve confirm (modeResolveConfirm) ---

func (m Model) updateResolveConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		m.mode = modeNormal
		return m, m.resolveCmd(m.resolvePath, m.resolveTmp)
	case "esc", "n", "ctrl+c":
		m.mode = modeNormal
		team.AbortConflictResolve(m.resolveTmp) // the merge file is unused — remove it
		m.resolveTmp = ""
		return m, m.setCancel("resolve cancelled — memory untouched")
	}
	return m, nil
}

// resolveModal shows the first conflict hunk — marker lines in Danger — so the
// decision is made before $EDITOR opens.
func (m Model) resolveModal() string {
	t := m.theme()
	cw := m.boxWidth()
	panel := m.panelBg()
	lines := m.dlgHeader(cw, "↔", "resolve — both sides moved", t.Danger)
	for _, ln := range m.resolveHunk {
		c := t.Dim
		if strings.HasPrefix(ln, "<<<<<<<") || strings.HasPrefix(ln, "=======") || strings.HasPrefix(ln, ">>>>>>>") {
			c = t.Danger
		}
		lines = append(lines, padBG(onbg(c, panel).Render(clip("  "+ln, cw)), cw, panel))
	}
	lines = append(lines, padBG("", cw, panel))
	lines = append(lines, m.dlgText(cw, "Opens in $EDITOR. Your merge is written back and re-anchored.", t.Dim)...)
	lines = append(lines, padBG("", cw, panel))
	bleed := map[int]string{len(lines): t.Bg2}
	lines = append(lines, m.dlgFooter(cw, t.Danger, []dialogAction{{"esc cancel", false}, {"↵ open $EDITOR", true}}))
	return m.frameLines(lines, cw, t.Danger, bleed)
}

// --- reconcile confirm (modeReconcileConfirm) ---

func (m Model) updateReconcileConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		m.mode = modeNormal
		dir := m.driftDir
		project := ""
		if it, ok := m.selected(); ok {
			project = it.Context
		}
		added, removed, err := memory.ReconcileIndex(dir)
		if err != nil {
			return m, m.setDanger("index rebuild failed: " + err.Error())
		}
		m.driftDir = "" // force a fresh drift check after the rebuild
		return m, tea.Batch(m.setStatus(reconcileToast(added, removed, project)), reloadCmd())
	case "esc", "n", "ctrl+c":
		m.mode = modeNormal
		return m, m.setCancel("cancelled — index untouched")
	}
	return m, nil
}

// reconcileToast names what the rebuild actually did to which index.
func reconcileToast(added, removed int, project string) string {
	target := project + "/MEMORY.md"
	switch {
	case added > 0 && removed > 0:
		return fmt.Sprintf("%d index line%s written · %d removed — %s",
			added, plural(added), removed, target)
	case added > 0:
		return fmt.Sprintf("%d index line%s written to %s", added, plural(added), target)
	case removed > 0:
		return fmt.Sprintf("%d index line%s removed from %s", removed, plural(removed), target)
	default:
		return "index already in sync"
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// pick is a tiny ternary for toast grammar.
func pick(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

// reconcileModal names the drifted files and what a rebuild will do to them.
func (m Model) reconcileModal() string {
	t := m.theme()
	it, _ := m.selected()
	var body []string
	if n := len(m.driftUnindexed); n > 0 {
		body = append(body,
			pluralLine(n, "1 file on disk has no index line:", "%d files on disk have no index line:"),
			nameList(m.driftUnindexed))
	}
	if n := len(m.driftDangling); n > 0 {
		body = append(body,
			pluralLine(n, "1 index line points at no file:", "%d index lines point at no file:"),
			nameList(m.driftDangling))
	}
	switch {
	case len(m.driftUnindexed) > 0 && len(m.driftDangling) > 0:
		body = append(body, "engram writes the missing lines using each file's title and first sentence, and removes the dead ones. Nothing else in the index is touched.")
	case len(m.driftUnindexed) > 0:
		body = append(body, "engram writes the missing lines using each file's title and first sentence. Nothing else in the index is touched.")
	default:
		body = append(body, "engram removes the dead lines. Nothing else in the index is touched.")
	}
	return m.dialog("△", "reconcile "+it.Context+"/MEMORY.md", t.Warn,
		body, []dialogAction{{"esc cancel", false}, {"y reconcile", true}})
}

// nameList joins filenames for a dialog body, capped so a badly drifted
// project doesn't produce a screen-tall dialog.
func nameList(names []string) string {
	const maxNames = 6
	if len(names) <= maxNames {
		return strings.Join(names, " · ")
	}
	return strings.Join(names[:maxNames], " · ") + fmt.Sprintf(" · +%d more", len(names)-maxNames)
}
