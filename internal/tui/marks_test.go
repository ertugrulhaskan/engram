package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// space marks the row under the cursor and advances, so marking a run of rows
// takes one keystroke each rather than alternating space and j.
func TestMarkTogglesAndAdvances(t *testing.T) {
	var m tea.Model = ready(t)
	first, _ := m.(Model).selected()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	got := m.(Model)

	if !got.marks[first.Path] {
		t.Fatalf("space did not mark %q", first.Title)
	}
	if len(got.marks) != 1 {
		t.Errorf("marks = %d, want 1", len(got.marks))
	}
	if next, _ := got.selected(); next.Path == first.Path {
		t.Error("cursor did not advance after marking")
	}
}

// space on an already-marked row clears it — the same key both ways, so there is
// no separate unmark to discover.
func TestMarkToggleOff(t *testing.T) {
	var m tea.Model = ready(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace}) // mark + advance
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})    // back onto it
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace}) // unmark

	if n := len(m.(Model).marks); n != 0 {
		t.Errorf("marks = %d after toggling the same row twice, want 0", n)
	}
}

// Marking is a memories-only affordance: plans and the read-only files source
// have nothing to promote, so space there must not quietly accumulate state.
func TestMarkIgnoredOutsideMemories(t *testing.T) {
	var m tea.Model = ready(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = typeRunes(m, "/files")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.(Model); got.srcKind != srcFiles {
		t.Fatalf("setup: srcKind=%v, want srcFiles", got.srcKind)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if n := len(m.(Model).marks); n != 0 {
		t.Errorf("space marked %d rows in the files source, want 0", n)
	}
}

// esc clears marks before it clears a search filter. Marks are the more
// transient state, and leaving them set after an esc would arm a batch promote
// the user believes they dismissed.
func TestEscClearsMarksBeforeSearch(t *testing.T) {
	m := ready(t)
	m.search.SetValue("dev")
	m.marks = map[string]bool{"/x/a.md": true, "/x/b.md": true}

	var tm tea.Model = m
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := tm.(Model)

	if len(got.marks) != 0 {
		t.Errorf("marks = %d after esc, want 0", len(got.marks))
	}
	if got.search.Value() != "dev" {
		t.Errorf("esc also cleared the search (%q) — it should take two presses", got.search.Value())
	}
}

// The mark column exists only while something is marked, so every single-select
// view keeps the exact layout it had before batch promote existed.
func TestMarkColumnCollapsesWhenUnused(t *testing.T) {
	m := ready(t)
	if w := m.markColW(); w != 0 {
		t.Errorf("markColW = %d with nothing marked, want 0", w)
	}

	it, _ := m.selected()
	m.marks = map[string]bool{it.Path: true}
	if w := m.markColW(); w != runewidth.StringWidth(markGlyph)+1 {
		t.Errorf("markColW = %d, want glyph+gap = %d", w, runewidth.StringWidth(markGlyph)+1)
	}

	row := ansi.Strip(m.memRow(it, false, m.markColW(), m.badgeColW(), 0, 0, 0))
	if !strings.Contains(row, markGlyph) {
		t.Errorf("marked row does not draw the glyph:\n%q", row)
	}
}

// The mark column must be paid for out of the title, not out of the pane: a row
// is exactly as wide marked as unmarked. This is the drift the column would
// cause if nameW forgot to subtract it.
func TestMarkColumnDoesNotDriftRowWidth(t *testing.T) {
	m := ready(t)
	it, _ := m.selected()

	plain := ansi.Strip(m.memRow(it, false, 0, m.badgeColW(), 0, 0, 0))
	m.marks = map[string]bool{it.Path: true}
	marked := ansi.Strip(m.memRow(it, false, m.markColW(), m.badgeColW(), 0, 0, 0))

	if pw, mw := runewidth.StringWidth(plain), runewidth.StringWidth(marked); pw != mw {
		t.Errorf("row width drifted with the mark column: plain=%d marked=%d\n plain  %q\n marked %q", pw, mw, plain, marked)
	}
}

// A mark is a path, and the file under it can vanish — a delete, or an external
// change the reload poll picked up. A stale mark still satisfies the
// len(marks) > 0 test that routes promote into the batch, but the batch is built
// by walking m.memories, so promote answered "nothing marked is promotable" on
// every press until esc cleared them.
func TestReloadPrunesStaleMarks(t *testing.T) {
	m := ready(t)
	mems := sampleMemories()
	m.marks = map[string]bool{mems[0].Path: true, mems[1].Path: true}

	var tm tea.Model = m
	tm, _ = tm.Update(reloadMsg{mems: mems[1:], plans: samplePlans(), docs: sampleDocs()})
	got := tm.(Model)
	if got.marks[mems[0].Path] {
		t.Error("a mark survived its memory leaving the list")
	}
	if !got.marks[mems[1].Path] {
		t.Error("pruning dropped a mark whose memory is still listed")
	}
}

// The map is cleared outright when the last mark goes, matching toggleMarkList:
// nil is what collapses the mark column back to zero width, so a batch that
// emptied leaves no trace of itself in the list.
func TestReloadClearingLastMarkNilsTheMap(t *testing.T) {
	m := ready(t)
	mems := sampleMemories()
	m.marks = map[string]bool{mems[0].Path: true}
	m.batchItems = batchFixture()

	var tm tea.Model = m
	tm, _ = tm.Update(reloadMsg{mems: mems[1:], plans: samplePlans(), docs: sampleDocs()})
	got := tm.(Model)
	if got.marks != nil {
		t.Errorf("marks = %v, want nil once the last mark went", got.marks)
	}
	if got.batchItems != nil {
		t.Error("batchItems outlived the marks that armed them")
	}
}
