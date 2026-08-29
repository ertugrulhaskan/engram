package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/source"
)

// switchVia drives the palette to the named source ("/plans", "/files") on
// any model — the same route a user takes, so the switch itself stays covered.
func switchVia(m tea.Model, name string) tea.Model {
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = typeRunes(m, name)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return m
}

// toSource is switchVia over the ready fixture.
func toSource(t *testing.T, name string) Model {
	t.Helper()
	return switchVia(ready(t), name).(Model)
}

// TestCapsMatrix pins the ENGR-12 capability matrix. srcCaps is the one place
// capability is wired, and this test restates the decided matrix — a wiring
// row drifting from the decision fails here by name, the same way the
// typeLabel/typeName drift should have failed somewhere and didn't.
func TestCapsMatrix(t *testing.T) {
	want := [srcCount]source.Caps{
		srcMemories: {Edit: true, Create: true, Delete: true, Share: true},
		srcPlans:    {Delete: true},
		srcFiles:    {}, // read-only: repairs go through @Claude
	}
	if srcCaps != want {
		t.Errorf("srcCaps = %+v\nwant the ENGR-12 matrix %+v", srcCaps, want)
	}
	// The hint table is the other half of a source's policy: only the files
	// source names an escape hatch; every other denial is silent.
	wantHint := [srcCount]string{srcFiles: "read-only — edit with @Claude (ctrl+p, then @)"}
	if readOnlyHint != wantHint {
		t.Errorf("readOnlyHint = %q, want %q", readOnlyHint, wantHint)
	}
}

// Plans are view + delete only, and their denials are silent: e and n neither
// change mode nor flash a status (the keys are absent from the controls row,
// so there is nothing to explain), while d still opens the delete confirm.
func TestPlansViewAndDeleteOnly(t *testing.T) {
	for _, key := range []string{"e", "n"} {
		m := toSource(t, "/plans")
		if m.srcKind != srcPlans {
			t.Fatalf("setup: srcKind=%v, want srcPlans", m.srcKind)
		}
		before := m.status
		var tm tea.Model = m
		tm, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		got := tm.(Model)
		if got.mode != modeNormal {
			t.Errorf("key %q changed mode to %v on plans (want silent no-op)", key, got.mode)
		}
		if got.status != before {
			t.Errorf("key %q changed the status to %q on plans (want silent no-op)", key, got.status)
		}
		// A granted Edit would leave mode and status untouched and hand back the
		// editor command — the only trace of the key doing something.
		if cmd != nil {
			t.Errorf("key %q returned a command on plans (want nil: nothing to run)", key)
		}
	}

	m := toSource(t, "/plans")
	var tm tea.Model = m
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if got := tm.(Model); got.mode != modeConfirm {
		t.Errorf("d on plans gave mode %v, want modeConfirm (delete is granted)", got.mode)
	}
}

// Every team dispatcher answers a source without the Share capability with its
// "applies to memories" line instead of doing anything.
func TestTeamActionsShareGate(t *testing.T) {
	cases := []struct {
		verb   string
		action func(Model) (tea.Model, tea.Cmd)
	}{
		{"promote", Model.actionPromote},
		{"pull", Model.actionPull},
		{"withdraw", Model.actionWithdraw},
		{"resolve", Model.actionResolve},
	}
	for _, c := range cases {
		m := toSource(t, "/plans")
		tm, _ := c.action(m)
		want := c.verb + " applies to memories"
		if got := tm.(Model).status; got != want {
			t.Errorf("%s on plans: status %q, want %q", c.verb, got, want)
		}
	}
}

// The executors fail closed the way the gates do. Entering the delete confirm
// on a files row directly (the d gate would never allow it) must not remove
// the file: the routine switch has no case for a doc kind, so it refuses
// instead of falling through to memory.Delete.
func TestDeleteExecutorFailsClosed(t *testing.T) {
	for _, kind := range []string{"rules", "index"} { // both doc kinds the files source emits
		m := toSource(t, "/files")
		if _, ok := m.selected(); !ok {
			t.Fatal("setup: no files row selected")
		}
		tmp := filepath.Join(t.TempDir(), "CLAUDE.md")
		if err := os.WriteFile(tmp, []byte("# rules\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		m.rows[m.cursor].item.Kind = kind
		m.rows[m.cursor].item.Path = tmp
		m.mode = modeConfirm
		var tm tea.Model = m
		tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
		got := tm.(Model)
		if _, err := os.Stat(tmp); err != nil {
			t.Fatalf("y on a %s row removed the file: %v", kind, err)
		}
		if want := "can't delete " + kind + " items from here"; got.status != want {
			t.Errorf("%s: status %q, want %q", kind, got.status, want)
		}
		if got.mode != modeNormal {
			t.Errorf("%s: mode %v, want modeNormal", kind, got.mode)
		}
	}
}

// Likewise the create executor: reaching the new-memory prompt on a non-memories
// source (the n gate would never allow it) must not create anything —
// currentMemDir would otherwise fall back to the first project's memory dir.
func TestCreateExecutorFailsClosed(t *testing.T) {
	m := toSource(t, "/plans")
	m.mode = modeNew
	m.input.SetValue("stray")
	var tm tea.Model = m
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := tm.(Model)
	if want := "can't create from this source"; got.status != want {
		t.Errorf("status %q, want %q", got.status, want)
	}
	if got.mode != modeNormal {
		t.Errorf("mode %v, want modeNormal", got.mode)
	}
}

// Marks belong to the memories list. On a source that does not draw them, esc
// leaves them alone (and says nothing about them) rather than clearing state
// the user never saw; they are still there on the way back.
func TestEscLeavesHiddenMarksAlone(t *testing.T) {
	m := toSource(t, "/plans")
	m.marks = map[string]bool{"/x/a.md": true, "/x/b.md": true}
	var tm tea.Model = m
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := tm.(Model)
	if len(got.marks) != 2 {
		t.Errorf("esc on plans cleared hidden marks: %d left, want 2", len(got.marks))
	}
	if got.status == "2 marks cleared" {
		t.Errorf("esc on plans announced clearing marks it never showed")
	}
	// Back on memories they are drawn again, so a batch promote can't surprise.
	got.switchSource(srcMemories)
	if len(got.marks) != 2 || got.markColW() == 0 {
		t.Errorf("back on memories: %d marks, mark column width %d — want 2 marks drawn", len(got.marks), got.markColW())
	}
}
