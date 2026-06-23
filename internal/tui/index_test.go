package tui

import (
	"os"
	"path/filepath"
	"strings"
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
	if got := m.(Model); got.driftUnindexed != 1 || got.driftDangling != 1 {
		t.Fatalf("drift counts: got unindexed=%d dangling=%d, want 1/1", got.driftUnindexed, got.driftDangling)
	}
	// The top bar renders the warning badge (the cause wording itself is covered
	// by TestDriftSummary; the bar clips it on narrow terminals).
	if out := m.(Model).View(); !strings.Contains(out, "index out of sync") {
		t.Errorf("top bar missing drift warning:\n%s", out)
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

// driftSummary names the specific cause(s) of drift so the warning is actionable.
func TestDriftSummary(t *testing.T) {
	cases := []struct {
		un, dang int
		want     string
	}{
		{2, 0, "added without a MEMORY.md index line"},
		{0, 3, "deleted/renamed without updating MEMORY.md"},
		{1, 1, "added without an index line"},
	}
	for _, c := range cases {
		if got := driftSummary(c.un, c.dang); !strings.Contains(got, c.want) {
			t.Errorf("driftSummary(%d,%d)=%q, want substring %q", c.un, c.dang, got, c.want)
		}
	}
	// The both-case must mention the dangling cause too.
	if got := driftSummary(1, 1); !strings.Contains(got, "deleted/renamed") {
		t.Errorf("driftSummary(1,1)=%q, missing dangling cause", got)
	}
}
