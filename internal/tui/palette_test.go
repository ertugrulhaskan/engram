package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/plan"
)

func typeRunes(m tea.Model, s string) tea.Model {
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return m
}

// Ctrl+P then "/plans" + Enter switches the active source to plans.
func TestPaletteSourceSwitch(t *testing.T) {
	var m tea.Model = ready(t) // memories + sample plans, starts on memories
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.(Model).mode != modePalette {
		t.Fatalf("Ctrl+P did not open the palette (mode=%v)", m.(Model).mode)
	}
	m = typeRunes(m, "/plans")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := m.(Model)
	if got.srcKind != srcPlans {
		t.Fatalf("srcKind=%v, want srcPlans", got.srcKind)
	}
	if got.mode != modeNormal {
		t.Errorf("palette did not close (mode=%v)", got.mode)
	}
	if it, ok := got.selected(); !ok || it.Kind != "plan" {
		t.Errorf("selected is not a plan: %+v (ok=%v)", it, ok)
	}
}

// Typing a title fragment jumps to that item (here, a memory) by path.
func TestPaletteQuickJump(t *testing.T) {
	var m tea.Model = ready(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = typeRunes(m, "terse") // matches "prefers-terse-prose"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := m.(Model)
	if got.srcKind != srcMemories {
		t.Errorf("srcKind=%v, want srcMemories", got.srcKind)
	}
	if it, ok := got.selected(); !ok || !strings.Contains(it.Title, "terse") {
		t.Errorf("did not jump to the terse memory: %+v (ok=%v)", it, ok)
	}
}

// Plans support delete; memory-only keys (t/g/n/R) are inert under plans.
func TestPlanDeleteAndGating(t *testing.T) {
	dir := t.TempDir()
	pp := filepath.Join(dir, "p.md")
	if err := os.WriteFile(pp, []byte("# Plan: X\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plans := []plan.Plan{{Title: "X", Body: "# Plan: X\n\nbody\n", Path: pp}}

	var m tea.Model = New(nil, plans, config.Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = typeRunes(m, "/plans")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.(Model).srcKind != srcPlans {
		t.Fatal("did not switch to plans")
	}

	// Memory-only keys are no-ops under plans (no panic, stays on plans).
	for _, k := range []string{"t", "g", "n", "R"} {
		m = typeRunes(m, k)
		if m.(Model).srcKind != srcPlans || m.(Model).mode != modeNormal {
			t.Fatalf("key %q changed state under plans (src=%v mode=%v)", k, m.(Model).srcKind, m.(Model).mode)
		}
	}

	// Delete the plan: d then y removes the file via plan.Delete.
	m = typeRunes(m, "d")
	if m.(Model).mode != modeConfirm {
		t.Fatalf("d did not open confirm (mode=%v)", m.(Model).mode)
	}
	m = typeRunes(m, "y")
	if _, err := os.Stat(pp); !os.IsNotExist(err) {
		t.Errorf("plan file not deleted: stat err=%v", err)
	}
}
