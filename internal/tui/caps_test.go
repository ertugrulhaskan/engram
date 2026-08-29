package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/source"
)

// toSource switches the ready fixture to the named palette source ("/plans",
// "/files") — the same route a user takes, so the switch itself stays covered.
func toSource(t *testing.T, name string) Model {
	t.Helper()
	var m tea.Model = ready(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = typeRunes(m, name)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return m.(Model)
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
		var tm tea.Model = m
		tm, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		got := tm.(Model)
		if got.mode != modeNormal {
			t.Errorf("key %q changed mode to %v on plans (want silent no-op)", key, got.mode)
		}
		if got.status != "" {
			t.Errorf("key %q flashed %q on plans (want silent no-op)", key, got.status)
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
