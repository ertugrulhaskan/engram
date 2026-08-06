package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/team"
)

// The preview header follows the prototype: title first, the meta line
// (badge · short path · scope) under it.
func TestPreviewHeaderTitleFirst(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var cur tea.Model = New(sampleMemories(), nil, nil, config.Config{})
	cur, _ = cur.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := cur.(Model)
	it, ok := m.selected()
	if !ok {
		t.Fatal("no selection")
	}

	lines := strings.Split(ansi.Strip(m.previewPane()), "\n")
	if len(lines) < 2 {
		t.Fatalf("preview has %d lines", len(lines))
	}
	if !strings.Contains(lines[0], it.Title) {
		t.Errorf("first preview line %q does not carry the title %q", lines[0], it.Title)
	}
	if !strings.Contains(lines[1], it.Badge) || !strings.Contains(lines[1], shortPath(it)) {
		t.Errorf("second preview line %q missing badge %q or path %q", lines[1], it.Badge, shortPath(it))
	}
	// A personal row (no sync strip) keeps its edited stamp in the meta.
	if m.stripRows(it) == 0 && !strings.Contains(lines[1], "edited") {
		t.Errorf("personal meta %q lost the edited stamp", lines[1])
	}
}

// A shared row's meta shows badge · path · [scope] with no edited stamp —
// the sync strip right below carries the honest timestamp instead.
func TestPreviewMetaSharedRow(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := syncedModel(team.StateIncoming, nil)
	it, _ := m.selected()
	lines := strings.Split(ansi.Strip(m.previewPane()), "\n")
	meta := lines[1]
	for _, want := range []string{it.Badge, shortPath(it), "[global]"} {
		if !strings.Contains(meta, want) {
			t.Errorf("shared meta %q missing %q", meta, want)
		}
	}
	if strings.Contains(meta, "edited") {
		t.Errorf("shared meta %q duplicates the strip's stamp", meta)
	}
}

// shortPath derives the prototype's compact location from the real path.
func TestShortPath(t *testing.T) {
	cases := []struct {
		it   Item
		want string
	}{
		{Item{Path: "/u/x/.claude/projects/-u-app/memory/a.md", Context: "app"}, "app/memory/a.md"},
		{Item{Path: "/plans/p.md", Context: ""}, "p.md"},
		{Item{Path: "", Context: "app"}, ""},
	}
	for _, c := range cases {
		if got := shortPath(c.it); got != c.want {
			t.Errorf("shortPath(%q,%q)=%q, want %q", c.it.Path, c.it.Context, got, c.want)
		}
	}
}

// List type badges are bare colored words, not bracketed.
func TestListBadgeUnbracketed(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var cur tea.Model = New(sampleMemories(), nil, nil, config.Config{})
	cur, _ = cur.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := cur.(Model)

	row := ansi.Strip(m.memRow(Item{Badge: "project", BadgeColor: m.theme().OK, Title: "X"}, false, m.badgeColW(), 0, 0, 0))
	if !strings.Contains(row, "project") || strings.Contains(row, "[project]") {
		t.Errorf("list row %q should carry a bare badge word", row)
	}
}
