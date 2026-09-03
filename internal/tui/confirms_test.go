package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/team"
)

// A pull plan with real work opens the accounting dialog; y applies (status
// flips to pulling), esc cancels with nothing moved.
func TestPullConfirmFlow(t *testing.T) {
	var m tea.Model = New(sampleMemories(), nil, nil, config.Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	plan := team.PullResult{Updated: 3, Ahead: 1, Conflicts: 1, UpToDate: 2}
	m, _ = m.Update(pullPlanMsg{res: plan})
	got := m.(Model)
	if got.mode != modePullConfirm {
		t.Fatalf("plan with work should open the confirm, mode=%v", got.mode)
	}
	out := got.View()
	for _, want := range []string{
		"pull from the team store",
		"3 project memories fast-forward cleanly.",
		"1 has local edits — left alone.",
		"1 diverged — flagged as a conflict, not overwritten.",
		"A global memory you already hold is reconciled too.",
		"One you hold nowhere stays in the store.",
		"[y pull]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("pull confirm missing %q:\n%s", want, out)
		}
	}

	// esc cancels: back to normal, nothing claimed.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := m2.(Model); got.mode != modeNormal || !strings.Contains(got.status, "nothing moved") {
		t.Errorf("esc: mode=%v status=%q", got.mode, got.status)
	}

	// y applies: the status turns to pulling and a command is returned.
	m3, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if got := m3.(Model); got.mode != modeNormal || got.status != "pulling…" {
		t.Errorf("y: mode=%v status=%q", got.mode, got.status)
	}
	if cmd == nil {
		t.Error("y should return the apply command")
	}
}

// Zero-work plans (nothing would be written) never open the dialog — the
// status bar explains instead, danger-colored when conflicts are the reason.
func TestPullConfirmZeroWorkSkips(t *testing.T) {
	var m tea.Model = New(sampleMemories(), nil, nil, config.Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	m2, _ := m.Update(pullPlanMsg{res: team.PullResult{UpToDate: 4, Skipped: 1}})
	if got := m2.(Model); got.mode != modeNormal || !strings.Contains(got.status, "nothing to pull") {
		t.Errorf("clean zero-work: mode=%v status=%q", got.mode, got.status)
	}

	m3, _ := m.Update(pullPlanMsg{res: team.PullResult{Conflicts: 2}})
	got := m3.(Model)
	if got.mode != modeNormal || !strings.Contains(got.status, "2 in conflict") {
		t.Errorf("conflict-only zero-work: mode=%v status=%q", got.mode, got.status)
	}
	if got.statusKind != statusDanger {
		t.Errorf("conflict-only status should be danger, got %v", got.statusKind)
	}
}

// The resolve confirm shows the hunk; esc removes the unused merge temp file,
// y hands it to $EDITOR.
func TestResolveConfirmFlow(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "merge.md")
	if err := os.WriteFile(tmp, []byte("<<<<<<< yours (local)\nmine\n=======\ntheirs\n>>>>>>> team\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var tm tea.Model = New(sampleMemories(), nil, nil, config.Config{})
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := tm.(Model)
	m.mode = modeResolveConfirm
	m.resolvePath = "/x/mem.md"
	m.resolveTmp = tmp
	m.resolveRows, _ = resolveDiff([]string{"mine"}, []string{"theirs"}, resolveDiffContext, m.resolveDiffRows())

	out := m.View()
	for _, want := range []string{"resolve — both sides moved", "− yours · + the team store", "− mine", "+ theirs", "Opens in $EDITOR."} {
		if !strings.Contains(out, want) {
			t.Errorf("resolve confirm missing %q:\n%s", want, out)
		}
	}

	// esc: cancel and remove the temp file.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := m2.(Model); got.mode != modeNormal || !strings.Contains(got.status, "memory untouched") {
		t.Errorf("esc: mode=%v status=%q", got.mode, got.status)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("esc should remove the unused merge temp file")
	}

	// y (fresh file): hands off to the editor command.
	if err := os.WriteFile(tmp, []byte("<<<<<<< yours (local)\nx\n=======\ny\n>>>>>>> team\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Error("confirming should return the $EDITOR command")
	}
}

// esc on the reconcile confirm leaves the index untouched.
func TestReconcileConfirmCancel(t *testing.T) {
	dir, mem := driftedProject(t, "acme")
	before, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
	if err != nil {
		t.Fatal(err)
	}

	var m tea.Model = New([]memory.Memory{mem}, nil, nil, config.Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	if m.(Model).mode != modeReconcileConfirm {
		t.Fatalf("R should confirm first, mode=%v", m.(Model).mode)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := m.(Model); got.mode != modeNormal || !strings.Contains(got.status, "index untouched") {
		t.Errorf("esc: mode=%v status=%q", got.mode, got.status)
	}
	after, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("cancelled reconcile rewrote MEMORY.md")
	}
}

// reconcileToast names what actually happened, in every branch.
func TestReconcileToast(t *testing.T) {
	cases := []struct {
		added, removed int
		want           string
	}{
		{2, 0, "2 index lines written to acme/MEMORY.md"},
		{1, 0, "1 index line written to acme/MEMORY.md"},
		{0, 1, "1 index line removed from acme/MEMORY.md"},
		{2, 1, "2 index lines written · 1 removed — acme/MEMORY.md"},
		{0, 0, "index already in sync"},
	}
	for _, c := range cases {
		if got := reconcileToast(c.added, c.removed, "acme"); got != c.want {
			t.Errorf("reconcileToast(%d,%d)=%q, want %q", c.added, c.removed, got, c.want)
		}
	}
}

// Both ways out of the resolve confirm drop what it was holding. The sides are
// whole memory bodies kept on the Model only so a resize can re-diff them, and
// they are stale state waiting for a caller: actionResolve returns early when
// BeginConflictResolve fails, so anything rendering resolveModal without going
// through it would draw the previous memory's diff under the current name.
func TestResolveConfirmClearsItsStateOnBothExits(t *testing.T) {
	for _, key := range []string{"enter", "esc"} {
		m := ready(t)
		m.mode = modeResolveConfirm
		m.resolvePath, m.resolveTmp = "/a/memory/x.md", filepath.Join(t.TempDir(), "merge.md")
		m.setResolveSides([]string{"one", "two"}, []string{"one", "three"}, true)
		if len(m.resolveRows) == 0 {
			t.Fatalf("%s: setup produced no diff rows", key)
		}
		press := tea.KeyMsg{Type: tea.KeyEnter}
		if key == "esc" {
			press = tea.KeyMsg{Type: tea.KeyEsc}
		}
		next, _ := m.updateResolveConfirm(press)
		got := next.(Model)
		if got.resolveYours != nil || got.resolveTheirs != nil {
			t.Errorf("%s: sides retained (%d/%d lines)", key, len(got.resolveYours), len(got.resolveTheirs))
		}
		if got.resolveRows != nil {
			t.Errorf("%s: %d diff rows retained", key, len(got.resolveRows))
		}
		if got.resolvePath != "" || got.resolveTmp != "" {
			t.Errorf("%s: path=%q tmp=%q retained", key, got.resolvePath, got.resolveTmp)
		}
		if got.resolveIdent {
			t.Errorf("%s: resolveIdent retained", key)
		}
	}
}
