package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/plan"
)

func typeRunes(m tea.Model, s string) tea.Model {
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return m
}

// The empty palette is the full sectioned list: every memory and plan under
// "Jump to" (first row preselected), then Sources, Team, and Assistant — the
// prefix guide is retired; nothing needs a prefix to be reachable.
func TestPaletteSectionedEmpty(t *testing.T) {
	var m tea.Model = ready(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	got := m.(Model)
	rows := got.palRows

	wantRows := 5 + 2 + len(got.paletteCommands()) + len(got.teamVerbs()) + len(got.assistantProviders())
	if len(rows) != wantRows {
		t.Fatalf("empty palette = %d rows, want %d (jump + commands + verbs + assistant)", len(rows), wantRows)
	}
	if got.palCursor != 0 || rows[0].action != palJump {
		t.Fatalf("first row not a preselected jump row (cursor=%d action=%v)", got.palCursor, rows[0].action)
	}
	// Sections appear contiguously in display order.
	order := map[string]int{palSecJump: 0, palSecSources: 1, palSecTeam: 2, palSecAssistant: 3}
	last := -1
	for i, r := range rows {
		o, ok := order[r.section]
		if !ok {
			t.Fatalf("row %d has unknown section %q", i, r.section)
		}
		if o < last {
			t.Fatalf("row %d section %q out of order", i, r.section)
		}
		last = o
	}
	// The rendered box shows section headers and the header hint.
	box := got.paletteBox()
	for _, want := range []string{palSecJump, palSecSources, palHint} {
		if !strings.Contains(box, want) {
			t.Errorf("palette box missing %q", want)
		}
	}
	// Enter on the preselected row jumps to the first memory.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	after := m.(Model)
	if after.mode != modeNormal || after.srcKind != srcMemories {
		t.Fatalf("enter on jump row: mode=%v src=%v, want normal/memories", after.mode, after.srcKind)
	}
}

// The "memory" command became "memories"; the old spelling and prefixes keep
// working via the alias ("memory" is not a string prefix of "memories").
func TestPaletteMemoriesAlias(t *testing.T) {
	for _, q := range []string{"/memories", "/memory", "/mem"} {
		var m tea.Model = ready(t)
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
		m = typeRunes(m, q)
		rows := m.(Model).palRows
		found := false
		for _, r := range rows {
			if r.action == palSwitch && r.src == srcMemories {
				found = true
			}
		}
		if !found {
			t.Errorf("%q did not list the memories command: %+v", q, rows)
		}
	}
}

// A bare query fuzzy-matches project names too, and surfaces team verbs and
// the assistant without their prefixes.
func TestPaletteBareQueryBreadth(t *testing.T) {
	var m tea.Model = ready(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	rows := typeRunes(m, "webapp").(Model).palRows // a project name, not a title
	if len(rows) == 0 || rows[0].action != palJump || !strings.Contains(rows[0].label, "legacy-note") {
		t.Errorf("project-name query = %+v, want the webapp memory first", rows)
	}

	m, _ = ready(t).Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	found := false
	for _, r := range typeRunes(m, "prom").(Model).palRows {
		if r.action == palPromote {
			found = true
		}
	}
	if !found {
		t.Error("bare \"prom\" did not surface the promote verb")
	}

	m, _ = ready(t).Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	found = false
	for _, r := range typeRunes(m, "cla").(Model).palRows {
		if r.action == palAssistant {
			found = true
		}
	}
	if !found {
		t.Error("bare \"cla\" did not surface the assistant")
	}
}

// The Jump-to section is capped so a large corpus doesn't swamp the palette.
func TestPaletteJumpCap(t *testing.T) {
	mems := make([]memory.Memory, 0, palJumpCap+10)
	for i := 0; i < palJumpCap+10; i++ {
		mems = append(mems, mem("note-"+string(rune('a'+i%26))+string(rune('0'+i/26)), "d", memory.TypeProject,
			"acme", "/Users/me/.claude/projects/-acme/memory/n"+string(rune('a'+i%26))+string(rune('0'+i/26))+".md", "2024-01-06"))
	}
	var m tea.Model = New(mems, nil, nil, config.Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	got := m.(Model)
	jump := 0
	for _, r := range got.palRows {
		if r.section == palSecJump {
			jump++
		}
	}
	if jump != palJumpCap {
		t.Errorf("jump section = %d rows, want the %d cap", jump, palJumpCap)
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
