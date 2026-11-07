package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ertugrulhaskan/engram/internal/config"
	"github.com/ertugrulhaskan/engram/internal/memory"
)

func manyMemories(n int, proj string) []memory.Memory {
	out := make([]memory.Memory, n)
	for i := range out {
		out[i] = mem(fmt.Sprintf("m%03d", i), "d", memory.TypeProject, proj,
			fmt.Sprintf("/Users/me/.claude/projects/-%s/memory/m%03d.md", proj, i), "2024-01-01")
	}
	return out
}

func sized(t *testing.T, mems []memory.Memory, w, h int) Model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	um, _ := New(mems, nil, nil, config.Config{}).Update(tea.WindowSizeMsg{Width: w, Height: h})
	return um.(Model)
}

// D15: the palette held at most 64 characters, so ">init <url>" silently cut a
// long git URL to 58 and then ran init-team on the truncated one.
func TestPaletteHoldsALongInitURL(t *testing.T) {
	var m tea.Model = ready(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	cmd := ">init https://github.com/acme-engineering-org/engineering-team-claude-memories.git"
	m = typeRunes(m, cmd)
	if got := m.(Model).palette.Value(); got != cmd {
		t.Errorf("palette holds %q, want the whole command %q", got, cmd)
	}
}

// D16: with no memory selected (a filter that matches nothing), "n" opened the
// new-memory dialog and created the file in whichever project came first.
func TestNewNeedsASelectedProject(t *testing.T) {
	m := ready(t)
	m.search.SetValue("zzzz-matches-nothing")
	m.rebuildRows()
	if _, ok := m.selected(); ok {
		t.Fatal("precondition: nothing should be selected")
	}
	if dir := m.currentMemDir(); dir != "" {
		t.Errorf("currentMemDir() = %q with nothing selected; it must not guess a project", dir)
	}
	um, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if um.(Model).mode == modeNew {
		t.Error("n opened the new-memory dialog with no project selected")
	}
}

// D17: a palette jump to a memory the type filter hides did nothing — the
// cursor stayed where it was and the choice was dropped without a word.
func TestPaletteJumpClearsAHidingTypeFilter(t *testing.T) {
	m := ready(t)
	for i, tf := range typeCycle {
		if tf == memory.TypeFeedback {
			m.typeIdx = i
		}
	}
	m.rebuildRows()
	var tm tea.Model = m
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	tm = typeRunes(tm, "terse")
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := tm.(Model).selected()
	if !ok || got.Title != "prefers-terse-prose" {
		t.Errorf("after jumping to prefers-terse-prose the selection is %q (ok=%v)", got.Title, ok)
	}
}

// D18: the list's scroll offset only ever moved toward the cursor, so growing
// the window (or filtering the list down) left rows hidden above blank space.
func TestListScrollClampsWhenEverythingFits(t *testing.T) {
	m := sized(t, manyMemories(60, "acme"), 100, 24)
	m.cursor = len(m.rows) - 1
	m.ensureVisible()
	if m.top == 0 {
		t.Fatal("precondition: the list should be scrolled at 24 rows")
	}
	um, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 90})
	m = um.(Model)
	h := m.listRows() - m.bannerRows()
	if want := max(len(m.rows)-h, 0); m.top != want {
		t.Errorf("after growing the window top=%d, want %d (rows=%d, height=%d)", m.top, want, len(m.rows), h)
	}
}

// D19: when the selected memory disappears (deleted), the cursor jumped to the
// top of the list. It should land on the nearest memory above it.
func TestCursorStaysNearADeletedMemory(t *testing.T) {
	mems := append(manyMemories(3, "acme"), manyMemories(2, "widgets")...)
	m := sized(t, mems, 100, 30)
	// Select the last memory of the first group; remember the memory above it.
	last := -1
	for i, r := range m.rows {
		if r.kind == rowMemory && r.item.GroupKey == m.rows[m.firstMemRow()].item.GroupKey {
			last = i
		}
	}
	if last < 1 || m.rows[last-1].kind != rowMemory {
		t.Fatalf("precondition: the first group needs two memories (last=%d)", last)
	}
	m.cursor = last
	gone := m.rows[last].item.Path
	above := m.rows[last-1].item.Path
	var kept []memory.Memory
	for _, mm := range mems {
		if mm.Path != gone {
			kept = append(kept, mm)
		}
	}
	um, _ := m.Update(reloadMsg{mems: kept})
	if got, ok := um.(Model).selected(); !ok || got.Path != above {
		t.Errorf("after deleting the selected memory the cursor is on %q, want the one above it (%q)", got.Path, above)
	}
}

// D20: every reload (the 2s poll), theme switch or resize scrolled the preview
// back to the top while you were reading. Only a different memory should.
func TestPreviewKeepsScrollAcrossReload(t *testing.T) {
	long := mem("long-read", "d", memory.TypeProject, "acme", "/Users/me/.claude/projects/-acme/memory/long.md", "2024-01-01")
	long.Body = "# long-read\n\n" + strings.Repeat("A line of prose to scroll past.\n\n", 80)
	other := mem("other", "d", memory.TypeProject, "acme", "/Users/me/.claude/projects/-acme/memory/other.md", "2024-01-02")
	mems := []memory.Memory{long, other}
	m := sized(t, mems, 100, 30)
	m.selectByPath(long.Path)
	m.viewport.ScrollDown(10)
	if m.viewport.YOffset != 10 {
		t.Fatalf("precondition: preview should scroll (YOffset=%d)", m.viewport.YOffset)
	}
	um, _ := m.Update(reloadMsg{mems: mems})
	if got := um.(Model).viewport.YOffset; got != 10 {
		t.Errorf("a reload with the same memory moved the preview to YOffset=%d, want 10", got)
	}
	um, _ = um.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")}) // theme switch re-renders
	if got := um.(Model).viewport.YOffset; got != 10 {
		t.Errorf("a theme switch moved the preview to YOffset=%d, want 10", got)
	}
	m2 := um.(Model)
	m2.selectByPath(other.Path)
	if m2.viewport.YOffset != 0 {
		t.Errorf("selecting a different memory kept YOffset=%d, want the top", m2.viewport.YOffset)
	}
}

// Hardening: the drift banner prints a project name off disk; like every other
// untrusted line it must not carry a terminal escape sequence to the screen.
func TestDriftBannerSanitizesTheProjectName(t *testing.T) {
	m := ready(t)
	m.listW = 120 // wide enough that the project name is shown, not dropped
	m.driftUnindexed = []string{"x.md"}
	out := m.driftBanner(Item{Context: "a\x1b]8;;u\x07b"})
	if !strings.Contains(out, "a") || !strings.Contains(out, "b:") {
		t.Fatalf("precondition: the banner should show the project name: %q", out)
	}
	if strings.Contains(out, "\x1b]8;;") {
		t.Errorf("the banner passed an OSC sequence from the project name through: %q", out)
	}
}

// Narrowing the list with a search selects the first match, as before — the
// keep-your-place fallback is for a deleted memory only.
func TestSearchNarrowingSelectsTheFirstMatch(t *testing.T) {
	m := sized(t, manyMemories(30, "acme"), 100, 30)
	m.cursor = len(m.rows) - 3
	m.search.SetValue("m00") // leaves m000..m009
	m.rebuildRows()
	if got, ok := m.selected(); !ok || got.Title != "m000" {
		t.Errorf("after narrowing, the selection is %q (ok=%v), want the first match m000", got.Title, ok)
	}
}

// Deleting a memory in the middle of a group lands on the one above it, not
// the next one (which slides into the deleted row's index).
func TestCursorLandsAboveAMidListDeletion(t *testing.T) {
	mems := manyMemories(5, "acme")
	m := sized(t, mems, 100, 30)
	mid := m.firstMemRow() + 2
	gone, above := m.rows[mid].item.Path, m.rows[mid-1].item.Path
	m.cursor = mid
	var kept []memory.Memory
	for _, mm := range mems {
		if mm.Path != gone {
			kept = append(kept, mm)
		}
	}
	um, _ := m.Update(reloadMsg{mems: kept})
	if got, ok := um.(Model).selected(); !ok || got.Path != above {
		t.Errorf("after a mid-list delete the cursor is on %q, want the one above (%q)", got.Path, above)
	}
}
