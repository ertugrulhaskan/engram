package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/plan"
	"github.com/ertugrulhaskan/engram/internal/team"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return withStoreTimeFetch(m, nil)

	case storeTimeMsg:
		// The store's last-change time for one behind memory came back. On
		// failure the stamp stays omitted (asked-marker prevents a retry loop).
		if msg.err == nil {
			if m.storeTimes == nil {
				m.storeTimes = map[string]time.Time{}
			}
			m.storeTimes[msg.id] = msg.t
		}
		return m, nil

	case resolveFinishedMsg:
		if msg.err != nil {
			team.AbortConflictResolve(msg.tmpPath)
			return m, m.setDanger("editor error: " + msg.err.Error())
		}
		resolved, err := team.FinishConflictResolve(msg.memPath, msg.tmpPath)
		if err != nil {
			return m, m.setDanger("resolve: " + err.Error())
		}
		if !resolved {
			return m, m.setStatus("resolve aborted — nothing changed")
		}
		return m, tea.Batch(m.setStatus("merge written back — re-anchored as synced"), reloadCmd())

	case editorFinishedMsg:
		if msg.err != nil {
			return m, m.setDanger("editor error: " + msg.err.Error())
		}
		// Editing the config file (via /settings): re-read it and apply theme +
		// editor, rather than treating it as a memory.
		if cp, _ := config.Path(); msg.path != "" && msg.path == cp {
			cfg, err := config.Read()
			if err != nil {
				// Keep the settings the session already has rather than the
				// defaults a failed parse would hand back.
				return m, m.setDanger("settings not applied — " + configErr(err))
			}
			m.editorOverride = strings.TrimSpace(cfg.Editor)
			// The scan policy is read here too: it was left behind before, so a
			// promote kept using the policy the session started with while the
			// status line said the settings had been applied.
			m.scanAction, m.scanPII = cfg.ScanAction(), cfg.ScanPII()
			var dropped []string
			m.aliases, dropped = team.CleanAliases(cfg.ProjectAliases)
			// Only switch when the value resolves — an unknown or empty theme in a
			// hand-edited config keeps the current theme instead of resetting it.
			// Applied, not saved: the file just read is the source of truth.
			if idx, ok := resolveTheme(cfg.Theme); ok {
				m.applyTheme(idx)
			}
			if len(dropped) > 0 {
				return m, m.setDanger("settings updated — projectAliases entries ignored: " + strings.Join(dropped, "; "))
			}
			return m, m.setStatus("settings updated")
		}
		// Keep the folder's MEMORY.md index in sync with the new/edited file
		// (best-effort; the reload reflects the files regardless). The toast is
		// deliberately non-committal — the editor may have exited without saving.
		if msg.path != "" {
			_ = memory.UpsertIndexForPath(msg.path)
			return m, tea.Batch(m.setStatus("back from $EDITOR — MEMORY.md refreshed"), reloadCmd())
		}
		return m, reloadCmd()

	case assistantFinishedMsg:
		// The assistant may have edited many memory/plan files (and the index),
		// so don't touch a single path — just reload. Reset the drift cache so
		// the "out of sync" badge recomputes (mirrors the R-key handler).
		m.driftDir = ""
		who := msg.label
		if who == "" {
			who = "the assistant"
		}
		if msg.err != nil {
			return m, tea.Batch(m.setDanger(who+" exited: "+msg.err.Error()), reloadCmd())
		}
		return m, tea.Batch(m.setStatus("reloaded after "+who), reloadCmd())

	case scanFinishedMsg:
		// A pre-promote secret scan came back — promote, block, or warn per policy.
		return m.applyScanResult(msg)

	case initFinishedMsg:
		// `>init` finished cloning/scaffolding the team store; reload so sync state
		// (and the badge column) recomputes now that a store exists.
		m.driftDir = ""
		if msg.err != nil {
			return m, m.setDanger("init-team: " + msg.err.Error())
		}
		return m, tea.Batch(m.setStatus("team store ready"), reloadCmd())

	case withdrawFinishedMsg:
		m.driftDir = ""
		switch {
		case msg.err != nil:
			return m, tea.Batch(m.setDanger("withdraw failed: "+msg.err.Error()), reloadCmd())
		case !msg.pushed:
			return m, tea.Batch(m.setDanger("withdrawn locally; push failed — check your git remote/creds"), reloadCmd())
		default:
			return m, tea.Batch(m.setStatus("withdrawn · tombstone pushed"), reloadCmd())
		}

	case batchScanFinishedMsg:
		return m.applyBatchScanResult(msg)

	case promoteFinishedMsg:
		// Promote stamps the local file (engram frontmatter), so reload to reflect
		// it. Reset the drift cache like the assistant handler does.
		m.driftDir = ""
		// A batch that went through has spent its marks — clear them, so the next
		// promote isn't silently aimed at a set the user already acted on. A batch
		// that FAILED keeps them: PromoteBatch prepares every item before the
		// first write, so a failure in that phase wrote nothing, and a failure
		// partway through the writes is idempotent to retry. Either way the user
		// fixes the cause and presses promote again rather than re-marking.
		if msg.count > 0 && msg.err == nil {
			m.marks, m.batchItems = nil, nil
		}
		switch {
		case msg.err != nil:
			return m, tea.Batch(m.setDanger("promote failed: "+msg.err.Error()), reloadCmd())
		case !msg.pushed:
			// The suffixes belong here too. A batch that skipped or overrode a
			// flagged memory and then failed to push is the most partial
			// outcome there is; without them it read as the cleanest.
			return m, tea.Batch(m.setDanger(promoteNoun(msg)+" locally; push failed — check your git remote/creds"+
				overrodeSuffix(msg.overrode)+skippedSuffix(msg.skipped)), reloadCmd())
		case msg.override:
			return m, tea.Batch(m.setStatus("promoted with an override — pushed"), reloadCmd())
		default:
			return m, tea.Batch(m.setStatus(promoteNoun(msg)+" to the team store · pushed"+
				overrodeSuffix(msg.overrode)+skippedSuffix(msg.skipped)), reloadCmd())
		}

	case pullPlanMsg:
		// The background PullPlan accounting is in — open the confirm (or say
		// why there is nothing to confirm).
		return m.applyPullPlan(msg)

	case pullFinishedMsg:
		m.driftDir = ""
		if msg.err != nil {
			return m, tea.Batch(m.setDanger("pull failed: "+msg.err.Error()), reloadCmd())
		}
		// The toast names what moved; the full accounting was already confirmed
		// in the pull dialog.
		r := msg.res
		var parts []string
		if r.Placed > 0 {
			parts = append(parts, fmt.Sprintf("%d new", r.Placed))
		}
		if r.Updated > 0 {
			parts = append(parts, fmt.Sprintf("%d project memor%s fast-forwarded", r.Updated, pick(r.Updated == 1, "y", "ies")))
		}
		if r.Removed > 0 {
			parts = append(parts, fmt.Sprintf("%d withdrawn removed", r.Removed))
		}
		if r.Demoted > 0 {
			parts = append(parts, fmt.Sprintf("%d marked personal", r.Demoted))
		}
		if r.Conflicts > 0 {
			parts = append(parts, fmt.Sprintf("%d conflict%s left alone", r.Conflicts, plural(r.Conflicts)))
		}
		if len(parts) == 0 {
			parts = append(parts, fmt.Sprintf("nothing to pull — %d up to date", r.UpToDate))
		}
		summary := strings.Join(parts, " · ")
		if r.Conflicts > 0 {
			return m, tea.Batch(m.setDanger(summary), reloadCmd())
		}
		return m, tea.Batch(m.setStatus(summary), reloadCmd())

	case reloadMsg:
		if msg.err != nil {
			return m, m.setDanger("reload failed: " + msg.err.Error())
		}
		// Capture the selection before rebuilding (rebuildRows re-clamps the
		// cursor by index), then restore it by path so a background reload
		// doesn't make the selection jump.
		prevPath := ""
		if mm, ok := m.selected(); ok {
			prevPath = mm.Path
		}
		m.memories = msg.mems
		m.plans = msg.plans
		m.docs = msg.docs
		m.syncStates = msg.sync
		m.fsSig = msg.sig
		// A mark whose memory left the list is dead weight that still arms the
		// batch — drop it here, where the list is replaced.
		//
		// Doing that under an open batch scope dialog has to cancel the dialog,
		// not just thin the set. With batchItems emptied, the enter branch in
		// updatePromoteScope stops reading as a batch and falls through to the
		// single-memory path, which promotes m.promotePath — unset for a batch,
		// so it fails with "open :". A partial prune is worse than the error: the
		// dialog would go on to promote a set the user never chose.
		wasBatch := m.mode == modePromoteScope && len(m.batchItems) > 0
		var markCmd tea.Cmd
		if dropped := m.pruneMarks(msg.mems); dropped > 0 && wasBatch {
			m.mode = modeNormal
			m.batchItems = nil
			markCmd = m.setCancel("marked memories changed on disk — batch cancelled")
		}
		m.previewCache = nil
		m.driftDir = "" // index may have changed — recompute on next syncPreview
		// Store timestamps go stale with the states (a pull moves the store),
		// so drop both caches and let the selected row re-ask.
		m.storeTimes, m.storeTimeAsked = nil, nil
		m.rebuildRows()
		if prevPath != "" {
			m.selectByPath(prevPath)
		}
		return withStoreTimeFetch(m, markCmd)

	case clearStatusMsg:
		if msg.seq == m.statusSeq {
			m.status = ""
		}
		return m, nil

	case pollResultMsg:
		// The poll loop re-arms here and nowhere else.
		switch {
		case msg.err != nil:
			// Transient FS error — ignore so the footer doesn't churn.
		case m.fsSig == "":
			m.fsSig = msg.sig // first poll: adopt the baseline, don't reload
		case msg.sig != m.fsSig && m.mode != modeNew && m.mode != modeConfirm && m.mode != modePalette && m.mode != modeHelp && m.mode != modePromoteScope && m.mode != modeSecretWarn && m.mode != modeWithdrawConfirm && m.mode != modePullConfirm && m.mode != modeResolveConfirm && m.mode != modeReconcileConfirm:
			// Changed on disk and no modal is open → reload. Don't update fsSig
			// here; reloadMsg sets it atomically with the new memories.
			return m, tea.Batch(reloadCmd(), pollCmd())
		}
		return m, pollCmd()

	case tea.KeyMsg:
		return withStoreTimeFetch(m.dispatchKey(msg))
	}
	return m, nil
}

// dispatchKey routes a keypress to the active mode's handler.
func (m Model) dispatchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeFilter:
		return m.updateFilter(msg)
	case modeNew:
		return m.updateNew(msg)
	case modeConfirm:
		return m.updateConfirm(msg)
	case modePalette:
		return m.updatePalette(msg)
	case modeHelp:
		return m.updateHelp(msg)
	case modePromoteScope:
		return m.updatePromoteScope(msg)
	case modeSecretWarn:
		return m.updateSecretWarn(msg)
	case modeWithdrawConfirm:
		return m.updateWithdrawConfirm(msg)
	case modePullConfirm:
		return m.updatePullConfirm(msg)
	case modeResolveConfirm:
		return m.updateResolveConfirm(msg)
	case modeReconcileConfirm:
		return m.updateReconcileConfirm(msg)
	default:
		return m.updateNormal(msg)
	}
}

// --- normal-mode keys ---

func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Any normal-mode key clears a lingering status (e.g. "deleted"), so the
	// footer reverts to the key hints — the status line is a transient toast.
	m.status = ""
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "1", "2", "3":
		idx := int(msg.String()[0] - '1')
		return m, m.setTheme(idx)
	case "tab":
		if m.focus == focusList {
			m.focus = focusPreview
		} else {
			m.focus = focusList
		}
		return m, nil
	case "shift+tab":
		// Cycle the source strip. Works from either focus; per-source cursor,
		// type filter, and group mode persist (switchSource clears only the search).
		m.switchSource((m.srcKind + 1) % srcCount)
		return m, nil
	case "up", "k":
		if m.focus == focusPreview {
			m.viewport.LineUp(1)
		} else {
			m.move(-1)
		}
		return m, nil
	case "down", "j":
		if m.focus == focusPreview {
			m.viewport.LineDown(1)
		} else {
			m.move(1)
		}
		return m, nil
	case "pgup":
		if m.focus == focusPreview {
			m.viewport.HalfViewUp()
		} else {
			m.page(-1)
		}
		return m, nil
	case "pgdown":
		if m.focus == focusPreview {
			m.viewport.HalfViewDown()
		} else {
			m.page(1)
		}
		return m, nil
	case "g":
		if m.srcKind != srcMemories {
			return m, nil
		}
		if m.groupBy == groupProject {
			m.groupBy = groupType
		} else {
			m.groupBy = groupProject
		}
		m.rebuildRows()
		return m, nil
	case "t":
		if m.srcKind != srcMemories {
			return m, nil
		}
		m.typeIdx = (m.typeIdx + 1) % len(typeCycle)
		m.rebuildRows()
		return m, nil
	case "/":
		m.mode = modeFilter
		m.focus = focusList
		return m, m.search.Focus()
	case " ":
		// Space marks the row for a batch promote — a sharing affordance, so it
		// follows the Share capability (memories only today: plans and the
		// read-only files source have nothing to promote).
		if !m.caps().Share {
			return m, nil
		}
		it, ok := m.selected()
		if !ok {
			return m, nil
		}
		if m.marks == nil {
			m.marks = map[string]bool{}
		}
		if m.marks[it.Path] {
			delete(m.marks, it.Path)
		} else {
			m.marks[it.Path] = true
		}
		m.move(1) // marking a run of rows shouldn't need a keystroke between each
		return m, nil
	case "a":
		// Mark every memory the filters left — the whole list, not just the rows
		// on screen. The type filter is the selection, so
		// promoting every `feedback` memory is `t` until the chip reads feedback,
		// then `a` — no per-row marking, and no separate "promote a type" path to
		// keep in step with the batch one.
		if !m.caps().Share {
			return m, nil
		}
		return m.toggleMarkList()
	case "esc":
		// Marks come first: they are the most transient state esc can clear, and
		// leaving them set after an esc would arm a batch the user thinks is gone.
		// Only where they are shown, though: marks are a Share affordance, and
		// on a source that draws no mark column esc would otherwise announce
		// clearing state the user never saw — they wait for the return instead,
		// where the column draws them again and a batch promote confirms its
		// count before anything moves.
		if len(m.marks) > 0 && m.caps().Share {
			n := len(m.marks)
			m.marks, m.batchItems = nil, nil
			return m, m.setCancel(pluralLine(n, "1 mark cleared", "%d marks cleared"))
		}
		if m.search.Value() != "" {
			m.search.SetValue("")
			m.rebuildRows()
			return m, nil
		}
		// No filter to clear — dismiss the drift banner for this project until
		// the session ends (the status bar keeps offering R while drift lasts).
		if _, ok := m.driftBannerItem(); ok {
			if m.driftDismissed == nil {
				m.driftDismissed = map[string]bool{}
			}
			m.driftDismissed[m.driftDir] = true
			m.ensureVisible() // the list window grew a row back
		}
		return m, nil
	case "e":
		if !m.caps().Edit {
			return m.denyWrite()
		}
		if mm, ok := m.selected(); ok {
			return m, m.editCmd(mm.Path)
		}
		return m, nil
	case "n":
		if !m.caps().Create {
			return m.denyWrite()
		}
		m.mode = modeNew
		m.input.SetValue("")
		// boxWidth minus: 2 indent + 2 prompt ("› ") + 1 cursor cell, so the input
		// line fills the dialog without overflowing the border.
		if w := m.boxWidth() - 5; w > 8 {
			m.input.Width = w
		}
		return m, m.input.Focus()
	case "d":
		if !m.caps().Delete {
			return m.denyWrite()
		}
		if _, ok := m.selected(); ok {
			m.mode = modeConfirm
		}
		return m, nil
	case "ctrl+p", "ctrl+k":
		// Two aliases for one palette: ^P is the legacy opener, ^K the spec's.
		// Inside the palette ctrl+k keeps meaning "move up" (updatePalette).
		return m, m.openPalette("")
	case "P":
		// Direct team keys mirror the `>` palette verbs; the action methods
		// self-guard (source, git, store, state) with the same messages.
		return m.actionPromote()
	case "p":
		return m.actionPull()
	case "r":
		return m.actionResolve()
	case "@":
		// Open the palette filtered to assistants rather than launching one
		// outright: with several providers installed there is no single right
		// answer, and choosing silently would start a session the user never
		// picked. One rule for every install combination — and when none is on
		// $PATH the rows still list them all, each carrying its install hint.
		return m, m.openPalette("@")
	case "?":
		m.mode = modeHelp
		return m, nil
	case "R":
		// Rebuild the current project's MEMORY.md index — behind a confirm that
		// names the drifted files (updateReconcileConfirm does the write).
		if m.srcKind != srcMemories {
			return m, nil
		}
		dir := m.currentMemDir()
		if dir == "" {
			return m, nil
		}
		if dir == m.driftDir && m.driftErr != nil {
			// The check failed, so we don't know whether the index is in sync.
			// Saying it is would be a claim we can't back.
			return m, m.setDanger("could not check the index: " + m.driftErr.Error())
		}
		if dir != m.driftDir || len(m.driftUnindexed)+len(m.driftDangling) == 0 {
			return m, m.setStatus("index already in sync — nothing to write")
		}
		m.mode = modeReconcileConfirm
		return m, nil
	}
	return m, nil
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.search.SetValue("")
		m.search.Blur()
		m.rebuildRows()
		return m, nil
	case "enter":
		m.mode = modeNormal
		m.search.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	m.rebuildRows()
	return m, cmd
}

func (m Model) updateNew(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeNormal
		m.input.Blur()
		return m, m.setCancel("cancelled")
	case "enter":
		title := strings.TrimSpace(m.input.Value())
		m.mode = modeNormal
		m.input.Blur()
		if title == "" {
			return m, m.setCancel("cancelled")
		}
		// Only the memories source has a create routine, and the Create gate
		// lets no other source open this prompt — so any other source here is
		// a wiring gap. Refuse: currentMemDir falls back to the first project's
		// memory dir, which would otherwise plant a memory somewhere unrelated.
		if m.srcKind != srcMemories {
			return m, m.setDanger("can't create from this source")
		}
		dir := m.currentMemDir()
		if dir == "" {
			return m, m.setDanger("no project to add to")
		}
		path, err := memory.Create(dir, title)
		if err != nil {
			return m, m.setDanger("create failed: " + err.Error())
		}
		return m, m.editCmd(path)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// updateHelp dismisses the help overlay on any key (it's a transient cheat-sheet),
// except ctrl+c which still quits the app.
func (m Model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	m.mode = modeNormal
	return m, nil
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.mode = modeNormal
		if it, ok := m.selected(); ok {
			// The Delete gate decides whether a source gets this far; this
			// switch decides which routine runs, with no fall-through: a kind
			// without a routine is refused, never guessed.
			var err error
			toast := ""
			switch it.Kind {
			case "plan":
				err = plan.Delete(it.Path)
				toast = "plan deleted"
			case "memory":
				if err = memory.Delete(it.Path); err == nil {
					_ = memory.RemoveIndexForPath(it.Path) // drop its MEMORY.md bullet too
				}
				toast = "memory deleted — MEMORY.md updated"
			default:
				return m, m.setDanger("can't delete " + it.Kind + " items from here")
			}
			if err != nil {
				return m, m.setDanger("delete failed: " + err.Error())
			}
			return m, tea.Batch(m.setDanger(toast), reloadCmd())
		}
		return m, nil
	default:
		m.mode = modeNormal
		return m, m.setCancel("cancelled")
	}
}

// denyWrite answers an e/n/d key the current source's capabilities refuse: the
// source's read-only hint when it has one (files), otherwise a silent no-op —
// a denied key is also absent from the controls row, so silence matches what
// the UI advertises.
func (m Model) denyWrite() (tea.Model, tea.Cmd) {
	if hint := readOnlyHint[m.srcKind]; hint != "" {
		return m, m.setStatus(hint)
	}
	return m, nil
}

// denyShare answers a team action on a source without the Share capability:
// the verb names itself so the line reads the same from the palette and the
// key.
func (m Model) denyShare(verb string) (tea.Model, tea.Cmd) {
	return m, m.setStatus(verb + " applies to memories")
}
