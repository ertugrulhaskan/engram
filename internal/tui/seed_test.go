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
	out := m.buildSeedPrompt(claudeAssistant(t), "/home/me/proj", "/home/me/proj", driftDir(t), false)

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
	out := m.buildSeedPrompt(claudeAssistant(t), "/home/me/proj", "/home/me/proj", dir, false)
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
	out := m.buildSeedPrompt(claudeAssistant(t), projectsRoot(memDir), "/gone/old-project", memDir, true)

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

// seedLine returns the first line of a seed prompt that starts with prefix, or "".
func seedLine(out, prefix string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	return ""
}

// /files keeps instruction and index files read-only and its hint says to edit
// them with an assistant, so the prompt launched from there must put the
// selected file in scope, name it, and — when the file is inside the folders
// the session can write — list editing it among what the assistant can do.
// Every other source keeps the memories-and-plans scope and names no file.
func TestBuildSeedPromptFilesScope(t *testing.T) {
	files := toSource(t, "/files") // the global CLAUDE.md is selected
	it, ok := files.selected()
	if !ok {
		t.Fatal("setup: no doc selected in /files")
	}
	// Launched where the global doc lives (~/.claude), so it is writable.
	out := files.buildSeedPrompt(claudeAssistant(t), filepath.Dir(it.Path), "", "", false)
	if s := seedLine(out, "Scope:"); !strings.Contains(s, "instruction or index file selected") {
		t.Errorf("/files scope = %q, want it to include the selected instruction or index file", s)
	}
	if !strings.Contains(out, "Selected file: "+it.Path+"\n") {
		t.Errorf("/files prompt does not name the selected file %q:\n%s", it.Path, out)
	}
	if s := seedLine(out, "You can"); !strings.Contains(s, "edit the selected file") {
		t.Errorf("/files capability line = %q, want it to offer editing the selected file", s)
	}

	mem := modelWithMemory(t, t.TempDir(), t.TempDir())
	out = mem.buildSeedPrompt(claudeAssistant(t), t.TempDir(), "", "", false)
	s := seedLine(out, "Scope:")
	if s == "" || strings.Contains(s, "instruction") {
		t.Errorf("memories scope = %q, want memory files and plans only", s)
	}
	if strings.Contains(out, "Selected file:") || strings.Contains(out, "edit the selected file") {
		t.Errorf("memories prompt names or offers a selected file:\n%s", out)
	}
}

// The global ~/.gemini/GEMINI.md and ~/.codex/AGENTS.md sit outside every
// folder the session is opened with, and engram does not grant their
// directories (they hold each CLI's own config). So the prompt must not
// promise an edit there: it names the file, says the CLI may ask or refuse,
// and leaves editing out of what the assistant can do.
func TestBuildSeedPromptFilesOutsideSession(t *testing.T) {
	home := t.TempDir()
	doc := memory.DocFile{
		Path: filepath.Join(home, ".gemini", "GEMINI.md"), Title: "GEMINI.md", Body: "# global\n",
		Kind: memory.DocRules, Provider: memory.ProviderGemini, Scope: "global",
	}
	m0, _ := New(sampleMemories(), nil, []memory.DocFile{doc}, config.Config{}).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := switchVia(m0, "/files").(Model)
	if it, ok := m.selected(); !ok || it.Path != doc.Path {
		t.Fatalf("setup: selected %+v (ok=%v), want %s", it, ok, doc.Path)
	}

	// A global doc launches in ~/.claude with no add-dir; here that is home/.claude.
	out := m.buildSeedPrompt(claudeAssistant(t), filepath.Join(home, ".claude"), "", "", false)
	if s := seedLine(out, "Selected file:"); !strings.Contains(s, doc.Path) || !strings.Contains(s, "outside the folders") {
		t.Errorf("selected-file line = %q, want it to name %s and say it is outside the session", s, doc.Path)
	}
	if strings.Contains(out, "edit the selected file") {
		t.Errorf("prompt promises an edit the session cannot make:\n%s", out)
	}
}

// The provider note keeps a non-Claude assistant from reading Claude Code's
// files as its own rules. /files lists GEMINI.md itself, so there the note
// must not call the assistant's own file Claude Code's — but it still names
// CLAUDE.md, MEMORY.md, the memories and plans as Claude Code's data.
func TestBuildSeedPromptProviderNote(t *testing.T) {
	gemini, ok := findAssistant("gemini")
	if !ok {
		t.Fatal("the gemini assistant is missing from the registry")
	}
	const ownFile = "not Gemini's own GEMINI.md"
	const claudeFiles = "CLAUDE.md, MEMORY.md, the memory files and plans are Claude Code's, not Gemini's own instructions"

	var m Model // srcMemories
	if out := m.buildSeedPrompt(gemini, "/home/me/proj", "/home/me/proj", "", false); !strings.Contains(out, ownFile) {
		t.Errorf("memories prompt for @Gemini lacks the provider note:\n%s", out)
	}
	files := toSource(t, "/files") // a CLAUDE.md is selected: the case the note must still cover
	out := files.buildSeedPrompt(gemini, t.TempDir(), "", "", false)
	if strings.Contains(out, ownFile) {
		t.Errorf("/files prompt for @Gemini calls GEMINI.md Claude Code's file:\n%s", out)
	}
	if !strings.Contains(out, claudeFiles) {
		t.Errorf("/files prompt for @Gemini no longer marks Claude Code's files as Claude Code's:\n%s", out)
	}
}

// A /files row from a project Claude Code has never opened has a project dir
// but no memory dir. The handoff must launch in that project with no memory
// dir, not borrow the memory list's first project — which would put another
// project's memories, drift snapshot and --add-dir in front of the assistant.
func TestAssistantContextFilesDocWithoutMemoryDir(t *testing.T) {
	proj := t.TempDir()
	doc := memory.DocFile{
		Path: filepath.Join(proj, "AGENTS.md"), Title: "AGENTS.md", Body: "# rules\n",
		Kind: memory.DocRules, Provider: memory.ProviderAgents,
		Scope: "widgets", ProjectName: "widgets", ProjectDir: proj,
	}
	m0, _ := New(sampleMemories(), nil, []memory.DocFile{doc}, config.Config{}).Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := switchVia(m0, "/files").(Model)
	if it, ok := m.selected(); !ok || it.Path != doc.Path {
		t.Fatalf("setup: selected %+v (ok=%v), want %s", it, ok, doc.Path)
	}

	cwd, memDir, projDir, unresolved := m.assistantContext()
	if cwd != proj || memDir != "" || projDir != proj || unresolved {
		t.Errorf("cwd=%q memDir=%q projDir=%q unresolved=%v, want %q/\"\"/%q/false (no borrowed memory dir)", cwd, memDir, projDir, unresolved, proj, proj)
	}
	out := m.buildSeedPrompt(claudeAssistant(t), cwd, projDir, memDir, unresolved)
	if strings.Contains(out, "Memory directory:") {
		t.Errorf("prompt names a memory directory the project doesn't have:\n%s", out)
	}
	// The doc sits in the project, which is the launch dir: editing it is promised.
	if !strings.Contains(out, "edit the selected file") {
		t.Errorf("prompt does not offer editing a doc inside the launch dir:\n%s", out)
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
