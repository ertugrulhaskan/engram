package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
	"github.com/ertugrulhaskan/engram/internal/plan"
)

func samplePlans() []plan.Plan {
	d := func(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }
	return []plan.Plan{
		{Title: "Auto-reload on disk changes", Body: "# Plan: Auto-reload\n\nwatch the fs\n", Path: "/Users/me/.claude/plans/auto-reload.md", Modified: d("2026-06-22")},
		{Title: "Themed redesign", Body: "# Plan: Themed redesign\n\nfive themes\n", Path: "/Users/me/.claude/plans/themed.md", Modified: d("2026-06-19")},
	}
}

func mem(title, desc string, t memory.Type, proj, path, date string) memory.Memory {
	mod, _ := time.Parse("2006-01-02", date)
	return memory.Memory{
		Title:       title,
		Description: desc,
		Type:        t,
		Body:        "# " + title + "\n\n" + desc + "\n\n- point one\n- point two\n",
		Path:        path,
		Modified:    mod,
		Project:     memory.Project{Name: proj, MemoryDir: path},
	}
}

func sampleMemories() []memory.Memory {
	return []memory.Memory{
		mem("dev-server-already-running", "the dev server is usually up on :3000", memory.TypeProject, "engram", "/Users/me/.claude/projects/-Users-me-code-engram/memory/a.md", "2024-01-06"),
		mem("prefers-terse-prose", "keep explanations short and skimmable", memory.TypeUser, "engram", "/Users/me/.claude/projects/-Users-me-code-engram/memory/b.md", "2024-04-12"),
		mem("no-ai-attribution", "never add Claude trailers to commits or PRs", memory.TypeFeedback, "global", "/Users/me/.claude/projects/-global/memory/c.md", "2025-02-01"),
		mem("roadmap-doc", "see ROADMAP.md for sequencing of v2 sharing", memory.TypeReference, "global", "/Users/me/.claude/projects/-global/memory/d.md", "2023-11-28"),
		mem("legacy-note", "hand-written memory with no frontmatter", memory.TypeUnknown, "webapp", "/Users/me/.claude/projects/-Users-me-code-webapp/memory/e.md", "2024-08-10"),
	}
}

func sampleDocs() []memory.DocFile {
	memDir := "/Users/me/.claude/projects/-Users-me-code-engram/memory"
	return []memory.DocFile{
		{Path: "/Users/me/.claude/CLAUDE.md", Title: "CLAUDE.md", Body: "# global rules\n", Kind: memory.DocRules, Scope: "global"},
		{Path: memDir + "/MEMORY.md", Title: "MEMORY.md", Body: "# index\n", Kind: memory.DocIndex, Scope: "engram", ProjectName: "engram", MemoryDir: memDir},
	}
}

func render(m Model, w, h int) string {
	um, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return um.(Model).View()
}

// TestRender is a smoke test for the View pipeline at several sizes. Run with
// `go test -v` to print the frames and eyeball the layout.
func TestRender(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // theme-switch persistence must not touch the real config
	m := New(sampleMemories(), samplePlans(), nil, config.Config{})

	for _, sz := range []struct{ w, h int }{{100, 30}, {80, 24}, {64, 22}} {
		out := render(m, sz.w, sz.h)
		if !strings.Contains(out, "engram") {
			t.Fatalf("view at %dx%d missing brand bar", sz.w, sz.h)
		}
		// No rendered row may exceed the terminal width.
		for _, line := range strings.Split(out, "\n") {
			if w := lipgloss.Width(line); w > sz.w {
				t.Errorf("line wider than %d (=%d): %q", sz.w, w, line)
			}
		}
		if testing.Verbose() {
			fmt.Printf("\n========== %dx%d ==========\n%s\n", sz.w, sz.h, out)
		}
	}

	// Grouped by type + Tokyo Night theme: exercises the right-aligned project
	// column and theme switching.
	var cur tea.Model = New(sampleMemories(), samplePlans(), nil, config.Config{})
	cur, _ = cur.Update(tea.WindowSizeMsg{Width: 100, Height: 26})
	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if testing.Verbose() {
		fmt.Printf("\n========== grouped by type · Tokyo Night ==========\n%s\n", cur.(Model).View())
	}
}
