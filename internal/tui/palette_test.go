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

// The empty palette is a guide: three rows ("/" commands, "@" assistant, ">"
// team). Pressing Enter on the "/" row seeds "/" and reveals the command list
// without closing.
func TestPaletteEmptyGuide(t *testing.T) {
	var m tea.Model = ready(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	rows := m.(Model).palRows
	if len(rows) != 3 {
		t.Fatalf("empty palette = %d rows, want 3 (/, @ and > guides): %+v", len(rows), rows)
	}
	if rows[0].action != palPrefix || rows[0].prefix != "/" {
		t.Fatalf("first guide row = %+v, want palPrefix '/'", rows[0])
	}
	if rows[1].action != palPrefix || rows[1].prefix != "@" {
		t.Fatalf("second guide row = %+v, want palPrefix '@'", rows[1])
	}
	if rows[2].action != palPrefix || rows[2].prefix != ">" {
		t.Fatalf("third guide row = %+v, want palPrefix '>'", rows[2])
	}

	// Enter on the "/" guide seeds the prefix and lists the commands in-place.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := m.(Model)
	if got.mode != modePalette {
		t.Fatalf("palette closed after a guide row (mode=%v), want it to stay open", got.mode)
	}
	if got.palette.Value() != "/" {
		t.Fatalf("guide did not seed '/' (value=%q)", got.palette.Value())
	}
	if len(got.palRows) != len(got.paletteCommands()) {
		t.Fatalf("after '/' = %d rows, want %d commands", len(got.palRows), len(got.paletteCommands()))
	}
}

// Typing ">" lists the team verbs; ">prom" narrows to promote; ">init <url>"
// captures the URL as the init argument.
func TestPaletteTeamVerbs(t *testing.T) {
	var m tea.Model = ready(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = typeRunes(m, ">")
	if got, want := len(m.(Model).palRows), len(m.(Model).teamVerbs()); got != want {
		t.Fatalf("> shows %d verbs, want %d", got, want)
	}

	narrowed := typeRunes(m, "prom").(Model).palRows
	if len(narrowed) != 1 || narrowed[0].action != palPromote {
		t.Fatalf(">prom = %+v, want a single promote row", narrowed)
	}

	initRows := typeRunes(m, "init file:///tmp/x.git").(Model).palRows
	if len(initRows) != 1 || initRows[0].action != palInit || initRows[0].arg != "file:///tmp/x.git" {
		t.Fatalf(">init = %+v, want palInit with arg=file:///tmp/x.git", initRows)
	}
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

	var m tea.Model = New(nil, plans, nil, config.Config{})
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
