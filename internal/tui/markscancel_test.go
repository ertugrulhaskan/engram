package tui

import (
	"strings"
	"testing"
)

// batchHeader plurals. Marking one memory and pressing P is ordinary, and the
// header is the first thing that batch shows.
func TestBatchHeaderPlurals(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want string
	}{
		{1, "promote 1 memory — pick a scope"},
		{2, "promote 2 memories — pick a scope"},
		{17, "promote 17 memories — pick a scope"},
	} {
		if got := batchHeader(tc.n); got != tc.want {
			t.Errorf("batchHeader(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// pruneMarks reports how many it dropped, so the caller can tell the marked set
// changed rather than having to diff it.
func TestPruneMarksReportsDropped(t *testing.T) {
	mems := sampleMemories()
	m := Model{marks: map[string]bool{mems[0].Path: true, "/gone/a.md": true, "/gone/b.md": true}}
	if got := m.pruneMarks(mems); got != 2 {
		t.Errorf("dropped = %d, want 2", got)
	}
	if len(m.marks) != 1 {
		t.Errorf("surviving marks = %d, want 1", len(m.marks))
	}
	// Nothing to drop reports zero and leaves the marks alone.
	if got := m.pruneMarks(mems); got != 0 {
		t.Errorf("second prune dropped = %d, want 0", got)
	}
}

// The last mark leaving clears the map entirely, which is what collapses the
// mark column back to zero width.
func TestPruneMarksClearsWhenEmptied(t *testing.T) {
	m := Model{marks: map[string]bool{"/gone/a.md": true}, batchItems: []batchItem{{path: "/gone/a.md"}}}
	if got := m.pruneMarks(sampleMemories()); got != 1 {
		t.Fatalf("dropped = %d, want 1", got)
	}
	if m.marks != nil || m.batchItems != nil {
		t.Errorf("marks=%v batchItems=%v, want both nil", m.marks, m.batchItems)
	}
}

// A reload that changes the marked set while the batch scope dialog is open must
// cancel the dialog. Left open with batchItems emptied, enter stops reading as a
// batch and promotes m.promotePath — unset for a batch — failing with "open :".
func TestReloadCancelsOpenBatchDialog(t *testing.T) {
	mems := sampleMemories()
	m := Model{
		mode:       modePromoteScope,
		marks:      map[string]bool{mems[0].Path: true, "/gone/a.md": true},
		batchItems: []batchItem{{path: mems[0].Path}, {path: "/gone/a.md"}},
	}
	nm, _ := m.Update(reloadMsg{mems: mems})
	got := nm.(Model)

	if got.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal — the dialog must not stay open", got.mode)
	}
	if got.batchItems != nil {
		t.Errorf("batchItems = %v, want nil", got.batchItems)
	}
	if !strings.Contains(got.status, "batch cancelled") {
		t.Errorf("status = %q, want it to say the batch was cancelled", got.status)
	}
}

// A reload that leaves the marked set intact must NOT disturb an open dialog —
// background reloads are frequent, and cancelling on every one would make the
// batch unusable.
func TestReloadLeavesIntactBatchAlone(t *testing.T) {
	mems := sampleMemories()
	m := Model{
		mode:       modePromoteScope,
		marks:      map[string]bool{mems[0].Path: true},
		batchItems: []batchItem{{path: mems[0].Path}},
	}
	nm, _ := m.Update(reloadMsg{mems: mems})
	got := nm.(Model)

	if got.mode != modePromoteScope {
		t.Errorf("mode = %v, want the dialog still open", got.mode)
	}
	if len(got.batchItems) != 1 {
		t.Errorf("batchItems = %d, want 1 — an untouched set must survive", len(got.batchItems))
	}
}
