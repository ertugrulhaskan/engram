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

// driftedProject writes a memory dir whose MEMORY.md is out of sync both ways:
// a.md exists with no index line (unindexed) and the index points at a missing
// gone.md (dangling). Returns the dir and the one memory living in it.
func driftedProject(t *testing.T, name string) (string, memory.Memory) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n\nhook\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"),
		[]byte("# Memory index\n\n- [Gone](gone.md) — x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, memory.Memory{
		Title:   "A " + name,
		Type:    memory.TypeProject,
		Path:    filepath.Join(dir, "a.md"),
		Body:    "# A\n\nhook\n",
		Project: memory.Project{Name: name, MemoryDir: dir},
	}
}

// A selected project whose MEMORY.md is out of sync shows the banner as the
// first list-pane row — project-named, cause-named, offering R — and pressing
// R reconciles the index on disk. The old top-bar pill is gone.
func TestDriftBannerAndRebuild(t *testing.T) {
	dir, mem := driftedProject(t, "acme")

	var m tea.Model = New([]memory.Memory{mem}, nil, nil, config.Config{})
	// Wide enough (listW 80) for the banner's fullest tier: project + both
	// causes + chip all on the row.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	got := m.(Model)
	if !got.driftOut || got.driftUnindexed != 1 || got.driftDangling != 1 {
		t.Fatalf("drift: got out=%v unindexed=%d dangling=%d, want true/1/1",
			got.driftOut, got.driftUnindexed, got.driftDangling)
	}
	if got.bannerRows() != 1 {
		t.Fatalf("bannerRows=%d, want 1", got.bannerRows())
	}
	out := got.View()
	// Three separate segments: the MEMORY.md tint splits the sentence with SGR
	// sequences, so a contiguous match across it would fail on styled output.
	for _, want := range []string{"acme: 1 file has no line in ", "MEMORY.md", " · 1 line has no file"} {
		if !strings.Contains(out, want) {
			t.Errorf("banner missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "[R reconcile]") {
		t.Errorf("banner missing the R chip:\n%s", out)
	}
	if strings.Contains(out, "index out of sync") {
		t.Errorf("retired top-bar pill wording still renders:\n%s", out)
	}
	// The banner is the first list-pane line: it renders above the first row.
	if b, r := strings.Index(out, "△"), strings.Index(out, "A acme"); b == -1 || r == -1 || b > r {
		t.Errorf("banner (at %d) not above the first list row (at %d)", b, r)
	}
	// The banner row carries WarnBg — derived via the same lipgloss calls, never
	// hex math (termenv truncates channels on its float round-trip).
	th := got.theme()
	if lead := onbg(th.Warn, th.WarnBg).Render(" △ "); !strings.Contains(out, lead) {
		t.Errorf("banner lead %q not painted Warn-on-WarnBg", lead)
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

// driftCause names the specific cause(s) so the banner is actionable, with
// real singular/plural agreement in every branch.
func TestDriftCause(t *testing.T) {
	cases := []struct {
		un, dang int
		want     string
	}{
		{2, 0, "2 files have no line in MEMORY.md"},
		{1, 0, "1 file has no line in MEMORY.md"},
		{0, 3, "3 MEMORY.md lines have no file"},
		{0, 1, "1 MEMORY.md line has no file"},
		{2, 1, "2 files have no line in MEMORY.md · 1 line has no file"},
		{1, 2, "1 file has no line in MEMORY.md · 2 lines have no file"},
	}
	for _, c := range cases {
		if got := driftCause(c.un, c.dang); got != c.want {
			t.Errorf("driftCause(%d,%d)=%q, want %q", c.un, c.dang, got, c.want)
		}
	}
}

// esc dismisses the banner for the session — but an active filter clears
// first, and the dismissal is per project, not global.
func TestDriftBannerEscDismiss(t *testing.T) {
	_, memA := driftedProject(t, "acme")
	_, memB := driftedProject(t, "widgets")

	var m tea.Model = New([]memory.Memory{memA, memB}, nil, nil, config.Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 30}) // wide: the project-name tier renders

	// Commit a filter that still matches both, then esc: the filter clears, the
	// banner stays — esc only reaches the banner when there is nothing to clear.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := m.(Model)
	if got.search.Value() != "" {
		t.Fatalf("first esc should clear the committed filter, still %q", got.search.Value())
	}
	if got.bannerRows() != 1 {
		t.Fatalf("banner should survive the filter-clearing esc")
	}

	// Second esc dismisses — for the selected project only.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got = m.(Model)
	if got.bannerRows() != 0 {
		t.Fatalf("esc did not dismiss the banner")
	}
	if !strings.Contains(got.View(), "A acme") {
		t.Fatalf("list should still render after dismissal")
	}

	// The other project's banner is untouched: select it and the banner is back.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	got = m.(Model)
	if it, ok := got.selected(); !ok || it.Context != "widgets" {
		t.Fatalf("expected the widgets row selected, got %+v ok=%v", it, ok)
	}
	if got.bannerRows() != 1 {
		t.Errorf("dismissal leaked across projects")
	}
	if !strings.Contains(got.View(), "widgets: ") {
		t.Errorf("banner should name the newly selected project")
	}
}

// The banner never appears in the plans or files sources, and — regression for
// the driftDir cache — it comes back when returning to memories.
func TestDriftBannerNeverInPlansAndSurvivesRoundTrip(t *testing.T) {
	_, mem := driftedProject(t, "acme")
	plans := []plan.Plan{{Title: "X", Body: "# Plan: X\n\nbody\n", Path: "/tmp/x.md"}}

	var m tea.Model = New([]memory.Memory{mem}, plans, nil, config.Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if m.(Model).bannerRows() != 1 {
		t.Fatalf("precondition: banner visible in memories")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab}) // → plans
	got := m.(Model)
	if got.srcKind != srcPlans {
		t.Fatalf("expected plans source, got %v", got.srcKind)
	}
	if got.bannerRows() != 0 || strings.Contains(got.View(), "[R reconcile]") {
		t.Errorf("banner rendered in the plans source")
	}

	// Round-trip back to memories: the cached driftDir must not mask the
	// recompute (the old code left driftOut=false forever after this).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab}) // → files (empty)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab}) // → memories
	got = m.(Model)
	if got.srcKind != srcMemories {
		t.Fatalf("expected memories source after the cycle, got %v", got.srcKind)
	}
	if got.bannerRows() != 1 {
		t.Errorf("banner lost after a plans/files round-trip (driftDir cache regression)")
	}
}

// What doesn't fit sheds in a fixed order: project name, then the R chip, and
// only then does the cause clip — the first drift direction stays legible at
// every realistic width instead of drowning in an early ellipsis.
func TestDriftBannerShedding(t *testing.T) {
	_, mem := driftedProject(t, "acme")
	var m tea.Model = New([]memory.Memory{mem}, nil, nil, config.Config{})

	// 140 cols → listW 56, avail 52: the full sentence (58) misses, the bare
	// cause (52) only fits chip-less — project and chip both shed.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	out := m.(Model).View()
	if strings.Contains(out, "acme: 1 file") {
		t.Errorf("project name should shed first at 140 cols:\n%s", out)
	}
	if strings.Contains(out, "[R reconcile]") {
		t.Errorf("chip should shed at 140 cols:\n%s", out)
	}
	for _, want := range []string{"1 file has no line in ", " · 1 line has no file"} {
		if !strings.Contains(out, want) {
			t.Errorf("cause should render whole at 140 cols, missing %q:\n%s", want, out)
		}
	}

	// 100 cols → listW 40, avail 36: even the cause clips, but its first
	// direction survives the ellipsis.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	out = m.(Model).View()
	if !strings.Contains(out, "1 file has no line in ") {
		t.Errorf("first drift direction lost at 100 cols:\n%s", out)
	}
	if !strings.Contains(out, "…") {
		t.Errorf("expected the clipped-cause tier at 100 cols:\n%s", out)
	}
}

// The banner borrows its row from the list window: with it visible the scroll
// window is one row shorter, and the frame height does not change.
func TestDriftBannerGeometry(t *testing.T) {
	_, mem := driftedProject(t, "acme")
	var m tea.Model = New([]memory.Memory{mem}, nil, nil, config.Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	got := m.(Model)

	withBanner := got.View()
	hWith := strings.Count(withBanner, "\n") + 1

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // dismiss
	got = m.(Model)
	withoutBanner := got.View()
	hWithout := strings.Count(withoutBanner, "\n") + 1

	if hWith != hWithout {
		t.Errorf("frame height changed with the banner: %d vs %d", hWith, hWithout)
	}
	// The frame convention is height-1 lines (a full-height frame would scroll
	// the alt-screen — see TestFrameNeverExceedsTerminal).
	if hWith != 23 {
		t.Errorf("frame height %d, want 23", hWith)
	}
}
