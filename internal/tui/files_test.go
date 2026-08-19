package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
)

// Ctrl+P then "/files" + Enter switches to the read-only files source, and the
// selected row is an instruction/index doc (Kind "rules" or "index").
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

// TestDocBadgesNameTheProvider pins the /files badge rule: Claude's own rules
// read "rules", the memory index reads "index", and every other vendor's
// instruction file reads its provider name. Without this, adding a provider
// silently falls through to "rules" and a project group shows three
// indistinguishable rows.
func TestDocBadgesNameTheProvider(t *testing.T) {
	projDir := "/Users/me/code/app"
	docs := []memory.DocFile{
		{Title: "CLAUDE.md", Kind: memory.DocRules, Provider: memory.ProviderClaude, Scope: "app", ProjectDir: projDir},
		{Title: "AGENTS.md", Kind: memory.DocRules, Provider: memory.ProviderAgents, Scope: "app", ProjectDir: projDir},
		{Title: "GEMINI.md", Kind: memory.DocRules, Provider: memory.ProviderGemini, Scope: "app", ProjectDir: projDir},
		{Title: "copilot-instructions.md", Kind: memory.DocRules, Provider: memory.ProviderCopilot, Scope: "app", ProjectDir: projDir},
		{Title: "MEMORY.md", Kind: memory.DocIndex, Provider: memory.ProviderClaude, Scope: "app", ProjectDir: projDir},
	}
	m := New(nil, nil, docs, config.Config{})
	items := m.docItems()
	if len(items) != len(docs) {
		t.Fatalf("docItems returned %d items, want %d", len(items), len(docs))
	}

	want := []string{"rules", "agents", "gemini", "copilot", "index"}
	for i, w := range want {
		if items[i].Badge != w {
			t.Errorf("%s badge = %q, want %q", docs[i].Title, items[i].Badge, w)
		}
	}

	// Badges must fit the column: the renderer caps it at badgeWidth and clips
	// anything longer, so a too-long provider name would be silently truncated.
	// Measured the way the renderer measures — display cells, not bytes.
	for _, it := range items {
		if w := runewidth.StringWidth(it.Badge); w > badgeWidth {
			t.Errorf("badge %q is %d cells, over the %d-cell cap — it will be clipped", it.Badge, w, badgeWidth)
		}
	}

	// The non-Claude rules files share one colour (the label carries which
	// vendor); Claude's own rules and the index are each distinct from it.
	vendor := items[1].BadgeColor
	for _, i := range []int{2, 3} {
		if items[i].BadgeColor != vendor {
			t.Errorf("%s badge colour = %q, want the shared vendor colour %q", docs[i].Title, items[i].BadgeColor, vendor)
		}
	}
	if items[0].BadgeColor == vendor {
		t.Errorf("CLAUDE.md badge colour matches the vendor colour %q — Claude's own rules must read differently", vendor)
	}
	if items[4].BadgeColor == vendor {
		t.Errorf("MEMORY.md badge colour matches the vendor colour %q — the index must read differently", vendor)
	}
}

// TestShortPathNoRepeatedProject covers the preview meta's location line. A file
// in the project ROOT has a parent dir whose base is the project name itself, so
// the generic "project/parent/file" shape doubled it — "acme/acme/AGENTS.md".
// That affected every project's own CLAUDE.md since the /files source shipped;
// it only became obvious once whole projects were listed from a scan root.
func TestShortPathNoRepeatedProject(t *testing.T) {
	cases := []struct {
		name string
		it   Item
		want string
	}{
		{"project-root rules file", Item{Path: "/code/acme/AGENTS.md", Context: "acme"}, "acme/AGENTS.md"},
		{"project-root CLAUDE.md", Item{Path: "/code/acme/CLAUDE.md", Context: "acme"}, "acme/CLAUDE.md"},
		{"nested keeps its parent", Item{Path: "/code/acme/.github/copilot-instructions.md", Context: "acme"}, "acme/.github/copilot-instructions.md"},
		{"memory keeps its parent", Item{Path: "/x/y/memory/thing.md", Context: "acme"}, "acme/memory/thing.md"},
		{"global doc", Item{Path: "/home/me/.claude/CLAUDE.md", Context: "global"}, "global/.claude/CLAUDE.md"},
		{"no context", Item{Path: "/code/acme/AGENTS.md"}, "AGENTS.md"},
		{"no path", Item{Context: "acme"}, ""},
	}
	for _, c := range cases {
		if got := shortPath(c.it); got != c.want {
			t.Errorf("%s: shortPath = %q, want %q", c.name, got, c.want)
		}
	}
}
