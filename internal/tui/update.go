package tui

import (
	"fmt"
	"strings"

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
		return m, nil

	case editorFinishedMsg:
		if msg.err != nil {
			return m, m.setDanger("editor error: " + msg.err.Error())
		}
		// Editing the config file (via /settings): re-read it and apply theme +
		// editor, rather than treating it as a memory.
		if cp, _ := config.Path(); msg.path != "" && msg.path == cp {
			cfg := config.Load()
			m.editorOverride = strings.TrimSpace(cfg.Editor)
			for i, th := range themes {
				if th.Name == cfg.Theme {
					m.setTheme(i)
					break
				}
			}
			return m, m.setStatus("settings updated")
		}
		// Keep the folder's MEMORY.md index in sync with the new/edited file
		// (best-effort; the reload reflects the files regardless).
		if msg.path != "" {
			_ = memory.UpsertIndexForPath(msg.path)
		}
		return m, reloadCmd()

	case assistantFinishedMsg:
		// The assistant may have edited many memory/plan files (and the index),
		// so don't touch a single path — just reload. Reset the drift cache so
		// the "out of sync" badge recomputes (mirrors the R-key handler).
		m.driftDir = ""
		if msg.err != nil {
			return m, tea.Batch(m.setDanger("claude exited: "+msg.err.Error()), reloadCmd())
		}
		return m, tea.Batch(m.setStatus("reloaded after @Claude"), reloadCmd())

	case promoteFinishedMsg:
		// Promote stamps the local file (engram frontmatter), so reload to reflect
		// it. Reset the drift cache like the assistant handler does.
		m.driftDir = ""
		switch {
		case msg.err != nil:
			return m, tea.Batch(m.setDanger("promote failed: "+msg.err.Error()), reloadCmd())
		case !msg.pushed:
			return m, tea.Batch(m.setDanger("promoted locally; push failed — check your git remote/creds"), reloadCmd())
		default:
			return m, tea.Batch(m.setStatus("promoted to team"), reloadCmd())
		}

	case pullFinishedMsg:
		m.driftDir = ""
		if msg.err != nil {
			return m, tea.Batch(m.setDanger("pull failed: "+msg.err.Error()), reloadCmd())
		}
		r := msg.res
		summary := fmt.Sprintf("pull: %d new · %d up-to-date · %d conflict · %d skipped",
			r.Placed, r.UpToDate, r.Conflicts, r.Skipped)
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
		m.fsSig = msg.sig
		m.previewCache = nil
		m.driftDir = "" // index may have changed — recompute on next syncPreview
		m.rebuildRows()
		if prevPath != "" {
			m.selectByPath(prevPath)
		}
		return m, nil

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
		case msg.sig != m.fsSig && m.mode != modeNew && m.mode != modeConfirm && m.mode != modePalette && m.mode != modeHelp && m.mode != modePromoteScope:
			// Changed on disk and no modal is open → reload. Don't update fsSig
			// here; reloadMsg sets it atomically with the new memories.
			return m, tea.Batch(reloadCmd(), pollCmd())
		}
		return m, pollCmd()

	case tea.KeyMsg:
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
		default:
			return m.updateNormal(msg)
		}
	}
	return m, nil
}

// --- normal-mode keys ---

func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Any normal-mode key clears a lingering status (e.g. "deleted"), so the
	// footer reverts to the key hints — the status line is a transient toast.
	m.status = ""
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "1", "2", "3", "4", "5":
		idx := int(msg.String()[0] - '1')
		m.setTheme(idx)
		return m, nil
	case "tab":
		if m.focus == focusList {
			m.focus = focusPreview
		} else {
			m.focus = focusList
		}
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
	case "esc":
		if m.search.Value() != "" {
			m.search.SetValue("")
			m.rebuildRows()
		}
		return m, nil
	case "e":
		if m.srcKind == srcFiles { // CLAUDE.md / MEMORY.md are read-only here
			return m, m.setStatus("read-only — edit with @Claude (ctrl+p, then @)")
		}
		if m.srcKind != srcMemories { // plans are view + delete only
			return m, nil
		}
		if mm, ok := m.selected(); ok {
			return m, m.editCmd(mm.Path)
		}
		return m, nil
	case "n":
		if m.srcKind == srcFiles { // read-only: no creating CLAUDE.md / MEMORY.md here
			return m, m.setStatus("read-only — edit with @Claude (ctrl+p, then @)")
		}
		if m.srcKind != srcMemories {
			return m, nil
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
		if m.srcKind == srcFiles { // read-only: never delete a CLAUDE.md / MEMORY.md
			return m, m.setStatus("read-only — edit with @Claude (ctrl+p, then @)")
		}
		if _, ok := m.selected(); ok {
			m.mode = modeConfirm
		}
		return m, nil
	case "p":
		// Promote the selected memory to the team store. Memories only; needs an
		// initialized team store. The scope picker (this project / global) follows.
		if m.srcKind != srcMemories {
			return m, nil
		}
		it, ok := m.selected()
		if !ok {
			return m, nil
		}
		if !team.IsInitialized() {
			return m, m.setDanger("no team store — run `engram init-team <git-url>` first")
		}
		key, _ := team.ProjectKey(it.ProjectDir) // "" when the project has no remote
		m.promotePath = it.Path
		m.promoteTitle = it.Title
		m.promoteKey = key
		m.promoteCursor = 0
		if key == "" {
			m.promoteCursor = 1 // only "global" is available
		}
		m.mode = modePromoteScope
		return m, nil
	case "P":
		// Pull project-scoped team memories into their matching local projects.
		if m.srcKind != srcMemories {
			return m, nil
		}
		if !team.IsInitialized() {
			return m, m.setDanger("no team store — run `engram init-team <git-url>` first")
		}
		return m, tea.Batch(m.setStatus("pulling…"), m.pullCmd())
	case "ctrl+p":
		m.mode = modePalette
		m.palette.SetValue("")
		m.rebuildPalette()
		return m, m.palette.Focus()
	case "?":
		m.mode = modeHelp
		return m, nil
	case "R":
		// Rebuild the current project's MEMORY.md index: drop dangling bullets,
		// add unindexed files, preserve order. Fixes drift from external moves.
		if m.srcKind != srcMemories {
			return m, nil
		}
		dir := m.currentMemDir()
		if dir == "" {
			return m, nil
		}
		added, removed, err := memory.ReconcileIndex(dir)
		if err != nil {
			return m, m.setDanger("index rebuild failed: " + err.Error())
		}
		m.driftDir = "" // force a fresh drift check after the rebuild
		return m, tea.Batch(m.setStatus(fmt.Sprintf("index rebuilt  +%d  −%d", added, removed)), reloadCmd())
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
			var err error
			if it.Kind == "plan" {
				err = plan.Delete(it.Path)
			} else {
				err = memory.Delete(it.Path)
			}
			if err != nil {
				return m, m.setDanger("delete failed: " + err.Error())
			}
			if it.Kind == "memory" {
				_ = memory.RemoveIndexForPath(it.Path) // drop its MEMORY.md bullet too
			}
			return m, tea.Batch(m.setDanger("deleted “"+clip(it.Title, 40)+"”"), reloadCmd())
		}
		return m, nil
	default:
		m.mode = modeNormal
		return m, m.setCancel("cancelled")
	}
}
