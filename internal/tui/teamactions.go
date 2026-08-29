package tui

import (
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/team"
)

// noStoreHint is the one message every team action shows when the store isn't set
// up yet, pointing at the palette command that fixes it.
const noStoreHint = "no team store — run `>init <git-url>` first"

// gitMissing returns a danger command when git isn't on PATH — team sharing shells
// out to git for everything — or nil when git is available. Every team action
// checks it first so a missing git reads as a clear message, not a raw exec error.
func (m Model) gitMissing() tea.Cmd {
	if !team.HasGit() {
		return m.setDanger("git not found — team sharing needs git (https://git-scm.com)")
	}
	return nil
}

// The team actions below are the bodies behind the `>` command palette (promote /
// pull / withdraw / resolve / init). Each acts on the currently-selected memory,
// self-guards on an initialized store, and returns the next mode/command — so the
// palette dispatch stays a thin switch and the guard message stays consistent.

// actionPromote opens the scope picker for the selected memory.
func (m Model) actionPromote() (tea.Model, tea.Cmd) {
	if !m.caps().Share {
		return m.denyShare("promote")
	}
	if cmd := m.gitMissing(); cmd != nil {
		return m, cmd
	}
	if !team.IsInitialized() {
		return m, m.setDanger(noStoreHint)
	}
	// Marks win over the cursor row: if the user marked a set, that set is what
	// they mean by "promote", regardless of where the cursor happens to sit —
	// including on no row at all, because a filter hid every one. The cursor
	// lookup therefore comes after this branch: ahead of it, a marked batch
	// under a matchless filter died here silently, and esc — the natural way
	// out of a filter — clears the marks before it clears the search.
	if len(m.marks) > 0 {
		return m.actionPromoteBatch()
	}
	it, ok := m.selected()
	if !ok {
		return m, nil
	}
	key, _ := team.ProjectKey(it.ProjectDir) // "" when the project has no remote
	m.batchItems = nil                       // this promote is the cursor row, not a leftover batch
	m.promotePath = it.Path
	m.promoteTitle = it.Title
	m.promoteKey = key
	m.promoteCursor = 0
	if key == "" {
		m.promoteCursor = 1 // only "global" is available
	}
	m.mode = modePromoteScope
	return m, nil
}

// actionPull plans a pull of project-scoped team memories: the accounting is
// computed (and shown for confirmation) before anything moves.
func (m Model) actionPull() (tea.Model, tea.Cmd) {
	if !m.caps().Share {
		return m.denyShare("pull")
	}
	if cmd := m.gitMissing(); cmd != nil {
		return m, cmd
	}
	if !team.IsInitialized() {
		return m, m.setDanger(noStoreHint)
	}
	return m, tea.Batch(m.setStatus("checking the team store…"), m.planPullCmd())
}

// actionWithdraw asks to take a shared memory back (a confirm follows).
func (m Model) actionWithdraw() (tea.Model, tea.Cmd) {
	if !m.caps().Share {
		return m.denyShare("withdraw")
	}
	if cmd := m.gitMissing(); cmd != nil {
		return m, cmd
	}
	it, ok := m.selected()
	if !ok {
		return m, nil
	}
	if !team.IsInitialized() {
		return m, m.setDanger(noStoreHint)
	}
	if m.syncStates[it.Path] == team.StateNone {
		return m, m.setStatus("not shared — nothing to withdraw")
	}
	// Resolve the owner guardrail's inputs now, so the confirm can disclose an
	// unverifiable check rather than the user finding out nothing was proven.
	own, err := team.CheckOwner(it.Path)
	if err != nil {
		return m, m.setDanger("withdraw: " + err.Error())
	}
	m.withdrawOwner = own
	m.withdrawPath = it.Path
	m.withdrawTitle = it.Title
	m.mode = modeWithdrawConfirm
	return m, nil
}

// actionResolve opens both versions of a conflicting memory in $EDITOR. Diverged/
// Differs are conflicts; Incoming is included as a manual fallback. It used to be
// the only way to take a clean global update, because pull walked past a global
// memory in a project with no git remote — resolveTargets now keeps those
// projects as global-pull targets, so `p` covers the ordinary case and this
// remains for the ones it can't.
func (m Model) actionResolve() (tea.Model, tea.Cmd) {
	if !m.caps().Share {
		return m.denyShare("resolve")
	}
	if cmd := m.gitMissing(); cmd != nil {
		return m, cmd
	}
	it, ok := m.selected()
	if !ok {
		return m, nil
	}
	if !team.IsInitialized() {
		return m, m.setDanger(noStoreHint)
	}
	if s := m.syncStates[it.Path]; s != team.StateDiverged && s != team.StateDiffers && s != team.StateIncoming {
		return m, m.setStatus("nothing to resolve — already in sync")
	}
	tmp, err := team.BeginConflictResolve(it.Path)
	if err != nil {
		return m, m.setDanger("resolve: " + err.Error())
	}
	// Show the first conflict hunk before $EDITOR opens. The preview is
	// best-effort: without one the confirm still explains what happens next.
	hunk, _ := team.FirstConflictHunk(tmp, 7)
	m.resolvePath, m.resolveTmp, m.resolveHunk = it.Path, tmp, hunk
	m.mode = modeResolveConfirm
	return m, nil
}

// actionInit sets up the team store from a git URL (the one action that doesn't need
// an existing store). The URL is the argument typed after `>init`.
func (m Model) actionInit(url string) (tea.Model, tea.Cmd) {
	url = strings.TrimSpace(url)
	if url == "" {
		return m, m.setDanger("usage: >init <git-url>")
	}
	if cmd := m.gitMissing(); cmd != nil {
		return m, cmd
	}
	if team.IsInitialized() {
		return m, m.setStatus("team store already set up")
	}
	return m, m.initCmd(url)
}

// initFinishedMsg reports the outcome of an init-team run.
type initFinishedMsg struct{ err error }

// initCmd sets up the team store by re-invoking engram's own `init-team`
// subcommand as a child process via ExecProcess. That suspends the alt-screen TUI
// and hands git the real terminal, so clone progress and any credential /
// SSH-passphrase prompt behave normally — running InitTeam in-process would
// scribble over the frame and deadlock on the prompt (Bubble Tea holds the tty in
// raw mode). Mirrors the editor handoff in editor.go / resolve.go.
func (m Model) initCmd(url string) tea.Cmd {
	self, err := os.Executable()
	if err != nil || self == "" {
		self = os.Args[0]
	}
	c := exec.Command(self, "init-team", url)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return initFinishedMsg{err: err}
	})
}
