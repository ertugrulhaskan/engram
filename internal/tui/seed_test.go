package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
)

// driftDir builds a memory dir with one unindexed file (a.md) and one dangling
// index entry (gone.md), mirroring the memory package's drift fixtures.
func driftDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "memory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte("# index\n\n- [Gone](gone.md) — hook\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// modelWithMemory builds a ready model holding one memory whose project/memory
// dirs are the given paths, with that memory selected.
func modelWithMemory(t *testing.T, projDir, memDir string) Model {
	t.Helper()
	mm := memory.Memory{
		Title: "m", Body: "# m\n", Path: filepath.Join(memDir, "m.md"),
		Project: memory.Project{Name: "m", Dir: projDir, MemoryDir: memDir},
	}
	m, _ := New([]memory.Memory{mm}, nil, nil, config.Config{}).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	got := m.(Model)
	got.selectByPath(mm.Path)
	return got
}

// "@" surfaces only the assistant providers; "@cla" still matches "claude";
// a non-matching suffix yields nothing, and no fuzzy-jump memory rows leak in.
//
// Every provider is stubbed as installed, so the row count is the registry's
// and not a fact about whichever CLIs this machine happens to have — the palette
// otherwise lists only installed ones, which made this assertion machine-dependent.
func TestPaletteAssistant(t *testing.T) {
	old := lookPath
	defer func() { lookPath = old }()
	lookPath = func(bin string) string { return "/usr/bin/" + bin }

	var m tea.Model = ready(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = typeRunes(m, "@")
	got := m.(Model)
	if len(got.palRows) != len(assistants) {
		t.Fatalf("'@' produced %d rows, want %d (no fuzzy leak): %+v", len(got.palRows), len(assistants), got.palRows)
	}
	for _, r := range got.palRows {
		if r.action != palAssistant || r.provider == "" {
			t.Fatalf("'@' row = %+v, want a palAssistant row naming a provider", r)
		}
	}

	m = typeRunes(m, "cla") // input is now "@cla"
	if rows := m.(Model).palRows; len(rows) != 1 || rows[0].provider != "claude" {
		t.Fatalf("'@cla' rows = %+v, want one claude row", rows)
	}

	m = typeRunes(m, "xx") // "@claxx" — no provider has that prefix
	if rows := m.(Model).palRows; len(rows) != 0 {
		t.Fatalf("'@claxx' rows = %+v, want none", rows)
	}
}

// Selecting @Claude from the palette dispatches the assistant and closes the
// palette; with claude missing it surfaces a danger status instead of crashing.
func TestPaletteAssistantDispatchMissingBinary(t *testing.T) {
	old := lookPath
	defer func() { lookPath = old }()
	lookPath = func(string) string { return "" }

	var m tea.Model = ready(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = typeRunes(m, "@")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := m.(Model)
	if got.mode != modeNormal {
		t.Errorf("palette did not close (mode=%v)", got.mode)
	}
	if got.statusKind != statusDanger || !strings.Contains(got.status, "claude") {
		t.Errorf("status = %q (kind=%v), want a danger message about claude", got.status, got.statusKind)
	}
}

func TestBuildSeedPromptDrift(t *testing.T) {
	var m Model // srcKind defaults to srcMemories
	out := m.buildSeedPrompt(claudeAssistant(t), "/home/me/proj", driftDir(t), false)

	for _, want := range []string{"out of sync", "a.md", "gone.md", "Ask before editing", "/home/me/proj"} {
		if !strings.Contains(out, want) {
			t.Errorf("seed prompt missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "orphaned") {
		t.Errorf("non-orphan prompt should not mention orphaned:\n%s", out)
	}
}

func TestBuildSeedPromptInSync(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte("# index\n\n- [A](a.md) — hook\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var m Model
	out := m.buildSeedPrompt(claudeAssistant(t), "/home/me/proj", dir, false)
	if !strings.Contains(out, "in sync") {
		t.Errorf("expected 'in sync' phrasing:\n%s", out)
	}
	if strings.Contains(out, "out of sync") {
		t.Errorf("in-sync dir falsely reported out of sync:\n%s", out)
	}
}

func TestBuildSeedPromptUnresolved(t *testing.T) {
	memDir := driftDir(t)
	var m Model
	out := m.buildSeedPrompt(claudeAssistant(t), "/gone/old-project", memDir, true)

	for _, want := range []string{"couldn't resolve", "renamed or moved", "/gone/old-project", "relocate"} {
		if !strings.Contains(out, want) {
			t.Errorf("unresolved prompt missing %q:\n%s", want, out)
		}
	}
	// It must not assert as fact that the memories are misplaced.
	if strings.Contains(out, "are now orphaned") {
		t.Errorf("unresolved prompt should not assert the memories are orphaned:\n%s", out)
	}
}

func TestAssistantContext(t *testing.T) {
	memDir := filepath.Join(t.TempDir(), "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Project dir exists → launch there, resolved.
	proj := t.TempDir()
	m := modelWithMemory(t, proj, memDir)
	cwd, md, _, unresolved := m.assistantContext()
	if cwd != proj || md != memDir || unresolved {
		t.Errorf("present project: cwd=%q md=%q unresolved=%v, want %q/%q/false", cwd, md, unresolved, proj, memDir)
	}

	// Project dir unresolvable (renamed/moved or undecodable) → launch in the
	// ~/.claude/projects root (two levels up from the memory dir), not $HOME.
	gone := filepath.Join(t.TempDir(), "gone")
	wantRoot := filepath.Dir(filepath.Dir(memDir))
	m2 := modelWithMemory(t, gone, memDir)
	cwd2, md2, _, unresolved2 := m2.assistantContext()
	if cwd2 != wantRoot || md2 != memDir || !unresolved2 {
		t.Errorf("unresolved project: cwd=%q md=%q unresolved=%v, want %q/%q/true", cwd2, md2, unresolved2, wantRoot, memDir)
	}
	if !within(memDir, cwd2) {
		t.Errorf("memDir %q should be within fallback cwd %q (so no redundant --add-dir)", memDir, cwd2)
	}
}

// On a clean exit the assistant handler resets the drift cache and reloads; on
// error it surfaces a danger status and still reloads. Both messages name the
// provider that ran, so a non-claude label is used here — the wording must come
// from the message, not from a hardcoded "claude".
func TestAssistantFinishedReloads(t *testing.T) {
	m := ready(t)
	m.driftDir = "stale"
	m2, cmd := m.Update(assistantFinishedMsg{})
	got := m2.(Model)
	if got.driftDir != "" {
		t.Errorf("driftDir = %q, want reset", got.driftDir)
	}
	if cmd == nil {
		t.Error("expected a reload command")
	}
	if got.status == "" {
		t.Error("expected a status message after a clean exit")
	}

	m3, _ := ready(t).Update(assistantFinishedMsg{label: "@Gemini", err: errors.New("boom")})
	bad := m3.(Model)
	if bad.statusKind != statusDanger || !strings.Contains(bad.status, "@Gemini") {
		t.Errorf("error exit status = %q (kind=%v), want a danger message", bad.status, bad.statusKind)
	}
}
