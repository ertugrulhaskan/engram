package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Ctrl+P then "/files" + Enter switches to the read-only files source, and the
// selected row is a CLAUDE.md / MEMORY.md doc (Kind "rules" or "index").
func TestPaletteFilesSwitch(t *testing.T) {
	var m tea.Model = ready(t) // ready() seeds sampleDocs()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = typeRunes(m, "/files")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := m.(Model)
	if got.srcKind != srcFiles {
		t.Fatalf("srcKind=%v, want srcFiles", got.srcKind)
	}
	it, ok := got.selected()
	if !ok || (it.Kind != "rules" && it.Kind != "index") {
		t.Fatalf("selected is not a doc: %+v (ok=%v)", it, ok)
	}
}

// Launching @Claude from /files on the GLOBAL CLAUDE.md (which has no project of
// its own) must NOT borrow an unrelated project from the memory list: it launches
// in ~/.claude with an empty memDir/projDir.
func TestAssistantContextGlobalDoc(t *testing.T) {
	var m tea.Model = ready(t) // memories present + sampleDocs (global CLAUDE.md first)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = typeRunes(m, "/files")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := m.(Model)
	it, ok := got.selected()
	if !ok || it.Title != "CLAUDE.md" || it.Context != "global" {
		t.Fatalf("expected the global CLAUDE.md selected, got %+v (ok=%v)", it, ok)
	}
	cwd, memDir, projDir, unresolved := got.assistantContext()
	if memDir != "" || projDir != "" || unresolved {
		t.Errorf("global doc: memDir=%q projDir=%q unresolved=%v, want empty/empty/false (must not borrow a project)", memDir, projDir, unresolved)
	}
	if cwd != claudeHome() {
		t.Errorf("global doc cwd=%q, want claudeHome %q", cwd, claudeHome())
	}
}

// In the files source, e and d are read-only: they surface a hint pointing at
// @Claude, never open the editor or the delete-confirm modal.
func TestFilesReadOnly(t *testing.T) {
	toFiles := func() Model {
		var m tea.Model = ready(t)
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
		m = typeRunes(m, "/files")
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		return m.(Model)
	}

	for _, key := range []string{"e", "d"} {
		m := toFiles()
		var cmd tea.Cmd
		var tm tea.Model = m
		tm, cmd = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		got := tm.(Model)
		if got.mode != modeNormal {
			t.Errorf("key %q changed mode to %v in files source (should stay normal/read-only)", key, got.mode)
		}
		if got.status == "" {
			t.Errorf("key %q gave no read-only hint", key)
		}
		_ = cmd
	}
}
