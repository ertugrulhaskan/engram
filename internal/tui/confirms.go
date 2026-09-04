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

// pullPlanMsg carries the accounting team.PullPlan computed in the background,
// and the targets that walk resolved: y applies exactly these, so the confirmed
// accounting is the applied accounting even if the alias map moves meanwhile.
type pullPlanMsg struct {
	res     team.PullResult
	targets []team.ProjectTarget
	err     error
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
	if r.Placed+r.Updated+r.Removed+r.Demoted == 0 {
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
	m.pullTargets = msg.targets
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
		m.pullTargets = nil // a cancelled plan is never applied later
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
	if r.Demoted > 0 {
		body = append(body, pluralLine(r.Demoted,
			"1 you withdrew elsewhere — kept here, marked personal.",
			"%d you withdrew elsewhere — kept here, marked personal."))
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
	if r.Skipped > 0 {
		// The accounting claims to be full, so a memory with no local project
		// to land in has to be named here too, not only on the no-work path.
		body = append(body, pluralLine(r.Skipped,
			"1 has no matching local project — skipped.",
			"%d have no matching local project — skipped."))
	}
	// This line used to read "Global memories are skipped; take those with
	// resolve." That stopped being true when pull learned to reconcile globals:
	// the confirm was describing the opposite of what the y key was about to do,
	// on a dialog whose whole job is informed consent for a destructive action.
	// Two entries, each short enough to survive wrapPlain's wrap intact.
	body = append(body, "A global memory you already hold is reconciled too.")
	body = append(body, "One you hold nowhere stays in the store.")
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
		path, tmp := m.resolvePath, m.resolveTmp
		m.clearResolve()
		return m, m.resolveCmd(path, tmp)
	case "esc", "n", "ctrl+c":
		m.mode = modeNormal
		team.AbortConflictResolve(m.resolveTmp) // the merge file is unused — remove it
		m.clearResolve()
		return m, m.setCancel("resolve cancelled — memory untouched")
	}
	return m, nil
}

// clearResolve drops everything the resolve confirm was holding, on both ways
// out of it. Two reasons it takes all of it and not just the merge file. The
// sides are whole memory bodies, kept on the Model only so a resize can re-diff
// them, and a session that resolved one 4,000-line memory held both copies
// until it exited. And they are stale state waiting for a caller: actionResolve
// returns early when team.BeginConflictResolve fails, so anything that rendered
// resolveModal without going through it would draw the *previous* memory's
// diff under the current memory's name.
func (m *Model) clearResolve() {
	m.resolvePath, m.resolveTmp = "", ""
	m.resolveYours, m.resolveTheirs = nil, nil
	m.resolveAligned, m.resolveChanged = nil, false
	m.resolveRows = nil
	m.resolveIdent = false
	m.resolveSame = resolveDiffers // the zero value; with no rows it draws nothing
}

// Shape of the inline diff in the resolve confirm: two lines of context around
// each change, and a row budget so a long conflict can't outgrow the frame.
const (
	resolveDiffContext = 2
	resolveDiffMaxRows = 12 // the most diff rows worth showing before "see it in $EDITOR"
	resolveDiffMinRows = 3  // fewer rows than this says nothing useful, so show none at all
	// The modal's two text blocks, named once because the row budget measures
	// the same strings the modal renders. Reworded in place, a sentence that
	// wrapped to one more row would cost the footer without the budget noticing.
	resolveLegend    = "− yours · + the team store"
	resolveMechanics = "Opens in $EDITOR. Your merge is written back and re-anchored."
	// The three lines that stand in for the diff when there is nothing to draw.
	// They replace the legend *and* the rows, so the modal is at its shortest
	// here — but only while they stay short enough not to wrap past two rows on
	// the narrowest dialog (a 30-cell box leaves a 26-cell text column), which
	// is what keeps these branches inside the frame that the diff branch is
	// budgeted into. resolveMsgInvisible names line endings and nothing else on
	// purpose: contentLines normalizes CRLF and one trailing empty line, so a
	// trailing *space* survives it and shows up as an ordinary changed row.
	resolveMsgIdentical = "Identical — only the sync anchor differs."
	resolveMsgInvisible = "Line endings or a final newline differ."
	resolveMsgNoRoom    = "The frame is too short to preview the diff."
)

// resolveChromeRows is what the modal costs besides the diff rows: the header
// block (2), the legend, two blank lines, the mechanics line, a blank, the
// footer, and the frame's own two border rows. The two text blocks are
// measured through the same wrap the renderer uses instead of being counted as
// one row each — the mechanics sentence is 61 cells against a text column of
// boxWidth()-4, so under 80 columns it takes two rows, which a hand-counted
// constant missed.
func (m Model) resolveChromeRows() int {
	tw := m.boxWidth() - 4
	return 8 + len(wrapPlain(resolveLegend, tw)) + len(wrapPlain(resolveMechanics, tw))
}

// resolveDiffRows is how many diff rows fit. The budget is dialogRows — what a
// floating box can occupy, which is neither the terminal's height nor even the
// frame's. Sized against m.height the modal came out a row too tall and lost
// the footer off the bottom: the only statement of what enter does, and exactly
// what deriving this was meant to protect.
// It returns 0 when the frame can't seat even resolveDiffMinRows: the modal
// then says so in a line instead of keeping its rows. A preview bought by
// pushing the footer off the bottom is the wrong trade — it costs the sentence
// that says what enter does to show a diff the user can't act on, and $EDITOR
// is about to show the whole thing anyway.
//
// What this budget cannot do is make the modal fit a frame too small for the
// dialog anatomy itself. With no diff rows at all the box is still 9 rows —
// header (2), the message, a blank, the mechanics line, a blank, the footer,
// two borders — so below roughly 11 terminal rows (or 14 under 42 columns,
// where the mechanics line takes three) it is clipped like every other dialog
// at that size; helpModal is 28 rows against a 12-row frame there. That floor
// belongs to the shared anatomy, not to this function.
func (m Model) resolveDiffRows() int {
	n := m.dialogRows() - m.resolveChromeRows()
	if n > resolveDiffMaxRows {
		n = resolveDiffMaxRows
	}
	if n < resolveDiffMinRows {
		return 0
	}
	return n
}

// setResolveDiff (re)computes the inline diff for the frame as it is now. The
// confirm calls it when it opens and again on every resize: the row budget is
// derived from the frame, so rows sized for the old one are precisely the
// too-tall dialog that budget exists to prevent.
func (m *Model) setResolveDiff() {
	// The alignment is cached because only the row budget depends on the frame:
	// a drag-resize is a stream of WindowSizeMsg, and re-running the O(n*m) LCS
	// per event — 250k cells for two 500-line memories, on the event loop — to
	// change one integer is what made the dialog stutter. alignResolve is
	// recomputed only when the sides change (actionResolve calls setResolveSides).
	rows, changed := m.resolveAligned, m.resolveChanged
	if n := m.resolveDiffRows(); !changed || n < 1 {
		rows = nil
	} else {
		rows = capDiff(rows, n)
	}
	m.resolveRows = rows
	switch {
	case changed && len(rows) > 0:
		m.resolveSame = resolveDiffers
	case changed:
		m.resolveSame = resolveNoRoom
	case m.resolveIdent:
		m.resolveSame = resolveIdentical
	default:
		// Nothing the diff can show, yet the bytes differ — so it is one of the
		// things contentLines normalizes: a CRLF copy, or a trailing newline.
		// (A trailing *space* survives it and shows as an ordinary changed row.)
		// Saying "identical" here would be wrong.
		m.resolveSame = resolveInvisible
	}
}

// setResolveSides takes the two versions of a conflicting memory and runs the
// frame-independent half of the diff once. setResolveDiff then re-caps it for
// whatever frame is current, including after a resize, without re-aligning.
func (m *Model) setResolveSides(yours, theirs []string, identical bool) {
	m.resolveYours, m.resolveTheirs, m.resolveIdent = yours, theirs, identical
	m.resolveAligned, m.resolveChanged = alignDiff(yours, theirs, resolveDiffContext)
	m.setResolveDiff()
}

// resolveModal shows an inline diff of the two sides — yours in the same color
// the row badges use for "ahead", the store's in the "behind" color, so the
// legend is one the user already knows — so the decision is made before
// $EDITOR opens.
func (m Model) resolveModal() string {
	t := m.theme()
	cw := m.boxWidth()
	panel := m.panelBg()
	lines := m.dlgHeader(cw, "↔", "resolve — both sides moved", t.Danger)
	switch {
	case m.resolveSame == resolveIdentical:
		lines = append(lines, m.dlgText(cw, resolveMsgIdentical, t.Dim)...)
	case m.resolveSame == resolveInvisible:
		lines = append(lines, m.dlgText(cw, resolveMsgInvisible, t.Warn)...)
	case m.resolveSame == resolveNoRoom:
		// The sides differ, but the rows won't fit. Say that rather than draw a
		// truncated preview, and let the mechanics line below point at $EDITOR.
		lines = append(lines, m.dlgText(cw, resolveMsgNoRoom, t.Warn)...)
	case len(m.resolveRows) > 0:
		lines = append(lines, m.dlgText(cw, resolveLegend, t.Faint)...)
		lines = append(lines, padBG("", cw, panel))
		for _, r := range m.resolveRows {
			mark, txt, c := " ", r.text, t.Dim
			switch r.op {
			case diffYours:
				mark, c = "−", t.Warn
			case diffTheirs:
				mark, c = "+", t.Info
			case diffElide:
				mark, c = "⋮", t.Faint
				txt = pluralLine(r.n, "1 unchanged line", "%d unchanged lines")
			case diffMore:
				// Not "unchanged": the cap cuts the diff wherever it fell.
				mark, c = "⋮", t.Warn
				txt = pluralLine(r.n, "1 more line — see it all in $EDITOR", "%d more lines — see them all in $EDITOR")
			}
			lines = append(lines, padBG(onbg(c, panel).Render(clip("  "+mark+" "+txt, cw)), cw, panel))
		}
	}
	lines = append(lines, padBG("", cw, panel))
	lines = append(lines, m.dlgText(cw, resolveMechanics, t.Dim)...)
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
