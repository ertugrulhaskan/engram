package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
)

// A selected project whose MEMORY.md is out of sync flags driftOut, and pressing
// R reconciles the index on disk.
func TestDriftFlagAndRebuild(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n\nhook\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Index references a missing file (dangling) and omits a.md (unindexed).
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"),
		[]byte("# Memory index\n\n- [Gone](gone.md) — x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mems := []memory.Memory{{
		Title:   "A",
		Type:    memory.TypeProject,
		Path:    filepath.Join(dir, "a.md"),
		Body:    "# A\n\nhook\n",
		Project: memory.Project{Name: "p", MemoryDir: dir},
	}}

	var m tea.Model = New(mems, nil, config.Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !m.(Model).driftOut {
		t.Fatalf("expected driftOut=true (a.md unindexed, gone.md dangling)")
	}

	// R reconciles synchronously inside the handler.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	un, dang, err := memory.IndexDrift(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(un) != 0 || len(dang) != 0 {
		t.Errorf("after R the index still drifts: unindexed=%v dangling=%v", un, dang)
	}
}
